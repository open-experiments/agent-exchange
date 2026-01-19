package store

import (
	"context"
	"time"

	"github.com/parlakisik/agent-exchange/aex-identity/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoStore struct {
	tenants *mongo.Collection
	keys    *mongo.Collection
}

func NewMongoStore(client *mongo.Client, dbName, tenantsColl, keysColl string) *MongoStore {
	db := client.Database(dbName)
	return &MongoStore{
		tenants: db.Collection(tenantsColl),
		keys:    db.Collection(keysColl),
	}
}

func (s *MongoStore) EnsureIndexes(ctx context.Context) error {
	_, err := s.tenants.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}
	_, err = s.keys.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "id", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "tenant_id", Value: 1}}},
		{Keys: bson.D{{Key: "key_hash", Value: 1}}, Options: options.Index().SetUnique(true)},
	})
	return err
}

func (s *MongoStore) CreateTenant(ctx context.Context, t model.Tenant) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.tenants.InsertOne(ctx, t)
	return err
}

func (s *MongoStore) GetTenant(ctx context.Context, tenantID string) (*model.Tenant, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res := s.tenants.FindOne(ctx, bson.M{"id": tenantID})
	if res.Err() == mongo.ErrNoDocuments {
		return nil, nil
	}
	if res.Err() != nil {
		return nil, res.Err()
	}
	var t model.Tenant
	if err := res.Decode(&t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *MongoStore) UpdateTenant(ctx context.Context, t model.Tenant) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.tenants.ReplaceOne(ctx, bson.M{"id": t.ID}, t, options.Replace().SetUpsert(false))
	return err
}

func (s *MongoStore) CreateAPIKey(ctx context.Context, k model.APIKey) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.keys.InsertOne(ctx, k)
	return err
}

func (s *MongoStore) ListAPIKeys(ctx context.Context, tenantID string) ([]model.APIKey, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cur, err := s.keys.Find(ctx, bson.M{"tenant_id": tenantID})
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()

	var out []model.APIKey
	for cur.Next(ctx) {
		var k model.APIKey
		if err := cur.Decode(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *MongoStore) GetAPIKey(ctx context.Context, tenantID string, keyID string) (*model.APIKey, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res := s.keys.FindOne(ctx, bson.M{"tenant_id": tenantID, "id": keyID})
	if res.Err() == mongo.ErrNoDocuments {
		return nil, nil
	}
	if res.Err() != nil {
		return nil, res.Err()
	}
	var k model.APIKey
	if err := res.Decode(&k); err != nil {
		return nil, err
	}
	return &k, nil
}

func (s *MongoStore) UpdateAPIKey(ctx context.Context, k model.APIKey) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.keys.ReplaceOne(ctx, bson.M{"id": k.ID}, k, options.Replace().SetUpsert(false))
	return err
}

func (s *MongoStore) FindAPIKeyByHash(ctx context.Context, keyHash string) (*model.APIKey, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res := s.keys.FindOne(ctx, bson.M{"key_hash": keyHash})
	if res.Err() == mongo.ErrNoDocuments {
		return nil, nil
	}
	if res.Err() != nil {
		return nil, res.Err()
	}
	var k model.APIKey
	if err := res.Decode(&k); err != nil {
		return nil, err
	}
	return &k, nil
}
