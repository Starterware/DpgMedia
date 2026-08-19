package transcription

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikael/dpgmedia/internal/domain"
	"github.com/mikael/dpgmedia/internal/store"
)

const testDelay = 20 * time.Millisecond

func writeMedia(t *testing.T, name string, size int) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, make([]byte, size), 0o644))

	return "file://" + path
}

func newTestWorker(t *testing.T) (*Worker, *store.LocalMessageStore) {
	t.Helper()

	messages, err := store.OpenLocalMessageStore(store.LocalMessageStoreOptions{TTL: 7 * 24 * time.Hour})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, messages.Close()) })

	worker := NewWorker(Options{
		Messages: messages,
		Delay:    testDelay,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	return worker, messages
}

func transcribe(t *testing.T, worker *Worker, messages store.MessageStore, uri string) domain.Message {
	t.Helper()

	msg, err := messages.Create(t.Context(), domain.Message{
		ID:      "msg_1",
		UserID:  "usr_98765",
		Type:    domain.TypeAudio,
		MediaID: "med_1",
	})
	require.NoError(t, err)
	require.Equal(t, domain.StatusPendingTranscription, msg.Status)

	worker.Enqueue(msg.ID, uri)
	worker.Wait()

	got, err := messages.Get(t.Context(), msg.ID)
	require.NoError(t, err)

	return got
}

func TestWorkerMarksTheMessageReady(t *testing.T) {
	worker, messages := newTestWorker(t)

	got := transcribe(t, worker, messages, writeMedia(t, "voice-note.wav", 1024))

	assert.Equal(t, domain.StatusReady, got.Status)
	assert.Nil(t, got.Failure)
}

func TestWorkerWaitsForTheJobToFinish(t *testing.T) {
	worker, messages := newTestWorker(t)

	started := time.Now()
	transcribe(t, worker, messages, writeMedia(t, "voice-note.wav", 1024))

	assert.GreaterOrEqual(t, time.Since(started), testDelay,
		"Wait must not return before the transcription has run")
}

func TestWorkerRecordsFailures(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone.wav")

	tests := []struct {
		name       string
		uri        string
		wantCode   domain.FailureCode
		wantReason string
	}{
		{
			name:       "media file is gone",
			uri:        "file://" + missing,
			wantCode:   domain.FailureMediaUnavailable,
			wantReason: "no such file",
		},
		{
			name:       "media is not stored locally",
			uri:        "https://cdn.example.com/voice-note.wav",
			wantCode:   domain.FailureMediaUnavailable,
			wantReason: "unsupported media uri",
		},
		{
			name:       "media file is empty",
			uri:        writeMedia(t, "empty.wav", 0),
			wantCode:   domain.FailureTranscriptionError,
			wantReason: "is empty",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			worker, messages := newTestWorker(t)

			got := transcribe(t, worker, messages, tc.uri)

			assert.Equal(t, domain.StatusFailedTranscription, got.Status)
			require.NotNil(t, got.Failure, "a failed message carries the details of the failure")
			assert.Equal(t, tc.wantCode, got.Failure.Code)
			assert.Contains(t, got.Failure.Reason, tc.wantReason)
			assert.False(t, got.Failure.FailedAt.IsZero())
		})
	}
}

func TestWorkerSurvivesAnUnknownMessage(t *testing.T) {
	worker, _ := newTestWorker(t)

	worker.Enqueue("msg_missing", writeMedia(t, "voice-note.wav", 1024))
	worker.Wait()
}

func TestWorkerRunsJobsConcurrently(t *testing.T) {
	worker, messages := newTestWorker(t)
	uri := writeMedia(t, "voice-note.wav", 1024)

	const jobs = 10
	for i := range jobs {
		msg, err := messages.Create(context.Background(), domain.Message{
			ID:      fmt.Sprintf("msg_%d", i),
			UserID:  "usr_98765",
			Type:    domain.TypeAudio,
			MediaID: "med_1",
		})
		require.NoError(t, err)
		worker.Enqueue(msg.ID, uri)
	}

	started := time.Now()
	worker.Wait()
	assert.Less(t, time.Since(started), jobs*testDelay, "the jobs do not queue up behind each other")

	stored, err := messages.List(context.Background(), jobs)
	require.NoError(t, err)
	require.Len(t, stored, jobs)
	for _, msg := range stored {
		assert.Equal(t, domain.StatusReady, msg.Status, "message %s", msg.ID)
	}
}
