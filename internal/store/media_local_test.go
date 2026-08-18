package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikael/dpgmedia/internal/domain"
)

const testCatalog = `[
  {
    "media_id": "med_260ad06c-8494-495d-addf-9e0706447335",
    "uri": "file://assets/audio-berichten/260AD06C-8494-495D-ADDF-9E0706447335.wav",
    "media_type": "AUDIO",
    "content_type": "audio/wav",
    "file_name": "260AD06C-8494-495D-ADDF-9E0706447335.wav",
    "size_bytes": 1345994,
    "created_at": "2026-04-23T08:03:54Z"
  },
  {
    "media_id": "med_613b1cf3-4bf8-469b-a9f7-29c44b9d193d",
    "uri": "file://assets/audio-berichten/613B1CF3-4BF8-469B-A9F7-29C44B9D193D.wav",
    "media_type": "AUDIO",
    "content_type": "audio/wav",
    "file_name": "613B1CF3-4BF8-469B-A9F7-29C44B9D193D.wav",
    "size_bytes": 435020,
    "created_at": "2026-04-23T08:03:37Z"
  }
]`

func writeCatalog(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "catalog.json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))

	return path
}

func openTestMediaStore(t *testing.T, body string) *LocalMediaStore {
	t.Helper()

	s, err := OpenLocalMediaStore(LocalMediaStoreOptions{Path: writeCatalog(t, body)})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })

	return s
}

func TestLocalMediaGet(t *testing.T) {
	s := openTestMediaStore(t, testCatalog)

	item, err := s.Get(context.Background(), "med_260ad06c-8494-495d-addf-9e0706447335")
	require.NoError(t, err)

	assert.Equal(t, domain.Media{
		ID:          "med_260ad06c-8494-495d-addf-9e0706447335",
		URI:         "file://assets/audio-berichten/260AD06C-8494-495D-ADDF-9E0706447335.wav",
		Type:        domain.TypeAudio,
		ContentType: "audio/wav",
		FileName:    "260AD06C-8494-495D-ADDF-9E0706447335.wav",
		SizeBytes:   1345994,
		CreatedAt:   time.Date(2026, 4, 23, 8, 3, 54, 0, time.UTC),
	}, item)
}

func TestLocalMediaGetUnknownID(t *testing.T) {
	s := openTestMediaStore(t, testCatalog)

	_, err := s.Get(context.Background(), "med_missing")
	require.ErrorIs(t, err, ErrMediaNotFound)
	assert.Contains(t, err.Error(), "med_missing")
}

func TestLocalMediaGetHonoursContext(t *testing.T) {
	s := openTestMediaStore(t, testCatalog)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.Get(ctx, "med_260ad06c-8494-495d-addf-9e0706447335")
	require.ErrorIs(t, err, context.Canceled)
}

func TestLocalMediaEmptyCatalog(t *testing.T) {
	s := openTestMediaStore(t, `[]`)

	_, err := s.Get(context.Background(), "med_260ad06c-8494-495d-addf-9e0706447335")
	require.ErrorIs(t, err, ErrMediaNotFound)
}

func TestLocalMediaOpenRejectsBadCatalog(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{name: "malformed json", body: `[`, wantErr: "read media catalog"},
		{name: "not a list", body: `{"media_id":"med_1"}`, wantErr: "read media catalog"},
		{
			name:    "entry the domain rejects",
			body:    `[{"media_id":"","uri":"file://a.wav","media_type":"AUDIO"}]`,
			wantErr: "entry 0",
		},
		{
			name: "duplicate media_id",
			body: `[
				{"media_id":"med_1","uri":"file://a.wav","media_type":"AUDIO"},
				{"media_id":"med_1","uri":"file://b.wav","media_type":"AUDIO"}
			]`,
			wantErr: "duplicate media_id med_1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := OpenLocalMediaStore(LocalMediaStoreOptions{Path: writeCatalog(t, tc.body)})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestLocalMediaOpenMissingFile(t *testing.T) {
	_, err := OpenLocalMediaStore(LocalMediaStoreOptions{Path: filepath.Join(t.TempDir(), "absent.json")})

	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestShippedMediaCatalogLoads(t *testing.T) {
	s, err := OpenLocalMediaStore(LocalMediaStoreOptions{Path: filepath.Join("..", "..", "data", "media", "catalog.json")})
	require.NoError(t, err)

	_, err = s.Get(context.Background(), "med_260ad06c-8494-495d-addf-9e0706447335")
	assert.NoError(t, err, "the shipped catalog resolves the ids of the files in assets/audio-berichten")
}
