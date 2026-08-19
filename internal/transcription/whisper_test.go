package transcription

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAPIKey = "sk-test-key"

func newTestWhisper(t *testing.T, handler http.HandlerFunc) *Whisper {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return NewWhisper(WhisperOptions{
		APIKey:  testAPIKey,
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	})
}

func TestWhisperSendsTheMediaFile(t *testing.T) {
	var (
		gotPath  string
		gotAuth  string
		gotModel string
		gotName  string
		gotBytes []byte
	)

	client := newTestWhisper(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")

		require.NoError(t, r.ParseMultipartForm(1<<20))
		gotModel = r.FormValue("model")

		file, header, err := r.FormFile("file")
		require.NoError(t, err)
		defer func() { _ = file.Close() }()

		gotName = header.Filename
		gotBytes, err = io.ReadAll(file)
		require.NoError(t, err)

		_, _ = w.Write([]byte(`{"text":"  Hallo allemaal.  "}`))
	})

	path := writeMediaFile(t, "voice-note.wav", 512)

	text, err := client.Transcribe(t.Context(), path)
	require.NoError(t, err)

	assert.Equal(t, "Hallo allemaal.", text, "the transcript is trimmed")
	assert.Equal(t, "/audio/transcriptions", gotPath)
	assert.Equal(t, "Bearer "+testAPIKey, gotAuth)
	assert.Equal(t, DefaultModel, gotModel)
	assert.Equal(t, "voice-note.wav", gotName)
	assert.Len(t, gotBytes, 512)
}

func TestWhisperWithoutAnAPIKey(t *testing.T) {
	for name, key := range map[string]string{"unset": "", "blank": " \n"} {
		t.Run(name, func(t *testing.T) {
			client := NewWhisper(WhisperOptions{APIKey: key})

			_, err := client.Transcribe(t.Context(), writeMediaFile(t, "voice-note.wav", 512))
			require.ErrorIs(t, err, ErrNoAPIKey)
		})
	}
}

func TestWhisperReportsAPIErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{
			name:    "error envelope",
			status:  http.StatusUnauthorized,
			body:    `{"error":{"message":"Incorrect API key provided"}}`,
			wantErr: "responded 401: Incorrect API key provided",
		},
		{
			name:    "unparsable body",
			status:  http.StatusBadGateway,
			body:    "upstream is down",
			wantErr: "responded 502: upstream is down",
		},
		{
			name:    "unparsable success body",
			status:  http.StatusOK,
			body:    "not json",
			wantErr: "decode",
		},
		{
			name:    "empty transcript",
			status:  http.StatusOK,
			body:    `{"text":"   "}`,
			wantErr: "empty transcript",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := newTestWhisper(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})

			_, err := client.Transcribe(t.Context(), writeMediaFile(t, "voice-note.wav", 512))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.NotContains(t, err.Error(), testAPIKey, "an error must not leak the api key")
		})
	}
}

func TestWhisperWithoutTheMediaFile(t *testing.T) {
	client := newTestWhisper(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a missing file must not reach the api")
	})

	_, err := client.Transcribe(t.Context(), "/does/not/exist.wav")
	require.ErrorIs(t, err, os.ErrNotExist)
}
