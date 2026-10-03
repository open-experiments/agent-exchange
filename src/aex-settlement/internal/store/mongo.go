package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/parlakisik/agent-exchange/aex-settlement/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoSettlementStore struct {
	client       *mongo.Client
	executions   *mongo.Collection
	ledger       *mongo.Collection
	balances     *mongo.Collection
	transactions *mongo.Collection

	// Multi-document transactions need a replica set or mongos; the local and
	// demo compose stacks run a standalone mongod. Detected on first use.
	txMu        sync.Mutex
	txChecked   bool
	txSupported bool

	opRetention time.Duration
}

func NewMongoSettlementStore(client *mongo.Client, dbName string) *MongoSettlementStore {
	db := client.Database(dbName)
	return &MongoSettlementStore{
		client:       client,
		executions:   db.Collection("executions"),
		ledger:       db.Collection("ledger_entries"),
		balances:     db.Collection("tenant_balances"),
		transactions: db.Collection("transactions"),
		opRetention:  DefaultBalanceOpRetention,
	}
}

// SetBalanceOpRetention sets how long applied settlement op IDs are kept.
// Call it before the store is used.
func (s *MongoSettlementStore) SetBalanceOpRetention(d time.Duration) {
	s.opRetention = d
}

func (s *MongoSettlementStore) BalanceOpRetention() time.Duration {
	return s.opRetention
}

// EnsureIndexes creates the store's indexes. The unique contract_id index on
// executions is what guarantees one settlement per contract, so callers must
// treat a failure here as fatal.
func (s *MongoSettlementStore) EnsureIndexes(ctx context.Context) error {
	// Executions indexes
	_, err := s.executions.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "consumer_id", Value: 1}, {Key: "created_at", Value: -1}}},
		{Keys: bson.D{{Key: "provider_id", Value: 1}, {Key: "created_at", Value: -1}}},
		{Keys: bson.D{{Key: "domain", Value: 1}, {Key: "created_at", Value: -1}}},
		{Keys: bson.D{{Key: "contract_id", Value: 1}}, Options: options.Index().SetUnique(true)},
		// Partial: only PENDING executions are indexed, so the resumer's
		// periodic scan stays cheap however many executions are SETTLED.
		{
			Keys: bson.D{{Key: "pending_since", Value: 1}},
			Options: options.Index().SetPartialFilterExpression(
				bson.M{"settlement_status": model.SettlementPending},
			),
		},
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

// InsertExecution inserts a new execution. The unique contract_id index turns
// a second insert for the same contract into ErrExecutionExists.
func (s *MongoSettlementStore) InsertExecution(ctx context.Context, execution model.Execution) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.executions.InsertOne(ctx, execution)
	if mongo.IsDuplicateKeyError(err) {
		return ErrExecutionExists
	}
	return err
}

// ReplaceUnchargedFailure replaces the execution only while the stored
// document still records an uncharged failure, so of several concurrent
// upgrades exactly one succeeds.
func (s *MongoSettlementStore) ReplaceUnchargedFailure(ctx context.Context, execution model.Execution) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	filter := bson.M{
		"_id":               execution.ID,
		"contract_id":       execution.ContractID,
		"status":            "FAILED",
		"success":           false,
		"charged":           bson.M{"$ne": true},
		"settlement_status": model.SettlementSettled,
	}
	res, err := s.executions.ReplaceOne(ctx, filter, execution)
	if err != nil {
		return err
	}
	if res.MatchedCount == 1 {
		return nil
	}
	return s.executionExistsErr(ctx, execution.ID)
}

// executionExistsErr returns ErrExecutionExists when executionID is stored,
// ErrExecutionNotFound otherwise.
func (s *MongoSettlementStore) executionExistsErr(ctx context.Context, executionID string) error {
	n, err := s.executions.CountDocuments(ctx, bson.M{"_id": executionID}, options.Count().SetLimit(1))
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrExecutionNotFound
	}
	return ErrExecutionExists
}

