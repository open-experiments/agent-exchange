package config

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	Port string

	BidGatewayURL  string // required
	TrustBrokerURL string // optional

	// MongoDB (optional persistence)
	MongoURI        string
	MongoDatabase   string
	MongoCollection string

	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

func Load() Config {
	cfg := Config{
		Port:            getenv("PORT", "8080"),
		BidGatewayURL:   strings.TrimRight(strings.TrimSpace(os.Getenv("BID_GATEWAY_URL")), "/"),
		TrustBrokerURL:  strings.TrimRight(strings.TrimSpace(os.Getenv("TRUST_BROKER_URL")), "/"),
		MongoURI:        strings.TrimSpace(os.Getenv("MONGO_URI")),
		MongoDatabase:   getenv("MONGO_DB", "aex"),
		MongoCollection: getenv("MONGO_COLLECTION_EVALUATIONS", "bid_evaluations"),
		ReadTimeout:     10 * time.Second,
		WriteTimeout:    20 * time.Second,
		IdleTimeout:     60 * time.Second,
	}
	return cfg
}

func getenv(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

