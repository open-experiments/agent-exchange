package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/parlakisik/agent-exchange/aex-contract-engine/internal/config"
	"github.com/parlakisik/agent-exchange/aex-contract-engine/internal/httpapi"
	"github.com/parlakisik/agent-exchange/aex-contract-engine/internal/service"
	"github.com/parlakisik/agent-exchange/aex-contract-engine/internal/store"
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
	tracerShutdown, err := telemetry.InitTracer(context.Background(), "aex-contract-engine", otlpEndpoint)
	if err != nil {
		log.Fatalf("failed to initialize tracer: %v", err)
	}
	defer func() {
		if err := tracerShutdown(context.Background()); err != nil {
			log.Printf("failed to shutdown tracer: %v", err)
		}
	}()

	// Initialize Prometheus metrics
	metricsHandler, err := telemetry.InitMeter("aex-contract-engine")
	if err != nil {
		log.Fatalf("failed to initialize metrics: %v", err)
	}

	var st store.ContractStore
	var mongoClient *mongo.Client
	if cfg.MongoURI != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		c, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoURI))
		if err != nil {
			log.Fatal(err)
		}
		mongoClient = c
		ms := store.NewMongoContractStore(c, cfg.MongoDatabase, cfg.MongoCollection)
		if err := ms.EnsureIndexes(ctx); err != nil {
			log.Printf("mongo index creation failed: %v", err)
		}
		st = ms
		log.Printf("mongo enabled uri=%s db=%s collection=%s", cfg.MongoURI, cfg.MongoDatabase, cfg.MongoCollection)
	} else {
		st = store.NewMemoryContractStore()
		log.Printf("mongo disabled (set MONGO_URI to enable)")
	}

	svc, err := service.New(st, cfg.BidGatewayURL, cfg.WorkPublisherURL, cfg.SettlementURL)
	if err != nil {
		log.Fatal(err)
	}
	if cfg.WorkPublisherURL == "" {
		slog.Warn("WORK_PUBLISHER_URL not set; contracts will record an unknown consumer and will not be settled")
	} else {
		log.Printf("work publisher enabled url=%s", cfg.WorkPublisherURL)
	}
	if cfg.SettlementURL == "" {
		slog.Warn("SETTLEMENT_URL not set; completed contracts will not be sent to settlement")
	} else {
		log.Printf("settlement enabled url=%s", cfg.SettlementURL)
	}

	// Setup HTTP router with metrics endpoint
	mux := http.NewServeMux()
	mux.Handle("/", httpapi.NewRouter(svc))
	mux.Handle("GET /metrics", metricsHandler)

	// Wrap with OpenTelemetry tracing middleware
	handler := telemetry.HTTPMiddleware("aex-contract-engine")(mux)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	go func() {
		log.Printf("listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)

	// Settlement notifications run detached from requests and may still be
	// retrying; give them their full timeout rather than what is left of
	// shutdownCtx.
	settleCtx, settleCancel := context.WithTimeout(context.Background(), service.SettlementTimeout+5*time.Second)
	defer settleCancel()
	if err := svc.WaitForSettlements(settleCtx); err != nil {
		log.Printf("pending settlement notifications not finished before shutdown: %v", err)
	}
	if mongoClient != nil {
		// shutdownCtx may have expired while waiting on settlements.
		disconnectCtx, disconnectCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer disconnectCancel()
		_ = mongoClient.Disconnect(disconnectCtx)
	}
}
