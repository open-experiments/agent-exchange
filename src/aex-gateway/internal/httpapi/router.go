package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/parlakisik/agent-exchange/aex-gateway/internal/config"
	"github.com/parlakisik/agent-exchange/aex-gateway/internal/middleware"
	"github.com/parlakisik/agent-exchange/aex-gateway/internal/proxy"
)

func NewRouter(cfg *config.Config) http.Handler {
	mux := http.NewServeMux()

	// Create dependencies
	apiKeyValidator := middleware.NewInMemoryAPIKeyValidator()
	rateLimiter := middleware.NewRateLimiter(cfg.RateLimitPerMinute, cfg.RateLimitBurstSize)
	proxyRouter := proxy.NewRouter(cfg)

	// Health endpoints (no auth required)
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("GET /ready", readyHandler)
	mux.HandleFunc("GET /v1/info", infoHandler)

	// OPTIONS preflight handler (no auth required)
	mux.HandleFunc("OPTIONS /v1/", preflightHandler)

	// API routes with middleware stack
	apiHandler := applyMiddleware(proxyRouter,
		middleware.RateLimit(rateLimiter),
		middleware.Auth(apiKeyValidator),
	)

	// Mount API handler for all /v1/* paths
	mux.Handle("/v1/", apiHandler)

	// Apply global middleware
	handler := applyMiddleware(mux,
		middleware.Timeout(cfg.RequestTimeout),
		middleware.CORSAllowAll,
		middleware.Recovery,
		middleware.Logging,
		middleware.RequestID,
	)

	return handler
}

func applyMiddleware(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	// Apply in reverse order so first middleware is outermost
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
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

func infoHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"name":    "Agent Exchange Gateway",
		"version": "1.0.0",
		"phase":   "A",
	})
}

func preflightHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

