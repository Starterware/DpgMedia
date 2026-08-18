package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikael/dpgmedia/internal/domain"
)

const testTTL = 7 * 24 * time.Hour

var testNow = time.Date(2026, 8, 17, 14, 54, 49, 0, time.UTC)

func openTestMessageStore(t *testing.T, opts LocalMessageStoreOptions) (*LocalMessageStore, string) {
	t.Helper()

	if opts.Path == "" {
		opts.Path = filepath.Join(t.TempDir(), "messages.jsonl")
	}
	if opts.TTL == 0 {
		opts.TTL = testTTL
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return testNow }
	}

	s, err := OpenLocalMessageStore(opts)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })

	return s, opts.Path
}

func testMessage(id string) domain.Message {
	return domain.Message{
		ID:          id,
		UserID:      "usr_98765",
		Type:        domain.TypeText,
		TextContent: "hello",
	}
}

func TestLocalCreateAndGet(t *testing.T) {
	s, _ := openTestMessageStore(t, LocalMessageStoreOptions{})

	created, err := s.Create(context.Background(), testMessage("msg_1"))
	require.NoError(t, err)
	assert.Equal(t, testNow, created.CreatedAt)
	assert.Equal(t, testNow.Add(testTTL), created.ExpiresAt,
		"the store stamps the retention deadline")

	got, err := s.Get(context.Background(), "msg_1")
	require.NoError(t, err)
	assert.Equal(t, created, got)
}

func TestLocalCreateOwnsTheTimestamps(t *testing.T) {
	s, _ := openTestMessageStore(t, LocalMessageStoreOptions{})

	msg := testMessage("msg_1")
	msg.CreatedAt = testNow.Add(-time.Hour)
	msg.ExpiresAt = testNow.Add(time.Hour)

	created, err := s.Create(context.Background(), msg)
	require.NoError(t, err)
	assert.Equal(t, testNow, created.CreatedAt, "a caller cannot backdate a record")
	assert.Equal(t, testNow.Add(testTTL), created.ExpiresAt, "the deadline follows the store's clock")

	got, err := s.Get(context.Background(), "msg_1")
	require.NoError(t, err)
	assert.Equal(t, created, got)
}

func TestLocalCreateWithoutTTL(t *testing.T) {
	s, _ := openTestMessageStore(t, LocalMessageStoreOptions{TTL: -1})

	created, err := s.Create(context.Background(), testMessage("msg_1"))
	require.NoError(t, err)
	assert.True(t, created.ExpiresAt.IsZero(), "expires_at = %v, want zero", created.ExpiresAt)
}

func TestLocalCreateInvalid(t *testing.T) {
	s, _ := openTestMessageStore(t, LocalMessageStoreOptions{})

	msg := testMessage("msg_1")
	msg.UserID = ""

	_, err := s.Create(context.Background(), msg)
	require.ErrorIs(t, err, domain.ErrInvalidMessage)

	messages, err := s.List(context.Background(), 0)
	require.NoError(t, err)
	assert.Empty(t, messages, "a rejected message must not be stored")
}

func TestLocalCreateDuplicate(t *testing.T) {
	s, _ := openTestMessageStore(t, LocalMessageStoreOptions{})

	_, err := s.Create(context.Background(), testMessage("msg_1"))
	require.NoError(t, err)

	_, err = s.Create(context.Background(), testMessage("msg_1"))
	require.ErrorIs(t, err, ErrMessageAlreadyExists)
}

func TestLocalGetUnknown(t *testing.T) {
	s, _ := openTestMessageStore(t, LocalMessageStoreOptions{})

	_, err := s.Get(context.Background(), "msg_missing")
	require.ErrorIs(t, err, ErrMessageNotFound)
}

func TestLocalKeepsExpiredRecordUntilReload(t *testing.T) {
	now := testNow
	clock := func() time.Time { return now }

	s, path := openTestMessageStore(t, LocalMessageStoreOptions{Now: clock})

	_, err := s.Create(context.Background(), testMessage("msg_1"))
	require.NoError(t, err)

	now = testNow.Add(testTTL)

	messages, err := s.List(context.Background(), 0)
	require.NoError(t, err)
	assert.Len(t, messages, 1)

	require.NoError(t, s.Close())

	reopened, _ := openTestMessageStore(t, LocalMessageStoreOptions{Path: path, Now: clock})

	_, err = reopened.Get(context.Background(), "msg_1")
	assert.ErrorIs(t, err, ErrMessageNotFound, "the record is dropped while replaying the file")

	messages, err = reopened.List(context.Background(), 0)
	require.NoError(t, err)
	assert.Empty(t, messages)
}

