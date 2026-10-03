package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/parlakisik/agent-exchange/aex-settlement/internal/model"
	"github.com/parlakisik/agent-exchange/aex-settlement/internal/payment"
	"github.com/parlakisik/agent-exchange/aex-settlement/internal/store"
	"github.com/parlakisik/agent-exchange/internal/ap2"
	"github.com/parlakisik/agent-exchange/internal/events"
	"github.com/shopspring/decimal"
)

var (
	ErrExecutionExists   = errors.New("execution already recorded")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrInvalidAmount     = errors.New("invalid amount")
	ErrAP2PaymentFailed  = errors.New("AP2 payment failed")
	PlatformFeeRate      = decimal.RequireFromString("0.15") // 15% platform fee
)

type Service struct {
	store           store.SettlementStore
	events          *events.Publisher
	ap2Handler      *ap2.PaymentHandler
	ap2Enabled      bool
	paymentProvider *payment.ProviderClient
}

func New(st store.SettlementStore, pub *events.Publisher) *Service {
	if pub == nil {
		pub = events.NewPublisher("aex-settlement")
	}

	// Initialize AP2 with mock credentials provider
	credentials := ap2.NewMockCredentialsProvider()
	ap2Handler := ap2.NewPaymentHandler(credentials)

	// Check if AP2 is enabled via environment (default: true)
	ap2Enabled := os.Getenv("AP2_ENABLED") != "false"

	// Initialize payment provider client for payment provider marketplace
	paymentProviderClient := payment.NewProviderClient()

	slog.Info("settlement service initialized",
		"ap2_enabled", ap2Enabled,
		"payment_provider_marketplace", true,
	)

	return &Service{
		store:           st,
		events:          pub,
		ap2Handler:      ap2Handler,
		ap2Enabled:      ap2Enabled,
		paymentProvider: paymentProviderClient,
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

	// Determine currency
	currency := event.Currency
	if currency == "" {
		currency = "USD"
	}

	// Determine work category for payment provider selection
	workCategory := event.WorkCategory
	if workCategory == "" {
		workCategory = s.detectWorkCategory(event.Domain, event.Description)
	}

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
		WorkCategory:   workCategory,
	}

	// Get bids from payment providers and select best one
	paymentBidReq := model.PaymentBidRequest{
		Amount:       agreedPrice.InexactFloat64(),
		Currency:     currency,
		WorkCategory: workCategory,
		ConsumerID:   event.ConsumerID,
		ContractID:   event.ContractID,
	}

	bids, err := s.paymentProvider.GetPaymentBids(ctx, paymentBidReq)
	if err != nil {
		slog.WarnContext(ctx, "failed to get payment provider bids", "error", err)
	}

	if len(bids) > 0 {
		// Select best provider (lowest fee by default)
		selection := s.paymentProvider.SelectBestProvider(bids, "lowest_fee")
		selectedBid := selection.SelectedProvider

		// Calculate payment costs based on selected provider
		baseFee := agreedPrice.Mul(decimal.NewFromFloat(selectedBid.BaseFeePercent / 100)).Round(2)
		reward := agreedPrice.Mul(decimal.NewFromFloat(selectedBid.RewardPercent / 100)).Round(2)
		netCost := baseFee.Sub(reward).Round(2)

		execution.PaymentProviderID = selectedBid.ProviderID
		execution.PaymentProviderName = selectedBid.ProviderName
		execution.PaymentBaseFee = baseFee.String()
		execution.PaymentReward = reward.String()
		execution.PaymentNetCost = netCost.String()

		slog.InfoContext(ctx, "payment provider selected",
			"contract_id", event.ContractID,
			"work_category", workCategory,
			"provider_id", selectedBid.ProviderID,
			"provider_name", selectedBid.ProviderName,
			"base_fee", baseFee.String(),
			"reward", reward.String(),
			"net_cost", netCost.String(),
			"all_bids", len(bids),
		)
	}

	// Process AP2 payment if enabled
	useAP2 := s.ap2Enabled && (event.UseAP2 || s.ap2Enabled)
	if useAP2 {
		ap2Result, err := s.processAP2Payment(ctx, event, agreedPrice.InexactFloat64(), currency)
		if err != nil {
			slog.ErrorContext(ctx, "AP2 payment failed, falling back to internal settlement",
				"error", err,
				"contract_id", event.ContractID,
			)
		} else if ap2Result != nil && ap2Result.Success {
			// Update execution with AP2 payment info
			execution.AP2Enabled = true
			execution.PaymentMandateID = ap2Result.PaymentMandateID
			execution.PaymentReceiptID = ap2Result.ReceiptID
			execution.PaymentTransactionID = ap2Result.TransactionID
			execution.PaymentMethod = ap2Result.PaymentMethod

			slog.InfoContext(ctx, "AP2 payment successful",
				"contract_id", event.ContractID,
				"mandate_id", ap2Result.PaymentMandateID,
				"receipt_id", ap2Result.ReceiptID,
				"transaction_id", ap2Result.TransactionID,
			)
		}
	}

	// Save execution
	if err := s.store.SaveExecution(ctx, execution); err != nil {
		return fmt.Errorf("save execution: %w", err)
	}

	// Process internal settlement (update ledgers and balances)
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
		"ap2_enabled", execution.AP2Enabled,
	)

	// Publish settlement completed event
	eventData := map[string]any{
		"execution_id":    execution.ID,
		"contract_id":     execution.ContractID,
		"consumer_id":     execution.ConsumerID,
		"provider_id":     execution.ProviderID,
		"agreed_price":    execution.AgreedPrice,
		"platform_fee":    execution.PlatformFee,
		"provider_payout": execution.ProviderPayout,
		"ap2_enabled":     execution.AP2Enabled,
	}
	if execution.AP2Enabled {
		eventData["payment_mandate_id"] = execution.PaymentMandateID
		eventData["payment_receipt_id"] = execution.PaymentReceiptID
		eventData["payment_transaction_id"] = execution.PaymentTransactionID
	}
	_ = s.events.Publish(ctx, events.EventSettlementCompleted, eventData)

	return nil
}

