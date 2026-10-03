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
	// ErrExecutionExists means the contract is already fully settled (or was
	// recorded with no money to move). It never means "recorded but unpaid".
	ErrExecutionExists = store.ErrExecutionExists
	// ErrConsumerAccountNotFound means the consumer has no balance account
	// (no deposit was ever made), so there is nothing to debit.
	ErrConsumerAccountNotFound = errors.New("consumer balance account not found")
	ErrInsufficientFunds       = errors.New("insufficient funds")
	ErrInvalidAmount           = errors.New("invalid amount")
	ErrAP2PaymentFailed        = errors.New("AP2 payment failed")
	// ErrSettlementTooOld means a PENDING execution is too old to replay
	// safely: its balance ops may no longer be remembered. It needs an
	// operator to reconcile it by hand.
	ErrSettlementTooOld = errors.New("pending settlement too old to replay safely")
	PlatformFeeRate     = decimal.RequireFromString("0.15") // 15% platform fee
)

// unknownConsumerID is the placeholder aex-contract-engine records when a
// contract's consumer could not be resolved. It must never be charged.
const unknownConsumerID = "unknown"

// ValidationError rejects a contract.completed event that cannot be settled.
type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

// Default bounds for one settlement. defaultSettleTimeout covers the whole
// request-path settlement; defaultPaymentTimeout caps the payment provider
// and AP2 calls inside it so the store work always has time to finish.
const (
	defaultSettleTimeout  = 30 * time.Second
	defaultPaymentTimeout = 5 * time.Second
)

type Service struct {
	store           store.SettlementStore
	events          *events.Publisher
	ap2Handler      *ap2.PaymentHandler
	ap2Enabled      bool
	paymentProvider *payment.ProviderClient
	settleTimeout   time.Duration
	paymentTimeout  time.Duration
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
		settleTimeout:   defaultSettleTimeout,
		paymentTimeout:  defaultPaymentTimeout,
	}
}

// normalizeCompletionEvent trims the identifiers once, so validation and
// everything stored or charged use the same values.
func normalizeCompletionEvent(event model.ContractCompletedEvent) model.ContractCompletedEvent {
	event.ContractID = strings.TrimSpace(event.ContractID)
	event.ConsumerID = strings.TrimSpace(event.ConsumerID)
	event.ProviderID = strings.TrimSpace(event.ProviderID)
	return event
}

// validateCompletionEvent enforces the charging rules: a completion is only
// settled against a real consumer and provider for a valid price. The event
// must already be normalized.
func validateCompletionEvent(event model.ContractCompletedEvent) (decimal.Decimal, error) {
	if event.ContractID == "" {
		return decimal.Zero, &ValidationError{Code: "CONTRACT_ID_REQUIRED", Message: "contract_id is required"}
	}
	if event.ConsumerID == "" || event.ConsumerID == unknownConsumerID {
		return decimal.Zero, &ValidationError{Code: "INVALID_CONSUMER_ID", Message: "consumer_id is required and must identify a real consumer"}
	}
	if event.ProviderID == "" {
		return decimal.Zero, &ValidationError{Code: "PROVIDER_ID_REQUIRED", Message: "provider_id is required"}
	}
	agreedPrice, err := decimal.NewFromString(event.AgreedPrice)
	if err != nil || agreedPrice.IsNegative() {
		return decimal.Zero, &ValidationError{Code: "INVALID_AGREED_PRICE", Message: "agreed_price must be a non-negative decimal"}
	}
	return agreedPrice, nil
}

