package config

import (
	"log/slog"
	"testing"
	"time"

	"github.com/mikael/dpgmedia/internal/logging"
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

func TestValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"port too high", []string{"-port", "70000"}, "port (PORT)"},
		{"port zero", []string{"-port", "0"}, "port (PORT)"},
		{"empty env name", []string{"-env", ""}, "env (APP_ENV)"},
		{"negative read timeout", []string{"-server-read-timeout=-1s"}, "server-read-timeout"},
		{"zero shutdown timeout", []string{"-server-shutdown-timeout", "0s"}, "server-shutdown-timeout"},
		{"non-positive body cap", []string{"-server-max-body-bytes", "0"}, "server-max-body-bytes"},
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

// Accepted level and format spellings are covered by logging_test.go; this only
// checks that the settings reach the logger options.
func TestLogConfigOptions(t *testing.T) {
	cfg := &Config{LogConfig: LogConfig{Level: "debug", Format: "text"}}

	opts := cfg.LogConfigOptions()

	assert.Equal(t, slog.LevelDebug, opts.Level)
	assert.Equal(t, logging.FormatText, opts.Format)
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
		LogConfig: LogConfig{Level: "debug", Format: "text"},
	}

	value := cfg.LogValue()
	require.Equal(t, slog.KindGroup, value.Kind())

	attrs := make(map[string]string, len(value.Group()))
	for _, attr := range value.Group() {
		attrs[attr.Key] = attr.Value.String()
	}

	assert.Equal(t, map[string]string{
		"app_env":             "production",
		"port":                "9000",
		"read_header_timeout": "5s",
		"read_timeout":        "15s",
		"write_timeout":       "15s",
		"idle_timeout":        "2m0s",
		"shutdown_timeout":    "10s",
		"max_body_bytes":      "2048",
		"log_level":           "debug",
		"log_format":          "text",
	}, attrs)
}
