package logging

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"DEBUG", slog.LevelDebug},
		{"Warning", slog.LevelWarn},
		{"  info  ", slog.LevelInfo},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			level, err := ParseLevel(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, level)
		})
	}
}

func TestParseLevelUnknown(t *testing.T) {
	for _, in := range []string{"", "   ", "chatty", "trace", "fatal", "inf"} {
		t.Run(in, func(t *testing.T) {
			level, err := ParseLevel(in)
			assert.ErrorContains(t, err, "unknown log level")
			assert.Zero(t, level)
		})
	}
}

func TestParseFormat(t *testing.T) {
	tests := []struct {
		in   string
		want Format
	}{
		{"json", FormatJSON},
		{"text", FormatText},
		{"JSON", FormatJSON},
		{"Text", FormatText},
		{"  json  ", FormatJSON},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			format, err := ParseFormat(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, format)
		})
	}
}

func TestParseFormatUnknown(t *testing.T) {
	for _, in := range []string{"", "   ", "yaml", "logfmt", "jsn"} {
		t.Run(in, func(t *testing.T) {
			format, err := ParseFormat(in)
			assert.ErrorContains(t, err, "unknown log format")
			assert.Empty(t, format)
		})
	}
}
