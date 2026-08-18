package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/mikael/dpgmedia/internal/id"
	"github.com/mikael/dpgmedia/internal/message"
	"github.com/mikael/dpgmedia/internal/store"
)

type createMessageRequest struct {
	UserID      string       `json:"user_id"`
	MessageType message.Type `json:"message_type"`
	TextContent string       `json:"text_content"`
	MediaID     string       `json:"media_id"`
}

type createMessageData struct {
	MessageID string `json:"message_id"`
	CreatedAt string `json:"created_at"`
}

type messagesHandler struct {
	maxBodyBytes int64
	store        store.MessageStore
}

func (h *messagesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	msg, err := h.store.Create(ctx, message.Message{
		ID:          id.New("msg"),
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

func (h *messagesHandler) decode(w http.ResponseWriter, r *http.Request) (createMessageRequest, details) {
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

func (r *createMessageRequest) validate() details {
	var errs details

	if r.UserID == "" {
		errs.add("user_id", issueRequired, "Required field")
	}

	messageType, err := message.ParseType(string(r.MessageType))
	if err != nil {
		errs.add("message_type", issueInvalid, "Must be one of "+message.TypeNames)
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
