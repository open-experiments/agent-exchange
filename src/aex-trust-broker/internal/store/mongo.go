package store

import (
	"context"
	"time"

	"github.com/parlakisik/agent-exchange/aex-trust-broker/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoStore struct {
	trust    *mongo.Collection
	outcomes *mongo.Collection
}

func NewMongoStore(client *mongo.Client, dbName, trustColl, outcomesColl string) *MongoStore {
	db := client.Database(dbName)
	return &MongoStore{
		trust:    db.Collection(trustColl),
		outcomes: db.Collection(outcomesColl),
	}
}

func (s *MongoStore) EnsureIndexes(ctx context.Context) error {
	_, err := s.trust.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "provider_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}
	_, err = s.outcomes.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "provider_id", Value: 1}, {Key: "completed_at", Value: -1}},
	})
	return err
}

func (s *MongoStore) UpsertTrustRecord(ctx context.Context, rec model.TrustRecord) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.trust.ReplaceOne(ctx, bson.M{"provider_id": rec.ProviderID}, rec, options.Replace().SetUpsert(true))
	return err
}

func (s *MongoStore) GetTrustRecord(ctx context.Context, providerID string) (*model.TrustRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res := s.trust.FindOne(ctx, bson.M{"provider_id": providerID})
	if res.Err() == mongo.ErrNoDocuments {
		return nil, nil
	}
	if res.Err() != nil {
		return nil, res.Err()
	}
	var rec model.TrustRecord
	if err := res.Decode(&rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

func (s *MongoStore) SaveOutcome(ctx context.Context, out model.ContractOutcome) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.outcomes.InsertOne(ctx, out)
	return err
}

func (s *MongoStore) ListOutcomes(ctx context.Context, providerID string, limit int) ([]model.ContractOutcome, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	opts := options.Find().SetSort(bson.D{{Key: "completed_at", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	cur, err := s.outcomes.Find(ctx, bson.M{"provider_id": providerID}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []model.ContractOutcome
	for cur.Next(ctx) {
		var o model.ContractOutcome
		if err := cur.Decode(&o); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return out, nil
}



