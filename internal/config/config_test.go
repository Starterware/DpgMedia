package config

import (
	"log/slog"
	"testing"
	"time"

	"github.com/mikael/dpgmedia/internal/logging"
	"github.com/mikael/dpgmedia/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := LoadArgs(nil)
	require.NoError(t, err)

	want := &Config{
		AppEnv: "development",
		Port:   8080,
		Server: ServerConfig{
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       120 * time.Second,
			ShutdownTimeout:   10 * time.Second,
			MaxBodyBytes:      32 << 10,
		},
		Store: StoreConfig{
			Driver:      "local",
			MessagePath: "data/messages.jsonl",
			MediaPath:   "data/media/catalog.json",
			MessageTTL:  7 * 24 * time.Hour,
		},
		Transcription: TranscriptionConfig{
			Model:   "whisper-1",
			BaseURL: "https://api.openai.com/v1",
			Timeout: 2 * time.Minute,
		},
		LogConfig: LogConfig{Level: "info", Format: "json"},
	}
	assert.Equal(t, want, cfg)
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("PORT", "9000")
	t.Setenv("SERVER_READ_TIMEOUT", "30s")
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := LoadArgs(nil)
	require.NoError(t, err)

	assert.Equal(t, 9000, cfg.Port)
	assert.Equal(t, 30*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, "debug", cfg.LogConfig.Level)
}

func TestLoadReadsTheAPIKeyFromTheEnvironmentOnly(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-secret")

	cfg, err := LoadArgs(nil)
	require.NoError(t, err)
	assert.Equal(t, "sk-secret", cfg.Transcription.APIKey)

	cfg, err = LoadArgs([]string{"-openai-api-key", "sk-other"})
	require.Error(t, err, "the key must not be settable as a flag")
	assert.Nil(t, cfg)
}

func TestLoadTreatsABlankAPIKeyAsUnset(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "  \n")

	cfg, err := LoadArgs(nil)
	require.NoError(t, err)
	assert.Empty(t, cfg.Transcription.APIKey,
		"whitespace must not be sent as a bearer token, it reads as a missing key at the API")
}

func TestFlagsOverrideEnv(t *testing.T) {
	t.Setenv("PORT", "9000")
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := LoadArgs([]string{"-port", "9100", "-log-level", "warn", "-server-read-timeout", "1m"})
	require.NoError(t, err)

	assert.Equal(t, 9100, cfg.Port, "flag must beat env")
	assert.Equal(t, "warn", cfg.LogConfig.Level)
	assert.Equal(t, time.Minute, cfg.Server.ReadTimeout)
}

func TestUnparsableEnvFallsBackToDefault(t *testing.T) {
	t.Setenv("PORT", "eight-thousand")
	t.Setenv("SERVER_READ_TIMEOUT", "quickly")
	t.Setenv("SERVER_MAX_BODY_BYTES", "lots")

	cfg, err := LoadArgs(nil)
	require.NoError(t, err)

	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, 15*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, int64(32<<10), cfg.Server.MaxBodyBytes)
}

func TestUnparsableArgs(t *testing.T) {
	cfg, err := LoadArgs([]string{"-nope"})
	require.Error(t, err)
	assert.Nil(t, cfg, "a config that failed to parse must not reach the caller")
}

func TestValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"port too high", []string{"-port", "70000"}, "port (PORT)"},
		{"port zero", []string{"-port", "0"}, "port (PORT)"},
		{"empty env name", []string{"-env", ""}, "env (APP_ENV)"},
		{"negative read header timeout", []string{"-server-read-header-timeout=-1s"}, "server-read-header-timeout"},
		{"negative read timeout", []string{"-server-read-timeout=-1s"}, "server-read-timeout"},
		{"negative write timeout", []string{"-server-write-timeout=-1s"}, "server-write-timeout"},
		{"negative idle timeout", []string{"-server-idle-timeout=-1s"}, "server-idle-timeout"},
		{"zero shutdown timeout", []string{"-server-shutdown-timeout", "0s"}, "server-shutdown-timeout"},
		{"non-positive body cap", []string{"-server-max-body-bytes", "0"}, "server-max-body-bytes"},
		{"unknown store driver", []string{"-store-driver", "dynamodb"}, "store-driver (STORE_DRIVER)"},
		{"empty media path", []string{"-store-media-path", ""}, "store-media-path (STORE_MEDIA_PATH)"},
		{"zero message ttl", []string{"-store-message-ttl", "0s"}, "store-message-ttl (STORE_MESSAGE_TTL)"},
		{"empty transcription model", []string{"-transcription-model", ""}, "transcription-model (TRANSCRIPTION_MODEL)"},
		{"empty transcription base url", []string{"-transcription-base-url", ""}, "transcription-base-url (TRANSCRIPTION_BASE_URL)"},
		{"zero transcription timeout", []string{"-transcription-timeout", "0s"}, "transcription-timeout (TRANSCRIPTION_TIMEOUT)"},
		{"negative transcription timeout", []string{"-transcription-timeout=-1s"}, "transcription-timeout (TRANSCRIPTION_TIMEOUT)"},
		{"unknown log level", []string{"-log-level", "chatty"}, "log-level (LOG_LEVEL)"},
		{"unknown log format", []string{"-log-format", "yaml"}, "log-format (LOG_FORMAT)"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := LoadArgs(tc.args)
			require.Error(t, err)
			assert.Nil(t, cfg)
			assert.ErrorContains(t, err, tc.want)
		})
	}
}

func TestLogConfigOptions(t *testing.T) {
	cfg := &Config{LogConfig: LogConfig{Level: "debug", Format: "text"}}

	opts := cfg.LogConfigOptions()

	assert.Equal(t, slog.LevelDebug, opts.Level)
	assert.Equal(t, logging.FormatText, opts.Format)
}

func TestStoreDriver(t *testing.T) {
	cfg := &Config{Store: StoreConfig{Driver: "local"}}

	assert.Equal(t, store.DriverLocal, cfg.StoreDriver())
}

func TestLog(t *testing.T) {
	cfg := &Config{
		AppEnv: "production",
		Port:   9000,
		Server: ServerConfig{
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       2 * time.Minute,
			ShutdownTimeout:   10 * time.Second,
			MaxBodyBytes:      2048,
		},
		Store: StoreConfig{
			Driver:      "local",
			MessagePath: "data/messages.jsonl",
			MediaPath:   "data/media/catalog.json",
			MessageTTL:  7 * 24 * time.Hour,
		},
		Transcription: TranscriptionConfig{
			APIKey:  "sk-secret",
			Model:   "whisper-1",
			BaseURL: "https://api.openai.com/v1",
			Timeout: 2 * time.Minute,
		},
		LogConfig: LogConfig{Level: "debug", Format: "text"},
	}

	value := cfg.LogValue()
	require.Equal(t, slog.KindGroup, value.Kind())

	attrs := make(map[string]string, len(value.Group()))
	for _, attr := range value.Group() {
		attrs[attr.Key] = attr.Value.String()
	}

	assert.Equal(t, map[string]string{
		"app_env":                   "production",
		"port":                      "9000",
		"read_header_timeout":       "5s",
		"read_timeout":              "15s",
		"write_timeout":             "15s",
		"idle_timeout":              "2m0s",
		"shutdown_timeout":          "10s",
		"max_body_bytes":            "2048",
		"store_driver":              "local",
		"store_message_path":        "data/messages.jsonl",
		"store_media_path":          "data/media/catalog.json",
		"store_message_ttl":         "168h0m0s",
		"transcription_model":       "whisper-1",
		"transcription_base_url":    "https://api.openai.com/v1",
		"transcription_timeout":     "2m0s",
		"transcription_api_key_set": "true",
		"log_level":                 "debug",
		"log_format":                "text",
	}, attrs)
}
