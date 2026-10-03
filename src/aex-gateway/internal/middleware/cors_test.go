package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORS(t *testing.T) {
	const listed = "https://app.example.com"
	cases := []struct {
		name        string
		allowed     []string
		method      string
		origin      string
		wantStatus  int
		wantNext    bool
		wantOrigin  string
		wantCreds   string
		wantVary    string
		wantHeaders bool
	}{
		{"wildcard", []string{"*"}, http.MethodGet, "https://any.example.org", http.StatusOK, true, "*", "", "", true},
		{"wildcard no origin", []string{"*"}, http.MethodGet, "", http.StatusOK, true, "*", "", "", true},
		{"wildcard preflight", []string{"*"}, http.MethodOptions, "https://any.example.org", http.StatusNoContent, false, "*", "", "", true},
		{"wildcard among listed", []string{listed, "*"}, http.MethodGet, "https://any.example.org", http.StatusOK, true, "*", "", "", true},
		{"listed origin", []string{"https://other.example.com", listed}, http.MethodGet, listed, http.StatusOK, true, listed, "true", "Origin", true},
		{"listed preflight", []string{listed}, http.MethodOptions, listed, http.StatusNoContent, false, listed, "true", "Origin", true},
		{"unlisted origin", []string{listed}, http.MethodGet, "https://evil.example.net", http.StatusOK, true, "", "", "Origin", false},
		{"unlisted preflight", []string{listed}, http.MethodOptions, "https://evil.example.net", http.StatusNoContent, false, "", "", "Origin", false},
		{"no origin header", []string{listed}, http.MethodGet, "", http.StatusOK, true, "", "", "", false},
		{"empty list", nil, http.MethodGet, listed, http.StatusOK, true, "", "", "Origin", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})
			req := httptest.NewRequest(c.method, "/v1/work", nil)
			if c.origin != "" {
				req.Header.Set("Origin", c.origin)
			}
			if c.method == http.MethodOptions {
				req.Header.Set("Access-Control-Request-Method", "POST")
			}
			rec := httptest.NewRecorder()
			CORS(c.allowed)(next).ServeHTTP(rec, req)

			h := rec.Header()
			if rec.Code != c.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, c.wantStatus)
			}
			if called != c.wantNext {
				t.Errorf("next called = %v, want %v", called, c.wantNext)
			}
			if got := h.Get("Access-Control-Allow-Origin"); got != c.wantOrigin {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, c.wantOrigin)
			}
			if got := h.Get("Access-Control-Allow-Credentials"); got != c.wantCreds {
				t.Errorf("Access-Control-Allow-Credentials = %q, want %q", got, c.wantCreds)
			}
			if got := h.Get("Vary"); got != c.wantVary {
				t.Errorf("Vary = %q, want %q", got, c.wantVary)
			}
			wantMethods, wantAllow, wantMaxAge := "", "", ""
			if c.wantHeaders {
				wantMethods = "GET, POST, PUT, DELETE, OPTIONS"
				wantAllow = "Content-Type, Authorization, X-API-Key, X-Request-ID, X-Idempotency-Key"
				wantMaxAge = "86400"
			}
			if got := h.Get("Access-Control-Allow-Methods"); got != wantMethods {
				t.Errorf("Access-Control-Allow-Methods = %q, want %q", got, wantMethods)
			}
			if got := h.Get("Access-Control-Allow-Headers"); got != wantAllow {
				t.Errorf("Access-Control-Allow-Headers = %q, want %q", got, wantAllow)
			}
			if got := h.Get("Access-Control-Max-Age"); got != wantMaxAge {
				t.Errorf("Access-Control-Max-Age = %q, want %q", got, wantMaxAge)
			}
		})
	}
}
