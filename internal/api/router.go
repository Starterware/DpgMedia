package api

import (
	"log/slog"
	"net/http"

	"github.com/mikael/dpgmedia/internal/store"
)

type Options struct {
	MaxBodyBytes int64
	Store        store.MessageStore
}

func NewRouter(logger *slog.Logger, opts Options) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", handleHealth)
	mux.Handle("POST /api/v1/messages", &createMessageHandler{
		maxBodyBytes: opts.MaxBodyBytes,
		store:        opts.Store,
	})
	mux.Handle("GET /api/v1/messages", &listMessagesHandler{store: opts.Store})

	return requestLogger(logger, mux)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(r.Context(), w, http.StatusOK, map[string]string{"status": "ok"})
}
