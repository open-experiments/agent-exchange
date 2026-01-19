package config

import (
	"os"
)

// Config holds the service configuration.
type Config struct {
	Port     string
	MongoURI string
	MongoDB  string
}

// Load reads configuration from environment variables.
func Load() *Config {
	return &Config{
		Port:     getEnv("PORT", "8111"),
		MongoURI: getEnv("MONGO_URI", ""),
		MongoDB:  getEnv("MONGO_DB", "aex"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
