package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func post(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, req)
	return rec
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
	newTestRouter().ServeHTTP(rec, req)

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
	newTestRouter().ServeHTTP(rec, req)

	body := decodeError(t, rec, errInvalidRequest)
	assert.Equal(t, "req_8f92a10c", body.RequestID)
}

func TestMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/messages", nil)
	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}
