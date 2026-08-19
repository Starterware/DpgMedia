package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikael/dpgmedia/internal/store"
	"github.com/mikael/dpgmedia/internal/transcription"
)

const (
	testMaxBodyBytes = 32 << 10
	testStoreTTL     = 7 * 24 * time.Hour
)

const testCatalog = `[
  {
    "media_id": "med_abc890_m4a",
    "uri": "file://assets/audio-berichten/voice-note.m4a",
    "media_type": "AUDIO",
    "content_type": "audio/mp4",
    "file_name": "voice-note.m4a",
    "size_bytes": 4931628,
    "created_at": "2026-04-23T08:01:17Z"
  },
  {
    "media_id": "med_def123_mp4",
    "uri": "file://assets/video-berichten/clip.mp4",
    "media_type": "VIDEO",
    "content_type": "video/mp4",
    "file_name": "clip.mp4",
    "size_bytes": 8123456,
    "created_at": "2026-04-23T08:02:41Z"
  }
]`

func newTestRouter(t *testing.T) http.Handler {
	return newTestRouterWithMessageStore(t, newTestMessageStore(t))
}

func newTestRouterWithMessageStore(t *testing.T, messages store.MessageStore) http.Handler {
	t.Helper()

	return newTestRouterWithTranscriber(t, messages, &recordingTranscriber{})
}

func newTestRouterWithMediaStore(t *testing.T, media store.MediaStore) http.Handler {
	t.Helper()

	return NewRouter(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Options{
			MaxBodyBytes: testMaxBodyBytes,
			MessageStore: newTestMessageStore(t),
			MediaStore:   media,
			Transcriber:  &recordingTranscriber{},
		},
	)
}

func newTestRouterWithTranscriber(t *testing.T, messages store.MessageStore, transcriber transcription.Transcriber) http.Handler {
	t.Helper()

	return NewRouter(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Options{
			MaxBodyBytes: testMaxBodyBytes,
			MessageStore: messages,
			MediaStore:   newTestMediaStore(t),
			Transcriber:  transcriber,
		},
	)
}

func newTestMessageStore(t *testing.T) *store.LocalMessageStore {
	t.Helper()

	messages, err := store.OpenLocalMessageStore(store.LocalMessageStoreOptions{TTL: testStoreTTL})
	require.NoError(t, err)

	return messages
}

func newTestMediaStore(t *testing.T) *store.LocalMediaStore {
	t.Helper()

	path := filepath.Join(t.TempDir(), "catalog.json")
	require.NoError(t, os.WriteFile(path, []byte(testCatalog), 0o644))

	media, err := store.OpenLocalMediaStore(store.LocalMediaStoreOptions{Path: path})
	require.NoError(t, err)

	return media
}

type transcriptionJob struct {
	messageID string
	uri       string
}

type recordingTranscriber struct {
	mu       sync.Mutex
	enqueued []transcriptionJob
}

func (r *recordingTranscriber) Enqueue(messageID, uri string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enqueued = append(r.enqueued, transcriptionJob{messageID: messageID, uri: uri})
}

func (r *recordingTranscriber) jobs() []transcriptionJob {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.enqueued)
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
