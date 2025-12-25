package httpapi

import (
	"github.com/parlakisik/agent-exchange/aex-bid-gateway/internal/service"
	"net/http"
)

func NewRouter(svc *service.Service) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /v1/bids", svc.HandleSubmitBid)
	mux.HandleFunc("GET /internal/bids", svc.HandleInternalListBids)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	return mux
}
