// Package ap2 implements AP2 payment provider for Token Bank.
package ap2

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TokenPaymentProvider implements AP2 payment provider using AEX tokens.
type TokenPaymentProvider struct {
	mu              sync.RWMutex
	mandates        map[string]*MandateRecord
	transferHandler TransferHandler
}

// TransferHandler is an interface for executing token transfers.
type TransferHandler interface {
	Transfer(fromAgentID, toAgentID string, amount float64, reference, description string) (string, error)
	GetBalance(agentID string) (float64, error)
}

// NewTokenPaymentProvider creates a new AP2 payment provider.
func NewTokenPaymentProvider(handler TransferHandler) *TokenPaymentProvider {
	return &TokenPaymentProvider{
		mandates:        make(map[string]*MandateRecord),
		transferHandler: handler,
	}
}

// GetCapabilities returns the provider's capabilities.
func (p *TokenPaymentProvider) GetCapabilities() *ProviderCapabilities {
	return &ProviderCapabilities{
		ProviderID:       "aex-token-bank",
		ProviderName:     "AEX Token Bank",
		SupportedMethods: []string{"aex-token", "AEX_BALANCE"},
		TokenType:        "AEX",
		FraudProtection:  "standard",
		Version:          "1.0.0",
	}
}

// SubmitBid responds to a bid request with pricing.
func (p *TokenPaymentProvider) SubmitBid(req *BidRequest) *BidResponse {
	// Token Bank offers competitive pricing for AEX token payments
	// No fees for internal token transfers
	return &BidResponse{
		ProviderID:            "aex-token-bank",
		ProviderName:          "AEX Token Bank",
		BaseFeePercent:        0.0,  // No base fee for token transfers
		RewardPercent:         0.5,  // 0.5% reward for using tokens
		NetFeePercent:         -0.5, // Negative = cashback
		ProcessingTimeSeconds: 1,    // Near-instant settlement
		SupportedMethods:      []string{"aex-token", "AEX_BALANCE"},
		FraudProtection:       "standard",
	}
}

// CreateIntentMandate creates a new intent mandate.
func (p *TokenPaymentProvider) CreateIntentMandate(
	consumerID string,
	providerID string,
	amount float64,
	description string,
	expiresIn time.Duration,
) (*IntentMandate, string, error) {
	intent := &IntentMandate{
		UserCartConfirmationRequired: false, // Agent-to-agent flow, no user confirmation
		NaturalLanguageDescription:   description,
		Merchants:                    []string{providerID},
		RequiresRefundability:        false,
		IntentExpiry:                 time.Now().Add(expiresIn).Format(time.RFC3339),
	}

	// Store mandate record
	recordID := uuid.New().String()
	record := &MandateRecord{
		ID:            recordID,
		Type:          "intent",
		ConsumerID:    consumerID,
		ProviderID:    providerID,
		Amount:        amount,
		Currency:      "AEX",
		Status:        "pending",
		IntentMandate: intent,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(expiresIn),
	}

	p.mu.Lock()
	p.mandates[recordID] = record
	p.mu.Unlock()

	return intent, recordID, nil
}

// CreateCartMandate creates a cart mandate from an intent mandate.
func (p *TokenPaymentProvider) CreateCartMandate(
	intentID string,
	items []PaymentItem,
	total PaymentItem,
	expiresIn time.Duration,
) (*CartMandate, string, error) {
	p.mu.RLock()
	intentRecord, exists := p.mandates[intentID]
	p.mu.RUnlock()

	if !exists {
		return nil, "", fmt.Errorf("intent mandate not found: %s", intentID)
	}

	if intentRecord.Status != "pending" {
		return nil, "", fmt.Errorf("intent mandate is not pending: %s", intentRecord.Status)
	}

	cartID := uuid.New().String()
	cart := &CartMandate{
		Contents: CartContents{
			ID:                           cartID,
			UserCartConfirmationRequired: false,
			PaymentRequest: PaymentRequest{
				ID:               uuid.New().String(),
				SupportedMethods: []string{"aex-token"},
				DisplayItems:     items,
				Total:            total,
			},
			CartExpiry:   time.Now().Add(expiresIn).Format(time.RFC3339),
			MerchantName: intentRecord.ProviderID,
		},
		MerchantAuthorization: p.signCart(cartID, intentRecord.ProviderID),
	}

	// Create new record for cart mandate
	recordID := uuid.New().String()
	record := &MandateRecord{
		ID:            recordID,
		Type:          "cart",
		ConsumerID:    intentRecord.ConsumerID,
		ProviderID:    intentRecord.ProviderID,
		Amount:        intentRecord.Amount,
		Currency:      "AEX",
		Status:        "pending",
		IntentMandate: intentRecord.IntentMandate,
		CartMandate:   cart,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(expiresIn),
	}

	p.mu.Lock()
	p.mandates[recordID] = record
	// Mark intent as used
	intentRecord.Status = "used"
	intentRecord.UpdatedAt = time.Now()
	p.mu.Unlock()

	return cart, recordID, nil
}

