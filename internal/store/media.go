package store

import (
	"context"
	"errors"

	"github.com/mikael/dpgmedia/internal/domain"
)

type MediaStore interface {
	Get(ctx context.Context, id string) (domain.Media, error)

	Close() error
}

var ErrMediaNotFound = errors.New("media not found")