// ProcessContractCompletion handles a contract.completed event.
//
// Each contract settles at most once. A successful completion is recorded as
// a PENDING execution (unique on contract_id) and then driven to SETTLED by
// settlePending, which is also what the Resumer runs for executions a request
// left PENDING. ErrExecutionExists is only returned once the money has moved,
// or for a repeated unsuccessful completion. An unsuccessful completion is
// recorded as FAILED with charged=false and no balance change; a later
// successful completion for the same contract upgrades it and settles it.
//
// The store work runs detached from ctx's cancellation, bounded by its own
// timeout, so a caller that disconnects cannot abandon a settlement halfway.
func (s *Service) ProcessContractCompletion(ctx context.Context, event model.ContractCompletedEvent) (model.SettlementResult, error) {
	event = normalizeCompletionEvent(event)
	agreedPrice, err := validateCompletionEvent(event)
	if err != nil {
		return model.SettlementResult{}, err
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.settleTimeout)
	defer cancel()

	// Each pass either finishes or loses a race against a concurrent writer
	// of the same contract, in which case the next pass sees its result.
	const attempts = 3
	for i := 0; i < attempts; i++ {
		existing, err := s.store.GetExecutionByContract(ctx, event.ContractID)
		switch {
		case err == nil:
			result, raced, err := s.handleExistingExecution(ctx, existing, event, agreedPrice)
			if raced {
				continue
			}
			return result, err
		case !errors.Is(err, store.ErrExecutionNotFound):
			return model.SettlementResult{}, fmt.Errorf("look up execution for contract: %w", err)
		}

		if event.Success {
			if err := s.requireConsumerAccount(ctx, event.ConsumerID); err != nil {
				return model.SettlementResult{}, err
			}
		}

		execution := s.newExecution(event, agreedPrice, generateID("exec"), time.Now().UTC())
		if err := s.store.InsertExecution(ctx, execution); err != nil {
			if errors.Is(err, store.ErrExecutionExists) {
				// A concurrent request recorded this contract first.
				continue
			}
			return model.SettlementResult{}, fmt.Errorf("save execution: %w", err)
		}

		if !execution.Charged {
			slog.InfoContext(ctx, "contract_recorded_without_charge",
				"execution_id", execution.ID,
				"contract_id", execution.ContractID,
				"consumer_id", execution.ConsumerID,
				"provider_id", execution.ProviderID,
			)
			return settlementResult(execution), nil
		}
		return s.settlePending(ctx, execution)
	}
	return model.SettlementResult{}, fmt.Errorf("contract %s changed concurrently", event.ContractID)
}

// handleExistingExecution handles a completion for a contract that already
// has an execution: a PENDING one is finished, an uncharged failure is
// upgraded by a successful completion, anything else is a duplicate. raced is
// true when a concurrent request changed the execution first and the caller
// should look it up again.
func (s *Service) handleExistingExecution(ctx context.Context, existing model.Execution, event model.ContractCompletedEvent, agreedPrice decimal.Decimal) (result model.SettlementResult, raced bool, err error) {
	if !existing.IsSettled() {
		slog.InfoContext(ctx, "resuming pending settlement",
			"execution_id", existing.ID,
			"contract_id", existing.ContractID,
		)
		result, err := s.settlePending(ctx, existing)
		return result, false, err
	}
	if !event.Success || !existing.IsUnchargedFailure() {
		slog.WarnContext(ctx, "execution already exists", "contract_id", existing.ContractID)
		return model.SettlementResult{}, false, ErrExecutionExists
	}

	if err := s.requireConsumerAccount(ctx, event.ConsumerID); err != nil {
		return model.SettlementResult{}, false, err
	}
	upgraded := s.newExecution(event, agreedPrice, existing.ID, existing.CreatedAt)
	if err := s.store.ReplaceUnchargedFailure(ctx, upgraded); err != nil {
		if errors.Is(err, store.ErrExecutionExists) || errors.Is(err, store.ErrExecutionNotFound) {
			return model.SettlementResult{}, true, nil
		}
		return model.SettlementResult{}, false, fmt.Errorf("upgrade failed execution: %w", err)
	}
	slog.InfoContext(ctx, "failed execution upgraded to charged settlement",
		"execution_id", upgraded.ID,
		"contract_id", upgraded.ContractID,
	)
	result, err = s.settlePending(ctx, upgraded)
	return result, false, err
}

// requireConsumerAccount returns ErrConsumerAccountNotFound unless the
// consumer has a balance account to debit.
func (s *Service) requireConsumerAccount(ctx context.Context, consumerID string) error {
	exists, err := s.store.TenantExists(ctx, consumerID)
	if err != nil {
		return fmt.Errorf("look up consumer balance account: %w", err)
	}
	if !exists {
		return ErrConsumerAccountNotFound
	}
	return nil
}

