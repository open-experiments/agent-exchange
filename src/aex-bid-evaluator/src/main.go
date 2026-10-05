package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/config"
	"github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/httpapi"
	"github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/service"
	"github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/store"
	"github.com/parlakisik/agent-exchange/internal/telemetry"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	cfg := config.Load()

	// Setup structured logging with trace correlation
	logHandler := telemetry.TraceHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	logger := slog.New(logHandler)
	slog.SetDefault(logger)

	// Initialize OpenTelemetry tracing
	otlpEndpoint := os.Getenv("OTLP_ENDPOINT")
	tracerShutdown, err := telemetry.InitTracer(context.Background(), "aex-bid-evaluator", otlpEndpoint)
	if err != nil {
		slog.Error("failed to initialize tracer", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := tracerShutdown(context.Background()); err != nil {
			slog.Error("failed to shutdown tracer", "error", err)
		}
	}()

	// Initialize Prometheus metrics
	metricsHandler, err := telemetry.InitMeter("aex-bid-evaluator")
	if err != nil {
		slog.Error("failed to initialize metrics", "error", err)
		os.Exit(1)
	}

	var st store.EvaluationStore
	var mongoClient *mongo.Client
	if cfg.MongoURI != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		c, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoURI))
		if err != nil {
			slog.Error("failed to connect to mongodb", "error", err)
			os.Exit(1)
		}
		mongoClient = c
		ms := store.NewMongoEvaluationStore(c, cfg.MongoDatabase, cfg.MongoCollection)
		if err := ms.EnsureIndexes(ctx); err != nil {
			slog.Warn("mongo index creation failed", "error", err)
		}
		st = ms
		slog.Info("mongo enabled", "uri", cfg.MongoURI, "db", cfg.MongoDatabase, "collection", cfg.MongoCollection)
	} else {
		st = store.NewMemoryEvaluationStore()
		slog.Info("mongo disabled (set MONGO_URI to enable)")
	}

	svc, err := service.New(cfg.BidGatewayURL, cfg.TrustBrokerURL, cfg.CertAuthURL, cfg.WorkPublisherURL, st)
	if err != nil {
		slog.Error("failed to create service", "error", err)
		os.Exit(1)
	}
	if cfg.WorkPublisherURL == "" {
		slog.Info("work-publisher disabled (set WORK_PUBLISHER_URL to fetch work specs; requests must include budget.max_price)")
	}

	// Setup HTTP router with metrics endpoint
	mux := http.NewServeMux()
	mux.Handle("/", httpapi.NewRouter(svc))
	mux.Handle("GET /metrics", metricsHandler)

	// Wrap with OpenTelemetry tracing middleware
	handler := telemetry.HTTPMiddleware("aex-bid-evaluator")(mux)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	go func() {
		slog.Info("http server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	slog.Info("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	if mongoClient != nil {
		_ = mongoClient.Disconnect(shutdownCtx)
	}

	slog.Info("server stopped")
}
