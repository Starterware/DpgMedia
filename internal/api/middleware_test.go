package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestIDFrom(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"key absent", context.Background(), ""},
		{"key set to empty string", context.WithValue(context.Background(), requestIDKey, ""), ""},
		{"key set to another type", context.WithValue(context.Background(), requestIDKey, 42), ""},
		{"key set to a request id", context.WithValue(context.Background(), requestIDKey, "req_8f92a10c"), "req_8f92a10c"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, requestIDFrom(tc.ctx))
		})
	}
}

func TestLoggerFrom(t *testing.T) {
	stored := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name string
		ctx  context.Context
		want *slog.Logger
	}{
		{"key absent", context.Background(), slog.Default()},
		{"key set to another type", context.WithValue(context.Background(), loggerKey, "not a logger"), slog.Default()},
		{"key set to a logger", withLogger(context.Background(), stored), stored},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Same(t, tc.want, loggerFrom(tc.ctx))
		})
	}
}

func TestStatusRecorder(t *testing.T) {
	newRecorder := func() (*httptest.ResponseRecorder, *statusRecorder) {
		underlying := httptest.NewRecorder()
		return underlying, &statusRecorder{ResponseWriter: underlying, status: http.StatusOK}
	}

	t.Run("write without an explicit status records 200", func(t *testing.T) {
		underlying, rec := newRecorder()

		n, err := rec.Write([]byte("hello"))
		require.NoError(t, err)

		assert.Equal(t, 5, n)
		assert.True(t, rec.wrote)
		assert.Equal(t, http.StatusOK, rec.status)
		assert.Equal(t, http.StatusOK, underlying.Code)
		assert.Equal(t, "hello", underlying.Body.String())
	})

	t.Run("the first status wins", func(t *testing.T) {
		underlying, rec := newRecorder()

		rec.WriteHeader(http.StatusUnprocessableEntity)
		rec.WriteHeader(http.StatusInternalServerError)

		assert.Equal(t, http.StatusUnprocessableEntity, rec.status)
		assert.Equal(t, http.StatusUnprocessableEntity, underlying.Code)
	})

	t.Run("a write after an explicit status keeps it", func(t *testing.T) {
		underlying, rec := newRecorder()

		rec.WriteHeader(http.StatusCreated)
		_, err := rec.Write([]byte("body"))
		require.NoError(t, err)

		assert.Equal(t, http.StatusCreated, rec.status)
		assert.Equal(t, http.StatusCreated, underlying.Code)
	})

	t.Run("bytes accumulate across writes", func(t *testing.T) {
		_, rec := newRecorder()

		_, err := rec.Write([]byte("ab"))
		require.NoError(t, err)
		_, err = rec.Write([]byte("cde"))
		require.NoError(t, err)

		assert.Equal(t, 5, rec.bytes)
	})
}

func TestRequestIDIsUUID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, req)

	got := rec.Header().Get("X-Request-Id")
	require.NotEmpty(t, got, "handler must echo a request id")

	parsed, err := uuid.Parse(got)
	require.NoError(t, err, "request id %q is not a UUID", got)
	assert.Equal(t, uuid.Version(4), parsed.Version(), "want a random (v4) UUID")
	assert.Equal(t, uuid.RFC4122, parsed.Variant())
}

func TestRequestIDIsUnique(t *testing.T) {
	const runs = 100
	seen := make(map[string]struct{}, runs)

	for range runs {
		rec := httptest.NewRecorder()
		newTestRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

		id := rec.Header().Get("X-Request-Id")
		_, dup := seen[id]
		require.False(t, dup, "duplicate request id %q", id)
		seen[id] = struct{}{}
	}
}

func TestRequestIDFromClientIsPreserved(t *testing.T) {
	tests := []struct {
		name string
		sent string
	}{
		{"upstream uuid", "6ba7b810-9dad-11d1-80b4-00c04fd430c8"},
		{"opaque trace id", "trace-abc-123"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			req.Header.Set("X-Request-Id", tc.sent)
			rec := httptest.NewRecorder()
			newTestRouter().ServeHTTP(rec, req)

			assert.Equal(t, tc.sent, rec.Header().Get("X-Request-Id"))
		})
	}
}
