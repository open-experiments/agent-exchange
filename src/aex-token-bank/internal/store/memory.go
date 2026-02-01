package store

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/parlakisik/agent-exchange/aex-token-bank/internal/model"
)

var (
	ErrWalletNotFound          = errors.New("wallet not found")
	ErrWalletAlreadyExists     = errors.New("wallet already exists for this agent")
	ErrInsufficientBalance     = errors.New("insufficient balance")
	ErrInvalidAmount           = errors.New("invalid amount")
	ErrInvalidToken            = errors.New("invalid authentication token")
	ErrInsufficientTreasury    = errors.New("insufficient treasury funds")
	ErrTreasuryAlreadyExists   = errors.New("treasury already initialized")
	ErrTreasuryNotInitialized  = errors.New("treasury not initialized")
)

// TokenStore defines the interface for token storage
type TokenStore interface {
	CreateWallet(agentID, agentName string, initialTokens float64) (*model.Wallet, error)
	GetWallet(agentID string) (*model.Wallet, error)
	GetAllWallets() ([]model.Wallet, error)
	GetBalance(agentID string) (float64, error)
	Deposit(agentID string, amount float64, description string) (*model.Transaction, error)
	Withdraw(agentID string, amount float64, description string) (*model.Transaction, error)
	Transfer(fromAgentID, toAgentID string, amount float64, reference, description string) (*model.Transaction, error)
	GetTransactionHistory(agentID string) ([]model.Transaction, error)
}

// MemoryStore implements TokenStore with in-memory storage
type MemoryStore struct {
	mu           sync.RWMutex
	wallets      map[string]*model.Wallet       // agentID -> wallet
	transactions map[string][]model.Transaction // agentID -> transactions
	treasury     *model.Treasury                // Bank's token reserve
	tokenHashes  map[string]string              // tokenHash -> agentID (for auth)
}

// NewMemoryStore creates a new in-memory token store
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		wallets:      make(map[string]*model.Wallet),
		transactions: make(map[string][]model.Transaction),
		tokenHashes:  make(map[string]string),
	}
}

