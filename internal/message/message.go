package message

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Type string

const (
	TypeText    Type = "TEXT"
	TypeAudio   Type = "AUDIO"
	TypeVideo   Type = "VIDEO"
	TypePicture Type = "PICTURE"
)

const TypeNames = "TEXT, AUDIO, VIDEO, PICTURE"

func (t Type) Valid() bool {
	switch t {
	case TypeText, TypeAudio, TypeVideo, TypePicture:
		return true
	default:
		return false
	}
}

func (t Type) RequiresMedia() bool {
	return t != TypeText
}

func ParseType(s string) (Type, error) {
	parsed := Type(strings.ToUpper(strings.TrimSpace(s)))
	if !parsed.Valid() {
		return "", fmt.Errorf("unknown message type %q, want one of %s", s, TypeNames)
	}
	return parsed, nil
}

type Message struct {
	ID          string
	UserID      string
	Type        Type
	TextContent string
	MediaID     string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

var ErrInvalid = errors.New("invalid message")

func (m Message) Validate() error {
	switch {
	case strings.TrimSpace(m.ID) == "":
		return fmt.Errorf("%w: id is required", ErrInvalid)
	case strings.TrimSpace(m.UserID) == "":
		return fmt.Errorf("%w: user_id is required", ErrInvalid)
	case !m.Type.Valid():
		return fmt.Errorf("%w: unknown type %q", ErrInvalid, m.Type)
	default:
		return nil
	}
}

func (m Message) Expired(now time.Time) bool {
	return !m.ExpiresAt.IsZero() && !now.Before(m.ExpiresAt)
}
