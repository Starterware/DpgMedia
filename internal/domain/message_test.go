package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	valid := Message{
		ID:          "msg_1",
		UserID:      "usr_98765",
		Type:        TypeText,
		TextContent: "hello",
	}

	tests := []struct {
		name    string
		mutate  func(*Message)
		wantMsg string
	}{
		{name: "complete message"},
		{
			name:    "missing id",
			mutate:  func(m *Message) { m.ID = "" },
			wantMsg: "id is required",
		},
		{
			name:    "blank id",
			mutate:  func(m *Message) { m.ID = "   " },
			wantMsg: "id is required",
		},
		{
			name:    "missing user_id",
			mutate:  func(m *Message) { m.UserID = "" },
			wantMsg: "user_id is required",
		},
		{
			name:    "blank user_id",
			mutate:  func(m *Message) { m.UserID = "\t" },
			wantMsg: "user_id is required",
		},
		{
			name:    "unknown type",
			mutate:  func(m *Message) { m.Type = Type("GIF") },
			wantMsg: `unknown type "GIF"`,
		},
		{
			name:    "empty type",
			mutate:  func(m *Message) { m.Type = "" },
			wantMsg: `unknown type ""`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := valid
			if tc.mutate != nil {
				tc.mutate(&msg)
			}

			err := msg.Validate()

			if tc.wantMsg == "" {
				assert.NoError(t, err)
				return
			}

			require.ErrorIs(t, err, ErrInvalidMessage)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

func TestExpired(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{name: "no deadline never expires", expiresAt: time.Time{}},
		{name: "deadline in the future", expiresAt: now.Add(time.Second)},
		{name: "deadline exactly now", expiresAt: now, want: true},
		{name: "deadline in the past", expiresAt: now.Add(-time.Second), want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Message{ExpiresAt: tc.expiresAt}.Expired(now))
		})
	}
}
