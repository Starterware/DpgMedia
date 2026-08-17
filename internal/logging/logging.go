package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

type Format string

const (
	FormatJSON Format = "json"
	FormatText Format = "text"
)

type Options struct {
	Level  slog.Level
	Format Format
	Output io.Writer
}

func New(opts Options) *slog.Logger {
	out := opts.Output
	if out == nil {
		out = os.Stdout
	}

	handlerOpts := &slog.HandlerOptions{Level: opts.Level}

	var handler slog.Handler
	if opts.Format == FormatText {
		handler = slog.NewTextHandler(out, handlerOpts)
	} else {
		handler = slog.NewJSONHandler(out, handlerOpts)
	}

	return slog.New(handler)
}

func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown log level %q, want one of debug, info, warn, error", s)
	}
}

func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "json":
		return FormatJSON, nil
	case "text":
		return FormatText, nil
	default:
		return "", fmt.Errorf("unknown log format %q, want one of json, text", s)
	}
}
