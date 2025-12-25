package config

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	Port string

	MongoURI               string
	MongoDatabase          string
	MongoCollectionTenants string
	MongoCollectionAPIKeys string

	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

func Load() Config {
	return Config{
		Port:                   getenv("PORT", "8080"),
		MongoURI:               strings.TrimSpace(os.Getenv("MONGO_URI")),
		MongoDatabase:          getenv("MONGO_DB", "aex"),
		MongoCollectionTenants: getenv("MONGO_COLLECTION_TENANTS", "tenants"),
		MongoCollectionAPIKeys: getenv("MONGO_COLLECTION_APIKEYS", "api_keys"),
		ReadTimeout:            10 * time.Second,
		WriteTimeout:           20 * time.Second,
		IdleTimeout:            60 * time.Second,
	}
}

func getenv(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

