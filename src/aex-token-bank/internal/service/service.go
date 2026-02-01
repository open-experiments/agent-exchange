package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"

	"github.com/parlakisik/agent-exchange/aex-token-bank/internal/model"
	"github.com/parlakisik/agent-exchange/aex-token-bank/internal/store"
)

// TokenService handles business logic for token operations
type TokenService struct {
	store         *store.MemoryStore
	defaultTokens float64
	initialized   bool // Whether initialized from registry
}

// New creates a new TokenService
func New(memStore *store.MemoryStore, defaultTokens float64) *TokenService {
	return &TokenService{
		store:         memStore,
		defaultTokens: defaultTokens,
	}
}

// CreateWallet creates a new wallet for an agent
func (s *TokenService) CreateWallet(req *model.CreateWalletRequest) (*model.Wallet, error) {
	initialTokens := req.InitialTokens
	if initialTokens == 0 {
		initialTokens = s.defaultTokens
	}

	return s.store.CreateWallet(req.AgentID, req.AgentName, initialTokens)
}

// GetWallet retrieves a wallet by agent ID
func (s *TokenService) GetWallet(agentID string) (*model.Wallet, error) {
	return s.store.GetWallet(agentID)
}

// GetAllWallets retrieves all wallets
func (s *TokenService) GetAllWallets() (*model.WalletListResponse, error) {
	wallets, err := s.store.GetAllWallets()
	if err != nil {
		return nil, err
	}

	return &model.WalletListResponse{
		Wallets: wallets,
		Count:   len(wallets),
	}, nil
}

// GetBalance retrieves the balance for an agent
func (s *TokenService) GetBalance(agentID string) (*model.BalanceResponse, error) {
	balance, err := s.store.GetBalance(agentID)
	if err != nil {
		return nil, err
	}

	return &model.BalanceResponse{
		AgentID:   agentID,
		Balance:   balance,
		TokenType: "AEX",
	}, nil
}

// Deposit adds tokens to an agent's wallet
func (s *TokenService) Deposit(agentID string, req *model.DepositRequest) (*model.Transaction, error) {
	return s.store.Deposit(agentID, req.Amount, req.Description)
}

// Withdraw removes tokens from an agent's wallet
func (s *TokenService) Withdraw(agentID string, req *model.WithdrawRequest) (*model.Transaction, error) {
	return s.store.Withdraw(agentID, req.Amount, req.Description)
}

// Transfer moves tokens between two agents
func (s *TokenService) Transfer(req *model.TransferRequest) (*model.Transaction, error) {
	return s.store.Transfer(req.FromAgentID, req.ToAgentID, req.Amount, req.Reference, req.Description)
}

// GetTransactionHistory retrieves transaction history for an agent
func (s *TokenService) GetTransactionHistory(agentID string) (*model.TransactionListResponse, error) {
	transactions, err := s.store.GetTransactionHistory(agentID)
	if err != nil {
		return nil, err
	}

	return &model.TransactionListResponse{
		Transactions: transactions,
		Count:        len(transactions),
	}, nil
}

// ===== Phase 7: Secure Banking Model =====

// InitializeFromRegistry sets up treasury and agent wallets from a registry config
func (s *TokenService) InitializeFromRegistry(registry *model.AgentRegistry) error {
	if s.initialized {
		return fmt.Errorf("service already initialized from registry")
	}

	// 1. Create treasury
	treasury, err := s.store.CreateTreasury(registry.Treasury.TotalSupply, registry.Treasury.TokenType)
	if err != nil {
		return fmt.Errorf("failed to create treasury: %w", err)
	}
	slog.Info("treasury created",
		"total_supply", treasury.TotalSupply,
		"token_type", treasury.TokenType,
	)

	// 2. Create wallets for each registered agent
	for _, agent := range registry.Agents {
		// Hash the token for storage
		tokenHash := sha256Hex(agent.Token)

		// Create wallet with 0 balance initially
		wallet, err := s.store.CreateWalletWithAuth(
			agent.AgentID,
			agent.AgentName,
			0, // Start with 0, will allocate from treasury
			tokenHash,
		)
		if err != nil {
			return fmt.Errorf("failed to create wallet for %s: %w", agent.AgentID, err)
		}

		// Transfer allocation from treasury to wallet
		if agent.Allocation > 0 {
			_, err = s.store.TransferFromTreasury(agent.AgentID, agent.Allocation)
			if err != nil {
				return fmt.Errorf("failed to allocate tokens for %s: %w", agent.AgentID, err)
			}
		}

		slog.Info("agent wallet initialized",
			"agent_id", wallet.AgentID,
			"agent_name", wallet.AgentName,
			"allocation", agent.Allocation,
		)
	}

	s.initialized = true
	slog.Info("token bank initialized from registry",
		"total_agents", len(registry.Agents),
	)

	return nil
}

// GetTreasury returns the current treasury state
func (s *TokenService) GetTreasury() (*model.TreasuryResponse, error) {
	treasury, err := s.store.GetTreasury()
	if err != nil {
		return nil, err
	}

	return &model.TreasuryResponse{
		TotalSupply: treasury.TotalSupply,
		Allocated:   treasury.Allocated,
		Available:   treasury.Available,
		TokenType:   treasury.TokenType,
	}, nil
}

// GetAgentIDByTokenHash implements the AgentAuthenticator interface for auth middleware
func (s *TokenService) GetAgentIDByTokenHash(tokenHash string) (string, error) {
	return s.store.GetAgentIDByTokenHash(tokenHash)
}

// IsInitialized returns whether the service was initialized from a registry
func (s *TokenService) IsInitialized() bool {
	return s.initialized
}

// sha256Hex returns the SHA256 hash of a string as a hex-encoded string
func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
