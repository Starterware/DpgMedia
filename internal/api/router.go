package api

import (
	"log/slog"
	"net/http"
)

type Options struct {
	MaxBodyBytes int64
}

func NewRouter(logger *slog.Logger, opts Options) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", handleHealth)
	mux.Handle("POST /api/v1/messages", &messagesHandler{maxBodyBytes: opts.MaxBodyBytes})

	return requestLogger(logger, mux)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(r.Context(), w, http.StatusOK, map[string]string{"status": "ok"})
}