// CreatePaymentMandate creates a payment mandate from a cart mandate.
func (p *TokenPaymentProvider) CreatePaymentMandate(
	cartID string,
	paymentMethod string,
) (*PaymentMandate, string, error) {
	p.mu.RLock()
	cartRecord, exists := p.mandates[cartID]
	p.mu.RUnlock()

	if !exists {
		return nil, "", fmt.Errorf("cart mandate not found: %s", cartID)
	}

	if cartRecord.Status != "pending" {
		return nil, "", fmt.Errorf("cart mandate is not pending: %s", cartRecord.Status)
	}

	paymentMandateID := uuid.New().String()
	mandate := &PaymentMandate{
		PaymentMandateContents: PaymentMandateContents{
			PaymentMandateID:    paymentMandateID,
			PaymentDetailsID:    cartRecord.CartMandate.Contents.PaymentRequest.ID,
			PaymentDetailsTotal: cartRecord.CartMandate.Contents.PaymentRequest.Total,
			PaymentResponse: PaymentResponse{
				MethodName: paymentMethod,
				Details: map[string]interface{}{
					"token_type": "AEX",
					"wallet_id":  cartRecord.ConsumerID,
				},
			},
			MerchantAgent: cartRecord.ProviderID,
			Timestamp:     time.Now().Format(time.RFC3339),
		},
		UserAuthorization: p.signPayment(paymentMandateID, cartRecord.ConsumerID),
	}

	// Create new record for payment mandate
	recordID := uuid.New().String()
	record := &MandateRecord{
		ID:             recordID,
		Type:           "payment",
		ConsumerID:     cartRecord.ConsumerID,
		ProviderID:     cartRecord.ProviderID,
		Amount:         cartRecord.Amount,
		Currency:       "AEX",
		Status:         "pending",
		IntentMandate:  cartRecord.IntentMandate,
		CartMandate:    cartRecord.CartMandate,
		PaymentMandate: mandate,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	p.mu.Lock()
	p.mandates[recordID] = record
	// Mark cart as used
	cartRecord.Status = "used"
	cartRecord.UpdatedAt = time.Now()
	p.mu.Unlock()

	return mandate, recordID, nil
}

// ProcessPayment processes a payment mandate and executes the token transfer.
func (p *TokenPaymentProvider) ProcessPayment(req *ProcessPaymentRequest) (*ProcessPaymentResponse, error) {
	// Validate mandate
	if req.PaymentMandate.PaymentMandateContents.PaymentMandateID == "" {
		return &ProcessPaymentResponse{
			Success: false,
			Error:   "invalid payment mandate: missing mandate ID",
		}, nil
	}

	// Check consumer balance
	balance, err := p.transferHandler.GetBalance(req.FromAgentID)
	if err != nil {
		return &ProcessPaymentResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to check balance: %v", err),
		}, nil
	}

	if balance < req.Amount {
		return &ProcessPaymentResponse{
			Success: false,
			Receipt: &PaymentReceipt{
				PaymentMandateID: req.PaymentMandate.PaymentMandateContents.PaymentMandateID,
				Timestamp:        time.Now().Format(time.RFC3339),
				PaymentID:        uuid.New().String(),
				Amount: Amount{
					Currency: req.Currency,
					Value:    fmt.Sprintf("%.2f", req.Amount),
				},
				PaymentStatus: "failure",
				Error: &PaymentError{
					Code:    "INSUFFICIENT_FUNDS",
					Message: fmt.Sprintf("insufficient balance: have %.2f AEX, need %.2f AEX", balance, req.Amount),
				},
			},
			Error: fmt.Sprintf("insufficient balance: have %.2f AEX, need %.2f AEX", balance, req.Amount),
		}, nil
	}

	// Execute token transfer
	txID, err := p.transferHandler.Transfer(
		req.FromAgentID,
		req.ToAgentID,
		req.Amount,
		req.Reference,
		req.Description,
	)
	if err != nil {
		return &ProcessPaymentResponse{
			Success: false,
			Receipt: &PaymentReceipt{
				PaymentMandateID: req.PaymentMandate.PaymentMandateContents.PaymentMandateID,
				Timestamp:        time.Now().Format(time.RFC3339),
				PaymentID:        uuid.New().String(),
				Amount: Amount{
					Currency: req.Currency,
					Value:    fmt.Sprintf("%.2f", req.Amount),
				},
				PaymentStatus: "error",
				Error: &PaymentError{
					Code:    "TRANSFER_FAILED",
					Message: err.Error(),
				},
			},
			Error: fmt.Sprintf("transfer failed: %v", err),
		}, nil
	}

	// Create successful receipt
	paymentID := uuid.New().String()
	receipt := &PaymentReceipt{
		PaymentMandateID: req.PaymentMandate.PaymentMandateContents.PaymentMandateID,
		Timestamp:        time.Now().Format(time.RFC3339),
		PaymentID:        paymentID,
		Amount: Amount{
			Currency: req.Currency,
			Value:    fmt.Sprintf("%.2f", req.Amount),
		},
		PaymentStatus: "success",
		Success: &PaymentSuccess{
			MerchantConfirmationID: req.PaymentMandate.PaymentMandateContents.MerchantAgent,
			PSPConfirmationID:      txID,
			NetworkConfirmationID:  "aex-token-bank",
		},
		PaymentMethodDetails: map[string]interface{}{
			"token_type":     "AEX",
			"transaction_id": txID,
			"from_wallet":    req.FromAgentID,
			"to_wallet":      req.ToAgentID,
		},
	}

	return &ProcessPaymentResponse{
		Success:       true,
		Receipt:       receipt,
		TransactionID: txID,
	}, nil
}

