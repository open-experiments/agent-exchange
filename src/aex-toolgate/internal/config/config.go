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
	// OperatorToken is the bearer token the operator endpoints require
	// (settling a held call, reading the record chain). Without it those
	// endpoints are refused: they must not be reachable by the gated agent.
	OperatorToken string
	// ExposeArgs serves full argument values on GET /v1/records. Off by
	// default; the values include whatever the tools take, such as payment
	// amounts and account numbers.
	ExposeArgs bool

	UpstreamTimeout time.Duration
}

func Load() *Config {
	return &Config{
		Port:            getEnv("PORT", "8090"),
		Environment:     getEnv("ENVIRONMENT", "development"),
		PolicyFile:      getEnv("POLICY_FILE", ""),
		UpstreamURL:     getEnv("UPSTREAM_URL", "http://localhost:8091"),
		UpstreamPrefix:  getEnv("UPSTREAM_PREFIX", "/tools"),
		Mode:            getEnv("MODE", "full"),
		NATSURL:         getEnv("NATS_URL", ""),
		OperatorToken:   getEnv("OPERATOR_TOKEN", ""),
		ExposeArgs:      getEnv("EXPOSE_ARGS", "") == "true",
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
