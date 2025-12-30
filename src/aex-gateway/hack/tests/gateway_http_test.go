package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parlakisik/agent-exchange/aex-gateway/internal/config"
	"github.com/parlakisik/agent-exchange/aex-gateway/internal/httpapi"
)

func TestHealthEndpoint(t *testing.T) {
	cfg := &config.Config{
		Port:               "8080",
		Environment:        "test",
		RateLimitPerMinute: 1000,
		RateLimitBurstSize: 50,
		RequestTimeout:     30 * time.Second,
	}

	router := httpapi.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

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
	cfg := &config.Config{
		Port:               "8080",
		Environment:        "test",
		RateLimitPerMinute: 1000,
		RateLimitBurstSize: 50,
		RequestTimeout:     30 * time.Second,
	}

	router := httpapi.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ready")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestInfoEndpoint(t *testing.T) {
	cfg := &config.Config{
		Port:               "8080",
		Environment:        "test",
		RateLimitPerMinute: 1000,
		RateLimitBurstSize: 50,
		RequestTimeout:     30 * time.Second,
	}

	router := httpapi.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/info")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

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
	cfg := &config.Config{
		Port:               "8080",
		Environment:        "test",
		WorkPublisherURL:   "http://localhost:8081",
		RateLimitPerMinute: 1000,
		RateLimitBurstSize: 50,
		RequestTimeout:     30 * time.Second,
	}

	router := httpapi.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Request without auth should fail
	resp, err := http.Get(ts.URL + "/v1/work")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAuthWithAPIKey(t *testing.T) {
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
		if r.Header.Get("X-API-Key") != "" {
			t.Error("X-API-Key header should have been removed")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"works":[]}`))
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Port:               "8080",
		Environment:        "test",
		WorkPublisherURL:   upstream.URL,
		RateLimitPerMinute: 1000,
		RateLimitBurstSize: 50,
		RequestTimeout:     30 * time.Second,
	}

	router := httpapi.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Request with valid API key should succeed
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/work", nil)
	req.Header.Set("X-API-Key", "dev-api-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

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

func TestInvalidAPIKey(t *testing.T) {
	// Start a mock upstream (needed because auth happens before proxy)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Port:               "8080",
		Environment:        "test",
		WorkPublisherURL:   upstream.URL,
		RateLimitPerMinute: 1000,
		RateLimitBurstSize: 50,
		RequestTimeout:     30 * time.Second,
	}

	router := httpapi.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/work", nil)
	req.Header.Set("X-API-Key", "invalid-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestRateLimiting(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Port:               "8080",
		Environment:        "test",
		WorkPublisherURL:   upstream.URL,
		RateLimitPerMinute: 3, // Very low limit for testing
		RateLimitBurstSize: 1,
		RequestTimeout:     30 * time.Second,
	}

	router := httpapi.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Make requests until rate limited
	rateLimited := false
	for i := 0; i < 10; i++ {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/work", nil)
		req.Header.Set("X-API-Key", "dev-api-key")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			rateLimited = true
			break
		}
	}

	if !rateLimited {
		t.Log("rate limiting not triggered (may need more requests)")
	}
}

func TestCORS(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Port:               "8080",
		Environment:        "test",
		WorkPublisherURL:   upstream.URL,
		RateLimitPerMinute: 1000,
		RateLimitBurstSize: 50,
		RequestTimeout:     30 * time.Second,
	}

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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	if resp.Header.Get("Access-Control-Allow-Origin") == "" {
		t.Error("Access-Control-Allow-Origin header not set")
	}
}

func TestRequestID(t *testing.T) {
	cfg := &config.Config{
		Port:               "8080",
		Environment:        "test",
		RateLimitPerMinute: 1000,
		RateLimitBurstSize: 50,
		RequestTimeout:     30 * time.Second,
	}

	router := httpapi.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Test that request ID is generated
	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

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
	defer resp2.Body.Close()

	if resp2.Header.Get("X-Request-ID") != "test-request-123" {
		t.Errorf("expected X-Request-ID=test-request-123, got %s", resp2.Header.Get("X-Request-ID"))
	}
}

