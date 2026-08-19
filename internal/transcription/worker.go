package transcription

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mikael/dpgmedia/internal/domain"
	"github.com/mikael/dpgmedia/internal/store"
)

type Transcriber interface {
	Enqueue(messageID, uri string)
}

type Options struct {
	Messages store.MessageStore
	Speech   Speech
	Logger   *slog.Logger
}

type Worker struct {
	messages store.MessageStore
	speech   Speech
	logger   *slog.Logger
	wg       sync.WaitGroup
}

var _ Transcriber = (*Worker)(nil)

func NewWorker(opts Options) *Worker {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Worker{
		messages: opts.Messages,
		speech:   opts.Speech,
		logger:   logger.With(slog.String("component", "transcription")),
	}
}

func (w *Worker) Enqueue(messageID, uri string) {
	w.wg.Go(func() { w.run(messageID, uri) })
}

func (w *Worker) Wait() {
	w.wg.Wait()
}

func (w *Worker) run(messageID, uri string) {
	logger := w.logger.With(slog.String("message_id", messageID), slog.String("media_uri", uri))
	logger.Info("transcription started")

	ctx := context.Background()
	started := time.Now()

	transcript, err := w.transcribe(ctx, uri)

	update := store.MessageUpdate{Status: domain.StatusReady, Transcript: transcript}
	if err != nil {
		update = store.MessageUpdate{Status: domain.StatusFailedTranscription, Failure: newFailure(err)}
		logger.Warn("transcription failed",
			slog.String("failure_code", string(update.Failure.Code)),
			slog.Any("error", err),
		)
	}

	if _, err := w.messages.Update(ctx, messageID, update); err != nil {
		logger.Error("failed to record transcription result",
			slog.String("status", string(update.Status)),
			slog.Any("error", err),
		)
		return
	}

	logger.Info("transcription finished",
		slog.String("status", string(update.Status)),
		slog.Int("transcript_length", len(update.Transcript)),
		slog.Duration("duration", time.Since(started).Round(time.Millisecond)),
	)
}

const fileScheme = "file://"

var (
	errMediaUnavailable = errors.New("media unavailable")
	errTranscription    = errors.New("transcription error")
)

func (w *Worker) transcribe(ctx context.Context, uri string) (string, error) {
	path, ok := strings.CutPrefix(uri, fileScheme)
	if !ok {
		return "", fmt.Errorf("%w: unsupported media uri %q", errMediaUnavailable, uri)
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errMediaUnavailable, err)
	}
	if info.Size() == 0 {
		return "", fmt.Errorf("%w: media file %s is empty", errTranscription, filepath.Base(path))
	}

	transcript, err := w.speech.Transcribe(ctx, path)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errTranscription, err)
	}

	return transcript, nil
}

func newFailure(err error) *domain.Failure {
	code := domain.FailureTranscriptionError
	if errors.Is(err, errMediaUnavailable) {
		code = domain.FailureMediaUnavailable
	}
	return &domain.Failure{Code: code, Reason: err.Error()}
}
