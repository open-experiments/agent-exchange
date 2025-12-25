package httpapi

import (
	"net/http"

	"github.com/parlakisik/agent-exchange/aex-trust-broker/internal/service"
)

func NewRouter(svc *service.Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/providers/", svc.HandleGetTrust) // /v1/providers/{id}/trust
	mux.HandleFunc("POST /internal/trust/batch", svc.HandleBatchTrust)
	mux.HandleFunc("POST /internal/outcomes", svc.HandleRecordOutcome)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	return mux
}

