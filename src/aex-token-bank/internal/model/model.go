package model

import (
	"time"
)

// Wallet represents an agent's token wallet
type Wallet struct {
	ID            string    `json:"id"`
	AgentID       string    `json:"agent_id"`
	AgentName     string    `json:"agent_name"`
	Balance       float64   `json:"balance"`
	TokenType     string    `json:"token_type"` // "AEX"
	TokenHash     string    `json:"-"`          // SHA256 hash of auth token (not serialized)
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Transaction represents a token transfer between wallets
type Transaction struct {
	ID          string    `json:"id"`
	FromWallet  string    `json:"from_wallet"`
	ToWallet    string    `json:"to_wallet"`
	Amount      float64   `json:"amount"`
	TokenType   string    `json:"token_type"`
	Reference   string    `json:"reference"`   // contract_id, etc.
	Description string    `json:"description"`
	Status      string    `json:"status"` // pending, completed, failed
	CreatedAt   time.Time `json:"created_at"`
}

// TransactionType represents the type of transaction
type TransactionType string

const (
	TransactionTypeDeposit  TransactionType = "DEPOSIT"
	TransactionTypeWithdraw TransactionType = "WITHDRAW"
	TransactionTypeTransfer TransactionType = "TRANSFER"
)

// TransactionStatus represents the status of a transaction
type TransactionStatus string

const (
	TransactionStatusPending   TransactionStatus = "pending"
	TransactionStatusCompleted TransactionStatus = "completed"
	TransactionStatusFailed    TransactionStatus = "failed"
)

// CreateWalletRequest represents a request to create a new wallet
type CreateWalletRequest struct {
	AgentID       string  `json:"agent_id"`
	AgentName     string  `json:"agent_name"`
	InitialTokens float64 `json:"initial_tokens,omitempty"`
}

// DepositRequest represents a request to deposit tokens
type DepositRequest struct {
	Amount      float64 `json:"amount"`
	Description string  `json:"description,omitempty"`
}

// WithdrawRequest represents a request to withdraw tokens
type WithdrawRequest struct {
	Amount      float64 `json:"amount"`
	Description string  `json:"description,omitempty"`
}

// TransferRequest represents a request to transfer tokens between wallets
type TransferRequest struct {
	FromAgentID string  `json:"from_agent_id"`
	ToAgentID   string  `json:"to_agent_id"`
	Amount      float64 `json:"amount"`
	Reference   string  `json:"reference,omitempty"`
	Description string  `json:"description,omitempty"`
}

// BalanceResponse represents a balance query response
type BalanceResponse struct {
	AgentID   string  `json:"agent_id"`
	Balance   float64 `json:"balance"`
	TokenType string  `json:"token_type"`
}

// WalletListResponse represents a list of wallets
type WalletListResponse struct {
	Wallets []Wallet `json:"wallets"`
	Count   int      `json:"count"`
}

// TransactionListResponse represents a list of transactions
type TransactionListResponse struct {
	Transactions []Transaction `json:"transactions"`
	Count        int           `json:"count"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// ===== Phase 7: Secure Banking Model =====

// Treasury represents the bank's token reserve
type Treasury struct {
	ID          string    `json:"id"`
	TotalSupply float64   `json:"total_supply"`
	Allocated   float64   `json:"allocated"`
	Available   float64   `json:"available"`
	TokenType   string    `json:"token_type"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TreasuryResponse represents treasury info for API responses
type TreasuryResponse struct {
	TotalSupply float64 `json:"total_supply"`
	Allocated   float64 `json:"allocated"`
	Available   float64 `json:"available"`
	TokenType   string  `json:"token_type"`
}

// AgentRegistryEntry represents a pre-registered agent in the bank
type AgentRegistryEntry struct {
	AgentID    string  `json:"agent_id"`
	AgentName  string  `json:"agent_name"`
	Allocation float64 `json:"allocation"`
	Token      string  `json:"token"`      // Plain text in config file
	TokenHash  string  `json:"-"`          // SHA256 hash (not serialized)
}

// TreasuryConfig defines the token economy configuration
type TreasuryConfig struct {
	TotalSupply float64 `json:"total_supply"`
	TokenType   string  `json:"token_type"`
}

// AgentRegistry holds all registered agents and treasury config
type AgentRegistry struct {
	Treasury TreasuryConfig       `json:"treasury"`
	Agents   []AgentRegistryEntry `json:"agents"`
}
