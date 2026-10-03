package config

import (
	"os"
	"strconv"
	"strings"
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
	CertAuthURL         string
	// ToolsURL is the provider tool endpoint behind /v1/tools/; empty leaves
	// the route unmounted.
	ToolsURL string

	// APIKeyValidator selects how X-API-Key is checked: "identity" (call the
	// identity service at IdentityURL, the production path) or "memory" (an
	// empty in-memory table, for tests that add keys explicitly).
	APIKeyValidator string

	// RouteScopesFile is an optional JSON file merged over the default route
	// scope map ({"METHOD /prefix": "scope"}).
	RouteScopesFile string

	// Rate limiting
	RateLimitPerMinute int
	RateLimitBurstSize int

	// Timeouts
	RequestTimeout time.Duration
	ProxyTimeout   time.Duration

	// Redis
	RedisURL string

	// Auth
	JWTSecret string

	// AllowedOrigins lists the browser origins allowed by CORS, from the
	// comma-separated ALLOWED_ORIGINS; "*" (the default) allows any origin.
	// Production should name its origins explicitly.
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
		CertAuthURL:         getEnv("CERTAUTH_URL", "http://localhost:8089"),
		ToolsURL:            getEnv("TOOLS_URL", ""),
		APIKeyValidator:     getEnv("API_KEY_VALIDATOR", "identity"),
		RouteScopesFile:     getEnv("ROUTE_SCOPES_FILE", ""),
		RateLimitPerMinute:  getEnvInt("RATE_LIMIT_PER_MINUTE", 1000),
		RateLimitBurstSize:  getEnvInt("RATE_LIMIT_BURST_SIZE", 50),
		RequestTimeout:      time.Duration(getEnvInt("REQUEST_TIMEOUT_SECONDS", 30)) * time.Second,
		ProxyTimeout:        time.Duration(getEnvInt("PROXY_TIMEOUT_SECONDS", 25)) * time.Second,
		RedisURL:            getEnv("REDIS_URL", "redis://localhost:6379"),
		JWTSecret:           getEnv("JWT_SECRET", ""),
		AllowedOrigins:      getEnvList("ALLOWED_ORIGINS", []string{"*"}),
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

// getEnvList splits a comma-separated list of origins, trimming spaces and a
// trailing "/" (browsers send Origin without one) and dropping empty entries;
// unset or no entries gives defaultValue.
func getEnvList(key string, defaultValue []string) []string {
	var out []string
	for _, item := range strings.Split(os.Getenv(key), ",") {
		if item = strings.TrimSuffix(strings.TrimSpace(item), "/"); item != "" {
			out = append(out, item)
		}
	}
	if len(out) == 0 {
		return defaultValue
	}
	return out
}