// newExecution builds the execution record for a completion. A successful
// completion is PENDING and charged; its payment step runs later, in
// settlePending, once the execution is recorded.
func (s *Service) newExecution(event model.ContractCompletedEvent, agreedPrice decimal.Decimal, id string, createdAt time.Time) model.Execution {
	breakdown := s.calculateCost(agreedPrice)

	workCategory := event.WorkCategory
	if workCategory == "" {
		workCategory = s.detectWorkCategory(event.Domain, event.Description)
	}
	currency := event.Currency
	if currency == "" {
		currency = "USD"
	}

	execution := model.Execution{
		ID:                     id,
		WorkID:                 event.WorkID,
		ContractID:             event.ContractID,
		AgentID:                event.AgentID,
		ConsumerID:             event.ConsumerID,
		ProviderID:             event.ProviderID,
		Domain:                 event.Domain,
		StartedAt:              event.StartedAt,
		CompletedAt:            event.CompletedAt,
		DurationMs:             event.CompletedAt.Sub(event.StartedAt).Milliseconds(),
		Status:                 "COMPLETED",
		Success:                event.Success,
		Charged:                event.Success,
		AgreedPrice:            breakdown.AgreedPrice,
		PlatformFee:            breakdown.PlatformFee,
		ProviderPayout:         breakdown.ProviderPayout,
		Metadata:               event.Metadata,
		CreatedAt:              createdAt,
		Description:            event.Description,
		Currency:               currency,
		RequestedPaymentMethod: event.PaymentMethod,
		WorkCategory:           workCategory,
	}
	now := time.Now().UTC()
	if event.Success {
		execution.SettlementStatus = model.SettlementPending
		execution.PendingSince = &now
	} else {
		execution.Status = "FAILED"
		execution.SettlementStatus = model.SettlementSettled
		execution.SettledAt = &now
	}
	return execution
}

// settlePending drives a PENDING execution to SETTLED: it runs the payment
// step unless its outcome is already recorded, moves the money, marks the
// execution SETTLED and publishes settlement.completed. The request path and
// the Resumer both call it, possibly at the same time and on different
// replicas. Every step is idempotent and none assumes it is the only runner:
// the payment outcome and the SETTLED transition are compare-and-set, and
// balance ops and ledger entries are keyed by contract ID.
func (s *Service) settlePending(ctx context.Context, execution model.Execution) (model.SettlementResult, error) {
	if err := s.checkReplayable(execution); err != nil {
		return model.SettlementResult{}, err
	}

	if !execution.PaymentRecorded {
		payment := s.runPaymentStep(ctx, execution)
		recorded, err := s.store.RecordExecutionPayment(ctx, execution.ID, payment)
		if err != nil {
			return model.SettlementResult{}, fmt.Errorf("record payment: %w", err)
		}
		execution = recorded
	}

	if err := s.settleExecution(ctx, execution); err != nil {
		return model.SettlementResult{}, fmt.Errorf("settle execution: %w", err)
	}

	settledAt := time.Now().UTC()
	transitioned, err := s.store.MarkExecutionSettled(ctx, execution.ID, settledAt)
	if err != nil {
		return model.SettlementResult{}, fmt.Errorf("mark execution settled: %w", err)
	}
	execution.SettlementStatus = model.SettlementSettled
	if !transitioned {
		// A concurrent runner settled it and publishes the event.
		return settlementResult(execution), nil
	}
	execution.SettledAt = &settledAt

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

	return settlementResult(execution), nil
}

// checkReplayable refuses to settle an execution that has been PENDING for
// more than half the balance op retention. Its balance ops may have been
// applied long ago, and once the store forgets them a replay would apply them
// a second time. The margin keeps an op that was remembered when this check
// passed remembered while the settlement runs.
func (s *Service) checkReplayable(execution model.Execution) error {
	if execution.PendingSince == nil {
		return nil
	}
	limit := s.store.BalanceOpRetention() / 2
	if age := time.Since(*execution.PendingSince); age > limit {
		slog.Error("settlement_pending_too_old_to_replay",
			"execution_id", execution.ID,
			"contract_id", execution.ContractID,
			"pending_for", age.String(),
			"replay_limit", limit.String(),
		)
		return fmt.Errorf("%w: execution %s pending for %s", ErrSettlementTooOld, execution.ID, age)
	}
	return nil
}

