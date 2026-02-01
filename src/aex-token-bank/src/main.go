package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/parlakisik/agent-exchange/aex-token-bank/internal/config"
	"github.com/parlakisik/agent-exchange/aex-token-bank/internal/httpapi"
	"github.com/parlakisik/agent-exchange/aex-token-bank/internal/model"
	"github.com/parlakisik/agent-exchange/aex-token-bank/internal/service"
	"github.com/parlakisik/agent-exchange/aex-token-bank/internal/store"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Setup structured logging
	logLevel := slog.LevelInfo
	if cfg.Environment == "development" {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	slog.Info("starting aex-token-bank",
		"environment", cfg.Environment,
		"port", cfg.Port,
		"default_tokens", cfg.InitialTokens,
		"agent_registry_file", cfg.AgentRegistryFile,
	)

	// Initialize in-memory store
	tokenStore := store.NewMemoryStore()
	slog.Info("using in-memory store")

	// Initialize service
	svc := service.New(tokenStore, cfg.InitialTokens)

	// Phase 7: Initialize from agent registry if configured
	if cfg.AgentRegistryFile != "" {
		if err := loadAndInitializeRegistry(svc, cfg.AgentRegistryFile); err != nil {
			slog.Error("failed to initialize from agent registry", "error", err)
			// Continue without registry - fall back to legacy mode
			slog.Warn("running in legacy mode (agents can self-register)")
		}
	} else {
		slog.Info("no agent registry configured, running in legacy mode")
	}

	// Setup HTTP router
	router := httpapi.NewRouter(svc)

	// Create HTTP server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		slog.Info("http server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "error", err)
			os.Exit(1)
		}
	}()

	// Register with AEX Provider Registry if enabled
	if cfg.AEXRegisterEnabled && cfg.AEXRegistryURL != "" {
		go registerWithAEX(cfg)
	}

	// Graceful shutdown on SIGINT/SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}

// registerWithAEX registers the Token Bank with AEX Provider Registry
func registerWithAEX(cfg *config.Config) {
	// Wait a bit for the server to start
	time.Sleep(2 * time.Second)

	// Determine our hostname
	hostname := os.Getenv("HOSTNAME")
	if hostname == "" {
		hostname = "aex-token-bank"
	}

	payload := map[string]interface{}{
		"name":        "AEX Token Bank",
		"description": "Token banking and AP2 payment processing service",
		"endpoint":    "http://" + hostname + ":" + cfg.Port,
		"capabilities": []string{
			"token_banking",
			"ap2_payments",
			"wallet_management",
		},
		"metadata": map[string]interface{}{
			"supported_methods": []string{"aex-token"},
			"ap2_enabled":       true,
			"token_type":        "AEX",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("failed to marshal AEX registration payload", "error", err)
		return
	}

	// Retry registration a few times
	for i := 0; i < 5; i++ {
		resp, err := http.Post(cfg.AEXRegistryURL+"/v1/providers", "application/json", bytes.NewBuffer(body))
		if err != nil {
			slog.Warn("failed to register with AEX", "attempt", i+1, "error", err)
			time.Sleep(3 * time.Second)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
			slog.Info("registered with AEX Provider Registry",
				"registry_url", cfg.AEXRegistryURL,
				"capabilities", []string{"token_banking", "ap2_payments", "wallet_management"},
			)
			return
		}

		slog.Warn("AEX registration failed",
			"attempt", i+1,
			"status", resp.StatusCode,
		)
		time.Sleep(3 * time.Second)
	}

	slog.Warn("could not register with AEX Provider Registry after retries, continuing anyway")
}

// loadAndInitializeRegistry loads the agent registry JSON and initializes the token bank
func loadAndInitializeRegistry(svc *service.TokenService, filePath string) error {
	slog.Info("loading agent registry", "file", filePath)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	var registry model.AgentRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return err
	}

	slog.Info("agent registry loaded",
		"treasury_supply", registry.Treasury.TotalSupply,
		"num_agents", len(registry.Agents),
	)

	return svc.InitializeFromRegistry(&registry)
}
