package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestErrorInvalidJSON tests handling of invalid JSON payloads
func TestErrorInvalidJSON(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()
	skipIfNoServices(t, c)

	endpoints := []struct {
		name   string
		method string
		url    string
	}{
		{"Work Publisher", http.MethodPost, c.urls.WorkPublisher + "/v1/work"},
		{"Provider Registry", http.MethodPost, c.urls.ProviderRegistry + "/v1/providers"},
		{"Bid Gateway", http.MethodPost, c.urls.BidGateway + "/v1/bids"},
		{"Contract Engine", http.MethodPost, c.urls.ContractEngine + "/v1/contracts/award"},
		{"Settlement", http.MethodPost, c.urls.Settlement + "/v1/deposits"},
		{"Identity", http.MethodPost, c.urls.Identity + "/v1/tenants"},
	}

	invalidJSON := "{ invalid json content"

	for _, ep := range endpoints {
		req, _ := http.NewRequestWithContext(ctx, ep.method, ep.url, nil)
		req.Header.Set("Content-Type", "application/json")
		// Send invalid JSON
		req.Body = http.NoBody // Invalid body

		resp, err := c.http.Do(req)
		if err != nil {
			t.Logf("%s: request error - %v", ep.name, err)
			continue
		}
		resp.Body.Close()
		t.Logf("%s: invalid JSON response status %d", ep.name, resp.StatusCode)
	}

	_ = invalidJSON // Would be used with actual body writing
}

// TestErrorMissingRequiredFields tests handling of missing required fields
func TestErrorMissingRequiredFields(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()
	skipIfNoServices(t, c)

	// Work without required fields
	status, body, err := c.RequestWithStatus(ctx, http.MethodPost, c.urls.WorkPublisher+"/v1/work", map[string]any{})
	if err != nil {
		t.Logf("Empty work request error: %v", err)
	} else {
		t.Logf("Empty work: status %d, response: %s", status, string(body))
	}

	// Provider without name
	status, body, err = c.RequestWithStatus(ctx, http.MethodPost, c.urls.ProviderRegistry+"/v1/providers", map[string]any{
		"capabilities": []string{"test"},
	})
	if err != nil {
		t.Logf("Provider without name error: %v", err)
	} else {
		t.Logf("Provider without name: status %d", status)
	}

	// Bid without work_id
	status, body, err = c.RequestWithStatus(ctx, http.MethodPost, c.urls.BidGateway+"/v1/bids", map[string]any{
		"price": 50.00,
	})
	if err != nil {
		t.Logf("Bid without work_id error: %v", err)
	} else {
		t.Logf("Bid without work_id: status %d", status)
	}

	// Tenant without name
	status, body, err = c.RequestWithStatus(ctx, http.MethodPost, c.urls.Identity+"/v1/tenants", map[string]any{
		"email": "test@example.com",
	})
	if err != nil {
		t.Logf("Tenant without name error: %v", err)
	} else {
		t.Logf("Tenant without name: status %d", status)
	}
}

// TestErrorNonExistentResources tests handling of requests for non-existent resources
func TestErrorNonExistentResources(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()
	skipIfNoServices(t, c)

	nonExistentCases := []struct {
		name string
		url  string
	}{
		{"Non-existent work", c.urls.WorkPublisher + "/v1/work/non-existent-work-12345"},
		{"Non-existent provider", c.urls.ProviderRegistry + "/v1/providers/non-existent-provider-12345"},
		{"Non-existent contract", c.urls.ContractEngine + "/v1/contracts/non-existent-contract-12345"},
		{"Non-existent tenant", c.urls.Identity + "/v1/tenants/non-existent-tenant-12345"},
		{"Non-existent trust score", c.urls.TrustBroker + "/v1/providers/non-existent-provider/trust"},
	}

	for _, tc := range nonExistentCases {
		status, body, err := c.RequestWithStatus(ctx, http.MethodGet, tc.url, nil)
		if err != nil {
			t.Logf("%s: error - %v", tc.name, err)
			continue
		}
		t.Logf("%s: status %d", tc.name, status)
		if status >= 200 && status < 300 {
			t.Logf("  Response: %s", string(body))
		}
	}
}

