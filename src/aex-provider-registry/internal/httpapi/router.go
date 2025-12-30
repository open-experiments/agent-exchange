package httpapi

import (
	"net/http"

	"github.com/parlakisik/agent-exchange/aex-provider-registry/internal/service"
)

func NewRouter(svc *service.Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/providers", svc.HandleRegisterProvider)
	mux.HandleFunc("GET /v1/providers/", svc.HandleGetProvider)
	mux.HandleFunc("POST /v1/subscriptions", svc.HandleCreateSubscription)
	mux.HandleFunc("GET /v1/subscriptions", svc.HandleListSubscriptions)
	mux.HandleFunc("GET /internal/v1/providers/subscribed", svc.HandleInternalSubscribed)
	mux.HandleFunc("GET /internal/v1/providers/validate-key", svc.HandleValidateAPIKey)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	return mux
}


