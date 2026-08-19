package api

import (
	"log/slog"
	"net/http"

	"github.com/mikael/dpgmedia/internal/store"
	"github.com/mikael/dpgmedia/internal/transcription"
)

type Options struct {
	MaxBodyBytes int64
	MessageStore store.MessageStore
	MediaStore   store.MediaStore
	Transcriber  transcription.Transcriber
}

func NewRouter(logger *slog.Logger, opts Options) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", handleHealth)
	mux.Handle("POST /api/v1/messages", &createMessageHandler{
		maxBodyBytes: opts.MaxBodyBytes,
		messages:     opts.MessageStore,
		media:        opts.MediaStore,
		transcriber:  opts.Transcriber,
	})
	mux.Handle("GET /api/v1/messages", &listMessagesHandler{messages: opts.MessageStore})

	return requestLogger(logger, mux)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(r.Context(), w, http.StatusOK, map[string]string{"status": "ok"})
}
