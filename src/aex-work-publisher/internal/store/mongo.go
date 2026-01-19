package store

import (
	"context"
	"errors"
	"time"

	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoWorkStore struct {
	coll *mongo.Collection
}

func NewMongoWorkStore(client *mongo.Client, dbName string, collName string) *MongoWorkStore {
	return &MongoWorkStore{
		coll: client.Database(dbName).Collection(collName),
	}
}

func (s *MongoWorkStore) EnsureIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "consumer_id", Value: 1}, {Key: "created_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "state", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "category", Value: 1}},
		},
	}
	_, err := s.coll.Indexes().CreateMany(ctx, indexes)
	return err
}

func (s *MongoWorkStore) SaveWork(ctx context.Context, work model.WorkSpec) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.coll.InsertOne(ctx, work)
	return err
}

func (s *MongoWorkStore) GetWork(ctx context.Context, workID string) (model.WorkSpec, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var work model.WorkSpec
	err := s.coll.FindOne(ctx, bson.M{"id": workID}).Decode(&work)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return model.WorkSpec{}, errors.New("work not found")
		}
		return model.WorkSpec{}, err
	}
	return work, nil
}

func (s *MongoWorkStore) UpdateWork(ctx context.Context, work model.WorkSpec) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	result, err := s.coll.ReplaceOne(ctx, bson.M{"id": work.ID}, work)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return errors.New("work not found")
	}
	return nil
}

func (s *MongoWorkStore) ListWork(ctx context.Context, consumerID string, limit int) ([]model.WorkSpec, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}

	cur, err := s.coll.Find(ctx, bson.M{"consumer_id": consumerID}, opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()

	var works []model.WorkSpec
	for cur.Next(ctx) {
		var work model.WorkSpec
		if err := cur.Decode(&work); err != nil {
			return nil, err
		}
		works = append(works, work)
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return works, nil
}

func (s *MongoWorkStore) Close() error {
	// MongoDB client is shared, no need to close here
	return nil
}
