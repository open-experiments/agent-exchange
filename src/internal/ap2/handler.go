package ap2

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// PaymentHandler handles AP2 payment flows for AEX settlements.
type PaymentHandler struct {
	generator   *MandateGenerator
	credentials CredentialsProvider
}

// NewPaymentHandler creates a new AP2 payment handler.
func NewPaymentHandler(credentials CredentialsProvider) *PaymentHandler {
	return &PaymentHandler{
		generator:   NewMandateGenerator(),
		credentials: credentials,
	}
}

// ProcessPaymentRequest represents a request to process a payment.
type ProcessPaymentRequest struct {
	ContractID    string  `json:"contract_id"`
	WorkID        string  `json:"work_id"`
	ConsumerID    string  `json:"consumer_id"`
	ProviderID    string  `json:"provider_id"`
	Description   string  `json:"description"`
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
	Domain        string  `json:"domain"`
	PaymentMethod string  `json:"payment_method,omitempty"` // Optional: specific method ID
}

// PaymentResult contains the result of a payment processing.
type PaymentResult struct {
	Success          bool            `json:"success"`
	Receipt          *PaymentReceipt `json:"receipt,omitempty"`
	IntentMandate    *IntentMandate  `json:"intent_mandate,omitempty"`
	CartMandate      *CartMandate    `json:"cart_mandate,omitempty"`
	PaymentMandate   *PaymentMandate `json:"payment_mandate,omitempty"`
	ErrorMessage     string          `json:"error_message,omitempty"`
}

// ProcessPayment handles the full AP2 payment flow for a contract.
func (h *PaymentHandler) ProcessPayment(ctx context.Context, req ProcessPaymentRequest) (*PaymentResult, error) {
	slog.InfoContext(ctx, "ap2_payment_started",
		"contract_id", req.ContractID,
		"consumer_id", req.ConsumerID,
		"provider_id", req.ProviderID,
		"amount", req.Amount,
		"currency", req.Currency,
	)

	result := &PaymentResult{}

	// 1. Create ContractInfo
	info := ContractInfo{
		ContractID:  req.ContractID,
		WorkID:      req.WorkID,
		ConsumerID:  req.ConsumerID,
		ProviderID:  req.ProviderID,
		Description: req.Description,
		Amount:      req.Amount,
		Currency:    req.Currency,
		Domain:      req.Domain,
	}

	// 2. Generate Intent Mandate
	intentMandate := h.generator.GenerateIntentMandate(info, 24*time.Hour)
	result.IntentMandate = intentMandate

	slog.DebugContext(ctx, "intent_mandate_created",
		"description", intentMandate.NaturalLanguageDescription,
		"merchants", intentMandate.Merchants,
	)

	// 3. Generate Cart Mandate (provider signs as merchant)
	cartMandate := h.generator.GenerateCartMandate(info, intentMandate, 15*time.Minute)
	result.CartMandate = cartMandate

	slog.DebugContext(ctx, "cart_mandate_created",
		"cart_id", cartMandate.Contents.ID,
		"merchant", cartMandate.Contents.MerchantName,
		"total", cartMandate.Contents.PaymentRequest.Details.Total.Amount.Value,
	)

	// 4. Get payment methods for consumer
	methods, err := h.credentials.GetPaymentMethods(ctx, req.ConsumerID)
	if err != nil {
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("failed to get payment methods: %v", err)
		return result, nil
	}

	if len(methods) == 0 {
		result.Success = false
		result.ErrorMessage = "no payment methods available"
		return result, nil
	}

	// 5. Select payment method
	var selectedMethod *PaymentMethod
	if req.PaymentMethod != "" {
		// Use specified method
		for i := range methods {
			if methods[i].ID == req.PaymentMethod {
				selectedMethod = &methods[i]
				break
			}
		}
		if selectedMethod == nil {
			result.Success = false
			result.ErrorMessage = fmt.Sprintf("payment method %s not found", req.PaymentMethod)
			return result, nil
		}
	} else {
		// Use default or first method
		for i := range methods {
			if methods[i].IsDefault {
				selectedMethod = &methods[i]
				break
			}
		}
		if selectedMethod == nil {
			selectedMethod = &methods[0]
		}
	}

	slog.DebugContext(ctx, "payment_method_selected",
		"method_id", selectedMethod.ID,
		"type", selectedMethod.Type,
		"display_name", selectedMethod.DisplayName,
	)

	// 6. Create PaymentResponse based on method type
	var paymentResponse PaymentResponse

	if selectedMethod.Type == "AEX_BALANCE" {
		// Use internal balance
		paymentResponse = CreatePaymentResponseFromBalance(
			cartMandate.Contents.PaymentRequest.Details.ID,
			req.ConsumerID,
		)
	} else {
		// Get payment token
		token, err := h.credentials.GetPaymentToken(ctx, req.ConsumerID, selectedMethod.ID)
		if err != nil {
			result.Success = false
			result.ErrorMessage = fmt.Sprintf("failed to get payment token: %v", err)
			return result, nil
		}

		paymentResponse = CreatePaymentResponseFromToken(
			cartMandate.Contents.PaymentRequest.Details.ID,
			selectedMethod.Type,
			token,
		)
	}

	// 7. Generate Payment Mandate
	paymentMandate := h.generator.GeneratePaymentMandate(cartMandate, paymentResponse, req.ProviderID)
	result.PaymentMandate = paymentMandate

	slog.DebugContext(ctx, "payment_mandate_created",
		"mandate_id", paymentMandate.PaymentMandateContents.PaymentMandateID,
		"method", paymentResponse.MethodName,
	)

	// 8. Process payment through credentials provider
	receipt, err := h.credentials.ProcessPayment(ctx, paymentMandate)
	if err != nil {
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("payment processing failed: %v", err)
		return result, nil
	}

	result.Receipt = receipt
	result.Success = receipt.Status == "SUCCESS"

	if !result.Success {
		result.ErrorMessage = receipt.ErrorMessage
	}

	slog.InfoContext(ctx, "ap2_payment_completed",
		"contract_id", req.ContractID,
		"success", result.Success,
		"receipt_id", receipt.ReceiptID,
		"transaction_id", receipt.TransactionID,
		"status", receipt.Status,
	)

	return result, nil
}

// GetPaymentMethods returns available payment methods for a user.
func (h *PaymentHandler) GetPaymentMethods(ctx context.Context, userID string) ([]PaymentMethod, error) {
	return h.credentials.GetPaymentMethods(ctx, userID)
}

// ValidateMandates validates the mandate chain for a payment.
func (h *PaymentHandler) ValidateMandates(cart *CartMandate, payment *PaymentMandate) error {
	if err := h.generator.ValidateCartMandate(cart); err != nil {
		return fmt.Errorf("invalid cart mandate: %w", err)
	}

	if err := h.generator.ValidatePaymentMandate(payment); err != nil {
		return fmt.Errorf("invalid payment mandate: %w", err)
	}

	// Verify payment mandate references the cart
	if payment.PaymentMandateContents.PaymentDetailsID != cart.Contents.PaymentRequest.Details.ID {
		return fmt.Errorf("payment mandate does not reference cart")
	}

	return nil
}
