package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/parlakisik/agent-exchange/aex-credentials-provider/internal/service"
	"github.com/parlakisik/agent-exchange/internal/ap2"
)

// Handlers contains HTTP handlers for the Credentials Provider API.
type Handlers struct {
	svc *service.Service
}

// NewHandlers creates new HTTP handlers.
func NewHandlers(svc *service.Service) *Handlers {
	return &Handlers{svc: svc}
}

// GetPaymentMethods handles GET /v1/users/{userID}/payment-methods
func (h *Handlers) GetPaymentMethods(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("userID")
	if userID == "" {
		http.Error(w, "user_id is required", http.StatusBadRequest)
		return
	}

	methods, err := h.svc.GetPaymentMethods(r.Context(), userID)
	if err != nil {
		slog.Error("failed to get payment methods", "error", err, "user_id", userID)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"user_id": userID,
		"methods": methods,
	})
}

// GetPaymentTokenRequest is the request body for getting a payment token.
type GetPaymentTokenRequest struct {
	MethodID string `json:"method_id"`
}

// GetPaymentToken handles POST /v1/users/{userID}/tokens
func (h *Handlers) GetPaymentToken(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("userID")
	if userID == "" {
		http.Error(w, "user_id is required", http.StatusBadRequest)
		return
	}

	var req GetPaymentTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.MethodID == "" {
		http.Error(w, "method_id is required", http.StatusBadRequest)
		return
	}

	token, err := h.svc.GetPaymentToken(r.Context(), userID, req.MethodID)
	if err != nil {
		slog.Error("failed to get payment token", "error", err, "user_id", userID, "method_id", req.MethodID)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	respondJSON(w, http.StatusOK, token)
}

// ProcessPayment handles POST /v1/payments
func (h *Handlers) ProcessPayment(w http.ResponseWriter, r *http.Request) {
	var mandate ap2.PaymentMandate
	if err := json.NewDecoder(r.Body).Decode(&mandate); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	receipt, err := h.svc.ProcessPayment(r.Context(), &mandate)
	if err != nil {
		slog.Error("failed to process payment", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	status := http.StatusOK
	if receipt.Status != "SUCCESS" {
		status = http.StatusPaymentRequired
	}

	respondJSON(w, status, receipt)
}

// AddPaymentMethod handles POST /v1/users/{userID}/payment-methods
func (h *Handlers) AddPaymentMethod(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("userID")
	if userID == "" {
		http.Error(w, "user_id is required", http.StatusBadRequest)
		return
	}

	var method ap2.PaymentMethod
	if err := json.NewDecoder(r.Body).Decode(&method); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.svc.AddPaymentMethod(r.Context(), userID, method); err != nil {
		slog.Error("failed to add payment method", "error", err, "user_id", userID)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	respondJSON(w, http.StatusCreated, map[string]string{
		"status":  "created",
		"user_id": userID,
	})
}

// GetReceipt handles GET /v1/receipts/{receiptID}
func (h *Handlers) GetReceipt(w http.ResponseWriter, r *http.Request) {
	receiptID := r.PathValue("receiptID")
	if receiptID == "" {
		http.Error(w, "receipt_id is required", http.StatusBadRequest)
		return
	}

	receipt, err := h.svc.GetReceipt(r.Context(), receiptID)
	if err != nil {
		slog.Error("failed to get receipt", "error", err, "receipt_id", receiptID)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	respondJSON(w, http.StatusOK, receipt)
}

// Health handles GET /health
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{
		"status":  "healthy",
		"service": "aex-credentials-provider",
	})
}

// AgentCard handles GET /.well-known/agent.json
func (h *Handlers) AgentCard(w http.ResponseWriter, r *http.Request) {
	card := map[string]interface{}{
		"name":        "AEX Credentials Provider",
		"description": "AP2-compliant credentials provider for AEX payments",
		"url":         "http://credentials-provider.aex.local:8111",
		"version":     "1.0.0",
		"capabilities": map[string]interface{}{
			"extensions": []map[string]interface{}{
				{
					"uri":         "https://github.com/google-agentic-commerce/ap2/tree/v0.1",
					"description": "AP2 Credentials Provider",
					"required":    true,
					"params": map[string]interface{}{
						"roles": []string{"credentials-provider"},
					},
				},
			},
		},
		"skills": []map[string]interface{}{
			{
				"id":          "get_payment_methods",
				"name":        "Get Payment Methods",
				"description": "Returns available payment methods for a user",
				"tags":        []string{"payment", "methods", "ap2"},
			},
			{
				"id":          "get_payment_token",
				"name":        "Get Payment Token",
				"description": "Returns a tokenized payment credential",
				"tags":        []string{"payment", "token", "ap2"},
			},
			{
				"id":          "process_payment",
				"name":        "Process Payment",
				"description": "Processes a payment using an AP2 PaymentMandate",
				"tags":        []string{"payment", "process", "ap2"},
			},
		},
	}

	respondJSON(w, http.StatusOK, card)
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
