package model

import (
	"time"
)

// Execution represents a completed work execution with pricing
type Execution struct {
	ID          string    `json:"id" bson:"_id"`
	WorkID      string    `json:"work_id" bson:"work_id"`
	ContractID  string    `json:"contract_id" bson:"contract_id"`
	AgentID     string    `json:"agent_id" bson:"agent_id"`
	ConsumerID  string    `json:"consumer_id" bson:"consumer_id"`
	ProviderID  string    `json:"provider_id" bson:"provider_id"`
	Domain      string    `json:"domain" bson:"domain"`
	StartedAt   time.Time `json:"started_at" bson:"started_at"`
	CompletedAt time.Time `json:"completed_at" bson:"completed_at"`
	DurationMs  int64     `json:"duration_ms" bson:"duration_ms"`
	Status      string    `json:"status" bson:"status"` // COMPLETED|FAILED
	Success     bool      `json:"success" bson:"success"`
	// SettlementStatus is PENDING until the consumer debit, provider credit and
	// their ledger entries are all applied, then SETTLED. Executions recorded
	// before this field existed have it empty and are treated as SETTLED.
	SettlementStatus string     `json:"settlement_status,omitempty" bson:"settlement_status,omitempty"`
	SettledAt        *time.Time `json:"settled_at,omitempty" bson:"settled_at,omitempty"`
	// PendingSince is when the execution entered PENDING. The resumer uses it
	// to find stranded settlements and to refuse ones too old to replay safely.
	PendingSince *time.Time `json:"pending_since,omitempty" bson:"pending_since,omitempty"`
	// Charged is false when no money moves for this execution (success=false).
	Charged        bool                   `json:"charged" bson:"charged"`
	AgreedPrice    string                 `json:"agreed_price" bson:"agreed_price"`       // Decimal as string
	PlatformFee    string                 `json:"platform_fee" bson:"platform_fee"`       // Decimal as string
	ProviderPayout string                 `json:"provider_payout" bson:"provider_payout"` // Decimal as string
	Metadata       map[string]interface{} `json:"metadata,omitempty" bson:"metadata,omitempty"`
	CreatedAt      time.Time              `json:"created_at" bson:"created_at"`

	// Inputs to the payment step, kept so a resumed settlement can run it
	// without the original event.
	Description            string `json:"description,omitempty" bson:"description,omitempty"`
	Currency               string `json:"currency,omitempty" bson:"currency,omitempty"`
	RequestedPaymentMethod string `json:"requested_payment_method,omitempty" bson:"requested_payment_method,omitempty"`
	// PaymentRecorded is true once payment provider selection and AP2 have
	// run and their outcome is stored, so a resumed settlement never redoes them.
	PaymentRecorded bool `json:"payment_recorded,omitempty" bson:"payment_recorded,omitempty"`

	// AP2 Payment fields
	AP2Enabled           bool   `json:"ap2_enabled,omitempty" bson:"ap2_enabled,omitempty"`
	PaymentMandateID     string `json:"payment_mandate_id,omitempty" bson:"payment_mandate_id,omitempty"`
	PaymentReceiptID     string `json:"payment_receipt_id,omitempty" bson:"payment_receipt_id,omitempty"`
	PaymentTransactionID string `json:"payment_transaction_id,omitempty" bson:"payment_transaction_id,omitempty"`
	PaymentMethod        string `json:"payment_method,omitempty" bson:"payment_method,omitempty"`

	// Payment Provider fields (for payment provider marketplace)
	PaymentProviderID   string `json:"payment_provider_id,omitempty" bson:"payment_provider_id,omitempty"`
	PaymentProviderName string `json:"payment_provider_name,omitempty" bson:"payment_provider_name,omitempty"`
	PaymentBaseFee      string `json:"payment_base_fee,omitempty" bson:"payment_base_fee,omitempty"`
	PaymentReward       string `json:"payment_reward,omitempty" bson:"payment_reward,omitempty"`
	PaymentNetCost      string `json:"payment_net_cost,omitempty" bson:"payment_net_cost,omitempty"` // Can be negative (cashback)
	WorkCategory        string `json:"work_category,omitempty" bson:"work_category,omitempty"`
}

// LedgerEntry represents an immutable ledger entry
type LedgerEntry struct {
	ID            string    `json:"id" bson:"_id"`
	TenantID      string    `json:"tenant_id" bson:"tenant_id"`
	EntryType     string    `json:"entry_type" bson:"entry_type"` // DEBIT|CREDIT|DEPOSIT|WITHDRAWAL
	Amount        int64     `json:"amount" bson:"amount"`         // Amount in cents
	BalanceAfter  int64     `json:"balance_after" bson:"balance_after"`
	ReferenceType string    `json:"reference_type" bson:"reference_type"` // execution|deposit|withdrawal
	ReferenceID   string    `json:"reference_id,omitempty" bson:"reference_id,omitempty"`
	Description   string    `json:"description" bson:"description"`
	CreatedAt     time.Time `json:"created_at" bson:"created_at"`
}

// Settlement statuses for Execution.SettlementStatus
const (
	SettlementPending = "PENDING"
	SettlementSettled = "SETTLED"
)

// IsSettled reports whether the execution needs no further settlement work.
func (e Execution) IsSettled() bool {
	return e.SettlementStatus == "" || e.SettlementStatus == SettlementSettled
}

// IsUnchargedFailure reports whether the execution records a success=false
// completion that moved no money. Only such an execution may later be
// upgraded to a charged settlement. Executions recorded before
// settlement_status existed always have status COMPLETED, so they never match.
func (e Execution) IsUnchargedFailure() bool {
	return e.Status == "FAILED" && !e.Success && !e.Charged && e.SettlementStatus == SettlementSettled
}

