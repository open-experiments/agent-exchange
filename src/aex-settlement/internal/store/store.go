package store

import (
	"context"

	"github.com/parlakisik/agent-exchange/aex-settlement/internal/model"
)

// SettlementStore defines the interface for settlement persistence
type SettlementStore interface {
	// Executions
	SaveExecution(ctx context.Context, execution model.Execution) error
	GetExecution(ctx context.Context, executionID string) (model.Execution, error)
	ListExecutionsByTenant(ctx context.Context, tenantID string, limit int) ([]model.Execution, error)
	ListExecutionsByContract(ctx context.Context, contractID string) (model.Execution, error)

	// Ledger
	AppendLedgerEntry(ctx context.Context, entry model.LedgerEntry) error
	GetLedgerEntries(ctx context.Context, tenantID string, limit int) ([]model.LedgerEntry, error)

	// Balances
	GetBalance(ctx context.Context, tenantID string) (model.TenantBalance, error)
	UpdateBalance(ctx context.Context, balance model.TenantBalance) error

	// Transactions
	SaveTransaction(ctx context.Context, tx model.Transaction) error
	GetTransaction(ctx context.Context, txID string) (model.Transaction, error)
	ListTransactions(ctx context.Context, tenantID string, limit int) ([]model.Transaction, error)

	Close() error
}
