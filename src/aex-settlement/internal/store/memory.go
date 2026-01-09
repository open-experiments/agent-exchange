package store

import (
	"context"
	"fmt"
	"sync"

	"github.com/parlakisik/agent-exchange/aex-settlement/internal/model"
)

// MemoryStore implements SettlementStore using in-memory storage
type MemoryStore struct {
	mu           sync.RWMutex
	executions   map[string]model.Execution
	ledger       []model.LedgerEntry
	balances     map[string]model.TenantBalance
	transactions map[string]model.Transaction
}

// NewMemoryStore creates a new in-memory store
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		executions:   make(map[string]model.Execution),
		ledger:       make([]model.LedgerEntry, 0),
		balances:     make(map[string]model.TenantBalance),
		transactions: make(map[string]model.Transaction),
	}
}

func (s *MemoryStore) SaveExecution(ctx context.Context, execution model.Execution) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executions[execution.ID] = execution
	return nil
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

func (s *MemoryStore) ListExecutionsByContract(ctx context.Context, contractID string) (model.Execution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, exec := range s.executions {
		if exec.ContractID == contractID {
			return exec, nil
		}
	}
	return model.Execution{}, fmt.Errorf("execution not found for contract: %s", contractID)
}

func (s *MemoryStore) AppendLedgerEntry(ctx context.Context, entry model.LedgerEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
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
			Balance:  "0.00",
			Currency: "USD",
		}, nil
	}
	return balance, nil
}

func (s *MemoryStore) UpdateBalance(ctx context.Context, balance model.TenantBalance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.balances[balance.TenantID] = balance
	return nil
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
