package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/config"
	"github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/httpapi"
	"github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/service"
	"github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/store"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	cfg := config.Load()

	var st store.EvaluationStore
	var mongoClient *mongo.Client
	if cfg.MongoURI != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		c, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoURI))
		if err != nil {
			log.Fatal(err)
		}
		mongoClient = c
		ms := store.NewMongoEvaluationStore(c, cfg.MongoDatabase, cfg.MongoCollection)
		if err := ms.EnsureIndexes(ctx); err != nil {
			log.Printf("mongo index creation failed: %v", err)
		}
		st = ms
		log.Printf("mongo enabled uri=%s db=%s collection=%s", cfg.MongoURI, cfg.MongoDatabase, cfg.MongoCollection)
	} else {
		st = store.NewMemoryEvaluationStore()
		log.Printf("mongo disabled (set MONGO_URI to enable)")
	}

	svc, err := service.New(cfg.BidGatewayURL, cfg.TrustBrokerURL, st)
	if err != nil {
		log.Fatal(err)
	}

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      httpapi.NewRouter(svc),
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
