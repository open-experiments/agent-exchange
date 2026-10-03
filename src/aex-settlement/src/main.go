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
	"github.com/parlakisik/agent-exchange/internal/events"
	aexnats "github.com/parlakisik/agent-exchange/internal/nats"
	"github.com/parlakisik/agent-exchange/internal/telemetry"
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

	// Setup structured logging with trace correlation
	logLevel := slog.LevelInfo
	if cfg.Environment == "development" {
		logLevel = slog.LevelDebug
	}
	logHandler := telemetry.TraceHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	logger := slog.New(logHandler)
	slog.SetDefault(logger)

	// Initialize OpenTelemetry tracing
	otlpEndpoint := os.Getenv("OTLP_ENDPOINT")
	tracerShutdown, err := telemetry.InitTracer(context.Background(), "aex-settlement", otlpEndpoint)
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
	metricsHandler, err := telemetry.InitMeter("aex-settlement")
	if err != nil {
		slog.Error("failed to initialize metrics", "error", err)
		os.Exit(1)
	}

	slog.Info("starting aex-settlement",
		"environment", cfg.Environment,
		"port", cfg.Port,
		"store_type", cfg.StoreType,
	)

	var settlementStore store.SettlementStore
	var mongoClient *mongo.Client

	if cfg.StoreType == "memory" {
		memoryStore := store.NewMemoryStore()
		memoryStore.SetBalanceOpRetention(cfg.BalanceOpRetention)
		settlementStore = memoryStore
		slog.Info("using in-memory store")
	} else {
		// Connect to MongoDB
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		clientOpts := options.Client().ApplyURI(cfg.MongoURI)
		var err error
		mongoClient, err = mongo.Connect(ctx, clientOpts)
		if err != nil {
			slog.Error("failed to connect to mongodb", "error", err)
			os.Exit(1)
		}

		if err := mongoClient.Ping(ctx, nil); err != nil {
			slog.Error("failed to ping mongodb", "error", err)
			os.Exit(1)
		}

		// Initialize store
		mongoStore := store.NewMongoSettlementStore(mongoClient, cfg.MongoDB)
		mongoStore.SetBalanceOpRetention(cfg.BalanceOpRetention)
		// The unique contract_id index is what makes settlement idempotent;
		// refuse to serve without it.
		if err := mongoStore.EnsureIndexes(ctx); err != nil {
			slog.Error("failed to create indexes", "error", err)
			os.Exit(1)
		}
		settlementStore = mongoStore
		slog.Info("using mongodb store", "uri", cfg.MongoURI, "db", cfg.MongoDB)
	}

	defer func() {
		if mongoClient != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := mongoClient.Disconnect(ctx); err != nil {
				slog.Error("failed to disconnect mongodb", "error", err)
			}
		}
	}()

	// Initialize NATS and event publisher
	publisher := events.NewPublisher("aex-settlement")
	if cfg.WebhookSecret != "" {
		publisher.WithWebhookSecret(cfg.WebhookSecret)
	}
	if cfg.NatsURL != "" {
		natsCfg := aexnats.DefaultConfig()
		natsCfg.URL = cfg.NatsURL
		natsCfg.Name = "aex-settlement"
		natsCfg.StreamReplicas = cfg.NatsStreamReplicas
		natsClient, natsErr := aexnats.Connect(natsCfg)
		if natsErr != nil {
			slog.Warn("failed to connect to NATS, events will be log-only", "error", natsErr)
		} else {
			if err := natsClient.EnsureStreams(); err != nil {
				slog.Warn("failed to ensure NATS streams", "error", err)
			}
			publisher.WithNATS(natsClient)
			slog.Info("NATS connected", "url", cfg.NatsURL, "replicas", cfg.NatsStreamReplicas)
			defer func() {
				if err := natsClient.Close(); err != nil {
					slog.Error("failed to close NATS", "error", err)
				}
			}()
		}
	}

	// Initialize service
	svc := service.New(settlementStore, publisher)

	// Resume settlements a request left PENDING (crash, store error,
	// timeout). Safe on every replica: all settlement steps are idempotent.
	resumer := service.NewResumer(svc, service.ResumerConfig{
		Interval: cfg.ResumeInterval,
		Grace:    cfg.ResumeGrace,
		AlertAge: cfg.PendingAlertAge,
	})
	resumerCtx, stopResumer := context.WithCancel(context.Background())
	resumerDone := make(chan struct{})
	go func() {
		defer close(resumerDone)
		resumer.Run(resumerCtx)
	}()

	// Setup HTTP router
	mux := http.NewServeMux()
	mux.Handle("/", httpapi.NewRouter(svc))
	mux.Handle("GET /metrics", metricsHandler)

	// Wrap with OpenTelemetry tracing middleware
	handler := telemetry.HTTPMiddleware("aex-settlement")(mux)

	// Create HTTP server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}

	stopResumer()
	<-resumerDone

	slog.Info("server stopped")
}
