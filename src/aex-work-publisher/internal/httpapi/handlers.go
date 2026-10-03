package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

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

	consumerID, ok := requireConsumerID(w, r)
	if !ok {
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "failed to read request")
		return
	}
	defer func() { _ = r.Body.Close() }()

	var req model.WorkSubmission
	if err := json.Unmarshal(body, &req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	resp, err := h.svc.PublishWork(ctx, consumerID, req)
	if err != nil {
		if errors.Is(err, service.ErrInvalidWorkSpec) {
			respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
		slog.ErrorContext(ctx, "failed to publish work", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to publish work")
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

// HandleGetWork handles GET /v1/work/{work_id}
func (h *Handlers) HandleGetWork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	workID := extractWorkID(r.URL.Path)
	if workID == "" {
		respondError(w, http.StatusBadRequest, "WORK_ID_REQUIRED", "work_id is required")
		return
	}

	work, err := h.svc.GetWork(ctx, workID)
	if err != nil {
		if errors.Is(err, service.ErrWorkNotFound) {
			respondError(w, http.StatusNotFound, "NOT_FOUND", "work not found")
			return
		}
		slog.ErrorContext(ctx, "failed to get work", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get work")
		return
	}

	writeJSON(w, http.StatusOK, work)
}

// HandleCancelWork handles POST /v1/work/{work_id}/cancel
func (h *Handlers) HandleCancelWork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	consumerID, ok := requireConsumerID(w, r)
	if !ok {
		return
	}

	workID := extractWorkID(r.URL.Path)
	if workID == "" {
		respondError(w, http.StatusBadRequest, "WORK_ID_REQUIRED", "work_id is required")
		return
	}

	work, err := h.svc.CancelWork(ctx, workID, consumerID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWorkNotFound):
			respondError(w, http.StatusNotFound, "NOT_FOUND", "work not found")
		case errors.Is(err, service.ErrNotAuthorized),
			errors.Is(err, service.ErrInvalidState),
			errors.Is(err, service.ErrVersionConflict):
			respondError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		default:
			slog.ErrorContext(ctx, "failed to cancel work", "error", err)
			respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to cancel work")
		}
		return
	}

	writeJSON(w, http.StatusOK, work)
}

// HandleBidSubmitted handles POST /internal/work/{work_id}/bids (internal endpoint)
func (h *Handlers) HandleBidSubmitted(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	workID := extractWorkID(r.URL.Path)
	if workID == "" {
		respondError(w, http.StatusBadRequest, "WORK_ID_REQUIRED", "work_id is required")
		return
	}

	var req struct {
		BidID string `json:"bid_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := h.svc.OnBidSubmitted(ctx, workID, req.BidID); err != nil {
		slog.ErrorContext(ctx, "failed to record bid", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to record bid")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// HandleCloseBidWindow handles POST /internal/work/{work_id}/close-bids (internal endpoint)
func (h *Handlers) HandleCloseBidWindow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	workID := extractWorkID(r.URL.Path)
	if workID == "" {
		respondError(w, http.StatusBadRequest, "WORK_ID_REQUIRED", "work_id is required")
		return
	}

	if err := h.svc.CloseBidWindow(ctx, workID); err != nil {
		slog.ErrorContext(ctx, "failed to close bid window", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to close bid window")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// requireConsumerID resolves the consumer that owns the request. The gateway
// authenticates the caller and sets X-Tenant-ID to the validated tenant,
// overwriting anything the client sent, so it takes precedence. X-Consumer-ID
// is accepted for direct service-to-service calls that bypass the gateway.
// A request with neither is rejected with 401: the consumer is the tenant that
// settlement charges, so there is no safe default.
func requireConsumerID(w http.ResponseWriter, r *http.Request) (string, bool) {
	consumerID := strings.TrimSpace(r.Header.Get("X-Tenant-ID"))
	if consumerID == "" {
		consumerID = strings.TrimSpace(r.Header.Get("X-Consumer-ID"))
	}
	if consumerID == "" {
		respondError(w, http.StatusUnauthorized, "CONSUMER_ID_REQUIRED",
			"consumer identity is required (X-Tenant-ID or X-Consumer-ID header)")
		return "", false
	}
	return consumerID, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
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
