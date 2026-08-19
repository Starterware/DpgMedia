package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikael/dpgmedia/internal/domain"
	"github.com/mikael/dpgmedia/internal/store"
)

func post(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	return postTo(t, newTestRouter(t), body)
}

func postTo(t *testing.T, router http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func listFrom(t *testing.T, router http.Handler, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/messages"+query, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func seedMessages(t *testing.T, messages store.MessageStore, n int) []string {
	t.Helper()

	ids := make([]string, 0, n)
	for i := range n {
		msg, err := messages.Create(t.Context(), domain.Message{
			ID:          fmt.Sprintf("msg_%02d", i),
			UserID:      "usr_98765",
			Type:        domain.TypeText,
			TextContent: fmt.Sprintf("hello %d", i),
		})
		require.NoError(t, err)
		ids = append(ids, msg.ID)
	}

	slices.Reverse(ids)
	return ids
}

type storeFailure struct{ store.MessageStore }

type mediaFailure struct{ store.MediaStore }

func (mediaFailure) Get(context.Context, string) (domain.Media, error) {
	return domain.Media{}, errors.New("catalog unavailable")
}

func (storeFailure) Create(context.Context, domain.Message) (domain.Message, error) {
	return domain.Message{}, errors.New("store unavailable")
}

func (storeFailure) List(context.Context, int) ([]domain.Message, error) {
	return nil, errors.New("store unavailable")
}

func TestCreateMessageAudio(t *testing.T) {
	rec := post(t, `{
		"user_id": "usr_98765",
		"message_type": "AUDIO",
		"text_content": "hello",
		"media_id": "med_abc890_m4a"
	}`)

	require.Equal(t, http.StatusCreated, rec.Code, "body: %s", rec.Body)

	data := decodeSuccess[createMessageData](t, rec)
	assert.True(t, strings.HasPrefix(data.MessageID, "msg_"), "message_id = %q, want msg_ prefix", data.MessageID)

	_, err := time.Parse(time.RFC3339, data.CreatedAt)
	assert.NoError(t, err, "created_at = %q, want RFC 3339", data.CreatedAt)
}

func TestCreateMessageIsStored(t *testing.T) {
	messages := newTestMessageStore(t)
	rec := postTo(t, newTestRouterWithMessageStore(t, messages), `{
		"user_id": "usr_98765",
		"message_type": "AUDIO",
		"text_content": "hello",
		"media_id": "med_abc890_m4a"
	}`)

	require.Equal(t, http.StatusCreated, rec.Code, "body: %s", rec.Body)
	data := decodeSuccess[createMessageData](t, rec)

	stored, err := messages.Get(t.Context(), data.MessageID)
	require.NoError(t, err, "the returned message_id must be readable from the store")

	assert.Equal(t, "usr_98765", stored.UserID)
	assert.Equal(t, domain.TypeAudio, stored.Type)
	assert.Equal(t, "med_abc890_m4a", stored.MediaID)
	assert.Equal(t, "hello", stored.TextContent)

	assert.Equal(t, data.CreatedAt, stored.CreatedAt.Format(time.RFC3339),
		"the stored record and the response report the same instant")
	assert.Equal(t, stored.CreatedAt.Add(testStoreTTL), stored.ExpiresAt)
}

func TestCreateMessageQueuesTranscription(t *testing.T) {
	messages := newTestMessageStore(t)
	transcriber := &recordingTranscriber{}

	rec := postTo(t, newTestRouterWithTranscriber(t, messages, transcriber), `{
		"user_id": "usr_98765",
		"message_type": "AUDIO",
		"media_id": "med_abc890_m4a"
	}`)

	require.Equal(t, http.StatusCreated, rec.Code, "body: %s", rec.Body)
	data := decodeSuccess[createMessageData](t, rec)
	assert.Equal(t, domain.StatusPendingTranscription, data.Status)

	queued := transcriber.jobs()
	require.Len(t, queued, 1, "an accepted audio message is handed to the transcription job")
	assert.Equal(t, data.MessageID, queued[0].messageID)
	assert.Equal(t, "file://assets/audio-berichten/voice-note.m4a", queued[0].uri,
		"the handler resolves the media uri so the job needs no second catalog lookup")
}

func TestCreateMessageWithoutMediaIsReadyImmediately(t *testing.T) {
	transcriber := &recordingTranscriber{}

	rec := postTo(t, newTestRouterWithTranscriber(t, newTestMessageStore(t), transcriber),
		`{"user_id":"usr_1","message_type":"TEXT","text_content":"hello"}`)

	require.Equal(t, http.StatusCreated, rec.Code, "body: %s", rec.Body)
	assert.Equal(t, domain.StatusReady, decodeSuccess[createMessageData](t, rec).Status)
	assert.Empty(t, transcriber.jobs(), "text carries no audio to transcribe")
}

func TestCreateMessageUnknownMedia(t *testing.T) {
	transcriber := &recordingTranscriber{}

	rec := postTo(t, newTestRouterWithTranscriber(t, newTestMessageStore(t), transcriber),
		`{"user_id":"usr_1","message_type":"AUDIO","media_id":"med_missing"}`)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "body: %s", rec.Body)
	body := decodeError(t, rec, errValidation)
	assert.Equal(t, details{
		{Field: "media_id", Issue: issueNotFound, Description: `No media found for id "med_missing"`},
	}, details(body.Details))
	assert.Empty(t, transcriber.jobs(), "a rejected message must not queue a job")
}

