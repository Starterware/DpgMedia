package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/mikael/dpgmedia/internal/domain"
)

type LocalMediaStore struct {
	byID map[string]domain.Media
	path string
}

type LocalMediaStoreOptions struct {
	Path string
}

type mediaRecord struct {
	MediaID     string      `json:"media_id"`
	URI         string      `json:"uri"`
	MediaType   domain.Type `json:"media_type"`
	ContentType string      `json:"content_type"`
	FileName    string      `json:"file_name"`
	SizeBytes   int64       `json:"size_bytes"`
	CreatedAt   time.Time   `json:"created_at"`
}

func (r mediaRecord) media() domain.Media {
	return domain.Media{
		ID:          r.MediaID,
		URI:         r.URI,
		Type:        r.MediaType,
		ContentType: r.ContentType,
		FileName:    r.FileName,
		SizeBytes:   r.SizeBytes,
		CreatedAt:   r.CreatedAt,
	}
}

var _ MediaStore = (*LocalMediaStore)(nil)

func OpenLocalMediaStore(opts LocalMediaStoreOptions) (*LocalMediaStore, error) {
	s := &LocalMediaStore{
		byID: make(map[string]domain.Media),
		path: opts.Path,
	}

	body, err := os.ReadFile(s.path)
	if err != nil {
		return nil, fmt.Errorf("read media catalog %s: %w", s.path, err)
	}

	var records []mediaRecord
	if err := json.Unmarshal(body, &records); err != nil {
		return nil, fmt.Errorf("read media catalog %s: %w", s.path, err)
	}

	for i, rec := range records {
		item := rec.media()
		if err := item.Validate(); err != nil {
			return nil, fmt.Errorf("read media catalog %s: entry %d: %w", s.path, i, err)
		}
		if _, ok := s.byID[item.ID]; ok {
			return nil, fmt.Errorf("read media catalog %s: entry %d: duplicate media_id %s",
				s.path, i, item.ID)
		}
		s.byID[item.ID] = item
	}

	return s, nil
}

func (s *LocalMediaStore) Get(ctx context.Context, id string) (domain.Media, error) {
	if err := ctx.Err(); err != nil {
		return domain.Media{}, err
	}

	item, ok := s.byID[id]
	if !ok {
		return domain.Media{}, fmt.Errorf("%w: %s", ErrMediaNotFound, id)
	}

	return item, nil
}

func (s *LocalMediaStore) Close() error {
	return nil
}
