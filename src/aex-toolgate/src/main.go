// Command aex-toolgate is the tool-call gate as a service: an authorizing
// reverse proxy a provider runs in front of its tool endpoint. See
// docs/TOOLGATE.md and internal/httpapi for the API.
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

	"github.com/parlakisik/agent-exchange/aex-toolgate/internal/config"
	"github.com/parlakisik/agent-exchange/aex-toolgate/internal/httpapi"
	"github.com/parlakisik/agent-exchange/internal/events"
	aexnats "github.com/parlakisik/agent-exchange/internal/nats"
	"github.com/parlakisik/agent-exchange/internal/toolgate"
)

func main() {
	cfg := config.Load()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	policy, err := toolgate.LoadPolicy(cfg.PolicyFile)
	if err != nil {
		log.Fatal(err)
	}
	mode := toolgate.ModeFull
	if cfg.Mode == "scope" {
		mode = toolgate.ModeScope
	}
	opts := []toolgate.Option{toolgate.WithMode(mode)}

	var nc *aexnats.Client
	if cfg.NATSURL != "" {
		ncfg := aexnats.DefaultConfig()
		ncfg.URL = cfg.NATSURL
		ncfg.Name = "aex-toolgate"
		nc, err = aexnats.Connect(ncfg)
		if err != nil {
			log.Fatalf("nats: %v", err)
		}
		defer nc.Close()
		if err := nc.EnsureStreams(); err != nil {
			log.Fatalf("nats streams: %v", err)
		}
		opts = append(opts, toolgate.WithPublisher(events.NewPublisherWithNATS("aex-toolgate", nc)))
	}
	gate, err := toolgate.New(policy, opts...)
	if err != nil {
		log.Fatal(err)
	}

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      httpapi.New(gate, cfg.UpstreamURL, cfg.UpstreamPrefix, cfg.UpstreamTimeout),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: cfg.UpstreamTimeout + 5*time.Second,
		IdleTimeout:  60 * time.Second,
	}
	go func() {
		slog.Info("aex-toolgate listening", "port", cfg.Port, "env", cfg.Environment, "provider", policy.Provider,
			"mode", cfg.Mode, "rules", len(policy.Rules), "tools", len(policy.ToolScopes), "upstream", cfg.UpstreamURL, "nats", cfg.NATSURL != "")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