// processAP2Payment handles AP2 payment processing
func (s *Service) processAP2Payment(ctx context.Context, event model.ContractCompletedEvent, amount float64, currency string) (*model.AP2PaymentResult, error) {
	description := event.Description
	if description == "" {
		description = fmt.Sprintf("Payment for contract %s in domain %s", event.ContractID, event.Domain)
	}

	req := ap2.ProcessPaymentRequest{
		ContractID:    event.ContractID,
		WorkID:        event.WorkID,
		ConsumerID:    event.ConsumerID,
		ProviderID:    event.ProviderID,
		Description:   description,
		Amount:        amount,
		Currency:      currency,
		Domain:        event.Domain,
		PaymentMethod: event.PaymentMethod,
	}

	result, err := s.ap2Handler.ProcessPayment(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("AP2 payment processing error: %w", err)
	}

	if result == nil {
		return nil, fmt.Errorf("AP2 returned nil result")
	}

	ap2Result := &model.AP2PaymentResult{
		Success:      result.Success,
		ErrorMessage: result.ErrorMessage,
	}

	if result.PaymentMandate != nil {
		ap2Result.PaymentMandateID = result.PaymentMandate.PaymentMandateContents.PaymentMandateID
		ap2Result.PaymentMethod = result.PaymentMandate.PaymentMandateContents.PaymentResponse.MethodName
	}

	if result.Receipt != nil {
		ap2Result.ReceiptID = result.Receipt.ReceiptID
		ap2Result.TransactionID = result.Receipt.TransactionID
	}

	return ap2Result, nil
}

// decimalToCents converts a decimal string (e.g. "12.50") to cents (e.g. 1250).
// Uses shopspring/decimal for precise conversion then truncates to int64.
func decimalToCents(amount string) (int64, error) {
	d, err := decimal.NewFromString(amount)
	if err != nil {
		return 0, fmt.Errorf("invalid decimal %q: %w", amount, err)
	}
	// Multiply by 100 and round to nearest cent, then convert to int64
	cents := d.Mul(decimal.NewFromInt(100)).Round(0).IntPart()
	return cents, nil
}

// centsToDecimalString converts cents (e.g. 1250) to a decimal display string (e.g. "12.50").
func centsToDecimalString(cents int64) string {
	d := decimal.New(cents, -2) // cents * 10^-2
	return d.StringFixed(2)
}