func TestCreateMessageMediaTypeMismatch(t *testing.T) {
	rec := post(t, `{"user_id":"usr_1","message_type":"AUDIO","media_id":"med_def123_mp4"}`)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "body: %s", rec.Body)
	body := decodeError(t, rec, errValidation)
	assert.Equal(t, details{
		{Field: "media_id", Issue: issueInvalid, Description: `Media "med_def123_mp4" is VIDEO, not AUDIO`},
	}, details(body.Details))
}

func TestCreateMessageMediaFailure(t *testing.T) {
	rec := postTo(t, newTestRouterWithMediaStore(t, mediaFailure{}),
		`{"user_id":"usr_1","message_type":"AUDIO","media_id":"med_abc890_m4a"}`)

	require.Equal(t, http.StatusInternalServerError, rec.Code, "body: %s", rec.Body)

	body := decodeError(t, rec, errInternal)
	assert.Empty(t, body.Details, "an internal failure must not leak store details")
}

func TestCreateMessageStoreFailure(t *testing.T) {
	rec := postTo(t, newTestRouterWithMessageStore(t, storeFailure{}),
		`{"user_id":"usr_1","message_type":"TEXT","text_content":"hello"}`)

	require.Equal(t, http.StatusInternalServerError, rec.Code, "body: %s", rec.Body)

	body := decodeError(t, rec, errInternal)
	assert.Empty(t, body.Details, "an internal failure must not leak store details")
}

func TestCreateMessageValidation(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantKind    errorKind
		wantDetails details
	}{
		{
			name: "text ok",
			body: `{"user_id":"usr_1","message_type":"TEXT","text_content":"hello"}`,
		},
		{
			name: "lowercase type accepted",
			body: `{"user_id":"usr_1","message_type":"text","text_content":"hi"}`,
		},
		{
			name:     "missing user_id",
			body:     `{"message_type":"TEXT","text_content":"hello"}`,
			wantKind: errValidation,
			wantDetails: details{
				{Field: "user_id", Issue: issueRequired, Description: "Required field"},
			},
		},
		{
			name:     "unknown type",
			body:     `{"user_id":"usr_1","message_type":"GIF","media_id":"med_1"}`,
			wantKind: errValidation,
			wantDetails: details{
				{Field: "message_type", Issue: issueInvalid, Description: "Must be one of TEXT, AUDIO, VIDEO, PICTURE"},
			},
		},
		{
			name:     "text without content",
			body:     `{"user_id":"usr_1","message_type":"TEXT"}`,
			wantKind: errValidation,
			wantDetails: details{
				{Field: "text_content", Issue: issueRequired, Description: "Required field for message_type TEXT"},
			},
		},
		{
			name:     "text with media",
			body:     `{"user_id":"usr_1","message_type":"TEXT","text_content":"hi","media_id":"med_1"}`,
			wantKind: errValidation,
			wantDetails: details{
				{Field: "media_id", Issue: issueNotAllowed, Description: "Must be empty for message_type TEXT"},
			},
		},
		{
			name:     "media without media_id",
			body:     `{"user_id":"usr_1","message_type":"PICTURE"}`,
			wantKind: errValidation,
			wantDetails: details{
				{Field: "media_id", Issue: issueRequired, Description: "Required field for message_type PICTURE"},
			},
		},
		{
			name:     "every field invalid at once",
			body:     `{"message_type":"TEXT"}`,
			wantKind: errValidation,
			wantDetails: details{
				{Field: "user_id", Issue: issueRequired, Description: "Required field"},
				{Field: "text_content", Issue: issueRequired, Description: "Required field for message_type TEXT"},
			},
		},
		{
			name:     "malformed json",
			body:     `{"user_id":`,
			wantKind: errInvalidRequest,
			wantDetails: details{
				{Field: "body", Issue: issueMalformed, Description: "Malformed JSON body: unexpected EOF"},
			},
		},
		{
			name:     "unknown field",
			body:     `{"user_id":"usr_1","message_type":"TEXT","text_content":"hi","foo":1}`,
			wantKind: errInvalidRequest,
			wantDetails: details{
				{Field: "body", Issue: issueMalformed, Description: `Malformed JSON body: json: unknown field "foo"`},
			},
		},
		{
			name:     "empty body",
			body:     ``,
			wantKind: errInvalidRequest,
			wantDetails: details{
				{Field: "body", Issue: issueEmpty, Description: "Request body is empty"},
			},
		},
		{
			name:     "trailing object",
			body:     `{"user_id":"usr_1","message_type":"TEXT","text_content":"hi"}{}`,
			wantKind: errInvalidRequest,
			wantDetails: details{
				{Field: "body", Issue: issueMalformed, Description: "Body must contain exactly one JSON object"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := post(t, tc.body)

			if tc.wantDetails == nil {
				require.Equal(t, http.StatusCreated, rec.Code, "body: %s", rec.Body)
				return
			}

			require.Equal(t, tc.wantKind.status, rec.Code, "body: %s", rec.Body)

			body := decodeError(t, rec, tc.wantKind)
			assert.Equal(t, tc.wantDetails, details(body.Details))
		})
	}
}

