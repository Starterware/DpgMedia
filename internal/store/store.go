package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mikael/dpgmedia/internal/message"
)

type MessageStore interface {
	Create(ctx context.Context, msg message.Message) (message.Message, error)

	Get(ctx context.Context, id string) (message.Message, error)

	List(ctx context.Context, limit int) ([]message.Message, error)

	Close() error
}

var (
	ErrNotFound      = errors.New("message not found")
	ErrAlreadyExists = errors.New("message already exists")
)

type Driver string

const (
	DriverLocal Driver = "local"
)

func ParseDriver(s string) (Driver, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case string(DriverLocal):
		return DriverLocal, nil
	default:
		return "", fmt.Errorf("unknown store driver %q, want one of %s", s, DriverLocal)
	}
}
