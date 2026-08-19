package transcription

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeMediaFile(t *testing.T, name string, size int) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, make([]byte, size), 0o644))

	return path
}

func writeMedia(t *testing.T, name string, size int) string {
	t.Helper()

	return fileScheme + writeMediaFile(t, name, size)
}