func TestCreateMessageWrongContentType(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)

	body := decodeError(t, rec, errInvalidRequest)
	assert.Equal(t, details{
		{Field: "content_type", Issue: issueUnsupported, Description: `Unsupported content type "text/plain", expected application/json`},
	}, details(body.Details))
}

func TestCreateMessageBodyTooLarge(t *testing.T) {
	body := `{"user_id":"usr_1","message_type":"TEXT","text_content":"` +
		strings.Repeat("a", testMaxBodyBytes) + `"}`

	rec := post(t, body)
	require.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body)

	errBody := decodeError(t, rec, errInvalidRequest)
	assert.Equal(t, details{
		{Field: "body", Issue: issueTooLarge, Description: fmt.Sprintf("Request body exceeds %d bytes", testMaxBodyBytes)},
	}, details(errBody.Details))
}

func TestErrorCarriesRequestID(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(``))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-Id", "req_8f92a10c")
	rec := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(rec, req)

	body := decodeError(t, rec, errInvalidRequest)
	assert.Equal(t, "req_8f92a10c", body.RequestID)
}

func TestListMessages(t *testing.T) {
	messages := newTestMessageStore(t)
	ids := seedMessages(t, messages, 3)

	rec := listFrom(t, newTestRouterWithMessageStore(t, messages), "")
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body)

	data := decodeSuccess[listMessagesData](t, rec)
	assert.Equal(t, listMeta{Count: 3, Limit: defaultListLimit}, data.Meta)

	got := make([]string, 0, len(data.Messages))
	for _, msg := range data.Messages {
		got = append(got, msg.MessageID)
	}
	assert.Equal(t, ids, got, "messages are returned newest first")

	newest := data.Messages[0]
	assert.Equal(t, "usr_98765", newest.UserID)
	assert.Equal(t, domain.TypeText, newest.MessageType)
	assert.Equal(t, "hello 2", newest.TextContent)
	assert.Empty(t, newest.MediaID)

	createdAt, err := time.Parse(time.RFC3339, newest.CreatedAt)
	assert.NoError(t, err, "created_at = %q, want RFC 3339", newest.CreatedAt)

	expiresAt, err := time.Parse(time.RFC3339, newest.ExpiresAt)
	require.NoError(t, err, "expires_at = %q, want RFC 3339", newest.ExpiresAt)
	assert.Equal(t, createdAt.Add(testStoreTTL), expiresAt)
}

func TestListMessagesContent(t *testing.T) {
	messages := newTestMessageStore(t)
	seedMessages(t, messages, 1)

	rec := listFrom(t, newTestRouterWithMessageStore(t, messages), "")
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body)

	msg := decodeSuccess[listMessagesData](t, rec).Messages[0]
	assert.NotEmpty(t, msg.MessageID)
	assert.Equal(t, "usr_98765", msg.UserID)
	assert.Equal(t, domain.TypeText, msg.MessageType)
	assert.Equal(t, "hello 0", msg.TextContent)
	assert.Empty(t, msg.MediaID)

	_, err := time.Parse(time.RFC3339, msg.CreatedAt)
	assert.NoError(t, err, "created_at = %q, want RFC 3339", msg.CreatedAt)
}

func TestListMessagesEmpty(t *testing.T) {
	rec := listFrom(t, newTestRouter(t), "")
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body)

	assert.JSONEq(t, `{"data":{"messages":[],"meta":{"count":0,"limit":50}}}`, rec.Body.String(),
		"an empty store must return an empty array, not null")
}

