package config

import (
	"os"
	"strconv"
	"time"
)

// Config is read from the environment, like the other AEX services.
type Config struct {
	Port        string
	Environment string

	// PolicyFile is the provider's policy (scopes per tool, argument rules).
	PolicyFile string
	// UpstreamURL is the provider's tool endpoint; the gate forwards an allowed
	// call to UpstreamURL + UpstreamPrefix + "/" + tool.
	UpstreamURL    string
	UpstreamPrefix string
	// Mode is "full" (scope, then argument-value rules) or "scope" (scope only;
	// the gateway's model moved to the call, kept for comparison runs).
	Mode string
	// NATSURL, when set, publishes every toolcall.* event into JetStream.
	NATSURL string

	UpstreamTimeout time.Duration
}

func Load() *Config {
	return &Config{
		Port:            getEnv("PORT", "8090"),
		Environment:     getEnv("ENVIRONMENT", "development"),
		PolicyFile:      getEnv("POLICY_FILE", "/etc/toolgate/policy.json"),
		UpstreamURL:     getEnv("UPSTREAM_URL", "http://localhost:8091"),
		UpstreamPrefix:  getEnv("UPSTREAM_PREFIX", "/tools"),
		Mode:            getEnv("MODE", "full"),
		NATSURL:         getEnv("NATS_URL", ""),
		UpstreamTimeout: time.Duration(getEnvInt("UPSTREAM_TIMEOUT_SECONDS", 20)) * time.Second,
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}
