package store

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/mikael/dpgmedia/internal/domain"
)

type LocalMessageStore struct {
	mu   sync.RWMutex
	byID map[string]domain.Message

	file   *os.File
	closed bool
	path   string
	ttl    time.Duration
	now    func() time.Time
}

type LocalMessageStoreOptions struct {
	Path string
	TTL  time.Duration
	Now  func() time.Time
}

type messageRecord struct {
	MessageID   string         `json:"message_id"`
	UserID      string         `json:"user_id"`
	MessageType domain.Type    `json:"message_type"`
	TextContent string         `json:"text_content,omitempty"`
	MediaID     string         `json:"media_id,omitempty"`
	Transcript  string         `json:"transcript,omitempty"`
	Status      domain.Status  `json:"status"`
	Failure     *failureRecord `json:"failure,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	ExpiresAt   time.Time      `json:"expires_at"`
}

type failureRecord struct {
	Code     domain.FailureCode `json:"code"`
	Reason   string             `json:"reason"`
	FailedAt time.Time          `json:"failed_at"`
}

func newMessageRecord(msg domain.Message) messageRecord {
	return messageRecord{
		MessageID:   msg.ID,
		UserID:      msg.UserID,
		MessageType: msg.Type,
		TextContent: msg.TextContent,
		MediaID:     msg.MediaID,
		Transcript:  msg.Transcript,
		Status:      msg.Status,
		Failure:     newFailureRecord(msg.Failure),
		CreatedAt:   msg.CreatedAt,
		ExpiresAt:   msg.ExpiresAt,
	}
}

func newFailureRecord(failure *domain.Failure) *failureRecord {
	if failure == nil {
		return nil
	}
	return &failureRecord{Code: failure.Code, Reason: failure.Reason, FailedAt: failure.FailedAt}
}

func (r *failureRecord) failure() *domain.Failure {
	if r == nil {
		return nil
	}
	return &domain.Failure{Code: r.Code, Reason: r.Reason, FailedAt: r.FailedAt}
}

func (r messageRecord) message() domain.Message {
	// Records written before the status field existed read back as available,
	// which is what they were.
	if r.Status == "" {
		r.Status = domain.StatusReady
	}

	return domain.Message{
		ID:          r.MessageID,
		UserID:      r.UserID,
		Type:        r.MessageType,
		TextContent: r.TextContent,
		MediaID:     r.MediaID,
		Transcript:  r.Transcript,
		Status:      r.Status,
		Failure:     r.Failure.failure(),
		CreatedAt:   r.CreatedAt,
		ExpiresAt:   r.ExpiresAt,
	}
}

var _ MessageStore = (*LocalMessageStore)(nil)

func OpenLocalMessageStore(opts LocalMessageStoreOptions) (*LocalMessageStore, error) {
	s := &LocalMessageStore{
		byID: make(map[string]domain.Message),
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

func (s *LocalMessageStore) load() error {
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

		var rec messageRecord
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

func (s *LocalMessageStore) Create(ctx context.Context, msg domain.Message) (domain.Message, error) {
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}

	msg.CreatedAt = s.now().UTC()

	msg.ExpiresAt = time.Time{}
	if s.ttl > 0 {
		msg.ExpiresAt = msg.CreatedAt.Add(s.ttl)
	}

	if msg.Status == "" {
		msg.Status = domain.InitialStatus(msg.Type)
	}

	if err := msg.Validate(); err != nil {
		return domain.Message{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.byID[msg.ID]; ok {
		return domain.Message{}, fmt.Errorf("%w: %s", ErrMessageAlreadyExists, msg.ID)
	}

	if err := s.append(msg); err != nil {
		return domain.Message{}, err
	}
	s.byID[msg.ID] = msg

	return msg, nil
}

func (s *LocalMessageStore) append(msg domain.Message) error {
	if s.file == nil {
		return nil
	}

	line, err := json.Marshal(newMessageRecord(msg))
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

func (s *LocalMessageStore) Update(ctx context.Context, id string, update MessageUpdate) (domain.Message, error) {
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.byID[id]
	if !ok {
		return domain.Message{}, fmt.Errorf("%w: %s", ErrMessageNotFound, id)
	}

	msg := stored
	msg.Status = update.Status
	msg.Transcript = update.Transcript
	msg.Failure = update.Failure

	if update.Failure != nil && update.Failure.FailedAt.IsZero() {
		stamped := *update.Failure
		stamped.FailedAt = s.now().UTC()
		msg.Failure = &stamped
	}

	if err := msg.Validate(); err != nil {
		return domain.Message{}, err
	}

	s.byID[id] = msg

	if err := s.rewrite(); err != nil {
		s.byID[id] = stored
		return domain.Message{}, err
	}

	return msg, nil
}

func (s *LocalMessageStore) rewrite() error {
	if s.file == nil {
		return nil
	}

	messages := make([]domain.Message, 0, len(s.byID))
	for _, msg := range s.byID {
		messages = append(messages, msg)
	}

	sort.Slice(messages, func(i, j int) bool {
		if messages[i].CreatedAt.Equal(messages[j].CreatedAt) {
			return messages[i].ID < messages[j].ID
		}
		return messages[i].CreatedAt.Before(messages[j].CreatedAt)
	})

	var buf bytes.Buffer
	for _, msg := range messages {
		line, err := json.Marshal(newMessageRecord(msg))
		if err != nil {
			return fmt.Errorf("encode message %s: %w", msg.ID, err)
		}
		buf.Write(append(line, '\n'))
	}

	// The records are rebuilt beside the store and moved onto it in one rename,
	// so an interrupted rewrite cannot leave a half-written store behind.
	tmp := s.path + ".tmp"
	defer func() { _ = os.Remove(tmp) }()

	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write store file %s: %w", tmp, err)
	}

	if err := s.file.Close(); err != nil {
		return fmt.Errorf("close store file %s: %w", s.path, err)
	}

	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace store file %s: %w", s.path, err)
	}

	file, err := os.OpenFile(s.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open store file %s: %w", s.path, err)
	}
	s.file = file

	return nil
}

func (s *LocalMessageStore) Get(ctx context.Context, id string) (domain.Message, error) {
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	msg, ok := s.byID[id]
	if !ok {
		return domain.Message{}, fmt.Errorf("%w: %s", ErrMessageNotFound, id)
	}

	return msg, nil
}

func (s *LocalMessageStore) List(ctx context.Context, limit int) ([]domain.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	messages := make([]domain.Message, 0, len(s.byID))
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

func (s *LocalMessageStore) Close() error {
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
