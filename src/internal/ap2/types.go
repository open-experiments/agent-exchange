// Package ap2 implements the Agent Payments Protocol (AP2) types.
// AP2 enables AI agents to securely execute autonomous financial transactions.
// See: https://github.com/google-agentic-commerce/AP2
package ap2

import (
	"time"
)

// AP2 Data Keys for A2A message parts
const (
	IntentMandateDataKey  = "ap2.mandates.IntentMandate"
	CartMandateDataKey    = "ap2.mandates.CartMandate"
	PaymentMandateDataKey = "ap2.mandates.PaymentMandate"
)

// AP2 Roles that agents can perform
const (
	RoleMerchant            = "merchant"
	RoleShopper             = "shopper"
	RoleCredentialsProvider = "credentials-provider"
	RolePaymentProcessor    = "payment-processor"
)

// PaymentCurrencyAmount represents a monetary amount with currency.
// Based on W3C Payment Request API.
type PaymentCurrencyAmount struct {
	Currency string  `json:"currency"` // ISO 4217 currency code (e.g., "USD")
	Value    float64 `json:"value"`    // Monetary value
}

// PaymentItem represents an item for purchase.
// Based on W3C Payment Request API.
type PaymentItem struct {
	Label        string                `json:"label"`                   // Human-readable description
	Amount       PaymentCurrencyAmount `json:"amount"`                  // Monetary amount
	Pending      *bool                 `json:"pending,omitempty"`       // If true, amount is not final
	RefundPeriod int                   `json:"refund_period,omitempty"` // Refund duration in days (default: 30)
}

// PaymentShippingOption describes a shipping option.
type PaymentShippingOption struct {
	ID       string                `json:"id"`                 // Unique identifier
	Label    string                `json:"label"`              // Human-readable description
	Amount   PaymentCurrencyAmount `json:"amount"`             // Cost of shipping
	Selected bool                  `json:"selected,omitempty"` // If true, this is the default
}

// PaymentOptions specifies what information to collect.
type PaymentOptions struct {
	RequestPayerName  bool   `json:"request_payer_name,omitempty"`
	RequestPayerEmail bool   `json:"request_payer_email,omitempty"`
	RequestPayerPhone bool   `json:"request_payer_phone,omitempty"`
	RequestShipping   bool   `json:"request_shipping,omitempty"`
	ShippingType      string `json:"shipping_type,omitempty"` // "shipping", "delivery", or "pickup"
}

// PaymentMethodData indicates a payment method and associated data.
type PaymentMethodData struct {
	SupportedMethods string                 `json:"supported_methods"` // Payment method identifier (e.g., "CARD")
	Data             map[string]interface{} `json:"data,omitempty"`    // Method-specific details
}

// PaymentDetailsModifier provides details that modify payment based on method.
type PaymentDetailsModifier struct {
	SupportedMethods       string                 `json:"supported_methods"`
	Total                  *PaymentItem           `json:"total,omitempty"`
	AdditionalDisplayItems []PaymentItem          `json:"additional_display_items,omitempty"`
	Data                   map[string]interface{} `json:"data,omitempty"`
}

// PaymentDetailsInit contains the details of the payment being requested.
type PaymentDetailsInit struct {
	ID              string                   `json:"id"`                        // Unique identifier
	DisplayItems    []PaymentItem            `json:"display_items"`             // Items to display
	ShippingOptions []PaymentShippingOption  `json:"shipping_options,omitempty"`
	Modifiers       []PaymentDetailsModifier `json:"modifiers,omitempty"`
	Total           PaymentItem              `json:"total"` // Total payment amount
}

// PaymentRequest is a request for payment.
// Based on W3C Payment Request API.
type PaymentRequest struct {
	MethodData      []PaymentMethodData `json:"method_data"`                // Supported payment methods
	Details         PaymentDetailsInit  `json:"details"`                    // Financial details
	Options         *PaymentOptions     `json:"options,omitempty"`          // Collection options
	ShippingAddress *ContactAddress     `json:"shipping_address,omitempty"` // User's shipping address
}

// ContactAddress represents a physical address.
type ContactAddress struct {
	Country            string   `json:"country,omitempty"`
	AddressLine        []string `json:"address_line,omitempty"`
	Region             string   `json:"region,omitempty"`
	City               string   `json:"city,omitempty"`
	DependentLocality  string   `json:"dependent_locality,omitempty"`
	PostalCode         string   `json:"postal_code,omitempty"`
	SortingCode        string   `json:"sorting_code,omitempty"`
	Organization       string   `json:"organization,omitempty"`
	Recipient          string   `json:"recipient,omitempty"`
	Phone              string   `json:"phone,omitempty"`
}

// PaymentResponse indicates a user has chosen a payment method.
type PaymentResponse struct {
	RequestID       string                 `json:"request_id"`                 // From original PaymentRequest
	MethodName      string                 `json:"method_name"`                // Payment method chosen
	Details         map[string]interface{} `json:"details,omitempty"`          // Method-specific details
	ShippingAddress *ContactAddress        `json:"shipping_address,omitempty"`
	ShippingOption  *PaymentShippingOption `json:"shipping_option,omitempty"`
	PayerName       string                 `json:"payer_name,omitempty"`
	PayerEmail      string                 `json:"payer_email,omitempty"`
	PayerPhone      string                 `json:"payer_phone,omitempty"`
}

