package config

import (
	"os"
	"strconv"
	"time"
)

func getEnv(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if val, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if val, ok := os.LookupEnv(key); ok {
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if val, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultValue
}
