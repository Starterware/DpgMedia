package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetEnv(t *testing.T) {
	assert.Equal(t, "fallback", getEnv("TEST_UNSET", "fallback"))

	t.Setenv("TEST_STR", "value")
	assert.Equal(t, "value", getEnv("TEST_STR", "fallback"))

	t.Setenv("TEST_STR", "")
	assert.Empty(t, getEnv("TEST_STR", "fallback"))
}

func TestGetEnvInt(t *testing.T) {
	assert.Equal(t, 7, getEnvInt("TEST_UNSET", 7))

	t.Setenv("TEST_INT", "42")
	assert.Equal(t, 42, getEnvInt("TEST_INT", 7))

	t.Setenv("TEST_INT", "forty-two")
	assert.Equal(t, 7, getEnvInt("TEST_INT", 7), "unparsable value falls back to the default")
}

func TestGetEnvInt64(t *testing.T) {
	assert.Equal(t, int64(32<<10), getEnvInt64("TEST_UNSET", 32<<10))

	t.Setenv("TEST_INT64", "9007199254740993")
	assert.Equal(t, int64(9007199254740993), getEnvInt64("TEST_INT64", 0))

	t.Setenv("TEST_INT64", "2048.5")
	assert.Equal(t, int64(1024), getEnvInt64("TEST_INT64", 1024), "unparsable value falls back to the default")
}

func TestGetEnvDuration(t *testing.T) {
	assert.Equal(t, 5*time.Second, getEnvDuration("TEST_UNSET", 5*time.Second))

	t.Setenv("TEST_DURATION", "2m30s")
	assert.Equal(t, 150*time.Second, getEnvDuration("TEST_DURATION", 5*time.Second))

	t.Setenv("TEST_DURATION", "30")
	assert.Equal(t, 5*time.Second, getEnvDuration("TEST_DURATION", 5*time.Second), "unitless value falls back to the default")
}