// ProcessMandateChain processes a complete mandate chain (intent -> cart -> payment -> receipt).
func (p *TokenPaymentProvider) ProcessMandateChain(
	consumerID string,
	providerID string,
	amount float64,
	description string,
) (*PaymentReceipt, error) {
	// Step 1: Create Intent Mandate
	intent, intentID, err := p.CreateIntentMandate(
		consumerID,
		providerID,
		amount,
		description,
		24*time.Hour,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create intent mandate: %w", err)
	}
	_ = intent // Used for audit trail

	// Step 2: Create Cart Mandate
	items := []PaymentItem{
		{
			Label: description,
			Amount: Amount{
				Currency: "AEX",
				Value:    fmt.Sprintf("%.2f", amount),
			},
		},
	}
	total := PaymentItem{
		Label: "Total",
		Amount: Amount{
			Currency: "AEX",
			Value:    fmt.Sprintf("%.2f", amount),
		},
	}
	cart, cartID, err := p.CreateCartMandate(intentID, items, total, 15*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("failed to create cart mandate: %w", err)
	}
	_ = cart // Used for audit trail

	// Step 3: Create Payment Mandate
	paymentMandate, _, err := p.CreatePaymentMandate(cartID, "aex-token")
	if err != nil {
		return nil, fmt.Errorf("failed to create payment mandate: %w", err)
	}

	// Step 4: Process Payment
	req := &ProcessPaymentRequest{
		PaymentMandate: *paymentMandate,
		FromAgentID:    consumerID,
		ToAgentID:      providerID,
		Amount:         amount,
		Currency:       "AEX",
		Reference:      fmt.Sprintf("ap2-%s", paymentMandate.PaymentMandateContents.PaymentMandateID),
		Description:    description,
	}

	resp, err := p.ProcessPayment(req)
	if err != nil {
		return nil, fmt.Errorf("failed to process payment: %w", err)
	}

	if !resp.Success {
		return resp.Receipt, fmt.Errorf("payment failed: %s", resp.Error)
	}

	return resp.Receipt, nil
}

// GetMandateRecord retrieves a mandate record by ID.
func (p *TokenPaymentProvider) GetMandateRecord(id string) (*MandateRecord, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	record, exists := p.mandates[id]
	return record, exists
}

// ListMandates returns all mandate records for an agent.
func (p *TokenPaymentProvider) ListMandates(agentID string, mandateType string) []*MandateRecord {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var records []*MandateRecord
	for _, record := range p.mandates {
		if (record.ConsumerID == agentID || record.ProviderID == agentID) &&
			(mandateType == "" || record.Type == mandateType) {
			records = append(records, record)
		}
	}
	return records
}

// signCart creates a simple signature for the cart (demo purposes).
func (p *TokenPaymentProvider) signCart(cartID, merchantID string) string {
	data := fmt.Sprintf("%s:%s:%d", cartID, merchantID, time.Now().Unix())
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// signPayment creates a simple signature for the payment (demo purposes).
func (p *TokenPaymentProvider) signPayment(mandateID, userID string) string {
	data := fmt.Sprintf("%s:%s:%d", mandateID, userID, time.Now().Unix())
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// MarshalJSON implements custom JSON marshaling for MandateRecord.
func (r *MandateRecord) MarshalJSON() ([]byte, error) {
	type Alias MandateRecord
	return json.Marshal(&struct {
		*Alias
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		ExpiresAt string `json:"expires_at,omitempty"`
	}{
		Alias:     (*Alias)(r),
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
		UpdatedAt: r.UpdatedAt.Format(time.RFC3339),
		ExpiresAt: r.ExpiresAt.Format(time.RFC3339),
	})
}
