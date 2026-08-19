package transcription

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Speech interface {
	Transcribe(ctx context.Context, path string) (string, error)
}

const (
	DefaultBaseURL = "https://api.openai.com/v1"
	DefaultModel   = "whisper-1"
	DefaultTimeout = 2 * time.Minute
)

type WhisperOptions struct {
	APIKey  string
	Model   string
	BaseURL string
	Timeout time.Duration
	Client  *http.Client
}

type Whisper struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

var _ Speech = (*Whisper)(nil)

func NewWhisper(opts WhisperOptions) *Whisper {
	model := opts.Model
	if model == "" {
		model = DefaultModel
	}

	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	client := opts.Client
	if client == nil {
		timeout := opts.Timeout
		if timeout == 0 {
			timeout = DefaultTimeout
		}
		client = &http.Client{Timeout: timeout}
	}

	return &Whisper{
		apiKey:  strings.TrimSpace(opts.APIKey),
		model:   model,
		baseURL: strings.TrimSuffix(baseURL, "/"),
		client:  client,
	}
}

var ErrNoAPIKey = errors.New("no api key configured")

const maxResponseBytes = 1 << 20

func (w *Whisper) Transcribe(ctx context.Context, path string) (string, error) {
	if w.apiKey == "" {
		return "", ErrNoAPIKey
	}

	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	pr, pw := io.Pipe()
	form := multipart.NewWriter(pw)
	go func() { _ = pw.CloseWithError(w.writeForm(form, file, filepath.Base(path))) }()
	defer func() { _ = pr.Close() }()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.baseURL+"/audio/transcriptions", pr)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+w.apiKey)
	req.Header.Set("Content-Type", form.FormDataContentType())

	resp, err := w.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("call %s: %w", w.model, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("read %s response: %w", w.model, err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s responded %d: %s", w.model, resp.StatusCode, apiError(body))
	}

	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decode %s response: %w", w.model, err)
	}

	text := strings.TrimSpace(payload.Text)
	if text == "" {
		return "", fmt.Errorf("%s returned an empty transcript", w.model)
	}

	return text, nil
}

func (w *Whisper) writeForm(form *multipart.Writer, file io.Reader, name string) error {
	part, err := form.CreateFormFile("file", name)
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, file); err != nil {
		return err
	}
	if err := form.WriteField("model", w.model); err != nil {
		return err
	}
	return form.Close()
}

func apiError(body []byte) string {
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.Error.Message != "" {
		return payload.Error.Message
	}

	return strings.TrimSpace(string(body))
}
