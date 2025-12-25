package httpapi

import (
	"net/http"

	"github.com/parlakisik/agent-exchange/aex-contract-engine/internal/service"
)

func NewRouter(svc *service.Service) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /v1/work/", func(w http.ResponseWriter, r *http.Request) {
		if hasSuffix(r.URL.Path, "/award") {
			svc.HandleAward(w, r)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("GET /v1/contracts/", svc.HandleGetContract)
	mux.HandleFunc("POST /v1/contracts/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case hasSuffix(r.URL.Path, "/progress"):
			svc.HandleProgress(w, r)
		case hasSuffix(r.URL.Path, "/complete"):
			svc.HandleComplete(w, r)
		case hasSuffix(r.URL.Path, "/fail"):
			svc.HandleFail(w, r)
		default:
			http.NotFound(w, r)
		}
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	return mux
}

func hasSuffix(s, suf string) bool {
	if len(suf) > len(s) {
		return false
	}
	return s[len(s)-len(suf):] == suf
}
