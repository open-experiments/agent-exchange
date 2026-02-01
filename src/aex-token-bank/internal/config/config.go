package config

import (
	"os"
	"strings"
)

// Config holds the application configuration
type Config struct {
	Port               string
	Environment        string
	InitialTokens      float64
	AEXRegistryURL     string
	AEXRegisterEnabled bool
	AgentRegistryFile  string // Path to agent registry JSON file (Phase 7)
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8094"
	}

	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		env = "development"
	}

	aexRegistryURL := os.Getenv("AEX_REGISTRY_URL")
	aexRegisterEnabled := strings.ToLower(os.Getenv("AEX_REGISTER_ENABLED")) == "true"

	// Agent registry file for Phase 7 secure banking
	agentRegistryFile := os.Getenv("AGENT_REGISTRY_FILE")

	return &Config{
		Port:               port,
		Environment:        env,
		InitialTokens:      1000.0, // Default initial tokens for new wallets
		AEXRegistryURL:     aexRegistryURL,
		AEXRegisterEnabled: aexRegisterEnabled,
		AgentRegistryFile:  agentRegistryFile,
	}, nil
}
