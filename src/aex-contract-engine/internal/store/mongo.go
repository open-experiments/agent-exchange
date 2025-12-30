package store

import (
	"context"
	"time"

	"github.com/parlakisik/agent-exchange/aex-contract-engine/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoContractStore struct {
	coll *mongo.Collection
}

func NewMongoContractStore(client *mongo.Client, dbName string, collName string) *MongoContractStore {
	return &MongoContractStore{coll: client.Database(dbName).Collection(collName)}
}

func (s *MongoContractStore) EnsureIndexes(ctx context.Context) error {
	_, err := s.coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "contract_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	return err
}

func (s *MongoContractStore) Save(ctx context.Context, c model.Contract) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.coll.InsertOne(ctx, c)
	return err
}

func (s *MongoContractStore) Get(ctx context.Context, contractID string) (*model.Contract, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res := s.coll.FindOne(ctx, bson.M{"contract_id": contractID})
	if res.Err() == mongo.ErrNoDocuments {
		return nil, nil
	}
	if res.Err() != nil {
		return nil, res.Err()
	}
	var c model.Contract
	if err := res.Decode(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *MongoContractStore) Update(ctx context.Context, c model.Contract) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.coll.ReplaceOne(ctx, bson.M{"contract_id": c.ContractID}, c, options.Replace().SetUpsert(false))
	return err
}



