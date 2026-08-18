package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testMedia() Media {
	return Media{
		ID:          "med_260ad06c-8494-495d-addf-9e0706447335",
		URI:         "file://assets/audio-berichten/260AD06C-8494-495D-ADDF-9E0706447335.wav",
		Type:        TypeAudio,
		ContentType: "audio/wav",
		FileName:    "260AD06C-8494-495D-ADDF-9E0706447335.wav",
		SizeBytes:   1345994,
		CreatedAt:   time.Date(2026, 4, 23, 8, 3, 54, 0, time.UTC),
	}
}

func TestMediaValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Media)
		wantErr string
	}{
		{name: "audio", mutate: func(*Media) {}},
		{name: "video", mutate: func(m *Media) { m.Type = TypeVideo }},
		{name: "picture", mutate: func(m *Media) { m.Type = TypePicture }},
		{name: "missing id", mutate: func(a *Media) { a.ID = "" }, wantErr: "id is required"},
		{name: "blank id", mutate: func(a *Media) { a.ID = "   " }, wantErr: "id is required"},
		{name: "missing uri", mutate: func(a *Media) { a.URI = "" }, wantErr: "uri is required"},
		{name: "blank uri", mutate: func(a *Media) { a.URI = "  " }, wantErr: "uri is required"},
		{name: "missing type", mutate: func(a *Media) { a.Type = "" }, wantErr: "unknown type"},
		{name: "unknown type", mutate: func(a *Media) { a.Type = Type("GIF") }, wantErr: "unknown type"},
		{
			name:    "text is a message type but never a media item",
			mutate:  func(a *Media) { a.Type = TypeText },
			wantErr: "carries no media",
		},
		{name: "negative size", mutate: func(a *Media) { a.SizeBytes = -1 }, wantErr: "size_bytes"},
		{name: "zero size", mutate: func(a *Media) { a.SizeBytes = 0 }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := testMedia()
			tc.mutate(&m)

			err := m.Validate()

			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}

			require.ErrorIs(t, err, ErrInvalidMedia)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}
