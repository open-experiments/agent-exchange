package model

import (
	"time"
)

// Execution represents a completed work execution with pricing
type Execution struct {
	ID             string                 `json:"id" bson:"_id"`
	WorkID         string                 `json:"work_id" bson:"work_id"`
	ContractID     string                 `json:"contract_id" bson:"contract_id"`
	AgentID        string                 `json:"agent_id" bson:"agent_id"`
	ConsumerID     string                 `json:"consumer_id" bson:"consumer_id"`
	ProviderID     string                 `json:"provider_id" bson:"provider_id"`
	Domain         string                 `json:"domain" bson:"domain"`
	StartedAt      time.Time              `json:"started_at" bson:"started_at"`
	CompletedAt    time.Time              `json:"completed_at" bson:"completed_at"`
	DurationMs     int64                  `json:"duration_ms" bson:"duration_ms"`
	Status         string                 `json:"status" bson:"status"` // COMPLETED|FAILED
	Success        bool                   `json:"success" bson:"success"`
	AgreedPrice    string                 `json:"agreed_price" bson:"agreed_price"`       // Decimal as string
	PlatformFee    string                 `json:"platform_fee" bson:"platform_fee"`       // Decimal as string
	ProviderPayout string                 `json:"provider_payout" bson:"provider_payout"` // Decimal as string
	Metadata       map[string]interface{} `json:"metadata,omitempty" bson:"metadata,omitempty"`
	CreatedAt      time.Time              `json:"created_at" bson:"created_at"`
}

// LedgerEntry represents an immutable ledger entry
type LedgerEntry struct {
	ID            string    `json:"id" bson:"_id"`
	TenantID      string    `json:"tenant_id" bson:"tenant_id"`
	EntryType     string    `json:"entry_type" bson:"entry_type"` // DEBIT|CREDIT|DEPOSIT|WITHDRAWAL
	Amount        string    `json:"amount" bson:"amount"`         // Decimal as string
	BalanceAfter  string    `json:"balance_after" bson:"balance_after"`
	ReferenceType string    `json:"reference_type" bson:"reference_type"` // execution|deposit|withdrawal
	ReferenceID   string    `json:"reference_id,omitempty" bson:"reference_id,omitempty"`
	Description   string    `json:"description" bson:"description"`
	CreatedAt     time.Time `json:"created_at" bson:"created_at"`
}

// TenantBalance represents the current balance for a tenant
type TenantBalance struct {
	TenantID    string    `json:"tenant_id" bson:"_id"`
	Balance     string    `json:"balance" bson:"balance"` // Decimal as string
	Currency    string    `json:"currency" bson:"currency"`
	LastUpdated time.Time `json:"last_updated" bson:"last_updated"`
}

// Transaction represents a deposit or withdrawal
type Transaction struct {
	ID               string     `json:"id" bson:"_id"`
	TenantID         string     `json:"tenant_id" bson:"tenant_id"`
	Type             string     `json:"type" bson:"type"`     // DEPOSIT|WITHDRAWAL
	Amount           string     `json:"amount" bson:"amount"` // Decimal as string
	Status           string     `json:"status" bson:"status"` // PENDING|COMPLETED|FAILED
	PaymentMethod    string     `json:"payment_method,omitempty" bson:"payment_method,omitempty"`
	PaymentReference string     `json:"payment_reference,omitempty" bson:"payment_reference,omitempty"`
	CreatedAt        time.Time  `json:"created_at" bson:"created_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty" bson:"completed_at,omitempty"`
}

// UsageResponse represents usage data for a tenant
type UsageResponse struct {
	TenantID   string      `json:"tenant_id"`
	Period     string      `json:"period"`
	Executions []Execution `json:"executions"`
	TotalCost  string      `json:"total_cost"`
	Count      int         `json:"count"`
}

// BalanceResponse represents balance information
type BalanceResponse struct {
	TenantID string `json:"tenant_id"`
	Balance  string `json:"balance"`
	Currency string `json:"currency"`
}

// TransactionListResponse represents a list of transactions
type TransactionListResponse struct {
	Transactions []LedgerEntry `json:"transactions"`
	Count        int           `json:"count"`
}

// CostBreakdown represents the cost breakdown for a contract
type CostBreakdown struct {
	AgreedPrice    string `json:"agreed_price"`
	PlatformFee    string `json:"platform_fee"`
	ProviderPayout string `json:"provider_payout"`
}

// ContractCompletedEvent represents the event received when a contract is completed
type ContractCompletedEvent struct {
	ContractID  string                 `json:"contract_id"`
	WorkID      string                 `json:"work_id"`
	AgentID     string                 `json:"agent_id"`
	ConsumerID  string                 `json:"consumer_id"`
	ProviderID  string                 `json:"provider_id"`
	Domain      string                 `json:"domain"`
	StartedAt   time.Time              `json:"started_at"`
	CompletedAt time.Time              `json:"completed_at"`
	Success     bool                   `json:"success"`
	AgreedPrice string                 `json:"agreed_price"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}
