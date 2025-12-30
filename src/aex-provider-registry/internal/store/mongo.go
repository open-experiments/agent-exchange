package store

import (
	"context"
	"time"

	"github.com/parlakisik/agent-exchange/aex-provider-registry/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoStore struct {
	providers *mongo.Collection
	subs      *mongo.Collection
}

func NewMongoStore(client *mongo.Client, dbName, providersColl, subsColl string) *MongoStore {
	db := client.Database(dbName)
	return &MongoStore{
		providers: db.Collection(providersColl),
		subs:      db.Collection(subsColl),
	}
}

func (s *MongoStore) EnsureIndexes(ctx context.Context) error {
	_, err := s.providers.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "provider_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}
	_, err = s.providers.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "api_key_hash", Value: 1}},
		Options: options.Index().SetSparse(true),
	})
	if err != nil {
		return err
	}
	_, err = s.subs.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "subscription_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	return err
}

func (s *MongoStore) CreateProvider(ctx context.Context, p model.Provider) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.providers.InsertOne(ctx, p)
	return err
}

func (s *MongoStore) GetProvider(ctx context.Context, providerID string) (*model.Provider, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res := s.providers.FindOne(ctx, bson.M{"provider_id": providerID})
	if res.Err() == mongo.ErrNoDocuments {
		return nil, nil
	}
	if res.Err() != nil {
		return nil, res.Err()
	}
	var p model.Provider
	if err := res.Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *MongoStore) GetProviderByAPIKeyHash(ctx context.Context, apiKeyHash string) (*model.Provider, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res := s.providers.FindOne(ctx, bson.M{"api_key_hash": apiKeyHash})
	if res.Err() == mongo.ErrNoDocuments {
		return nil, nil
	}
	if res.Err() != nil {
		return nil, res.Err()
	}
	var p model.Provider
	if err := res.Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *MongoStore) ListProviders(ctx context.Context, providerIDs []string) ([]model.Provider, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cur, err := s.providers.Find(ctx, bson.M{"provider_id": bson.M{"$in": providerIDs}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []model.Provider
	for cur.Next(ctx) {
		var p model.Provider
		if err := cur.Decode(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *MongoStore) CreateSubscription(ctx context.Context, sub model.Subscription) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.subs.InsertOne(ctx, sub)
	return err
}

func (s *MongoStore) ListSubscriptions(ctx context.Context) ([]model.Subscription, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cur, err := s.subs.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []model.Subscription
	for cur.Next(ctx) {
		var sub model.Subscription
		if err := cur.Decode(&sub); err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return out, nil
}


