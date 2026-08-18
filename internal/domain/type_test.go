package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTypeValid(t *testing.T) {
	tests := []struct {
		messageType Type
		want        bool
	}{
		{TypeText, true},
		{TypeAudio, true},
		{TypeVideo, true},
		{TypePicture, true},
		{Type("GIF"), false},
		{Type("text"), false},
		{Type(""), false},
	}

	for _, tc := range tests {
		t.Run(string(tc.messageType), func(t *testing.T) {
			assert.Equal(t, tc.want, tc.messageType.Valid())
		})
	}
}

func TestTypeRequiresMedia(t *testing.T) {
	tests := []struct {
		messageType Type
		want        bool
	}{
		{TypeText, false},
		{TypeAudio, true},
		{TypeVideo, true},
		{TypePicture, true},
	}

	for _, tc := range tests {
		t.Run(string(tc.messageType), func(t *testing.T) {
			assert.Equal(t, tc.want, tc.messageType.RequiresMedia())
		})
	}
}

func TestParseType(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Type
		wantErr bool
	}{
		{name: "text", input: "TEXT", want: TypeText},
		{name: "audio", input: "AUDIO", want: TypeAudio},
		{name: "video", input: "VIDEO", want: TypeVideo},
		{name: "picture", input: "PICTURE", want: TypePicture},
		{name: "lowercase", input: "text", want: TypeText},
		{name: "mixed case", input: "AuDiO", want: TypeAudio},
		{name: "padded", input: "  VIDEO\t", want: TypeVideo},
		{name: "unknown", input: "GIF", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseType(tc.input)

			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unknown message type")
				assert.Contains(t, err.Error(), TypeNames, "the error lists what is accepted")
				assert.Empty(t, got, "a failed parse must not yield a usable type")
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
