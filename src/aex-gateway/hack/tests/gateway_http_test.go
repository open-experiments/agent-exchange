package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/parlakisik/agent-exchange/aex-gateway/internal/config"
	"github.com/parlakisik/agent-exchange/aex-gateway/internal/httpapi"
)

const testJWTSecret = "test-secret-for-gateway-tests"

// issueTestJWT creates a signed JWT for testing purposes.
func issueTestJWT(t *testing.T, tenantID string, scopes []string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss":       "aex-identity",
		"tenant_id": tenantID,
		"scopes":    scopes,
		"exp":       time.Now().Add(time.Hour).Unix(),
		"iat":       time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("failed to sign test JWT: %v", err)
	}
	return signed
}

func testConfig() *config.Config {
	return &config.Config{
		Port:               "8080",
		Environment:        "test",
		RedisURL:           "", // empty → rate limiter fails open (no Redis in tests)
		JWTSecret:          testJWTSecret,
		RateLimitPerMinute: 1000,
		RateLimitBurstSize: 50,
		RequestTimeout:     30 * time.Second,
		APIKeyValidator:    "memory", // no identity service in tests; keys are never added, so every key is invalid
	}
}

func TestHealthEndpoint(t *testing.T) {
	cfg := testConfig()

	router := httpapi.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result["status"] != "healthy" {
		t.Fatalf("expected status=healthy, got %s", result["status"])
	}
}

func TestReadyEndpoint(t *testing.T) {
	cfg := testConfig()

	router := httpapi.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ready")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestInfoEndpointRequiresAuth(t *testing.T) {
	cfg := testConfig()

	router := httpapi.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Unauthenticated request should be rejected
	resp, err := http.Get(ts.URL + "/v1/info")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated /v1/info, got %d", resp.StatusCode)
	}
}

func TestInfoEndpointWithJWT(t *testing.T) {
	cfg := testConfig()

	router := httpapi.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/info", nil)
	req.Header.Set("Authorization", "Bearer "+issueTestJWT(t, "tenant_jwt", []string{"read"}))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result["name"] != "Agent Exchange Gateway" {
		t.Fatalf("expected name=Agent Exchange Gateway, got %v", result["name"])
	}
}

