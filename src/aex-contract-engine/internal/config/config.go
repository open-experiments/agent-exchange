package config

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	Port string

	// Bid Gateway (used to fetch bid details when awarding)
	BidGatewayURL string

	// Work Publisher (looked up on award for the work's consumer; optional,
	// consumer recorded as "unknown" when empty)
	WorkPublisherURL string

	// Settlement (notified when a contract completes; optional, skipped when empty)
	SettlementURL string

	// MongoDB (optional persistence)
	MongoURI        string
	MongoDatabase   string
	MongoCollection string

	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

func Load() Config {
	return Config{
		Port:             getenv("PORT", "8080"),
		BidGatewayURL:    strings.TrimRight(strings.TrimSpace(os.Getenv("BID_GATEWAY_URL")), "/"),
		WorkPublisherURL: strings.TrimRight(strings.TrimSpace(os.Getenv("WORK_PUBLISHER_URL")), "/"),
		SettlementURL:    strings.TrimRight(strings.TrimSpace(os.Getenv("SETTLEMENT_URL")), "/"),
		MongoURI:         strings.TrimSpace(os.Getenv("MONGO_URI")),
		MongoDatabase:    getenv("MONGO_DB", "aex"),
		MongoCollection:  getenv("MONGO_COLLECTION_CONTRACTS", "contracts"),
		ReadTimeout:      10 * time.Second,
		WriteTimeout:     20 * time.Second,
		IdleTimeout:      60 * time.Second,
	}
}

func getenv(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}
