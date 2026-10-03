package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/parlakisik/agent-exchange/aex-gateway/internal/config"
	"github.com/parlakisik/agent-exchange/aex-gateway/internal/middleware"
)

// upstreamRecorder is a fake upstream that records the identity headers of
// the last request it received.
type upstreamRecorder struct {
	called     bool
	tenantID   string
	consumerID string
	hasAPIKey  bool
}

func newUpstream(t *testing.T, rec *upstreamRecorder) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.called = true
		rec.tenantID = r.Header.Get("X-Tenant-ID")
		rec.consumerID = r.Header.Get("X-Consumer-ID")
		rec.hasAPIKey = r.Header.Get("X-API-Key") != ""
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestServeHTTPIdentityHeaders(t *testing.T) {
	cases := []struct {
		name         string
		tenantID     string
		consumerID   string
		wantStatus   int
		wantUpstream bool
		wantTenant   string
	}{
		{"tenant set, client consumer header stripped", "tenant-a", "tenant-b", http.StatusOK, true, "tenant-a"},
		{"tenant set, no consumer header", "tenant-a", "", http.StatusOK, true, "tenant-a"},
		{"no tenant rejected", "", "tenant-b", http.StatusUnauthorized, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rec upstreamRecorder
			upstream := newUpstream(t, &rec)
			router := NewRouter(&config.Config{WorkPublisherURL: upstream.URL})

			req := httptest.NewRequest(http.MethodPost, "/v1/work", nil)
			req.Header.Set("X-API-Key", "client-key")
			req.Header.Set("X-Tenant-ID", "spoofed-tenant")
			if tc.consumerID != "" {
				req.Header.Set("X-Consumer-ID", tc.consumerID)
			}
			if tc.tenantID != "" {
				req = req.WithContext(context.WithValue(req.Context(), middleware.TenantIDKey, tc.tenantID))
			}

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if rec.called != tc.wantUpstream {
				t.Fatalf("upstream called = %v, want %v", rec.called, tc.wantUpstream)
			}
			if !tc.wantUpstream {
				return
			}
			if rec.tenantID != tc.wantTenant {
				t.Errorf("upstream X-Tenant-ID = %q, want %q", rec.tenantID, tc.wantTenant)
			}
			if rec.consumerID != "" {
				t.Errorf("upstream X-Consumer-ID = %q, want it stripped", rec.consumerID)
			}
			if rec.hasAPIKey {
				t.Error("upstream received X-API-Key, want it stripped")
			}
		})
	}
}
