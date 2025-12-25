package store

import (
	"context"
	"time"

	"github.com/parlakisik/agent-exchange/aex-bid-gateway/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoBidStore struct {
	coll *mongo.Collection
}

func NewMongoBidStore(client *mongo.Client, dbName string, collName string) *MongoBidStore {
	return &MongoBidStore{
		coll: client.Database(dbName).Collection(collName),
	}
}

func (s *MongoBidStore) EnsureIndexes(ctx context.Context) error {
	_, err := s.coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "work_id", Value: 1}, {Key: "received_at", Value: -1}},
	})
	return err
}

func (s *MongoBidStore) Save(ctx context.Context, bid model.BidPacket) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.coll.InsertOne(ctx, bid)
	return err
}

func (s *MongoBidStore) ListByWorkID(ctx context.Context, workID string) ([]model.BidPacket, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cur, err := s.coll.Find(ctx, bson.M{"work_id": workID}, options.Find().SetSort(bson.D{{Key: "received_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []model.BidPacket
	for cur.Next(ctx) {
		var b model.BidPacket
		if err := cur.Decode(&b); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