// PaymentDetails is the outcome of payment provider selection and AP2 for an
// execution. Its bson field names match Execution's.
type PaymentDetails struct {
	PaymentRecorded bool `bson:"payment_recorded"`

	AP2Enabled           bool   `bson:"ap2_enabled,omitempty"`
	PaymentMandateID     string `bson:"payment_mandate_id,omitempty"`
	PaymentReceiptID     string `bson:"payment_receipt_id,omitempty"`
	PaymentTransactionID string `bson:"payment_transaction_id,omitempty"`
	PaymentMethod        string `bson:"payment_method,omitempty"`

	PaymentProviderID   string `bson:"payment_provider_id,omitempty"`
	PaymentProviderName string `bson:"payment_provider_name,omitempty"`
	PaymentBaseFee      string `bson:"payment_base_fee,omitempty"`
	PaymentReward       string `bson:"payment_reward,omitempty"`
	PaymentNetCost      string `bson:"payment_net_cost,omitempty"`
}

// ApplyPayment copies p onto the execution.
func (e *Execution) ApplyPayment(p PaymentDetails) {
	e.PaymentRecorded = p.PaymentRecorded
	e.AP2Enabled = p.AP2Enabled
	e.PaymentMandateID = p.PaymentMandateID
	e.PaymentReceiptID = p.PaymentReceiptID
	e.PaymentTransactionID = p.PaymentTransactionID
	e.PaymentMethod = p.PaymentMethod
	e.PaymentProviderID = p.PaymentProviderID
	e.PaymentProviderName = p.PaymentProviderName
	e.PaymentBaseFee = p.PaymentBaseFee
	e.PaymentReward = p.PaymentReward
	e.PaymentNetCost = p.PaymentNetCost
}

// TenantBalance represents the current balance for a tenant
type TenantBalance struct {
	TenantID    string    `json:"tenant_id" bson:"_id"`
	Balance     int64     `json:"balance" bson:"balance"` // Balance in cents
	Currency    string    `json:"currency" bson:"currency"`
	LastUpdated time.Time `json:"last_updated" bson:"last_updated"`
	// SettlementOps records the settlement balance ops applied within the
	// store's retention window so a retried settlement never applies the same
	// op twice.
	SettlementOps []AppliedBalanceOp `json:"-" bson:"settlement_ops,omitempty"`
}

// AppliedBalanceOp is a settlement balance op already applied to a tenant.
type AppliedBalanceOp struct {
	OpID         string    `bson:"op_id"`
	BalanceAfter int64     `bson:"balance_after"`
	AppliedAt    time.Time `bson:"applied_at"`
}

// SettlementResult is the outcome of processing a contract.completed event.
type SettlementResult struct {
	Status      string `json:"status"` // settled|recorded
	ExecutionID string `json:"execution_id"`
	ContractID  string `json:"contract_id"`
	Charged     bool   `json:"charged"`
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
	TenantID     string `json:"tenant_id"`
	BalanceCents int64  `json:"balance_cents"`
	Balance      string `json:"balance"` // Formatted as decimal string for display (e.g. "12.50")
	Currency     string `json:"currency"`
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
	Description string                 `json:"description,omitempty"`
	StartedAt   time.Time              `json:"started_at"`
	CompletedAt time.Time              `json:"completed_at"`
	Success     bool                   `json:"success"`
	AgreedPrice string                 `json:"agreed_price"`
	Currency    string                 `json:"currency,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`

	// AP2 Payment options
	UseAP2        bool   `json:"use_ap2,omitempty"`
	PaymentMethod string `json:"payment_method,omitempty"`

	// Work category for payment provider selection
	WorkCategory string `json:"work_category,omitempty"` // "contracts", "compliance", "general"
}

// AP2PaymentResult contains the result of AP2 payment processing
type AP2PaymentResult struct {
	Success          bool   `json:"success"`
	PaymentMandateID string `json:"payment_mandate_id,omitempty"`
	ReceiptID        string `json:"receipt_id,omitempty"`
	TransactionID    string `json:"transaction_id,omitempty"`
	PaymentMethod    string `json:"payment_method,omitempty"`
	ErrorMessage     string `json:"error_message,omitempty"`
}

// PaymentProviderBid represents a bid from a payment provider
type PaymentProviderBid struct {
	ProviderID            string   `json:"provider_id"`
	ProviderName          string   `json:"provider_name"`
	BaseFeePercent        float64  `json:"base_fee_percent"`
	RewardPercent         float64  `json:"reward_percent"`
	NetFeePercent         float64  `json:"net_fee_percent"` // base_fee - reward (can be negative = cashback)
	ProcessingTimeSeconds int      `json:"processing_time_seconds"`
	SupportedMethods      []string `json:"supported_methods"`
	FraudProtection       string   `json:"fraud_protection"` // "none", "basic", "standard", "advanced"
}

// PaymentProviderSelection represents the selected payment provider and its bid
type PaymentProviderSelection struct {
	SelectedProvider PaymentProviderBid   `json:"selected_provider"`
	AllBids          []PaymentProviderBid `json:"all_bids"`
	WorkCategory     string               `json:"work_category"`
	SelectionReason  string               `json:"selection_reason"` // "lowest_fee", "fastest", "most_secure"
}

// PaymentBidRequest represents a request for payment provider bids
type PaymentBidRequest struct {
	Amount       float64 `json:"amount"`
	Currency     string  `json:"currency"`
	WorkCategory string  `json:"work_category"` // "contracts", "compliance", "general"
	ConsumerID   string  `json:"consumer_id"`
	ContractID   string  `json:"contract_id"`
}