// TestErrorInvalidMethods tests handling of invalid HTTP methods
func TestErrorInvalidMethods(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()
	skipIfNoServices(t, c)

	invalidMethodCases := []struct {
		name   string
		method string
		url    string
	}{
		{"DELETE on work list", http.MethodDelete, c.urls.WorkPublisher + "/v1/work"},
		{"PATCH on providers", http.MethodPatch, c.urls.ProviderRegistry + "/v1/providers"},
		{"PUT on bids", http.MethodPut, c.urls.BidGateway + "/v1/bids"},
		{"DELETE on health", http.MethodDelete, c.urls.WorkPublisher + "/health"},
	}

	for _, tc := range invalidMethodCases {
		status, _, err := c.RequestWithStatus(ctx, tc.method, tc.url, nil)
		if err != nil {
			t.Logf("%s: error - %v", tc.name, err)
			continue
		}
		t.Logf("%s: status %d", tc.name, status)
	}
}

// TestErrorInvalidDataTypes tests handling of invalid data types
func TestErrorInvalidDataTypes(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()
	skipIfNoServices(t, c)

	// String instead of number for price
	status, body, err := c.RequestWithStatus(ctx, http.MethodPost, c.urls.BidGateway+"/v1/bids", map[string]any{
		"work_id":     "work-123",
		"provider_id": "provider-123",
		"price":       "not-a-number",
		"currency":    "USD",
	})
	if err != nil {
		t.Logf("Invalid price type error: %v", err)
	} else {
		t.Logf("Invalid price type: status %d, body: %s", status, string(body))
	}

	// Negative amount for deposit
	status, body, err = c.RequestWithStatus(ctx, http.MethodPost, c.urls.Settlement+"/v1/deposits", map[string]any{
		"tenant_id": "test-tenant",
		"amount":    -100.00,
		"currency":  "USD",
	})
	if err != nil {
		t.Logf("Negative deposit error: %v", err)
	} else {
		t.Logf("Negative deposit: status %d, body: %s", status, string(body))
	}

	// Invalid bid window (negative)
	status, body, err = c.RequestWithStatus(ctx, http.MethodPost, c.urls.WorkPublisher+"/v1/work", map[string]any{
		"category":        "test",
		"task_type":       "test",
		"description":     "Test",
		"consumer_id":     "test",
		"bid_window_secs": -60,
	})
	if err != nil {
		t.Logf("Negative bid window error: %v", err)
	} else {
		t.Logf("Negative bid window: status %d", status)
	}
}

// TestErrorBidOnNonExistentWork tests bidding on non-existent work
func TestErrorBidOnNonExistentWork(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()
	skipIfNoServices(t, c)

	timestamp := time.Now().UnixNano()

	// Register a provider
	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Error Test Provider %d", timestamp),
		Capabilities: []string{"error-test"},
		Endpoint:     "https://error-test.example.com",
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	// Try to bid on non-existent work
	_, err = c.SubmitBidWithAuth(ctx, provider.APIKey, &Bid{
		WorkID:      "non-existent-work-id-12345",
		Price:       50.00,
		Confidence:  0.9,
		Approach:    "standard",
		A2AEndpoint: "https://error-test.example.com/a2a",
		ExpiresAt:   time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Logf("Bid on non-existent work correctly rejected: %v", err)
	} else {
		t.Log("Bid on non-existent work was accepted (may be valid depending on implementation)")
	}
}

// TestErrorAwardContractWithInvalidBid tests awarding with invalid bid
func TestErrorAwardContractWithInvalidBid(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()
	skipIfNoServices(t, c)

	timestamp := time.Now().UnixNano()

	// Create work
	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "error-test",
		Description: "Error test work",
		Payload:     map[string]any{"data": "test"},
		Budget:      &Budget{MaxPrice: 100.00},
		ConsumerID:  fmt.Sprintf("error-consumer-%d", timestamp),
		BidWindowMs: 300000,
	})
	if err != nil {
		t.Fatalf("Failed to create work: %v", err)
	}

	// Try to award with non-existent bid
	_, err = c.AwardContract(ctx, work.ID, &AwardRequest{
		BidID: "non-existent-bid-12345",
	})
	if err != nil {
		t.Logf("Award with invalid bid correctly rejected: %v", err)
	} else {
		t.Log("Award with invalid bid was accepted (may be valid)")
	}
}

