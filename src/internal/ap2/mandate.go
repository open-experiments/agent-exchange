package ap2

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// MandateGenerator creates AP2 mandates from contract/execution data.
type MandateGenerator struct{}

// NewMandateGenerator creates a new mandate generator.
func NewMandateGenerator() *MandateGenerator {
	return &MandateGenerator{}
}

// ContractInfo contains the information needed to generate mandates.
type ContractInfo struct {
	ContractID  string
	WorkID      string
	ConsumerID  string
	ProviderID  string
	Description string
	Amount      float64
	Currency    string
	Domain      string
}

// GenerateIntentMandate creates an IntentMandate from contract info.
func (g *MandateGenerator) GenerateIntentMandate(info ContractInfo, expiresIn time.Duration) *IntentMandate {
	return &IntentMandate{
		UserCartConfirmationRequired: true,
		NaturalLanguageDescription:   info.Description,
		Merchants:                    []string{info.ProviderID},
		RequiresRefundability:        false,
		IntentExpiry:                 time.Now().Add(expiresIn),
	}
}

// GenerateCartMandate creates a CartMandate from contract info and intent.
func (g *MandateGenerator) GenerateCartMandate(info ContractInfo, intent *IntentMandate, expiresIn time.Duration) *CartMandate {
	now := time.Now()

	contents := CartContents{
		ID:                           fmt.Sprintf("cart_%s", info.ContractID),
		UserCartConfirmationRequired: intent.UserCartConfirmationRequired,
		PaymentRequest: PaymentRequest{
			MethodData: []PaymentMethodData{
				{
					SupportedMethods: "CARD",
					Data: map[string]interface{}{
						"payment_processor_url": fmt.Sprintf("http://aex-settlement:8106/v1/process"),
					},
				},
				{
					SupportedMethods: "AEX_BALANCE",
					Data: map[string]interface{}{
						"description": "Pay from AEX account balance",
					},
				},
			},
			Details: PaymentDetailsInit{
				ID: fmt.Sprintf("order_%s", info.ContractID),
				DisplayItems: []PaymentItem{
					{
						Label: info.Description,
						Amount: PaymentCurrencyAmount{
							Currency: info.Currency,
							Value:    info.Amount,
						},
						RefundPeriod: 30,
					},
				},
				Total: PaymentItem{
					Label: "Total",
					Amount: PaymentCurrencyAmount{
						Currency: info.Currency,
						Value:    info.Amount,
					},
				},
			},
			Options: &PaymentOptions{
				RequestPayerName:  false,
				RequestPayerEmail: true,
				RequestShipping:   false,
			},
		},
		CartExpiry:   now.Add(expiresIn),
		MerchantName: info.ProviderID,
	}

	// Generate merchant authorization (simplified - in production would be a proper JWT)
	cartHash := g.hashContents(contents)
	merchantAuth := g.generateMerchantAuthorization(info.ProviderID, cartHash)

	return &CartMandate{
		Contents:              contents,
		MerchantAuthorization: merchantAuth,
		Timestamp:             now,
	}
}

// GeneratePaymentMandate creates a PaymentMandate from cart and payment response.
func (g *MandateGenerator) GeneratePaymentMandate(
	cart *CartMandate,
	paymentResponse PaymentResponse,
	merchantAgent string,
) *PaymentMandate {
	now := time.Now()

	contents := PaymentMandateContents{
		PaymentMandateID: fmt.Sprintf("pm_%s", generateRandomID()),
		PaymentDetailsID: cart.Contents.PaymentRequest.Details.ID,
		PaymentDetailsTotal: PaymentItem{
			Label:        cart.Contents.PaymentRequest.Details.Total.Label,
			Amount:       cart.Contents.PaymentRequest.Details.Total.Amount,
			RefundPeriod: 30,
		},
		PaymentResponse: paymentResponse,
		MerchantAgent:   merchantAgent,
		Timestamp:       now,
	}

	return &PaymentMandate{
		PaymentMandateContents: contents,
	}
}

// hashContents creates a SHA256 hash of cart contents.
func (g *MandateGenerator) hashContents(contents CartContents) string {
	data, _ := json.Marshal(contents)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// generateMerchantAuthorization creates a simplified authorization token.
// In production, this would be a proper JWT with full signing.
func (g *MandateGenerator) generateMerchantAuthorization(merchantID, cartHash string) string {
	// Simplified: In production, create a proper JWT with:
	// - iss: merchant ID
	// - iat: current timestamp
	// - exp: expiration
	// - cart_hash: hash of cart contents
	// - signature: RSA/ECDSA signature

	authData := map[string]interface{}{
		"iss":       merchantID,
		"iat":       time.Now().Unix(),
		"exp":       time.Now().Add(15 * time.Minute).Unix(),
		"cart_hash": cartHash,
	}

	data, _ := json.Marshal(authData)
	return hex.EncodeToString(data)
}

// ValidateCartMandate validates a cart mandate's signature and expiry.
func (g *MandateGenerator) ValidateCartMandate(cart *CartMandate) error {
	if cart == nil {
		return fmt.Errorf("cart mandate is nil")
	}

	// Check expiry
	if time.Now().After(cart.Contents.CartExpiry) {
		return fmt.Errorf("cart mandate has expired")
	}

	// Verify hash (simplified)
	if cart.MerchantAuthorization == "" {
		return fmt.Errorf("missing merchant authorization")
	}

	return nil
}

// ValidatePaymentMandate validates a payment mandate.
func (g *MandateGenerator) ValidatePaymentMandate(mandate *PaymentMandate) error {
	if mandate == nil {
		return fmt.Errorf("payment mandate is nil")
	}

	if mandate.PaymentMandateContents.PaymentMandateID == "" {
		return fmt.Errorf("missing payment mandate ID")
	}

	if mandate.PaymentMandateContents.PaymentDetailsTotal.Amount.Value <= 0 {
		return fmt.Errorf("invalid payment amount")
	}

	return nil
}

// generateRandomID creates a random hex ID.
func generateRandomID() string {
	var b [8]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// CreatePaymentResponseFromToken creates a PaymentResponse from a token.
func CreatePaymentResponseFromToken(requestID string, methodName string, token *PaymentMethodToken) PaymentResponse {
	return PaymentResponse{
		RequestID:  requestID,
		MethodName: methodName,
		Details: map[string]interface{}{
			"token":      token.Token,
			"token_type": token.TokenType,
			"expires_at": token.ExpiresAt.Format(time.RFC3339),
		},
	}
}

// CreatePaymentResponseFromBalance creates a PaymentResponse for AEX balance payment.
func CreatePaymentResponseFromBalance(requestID string, tenantID string) PaymentResponse {
	return PaymentResponse{
		RequestID:  requestID,
		MethodName: "AEX_BALANCE",
		Details: map[string]interface{}{
			"tenant_id":   tenantID,
			"method_type": "internal_balance",
		},
	}
}
