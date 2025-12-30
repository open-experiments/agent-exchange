package config

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	Port string

	// Auth
	ProviderAPIKeys     map[string]string // apiKey -> providerID (static fallback)
	ProviderRegistryURL string            // Provider registry URL for dynamic validation

	// MongoDB (local persistence)
	MongoURI        string
	MongoDatabase   string
	MongoCollection string

	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

func Load() Config {
	cfg := Config{
		Port:                getenv("PORT", "8080"),
		ProviderRegistryURL: strings.TrimSpace(os.Getenv("PROVIDER_REGISTRY_URL")),
		MongoURI:            strings.TrimSpace(os.Getenv("MONGO_URI")),
		MongoDatabase:       getenv("MONGO_DB", "aex"),
		MongoCollection:     getenv("MONGO_COLLECTION_BIDS", "bids"),
		ReadTimeout:         10 * time.Second,
		WriteTimeout:        20 * time.Second,
		IdleTimeout:         60 * time.Second,
	}

	cfg.ProviderAPIKeys = parseProviderAPIKeys(os.Getenv("PROVIDER_API_KEYS"))
	return cfg
}

func parseProviderAPIKeys(raw string) map[string]string {
	// Format: "prov_expedia:key1,prov_booking:key2"
	out := map[string]string{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
	pairs := strings.Split(raw, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 {
			continue
		}
		providerID := strings.TrimSpace(parts[0])
		apiKey := strings.TrimSpace(parts[1])
		if providerID == "" || apiKey == "" {
			continue
		}
		out[apiKey] = providerID
	}
	return out
}

func getenv(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}