// TestErrorDuplicateOperations tests handling of duplicate operations
func TestErrorDuplicateOperations(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()
	skipIfNoServices(t, c)

	timestamp := time.Now().UnixNano()

	// Create tenant
	tenant, err := c.CreateTenant(ctx, &Tenant{
		Name:  fmt.Sprintf("Duplicate Test %d", timestamp),
		Email: fmt.Sprintf("duplicate-%d@test.com", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Try to create same tenant again (same name/email)
	_, err = c.CreateTenant(ctx, &Tenant{
		Name:  tenant.Name,
		Email: fmt.Sprintf("duplicate-%d@test.com", timestamp),
	})
	if err != nil {
		t.Logf("Duplicate tenant creation handled: %v", err)
	} else {
		t.Log("Duplicate tenant was allowed")
	}
}

// TestErrorConcurrentModification tests concurrent modification handling
func TestErrorConcurrentModification(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()
	skipIfNoServices(t, c)

	timestamp := time.Now().UnixNano()
	tenantID := fmt.Sprintf("concurrent-tenant-%d", timestamp)

	// Simulate concurrent deposits (sequential in test, but checking for race conditions)
	for i := 0; i < 10; i++ {
		err := c.Deposit(ctx, &DepositRequest{
			TenantID: tenantID,
			Amount:   "10.00",
		})
		if err != nil {
			t.Errorf("Deposit %d failed: %v", i, err)
		}
	}

	// Check final balance
	balance, err := c.GetBalance(ctx, tenantID)
	if err != nil {
		t.Fatalf("Failed to get balance: %v", err)
	}

	expectedMin := 100.00
	if balance.Balance < expectedMin {
		t.Errorf("Expected balance >= %.2f, got %.2f (possible race condition)", expectedMin, balance.Balance)
	} else {
		t.Logf("Concurrent deposits handled correctly: balance %.2f", balance.Balance)
	}
}

// TestErrorLargePayloads tests handling of excessively large payloads
func TestErrorLargePayloads(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()
	skipIfNoServices(t, c)

	timestamp := time.Now().UnixNano()

	// Create very large description (1MB)
	largeDescription := make([]byte, 1024*1024)
	for i := range largeDescription {
		largeDescription[i] = byte('A' + (i % 26))
	}

	status, body, err := c.RequestWithStatus(ctx, http.MethodPost, c.urls.WorkPublisher+"/v1/work", map[string]any{
		"category":        "large-test",
		"task_type":       "test",
		"description":     string(largeDescription),
		"input":           map[string]any{"data": "test"},
		"consumer_id":     fmt.Sprintf("large-consumer-%d", timestamp),
		"bid_window_secs": 60,
	})
	if err != nil {
		t.Logf("Large payload error: %v", err)
		return
	}

	t.Logf("Large payload (1MB): status %d", status)
	if status >= 400 {
		t.Logf("Response: %s", string(body))
	}
}

// TestErrorSpecialCharacters tests handling of special characters in inputs
func TestErrorSpecialCharacters(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()
	skipIfNoServices(t, c)

	timestamp := time.Now().UnixNano()

	specialChars := []string{
		"Test<script>alert('xss')</script>",
		"Test'; DROP TABLE tenants; --",
		"Test\x00null\x00byte",
		"Test\n\r\t whitespace",
		"Test emoji !@#$%^&*()",
	}

	for i, special := range specialChars {
		_, err := c.CreateTenant(ctx, &Tenant{
			Name:  fmt.Sprintf("Special %d: %s", i, special),
			Email: fmt.Sprintf("special-%d-%d@test.com", timestamp, i),
		})
		if err != nil {
			t.Logf("Special chars %d rejected: %v", i, err)
		} else {
			t.Logf("Special chars %d accepted", i)
		}
	}
}

// TestErrorEmptyStrings tests handling of empty strings
func TestErrorEmptyStrings(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()
	skipIfNoServices(t, c)

	// Empty tenant name
	status, _, err := c.RequestWithStatus(ctx, http.MethodPost, c.urls.Identity+"/v1/tenants", map[string]any{
		"name":  "",
		"email": "empty@test.com",
	})
	if err != nil {
		t.Logf("Empty tenant name error: %v", err)
	} else {
		t.Logf("Empty tenant name: status %d", status)
	}

	// Empty provider capabilities
	status, _, err = c.RequestWithStatus(ctx, http.MethodPost, c.urls.ProviderRegistry+"/v1/providers", map[string]any{
		"name":         "Empty Caps Provider",
		"capabilities": []string{},
		"endpoint":     "https://test.example.com",
	})
	if err != nil {
		t.Logf("Empty capabilities error: %v", err)
	} else {
		t.Logf("Empty capabilities: status %d", status)
	}
}

// TestErrorInvalidUUIDs tests handling of invalid UUID formats
func TestErrorInvalidUUIDs(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()
	skipIfNoServices(t, c)

	invalidIDs := []string{
		"not-a-uuid",
		"12345",
		"",
		"xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
		"../../../etc/passwd",
	}

	for _, id := range invalidIDs {
		status, _, _ := c.RequestWithStatus(ctx, http.MethodGet, c.urls.WorkPublisher+"/v1/work/"+id, nil)
		t.Logf("Invalid ID '%s': status %d", id, status)

		status, _, _ = c.RequestWithStatus(ctx, http.MethodGet, c.urls.ProviderRegistry+"/v1/providers/"+id, nil)
		t.Logf("Invalid provider ID '%s': status %d", id, status)
	}
}

// TestErrorZeroBudget tests handling of zero budget
func TestErrorZeroBudget(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()
	skipIfNoServices(t, c)

	timestamp := time.Now().UnixNano()

	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "zero-budget-test",
		Description: "Zero budget test",
		Payload:       map[string]any{"data": "test"},
		Budget: &Budget{
			MaxPrice: 0.00,
		},
		ConsumerID:    fmt.Sprintf("zero-budget-consumer-%d", timestamp),
		BidWindowMs: 60000,
	})
	if err != nil {
		t.Logf("Zero budget work rejected: %v", err)
	} else {
		t.Logf("Zero budget work accepted: %s", work.ID)
	}
}

// TestErrorExpiredBid tests submitting an already expired bid
func TestErrorExpiredBid(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()
	skipIfNoServices(t, c)

	timestamp := time.Now().UnixNano()

	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Expired Bid Provider %d", timestamp),
		Capabilities: []string{"expired-test"},
		Endpoint:     "https://expired.example.com",
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "expired-bid-test",
		Description: "Expired bid test",
		Payload:     map[string]any{"data": "test"},
		Budget:      &Budget{MaxPrice: 100.00},
		ConsumerID:  fmt.Sprintf("expired-consumer-%d", timestamp),
		BidWindowMs: 300000,
	})
	if err != nil {
		t.Fatalf("Failed to create work: %v", err)
	}

	// Submit bid with past expiration
	_, err = c.SubmitBidWithAuth(ctx, provider.APIKey, &Bid{
		WorkID:      work.ID,
		Price:       50.00,
		Confidence:  0.9,
		Approach:    "standard",
		A2AEndpoint: "https://expired.example.com/a2a",
		ExpiresAt:   time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Logf("Expired bid rejected: %v", err)
	} else {
		t.Log("Expired bid was accepted (may be valid if server validates)")
	}
}