// CreateWallet creates a new wallet for an agent
func (s *MemoryStore) CreateWallet(agentID, agentName string, initialTokens float64) (*model.Wallet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.wallets[agentID]; exists {
		return nil, ErrWalletAlreadyExists
	}

	now := time.Now()
	wallet := &model.Wallet{
		ID:        uuid.New().String(),
		AgentID:   agentID,
		AgentName: agentName,
		Balance:   initialTokens,
		TokenType: "AEX",
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.wallets[agentID] = wallet
	s.transactions[agentID] = []model.Transaction{}

	// Record initial deposit if tokens > 0
	if initialTokens > 0 {
		tx := model.Transaction{
			ID:          uuid.New().String(),
			FromWallet:  "SYSTEM",
			ToWallet:    agentID,
			Amount:      initialTokens,
			TokenType:   "AEX",
			Reference:   "INITIAL_DEPOSIT",
			Description: "Initial token deposit",
			Status:      string(model.TransactionStatusCompleted),
			CreatedAt:   now,
		}
		s.transactions[agentID] = append(s.transactions[agentID], tx)
	}

	return wallet, nil
}

// GetWallet returns a wallet by agent ID
func (s *MemoryStore) GetWallet(agentID string) (*model.Wallet, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	wallet, exists := s.wallets[agentID]
	if !exists {
		return nil, ErrWalletNotFound
	}

	return wallet, nil
}

// GetAllWallets returns all wallets
func (s *MemoryStore) GetAllWallets() ([]model.Wallet, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	wallets := make([]model.Wallet, 0, len(s.wallets))
	for _, w := range s.wallets {
		wallets = append(wallets, *w)
	}

	return wallets, nil
}

// GetBalance returns the balance for an agent
func (s *MemoryStore) GetBalance(agentID string) (float64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	wallet, exists := s.wallets[agentID]
	if !exists {
		return 0, ErrWalletNotFound
	}

	return wallet.Balance, nil
}

// Deposit adds tokens to a wallet
func (s *MemoryStore) Deposit(agentID string, amount float64, description string) (*model.Transaction, error) {
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	wallet, exists := s.wallets[agentID]
	if !exists {
		return nil, ErrWalletNotFound
	}

	now := time.Now()
	wallet.Balance += amount
	wallet.UpdatedAt = now

	tx := model.Transaction{
		ID:          uuid.New().String(),
		FromWallet:  "EXTERNAL",
		ToWallet:    agentID,
		Amount:      amount,
		TokenType:   "AEX",
		Reference:   "DEPOSIT",
		Description: description,
		Status:      string(model.TransactionStatusCompleted),
		CreatedAt:   now,
	}

	s.transactions[agentID] = append(s.transactions[agentID], tx)

	return &tx, nil
}

// Withdraw removes tokens from a wallet
func (s *MemoryStore) Withdraw(agentID string, amount float64, description string) (*model.Transaction, error) {
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	wallet, exists := s.wallets[agentID]
	if !exists {
		return nil, ErrWalletNotFound
	}

	if wallet.Balance < amount {
		return nil, ErrInsufficientBalance
	}

	now := time.Now()
	wallet.Balance -= amount
	wallet.UpdatedAt = now

	tx := model.Transaction{
		ID:          uuid.New().String(),
		FromWallet:  agentID,
		ToWallet:    "EXTERNAL",
		Amount:      amount,
		TokenType:   "AEX",
		Reference:   "WITHDRAWAL",
		Description: description,
		Status:      string(model.TransactionStatusCompleted),
		CreatedAt:   now,
	}

	s.transactions[agentID] = append(s.transactions[agentID], tx)

	return &tx, nil
}

// Transfer moves tokens between two wallets
func (s *MemoryStore) Transfer(fromAgentID, toAgentID string, amount float64, reference, description string) (*model.Transaction, error) {
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	fromWallet, exists := s.wallets[fromAgentID]
	if !exists {
		return nil, errors.New("source wallet not found")
	}

	toWallet, exists := s.wallets[toAgentID]
	if !exists {
		return nil, errors.New("destination wallet not found")
	}

	if fromWallet.Balance < amount {
		return nil, ErrInsufficientBalance
	}

	now := time.Now()

	// Update balances
	fromWallet.Balance -= amount
	fromWallet.UpdatedAt = now
	toWallet.Balance += amount
	toWallet.UpdatedAt = now

	// Create transaction record
	tx := model.Transaction{
		ID:          uuid.New().String(),
		FromWallet:  fromAgentID,
		ToWallet:    toAgentID,
		Amount:      amount,
		TokenType:   "AEX",
		Reference:   reference,
		Description: description,
		Status:      string(model.TransactionStatusCompleted),
		CreatedAt:   now,
	}

	// Record transaction for both parties
	s.transactions[fromAgentID] = append(s.transactions[fromAgentID], tx)
	s.transactions[toAgentID] = append(s.transactions[toAgentID], tx)

	return &tx, nil
}

// GetTransactionHistory returns all transactions for an agent
func (s *MemoryStore) GetTransactionHistory(agentID string) ([]model.Transaction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, exists := s.wallets[agentID]; !exists {
		return nil, ErrWalletNotFound
	}

	transactions := s.transactions[agentID]
	if transactions == nil {
		return []model.Transaction{}, nil
	}

	return transactions, nil
}

// ===== Phase 7: Secure Banking Model =====

// CreateTreasury initializes the bank's treasury with a total supply
func (s *MemoryStore) CreateTreasury(totalSupply float64, tokenType string) (*model.Treasury, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.treasury != nil {
		return nil, ErrTreasuryAlreadyExists
	}

	now := time.Now()
	s.treasury = &model.Treasury{
		ID:          "treasury",
		TotalSupply: totalSupply,
		Allocated:   0,
		Available:   totalSupply,
		TokenType:   tokenType,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	return s.treasury, nil
}

// GetTreasury returns the current treasury state
func (s *MemoryStore) GetTreasury() (*model.Treasury, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.treasury == nil {
		return nil, ErrTreasuryNotInitialized
	}

	// Return a copy to prevent modification
	treasuryCopy := *s.treasury
	return &treasuryCopy, nil
}

// CreateWalletWithAuth creates a new wallet with authentication token hash
func (s *MemoryStore) CreateWalletWithAuth(agentID, agentName string, initialBalance float64, tokenHash string) (*model.Wallet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.wallets[agentID]; exists {
		return nil, ErrWalletAlreadyExists
	}

	now := time.Now()
	wallet := &model.Wallet{
		ID:        uuid.New().String(),
		AgentID:   agentID,
		AgentName: agentName,
		Balance:   initialBalance,
		TokenType: "AEX",
		TokenHash: tokenHash,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.wallets[agentID] = wallet
	s.transactions[agentID] = []model.Transaction{}

	// Register token hash for authentication
	if tokenHash != "" {
		s.tokenHashes[tokenHash] = agentID
	}

	return wallet, nil
}

// GetAgentIDByTokenHash looks up an agent ID by their authentication token hash
func (s *MemoryStore) GetAgentIDByTokenHash(tokenHash string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agentID, exists := s.tokenHashes[tokenHash]
	if !exists {
		return "", ErrInvalidToken
	}

	return agentID, nil
}

// TransferFromTreasury allocates tokens from the treasury to an agent's wallet
func (s *MemoryStore) TransferFromTreasury(toAgentID string, amount float64) (*model.Transaction, error) {
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.treasury == nil {
		return nil, ErrTreasuryNotInitialized
	}

	if s.treasury.Available < amount {
		return nil, ErrInsufficientTreasury
	}

	wallet, exists := s.wallets[toAgentID]
	if !exists {
		return nil, ErrWalletNotFound
	}

	now := time.Now()

	// Deduct from treasury
	s.treasury.Available -= amount
	s.treasury.Allocated += amount
	s.treasury.UpdatedAt = now

	// Credit to wallet
	wallet.Balance += amount
	wallet.UpdatedAt = now

	// Record transaction
	tx := model.Transaction{
		ID:          uuid.New().String(),
		FromWallet:  "TREASURY",
		ToWallet:    toAgentID,
		Amount:      amount,
		TokenType:   "AEX",
		Reference:   "ALLOCATION",
		Description: "Initial token allocation from bank treasury",
		Status:      string(model.TransactionStatusCompleted),
		CreatedAt:   now,
	}

	s.transactions[toAgentID] = append(s.transactions[toAgentID], tx)

	return &tx, nil
}

// RegisterTokenHash registers a token hash for an existing wallet (for migration)
func (s *MemoryStore) RegisterTokenHash(agentID, tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	wallet, exists := s.wallets[agentID]
	if !exists {
		return ErrWalletNotFound
	}

	wallet.TokenHash = tokenHash
	s.tokenHashes[tokenHash] = agentID

	return nil
}
