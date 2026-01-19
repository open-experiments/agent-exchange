package httpapi

import (
	"net/http"

	"github.com/parlakisik/agent-exchange/aex-provider-registry/internal/service"
)

func NewRouter(svc *service.Service) http.Handler {
	mux := http.NewServeMux()

	// Provider management
	mux.HandleFunc("POST /v1/providers", svc.HandleRegisterProvider)
	mux.HandleFunc("GET /v1/providers", svc.HandleListAllProviders)
	mux.HandleFunc("GET /v1/providers/search", svc.HandleSearchProviders)

	// Provider details (must come after /search to avoid conflicts)
	mux.HandleFunc("GET /v1/providers/{provider_id}/a2a", svc.HandleGetProviderWithA2A)
	mux.HandleFunc("POST /v1/providers/{provider_id}/agent-card", svc.HandleRegisterAgentCard)
	mux.HandleFunc("GET /v1/providers/{provider_id}", svc.HandleGetProvider)

	// Legacy single provider endpoint (fallback)
	mux.HandleFunc("GET /v1/providers/", svc.HandleGetProvider)

	// Subscriptions
	mux.HandleFunc("POST /v1/subscriptions", svc.HandleCreateSubscription)
	mux.HandleFunc("GET /v1/subscriptions", svc.HandleListSubscriptions)

	// Internal APIs
	mux.HandleFunc("GET /internal/v1/providers/subscribed", svc.HandleInternalSubscribed)
	mux.HandleFunc("GET /internal/v1/providers/validate-key", svc.HandleValidateAPIKey)

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	return mux
}
