package api

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingWriter struct {
	http.ResponseWriter
	err error
}

func (f failingWriter) Write([]byte) (int, error) {
	return 0, f.err
}

func newCapturingContext() (context.Context, *bytes.Buffer) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return withLogger(context.Background(), logger), &logs
}

func TestWriteJSONUnencodablePayload(t *testing.T) {
	ctx, logs := newCapturingContext()
	rec := httptest.NewRecorder()

	writeJSON(ctx, rec, http.StatusOK, map[string]any{"channel": make(chan int)})

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, jsonContentType, rec.Header().Get("Content-Type"))
	assert.JSONEq(t, fallbackErrorBody, rec.Body.String())
	assert.Contains(t, logs.String(), "failed to encode response body")
}

func TestWriteJSONWriteFailureIsLogged(t *testing.T) {
	ctx, logs := newCapturingContext()
	rec := httptest.NewRecorder()
	w := failingWriter{ResponseWriter: rec, err: errors.New("connection reset by peer")}

	writeJSON(ctx, w, http.StatusOK, map[string]string{"status": "ok"})

	assert.Equal(t, http.StatusOK, rec.Code, "the status is sent before the body fails")
	assert.Empty(t, rec.Body.String())
	assert.Contains(t, logs.String(), "failed to write response body")
	assert.Contains(t, logs.String(), "connection reset by peer")
}

func TestWriteErrorUsesKindAndContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), requestIDKey, "req_8f92a10c")
	rec := httptest.NewRecorder()
	rec.Header().Set("X-Request-Id", "req_8f92a10c") // requestLogger does this for real requests

	writeError(ctx, rec, errValidation, details{
		{Field: "user_id", Issue: issueRequired, Description: "Required field"},
	})

	require.Equal(t, errValidation.status, rec.Code)
	assert.Equal(t, jsonContentType, rec.Header().Get("Content-Type"))

	body := decodeError(t, rec, errValidation)
	assert.Equal(t, "req_8f92a10c", body.RequestID)
	assert.Equal(t, details{
		{Field: "user_id", Issue: issueRequired, Description: "Required field"},
	}, details(body.Details))
}
