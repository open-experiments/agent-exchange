package ap2

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// CredentialsProvider interface for managing payment credentials.
type CredentialsProvider interface {
	// GetPaymentMethods returns available payment methods for a user.
	GetPaymentMethods(ctx context.Context, userID string) ([]PaymentMethod, error)

	// GetPaymentToken returns a tokenized payment credential.
	GetPaymentToken(ctx context.Context, userID string, methodID string) (*PaymentMethodToken, error)

	// ProcessPayment processes a payment using the payment mandate.
	ProcessPayment(ctx context.Context, mandate *PaymentMandate) (*PaymentReceipt, error)

	// AddPaymentMethod adds a new payment method for a user.
	AddPaymentMethod(ctx context.Context, userID string, method PaymentMethod) error

	// GetDefaultPaymentMethod returns the default payment method for a user.
	GetDefaultPaymentMethod(ctx context.Context, userID string) (*PaymentMethod, error)
}

// MockCredentialsProvider is a mock implementation for testing/demo.
type MockCredentialsProvider struct {
	mu             sync.RWMutex
	paymentMethods map[string][]PaymentMethod // userID -> methods
	tokens         map[string]*PaymentMethodToken
}

// NewMockCredentialsProvider creates a new mock credentials provider.
func NewMockCredentialsProvider() *MockCredentialsProvider {
	cp := &MockCredentialsProvider{
		paymentMethods: make(map[string][]PaymentMethod),
		tokens:         make(map[string]*PaymentMethodToken),
	}

	// Add some default payment methods for demo users
	cp.initDemoData()

	return cp
}

// initDemoData adds demo payment methods.
func (cp *MockCredentialsProvider) initDemoData() {
	// Demo consumer payment methods
	demoMethods := []PaymentMethod{
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

	// Assign to common demo user IDs
	demoUsers := []string{
		"demo-consumer",
		"consumer-agent",
		"user-123",
		"orchestrator",
	}

	for _, userID := range demoUsers {
		cp.paymentMethods[userID] = demoMethods
	}
}

// GetPaymentMethods returns available payment methods for a user.
func (cp *MockCredentialsProvider) GetPaymentMethods(ctx context.Context, userID string) ([]PaymentMethod, error) {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	methods, ok := cp.paymentMethods[userID]
	if !ok {
		// Return default methods for unknown users
		return []PaymentMethod{
			{
				ID:               "pm_aex_balance",
				Type:             "AEX_BALANCE",
				DisplayName:      "AEX Account Balance",
				IsDefault:        true,
				SupportedMethods: []string{"AEX_BALANCE"},
			},
		}, nil
	}

	return methods, nil
}

// GetPaymentToken returns a tokenized payment credential.
func (cp *MockCredentialsProvider) GetPaymentToken(ctx context.Context, userID string, methodID string) (*PaymentMethodToken, error) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	// Verify method exists for user
	methods, ok := cp.paymentMethods[userID]
	if !ok {
		return nil, fmt.Errorf("no payment methods for user %s", userID)
	}

	var method *PaymentMethod
	for i := range methods {
		if methods[i].ID == methodID {
			method = &methods[i]
			break
		}
	}

	if method == nil {
		return nil, fmt.Errorf("payment method %s not found for user %s", methodID, userID)
	}

	// Generate token
	tokenID := generateTokenID()
	token := &PaymentMethodToken{
		Token:     tokenID,
		MethodID:  methodID,
		ExpiresAt: time.Now().Add(15 * time.Minute),
		TokenType: "SINGLE_USE",
	}

	cp.tokens[tokenID] = token

	return token, nil
}

// ProcessPayment processes a payment using the payment mandate.
func (cp *MockCredentialsProvider) ProcessPayment(ctx context.Context, mandate *PaymentMandate) (*PaymentReceipt, error) {
	if mandate == nil {
		return nil, fmt.Errorf("payment mandate is nil")
	}

	// Validate mandate
	if mandate.PaymentMandateContents.PaymentDetailsTotal.Amount.Value <= 0 {
		return &PaymentReceipt{
			ReceiptID:        fmt.Sprintf("rcpt_%s", generateTokenID()),
			PaymentMandateID: mandate.PaymentMandateContents.PaymentMandateID,
			Status:           "FAILED",
			Timestamp:        time.Now(),
			ErrorMessage:     "invalid payment amount",
		}, nil
	}

	// Check if using token
	if details := mandate.PaymentMandateContents.PaymentResponse.Details; details != nil {
		if tokenStr, ok := details["token"].(string); ok {
			cp.mu.RLock()
			token, exists := cp.tokens[tokenStr]
			cp.mu.RUnlock()

			if !exists {
				return &PaymentReceipt{
					ReceiptID:        fmt.Sprintf("rcpt_%s", generateTokenID()),
					PaymentMandateID: mandate.PaymentMandateContents.PaymentMandateID,
					Status:           "FAILED",
					Timestamp:        time.Now(),
					ErrorMessage:     "invalid or expired token",
				}, nil
			}

			if time.Now().After(token.ExpiresAt) {
				return &PaymentReceipt{
					ReceiptID:        fmt.Sprintf("rcpt_%s", generateTokenID()),
					PaymentMandateID: mandate.PaymentMandateContents.PaymentMandateID,
					Status:           "FAILED",
					Timestamp:        time.Now(),
					ErrorMessage:     "token has expired",
				}, nil
			}
		}
	}

	// Simulate successful payment
	receipt := &PaymentReceipt{
		ReceiptID:        fmt.Sprintf("rcpt_%s", generateTokenID()),
		PaymentMandateID: mandate.PaymentMandateContents.PaymentMandateID,
		Status:           "SUCCESS",
		TransactionID:    fmt.Sprintf("txn_%s", generateTokenID()),
		Amount:           mandate.PaymentMandateContents.PaymentDetailsTotal.Amount,
		Timestamp:        time.Now(),
	}

	return receipt, nil
}

// AddPaymentMethod adds a new payment method for a user.
func (cp *MockCredentialsProvider) AddPaymentMethod(ctx context.Context, userID string, method PaymentMethod) error {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	methods := cp.paymentMethods[userID]

	// Check if method already exists
	for _, m := range methods {
		if m.ID == method.ID {
			return fmt.Errorf("payment method %s already exists", method.ID)
		}
	}

	cp.paymentMethods[userID] = append(methods, method)
	return nil
}

// GetDefaultPaymentMethod returns the default payment method for a user.
func (cp *MockCredentialsProvider) GetDefaultPaymentMethod(ctx context.Context, userID string) (*PaymentMethod, error) {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	methods, ok := cp.paymentMethods[userID]
	if !ok || len(methods) == 0 {
		return nil, fmt.Errorf("no payment methods for user %s", userID)
	}

	// Find default method
	for i := range methods {
		if methods[i].IsDefault {
			return &methods[i], nil
		}
	}

	// Return first method if no default
	return &methods[0], nil
}

// generateTokenID creates a random token ID.
func generateTokenID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