// RecordExecutionPayment sets the payment fields only while payment_recorded
// is unset, then returns the stored execution, whichever caller recorded it.
func (s *MongoSettlementStore) RecordExecutionPayment(ctx context.Context, executionID string, payment model.PaymentDetails) (model.Execution, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	payment.PaymentRecorded = true
	var exec model.Execution
	err := s.executions.FindOneAndUpdate(ctx,
		bson.M{"_id": executionID, "payment_recorded": bson.M{"$ne": true}},
		bson.M{"$set": payment},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&exec)
	if err == nil {
		return exec, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return model.Execution{}, err
	}
	err = s.executions.FindOne(ctx, bson.M{"_id": executionID}).Decode(&exec)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.Execution{}, ErrExecutionNotFound
	}
	if err != nil {
		return model.Execution{}, err
	}
	return exec, nil
}

func (s *MongoSettlementStore) MarkExecutionSettled(ctx context.Context, executionID string, settledAt time.Time) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := s.executions.UpdateOne(ctx,
		bson.M{"_id": executionID, "settlement_status": model.SettlementPending},
		bson.M{"$set": bson.M{"settlement_status": model.SettlementSettled, "settled_at": settledAt}},
	)
	if err != nil {
		return false, err
	}
	if res.MatchedCount == 1 {
		return true, nil
	}
	if err := s.executionExistsErr(ctx, executionID); !errors.Is(err, ErrExecutionExists) {
		return false, err
	}
	return false, nil
}

func (s *MongoSettlementStore) ListPendingExecutions(ctx context.Context, pendingAfter, pendingBefore time.Time, limit int) ([]model.Execution, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "pending_since", Value: 1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	since := bson.M{"$lt": pendingBefore}
	if !pendingAfter.IsZero() {
		since["$gte"] = pendingAfter
	}
	filter := bson.M{
		"settlement_status": model.SettlementPending,
		"pending_since":     since,
	}
	cur, err := s.executions.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()

	var executions []model.Execution
	if err := cur.All(ctx, &executions); err != nil {
		return nil, err
	}
	return executions, nil
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
	defer func() { _ = cur.Close(ctx) }()

	var executions []model.Execution
	if err := cur.All(ctx, &executions); err != nil {
		return nil, err
	}

	return executions, nil
}

func (s *MongoSettlementStore) GetExecutionByContract(ctx context.Context, contractID string) (model.Execution, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var exec model.Execution
	err := s.executions.FindOne(ctx, bson.M{"contract_id": contractID}).Decode(&exec)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return model.Execution{}, ErrExecutionNotFound
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
	if mongo.IsDuplicateKeyError(err) {
		return ErrLedgerEntryExists
	}
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
	defer func() { _ = cur.Close(ctx) }()

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
				Balance:     0,
				Currency:    "USD",
				LastUpdated: time.Now().UTC(),
			}, nil
		}
		return model.TenantBalance{}, err
	}
	return balance, nil
}

func (s *MongoSettlementStore) TenantExists(ctx context.Context, tenantID string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	n, err := s.balances.CountDocuments(ctx, bson.M{"_id": tenantID}, options.Count().SetLimit(1))
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// IncrementBalance atomically increments the balance for a tenant using MongoDB $inc.
// This avoids the read-modify-write race condition. Upserts if the document does not exist.
// Returns the updated TenantBalance after the increment.
func (s *MongoSettlementStore) IncrementBalance(ctx context.Context, tenantID string, deltaCents int64, currency string) (model.TenantBalance, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	now := time.Now().UTC()
	filter := bson.M{"_id": tenantID}
	update := bson.M{
		"$inc": bson.M{"balance": deltaCents},
		"$set": bson.M{
			"currency":     currency,
			"last_updated": now,
		},
		"$setOnInsert": bson.M{"_id": tenantID},
	}
	opts := options.FindOneAndUpdate().
		SetUpsert(true).
		SetReturnDocument(options.After)

	var updated model.TenantBalance
	err := s.balances.FindOneAndUpdate(ctx, filter, update, opts).Decode(&updated)
	if err != nil {
		return model.TenantBalance{}, fmt.Errorf("increment balance: %w", err)
	}
	return updated, nil
}

// ApplyBalanceOp applies op with a single-document update that both changes
// the balance and records op.ID in settlement_ops, filtered on op.ID not being
// recorded yet. Re-applying an op is therefore a no-op that returns the
// balance recorded when it was first applied. It needs no transaction. The
// same update drops recorded ops older than the retention window.
func (s *MongoSettlementStore) ApplyBalanceOp(ctx context.Context, op BalanceOp) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// An upsert that races with another op creating the same tenant fails with
	// a duplicate key; the retry then finds the document.
	const attempts = 3
	for i := 0; i < attempts; i++ {
		after, done, err := s.tryApplyBalanceOp(ctx, op)
		if err != nil {
			return 0, err
		}
		if done {
			return after, nil
		}
	}
	return 0, fmt.Errorf("apply balance op %s: tenant %s changed concurrently", op.ID, op.TenantID)
}

