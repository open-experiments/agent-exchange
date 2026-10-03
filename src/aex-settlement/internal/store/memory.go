package store

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/parlakisik/agent-exchange/aex-settlement/internal/model"
)

// MemoryStore implements SettlementStore using in-memory storage.
// It enforces the same uniqueness rules as the MongoDB store: one execution
// per contract_id, unique ledger entry IDs and at-most-once balance ops.
type MemoryStore struct {
	mu                  sync.RWMutex
	executions          map[string]model.Execution
	executionByContract map[string]string // contract_id -> execution ID
	ledger              []model.LedgerEntry
	ledgerIDs           map[string]struct{}
	balances            map[string]model.TenantBalance
	transactions        map[string]model.Transaction
	opRetention         time.Duration
}

// NewMemoryStore creates a new in-memory store
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		executions:          make(map[string]model.Execution),
		executionByContract: make(map[string]string),
		ledger:              make([]model.LedgerEntry, 0),
		ledgerIDs:           make(map[string]struct{}),
		balances:            make(map[string]model.TenantBalance),
		transactions:        make(map[string]model.Transaction),
		opRetention:         DefaultBalanceOpRetention,
	}
}

// SetBalanceOpRetention sets how long applied settlement op IDs are kept.
func (s *MemoryStore) SetBalanceOpRetention(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.opRetention = d
}

func (s *MemoryStore) BalanceOpRetention() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.opRetention
}

func (s *MemoryStore) InsertExecution(ctx context.Context, execution model.Execution) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.executionByContract[execution.ContractID]; ok {
		return ErrExecutionExists
	}
	if _, ok := s.executions[execution.ID]; ok {
		return ErrExecutionExists
	}
	s.executions[execution.ID] = execution
	s.executionByContract[execution.ContractID] = execution.ID
	return nil
}

func (s *MemoryStore) ReplaceUnchargedFailure(ctx context.Context, execution model.Execution) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.executions[execution.ID]
	if !ok {
		return ErrExecutionNotFound
	}
	if !current.IsUnchargedFailure() || current.ContractID != execution.ContractID {
		return ErrExecutionExists
	}
	s.executions[execution.ID] = execution
	return nil
}

func (s *MemoryStore) RecordExecutionPayment(ctx context.Context, executionID string, payment model.PaymentDetails) (model.Execution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	exec, ok := s.executions[executionID]
	if !ok {
		return model.Execution{}, ErrExecutionNotFound
	}
	if !exec.PaymentRecorded {
		payment.PaymentRecorded = true
		exec.ApplyPayment(payment)
		s.executions[executionID] = exec
	}
	return exec, nil
}

func (s *MemoryStore) MarkExecutionSettled(ctx context.Context, executionID string, settledAt time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	exec, ok := s.executions[executionID]
	if !ok {
		return false, ErrExecutionNotFound
	}
	if exec.SettlementStatus != model.SettlementPending {
		return false, nil
	}
	exec.SettlementStatus = model.SettlementSettled
	exec.SettledAt = &settledAt
	s.executions[executionID] = exec
	return true, nil
}

func (s *MemoryStore) ListPendingExecutions(ctx context.Context, pendingAfter, pendingBefore time.Time, limit int) ([]model.Execution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []model.Execution
	for _, exec := range s.executions {
		if exec.SettlementStatus == model.SettlementPending && exec.PendingSince != nil &&
			exec.PendingSince.Before(pendingBefore) && !exec.PendingSince.Before(pendingAfter) {
			result = append(result, exec)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PendingSince.Before(*result[j].PendingSince) })
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *MemoryStore) GetExecution(ctx context.Context, executionID string) (model.Execution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	exec, ok := s.executions[executionID]
	if !ok {
		return model.Execution{}, fmt.Errorf("execution not found: %s", executionID)
	}
	return exec, nil
}

