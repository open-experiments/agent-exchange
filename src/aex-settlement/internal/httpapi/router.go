package httpapi

import (
	"net/http"

	"github.com/parlakisik/agent-exchange/aex-settlement/internal/service"
)

func NewRouter(svc *service.Service) http.Handler {
	h := NewHandlers(svc)
	mux := http.NewServeMux()

	// Public API
	mux.HandleFunc("/v1/usage", dispatchUsage(h))
	mux.HandleFunc("/v1/usage/transactions", h.GetTransactions)
	mux.HandleFunc("/v1/balance", h.GetBalance)
	mux.HandleFunc("/v1/deposits", h.ProcessDeposit)

	// Internal API
	mux.HandleFunc("/internal/settlement/complete", h.ProcessContractCompletion)

	// Health
	mux.HandleFunc("/health", h.Health)

	return mux
}

func dispatchUsage(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.GetUsage(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
