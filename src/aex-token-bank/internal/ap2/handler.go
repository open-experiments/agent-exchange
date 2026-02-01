// Package ap2 implements AP2 HTTP handlers for Token Bank.
package ap2

import (
	"encoding/json"
	"net/http"
	"time"
)

// Handler provides HTTP endpoints for AP2 payment processing.
type Handler struct {
	provider *TokenPaymentProvider
}

// NewHandler creates a new AP2 handler.
func NewHandler(provider *TokenPaymentProvider) *Handler {
	return &Handler{provider: provider}
}

// RegisterRoutes registers AP2 routes with the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /ap2/capabilities", h.GetCapabilities)
	mux.HandleFunc("POST /ap2/bid", h.SubmitBid)
	mux.HandleFunc("POST /ap2/intent", h.CreateIntentMandate)
	mux.HandleFunc("POST /ap2/cart", h.CreateCartMandate)
	mux.HandleFunc("POST /ap2/payment", h.CreatePaymentMandate)
	mux.HandleFunc("POST /ap2/process", h.ProcessPayment)
	mux.HandleFunc("POST /ap2/process-chain", h.ProcessMandateChain)
	mux.HandleFunc("GET /ap2/mandates/{agent_id}", h.ListMandates)
	mux.HandleFunc("GET /ap2/mandates/{agent_id}/{mandate_id}", h.GetMandate)
}

// GetCapabilities returns the provider's capabilities.
func (h *Handler) GetCapabilities(w http.ResponseWriter, r *http.Request) {
	caps := h.provider.GetCapabilities()
	respondJSON(w, http.StatusOK, caps)
}

// SubmitBid handles bid requests from settlement service.
func (h *Handler) SubmitBid(w http.ResponseWriter, r *http.Request) {
	var req BidRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	bid := h.provider.SubmitBid(&req)
	respondJSON(w, http.StatusOK, bid)
}

// CreateIntentMandateRequest is the request to create an intent mandate.
type CreateIntentMandateRequest struct {
	ConsumerID  string  `json:"consumer_id"`
	ProviderID  string  `json:"provider_id"`
	Amount      float64 `json:"amount"`
	Description string  `json:"description"`
	ExpiresIn   string  `json:"expires_in,omitempty"` // duration string, e.g., "24h"
}

// CreateIntentMandateResponse is the response after creating an intent mandate.
type CreateIntentMandateResponse struct {
	IntentMandate *IntentMandate `json:"intent_mandate"`
	MandateID     string         `json:"mandate_id"`
}

// CreateIntentMandate handles intent mandate creation.
func (h *Handler) CreateIntentMandate(w http.ResponseWriter, r *http.Request) {
	var req CreateIntentMandateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	expiresIn := 24 * time.Hour
	if req.ExpiresIn != "" {
		d, err := time.ParseDuration(req.ExpiresIn)
		if err == nil {
			expiresIn = d
		}
	}

	intent, mandateID, err := h.provider.CreateIntentMandate(
		req.ConsumerID,
		req.ProviderID,
		req.Amount,
		req.Description,
		expiresIn,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, &CreateIntentMandateResponse{
		IntentMandate: intent,
		MandateID:     mandateID,
	})
}

// CreateCartMandateRequest is the request to create a cart mandate.
type CreateCartMandateRequest struct {
	IntentMandateID string        `json:"intent_mandate_id"`
	Items           []PaymentItem `json:"items"`
	Total           PaymentItem   `json:"total"`
	ExpiresIn       string        `json:"expires_in,omitempty"` // duration string, e.g., "15m"
}

// CreateCartMandateResponse is the response after creating a cart mandate.
type CreateCartMandateResponse struct {
	CartMandate *CartMandate `json:"cart_mandate"`
	MandateID   string       `json:"mandate_id"`
}

// CreateCartMandate handles cart mandate creation.
func (h *Handler) CreateCartMandate(w http.ResponseWriter, r *http.Request) {
	var req CreateCartMandateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	expiresIn := 15 * time.Minute
	if req.ExpiresIn != "" {
		d, err := time.ParseDuration(req.ExpiresIn)
		if err == nil {
			expiresIn = d
		}
	}

	cart, mandateID, err := h.provider.CreateCartMandate(
		req.IntentMandateID,
		req.Items,
		req.Total,
		expiresIn,
	)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, &CreateCartMandateResponse{
		CartMandate: cart,
		MandateID:   mandateID,
	})
}

// CreatePaymentMandateRequest is the request to create a payment mandate.
type CreatePaymentMandateRequest struct {
	CartMandateID string `json:"cart_mandate_id"`
	PaymentMethod string `json:"payment_method"` // "aex-token"
}

// CreatePaymentMandateResponse is the response after creating a payment mandate.
type CreatePaymentMandateResponse struct {
	PaymentMandate *PaymentMandate `json:"payment_mandate"`
	MandateID      string          `json:"mandate_id"`
}

// CreatePaymentMandate handles payment mandate creation.
func (h *Handler) CreatePaymentMandate(w http.ResponseWriter, r *http.Request) {
	var req CreatePaymentMandateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.PaymentMethod == "" {
		req.PaymentMethod = "aex-token"
	}

	payment, mandateID, err := h.provider.CreatePaymentMandate(
		req.CartMandateID,
		req.PaymentMethod,
	)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, &CreatePaymentMandateResponse{
		PaymentMandate: payment,
		MandateID:      mandateID,
	})
}

// ProcessPayment handles direct payment processing.
func (h *Handler) ProcessPayment(w http.ResponseWriter, r *http.Request) {
	var req ProcessPaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Currency == "" {
		req.Currency = "AEX"
	}

	resp, err := h.provider.ProcessPayment(&req)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if resp.Success {
		respondJSON(w, http.StatusOK, resp)
	} else {
		respondJSON(w, http.StatusPaymentRequired, resp)
	}
}

// ProcessMandateChainRequest is the request for the simplified mandate chain flow.
type ProcessMandateChainRequest struct {
	ConsumerID  string  `json:"consumer_id"`
	ProviderID  string  `json:"provider_id"`
	Amount      float64 `json:"amount"`
	Description string  `json:"description"`
}

// ProcessMandateChain handles the complete mandate chain in one call.
func (h *Handler) ProcessMandateChain(w http.ResponseWriter, r *http.Request) {
	var req ProcessMandateChainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	receipt, err := h.provider.ProcessMandateChain(
		req.ConsumerID,
		req.ProviderID,
		req.Amount,
		req.Description,
	)
	if err != nil {
		// Return the receipt even on failure (contains error details)
		if receipt != nil {
			respondJSON(w, http.StatusPaymentRequired, map[string]interface{}{
				"success": false,
				"receipt": receipt,
				"error":   err.Error(),
			})
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"receipt": receipt,
	})
}

// ListMandates returns all mandates for an agent.
func (h *Handler) ListMandates(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agent_id")
	mandateType := r.URL.Query().Get("type")

	mandates := h.provider.ListMandates(agentID, mandateType)
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"mandates": mandates,
		"count":    len(mandates),
	})
}

// GetMandate returns a specific mandate by ID.
func (h *Handler) GetMandate(w http.ResponseWriter, r *http.Request) {
	mandateID := r.PathValue("mandate_id")

	record, exists := h.provider.GetMandateRecord(mandateID)
	if !exists {
		respondError(w, http.StatusNotFound, "mandate not found")
		return
	}

	respondJSON(w, http.StatusOK, record)
}

// respondJSON writes a JSON response.
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondError writes an error response.
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}
