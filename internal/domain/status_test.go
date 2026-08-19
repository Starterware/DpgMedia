package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusValid(t *testing.T) {
	tests := []struct {
		status Status
		want   bool
	}{
		{StatusReady, true},
		{StatusPendingTranscription, true},
		{StatusFailedTranscription, true},
		{Status("READY "), false},
		{Status("ready"), false},
		{Status("DELETED"), false},
		{Status(""), false},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			assert.Equal(t, tc.want, tc.status.Valid())
		})
	}
}

func TestStatusFailed(t *testing.T) {
	tests := []struct {
		status Status
		want   bool
	}{
		{StatusReady, false},
		{StatusPendingTranscription, false},
		{StatusFailedTranscription, true},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			assert.Equal(t, tc.want, tc.status.Failed())
		})
	}
}

func TestInitialStatus(t *testing.T) {
	tests := []struct {
		messageType Type
		want        Status
	}{
		{TypeText, StatusReady},
		{TypeAudio, StatusPendingTranscription},
		{TypeVideo, StatusReady},
		{TypePicture, StatusReady},
	}

	for _, tc := range tests {
		t.Run(string(tc.messageType), func(t *testing.T) {
			assert.Equal(t, tc.want, InitialStatus(tc.messageType))
		})
	}
}
