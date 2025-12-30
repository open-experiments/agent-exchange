package integration

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// DatabaseCleaner handles cleanup of test data from MongoDB
type DatabaseCleaner struct {
	client *mongo.Client
	db     *mongo.Database
}

// NewDatabaseCleaner creates a new database cleaner
func NewDatabaseCleaner(mongoURI, dbName string) (*DatabaseCleaner, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Verify connection
	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	return &DatabaseCleaner{
		client: client,
		db:     client.Database(dbName),
	}, nil
}

// Close closes the database connection
func (d *DatabaseCleaner) Close(ctx context.Context) error {
	return d.client.Disconnect(ctx)
}

// CleanAll removes all documents from all collections
func (d *DatabaseCleaner) CleanAll(ctx context.Context) error {
	collections := []string{
		"work_specs",
		"bids",
		"contracts",
		"providers",
		"subscriptions",
		"tenants",
		"api_keys",
		"trust_records",
		"outcomes",
		"evaluations",
		"ledger",
		"balances",
		"transactions",
	}

	for _, coll := range collections {
		_, err := d.db.Collection(coll).DeleteMany(ctx, bson.M{})
		if err != nil {
			return fmt.Errorf("failed to clean collection %s: %w", coll, err)
		}
	}

	return nil
}

// CleanCollection removes all documents from a specific collection
func (d *DatabaseCleaner) CleanCollection(ctx context.Context, collection string) error {
	_, err := d.db.Collection(collection).DeleteMany(ctx, bson.M{})
	return err
}

// CleanTestData removes documents matching a test pattern
func (d *DatabaseCleaner) CleanTestData(ctx context.Context, pattern string) error {
	filter := bson.M{
		"$or": []bson.M{
			{"name": bson.M{"$regex": pattern}},
			{"description": bson.M{"$regex": pattern}},
			{"email": bson.M{"$regex": pattern}},
		},
	}

	collections := []string{
		"work_specs",
		"providers",
		"tenants",
	}

	for _, coll := range collections {
		_, err := d.db.Collection(coll).DeleteMany(ctx, filter)
		if err != nil {
			// Ignore errors for non-existent fields
			continue
		}
	}

	return nil
}

// CleanOlderThan removes documents older than a certain duration
func (d *DatabaseCleaner) CleanOlderThan(ctx context.Context, duration time.Duration) error {
	cutoff := time.Now().Add(-duration)
	filter := bson.M{
		"created_at": bson.M{"$lt": cutoff},
	}

	collections := []string{
		"work_specs",
		"bids",
		"contracts",
		"evaluations",
	}

	for _, coll := range collections {
		_, err := d.db.Collection(coll).DeleteMany(ctx, filter)
		if err != nil {
			continue
		}
	}

	return nil
}

