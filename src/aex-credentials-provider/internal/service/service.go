package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/parlakisik/agent-exchange/internal/ap2"
)

// Service implements the AP2 Credentials Provider functionality.
type Service struct {
	mu             sync.RWMutex
	paymentMethods map[string][]ap2.PaymentMethod     // userID -> methods
	tokens         map[string]*ap2.PaymentMethodToken // tokenID -> token
	transactions   map[string]*ap2.PaymentReceipt     // receiptID -> receipt
}

// New creates a new Credentials Provider service.
func New() *Service {
	s := &Service{
		paymentMethods: make(map[string][]ap2.PaymentMethod),
		tokens:         make(map[string]*ap2.PaymentMethodToken),
		transactions:   make(map[string]*ap2.PaymentReceipt),
	}
	s.initDemoData()
	return s
}

// initDemoData sets up demo payment methods.
func (s *Service) initDemoData() {
	// Default payment methods available to all users
	defaultMethods := []ap2.PaymentMethod{
		{
			ID:               "pm_demo_visa_4242",
			Type:             "CARD",
			DisplayName:      "Visa ending in 4242",
			Last4:            "4242",
			ExpiryMonth:      12,
			ExpiryYear:       2027,
			Brand:            "Visa",
			IsDefault:        true,
			SupportedMethods: []string{"CARD"},
		},
		{
			ID:               "pm_demo_mc_5555",
			Type:             "CARD",
			DisplayName:      "Mastercard ending in 5555",
			Last4:            "5555",
			ExpiryMonth:      6,
			ExpiryYear:       2026,
			Brand:            "Mastercard",
			IsDefault:        false,
			SupportedMethods: []string{"CARD"},
		},
		{
			ID:               "pm_aex_balance",
			Type:             "AEX_BALANCE",
			DisplayName:      "AEX Account Balance",
			IsDefault:        false,
			SupportedMethods: []string{"AEX_BALANCE"},
		},
	}

	// Demo users
	demoUsers := []string{
		"demo-consumer",
		"consumer-agent",
		"user-123",
		"orchestrator",
		"legal-consumer",
	}

	for _, userID := range demoUsers {
		s.paymentMethods[userID] = defaultMethods
	}
}

// GetPaymentMethods returns available payment methods for a user.
func (s *Service) GetPaymentMethods(ctx context.Context, userID string) ([]ap2.PaymentMethod, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	methods, ok := s.paymentMethods[userID]
	if !ok {
		slog.InfoContext(ctx, "returning default methods for unknown user", "user_id", userID)
		// Return default AEX balance method for unknown users
		return []ap2.PaymentMethod{
			{
				ID:               "pm_aex_balance",
				Type:             "AEX_BALANCE",
				DisplayName:      "AEX Account Balance",
				IsDefault:        true,
				SupportedMethods: []string{"AEX_BALANCE"},
			},
		}, nil
	}

	slog.InfoContext(ctx, "returning payment methods", "user_id", userID, "count", len(methods))
	return methods, nil
}

// GetPaymentToken returns a tokenized payment credential.
func (s *Service) GetPaymentToken(ctx context.Context, userID string, methodID string) (*ap2.PaymentMethodToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Verify method exists for user
	methods := s.paymentMethods[userID]
	var method *ap2.PaymentMethod
	for i := range methods {
		if methods[i].ID == methodID {
			method = &methods[i]
			break
		}
	}

	if method == nil {
		// Check if it's a valid default method
		if methodID != "pm_aex_balance" {
			return nil, fmt.Errorf("payment method %s not found for user %s", methodID, userID)
		}
	}

	// Generate token
	tokenID := s.generateID("tok")
	token := &ap2.PaymentMethodToken{
		Token:     tokenID,
		MethodID:  methodID,
		ExpiresAt: time.Now().Add(15 * time.Minute),
		TokenType: "SINGLE_USE",
	}

	s.tokens[tokenID] = token

	slog.InfoContext(ctx, "payment token generated",
		"user_id", userID,
		"method_id", methodID,
		"token_id", tokenID,
		"expires_at", token.ExpiresAt,
	)

	return token, nil
}

