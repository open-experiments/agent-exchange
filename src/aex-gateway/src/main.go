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

	"github.com/parlakisik/agent-exchange/aex-gateway/internal/config"
	"github.com/parlakisik/agent-exchange/aex-gateway/internal/httpapi"
	"github.com/parlakisik/agent-exchange/internal/telemetry"
)

func main() {
	cfg := config.Load()

	// Refuse an empty, short or placeholder JWT_SECRET outside development:
	// with a guessable secret anyone can mint tokens for any tenant.
	warning, err := config.ValidateJWTSecret(cfg.Environment, cfg.JWTSecret)
	if err != nil {
		log.Fatalf("refusing to start: %v", err)
	}
	if warning != "" {
		log.Printf("WARNING: %s. Set a random JWT_SECRET of at least %d bytes for any shared deployment.", warning, config.MinJWTSecretLength)
	}

	// Setup structured logging with trace correlation
	logHandler := telemetry.TraceHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	logger := slog.New(logHandler)
	slog.SetDefault(logger)

	// Initialize OpenTelemetry tracing
	otlpEndpoint := os.Getenv("OTLP_ENDPOINT")
	tracerShutdown, err := telemetry.InitTracer(context.Background(), "aex-gateway", otlpEndpoint)
	if err != nil {
		log.Fatalf("failed to initialize tracer: %v", err)
	}
	defer func() {
		if err := tracerShutdown(context.Background()); err != nil {
			log.Printf("failed to shutdown tracer: %v", err)
		}
	}()

	// Initialize Prometheus metrics
	metricsHandler, err := telemetry.InitMeter("aex-gateway")
	if err != nil {
		log.Fatalf("failed to initialize metrics: %v", err)
	}

	// Setup HTTP router with metrics endpoint
	mux := http.NewServeMux()
	mux.Handle("/", httpapi.NewRouter(cfg))
	mux.Handle("GET /metrics", metricsHandler)

	// Wrap with OpenTelemetry tracing middleware
	handler := telemetry.HTTPMiddleware("aex-gateway")(mux)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: cfg.RequestTimeout + 5*time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("aex-gateway listening on :%s (env=%s)", cfg.Port, cfg.Environment)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
