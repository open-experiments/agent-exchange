package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/parlakisik/agent-exchange/aex-telemetry/internal/config"
	"github.com/parlakisik/agent-exchange/aex-telemetry/internal/httpapi"
	"github.com/parlakisik/agent-exchange/aex-telemetry/internal/service"
	"github.com/parlakisik/agent-exchange/aex-telemetry/internal/store"
)

func main() {
	cfg := config.Load()

	// Initialize store
	memStore := store.NewMemoryStore(cfg.MaxLogEntries, cfg.MaxMetricItems)

	// Initialize service
	svc := service.New(memStore)

	// Initialize HTTP server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      httpapi.NewRouter(svc),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("aex-telemetry listening on :%s (env=%s)", cfg.Port, cfg.Environment)
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
