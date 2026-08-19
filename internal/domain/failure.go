package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type FailureCode string

const (
	FailureMediaUnavailable   FailureCode = "MEDIA_UNAVAILABLE"
	FailureTranscriptionError FailureCode = "TRANSCRIPTION_ERROR"
)

func (c FailureCode) Valid() bool {
	switch c {
	case FailureMediaUnavailable, FailureTranscriptionError:
		return true
	default:
		return false
	}
}

type Failure struct {
	Code     FailureCode
	Reason   string
	FailedAt time.Time
}

var ErrInvalidFailure = errors.New("invalid failure")

func (f Failure) Validate() error {
	switch {
	case !f.Code.Valid():
		return fmt.Errorf("%w: unknown code %q", ErrInvalidFailure, f.Code)
	case strings.TrimSpace(f.Reason) == "":
		return fmt.Errorf("%w: reason is required", ErrInvalidFailure)
	default:
		return nil
	}
}