func TestLocalList(t *testing.T) {
	now := testNow
	s, _ := openTestMessageStore(t, LocalMessageStoreOptions{Now: func() time.Time { return now }})

	for i := range 3 {
		now = testNow.Add(time.Duration(i) * time.Minute)
		_, err := s.Create(context.Background(), testMessage(fmt.Sprintf("msg_%d", i)))
		require.NoError(t, err)
	}

	tests := []struct {
		name  string
		limit int
		want  []string
	}{
		{name: "no limit", limit: 0, want: []string{"msg_2", "msg_1", "msg_0"}},
		{name: "negative limit", limit: -1, want: []string{"msg_2", "msg_1", "msg_0"}},
		{name: "limited", limit: 2, want: []string{"msg_2", "msg_1"}},
		{name: "limit above count", limit: 10, want: []string{"msg_2", "msg_1", "msg_0"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			messages, err := s.List(context.Background(), tc.limit)
			require.NoError(t, err)

			ids := make([]string, 0, len(messages))
			for _, msg := range messages {
				ids = append(ids, msg.ID)
			}
			assert.Equal(t, tc.want, ids, "newest message first")
		})
	}
}

func TestLocalPersistsAcrossReopen(t *testing.T) {
	s, path := openTestMessageStore(t, LocalMessageStoreOptions{})

	msg := testMessage("msg")

	created, err := s.Create(context.Background(), msg)
	require.NoError(t, err)
	require.NoError(t, s.Close())

	reopened, _ := openTestMessageStore(t, LocalMessageStoreOptions{Path: path})

	got, err := reopened.Get(context.Background(), "msg")
	require.NoError(t, err)
	assert.Equal(t, created, got)
}

func TestLocalRejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("{\"message_id\":\"msg_1\"}\nnot json\n"), 0o644))

	_, err := OpenLocalMessageStore(LocalMessageStoreOptions{Path: path, TTL: testTTL})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "line 2")
}

func TestLocalCreatesMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "messages.jsonl")

	s, _ := openTestMessageStore(t, LocalMessageStoreOptions{Path: path})

	_, err := s.Create(context.Background(), testMessage("msg_1"))
	require.NoError(t, err)
	assert.FileExists(t, path)
}

func TestLocalWithoutPathKeepsMessagesInMemory(t *testing.T) {
	s, err := OpenLocalMessageStore(LocalMessageStoreOptions{TTL: testTTL})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })

	_, err = s.Create(context.Background(), testMessage("msg_1"))
	require.NoError(t, err)

	got, err := s.Get(context.Background(), "msg_1")
	require.NoError(t, err)
	assert.Equal(t, "msg_1", got.ID)
}

func TestLocalCreateAfterCloseFails(t *testing.T) {
	s, _ := openTestMessageStore(t, LocalMessageStoreOptions{})

	_, err := s.Create(context.Background(), testMessage("msg_1"))
	require.NoError(t, err)
	require.NoError(t, s.Close())
	require.NoError(t, s.Close(), "Close is idempotent")

	_, err = s.Create(context.Background(), testMessage("msg_2"))
	require.Error(t, err, "a record that cannot reach the file must not be accepted")

	_, err = s.Get(context.Background(), "msg_2")
	assert.ErrorIs(t, err, ErrMessageNotFound)

	got, err := s.Get(context.Background(), "msg_1")
	require.NoError(t, err, "records written before Close stay readable")
	assert.Equal(t, "msg_1", got.ID)
}

func TestLocalHonoursCanceledContext(t *testing.T) {
	s, _ := openTestMessageStore(t, LocalMessageStoreOptions{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.Create(ctx, testMessage("msg_1"))
	assert.ErrorIs(t, err, context.Canceled)

	_, err = s.Get(ctx, "msg_1")
	assert.ErrorIs(t, err, context.Canceled)

	_, err = s.List(ctx, 0)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestLocalConcurrentCreate(t *testing.T) {
	s, _ := openTestMessageStore(t, LocalMessageStoreOptions{})

	const writers = 25

	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Create(context.Background(), testMessage(fmt.Sprintf("msg_%d", i)))
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	messages, err := s.List(context.Background(), 0)
	require.NoError(t, err)
	assert.Len(t, messages, writers)
}
