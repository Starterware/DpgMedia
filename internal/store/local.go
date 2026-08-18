package store

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/mikael/dpgmedia/internal/message"
)

type Local struct {
	mu   sync.RWMutex
	byID map[string]message.Message

	file   *os.File
	closed bool
	path   string
	ttl    time.Duration
	now    func() time.Time
}

type LocalOptions struct {
	Path string
	TTL  time.Duration
	Now  func() time.Time
}

type record struct {
	MessageID   string       `json:"message_id"`
	UserID      string       `json:"user_id"`
	MessageType message.Type `json:"message_type"`
	TextContent string       `json:"text_content,omitempty"`
	MediaID     string       `json:"media_id,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	ExpiresAt   time.Time    `json:"expires_at"`
}

func newRecord(msg message.Message) record {
	return record{
		MessageID:   msg.ID,
		UserID:      msg.UserID,
		MessageType: msg.Type,
		TextContent: msg.TextContent,
		MediaID:     msg.MediaID,
		CreatedAt:   msg.CreatedAt,
		ExpiresAt:   msg.ExpiresAt,
	}
}

func (r record) message() message.Message {
	return message.Message{
		ID:          r.MessageID,
		UserID:      r.UserID,
		Type:        r.MessageType,
		TextContent: r.TextContent,
		MediaID:     r.MediaID,
		CreatedAt:   r.CreatedAt,
		ExpiresAt:   r.ExpiresAt,
	}
}

// Compile-time check that the file-backed store satisfies the contract.
var _ MessageStore = (*Local)(nil)

func OpenLocal(opts LocalOptions) (*Local, error) {
	s := &Local{
		byID: make(map[string]message.Message),
		path: opts.Path,
		ttl:  opts.TTL,
		now:  opts.Now,
	}
	if s.now == nil {
		s.now = time.Now
	}

	if s.path == "" {
		return s, nil
	}

	if dir := filepath.Dir(s.path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create store directory %s: %w", dir, err)
		}
	}

	if err := s.load(); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(s.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open store file %s: %w", s.path, err)
	}
	s.file = file

	return s, nil
}

func (s *Local) load() error {
	file, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read store file %s: %w", s.path, err)
	}
	defer func() { _ = file.Close() }()

	now := s.now()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)

	for line := 1; scanner.Scan(); line++ {
		if len(scanner.Bytes()) == 0 {
			continue
		}

		var rec record
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			return fmt.Errorf("read store file %s: line %d: %w", s.path, line, err)
		}

		msg := rec.message()
		if msg.Expired(now) {
			continue
		}
		s.byID[msg.ID] = msg
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read store file %s: %w", s.path, err)
	}

	return nil
}

func (s *Local) Create(ctx context.Context, msg message.Message) (message.Message, error) {
	if err := ctx.Err(); err != nil {
		return message.Message{}, err
	}
	if err := msg.Validate(); err != nil {
		return message.Message{}, err
	}

	msg.CreatedAt = s.now().UTC()

	msg.ExpiresAt = time.Time{}
	if s.ttl > 0 {
		msg.ExpiresAt = msg.CreatedAt.Add(s.ttl)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.byID[msg.ID]; ok {
		return message.Message{}, fmt.Errorf("%w: %s", ErrAlreadyExists, msg.ID)
	}

	if err := s.append(msg); err != nil {
		return message.Message{}, err
	}
	s.byID[msg.ID] = msg

	return msg, nil
}

func (s *Local) append(msg message.Message) error {
	if s.file == nil {
		return nil
	}

	line, err := json.Marshal(newRecord(msg))
	if err != nil {
		return fmt.Errorf("encode message %s: %w", msg.ID, err)
	}

	if _, err := s.file.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write store file %s: %w", s.path, err)
	}
	if err := s.file.Sync(); err != nil {
		return fmt.Errorf("flush store file %s: %w", s.path, err)
	}

	return nil
}

func (s *Local) Get(ctx context.Context, id string) (message.Message, error) {
	if err := ctx.Err(); err != nil {
		return message.Message{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	msg, ok := s.byID[id]
	if !ok {
		return message.Message{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}

	return msg, nil
}

func (s *Local) List(ctx context.Context, limit int) ([]message.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	messages := make([]message.Message, 0, len(s.byID))
	for _, msg := range s.byID {
		messages = append(messages, msg)
	}

	sort.Slice(messages, func(i, j int) bool {
		if messages[i].CreatedAt.Equal(messages[j].CreatedAt) {
			return messages[i].ID > messages[j].ID
		}
		return messages[i].CreatedAt.After(messages[j].CreatedAt)
	})

	if limit > 0 && len(messages) > limit {
		messages = messages[:limit]
	}

	return messages, nil
}

func (s *Local) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.file == nil || s.closed {
		return nil
	}
	s.closed = true

	if err := s.file.Close(); err != nil {
		return fmt.Errorf("close store file %s: %w", s.path, err)
	}

	return nil
}
