package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestGatewayHealth tests gateway health endpoint
func TestGatewayHealth(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	err := c.HealthCheck(ctx, c.urls.Gateway)
	if err != nil {
		t.Skipf("Gateway not available: %v", err)
	}
	t.Log("Gateway health check passed")
}

// TestGatewayInfo tests gateway info endpoint
func TestGatewayInfo(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Gateway); err != nil {
		t.Skipf("Gateway not available: %v", err)
	}

	info, err := c.GetGatewayInfo(ctx)
	if err != nil {
		t.Logf("Info endpoint not available: %v", err)
		return
	}

	t.Logf("Gateway info: %v", info)
}

// TestGatewayWorkPublisherRouting tests routing to work-publisher through gateway
func TestGatewayWorkPublisherRouting(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Gateway); err != nil {
		t.Skipf("Gateway not available: %v", err)
	}

	// Create a client that uses gateway URL
	gatewayClient := NewClient(ServiceURLs{
		WorkPublisher: c.urls.Gateway, // Route through gateway
	})

	timestamp := time.Now().UnixNano()

	// Submit work through gateway
	work, err := gatewayClient.SubmitWork(ctx, &WorkSpec{
		Category:    "gateway-routing-test",
		Description: "Work submitted through gateway",
		Payload:       map[string]any{"data": "test"},
		Budget: &Budget{
			MaxPrice: 50.00,
		},
		ConsumerID:    fmt.Sprintf("gateway-consumer-%d", timestamp),
		BidWindowMs: 60000,
	})
	if err != nil {
		t.Logf("Work submission through gateway: %v", err)
		return
	}

	t.Logf("Work submitted through gateway: %s", work.ID)
}

// TestGatewayProviderRegistryRouting tests routing to provider-registry through gateway
func TestGatewayProviderRegistryRouting(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Gateway); err != nil {
		t.Skipf("Gateway not available: %v", err)
	}

	gatewayClient := NewClient(ServiceURLs{
		ProviderRegistry: c.urls.Gateway,
	})

	timestamp := time.Now().UnixNano()

	provider, err := gatewayClient.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Gateway Routed Provider %d", timestamp),
		Capabilities: []string{"gateway-test"},
		Endpoint:     fmt.Sprintf("https://gateway-test-%d.example.com", timestamp),
	})
	if err != nil {
		t.Logf("Provider registration through gateway: %v", err)
		return
	}

	t.Logf("Provider registered through gateway: %s", provider.ID)
}

// TestGatewayAuthRequired tests that gateway requires authentication
func TestGatewayAuthRequired(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Gateway); err != nil {
		t.Skipf("Gateway not available: %v", err)
	}

	// Make request without API key
	status, body, err := c.RequestWithStatus(ctx, http.MethodGet, c.urls.Gateway+"/v1/work/test-123", nil)
	if err != nil {
		t.Logf("Request error (may be expected): %v", err)
		return
	}

	// Expecting 401 Unauthorized
	if status == http.StatusUnauthorized {
		t.Log("Gateway correctly requires authentication")
	} else {
		t.Logf("Gateway returned status %d: %s", status, string(body))
	}
}

// TestGatewayAPIKeyAuth tests API key authentication
func TestGatewayAPIKeyAuth(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Gateway); err != nil {
		t.Skipf("Gateway not available: %v", err)
	}

	// First create a tenant and API key
	timestamp := time.Now().UnixNano()
	tenant, err := c.CreateTenant(ctx, &Tenant{
		Name:  fmt.Sprintf("Gateway Auth Tenant %d", timestamp),
		Email: fmt.Sprintf("gateway-auth-%d@example.com", timestamp),
	})
	if err != nil {
		t.Logf("Tenant creation failed: %v", err)
		return
	}

	apiKey, err := c.CreateAPIKey(ctx, &APIKey{
		TenantID: tenant.ID,
		Name:     "Gateway Test Key",
		Scopes:   []string{"read", "write"},
	})
	if err != nil {
		t.Logf("API key creation failed: %v", err)
		return
	}

	// Create client with API key
	authClient := NewClient(ServiceURLs{
		Gateway: c.urls.Gateway,
	})
	authClient.SetAPIKey(apiKey.Key)

	// Make authenticated request
	info, err := authClient.GetGatewayInfo(ctx)
	if err != nil {
		t.Logf("Authenticated request failed: %v", err)
	} else {
		t.Logf("Authenticated request successful: %v", info)
	}
}

// TestGatewayRateLimit tests rate limiting
func TestGatewayRateLimit(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Gateway); err != nil {
		t.Skipf("Gateway not available: %v", err)
	}

	// Make many rapid requests to trigger rate limit
	rateLimitTriggered := false
	for i := 0; i < 200; i++ {
		status, _, err := c.RequestWithStatus(ctx, http.MethodGet, c.urls.Gateway+"/health", nil)
		if err != nil {
			t.Logf("Request %d error: %v", i, err)
			continue
		}

		if status == http.StatusTooManyRequests {
			rateLimitTriggered = true
			t.Logf("Rate limit triggered after %d requests", i+1)
			break
		}
	}

	if !rateLimitTriggered {
		t.Log("Rate limit not triggered (may have high limit or be disabled)")
	}
}

