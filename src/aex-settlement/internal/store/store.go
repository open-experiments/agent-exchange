package store

import (
	"context"
	"errors"
	"time"

	"github.com/parlakisik/agent-exchange/aex-settlement/internal/model"
)

var (
	// ErrExecutionExists is returned by InsertExecution when an execution for
	// the same contract_id is already recorded. One contract settles at most once.
	ErrExecutionExists = errors.New("execution already recorded")
	// ErrExecutionNotFound is returned when no execution matches the lookup.
	ErrExecutionNotFound = errors.New("execution not found")
	// ErrLedgerEntryExists is returned by AppendLedgerEntry when an entry with
	// the same ID is already recorded.
	ErrLedgerEntryExists = errors.New("ledger entry already recorded")
	// ErrTenantNotFound is returned by ApplyBalanceOp when the tenant has no
	// balance account and the op may not create one.
	ErrTenantNotFound = errors.New("tenant balance account not found")
)

// DefaultBalanceOpRetention is how long a tenant balance document remembers
// an applied settlement op ID. An op is only replayed while its execution is
// PENDING, and the service refuses to replay an execution that has been
// PENDING for more than half the retention (the resumer drives executions to
// SETTLED within minutes), so a replayed op is always still remembered.
// Trimming by age rather than by count means a busy tenant can never push a
// pending execution's op out of the window.
const DefaultBalanceOpRetention = 30 * 24 * time.Hour

// BalanceOp is a balance change that is applied at most once per ID.
type BalanceOp struct {
	ID         string
	TenantID   string
	DeltaCents int64
	Currency   string
	// CreateIfMissing creates the tenant balance account when it does not
	// exist. When false, ApplyBalanceOp returns ErrTenantNotFound instead.
	CreateIfMissing bool
}

// SettlementStore defines the interface for settlement persistence
type SettlementStore interface {
	// Executions

	// InsertExecution records a new execution. It returns ErrExecutionExists
	// when an execution for execution.ContractID is already recorded.
	InsertExecution(ctx context.Context, execution model.Execution) error
	// ReplaceUnchargedFailure replaces an execution that records an uncharged
	// failure (model.Execution.IsUnchargedFailure) with execution, matched on
	// execution.ID. It returns ErrExecutionExists when the stored execution is
	// no longer an uncharged failure, e.g. because a concurrent request
	// already replaced it, and ErrExecutionNotFound when there is none.
	ReplaceUnchargedFailure(ctx context.Context, execution model.Execution) error
	// RecordExecutionPayment stores the payment step's outcome on the
	// execution unless one is already recorded. It returns the execution as
	// stored afterwards, so every caller sees the first recorded outcome.
	RecordExecutionPayment(ctx context.Context, executionID string, payment model.PaymentDetails) (model.Execution, error)
	// MarkExecutionSettled moves a PENDING execution to SETTLED. It reports
	// whether this call made the transition; false means the execution was
	// already SETTLED.
	MarkExecutionSettled(ctx context.Context, executionID string, settledAt time.Time) (bool, error)
	// ListPendingExecutions returns up to limit PENDING executions that have
	// been pending since a time in [pendingAfter, pendingBefore), oldest
	// first. A zero pendingAfter means no lower bound.
	ListPendingExecutions(ctx context.Context, pendingAfter, pendingBefore time.Time, limit int) ([]model.Execution, error)
	GetExecution(ctx context.Context, executionID string) (model.Execution, error)
	ListExecutionsByTenant(ctx context.Context, tenantID string, limit int) ([]model.Execution, error)
	// GetExecutionByContract returns the execution for contractID, or
	// ErrExecutionNotFound. Any other error means the lookup itself failed.
	GetExecutionByContract(ctx context.Context, contractID string) (model.Execution, error)

	// Ledger

	// AppendLedgerEntry records an entry. It returns ErrLedgerEntryExists when
	// an entry with the same ID is already recorded.
	AppendLedgerEntry(ctx context.Context, entry model.LedgerEntry) error
	GetLedgerEntries(ctx context.Context, tenantID string, limit int) ([]model.LedgerEntry, error)

	// Balances
	GetBalance(ctx context.Context, tenantID string) (model.TenantBalance, error)
	// TenantExists reports whether the tenant has a balance account.
	TenantExists(ctx context.Context, tenantID string) (bool, error)
	// IncrementBalance atomically increments (or decrements if negative) the balance
	// for the given tenant by deltaCents. It upserts the document if it does not exist.
	// Returns the updated TenantBalance after the increment.
	IncrementBalance(ctx context.Context, tenantID string, deltaCents int64, currency string) (model.TenantBalance, error)
	// ApplyBalanceOp atomically applies op to the tenant balance unless an op
	// with the same ID was applied within BalanceOpRetention. It returns the
	// balance right after the op was applied, whether by this call or an
	// earlier one.
	ApplyBalanceOp(ctx context.Context, op BalanceOp) (int64, error)
	// BalanceOpRetention is how long an applied op ID is remembered.
	BalanceOpRetention() time.Duration

	// Transactions
	SaveTransaction(ctx context.Context, tx model.Transaction) error
	GetTransaction(ctx context.Context, txID string) (model.Transaction, error)
	ListTransactions(ctx context.Context, tenantID string, limit int) ([]model.Transaction, error)

	// WithTransaction executes fn within a database transaction when the
	// backend supports one. All store operations using the returned context
	// participate in the transaction. The transaction is committed if fn
	// returns nil, rolled back otherwise.
	WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error

	Close() error
}
