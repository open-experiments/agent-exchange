package httpapi

import (
	"net/http"

	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/service"
)

func NewRouter(svc *service.Service) http.Handler {
	h := NewHandlers(svc)
	mux := http.NewServeMux()

	// External API endpoints
	mux.HandleFunc("POST /v1/work", h.HandleSubmitWork)
	mux.HandleFunc("GET /v1/work/", h.HandleGetWork)      // /v1/work/{work_id}
	mux.HandleFunc("POST /v1/work/", dispatchWorkPOST(h)) // /v1/work/{work_id}/cancel

	// Internal API endpoints (called by other services)
	mux.HandleFunc("POST /internal/work/", dispatchInternalWorkPOST(h)) // /internal/work/{work_id}/bids or /close-bids

	// Health check
	mux.HandleFunc("GET /health", handleHealth)

	return mux
}

func dispatchWorkPOST(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only handle POST /v1/work/{work_id}/cancel
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check if path ends with /cancel
		if len(r.URL.Path) > 7 && r.URL.Path[len(r.URL.Path)-7:] == "/cancel" {
			h.HandleCancelWork(w, r)
			return
		}

		http.NotFound(w, r)
	}
}

func dispatchInternalWorkPOST(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check path suffix
		if len(r.URL.Path) > 5 && r.URL.Path[len(r.URL.Path)-5:] == "/bids" {
			h.HandleBidSubmitted(w, r)
			return
		}

		if len(r.URL.Path) > 11 && r.URL.Path[len(r.URL.Path)-11:] == "/close-bids" {
			h.HandleCloseBidWindow(w, r)
			return
		}

		http.NotFound(w, r)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"healthy","service":"aex-work-publisher"}`))
}
