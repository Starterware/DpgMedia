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
)

type messageType string

const (
	messageTypeText    messageType = "TEXT"
	messageTypeAudio   messageType = "AUDIO"
	messageTypeVideo   messageType = "VIDEO"
	messageTypePicture messageType = "PICTURE"
)

func (t messageType) valid() bool {
	switch t {
	case messageTypeText, messageTypeAudio, messageTypeVideo, messageTypePicture:
		return true
	default:
		return false
	}
}

func (t messageType) requiresMedia() bool {
	return t != messageTypeText
}

type createMessageRequest struct {
	UserID      string      `json:"user_id"`
	MessageType messageType `json:"message_type"`
	TextContent string      `json:"text_content"`
	MediaID     string      `json:"media_id"`
}

type createMessageData struct {
	MessageID string `json:"message_id"`
	CreatedAt string `json:"created_at"`
}

type messagesHandler struct {
	maxBodyBytes int64
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

	data := createMessageData{
		MessageID: id.New("msg"),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	logger.Info("message accepted",
		slog.String("message_id", data.MessageID),
		slog.String("user_id", req.UserID),
		slog.String("message_type", string(req.MessageType)),
		slog.String("media_id", req.MediaID),
		slog.Int("text_length", len(req.TextContent)),
	)

	writeSuccess(ctx, w, http.StatusCreated, data)
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
	req.MessageType = messageType(strings.ToUpper(strings.TrimSpace(string(req.MessageType))))

	return req, nil
}

func (r createMessageRequest) validate() details {
	var errs details

	if r.UserID == "" {
		errs.add("user_id", issueRequired, "Required field")
	}

	if !r.MessageType.valid() {
		errs.add("message_type", issueInvalid, "Must be one of TEXT, AUDIO, VIDEO, PICTURE")
		return errs
	}

	if r.MessageType.requiresMedia() {
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
