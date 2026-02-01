// Package ap2 implements AP2 (Agent Payments Protocol) support for Token Bank.
package ap2

import "time"

// IntentMandate represents the user's initial purchase intent.
type IntentMandate struct {
	UserCartConfirmationRequired bool     `json:"user_cart_confirmation_required"`
	NaturalLanguageDescription   string   `json:"natural_language_description"`
	Merchants                    []string `json:"merchants,omitempty"`
	SKUs                         []string `json:"skus,omitempty"`
	RequiresRefundability        bool     `json:"requires_refundability"`
	IntentExpiry                 string   `json:"intent_expiry"`
}

// PaymentItem represents an item in the payment request.
type PaymentItem struct {
	Label  string `json:"label"`
	Amount Amount `json:"amount"`
}

// Amount represents a currency amount.
type Amount struct {
	Currency string `json:"currency"`
	Value    string `json:"value"`
}

// PaymentRequest follows W3C Payment Request API structure.
type PaymentRequest struct {
	ID               string        `json:"id"`
	SupportedMethods []string      `json:"supportedMethods"`
	DisplayItems     []PaymentItem `json:"displayItems"`
	Total            PaymentItem   `json:"total"`
}

// CartContents represents the merchant-signed cart details.
type CartContents struct {
	ID                           string         `json:"id"`
	UserCartConfirmationRequired bool           `json:"user_cart_confirmation_required"`
	PaymentRequest               PaymentRequest `json:"payment_request"`
	CartExpiry                   string         `json:"cart_expiry"`
	MerchantName                 string         `json:"merchant_name"`
}

// CartMandate represents a merchant-signed cart.
type CartMandate struct {
	Contents              CartContents `json:"contents"`
	MerchantAuthorization string       `json:"merchant_authorization,omitempty"`
}

// PaymentResponse represents the user's chosen payment method.
type PaymentResponse struct {
	MethodName  string                 `json:"methodName"`
	Details     map[string]interface{} `json:"details,omitempty"`
	PayerEmail  string                 `json:"payerEmail,omitempty"`
	PayerPhone  string                 `json:"payerPhone,omitempty"`
	RequestID   string                 `json:"requestId,omitempty"`
	ShippingOption string              `json:"shippingOption,omitempty"`
}

// PaymentMandateContents contains the payment mandate details.
type PaymentMandateContents struct {
	PaymentMandateID    string      `json:"payment_mandate_id"`
	PaymentDetailsID    string      `json:"payment_details_id"`
	PaymentDetailsTotal PaymentItem `json:"payment_details_total"`
	PaymentResponse     PaymentResponse `json:"payment_response"`
	MerchantAgent       string      `json:"merchant_agent"`
	Timestamp           string      `json:"timestamp"`
}

// PaymentMandate contains the user's authorization for payment.
type PaymentMandate struct {
	PaymentMandateContents PaymentMandateContents `json:"payment_mandate_contents"`
	UserAuthorization      string                 `json:"user_authorization,omitempty"`
}

// PaymentSuccess represents a successful payment.
type PaymentSuccess struct {
	MerchantConfirmationID string `json:"merchant_confirmation_id"`
	PSPConfirmationID      string `json:"psp_confirmation_id,omitempty"`
	NetworkConfirmationID  string `json:"network_confirmation_id,omitempty"`
}

// PaymentError represents a payment error.
type PaymentError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// PaymentReceipt represents the final payment result.
type PaymentReceipt struct {
	PaymentMandateID     string                 `json:"payment_mandate_id"`
	Timestamp            string                 `json:"timestamp"`
	PaymentID            string                 `json:"payment_id"`
	Amount               Amount                 `json:"amount"`
	PaymentStatus        string                 `json:"payment_status"` // "success", "error", "failure"
	Success              *PaymentSuccess        `json:"success,omitempty"`
	Error                *PaymentError          `json:"error,omitempty"`
	PaymentMethodDetails map[string]interface{} `json:"payment_method_details,omitempty"`
}

// BidRequest is sent to payment providers to request a bid.
type BidRequest struct {
	Amount       float64 `json:"amount"`
	Currency     string  `json:"currency"`
	WorkCategory string  `json:"work_category"`
	ConsumerID   string  `json:"consumer_id"`
	ProviderID   string  `json:"provider_id"`
}

// BidResponse is returned by payment providers.
type BidResponse struct {
	ProviderID            string   `json:"provider_id"`
	ProviderName          string   `json:"provider_name"`
	BaseFeePercent        float64  `json:"base_fee_percent"`
	RewardPercent         float64  `json:"reward_percent"`
	NetFeePercent         float64  `json:"net_fee_percent"`
	ProcessingTimeSeconds int      `json:"processing_time_seconds"`
	SupportedMethods      []string `json:"supported_methods"`
	FraudProtection       string   `json:"fraud_protection"` // none, basic, standard, advanced
}

// ProcessPaymentRequest is sent to the token bank to process a payment.
type ProcessPaymentRequest struct {
	PaymentMandate   PaymentMandate `json:"payment_mandate"`
	FromAgentID      string         `json:"from_agent_id"`
	ToAgentID        string         `json:"to_agent_id"`
	Amount           float64        `json:"amount"`
	Currency         string         `json:"currency"`
	Reference        string         `json:"reference,omitempty"`
	Description      string         `json:"description,omitempty"`
}

// ProcessPaymentResponse is returned after processing a payment.
type ProcessPaymentResponse struct {
	Success       bool            `json:"success"`
	Receipt       *PaymentReceipt `json:"receipt,omitempty"`
	TransactionID string          `json:"transaction_id,omitempty"`
	Error         string          `json:"error,omitempty"`
}

// ProviderCapabilities describes what the token bank payment provider supports.
type ProviderCapabilities struct {
	ProviderID       string   `json:"provider_id"`
	ProviderName     string   `json:"provider_name"`
	SupportedMethods []string `json:"supported_methods"`
	TokenType        string   `json:"token_type"`
	FraudProtection  string   `json:"fraud_protection"`
	Version          string   `json:"version"`
}

// MandateRecord stores mandate information for audit trail.
type MandateRecord struct {
	ID                string         `json:"id"`
	Type              string         `json:"type"` // intent, cart, payment
	ConsumerID        string         `json:"consumer_id"`
	ProviderID        string         `json:"provider_id"`
	Amount            float64        `json:"amount"`
	Currency          string         `json:"currency"`
	Status            string         `json:"status"` // pending, completed, failed, expired
	IntentMandate     *IntentMandate `json:"intent_mandate,omitempty"`
	CartMandate       *CartMandate   `json:"cart_mandate,omitempty"`
	PaymentMandate    *PaymentMandate `json:"payment_mandate,omitempty"`
	PaymentReceipt    *PaymentReceipt `json:"payment_receipt,omitempty"`
	TransactionID     string         `json:"transaction_id,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	ExpiresAt         time.Time      `json:"expires_at,omitempty"`
}