func TestListMessagesLimit(t *testing.T) {
	messages := newTestMessageStore(t)
	ids := seedMessages(t, messages, 3)

	rec := listFrom(t, newTestRouterWithMessageStore(t, messages), "?limit=2")
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body)

	data := decodeSuccess[listMessagesData](t, rec)
	assert.Equal(t, listMeta{Count: 2, Limit: 2}, data.Meta)
	require.Len(t, data.Messages, 2)
	assert.Equal(t, ids[0], data.Messages[0].MessageID)
	assert.Equal(t, ids[1], data.Messages[1].MessageID)
}

func TestListMessagesInvalidLimit(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		wantLimit   int
		wantDetails details
	}{
		{name: "absent", query: "", wantLimit: defaultListLimit},
		{name: "empty", query: "?limit=", wantLimit: defaultListLimit},
		{name: "blank", query: "?limit=%20", wantLimit: defaultListLimit},
		{name: "padded", query: "?limit=%2010%20", wantLimit: 10},
		{name: "maximum", query: fmt.Sprintf("?limit=%d", maxListLimit), wantLimit: maxListLimit},
		{
			name:  "not a number",
			query: "?limit=many",
			wantDetails: details{{Field: "limit", Issue: issueInvalid,
				Description: fmt.Sprintf("Must be an integer between 1 and %d", maxListLimit)}},
		},
		{
			name:  "zero",
			query: "?limit=0",
			wantDetails: details{{Field: "limit", Issue: issueInvalid,
				Description: fmt.Sprintf("Must be between 1 and %d", maxListLimit)}},
		},
		{
			name:  "negative",
			query: "?limit=-1",
			wantDetails: details{{Field: "limit", Issue: issueInvalid,
				Description: fmt.Sprintf("Must be between 1 and %d", maxListLimit)}},
		},
		{
			name:  "above maximum",
			query: fmt.Sprintf("?limit=%d", maxListLimit+1),
			wantDetails: details{{Field: "limit", Issue: issueInvalid,
				Description: fmt.Sprintf("Must be between 1 and %d", maxListLimit)}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := listFrom(t, newTestRouter(t), tc.query)

			if tc.wantDetails == nil {
				require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body)
				assert.Equal(t, tc.wantLimit, decodeSuccess[listMessagesData](t, rec).Meta.Limit)
				return
			}

			require.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body)
			body := decodeError(t, rec, errInvalidRequest)
			assert.Equal(t, tc.wantDetails, details(body.Details))
		})
	}
}

func TestListMessagesStoreFailure(t *testing.T) {
	rec := listFrom(t, newTestRouterWithMessageStore(t, storeFailure{}), "")
	require.Equal(t, http.StatusInternalServerError, rec.Code, "body: %s", rec.Body)

	body := decodeError(t, rec, errInternal)
	assert.Empty(t, body.Details, "an internal failure must not leak store details")
}

func TestMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/messages", nil)
	rec := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestListMessagesReportsTheFailure(t *testing.T) {
	messages := newTestMessageStore(t)

	msg, err := messages.Create(t.Context(), domain.Message{
		ID:      "msg_1",
		UserID:  "usr_98765",
		Type:    domain.TypeAudio,
		MediaID: "med_abc890_m4a",
	})
	require.NoError(t, err)
	require.Equal(t, domain.StatusPendingTranscription, msg.Status)

	_, err = messages.UpdateStatus(t.Context(), msg.ID, domain.StatusFailedTranscription,
		&domain.Failure{Code: domain.FailureMediaUnavailable, Reason: "media unavailable: media not found"})
	require.NoError(t, err)

	rec := listFrom(t, newTestRouterWithMessageStore(t, messages), "")
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body)

	got := decodeSuccess[listMessagesData](t, rec).Messages[0]
	assert.Equal(t, domain.StatusFailedTranscription, got.Status)
	require.NotNil(t, got.Failure, "a failed message reports why it failed")
	assert.Equal(t, domain.FailureMediaUnavailable, got.Failure.Code)
	assert.Equal(t, "media unavailable: media not found", got.Failure.Reason)

	_, err = time.Parse(time.RFC3339, got.Failure.FailedAt)
	assert.NoError(t, err, "failed_at = %q, want RFC 3339", got.Failure.FailedAt)
}

func TestListMessagesOmitsTheFailureWhenThereIsNone(t *testing.T) {
	messages := newTestMessageStore(t)
	seedMessages(t, messages, 1)

	rec := listFrom(t, newTestRouterWithMessageStore(t, messages), "")
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body)

	assert.NotContains(t, rec.Body.String(), `"failure"`)
	assert.Contains(t, rec.Body.String(), `"status":"READY"`)
}
