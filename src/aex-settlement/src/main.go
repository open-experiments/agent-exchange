package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/parlakisik/agent-exchange/aex-settlement/internal/config"
	"github.com/parlakisik/agent-exchange/aex-settlement/internal/httpapi"
	"github.com/parlakisik/agent-exchange/aex-settlement/internal/service"
	"github.com/parlakisik/agent-exchange/aex-settlement/internal/store"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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

	slog.Info("starting aex-settlement",
		"environment", cfg.Environment,
		"port", cfg.Port,
	)

	// Connect to MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOpts := options.Client().ApplyURI(cfg.MongoURI)
	mongoClient, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		slog.Error("failed to connect to mongodb", "error", err)
		os.Exit(1)
	}

	if err := mongoClient.Ping(ctx, nil); err != nil {
		slog.Error("failed to ping mongodb", "error", err)
		os.Exit(1)
	}

	// Initialize store
	settlementStore := store.NewMongoSettlementStore(mongoClient, cfg.MongoDB)
	if err := settlementStore.EnsureIndexes(ctx); err != nil {
		slog.Warn("failed to create indexes", "error", err)
	}

	slog.Info("using mongodb store", "uri", cfg.MongoURI, "db", cfg.MongoDB)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := mongoClient.Disconnect(ctx); err != nil {
			slog.Error("failed to disconnect mongodb", "error", err)
		}
	}()

	// Initialize service
	svc := service.New(settlementStore)

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

	// Graceful shutdown on SIGINT/SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server...")

	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}