// runPaymentStep runs payment provider selection and AP2 for a charged
// execution, bounded by paymentTimeout so slow payment agents cannot hold up
// the settlement. Failures are logged and fall back to internal settlement,
// as they always have.
func (s *Service) runPaymentStep(ctx context.Context, execution model.Execution) model.PaymentDetails {
	ctx, cancel := context.WithTimeout(ctx, s.paymentTimeout)
	defer cancel()

	var payment model.PaymentDetails
	agreedPrice, err := decimal.NewFromString(execution.AgreedPrice)
	if err != nil {
		slog.ErrorContext(ctx, "payment step skipped: invalid agreed_price",
			"contract_id", execution.ContractID,
			"error", err,
		)
		return payment
	}

	// Get bids from payment providers and select best one
	paymentBidReq := model.PaymentBidRequest{
		Amount:       agreedPrice.InexactFloat64(),
		Currency:     execution.Currency,
		WorkCategory: execution.WorkCategory,
		ConsumerID:   execution.ConsumerID,
		ContractID:   execution.ContractID,
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

		payment.PaymentProviderID = selectedBid.ProviderID
		payment.PaymentProviderName = selectedBid.ProviderName
		payment.PaymentBaseFee = baseFee.String()
		payment.PaymentReward = reward.String()
		payment.PaymentNetCost = netCost.String()

		slog.InfoContext(ctx, "payment provider selected",
			"contract_id", execution.ContractID,
			"work_category", execution.WorkCategory,
			"provider_id", selectedBid.ProviderID,
			"provider_name", selectedBid.ProviderName,
			"base_fee", baseFee.String(),
			"reward", reward.String(),
			"net_cost", netCost.String(),
			"all_bids", len(bids),
		)
	}

	// Process AP2 payment if enabled
	if s.ap2Enabled {
		ap2Result, err := s.processAP2Payment(ctx, execution, agreedPrice.InexactFloat64())
		if err != nil {
			slog.ErrorContext(ctx, "AP2 payment failed, falling back to internal settlement",
				"error", err,
				"contract_id", execution.ContractID,
			)
		} else if ap2Result != nil && ap2Result.Success {
			payment.AP2Enabled = true
			payment.PaymentMandateID = ap2Result.PaymentMandateID
			payment.PaymentReceiptID = ap2Result.ReceiptID
			payment.PaymentTransactionID = ap2Result.TransactionID
			payment.PaymentMethod = ap2Result.PaymentMethod

			slog.InfoContext(ctx, "AP2 payment successful",
				"contract_id", execution.ContractID,
				"mandate_id", ap2Result.PaymentMandateID,
				"receipt_id", ap2Result.ReceiptID,
				"transaction_id", ap2Result.TransactionID,
			)
		}
	}

	return payment
}

func settlementResult(execution model.Execution) model.SettlementResult {
	status := "settled"
	if !execution.Charged {
		status = "recorded"
	}
	return model.SettlementResult{
		Status:      status,
		ExecutionID: execution.ID,
		ContractID:  execution.ContractID,
		Charged:     execution.Charged,
	}
}

