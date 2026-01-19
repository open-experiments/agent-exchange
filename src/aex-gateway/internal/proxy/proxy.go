package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/parlakisik/agent-exchange/aex-gateway/internal/config"
	"github.com/parlakisik/agent-exchange/aex-gateway/internal/middleware"
)

type Router struct {
	routes  map[string]string
	proxies map[string]*httputil.ReverseProxy
}

func NewRouter(cfg *config.Config) *Router {
	routes := map[string]string{
		"/v1/work":          cfg.WorkPublisherURL,
		"/v1/providers":     cfg.ProviderRegistryURL,
		"/v1/subscriptions": cfg.ProviderRegistryURL,
		"/v1/capabilities":  cfg.ProviderRegistryURL,
		"/v1/usage":         cfg.SettlementURL,
		"/v1/balance":       cfg.SettlementURL,
		"/v1/deposits":      cfg.SettlementURL,
		"/v1/bids":          cfg.BidGatewayURL,
		"/v1/contracts":     cfg.ContractEngineURL,
		"/v1/tenants":       cfg.IdentityURL,
	}

	proxies := make(map[string]*httputil.ReverseProxy)
	for prefix, upstream := range routes {
		u, err := url.Parse(upstream)
		if err != nil {
			continue
		}
		proxies[prefix] = httputil.NewSingleHostReverseProxy(u)
	}

	return &Router{
		routes:  routes,
		proxies: proxies,
	}
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := req.URL.Path

	// Find matching route
	var matchedPrefix string
	var proxy *httputil.ReverseProxy

	for prefix := range r.routes {
		if strings.HasPrefix(path, prefix) {
			if len(prefix) > len(matchedPrefix) {
				matchedPrefix = prefix
				proxy = r.proxies[prefix]
			}
		}
	}

	if proxy == nil {
		respondError(w, http.StatusNotFound, "endpoint_not_found", "Endpoint not found", req)
		return
	}

	// Add internal headers
	tenantID := middleware.GetTenantID(req.Context())
	requestID := middleware.GetRequestID(req.Context())

	req.Header.Set("X-Tenant-ID", tenantID)
	req.Header.Set("X-Request-ID", requestID)

	// Remove external auth headers (already validated)
	req.Header.Del("X-API-Key")
	req.Header.Del("Authorization")

	// Proxy the request
	proxy.ServeHTTP(w, req)
}

func respondError(w http.ResponseWriter, status int, code, message string, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":       code,
			"message":    message,
			"request_id": middleware.GetRequestID(r.Context()),
		},
	})
}
