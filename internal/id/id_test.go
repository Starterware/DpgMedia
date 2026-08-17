package id

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name       string
		prefix     string
		wantPrefix string
	}{
		{"plain prefix", "msg", "msg_"},
		{"trailing separator is not doubled", "msg_", "msg_"},
		{"prefix may contain a separator", "med_audio", "med_audio_"},
		{"empty prefix yields a bare uuid", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := New(tc.prefix)
			require.True(t, strings.HasPrefix(got, tc.wantPrefix), "got %q, want prefix %q", got, tc.wantPrefix)

			_, err := uuid.Parse(strings.TrimPrefix(got, tc.wantPrefix))
			require.NoError(t, err, "suffix of %q is not a UUID", got)
		})
	}
}

func TestNewIsUnique(t *testing.T) {
	const runs = 1000
	seen := make(map[string]struct{}, runs)

	for range runs {
		got := New("msg")
		_, dup := seen[got]
		require.False(t, dup, "duplicate id %q", got)
		seen[got] = struct{}{}
	}
}