// processAP2Payment handles AP2 payment processing
func (s *Service) processAP2Payment(ctx context.Context, execution model.Execution, amount float64) (*model.AP2PaymentResult, error) {
	description := execution.Description
	if description == "" {
		description = fmt.Sprintf("Payment for contract %s in domain %s", execution.ContractID, execution.Domain)
	}

	req := ap2.ProcessPaymentRequest{
		ContractID:    execution.ContractID,
		WorkID:        execution.WorkID,
		ConsumerID:    execution.ConsumerID,
		ProviderID:    execution.ProviderID,
		Description:   description,
		Amount:        amount,
		Currency:      execution.Currency,
		Domain:        execution.Domain,
		PaymentMethod: execution.RequestedPaymentMethod,
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

// settleExecution debits the consumer, credits the provider and records both
// ledger entries. It runs without a multi-document transaction (standalone
// MongoDB has none): each balance change is an op keyed by contract ID that
// the store applies at most once, and each ledger entry has an ID derived
// from the same key, so re-running after a partial failure completes the
// missing steps without repeating the finished ones.
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

	// 1. Debit consumer. The account must exist: a consumer is only charged
	// after funding it with a deposit.
	debitOp := store.BalanceOp{
		ID:         execution.ContractID + ":debit",
		TenantID:   execution.ConsumerID,
		DeltaCents: -agreedPriceCents,
		Currency:   "USD",
	}
	consumerBalance, err := s.store.ApplyBalanceOp(ctx, debitOp)
	if err != nil {
		if errors.Is(err, store.ErrTenantNotFound) {
			return ErrConsumerAccountNotFound
		}
		return fmt.Errorf("debit consumer balance: %w", err)
	}

	// Log warning if consumer goes negative (allowed for credit accounts)
	if consumerBalance < 0 {
		slog.WarnContext(ctx, "consumer has negative balance",
			"consumer_id", execution.ConsumerID,
			"balance_cents", consumerBalance,
		)
	}

	// 2. Consumer ledger entry (DEBIT)
	consumerEntry := model.LedgerEntry{
		ID:            "ledger_" + debitOp.ID,
		TenantID:      execution.ConsumerID,
		EntryType:     "DEBIT",
		Amount:        agreedPriceCents,
		BalanceAfter:  consumerBalance,
		ReferenceType: "execution",
		ReferenceID:   execution.ID,
		Description:   fmt.Sprintf("Payment for contract %s", execution.ContractID),
		CreatedAt:     now,
	}
	if err := s.appendLedgerEntryOnce(ctx, consumerEntry); err != nil {
		return fmt.Errorf("append consumer ledger entry: %w", err)
	}

	// 3. Credit provider. Providers are paid without a prior deposit, so the
	// account is created on first payout.
	creditOp := store.BalanceOp{
		ID:              execution.ContractID + ":credit",
		TenantID:        execution.ProviderID,
		DeltaCents:      providerPayoutCents,
		Currency:        "USD",
		CreateIfMissing: true,
	}
	providerBalance, err := s.store.ApplyBalanceOp(ctx, creditOp)
	if err != nil {
		return fmt.Errorf("credit provider balance: %w", err)
	}

	// 4. Provider ledger entry (CREDIT)
	providerEntry := model.LedgerEntry{
		ID:            "ledger_" + creditOp.ID,
		TenantID:      execution.ProviderID,
		EntryType:     "CREDIT",
		Amount:        providerPayoutCents,
		BalanceAfter:  providerBalance,
		ReferenceType: "execution",
		ReferenceID:   execution.ID,
		Description:   fmt.Sprintf("Payout for contract %s", execution.ContractID),
		CreatedAt:     now,
	}
	if err := s.appendLedgerEntryOnce(ctx, providerEntry); err != nil {
		return fmt.Errorf("append provider ledger entry: %w", err)
	}

	return nil
}

// appendLedgerEntryOnce records entry, treating an existing entry with the
// same ID as already recorded by an earlier attempt.
func (s *Service) appendLedgerEntryOnce(ctx context.Context, entry model.LedgerEntry) error {
	err := s.store.AppendLedgerEntry(ctx, entry)
	if errors.Is(err, store.ErrLedgerEntryExists) {
		return nil
	}
	return err
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

// ProcessDeposit processes a deposit for a tenant. A deposit creates the
// tenant's balance account if it does not exist yet.
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

	// Wrap all mutations in a transaction for atomicity (where the store
	// supports one). The COMPLETED transaction record is written last so it
	// never exists without the balance change it describes.
	err = s.store.WithTransaction(ctx, func(txCtx context.Context) error {
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

		if err := s.store.SaveTransaction(txCtx, tx); err != nil {
			return fmt.Errorf("save transaction: %w", err)
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
