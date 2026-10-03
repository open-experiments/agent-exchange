package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/parlakisik/agent-exchange/aex-settlement/internal/model"
	"github.com/parlakisik/agent-exchange/aex-settlement/internal/service"
)

type Handlers struct {
	svc *service.Service
}

func NewHandlers(svc *service.Service) *Handlers {
	return &Handlers{svc: svc}
}

// GetUsage retrieves usage data for a tenant
// GET /v1/usage?tenant_id={id}&limit={n}
func (h *Handlers) GetUsage(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		respondError(w, http.StatusBadRequest, "TENANT_ID_REQUIRED", "tenant_id is required")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	usage, err := h.svc.GetUsage(r.Context(), tenantID, limit)
	if err != nil {
		slog.ErrorContext(r.Context(), "get usage failed", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}

	respondJSON(w, http.StatusOK, usage)
}

// GetBalance retrieves balance for a tenant
// GET /v1/balance?tenant_id={id}
func (h *Handlers) GetBalance(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		respondError(w, http.StatusBadRequest, "TENANT_ID_REQUIRED", "tenant_id is required")
		return
	}

	balance, err := h.svc.GetBalance(r.Context(), tenantID)
	if err != nil {
		slog.ErrorContext(r.Context(), "get balance failed", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}

	respondJSON(w, http.StatusOK, balance)
}

// GetTransactions retrieves transaction history for a tenant
// GET /v1/usage/transactions?tenant_id={id}&limit={n}
func (h *Handlers) GetTransactions(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		respondError(w, http.StatusBadRequest, "TENANT_ID_REQUIRED", "tenant_id is required")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	transactions, err := h.svc.GetTransactions(r.Context(), tenantID, limit)
	if err != nil {
		slog.ErrorContext(r.Context(), "get transactions failed", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}

	respondJSON(w, http.StatusOK, transactions)
}

// ProcessDeposit handles a deposit request
// POST /v1/deposits
func (h *Handlers) ProcessDeposit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID string `json:"tenant_id"`
		Amount   string `json:"amount"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.TenantID == "" || req.Amount == "" {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "tenant_id and amount are required")
		return
	}

	tx, err := h.svc.ProcessDeposit(r.Context(), req.TenantID, req.Amount)
	if err != nil {
		slog.ErrorContext(r.Context(), "process deposit failed", "error", err)
		if errors.Is(err, service.ErrInvalidAmount) {
			respondError(w, http.StatusBadRequest, "INVALID_AMOUNT", "invalid amount")
			return
		}
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}

	respondJSON(w, http.StatusCreated, tx)
}

// ProcessContractCompletion handles internal contract completion events
// POST /internal/settlement/complete
//
// 200 {"status":"settled","charged":true,...}: the consumer was debited and the provider credited.
// A success=true event for a contract recorded as success=false also settles this way.
// 200 {"status":"recorded","charged":false,...}: success=false; recorded, no money moved.
// 400: the event names no real consumer/provider or has an invalid price.
// 402 CONSUMER_ACCOUNT_NOT_FOUND: the consumer has no balance account to debit.
// 409 EXECUTION_EXISTS: the contract was already settled (money moved exactly once),
// or success=false was repeated for a contract already recorded as failed.
// 5xx: the settlement may be recorded but unfinished; the caller should retry,
// and the settlement resumer finishes it regardless.
func (h *Handlers) ProcessContractCompletion(w http.ResponseWriter, r *http.Request) {
	var event model.ContractCompletedEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	result, err := h.svc.ProcessContractCompletion(r.Context(), event)
	if err != nil {
		var validationErr *service.ValidationError
		switch {
		case errors.As(err, &validationErr):
			slog.WarnContext(r.Context(), "contract completion rejected",
				"contract_id", event.ContractID,
				"code", validationErr.Code,
				"error", err,
			)
			respondError(w, http.StatusBadRequest, validationErr.Code, validationErr.Message)
		case errors.Is(err, service.ErrExecutionExists):
			slog.InfoContext(r.Context(), "contract already settled", "contract_id", event.ContractID)
			respondError(w, http.StatusConflict, "EXECUTION_EXISTS", "execution already recorded")
		case errors.Is(err, service.ErrConsumerAccountNotFound):
			slog.WarnContext(r.Context(), "contract completion rejected: consumer has no balance account",
				"contract_id", event.ContractID,
				"consumer_id", event.ConsumerID,
			)
			respondError(w, http.StatusPaymentRequired, "CONSUMER_ACCOUNT_NOT_FOUND", "consumer has no balance account to debit")
		default:
			slog.ErrorContext(r.Context(), "process contract completion failed", "error", err)
			respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		}
		return
	}

	respondJSON(w, http.StatusOK, result)
}

// Health check
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}

func respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, statusCode int, code string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":      code,
			"message":   message,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	})
}
