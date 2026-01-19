package httpapi

import (
	"net/http"

	"github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/service"
)

func NewRouter(svc *service.Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/v1/evaluate", svc.HandleEvaluate)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	return mux
}
