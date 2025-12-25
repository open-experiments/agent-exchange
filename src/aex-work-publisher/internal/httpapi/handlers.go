package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/model"
	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/service"
)

type Handlers struct {
	svc *service.Service
}

func NewHandlers(svc *service.Service) *Handlers {
	return &Handlers{svc: svc}
}

// HandleSubmitWork handles POST /v1/work
func (h *Handlers) HandleSubmitWork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// In production, extract from JWT token
	// For now, use header or query param
	consumerID := r.Header.Get("X-Consumer-ID")
	if consumerID == "" {
		consumerID = "default_consumer" // TODO: Replace with actual auth
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		http.Error(w, "failed to read request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req model.WorkSubmission
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	resp, err := h.svc.PublishWork(ctx, consumerID, req)
	if err != nil {
		slog.ErrorContext(ctx, "failed to publish work", "error", err)
		http.Error(w, "failed to publish work", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

// HandleGetWork handles GET /v1/work/{work_id}
func (h *Handlers) HandleGetWork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	workID := extractWorkID(r.URL.Path)
	if workID == "" {
		http.Error(w, "work_id is required", http.StatusBadRequest)
		return
	}

	work, err := h.svc.GetWork(ctx, workID)
	if err != nil {
		if err == service.ErrWorkNotFound {
			http.Error(w, "work not found", http.StatusNotFound)
			return
		}
		slog.ErrorContext(ctx, "failed to get work", "error", err)
		http.Error(w, "failed to get work", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, work)
}

// HandleCancelWork handles POST /v1/work/{work_id}/cancel
func (h *Handlers) HandleCancelWork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	consumerID := r.Header.Get("X-Consumer-ID")
	if consumerID == "" {
		consumerID = "default_consumer" // TODO: Replace with actual auth
	}

	workID := extractWorkID(r.URL.Path)
	if workID == "" {
		http.Error(w, "work_id is required", http.StatusBadRequest)
		return
	}

	work, err := h.svc.CancelWork(ctx, workID, consumerID)
	if err != nil {
		if err == service.ErrWorkNotFound {
			http.Error(w, "work not found", http.StatusNotFound)
			return
		}
		slog.ErrorContext(ctx, "failed to cancel work", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, work)
}

// HandleBidSubmitted handles POST /internal/work/{work_id}/bids (internal endpoint)
func (h *Handlers) HandleBidSubmitted(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	workID := extractWorkID(r.URL.Path)
	if workID == "" {
		http.Error(w, "work_id is required", http.StatusBadRequest)
		return
	}

	var req struct {
		BidID string `json:"bid_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.svc.OnBidSubmitted(ctx, workID, req.BidID); err != nil {
		slog.ErrorContext(ctx, "failed to record bid", "error", err)
		http.Error(w, "failed to record bid", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// HandleCloseBidWindow handles POST /internal/work/{work_id}/close-bids (internal endpoint)
func (h *Handlers) HandleCloseBidWindow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	workID := extractWorkID(r.URL.Path)
	if workID == "" {
		http.Error(w, "work_id is required", http.StatusBadRequest)
		return
	}

	if err := h.svc.CloseBidWindow(ctx, workID); err != nil {
		slog.ErrorContext(ctx, "failed to close bid window", "error", err)
		http.Error(w, "failed to close bid window", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func extractWorkID(path string) string {
	// Extract work_id from paths like:
	// /v1/work/{work_id}
	// /v1/work/{work_id}/cancel
	// /internal/work/{work_id}/bids
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 3 {
		return ""
	}
	// parts[0] = "v1" or "internal"
	// parts[1] = "work"
	// parts[2] = work_id
	return parts[2]
}
