package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mikael/dpgmedia/internal/domain"
	"github.com/mikael/dpgmedia/internal/store"
)

type createMessageRequest struct {
	UserID      string      `json:"user_id"`
	MessageType domain.Type `json:"message_type"`
	TextContent string      `json:"text_content"`
	MediaID     string      `json:"media_id"`
}

type createMessageData struct {
	MessageID string `json:"message_id"`
	CreatedAt string `json:"created_at"`
}

type createMessageHandler struct {
	maxBodyBytes int64
	store        store.MessageStore
}

func (h *createMessageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := loggerFrom(ctx)

	req, errs := h.decode(w, r)
	if len(errs) > 0 {
		logger.Warn("rejected message payload", slog.Any("details", errs))
		writeError(ctx, w, errInvalidRequest, errs)
		return
	}

	if errs := req.validate(); len(errs) > 0 {
		logger.Warn("rejected message payload",
			slog.String("user_id", req.UserID),
			slog.String("message_type", string(req.MessageType)),
			slog.Any("details", errs),
		)
		writeError(ctx, w, errValidation, errs)
		return
	}

	msg, err := h.store.Create(ctx, domain.Message{
		ID:          domain.NewMessageID(),
		UserID:      req.UserID,
		Type:        req.MessageType,
		TextContent: req.TextContent,
		MediaID:     req.MediaID,
	})
	if err != nil {
		logger.Error("failed to store message",
			slog.String("user_id", req.UserID),
			slog.String("message_type", string(req.MessageType)),
			slog.Any("error", err),
		)
		writeError(ctx, w, errInternal, nil)
		return
	}

	logger.Info("message stored",
		slog.String("message_id", msg.ID),
		slog.String("user_id", msg.UserID),
		slog.String("message_type", string(msg.Type)),
		slog.String("media_id", msg.MediaID),
		slog.Int("text_length", len(msg.TextContent)),
		slog.Time("expires_at", msg.ExpiresAt),
	)

	writeSuccess(ctx, w, http.StatusCreated, createMessageData{
		MessageID: msg.ID,
		CreatedAt: msg.CreatedAt.Format(time.RFC3339),
	})
}

func (h *createMessageHandler) decode(w http.ResponseWriter, r *http.Request) (createMessageRequest, details) {
	var req createMessageRequest
	var errs details

	if ct := r.Header.Get("Content-Type"); ct != "" {
		if mediaType := strings.TrimSpace(strings.Split(ct, ";")[0]); !strings.EqualFold(mediaType, "application/json") {
			errs.add("content_type", issueUnsupported,
				fmt.Sprintf("Unsupported content type %q, expected application/json", mediaType))
			return req, errs
		}
	}

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, h.maxBodyBytes))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		switch {
		case errors.Is(err, io.EOF):
			errs.add("body", issueEmpty, "Request body is empty")
		case errors.As(err, &maxErr):
			errs.add("body", issueTooLarge, fmt.Sprintf("Request body exceeds %d bytes", h.maxBodyBytes))
		default:
			errs.add("body", issueMalformed, fmt.Sprintf("Malformed JSON body: %s", err))
		}
		return req, errs
	}

	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		errs.add("body", issueMalformed, "Body must contain exactly one JSON object")
		return req, errs
	}

	req.UserID = strings.TrimSpace(req.UserID)
	req.MediaID = strings.TrimSpace(req.MediaID)

	return req, nil
}

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

type messageData struct {
	MessageID   string      `json:"message_id"`
	UserID      string      `json:"user_id"`
	MessageType domain.Type `json:"message_type"`
	TextContent string      `json:"text_content,omitempty"`
	MediaID     string      `json:"media_id,omitempty"`
	CreatedAt   string      `json:"created_at"`
	ExpiresAt   string      `json:"expires_at,omitempty"`
}

func newMessageData(msg domain.Message) messageData {
	data := messageData{
		MessageID:   msg.ID,
		UserID:      msg.UserID,
		MessageType: msg.Type,
		TextContent: msg.TextContent,
		MediaID:     msg.MediaID,
		CreatedAt:   msg.CreatedAt.Format(time.RFC3339),
	}
	if !msg.ExpiresAt.IsZero() {
		data.ExpiresAt = msg.ExpiresAt.Format(time.RFC3339)
	}
	return data
}

type listMessagesData struct {
	Messages []messageData `json:"messages"`
	Meta     listMeta      `json:"meta"`
}

type listMeta struct {
	Count int `json:"count"`
	Limit int `json:"limit"`
}

type listMessagesHandler struct {
	store store.MessageStore
}

func (h *listMessagesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := loggerFrom(ctx)

	limit, errs := parseLimit(r.URL.Query().Get("limit"))
	if len(errs) > 0 {
		logger.Warn("rejected list request", slog.Any("details", errs))
		writeError(ctx, w, errInvalidRequest, errs)
		return
	}

	messages, err := h.store.List(ctx, limit)
	if err != nil {
		logger.Error("failed to list messages", slog.Int("limit", limit), slog.Any("error", err))
		writeError(ctx, w, errInternal, nil)
		return
	}

	data := listMessagesData{
		Messages: make([]messageData, 0, len(messages)),
		Meta:     listMeta{Count: len(messages), Limit: limit},
	}
	for _, msg := range messages {
		data.Messages = append(data.Messages, newMessageData(msg))
	}

	logger.Debug("messages listed", slog.Int("limit", limit), slog.Int("count", data.Meta.Count))

	writeSuccess(ctx, w, http.StatusOK, data)
}

func parseLimit(raw string) (int, details) {
	var errs details

	if strings.TrimSpace(raw) == "" {
		return defaultListLimit, nil
	}

	limit, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		errs.add("limit", issueInvalid, fmt.Sprintf("Must be an integer between 1 and %d", maxListLimit))
		return 0, errs
	}

	if limit < 1 || limit > maxListLimit {
		errs.add("limit", issueInvalid, fmt.Sprintf("Must be between 1 and %d", maxListLimit))
		return 0, errs
	}

	return limit, nil
}

func (r *createMessageRequest) validate() details {
	var errs details

	if r.UserID == "" {
		errs.add("user_id", issueRequired, "Required field")
	}

	messageType, err := domain.ParseType(string(r.MessageType))
	if err != nil {
		errs.add("message_type", issueInvalid, "Must be one of "+domain.TypeNames)
		return errs
	}
	r.MessageType = messageType

	if r.MessageType.RequiresMedia() {
		if r.MediaID == "" {
			errs.add("media_id", issueRequired, fmt.Sprintf("Required field for message_type %s", r.MessageType))
		}
		return errs
	}

	if strings.TrimSpace(r.TextContent) == "" {
		errs.add("text_content", issueRequired, "Required field for message_type TEXT")
	}
	if r.MediaID != "" {
		errs.add("media_id", issueNotAllowed, "Must be empty for message_type TEXT")
	}
	return errs
}
