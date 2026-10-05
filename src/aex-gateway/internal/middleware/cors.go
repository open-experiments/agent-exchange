package middleware

import (
	"net/http"
	"strings"
)

var corsAllowedHeaders = strings.Join([]string{
	"Content-Type",
	"Authorization",
	"X-API-Key",
	"X-Request-ID",
	"X-Idempotency-Key",
}, ", ")

// CORS sets cross-origin headers for the configured origins. An entry of "*"
// allows every origin with a literal "Access-Control-Allow-Origin: *" (no
// credentials). Otherwise a request whose Origin is listed gets that origin
// echoed back, with credentials allowed and "Vary: Origin" so caches keep
// per-origin responses apart; an unlisted or missing Origin gets no CORS
// headers, which makes the browser refuse the response. An empty list allows
// no origin. Every OPTIONS request is answered 204 without reaching next, so
// preflights never hit authentication.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowAll := false
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o == "*" {
			allowAll = true
		}
		allowed[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			if allowAll {
				h.Set("Access-Control-Allow-Origin", "*")
				setCORSCommonHeaders(h)
			} else if origin := r.Header.Get("Origin"); origin != "" {
				h.Add("Vary", "Origin")
				if allowed[origin] {
					h.Set("Access-Control-Allow-Origin", origin)
					h.Set("Access-Control-Allow-Credentials", "true")
					setCORSCommonHeaders(h)
				}
			}

			// Handle preflight immediately
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func setCORSCommonHeaders(h http.Header) {
	h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	h.Set("Access-Control-Allow-Headers", corsAllowedHeaders)
	h.Set("Access-Control-Max-Age", "86400")
}
