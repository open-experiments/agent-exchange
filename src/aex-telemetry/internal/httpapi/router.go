package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/parlakisik/agent-exchange/aex-telemetry/internal/service"
)

func NewRouter(svc *service.Service) http.Handler {
	mux := http.NewServeMux()

	// Log endpoints
	mux.HandleFunc("POST /v1/logs", svc.HandleIngestLogs)
	mux.HandleFunc("GET /v1/logs", svc.HandleQueryLogs)

	// Metrics endpoints
	mux.HandleFunc("POST /v1/metrics", svc.HandleIngestMetrics)
	mux.HandleFunc("GET /v1/metrics", svc.HandleQueryMetrics)

	// Trace endpoints
	mux.HandleFunc("POST /v1/spans", svc.HandleIngestSpans)
	mux.HandleFunc("GET /v1/traces/{trace_id}", svc.HandleGetTrace)

	// Stats endpoint
	mux.HandleFunc("GET /v1/stats", svc.HandleGetStats)

	// Health endpoints
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("GET /ready", readyHandler)

	return mux
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func readyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

