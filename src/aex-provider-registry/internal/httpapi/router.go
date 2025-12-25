package httpapi

import (
	"net/http"

	"github.com/parlakisik/agent-exchange/aex-provider-registry/internal/service"
)

func NewRouter(svc *service.Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/providers", svc.HandleRegisterProvider)
	mux.HandleFunc("POST /v1/subscriptions", svc.HandleCreateSubscription)
	mux.HandleFunc("GET /internal/providers/subscribed", svc.HandleInternalSubscribed)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	return mux
}

