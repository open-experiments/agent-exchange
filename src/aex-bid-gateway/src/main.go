package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/parlakisik/agent-exchange/aex-bid-gateway/internal/config"
	"github.com/parlakisik/agent-exchange/aex-bid-gateway/internal/httpapi"
	"github.com/parlakisik/agent-exchange/aex-bid-gateway/internal/service"
	"github.com/parlakisik/agent-exchange/aex-bid-gateway/internal/store"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	cfg := config.Load()

	var st store.BidStore
	var mongoClient *mongo.Client
	if cfg.MongoURI != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		c, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoURI))
		if err != nil {
			log.Fatal(err)
		}
		mongoClient = c
		ms := store.NewMongoBidStore(c, cfg.MongoDatabase, cfg.MongoCollection)
		if err := ms.EnsureIndexes(ctx); err != nil {
			log.Printf("mongo index creation failed: %v", err)
		}
		st = ms
		log.Printf("mongo enabled uri=%s db=%s collection=%s", cfg.MongoURI, cfg.MongoDatabase, cfg.MongoCollection)
	} else {
		st = store.NewMemoryBidStore()
		log.Printf("mongo disabled (set MONGO_URI to enable)")
	}

	var svc *service.Service
	if cfg.ProviderRegistryURL != "" {
		svc = service.NewWithProviderRegistry(st, cfg.ProviderRegistryURL)
		log.Printf("provider auth: validating via provider-registry at %s", cfg.ProviderRegistryURL)
	} else if len(cfg.ProviderAPIKeys) > 0 {
		svc = service.New(st, cfg.ProviderAPIKeys)
		log.Printf("provider auth: using %d static API keys", len(cfg.ProviderAPIKeys))
	} else {
		svc = service.New(st, map[string]string{})
		log.Printf("provider auth: WARNING - no auth configured, all bids will be rejected")
	}
	handler := httpapi.NewRouter(svc)

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
	if mongoClient != nil {
		_ = mongoClient.Disconnect(shutdownCtx)
	}
}
