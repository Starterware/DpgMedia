package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Media struct {
	ID          string
	URI         string
	Type        Type
	ContentType string
	FileName    string
	SizeBytes   int64
	CreatedAt   time.Time
}

var ErrInvalidMedia = errors.New("invalid media")

func (m Media) Validate() error {
	switch {
	case strings.TrimSpace(m.ID) == "":
		return fmt.Errorf("%w: id is required", ErrInvalidMedia)
	case strings.TrimSpace(m.URI) == "":
		return fmt.Errorf("%w: uri is required", ErrInvalidMedia)
	case !m.Type.Valid():
		return fmt.Errorf("%w: unknown type %q", ErrInvalidMedia, m.Type)
	case !m.Type.RequiresMedia():
		// TEXT is a valid message type but never a media item: its payload
		// is inline, so there is no file to describe.
		return fmt.Errorf("%w: type %s carries no media", ErrInvalidMedia, m.Type)
	case m.SizeBytes < 0:
		return fmt.Errorf("%w: size_bytes must not be negative", ErrInvalidMedia)
	default:
		return nil
	}
}