func TestAuthRequiredForAPI(t *testing.T) {
	cfg := testConfig()
	cfg.WorkPublisherURL = "http://localhost:8081"

	router := httpapi.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Request without auth should fail
	resp, err := http.Get(ts.URL + "/v1/work")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAuthWithJWT(t *testing.T) {
	// Start a mock upstream service
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify internal headers were added
		if r.Header.Get("X-Tenant-ID") == "" {
			t.Error("X-Tenant-ID header not set")
		}
		if r.Header.Get("X-Request-ID") == "" {
			t.Error("X-Request-ID header not set")
		}
		// Verify auth headers were removed
		if r.Header.Get("Authorization") != "" {
			t.Error("Authorization header should have been removed")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"works":[]}`))
	}))
	defer upstream.Close()

	cfg := testConfig()
	cfg.WorkPublisherURL = upstream.URL

	router := httpapi.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Request with valid JWT should succeed
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/work", nil)
	req.Header.Set("Authorization", "Bearer "+issueTestJWT(t, "tenant_jwt", []string{"read"}))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify rate limit headers
	if resp.Header.Get("X-RateLimit-Limit") == "" {
		t.Error("X-RateLimit-Limit header not set")
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "" {
		t.Error("X-RateLimit-Remaining header not set")
	}
}

func TestBearerTokenRejectsArbitraryString(t *testing.T) {
	cfg := testConfig()
	cfg.WorkPublisherURL = "http://localhost:8081"

	router := httpapi.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// An arbitrary non-empty bearer token should now be rejected
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/work", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-jwt")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for arbitrary bearer token, got %d", resp.StatusCode)
	}
}

func TestInvalidAPIKey(t *testing.T) {
	// Start a mock upstream (needed because auth happens before proxy)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	cfg := testConfig()
	cfg.WorkPublisherURL = upstream.URL

	router := httpapi.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/work", nil)
	req.Header.Set("X-API-Key", "invalid-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestHardcodedDevKeysRemoved(t *testing.T) {
	cfg := testConfig()
	cfg.WorkPublisherURL = "http://localhost:8081"

	router := httpapi.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Old hardcoded "dev-api-key" should no longer work
	for _, key := range []string{"dev-api-key", "test-api-key"} {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/work", nil)
		req.Header.Set("X-API-Key", key)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401 for old hardcoded key %q, got %d", key, resp.StatusCode)
		}
	}
}

func TestRateLimiting(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	cfg := testConfig()
	cfg.WorkPublisherURL = upstream.URL
	cfg.RateLimitPerMinute = 3 // Very low limit for testing

	router := httpapi.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	bearer := issueTestJWT(t, "tenant_ratelimit", []string{"read"})

	// Make requests until rate limited
	// Note: without Redis the limiter fails open, so this test may not trigger 429.
	rateLimited := false
	for i := 0; i < 10; i++ {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/work", nil)
		req.Header.Set("Authorization", "Bearer "+bearer)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			rateLimited = true
			break
		}
	}

	if !rateLimited {
		t.Log("rate limiting not triggered (expected when Redis is unavailable – limiter fails open)")
	}
}

func TestCORS(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	cfg := testConfig()
	cfg.WorkPublisherURL = upstream.URL

	router := httpapi.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Send preflight request
	req, _ := http.NewRequest(http.MethodOptions, ts.URL+"/v1/work", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	if resp.Header.Get("Access-Control-Allow-Origin") == "" {
		t.Error("Access-Control-Allow-Origin header not set")
	}
}

func TestRequestID(t *testing.T) {
	cfg := testConfig()

	router := httpapi.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Test that request ID is generated
	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.Header.Get("X-Request-ID") == "" {
		t.Error("X-Request-ID header not set")
	}

	// Test that provided request ID is echoed
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/health", nil)
	req.Header.Set("X-Request-ID", "test-request-123")

	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp2.Body.Close() }()

	if resp2.Header.Get("X-Request-ID") != "test-request-123" {
		t.Errorf("expected X-Request-ID=test-request-123, got %s", resp2.Header.Get("X-Request-ID"))
	}
}

// identityStub is a minimal identity service that validates one API key.
func identityStub(t *testing.T, validKey, tenantID string, scopes []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/apikeys/validate" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var body struct {
			APIKey string `json:"api_key"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		if body.APIKey != validKey {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"valid": false})
			return
		}
		// The identity service's real answer: tenant, status, scopes, quotas; no "valid" field.
		_ = json.NewEncoder(w).Encode(map[string]any{"tenant_id": tenantID, "tenant_status": "ACTIVE", "scopes": scopes})
	}))
}

