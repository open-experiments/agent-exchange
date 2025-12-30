package store

import (
	"context"
	"time"

	"github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoEvaluationStore struct {
	coll *mongo.Collection
}

func NewMongoEvaluationStore(client *mongo.Client, dbName string, collName string) *MongoEvaluationStore {
	return &MongoEvaluationStore{
		coll: client.Database(dbName).Collection(collName),
	}
}

func (s *MongoEvaluationStore) EnsureIndexes(ctx context.Context) error {
	_, err := s.coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "work_id", Value: 1}, {Key: "evaluated_at", Value: -1}},
	})
	return err
}

func (s *MongoEvaluationStore) Save(ctx context.Context, ev model.BidEvaluation) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.coll.InsertOne(ctx, ev)
	return err
}

func (s *MongoEvaluationStore) GetLatest(ctx context.Context, workID string) (*model.BidEvaluation, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	opts := options.FindOne().SetSort(bson.D{{Key: "evaluated_at", Value: -1}})
	res := s.coll.FindOne(ctx, bson.M{"work_id": workID}, opts)
	if res.Err() == mongo.ErrNoDocuments {
		return nil, nil
	}
	if res.Err() != nil {
		return nil, res.Err()
	}
	var ev model.BidEvaluation
	if err := res.Decode(&ev); err != nil {
		return nil, err
	}
	return &ev, nil
}



