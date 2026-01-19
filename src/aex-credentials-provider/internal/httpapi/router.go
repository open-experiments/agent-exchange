package httpapi

import (
	"net/http"

	"github.com/parlakisik/agent-exchange/aex-credentials-provider/internal/service"
)

// NewRouter creates the HTTP router with all routes.
func NewRouter(svc *service.Service) *http.ServeMux {
	h := NewHandlers(svc)
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /health", h.Health)

	// Agent Card (A2A discovery)
	mux.HandleFunc("GET /.well-known/agent.json", h.AgentCard)

	// Payment Methods API
	mux.HandleFunc("GET /v1/users/{userID}/payment-methods", h.GetPaymentMethods)
	mux.HandleFunc("POST /v1/users/{userID}/payment-methods", h.AddPaymentMethod)
	mux.HandleFunc("POST /v1/users/{userID}/tokens", h.GetPaymentToken)

	// Payment Processing
	mux.HandleFunc("POST /v1/payments", h.ProcessPayment)

	// Receipts
	mux.HandleFunc("GET /v1/receipts/{receiptID}", h.GetReceipt)

	return mux
}
