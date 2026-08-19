package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

type successResponse struct {
	Data any `json:"data"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code      errorCode     `json:"code"`
	Message   string        `json:"message"`
	Details   []errorDetail `json:"details,omitempty"`
	Timestamp string        `json:"timestamp"`
	RequestID string        `json:"request_id"`
}

type errorDetail struct {
	Field       string    `json:"field"`
	Issue       issueCode `json:"issue"`
	Description string    `json:"description,omitempty"`
}

type issueCode string

const (
	issueRequired    issueCode = "REQUIRED"
	issueNotAllowed  issueCode = "NOT_ALLOWED"
	issueNotFound    issueCode = "NOT_FOUND"
	issueInvalid     issueCode = "INVALID_VALUE"
	issueMalformed   issueCode = "MALFORMED"
	issueEmpty       issueCode = "EMPTY"
	issueTooLarge    issueCode = "TOO_LARGE"
	issueUnsupported issueCode = "UNSUPPORTED"
)

type errorCode string

type errorKind struct {
	code    errorCode
	message string
	status  int
}

var (
	errInvalidRequest = errorKind{"INVALID_REQUEST", "Invalid request", http.StatusBadRequest}
	errValidation     = errorKind{"VALIDATION_ERROR", "Validation error", http.StatusUnprocessableEntity}
	errInternal       = errorKind{"INTERNAL_ERROR", "Internal server error", http.StatusInternalServerError}
)

type details []errorDetail

func (d *details) add(field string, issue issueCode, description string) {
	*d = append(*d, errorDetail{Field: field, Issue: issue, Description: description})
}

const jsonContentType = "application/json; charset=utf-8"

// fallbackErrorBody replaces a payload that could not be encoded. It is a
// literal because building a richer body would mean trusting the encoder that
// just failed.
const fallbackErrorBody = `{"error":{"code":"INTERNAL_ERROR","message":"Internal server error"}}`

func writeJSON(ctx context.Context, w http.ResponseWriter, status int, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		loggerFrom(ctx).Error("failed to encode response body", slog.Any("error", err))
		status, body = errInternal.status, []byte(fallbackErrorBody)
	}

	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)

	if _, err := w.Write(body); err != nil {
		loggerFrom(ctx).Warn("failed to write response body", slog.Any("error", err))
	}
}

func writeSuccess(ctx context.Context, w http.ResponseWriter, status int, data any) {
	writeJSON(ctx, w, status, successResponse{Data: data})
}

func writeError(ctx context.Context, w http.ResponseWriter, kind errorKind, errs details) {
	writeJSON(ctx, w, kind.status, errorResponse{Error: errorBody{
		Code:      kind.code,
		Message:   kind.message,
		Details:   errs,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		RequestID: requestIDFrom(ctx),
	}})
}