// ProcessPayment processes a payment using the payment mandate.
func (s *Service) ProcessPayment(ctx context.Context, mandate *ap2.PaymentMandate) (*ap2.PaymentReceipt, error) {
	if mandate == nil {
		return nil, fmt.Errorf("payment mandate is nil")
	}

	receiptID := s.generateID("rcpt")

	// Validate mandate
	if mandate.PaymentMandateContents.PaymentDetailsTotal.Amount.Value <= 0 {
		receipt := &ap2.PaymentReceipt{
			ReceiptID:        receiptID,
			PaymentMandateID: mandate.PaymentMandateContents.PaymentMandateID,
			Status:           "FAILED",
			Timestamp:        time.Now(),
			ErrorMessage:     "invalid payment amount",
		}
		s.mu.Lock()
		s.transactions[receiptID] = receipt
		s.mu.Unlock()
		return receipt, nil
	}

	// Validate token if provided
	if details := mandate.PaymentMandateContents.PaymentResponse.Details; details != nil {
		if tokenStr, ok := details["token"].(string); ok {
			s.mu.RLock()
			token, exists := s.tokens[tokenStr]
			s.mu.RUnlock()

			if !exists {
				receipt := &ap2.PaymentReceipt{
					ReceiptID:        receiptID,
					PaymentMandateID: mandate.PaymentMandateContents.PaymentMandateID,
					Status:           "FAILED",
					Timestamp:        time.Now(),
					ErrorMessage:     "invalid or expired token",
				}
				s.mu.Lock()
				s.transactions[receiptID] = receipt
				s.mu.Unlock()
				return receipt, nil
			}

			if time.Now().After(token.ExpiresAt) {
				receipt := &ap2.PaymentReceipt{
					ReceiptID:        receiptID,
					PaymentMandateID: mandate.PaymentMandateContents.PaymentMandateID,
					Status:           "FAILED",
					Timestamp:        time.Now(),
					ErrorMessage:     "token has expired",
				}
				s.mu.Lock()
				s.transactions[receiptID] = receipt
				s.mu.Unlock()
				return receipt, nil
			}

			// Invalidate single-use token
			s.mu.Lock()
			delete(s.tokens, tokenStr)
			s.mu.Unlock()
		}
	}

	// Process payment (simulated success)
	transactionID := s.generateID("txn")
	receipt := &ap2.PaymentReceipt{
		ReceiptID:        receiptID,
		PaymentMandateID: mandate.PaymentMandateContents.PaymentMandateID,
		Status:           "SUCCESS",
		TransactionID:    transactionID,
		Amount:           mandate.PaymentMandateContents.PaymentDetailsTotal.Amount,
		Timestamp:        time.Now(),
	}

	s.mu.Lock()
	s.transactions[receiptID] = receipt
	s.mu.Unlock()

	slog.InfoContext(ctx, "payment processed",
		"receipt_id", receiptID,
		"transaction_id", transactionID,
		"mandate_id", mandate.PaymentMandateContents.PaymentMandateID,
		"amount", mandate.PaymentMandateContents.PaymentDetailsTotal.Amount.Value,
		"currency", mandate.PaymentMandateContents.PaymentDetailsTotal.Amount.Currency,
		"status", receipt.Status,
	)

	return receipt, nil
}

// AddPaymentMethod adds a new payment method for a user.
func (s *Service) AddPaymentMethod(ctx context.Context, userID string, method ap2.PaymentMethod) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	methods := s.paymentMethods[userID]

	// Generate ID if not provided
	if method.ID == "" {
		method.ID = s.generateID("pm")
	}

	// Check if method already exists
	for _, m := range methods {
		if m.ID == method.ID {
			return fmt.Errorf("payment method %s already exists", method.ID)
		}
	}

	s.paymentMethods[userID] = append(methods, method)

	slog.InfoContext(ctx, "payment method added",
		"user_id", userID,
		"method_id", method.ID,
		"type", method.Type,
	)

	return nil
}

// GetReceipt retrieves a payment receipt by ID.
func (s *Service) GetReceipt(ctx context.Context, receiptID string) (*ap2.PaymentReceipt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	receipt, ok := s.transactions[receiptID]
	if !ok {
		return nil, fmt.Errorf("receipt %s not found", receiptID)
	}

	return receipt, nil
}

// generateID creates a random ID with prefix.
func (s *Service) generateID(prefix string) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return prefix + "_" + hex.EncodeToString(b[:])
}