func (s *MemoryStore) ListExecutionsByTenant(ctx context.Context, tenantID string, limit int) ([]model.Execution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []model.Execution
	for _, exec := range s.executions {
		if exec.ConsumerID == tenantID || exec.ProviderID == tenantID {
			result = append(result, exec)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (s *MemoryStore) GetExecutionByContract(ctx context.Context, contractID string) (model.Execution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.executionByContract[contractID]
	if !ok {
		return model.Execution{}, ErrExecutionNotFound
	}
	return s.executions[id], nil
}

func (s *MemoryStore) AppendLedgerEntry(ctx context.Context, entry model.LedgerEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.ledgerIDs[entry.ID]; ok {
		return ErrLedgerEntryExists
	}
	s.ledgerIDs[entry.ID] = struct{}{}
	s.ledger = append(s.ledger, entry)
	return nil
}

func (s *MemoryStore) GetLedgerEntries(ctx context.Context, tenantID string, limit int) ([]model.LedgerEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []model.LedgerEntry
	for i := len(s.ledger) - 1; i >= 0; i-- {
		if s.ledger[i].TenantID == tenantID {
			result = append(result, s.ledger[i])
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (s *MemoryStore) GetBalance(ctx context.Context, tenantID string) (model.TenantBalance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	balance, ok := s.balances[tenantID]
	if !ok {
		return model.TenantBalance{
			TenantID: tenantID,
			Balance:  0,
			Currency: "USD",
		}, nil
	}
	return balance, nil
}

func (s *MemoryStore) TenantExists(ctx context.Context, tenantID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.balances[tenantID]
	return ok, nil
}

func (s *MemoryStore) ApplyBalanceOp(ctx context.Context, op BalanceOp) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bal, ok := s.balances[op.TenantID]
	if !ok {
		if !op.CreateIfMissing {
			return 0, ErrTenantNotFound
		}
		bal = model.TenantBalance{TenantID: op.TenantID}
	}
	for _, applied := range bal.SettlementOps {
		if applied.OpID == op.ID {
			return applied.BalanceAfter, nil
		}
	}
	now := time.Now().UTC()
	bal.Balance += op.DeltaCents
	bal.Currency = op.Currency
	bal.LastUpdated = now
	cutoff := now.Add(-s.opRetention)
	ops := make([]model.AppliedBalanceOp, 0, len(bal.SettlementOps)+1)
	for _, applied := range bal.SettlementOps {
		if !applied.AppliedAt.Before(cutoff) {
			ops = append(ops, applied)
		}
	}
	ops = append(ops, model.AppliedBalanceOp{OpID: op.ID, BalanceAfter: bal.Balance, AppliedAt: now})
	bal.SettlementOps = ops
	s.balances[op.TenantID] = bal
	return bal.Balance, nil
}

func (s *MemoryStore) IncrementBalance(ctx context.Context, tenantID string, deltaCents int64, currency string) (model.TenantBalance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bal, ok := s.balances[tenantID]
	if !ok {
		bal = model.TenantBalance{
			TenantID: tenantID,
			Balance:  0,
			Currency: currency,
		}
	}
	bal.Balance += deltaCents
	bal.Currency = currency
	s.balances[tenantID] = bal
	return bal, nil
}

// WithTransaction executes fn directly for the in-memory store.
// The in-memory store uses a mutex for synchronization, so there is no
// multi-document transaction concept, but this satisfies the interface.
func (s *MemoryStore) WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func (s *MemoryStore) SaveTransaction(ctx context.Context, tx model.Transaction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transactions[tx.ID] = tx
	return nil
}

func (s *MemoryStore) GetTransaction(ctx context.Context, txID string) (model.Transaction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tx, ok := s.transactions[txID]
	if !ok {
		return model.Transaction{}, fmt.Errorf("transaction not found: %s", txID)
	}
	return tx, nil
}

func (s *MemoryStore) ListTransactions(ctx context.Context, tenantID string, limit int) ([]model.Transaction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []model.Transaction
	for _, tx := range s.transactions {
		if tx.TenantID == tenantID {
			result = append(result, tx)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (s *MemoryStore) Close() error {
	return nil
}
