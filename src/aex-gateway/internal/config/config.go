package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port        string
	Environment string

	// Upstream service URLs
	WorkPublisherURL    string
	ProviderRegistryURL string
	SettlementURL       string
	BidGatewayURL       string
	BidEvaluatorURL     string
	ContractEngineURL   string
	TrustBrokerURL      string
	IdentityURL         string

	// Rate limiting
	RateLimitPerMinute int
	RateLimitBurstSize int

	// Timeouts
	RequestTimeout time.Duration
	ProxyTimeout   time.Duration

	// CORS
	AllowedOrigins []string

	// Logging
	LogLevel string
}

func Load() *Config {
	return &Config{
		Port:                getEnv("PORT", "8080"),
		Environment:         getEnv("ENVIRONMENT", "development"),
		WorkPublisherURL:    getEnv("WORK_PUBLISHER_URL", "http://localhost:8081"),
		ProviderRegistryURL: getEnv("PROVIDER_REGISTRY_URL", "http://localhost:8085"),
		SettlementURL:       getEnv("SETTLEMENT_URL", "http://localhost:8088"),
		BidGatewayURL:       getEnv("BID_GATEWAY_URL", "http://localhost:8082"),
		BidEvaluatorURL:     getEnv("BID_EVALUATOR_URL", "http://localhost:8083"),
		ContractEngineURL:   getEnv("CONTRACT_ENGINE_URL", "http://localhost:8084"),
		TrustBrokerURL:      getEnv("TRUST_BROKER_URL", "http://localhost:8086"),
		IdentityURL:         getEnv("IDENTITY_URL", "http://localhost:8087"),
		RateLimitPerMinute:  getEnvInt("RATE_LIMIT_PER_MINUTE", 1000),
		RateLimitBurstSize:  getEnvInt("RATE_LIMIT_BURST_SIZE", 50),
		RequestTimeout:      time.Duration(getEnvInt("REQUEST_TIMEOUT_SECONDS", 30)) * time.Second,
		ProxyTimeout:        time.Duration(getEnvInt("PROXY_TIMEOUT_SECONDS", 25)) * time.Second,
		AllowedOrigins:      []string{"*"},
		LogLevel:            getEnv("LOG_LEVEL", "info"),
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

