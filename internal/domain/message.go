package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Message struct {
	ID          string
	UserID      string
	Type        Type
	TextContent string
	MediaID     string
	Status      Status
	Failure     *Failure
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

var ErrInvalidMessage = errors.New("invalid message")

func (m Message) Validate() error {
	switch {
	case strings.TrimSpace(m.ID) == "":
		return fmt.Errorf("%w: id is required", ErrInvalidMessage)
	case strings.TrimSpace(m.UserID) == "":
		return fmt.Errorf("%w: user_id is required", ErrInvalidMessage)
	case !m.Type.Valid():
		return fmt.Errorf("%w: unknown type %q", ErrInvalidMessage, m.Type)
	case !m.Status.Valid():
		return fmt.Errorf("%w: unknown status %q", ErrInvalidMessage, m.Status)
	case m.Status.Failed() && m.Failure == nil:
		return fmt.Errorf("%w: failure is required for status %s", ErrInvalidMessage, m.Status)
	case !m.Status.Failed() && m.Failure != nil:
		return fmt.Errorf("%w: failure is not allowed for status %s", ErrInvalidMessage, m.Status)
	case m.Failure != nil:
		return m.Failure.Validate()
	default:
		return nil
	}
}

func (m Message) Expired(now time.Time) bool {
	return !m.ExpiresAt.IsZero() && !now.Before(m.ExpiresAt)
}