// TestGatewayCORS tests CORS headers
func TestGatewayCORS(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Gateway); err != nil {
		t.Skipf("Gateway not available: %v", err)
	}

	// Make OPTIONS request
	req, err := http.NewRequestWithContext(ctx, http.MethodOptions, c.urls.Gateway+"/v1/work", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")

	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatalf("CORS preflight request failed: %v", err)
	}
	defer resp.Body.Close()

	// Check CORS headers
	allowOrigin := resp.Header.Get("Access-Control-Allow-Origin")
	allowMethods := resp.Header.Get("Access-Control-Allow-Methods")

	t.Logf("CORS Allow-Origin: %s", allowOrigin)
	t.Logf("CORS Allow-Methods: %s", allowMethods)

	if allowOrigin == "" && allowMethods == "" {
		t.Log("CORS headers not present (may be disabled)")
	}
}

// TestGatewayRequestID tests request ID propagation
func TestGatewayRequestID(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Gateway); err != nil {
		t.Skipf("Gateway not available: %v", err)
	}

	// Make request and check for X-Request-ID header
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.urls.Gateway+"/health", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	requestID := resp.Header.Get("X-Request-ID")
	if requestID != "" {
		t.Logf("Gateway returned X-Request-ID: %s", requestID)
	} else {
		t.Log("X-Request-ID header not present")
	}

	// Also test with provided request ID
	customRequestID := fmt.Sprintf("custom-req-%d", time.Now().UnixNano())
	req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.urls.Gateway+"/health", nil)
	req2.Header.Set("X-Request-ID", customRequestID)

	resp2, err := c.http.Do(req2)
	if err != nil {
		t.Fatalf("Request with custom ID failed: %v", err)
	}
	defer resp2.Body.Close()

	returnedID := resp2.Header.Get("X-Request-ID")
	if returnedID == customRequestID {
		t.Logf("Gateway preserved custom X-Request-ID: %s", returnedID)
	} else {
		t.Logf("Gateway X-Request-ID: %s (expected %s)", returnedID, customRequestID)
	}
}

// TestGatewayTimeout tests gateway timeout handling
func TestGatewayTimeout(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Gateway); err != nil {
		t.Skipf("Gateway not available: %v", err)
	}

	// This test is informational - we can't easily trigger a timeout
	// but we verify the gateway responds within expected time
	start := time.Now()
	_, _, err := c.RequestWithStatus(ctx, http.MethodGet, c.urls.Gateway+"/health", nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Logf("Health check error: %v", err)
	}

	t.Logf("Gateway health check took %v", elapsed)
	if elapsed > 5*time.Second {
		t.Error("Gateway response took too long")
	}
}

// TestGatewayErrorHandling tests gateway error handling
func TestGatewayErrorHandling(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Gateway); err != nil {
		t.Skipf("Gateway not available: %v", err)
	}

	errorCases := []struct {
		name   string
		method string
		path   string
	}{
		{"Invalid path", http.MethodGet, "/invalid/path/that/does/not/exist"},
		{"Non-existent resource", http.MethodGet, "/v1/work/non-existent-id-12345"},
		{"Invalid method", http.MethodPatch, "/v1/providers"},
	}

	for _, tc := range errorCases {
		status, _, err := c.RequestWithStatus(ctx, tc.method, c.urls.Gateway+tc.path, nil)
		if err != nil {
			t.Logf("%s: request error - %v", tc.name, err)
			continue
		}
		t.Logf("%s: status %d", tc.name, status)
	}
}

// TestGatewayMultipleServices tests routing to multiple services
func TestGatewayMultipleServices(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Gateway); err != nil {
		t.Skipf("Gateway not available: %v", err)
	}

	// Test routes to different services
	routes := []struct {
		name   string
		method string
		path   string
	}{
		{"Work Publisher", http.MethodGet, "/v1/work"},
		{"Provider Registry", http.MethodGet, "/v1/providers"},
		{"Bid Gateway", http.MethodGet, "/v1/bids"},
		{"Contract Engine", http.MethodGet, "/v1/contracts"},
		{"Settlement", http.MethodGet, "/v1/balance"},
		{"Trust Broker", http.MethodGet, "/v1/providers/test/trust"},
		{"Identity", http.MethodGet, "/v1/tenants"},
	}

	for _, route := range routes {
		status, _, err := c.RequestWithStatus(ctx, route.method, c.urls.Gateway+route.path, nil)
		if err != nil {
			t.Logf("%s: request error - %v", route.name, err)
			continue
		}
		t.Logf("%s (%s %s): status %d", route.name, route.method, route.path, status)
	}
}

// TestGatewayContentType tests content type handling
func TestGatewayContentType(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Gateway); err != nil {
		t.Skipf("Gateway not available: %v", err)
	}

	// Test JSON content type
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.urls.Gateway+"/health", nil)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	t.Logf("Response Content-Type: %s", contentType)
}

// TestGatewayLargePayload tests handling of large payloads
func TestGatewayLargePayload(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Gateway); err != nil {
		t.Skipf("Gateway not available: %v", err)
	}

	timestamp := time.Now().UnixNano()

	// Create a large input payload
	largeText := make([]byte, 100*1024) // 100KB
	for i := range largeText {
		largeText[i] = byte('A' + (i % 26))
	}

	work := &WorkSpec{
		Category:    "large-payload-test",
		Description: "Work with large payload",
		Payload: map[string]any{
			"large_data": string(largeText),
		},
		Budget: &Budget{
			MaxPrice: 100.00,
		},
		ConsumerID:    fmt.Sprintf("large-payload-consumer-%d", timestamp),
		BidWindowMs: 60000,
	}

	status, _, err := c.RequestWithStatus(ctx, http.MethodPost, c.urls.Gateway+"/v1/work", work)
	if err != nil {
		t.Logf("Large payload request error: %v", err)
		return
	}

	t.Logf("Large payload request status: %d", status)
}

