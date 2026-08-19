package store

import (
	"context"
	"errors"

	"github.com/mikael/dpgmedia/internal/domain"
)

type MessageStore interface {
	Create(ctx context.Context, msg domain.Message) (domain.Message, error)

	Get(ctx context.Context, id string) (domain.Message, error)

	Update(ctx context.Context, id string, update MessageUpdate) (domain.Message, error)

	List(ctx context.Context, limit int) ([]domain.Message, error)

	Close() error
}

type MessageUpdate struct {
	Status     domain.Status
	Transcript string
	Failure    *domain.Failure
}

var (
	ErrMessageNotFound      = errors.New("message not found")
	ErrMessageAlreadyExists = errors.New("message already exists")
)