// settleExecution updates ledgers and balances for an execution.
// All four operations (consumer debit, consumer ledger, provider credit, provider ledger)
// are wrapped in a database transaction to prevent partial settlement on failure.
// Balance updates use atomic $inc to prevent read-modify-write race conditions.
func (s *Service) settleExecution(ctx context.Context, execution model.Execution) error {
	now := time.Now().UTC()

	// Convert price strings to cents for atomic integer operations
	agreedPriceCents, err := decimalToCents(execution.AgreedPrice)
	if err != nil {
		return fmt.Errorf("parse agreed_price: %w", err)
	}
	providerPayoutCents, err := decimalToCents(execution.ProviderPayout)
	if err != nil {
		return fmt.Errorf("parse provider_payout: %w", err)
	}

	return s.store.WithTransaction(ctx, func(txCtx context.Context) error {
		// 1. Atomically debit consumer balance
		updatedConsumer, err := s.store.IncrementBalance(txCtx, execution.ConsumerID, -agreedPriceCents, "USD")
		if err != nil {
			return fmt.Errorf("debit consumer balance: %w", err)
		}

		// Log warning if consumer goes negative (allowed for credit accounts)
		if updatedConsumer.Balance < 0 {
			slog.WarnContext(txCtx, "consumer has negative balance",
				"consumer_id", execution.ConsumerID,
				"balance_cents", updatedConsumer.Balance,
			)
		}

		// 2. Create consumer ledger entry (DEBIT)
		consumerEntry := model.LedgerEntry{
			ID:            generateID("ledger"),
			TenantID:      execution.ConsumerID,
			EntryType:     "DEBIT",
			Amount:        agreedPriceCents,
			BalanceAfter:  updatedConsumer.Balance,
			ReferenceType: "execution",
			ReferenceID:   execution.ID,
			Description:   fmt.Sprintf("Payment for contract %s", execution.ContractID),
			CreatedAt:     now,
		}
		if err := s.store.AppendLedgerEntry(txCtx, consumerEntry); err != nil {
			return fmt.Errorf("append consumer ledger entry: %w", err)
		}

		// 3. Atomically credit provider balance
		updatedProvider, err := s.store.IncrementBalance(txCtx, execution.ProviderID, providerPayoutCents, "USD")
		if err != nil {
			return fmt.Errorf("credit provider balance: %w", err)
		}

		// 4. Create provider ledger entry (CREDIT)
		providerEntry := model.LedgerEntry{
			ID:            generateID("ledger"),
			TenantID:      execution.ProviderID,
			EntryType:     "CREDIT",
			Amount:        providerPayoutCents,
			BalanceAfter:  updatedProvider.Balance,
			ReferenceType: "execution",
			ReferenceID:   execution.ID,
			Description:   fmt.Sprintf("Payout for contract %s", execution.ContractID),
			CreatedAt:     now,
		}
		if err := s.store.AppendLedgerEntry(txCtx, providerEntry); err != nil {
			return fmt.Errorf("append provider ledger entry: %w", err)
		}

		return nil
	})
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
		TenantID:     balance.TenantID,
		BalanceCents: balance.Balance,
		Balance:      centsToDecimalString(balance.Balance),
		Currency:     balance.Currency,
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

// ProcessDeposit processes a deposit for a tenant.
// The balance update and ledger entry are wrapped in a transaction to prevent
// partial updates. The balance increment is atomic to prevent race conditions.
func (s *Service) ProcessDeposit(ctx context.Context, tenantID string, amount string) (model.Transaction, error) {
	amountDec, err := decimal.NewFromString(amount)
	if err != nil || amountDec.LessThanOrEqual(decimal.Zero) {
		return model.Transaction{}, ErrInvalidAmount
	}

	amountCents, err := decimalToCents(amount)
	if err != nil || amountCents <= 0 {
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

	// Wrap all mutations in a transaction for atomicity
	err = s.store.WithTransaction(ctx, func(txCtx context.Context) error {
		if err := s.store.SaveTransaction(txCtx, tx); err != nil {
			return fmt.Errorf("save transaction: %w", err)
		}

		// Atomically increment balance
		updatedBalance, err := s.store.IncrementBalance(txCtx, tenantID, amountCents, "USD")
		if err != nil {
			return fmt.Errorf("increment balance: %w", err)
		}

		// Create ledger entry
		entry := model.LedgerEntry{
			ID:            generateID("ledger"),
			TenantID:      tenantID,
			EntryType:     "DEPOSIT",
			Amount:        amountCents,
			BalanceAfter:  updatedBalance.Balance,
			ReferenceType: "deposit",
			ReferenceID:   tx.ID,
			Description:   "Deposit",
			CreatedAt:     now,
		}
		if err := s.store.AppendLedgerEntry(txCtx, entry); err != nil {
			return fmt.Errorf("append ledger entry: %w", err)
		}

		return nil
	})
	if err != nil {
		return model.Transaction{}, err
	}

	slog.InfoContext(ctx, "deposit_processed", "tx_id", tx.ID, "tenant_id", tenantID, "amount", amount, "amount_cents", amountCents)

	return tx, nil
}

func generateID(prefix string) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return prefix + "_" + hex.EncodeToString(b[:])
}

// detectWorkCategory determines the work category from domain and description
func (s *Service) detectWorkCategory(domain, description string) string {
	// Check domain first
	switch domain {
	case "compliance", "regulatory":
		return "compliance"
	case "contracts", "contract":
		return "contracts"
	case "ip", "patent", "trademark":
		return "ip_patent"
	case "real_estate", "property":
		return "real_estate"
	}

	// Check description for keywords
	desc := strings.ToLower(description)

	// Contract keywords
	if strings.Contains(desc, "contract") || strings.Contains(desc, "nda") ||
		strings.Contains(desc, "agreement") || strings.Contains(desc, "terms") {
		return "contracts"
	}

	// Compliance keywords
	if strings.Contains(desc, "compliance") || strings.Contains(desc, "regulatory") ||
		strings.Contains(desc, "audit") || strings.Contains(desc, "gdpr") ||
		strings.Contains(desc, "hipaa") || strings.Contains(desc, "sox") {
		return "compliance"
	}

	// IP/Patent keywords
	if strings.Contains(desc, "patent") || strings.Contains(desc, "trademark") ||
		strings.Contains(desc, "copyright") || strings.Contains(desc, "intellectual property") {
		return "ip_patent"
	}

	// Real estate keywords
	if strings.Contains(desc, "real estate") || strings.Contains(desc, "property") ||
		strings.Contains(desc, "lease") || strings.Contains(desc, "mortgage") {
		return "real_estate"
	}

	// Default to general legal
	return "legal_research"
}
