package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/parlakisik/agent-exchange/aex-settlement/internal/model"
	"github.com/parlakisik/agent-exchange/aex-settlement/internal/store"
	"github.com/parlakisik/agent-exchange/internal/events"
	"github.com/shopspring/decimal"
)

var (
	ErrExecutionExists   = errors.New("execution already recorded")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrInvalidAmount     = errors.New("invalid amount")
	PlatformFeeRate      = decimal.RequireFromString("0.15") // 15% platform fee
)

type Service struct {
	store  store.SettlementStore
	events *events.Publisher
}

func New(st store.SettlementStore) *Service {
	return &Service{
		store:  st,
		events: events.NewPublisher("aex-settlement"),
	}
}

// ProcessContractCompletion handles a contract.completed event
func (s *Service) ProcessContractCompletion(ctx context.Context, event model.ContractCompletedEvent) error {
	// Check if already processed
	_, err := s.store.ListExecutionsByContract(ctx, event.ContractID)
	if err == nil {
		slog.WarnContext(ctx, "execution already exists", "contract_id", event.ContractID)
		return ErrExecutionExists
	}

	// Calculate costs
	agreedPrice, err := decimal.NewFromString(event.AgreedPrice)
	if err != nil {
		return fmt.Errorf("invalid agreed_price: %w", err)
	}

	breakdown := s.calculateCost(agreedPrice)

	// Calculate duration
	durationMs := event.CompletedAt.Sub(event.StartedAt).Milliseconds()

	// Create execution record
	execution := model.Execution{
		ID:             generateID("exec"),
		WorkID:         event.WorkID,
		ContractID:     event.ContractID,
		AgentID:        event.AgentID,
		ConsumerID:     event.ConsumerID,
		ProviderID:     event.ProviderID,
		Domain:         event.Domain,
		StartedAt:      event.StartedAt,
		CompletedAt:    event.CompletedAt,
		DurationMs:     durationMs,
		Status:         "COMPLETED",
		Success:        event.Success,
		AgreedPrice:    breakdown.AgreedPrice,
		PlatformFee:    breakdown.PlatformFee,
		ProviderPayout: breakdown.ProviderPayout,
		Metadata:       event.Metadata,
		CreatedAt:      time.Now().UTC(),
	}

	// Save execution
	if err := s.store.SaveExecution(ctx, execution); err != nil {
		return fmt.Errorf("save execution: %w", err)
	}

	// Process settlement (update ledgers and balances)
	if err := s.settleExecution(ctx, execution); err != nil {
		return fmt.Errorf("settle execution: %w", err)
	}

	slog.InfoContext(ctx, "contract_settled",
		"execution_id", execution.ID,
		"contract_id", execution.ContractID,
		"consumer_id", execution.ConsumerID,
		"provider_id", execution.ProviderID,
		"agreed_price", execution.AgreedPrice,
		"provider_payout", execution.ProviderPayout,
	)

	// Publish settlement completed event
	_ = s.events.Publish(ctx, events.EventSettlementCompleted, map[string]any{
		"execution_id":    execution.ID,
		"contract_id":     execution.ContractID,
		"consumer_id":     execution.ConsumerID,
		"provider_id":     execution.ProviderID,
		"agreed_price":    execution.AgreedPrice,
		"platform_fee":    execution.PlatformFee,
		"provider_payout": execution.ProviderPayout,
	})

	return nil
}

// settleExecution updates ledgers and balances for an execution
func (s *Service) settleExecution(ctx context.Context, execution model.Execution) error {
	now := time.Now().UTC()

	// Parse amounts
	agreedPrice, _ := decimal.NewFromString(execution.AgreedPrice)
	providerPayout, _ := decimal.NewFromString(execution.ProviderPayout)

	// Debit consumer
	consumerBalance, err := s.store.GetBalance(ctx, execution.ConsumerID)
	if err != nil {
		return fmt.Errorf("get consumer balance: %w", err)
	}

	currentBalance, _ := decimal.NewFromString(consumerBalance.Balance)
	newConsumerBalance := currentBalance.Sub(agreedPrice)

	// Check for sufficient funds (could be negative for credit accounts)
	if newConsumerBalance.LessThan(decimal.Zero) {
		slog.WarnContext(ctx, "consumer has negative balance",
			"consumer_id", execution.ConsumerID,
			"balance", newConsumerBalance.String(),
		)
	}

	// Update consumer balance
	consumerBalance.Balance = newConsumerBalance.String()
	consumerBalance.LastUpdated = now
	if err := s.store.UpdateBalance(ctx, consumerBalance); err != nil {
		return fmt.Errorf("update consumer balance: %w", err)
	}

	// Create consumer ledger entry (DEBIT)
	consumerEntry := model.LedgerEntry{
		ID:            generateID("ledger"),
		TenantID:      execution.ConsumerID,
		EntryType:     "DEBIT",
		Amount:        agreedPrice.String(),
		BalanceAfter:  newConsumerBalance.String(),
		ReferenceType: "execution",
		ReferenceID:   execution.ID,
		Description:   fmt.Sprintf("Payment for contract %s", execution.ContractID),
		CreatedAt:     now,
	}
	if err := s.store.AppendLedgerEntry(ctx, consumerEntry); err != nil {
		return fmt.Errorf("append consumer ledger entry: %w", err)
	}

	// Credit provider
	providerBalance, err := s.store.GetBalance(ctx, execution.ProviderID)
	if err != nil {
		return fmt.Errorf("get provider balance: %w", err)
	}

	currentBalance, _ = decimal.NewFromString(providerBalance.Balance)
	newProviderBalance := currentBalance.Add(providerPayout)

	providerBalance.Balance = newProviderBalance.String()
	providerBalance.LastUpdated = now
	if err := s.store.UpdateBalance(ctx, providerBalance); err != nil {
		return fmt.Errorf("update provider balance: %w", err)
	}

	// Create provider ledger entry (CREDIT)
	providerEntry := model.LedgerEntry{
		ID:            generateID("ledger"),
		TenantID:      execution.ProviderID,
		EntryType:     "CREDIT",
		Amount:        providerPayout.String(),
		BalanceAfter:  newProviderBalance.String(),
		ReferenceType: "execution",
		ReferenceID:   execution.ID,
		Description:   fmt.Sprintf("Payout for contract %s", execution.ContractID),
		CreatedAt:     now,
	}
	if err := s.store.AppendLedgerEntry(ctx, providerEntry); err != nil {
		return fmt.Errorf("append provider ledger entry: %w", err)
	}

	return nil
}

