package httpapi

import (
	"net/http"
	"strings"

	"github.com/parlakisik/agent-exchange/aex-identity/internal/service"
)

func NewRouter(svc *service.Service) http.Handler {
	mux := http.NewServeMux()

	// External
	mux.HandleFunc("POST /v1/tenants", svc.HandleCreateTenant)
	mux.HandleFunc("GET /v1/tenants/", dispatchTenantGET(svc))       // /v1/tenants/{id} OR /v1/tenants/{id}/api-keys
	mux.HandleFunc("POST /v1/tenants/", dispatchTenantPOST(svc))     // /v1/tenants/{id}/suspend|activate|api-keys
	mux.HandleFunc("DELETE /v1/tenants/", dispatchTenantDELETE(svc)) // /v1/tenants/{id}/api-keys/{key_id}

	// Internal
	mux.HandleFunc("POST /internal/v1/apikeys/validate", svc.HandleValidateAPIKey)
	mux.HandleFunc("GET /internal/v1/tenants/", dispatchInternalQuotas(svc))

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	return mux
}

func dispatchTenantGET(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/api-keys"):
			svc.HandleListAPIKeys(w, r)
		default:
			svc.HandleGetTenant(w, r)
		}
	}
}

func dispatchTenantPOST(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			switch {
			case strings.HasSuffix(r.URL.Path, "/suspend"):
				svc.HandleSuspendTenant(w, r)
			case strings.HasSuffix(r.URL.Path, "/activate"):
				svc.HandleActivateTenant(w, r)
			case strings.HasSuffix(r.URL.Path, "/api-keys"):
				svc.HandleCreateAPIKey(w, r)
			default:
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	}
}

func dispatchTenantDELETE(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/api-keys/") {
			http.NotFound(w, r)
			return
		}
		svc.HandleRevokeAPIKey(w, r)
	}
}

func dispatchInternalQuotas(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/quotas") {
			http.NotFound(w, r)
			return
		}
		svc.HandleGetQuotas(w, r)
	}
}
