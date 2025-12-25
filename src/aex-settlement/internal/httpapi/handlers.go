package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

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
		http.Error(w, "tenant_id is required", http.StatusBadRequest)
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
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, usage)
}

// GetBalance retrieves balance for a tenant
// GET /v1/balance?tenant_id={id}
func (h *Handlers) GetBalance(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		http.Error(w, "tenant_id is required", http.StatusBadRequest)
		return
	}

	balance, err := h.svc.GetBalance(r.Context(), tenantID)
	if err != nil {
		slog.ErrorContext(r.Context(), "get balance failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, balance)
}

// GetTransactions retrieves transaction history for a tenant
// GET /v1/usage/transactions?tenant_id={id}&limit={n}
func (h *Handlers) GetTransactions(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		http.Error(w, "tenant_id is required", http.StatusBadRequest)
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
		http.Error(w, "internal error", http.StatusInternalServerError)
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
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.TenantID == "" || req.Amount == "" {
		http.Error(w, "tenant_id and amount are required", http.StatusBadRequest)
		return
	}

	tx, err := h.svc.ProcessDeposit(r.Context(), req.TenantID, req.Amount)
	if err != nil {
		slog.ErrorContext(r.Context(), "process deposit failed", "error", err)
		if err == service.ErrInvalidAmount {
			http.Error(w, "invalid amount", http.StatusBadRequest)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusCreated, tx)
}

// ProcessContractCompletion handles internal contract completion events
// POST /internal/settlement/complete
func (h *Handlers) ProcessContractCompletion(w http.ResponseWriter, r *http.Request) {
	var event model.ContractCompletedEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.svc.ProcessContractCompletion(r.Context(), event); err != nil {
		slog.ErrorContext(r.Context(), "process contract completion failed", "error", err)
		if err == service.ErrExecutionExists {
			http.Error(w, "execution already recorded", http.StatusConflict)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "settled"})
}

// Health check
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}

func respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

// Helper to extract path segments
func getPathSegment(path string, index int) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if index < len(parts) {
		return parts[index]
	}
	return ""
}
