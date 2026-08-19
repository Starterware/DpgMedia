package config

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/mikael/dpgmedia/internal/logging"
	"github.com/mikael/dpgmedia/internal/store"
	"github.com/mikael/dpgmedia/internal/transcription"
)

type Config struct {
	AppEnv        string
	Port          int
	Server        ServerConfig
	Store         StoreConfig
	Transcription TranscriptionConfig
	LogConfig     LogConfig
}

type ServerConfig struct {
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	MaxBodyBytes      int64
}

type StoreConfig struct {
	Driver      string
	MessagePath string
	MediaPath   string
	MessageTTL  time.Duration
}

type TranscriptionConfig struct {
	APIKey  string
	Model   string
	BaseURL string
	Timeout time.Duration
}

type LogConfig struct {
	Level  string
	Format string
}

func Load() (*Config, error) {
	return LoadArgs(os.Args[1:])
}

func LoadArgs(args []string) (*Config, error) {
	cfg := &Config{}
	fs := flag.NewFlagSet("server", flag.ContinueOnError)

	fs.StringVar(&cfg.AppEnv, "env", getEnv("APP_ENV", "development"), "Application environment")
	fs.IntVar(&cfg.Port, "port", getEnvInt("PORT", 8080), "HTTP server port")

	fs.DurationVar(&cfg.Server.ReadHeaderTimeout, "server-read-header-timeout", getEnvDuration("SERVER_READ_HEADER_TIMEOUT", 5*time.Second), "Server read header timeout")
	fs.DurationVar(&cfg.Server.ReadTimeout, "server-read-timeout", getEnvDuration("SERVER_READ_TIMEOUT", 15*time.Second), "Server read timeout")
	fs.DurationVar(&cfg.Server.WriteTimeout, "server-write-timeout", getEnvDuration("SERVER_WRITE_TIMEOUT", 15*time.Second), "Server write timeout")
	fs.DurationVar(&cfg.Server.IdleTimeout, "server-idle-timeout", getEnvDuration("SERVER_IDLE_TIMEOUT", 120*time.Second), "Server idle timeout")
	fs.DurationVar(&cfg.Server.ShutdownTimeout, "server-shutdown-timeout", getEnvDuration("SERVER_SHUTDOWN_TIMEOUT", 10*time.Second), "Graceful shutdown drain timeout")
	fs.Int64Var(&cfg.Server.MaxBodyBytes, "server-max-body-bytes", getEnvInt64("SERVER_MAX_BODY_BYTES", 32<<10), "Maximum accepted request body size in bytes")

	fs.StringVar(&cfg.Store.Driver, "store-driver", getEnv("STORE_DRIVER", string(store.DriverLocal)), "Message store driver (local)")
	fs.StringVar(&cfg.Store.MessagePath, "store-message-path", getEnv("STORE_MESSAGE_PATH", "data/messages.jsonl"), "Message store file for the local driver (empty keeps messages in memory only)")
	fs.StringVar(&cfg.Store.MediaPath, "store-media-path", getEnv("STORE_MEDIA_PATH", "data/media/catalog.json"), "Media catalog file listing the available media")
	fs.DurationVar(&cfg.Store.MessageTTL, "store-message-ttl", getEnvDuration("STORE_MESSAGE_TTL", 7*24*time.Hour), "How long a stored message is retained")

	cfg.Transcription.APIKey = strings.TrimSpace(getEnv("OPENAI_API_KEY", ""))
	fs.StringVar(&cfg.Transcription.Model, "transcription-model", getEnv("TRANSCRIPTION_MODEL", transcription.DefaultModel), "Speech-to-text model used to transcribe audio messages")
	fs.StringVar(&cfg.Transcription.BaseURL, "transcription-base-url", getEnv("TRANSCRIPTION_BASE_URL", transcription.DefaultBaseURL), "Base URL of the OpenAI-compatible transcription API")
	fs.DurationVar(&cfg.Transcription.Timeout, "transcription-timeout", getEnvDuration("TRANSCRIPTION_TIMEOUT", transcription.DefaultTimeout), "Time limit for a single transcription request")

	fs.StringVar(&cfg.LogConfig.Level, "log-level", getEnv("LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")
	fs.StringVar(&cfg.LogConfig.Format, "log-format", getEnv("LOG_FORMAT", "json"), "Log format (json, text)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.AppEnv == "" {
		return errors.New("env (APP_ENV) is required")
	}

	if c.Port <= 0 || c.Port > 65535 {
		return errors.New("port (PORT) must be between 1 and 65535")
	}

	if c.Server.ReadHeaderTimeout < 0 {
		return errors.New("server-read-header-timeout (SERVER_READ_HEADER_TIMEOUT) must not be negative")
	}
	if c.Server.ReadTimeout < 0 {
		return errors.New("server-read-timeout (SERVER_READ_TIMEOUT) must not be negative")
	}
	if c.Server.WriteTimeout < 0 {
		return errors.New("server-write-timeout (SERVER_WRITE_TIMEOUT) must not be negative")
	}
	if c.Server.IdleTimeout < 0 {
		return errors.New("server-idle-timeout (SERVER_IDLE_TIMEOUT) must not be negative")
	}
	if c.Server.ShutdownTimeout <= 0 {
		return errors.New("server-shutdown-timeout (SERVER_SHUTDOWN_TIMEOUT) must be positive")
	}
	if c.Server.MaxBodyBytes <= 0 {
		return errors.New("server-max-body-bytes (SERVER_MAX_BODY_BYTES) must be positive")
	}

	if _, err := store.ParseDriver(c.Store.Driver); err != nil {
		return fmt.Errorf("store-driver (STORE_DRIVER): %w", err)
	}
	if c.Store.MediaPath == "" {
		return errors.New("store-media-path (STORE_MEDIA_PATH) is required")
	}
	if c.Store.MessageTTL <= 0 {
		return errors.New("store-message-ttl (STORE_MESSAGE_TTL) must be positive")
	}

	if c.Transcription.Model == "" {
		return errors.New("transcription-model (TRANSCRIPTION_MODEL) is required")
	}
	if c.Transcription.BaseURL == "" {
		return errors.New("transcription-base-url (TRANSCRIPTION_BASE_URL) is required")
	}
	if c.Transcription.Timeout <= 0 {
		return errors.New("transcription-timeout (TRANSCRIPTION_TIMEOUT) must be positive")
	}

	if _, err := logging.ParseLevel(c.LogConfig.Level); err != nil {
		return fmt.Errorf("log-level (LOG_LEVEL): %w", err)
	}
	if _, err := logging.ParseFormat(c.LogConfig.Format); err != nil {
		return fmt.Errorf("log-format (LOG_FORMAT): %w", err)
	}

	return nil
}

func (c *Config) LogConfigOptions() logging.Options {
	level, _ := logging.ParseLevel(c.LogConfig.Level)
	format, _ := logging.ParseFormat(c.LogConfig.Format)
	return logging.Options{Level: level, Format: format}
}

func (c *Config) StoreDriver() store.Driver {
	driver, _ := store.ParseDriver(c.Store.Driver)
	return driver
}

func (c *Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("app_env", c.AppEnv),
		slog.Int("port", c.Port),
		slog.Duration("read_header_timeout", c.Server.ReadHeaderTimeout),
		slog.Duration("read_timeout", c.Server.ReadTimeout),
		slog.Duration("write_timeout", c.Server.WriteTimeout),
		slog.Duration("idle_timeout", c.Server.IdleTimeout),
		slog.Duration("shutdown_timeout", c.Server.ShutdownTimeout),
		slog.Int64("max_body_bytes", c.Server.MaxBodyBytes),
		slog.String("store_driver", c.Store.Driver),
		slog.String("store_message_path", c.Store.MessagePath),
		slog.String("store_media_path", c.Store.MediaPath),
		slog.Duration("store_message_ttl", c.Store.MessageTTL),
		slog.String("transcription_model", c.Transcription.Model),
		slog.String("transcription_base_url", c.Transcription.BaseURL),
		slog.Duration("transcription_timeout", c.Transcription.Timeout),
		slog.Bool("transcription_api_key_set", c.Transcription.APIKey != ""),
		slog.String("log_level", c.LogConfig.Level),
		slog.String("log_format", c.LogConfig.Format),
	)
}