func (s *MongoSettlementStore) tryApplyBalanceOp(ctx context.Context, op BalanceOp) (int64, bool, error) {
	now := time.Now().UTC()
	cutoff := now.Add(-s.opRetention)
	balance := bson.M{"$ifNull": bson.A{"$balance", int64(0)}}
	newBalance := bson.M{"$add": bson.A{balance, op.DeltaCents}}
	applied := bson.M{
		"op_id":         bson.M{"$literal": op.ID},
		"balance_after": newBalance,
		"applied_at":    now,
	}
	// Pipeline update: every expression reads the document as it was before
	// this update, so balance_after equals the new balance.
	update := mongo.Pipeline{
		{{Key: "$set", Value: bson.D{
			{Key: "balance", Value: newBalance},
			{Key: "currency", Value: bson.M{"$literal": op.Currency}},
			{Key: "last_updated", Value: now},
			{Key: "settlement_ops", Value: bson.M{"$concatArrays": bson.A{
				bson.M{"$filter": bson.M{
					"input": bson.M{"$ifNull": bson.A{"$settlement_ops", bson.A{}}},
					"as":    "op",
					"cond":  bson.M{"$gte": bson.A{"$$op.applied_at", cutoff}},
				}},
				bson.A{applied},
			}}},
		}}},
	}
	filter := bson.M{"_id": op.TenantID, "settlement_ops.op_id": bson.M{"$ne": op.ID}}
	opts := options.FindOneAndUpdate().
		SetUpsert(op.CreateIfMissing).
		SetReturnDocument(options.After)

	var updated model.TenantBalance
	err := s.balances.FindOneAndUpdate(ctx, filter, update, opts).Decode(&updated)
	if err == nil {
		return updated.Balance, true, nil
	}
	// No match (or, for an upsert, a duplicate _id): the tenant is missing,
	// the op was already applied, or another op created the tenant first.
	if !errors.Is(err, mongo.ErrNoDocuments) && !mongo.IsDuplicateKeyError(err) {
		return 0, false, fmt.Errorf("apply balance op %s: %w", op.ID, err)
	}

	var current model.TenantBalance
	err = s.balances.FindOne(ctx, bson.M{"_id": op.TenantID}).Decode(&current)
	if errors.Is(err, mongo.ErrNoDocuments) {
		if !op.CreateIfMissing {
			return 0, false, ErrTenantNotFound
		}
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read balance for op %s: %w", op.ID, err)
	}
	for _, a := range current.SettlementOps {
		if a.OpID == op.ID {
			return a.BalanceAfter, true, nil
		}
	}
	return 0, false, nil
}

// transactionsSupported reports whether the deployment is a replica set or a
// sharded cluster. Standalone mongod rejects transactions outright.
func (s *MongoSettlementStore) transactionsSupported(ctx context.Context) (bool, error) {
	s.txMu.Lock()
	defer s.txMu.Unlock()
	if s.txChecked {
		return s.txSupported, nil
	}

	var hello struct {
		SetName string `bson:"setName"`
		Msg     string `bson:"msg"`
	}
	if err := s.client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		return false, fmt.Errorf("detect mongodb topology: %w", err)
	}
	s.txSupported = hello.SetName != "" || hello.Msg == "isdbgrid"
	s.txChecked = true
	if !s.txSupported {
		slog.Warn("mongodb is standalone: multi-document transactions unavailable, store operations run without one")
	}
	return s.txSupported, nil
}

// WithTransaction executes fn within a MongoDB session transaction.
// All store operations using the returned context participate in the transaction.
// On a standalone mongod, which cannot run transactions, fn runs directly.
func (s *MongoSettlementStore) WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	supported, err := s.transactionsSupported(ctx)
	if err != nil {
		return err
	}
	if !supported {
		return fn(ctx)
	}

	session, err := s.client.StartSession()
	if err != nil {
		return fmt.Errorf("start session: %w", err)
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (interface{}, error) {
		return nil, fn(sessCtx)
	})
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
	defer func() { _ = cur.Close(ctx) }()

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