// IntentMandate represents the user's purchase intent.
// Used in human-present and human-not-present flows.
type IntentMandate struct {
	// If false, the agent can make purchases without user confirmation
	UserCartConfirmationRequired bool `json:"user_cart_confirmation_required"`

	// Natural language description of the user's intent
	NaturalLanguageDescription string `json:"natural_language_description"`

	// Merchants allowed to fulfill the intent (nil = any merchant)
	Merchants []string `json:"merchants,omitempty"`

	// Specific product SKUs (nil = any SKU)
	SKUs []string `json:"skus,omitempty"`

	// If true, items must be refundable
	RequiresRefundability bool `json:"requires_refundability,omitempty"`

	// When the intent mandate expires (ISO 8601 format)
	IntentExpiry time.Time `json:"intent_expiry"`

	// User signature (for human-not-present scenarios)
	UserSignature string `json:"user_signature,omitempty"`
}

// CartContents contains the detailed contents of a cart.
// Signed by the merchant to create a CartMandate.
type CartContents struct {
	ID                           string         `json:"id"`                              // Unique cart identifier
	UserCartConfirmationRequired bool           `json:"user_cart_confirmation_required"` // If true, user must confirm
	PaymentRequest               PaymentRequest `json:"payment_request"`                 // W3C PaymentRequest
	CartExpiry                   time.Time      `json:"cart_expiry"`                     // When cart expires
	MerchantName                 string         `json:"merchant_name"`                   // Name of the merchant
}

// CartMandate is a cart whose contents have been digitally signed by the merchant.
// Serves as a guarantee of items and price for a limited time.
type CartMandate struct {
	Contents CartContents `json:"contents"`

	// JWT that digitally signs the cart contents
	// Contains: iss, sub, aud, iat, exp, jti, cart_hash
	MerchantAuthorization string `json:"merchant_authorization,omitempty"`

	// Timestamp when mandate was created
	Timestamp time.Time `json:"timestamp"`
}

// PaymentMandateContents contains the data contents of a PaymentMandate.
type PaymentMandateContents struct {
	PaymentMandateID    string          `json:"payment_mandate_id"`    // Unique identifier
	PaymentDetailsID    string          `json:"payment_details_id"`    // From PaymentRequest
	PaymentDetailsTotal PaymentItem     `json:"payment_details_total"` // Total amount
	PaymentResponse     PaymentResponse `json:"payment_response"`      // User's payment choice
	MerchantAgent       string          `json:"merchant_agent"`        // Merchant identifier
	Timestamp           time.Time       `json:"timestamp"`             // When mandate was created
}

// PaymentMandate contains the user's instructions & authorization for payment.
// Shared with network/issuer for visibility into agentic transactions.
type PaymentMandate struct {
	PaymentMandateContents PaymentMandateContents `json:"payment_mandate_contents"`

	// Base64url-encoded verifiable presentation signing over cart and payment mandate
	UserAuthorization string `json:"user_authorization,omitempty"`
}

// PaymentReceipt represents the result of a payment transaction.
type PaymentReceipt struct {
	ReceiptID        string    `json:"receipt_id"`
	PaymentMandateID string    `json:"payment_mandate_id"`
	Status           string    `json:"status"` // "SUCCESS", "FAILED", "PENDING"
	TransactionID    string    `json:"transaction_id,omitempty"`
	Amount           PaymentCurrencyAmount `json:"amount"`
	Timestamp        time.Time `json:"timestamp"`
	ErrorMessage     string    `json:"error_message,omitempty"`
}

// AP2ExtensionParams defines the A2A extension parameters for AP2.
type AP2ExtensionParams struct {
	Roles []string `json:"roles"` // At least one role required
}

// PaymentMethod represents an available payment method from credentials provider.
type PaymentMethod struct {
	ID               string                 `json:"id"`                          // Unique method identifier
	Type             string                 `json:"type"`                        // "CARD", "BANK", "WALLET"
	DisplayName      string                 `json:"display_name"`                // e.g., "Visa ending in 4242"
	Last4            string                 `json:"last4,omitempty"`             // Last 4 digits (for cards)
	ExpiryMonth      int                    `json:"expiry_month,omitempty"`      // Card expiry month
	ExpiryYear       int                    `json:"expiry_year,omitempty"`       // Card expiry year
	Brand            string                 `json:"brand,omitempty"`             // e.g., "Visa", "Mastercard"
	IsDefault        bool                   `json:"is_default,omitempty"`        // If this is the default method
	SupportedMethods []string               `json:"supported_methods,omitempty"` // Payment method identifiers
	Metadata         map[string]interface{} `json:"metadata,omitempty"`          // Additional data
}

// PaymentMethodToken represents a tokenized payment credential.
type PaymentMethodToken struct {
	Token       string    `json:"token"`        // Tokenized credential
	MethodID    string    `json:"method_id"`    // Reference to PaymentMethod
	ExpiresAt   time.Time `json:"expires_at"`   // Token expiration
	TokenType   string    `json:"token_type"`   // e.g., "SINGLE_USE", "MULTI_USE"
}