// calculateCost calculates platform fee and provider payout
func (s *Service) calculateCost(agreedPrice decimal.Decimal) model.CostBreakdown {
	platformFee := agreedPrice.Mul(PlatformFeeRate).Round(6)
	providerPayout := agreedPrice.Sub(platformFee).Round(6)

	return model.CostBreakdown{
		AgreedPrice:    agreedPrice.String(),
		PlatformFee:    platformFee.String(),
		ProviderPayout: providerPayout.String(),
	}
}

// GetUsage retrieves usage data for a tenant
func (s *Service) GetUsage(ctx context.Context, tenantID string, limit int) (model.UsageResponse, error) {
	executions, err := s.store.ListExecutionsByTenant(ctx, tenantID, limit)
	if err != nil {
		return model.UsageResponse{}, err
	}

	// Calculate total cost
	totalCost := decimal.Zero
	for _, exec := range executions {
		price, _ := decimal.NewFromString(exec.AgreedPrice)
		totalCost = totalCost.Add(price)
	}

	return model.UsageResponse{
		TenantID:   tenantID,
		Period:     "all", // TODO: Add period filtering
		Executions: executions,
		TotalCost:  totalCost.String(),
		Count:      len(executions),
	}, nil
}

// GetBalance retrieves balance for a tenant
func (s *Service) GetBalance(ctx context.Context, tenantID string) (model.BalanceResponse, error) {
	balance, err := s.store.GetBalance(ctx, tenantID)
	if err != nil {
		return model.BalanceResponse{}, err
	}

	return model.BalanceResponse{
		TenantID: balance.TenantID,
		Balance:  balance.Balance,
		Currency: balance.Currency,
	}, nil
}

// GetTransactions retrieves ledger entries for a tenant
func (s *Service) GetTransactions(ctx context.Context, tenantID string, limit int) (model.TransactionListResponse, error) {
	entries, err := s.store.GetLedgerEntries(ctx, tenantID, limit)
	if err != nil {
		return model.TransactionListResponse{}, err
	}

	return model.TransactionListResponse{
		Transactions: entries,
		Count:        len(entries),
	}, nil
}

// ProcessDeposit processes a deposit for a tenant
func (s *Service) ProcessDeposit(ctx context.Context, tenantID string, amount string) (model.Transaction, error) {
	amountDec, err := decimal.NewFromString(amount)
	if err != nil || amountDec.LessThanOrEqual(decimal.Zero) {
		return model.Transaction{}, ErrInvalidAmount
	}

	now := time.Now().UTC()

	// Create transaction record
	tx := model.Transaction{
		ID:          generateID("tx"),
		TenantID:    tenantID,
		Type:        "DEPOSIT",
		Amount:      amount,
		Status:      "COMPLETED",
		CreatedAt:   now,
		CompletedAt: &now,
	}

	if err := s.store.SaveTransaction(ctx, tx); err != nil {
		return model.Transaction{}, fmt.Errorf("save transaction: %w", err)
	}

	// Update balance
	balance, err := s.store.GetBalance(ctx, tenantID)
	if err != nil {
		return model.Transaction{}, err
	}

	currentBalance, _ := decimal.NewFromString(balance.Balance)
	newBalance := currentBalance.Add(amountDec)

	balance.Balance = newBalance.String()
	balance.LastUpdated = now
	if err := s.store.UpdateBalance(ctx, balance); err != nil {
		return model.Transaction{}, err
	}

	// Create ledger entry
	entry := model.LedgerEntry{
		ID:            generateID("ledger"),
		TenantID:      tenantID,
		EntryType:     "DEPOSIT",
		Amount:        amount,
		BalanceAfter:  newBalance.String(),
		ReferenceType: "deposit",
		ReferenceID:   tx.ID,
		Description:   "Deposit",
		CreatedAt:     now,
	}
	if err := s.store.AppendLedgerEntry(ctx, entry); err != nil {
		return model.Transaction{}, err
	}

	slog.InfoContext(ctx, "deposit_processed", "tx_id", tx.ID, "tenant_id", tenantID, "amount", amount)

	return tx, nil
}

func generateID(prefix string) string {
	var b [8]byte
	rand.Read(b[:])
	return prefix + "_" + hex.EncodeToString(b[:])
}
