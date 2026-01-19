package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port           string
	Environment    string
	LogLevel       string
	MaxLogEntries  int
	MaxMetricItems int
}

func Load() *Config {
	return &Config{
		Port:           getEnv("PORT", "8089"),
		Environment:    getEnv("ENVIRONMENT", "development"),
		LogLevel:       getEnv("LOG_LEVEL", "info"),
		MaxLogEntries:  getEnvInt("MAX_LOG_ENTRIES", 10000),
		MaxMetricItems: getEnvInt("MAX_METRIC_ITEMS", 10000),
	}
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultValue
}
