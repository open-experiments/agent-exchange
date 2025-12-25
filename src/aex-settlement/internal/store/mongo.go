package store

import (
	"context"
	"errors"
	"time"

	"github.com/parlakisik/agent-exchange/aex-settlement/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoSettlementStore struct {
	executions   *mongo.Collection
	ledger       *mongo.Collection
	balances     *mongo.Collection
	transactions *mongo.Collection
}

func NewMongoSettlementStore(client *mongo.Client, dbName string) *MongoSettlementStore {
	db := client.Database(dbName)
	return &MongoSettlementStore{
		executions:   db.Collection("executions"),
		ledger:       db.Collection("ledger_entries"),
		balances:     db.Collection("tenant_balances"),
		transactions: db.Collection("transactions"),
	}
}

func (s *MongoSettlementStore) EnsureIndexes(ctx context.Context) error {
	// Executions indexes
	_, err := s.executions.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "consumer_id", Value: 1}, {Key: "created_at", Value: -1}}},
		{Keys: bson.D{{Key: "provider_id", Value: 1}, {Key: "created_at", Value: -1}}},
		{Keys: bson.D{{Key: "domain", Value: 1}, {Key: "created_at", Value: -1}}},
		{Keys: bson.D{{Key: "contract_id", Value: 1}}, Options: options.Index().SetUnique(true)},
	})
	if err != nil {
		return err
	}

	// Ledger indexes
	_, err = s.ledger.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "tenant_id", Value: 1}, {Key: "created_at", Value: -1}},
	})
	if err != nil {
		return err
	}

	// Transactions indexes
	_, err = s.transactions.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "tenant_id", Value: 1}, {Key: "created_at", Value: -1}},
	})

	return err
}

// Executions

func (s *MongoSettlementStore) SaveExecution(ctx context.Context, execution model.Execution) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.executions.InsertOne(ctx, execution)
	return err
}

func (s *MongoSettlementStore) GetExecution(ctx context.Context, executionID string) (model.Execution, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var exec model.Execution
	err := s.executions.FindOne(ctx, bson.M{"_id": executionID}).Decode(&exec)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return model.Execution{}, errors.New("execution not found")
		}
		return model.Execution{}, err
	}
	return exec, nil
}

func (s *MongoSettlementStore) ListExecutionsByTenant(ctx context.Context, tenantID string, limit int) ([]model.Execution, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}

	// Find executions where tenant is either consumer or provider
	filter := bson.M{
		"$or": []bson.M{
			{"consumer_id": tenantID},
			{"provider_id": tenantID},
		},
	}

	cur, err := s.executions.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var executions []model.Execution
	if err := cur.All(ctx, &executions); err != nil {
		return nil, err
	}

	return executions, nil
}

func (s *MongoSettlementStore) ListExecutionsByContract(ctx context.Context, contractID string) (model.Execution, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var exec model.Execution
	err := s.executions.FindOne(ctx, bson.M{"contract_id": contractID}).Decode(&exec)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return model.Execution{}, errors.New("execution not found")
		}
		return model.Execution{}, err
	}
	return exec, nil
}

// Ledger

func (s *MongoSettlementStore) AppendLedgerEntry(ctx context.Context, entry model.LedgerEntry) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.ledger.InsertOne(ctx, entry)
	return err
}

func (s *MongoSettlementStore) GetLedgerEntries(ctx context.Context, tenantID string, limit int) ([]model.LedgerEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}

	cur, err := s.ledger.Find(ctx, bson.M{"tenant_id": tenantID}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var entries []model.LedgerEntry
	if err := cur.All(ctx, &entries); err != nil {
		return nil, err
	}

	return entries, nil
}

// Balances

func (s *MongoSettlementStore) GetBalance(ctx context.Context, tenantID string) (model.TenantBalance, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var balance model.TenantBalance
	err := s.balances.FindOne(ctx, bson.M{"_id": tenantID}).Decode(&balance)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			// Return zero balance if not found
			return model.TenantBalance{
				TenantID:    tenantID,
				Balance:     "0",
				Currency:    "USD",
				LastUpdated: time.Now().UTC(),
			}, nil
		}
		return model.TenantBalance{}, err
	}
	return balance, nil
}

func (s *MongoSettlementStore) UpdateBalance(ctx context.Context, balance model.TenantBalance) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := s.balances.ReplaceOne(
		ctx,
		bson.M{"_id": balance.TenantID},
		balance,
		options.Replace().SetUpsert(true),
	)
	return err
}

// Transactions

func (s *MongoSettlementStore) SaveTransaction(ctx context.Context, tx model.Transaction) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.transactions.InsertOne(ctx, tx)
	return err
}

func (s *MongoSettlementStore) GetTransaction(ctx context.Context, txID string) (model.Transaction, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var tx model.Transaction
	err := s.transactions.FindOne(ctx, bson.M{"_id": txID}).Decode(&tx)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return model.Transaction{}, errors.New("transaction not found")
		}
		return model.Transaction{}, err
	}
	return tx, nil
}

func (s *MongoSettlementStore) ListTransactions(ctx context.Context, tenantID string, limit int) ([]model.Transaction, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}

	cur, err := s.transactions.Find(ctx, bson.M{"tenant_id": tenantID}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var txs []model.Transaction
	if err := cur.All(ctx, &txs); err != nil {
		return nil, err
	}

	return txs, nil
}

func (s *MongoSettlementStore) Close() error {
	// MongoDB client is shared, no need to close here
	return nil
}
