package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type contextKey string

const requestIDKey contextKey = "request_id"

func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-Id")
		if requestID == "" {
			requestID = uuid.NewString()
		}

		reqLogger := logger.With(
			slog.String("request_id", requestID),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
		)

		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		ctx = withLogger(ctx, reqLogger)

		w.Header().Set("X-Request-Id", requestID)
		reqLogger.Debug("request started", slog.String("remote_addr", r.RemoteAddr))

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r.WithContext(ctx))

		reqLogger.Debug("request completed",
			slog.Int("status", rec.status),
			slog.Int("bytes", rec.bytes),
			slog.Duration("duration", time.Since(start).Round(time.Microsecond)),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wrote {
		return
	}
	r.wrote = true
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func requestIDFrom(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

type loggerKeyType struct{}

var loggerKey loggerKeyType

func withLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func loggerFrom(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}
