package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikael/dpgmedia/internal/store"
)

const (
	testMaxBodyBytes = 32 << 10
	testStoreTTL     = 7 * 24 * time.Hour
)

func newTestRouter() http.Handler {
	return newTestRouterWithMessageStore(newTestMessageStore())
}

func newTestRouterWithMessageStore(messages store.MessageStore) http.Handler {
	return NewRouter(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Options{MaxBodyBytes: testMaxBodyBytes, Store: messages},
	)
}

func newTestMessageStore() *store.LocalMessageStore {
	messages, err := store.OpenLocalMessageStore(store.LocalMessageStoreOptions{TTL: testStoreTTL})
	if err != nil {
		panic(err)
	}
	return messages
}

func decodeSuccess[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()

	var envelope struct {
		Data T `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	return envelope.Data
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder, kind errorKind) errorBody {
	t.Helper()

	var envelope errorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))

	body := envelope.Error
	assert.Equal(t, kind.status, rec.Code)
	assert.Equal(t, kind.code, body.Code)

	assert.Equal(t, kind.message, body.Message)

	assert.Equal(t, rec.Header().Get("X-Request-Id"), body.RequestID,
		"request_id must match the id echoed in the header")

	_, err := time.Parse(time.RFC3339, body.Timestamp)
	assert.NoError(t, err, "timestamp = %q, want RFC 3339", body.Timestamp)

	return body
}