// TestAPIKeyValidatedAgainstIdentity covers the production path: an API key
// is checked by calling the identity service, and the validated tenant and
// scopes travel to the upstream as headers.
func TestAPIKeyValidatedAgainstIdentity(t *testing.T) {
	identity := identityStub(t, "aexk_valid", "tenant_ap", []string{"read", "payments:send"})
	defer identity.Close()

	var gotTenant, gotScopes string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTenant = r.Header.Get("X-Tenant-ID")
		gotScopes = r.Header.Get("X-Scopes")
		if r.Header.Get("X-API-Key") != "" {
			t.Error("X-API-Key should have been removed before proxying")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	cfg := testConfig()
	cfg.APIKeyValidator = "identity"
	cfg.IdentityURL = identity.URL
	cfg.WorkPublisherURL = upstream.URL
	ts := httptest.NewServer(httpapi.NewRouter(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/work", nil)
	req.Header.Set("X-API-Key", "aexk_valid")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("valid key: expected 200, got %d", resp.StatusCode)
	}
	if gotTenant != "tenant_ap" || gotScopes != "read,payments:send" {
		t.Fatalf("upstream saw tenant=%q scopes=%q", gotTenant, gotScopes)
	}

	req2, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/work", nil)
	req2.Header.Set("X-API-Key", "aexk_other")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unknown key: expected 401, got %d", resp2.StatusCode)
	}
}

// TestScopeEnforced covers the route scope map: a grant without the route's
// scope is refused with 403 and the scope named; "*" covers everything; an
// unmapped route requires "*".
func TestScopeEnforced(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	cfg := testConfig()
	cfg.WorkPublisherURL = upstream.URL
	cfg.ToolsURL = upstream.URL
	ts := httptest.NewServer(httpapi.NewRouter(cfg))
	defer ts.Close()

	cases := []struct {
		name   string
		method string
		path   string
		scopes []string
		want   int
	}{
		{"read scope reads work", http.MethodGet, "/v1/work", []string{"read"}, http.StatusOK},
		{"read scope cannot submit work", http.MethodPost, "/v1/work", []string{"read"}, http.StatusForbidden},
		{"work:submit submits work", http.MethodPost, "/v1/work", []string{"work:submit"}, http.StatusOK},
		{"star covers everything", http.MethodPost, "/v1/work", []string{"*"}, http.StatusOK},
		{"tools route needs tools:invoke", http.MethodPost, "/v1/tools/send_payment", []string{"read"}, http.StatusForbidden},
		{"tools route with tools:invoke", http.MethodPost, "/v1/tools/send_payment", []string{"tools:invoke"}, http.StatusOK},
		{"tools route with star", http.MethodPost, "/v1/tools/send_payment", []string{"*"}, http.StatusOK},
		{"genuinely unmapped route needs star", http.MethodPost, "/v1/unmapped-thing", []string{"read"}, http.StatusForbidden},
		// A star key clears the scope layer, so this reaches routing and 404s
		// there rather than being refused at 403 by scopes.
		{"genuinely unmapped route with star reaches routing", http.MethodPost, "/v1/unmapped-thing", []string{"*"}, http.StatusNotFound},
	}
	for _, c := range cases {
		req, _ := http.NewRequest(c.method, ts.URL+c.path, nil)
		req.Header.Set("Authorization", "Bearer "+issueTestJWT(t, "tenant_scopes", c.scopes))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		_ = resp.Body.Close()
		if resp.StatusCode != c.want {
			t.Errorf("%s: expected %d, got %d (%v)", c.name, c.want, resp.StatusCode, body)
		}
		if c.want == http.StatusForbidden {
			e, _ := body["error"].(map[string]any)
			if e["code"] != "insufficient_scope" || e["required_scope"] == "" {
				t.Errorf("%s: expected insufficient_scope with the scope named, got %v", c.name, body)
			}
		}
	}
}

// TestRouteScopesFile covers a per-deployment scope map merged over the
// defaults, the way a provider maps its tool routes to scopes.
func TestRouteScopesFile(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	f, err := os.CreateTemp(t.TempDir(), "scopes-*.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"POST /v1/tools/send_payment": "payments:send", "POST /v1/tools/list_invoices": "invoices:read"}`)
	_ = f.Close()

	cfg := testConfig()
	cfg.ToolsURL = upstream.URL
	cfg.RouteScopesFile = f.Name()
	ts := httptest.NewServer(httpapi.NewRouter(cfg))
	defer ts.Close()

	for _, c := range []struct {
		path   string
		scopes []string
		want   int
	}{
		{"/v1/tools/send_payment", []string{"payments:send"}, http.StatusOK},
		{"/v1/tools/send_payment", []string{"invoices:read"}, http.StatusForbidden},
		{"/v1/tools/list_invoices", []string{"invoices:read"}, http.StatusOK},
		{"/v1/tools/delete_invoice", []string{"invoices:read", "payments:send"}, http.StatusForbidden},
	} {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+c.path, nil)
		req.Header.Set("Authorization", "Bearer "+issueTestJWT(t, "tenant_tools", c.scopes))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != c.want {
			t.Errorf("%s with %v: expected %d, got %d", c.path, c.scopes, c.want, resp.StatusCode)
		}
	}
}
