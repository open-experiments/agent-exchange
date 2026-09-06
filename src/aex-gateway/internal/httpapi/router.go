package httpapi

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/parlakisik/agent-exchange/aex-gateway/internal/config"
	"github.com/parlakisik/agent-exchange/aex-gateway/internal/middleware"
	"github.com/parlakisik/agent-exchange/aex-gateway/internal/proxy"
)

func NewRouter(cfg *config.Config) http.Handler {
	mux := http.NewServeMux()

	// Create dependencies. API keys are validated against the identity
	// service unless the config asks for the in-memory table (tests).
	var apiKeyValidator middleware.APIKeyValidator
	if cfg.APIKeyValidator == "memory" {
		apiKeyValidator = middleware.NewInMemoryAPIKeyValidator()
	} else {
		apiKeyValidator = middleware.NewHTTPAPIKeyValidator(cfg.IdentityURL)
	}
	routeScopes, err := middleware.LoadRouteScopes(cfg.RouteScopesFile)
	if err != nil {
		log.Fatalf("aex-gateway: %v", err)
	}
	rateLimiter := middleware.NewRateLimiter(cfg.RedisURL, cfg.RateLimitPerMinute)
	proxyRouter := proxy.NewRouter(cfg)

	// Health endpoints (no auth required – needed for K8s probes)
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("GET /ready", readyHandler)

	// OPTIONS preflight handler (no auth required)
	mux.HandleFunc("OPTIONS /v1/", preflightHandler)

	// Authenticated info endpoint – served directly, not proxied
	infoHandler := applyMiddleware(http.HandlerFunc(infoHandlerFunc),
		middleware.RateLimit(rateLimiter),
		middleware.Auth(apiKeyValidator, cfg.JWTSecret),
	)
	mux.Handle("GET /v1/info", infoHandler)

	// API routes with middleware stack: rate limit, authenticate, then check
	// the route's scope against the grant before anything is proxied.
	apiHandler := applyMiddleware(proxyRouter,
		middleware.RateLimit(rateLimiter),
		middleware.Auth(apiKeyValidator, cfg.JWTSecret),
		middleware.RequireScope(routeScopes),
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

func infoHandlerFunc(w http.ResponseWriter, r *http.Request) {
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
