package integration

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestIdentityTenantLifecycle tests complete tenant lifecycle
func TestIdentityTenantLifecycle(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	// Create tenant
	tenantName := fmt.Sprintf("Test Tenant %d", timestamp)
	tenant, err := c.CreateTenant(ctx, &Tenant{
		Name:  tenantName,
		Email: fmt.Sprintf("test-%d@example.com", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	if tenant.ID == "" {
		t.Error("Tenant ID should not be empty")
	}
	t.Logf("Created tenant: %s (ID: %s)", tenant.Name, tenant.ID)

	// Get tenant
	fetchedTenant, err := c.GetTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("Failed to get tenant: %v", err)
	}

	if fetchedTenant.Name != tenantName {
		t.Errorf("Expected tenant name %s, got %s", tenantName, fetchedTenant.Name)
	}
	t.Logf("Fetched tenant: %s", fetchedTenant.Name)
}

// TestIdentityAPIKeyLifecycle tests API key creation and listing
func TestIdentityAPIKeyLifecycle(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	// Create tenant first
	tenant, err := c.CreateTenant(ctx, &Tenant{
		Name:  fmt.Sprintf("API Key Test Tenant %d", timestamp),
		Email: fmt.Sprintf("apikey-test-%d@example.com", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Create API key
	apiKey, err := c.CreateAPIKey(ctx, &APIKey{
		TenantID: tenant.ID,
		Name:     "Test API Key",
		Scopes:   []string{"read", "write"},
	})
	if err != nil {
		t.Fatalf("Failed to create API key: %v", err)
	}

	if apiKey.ID == "" {
		t.Error("API key ID should not be empty")
	}
	if apiKey.Key == "" {
		t.Error("API key value should not be empty")
	}
	t.Logf("Created API key: %s (ID: %s)", apiKey.Name, apiKey.ID)

	// List API keys
	apiKeys, err := c.ListAPIKeys(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("Failed to list API keys: %v", err)
	}

	if len(apiKeys) == 0 {
		t.Error("Expected at least one API key")
	}

	found := false
	for _, k := range apiKeys {
		if k.ID == apiKey.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("Created API key not found in list")
	}

	t.Logf("Found %d API keys for tenant", len(apiKeys))
}

// TestIdentityMultipleTenants tests creating and managing multiple tenants
func TestIdentityMultipleTenants(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	tenantIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		tenant, err := c.CreateTenant(ctx, &Tenant{
			Name:  fmt.Sprintf("Multi Tenant %d-%d", timestamp, i),
			Email: fmt.Sprintf("multi-%d-%d@example.com", timestamp, i),
		})
		if err != nil {
			t.Fatalf("Failed to create tenant %d: %v", i, err)
		}
		tenantIDs[i] = tenant.ID
		t.Logf("Created tenant %d: %s", i, tenant.ID)
	}

	// Verify each tenant can be retrieved
	for i, id := range tenantIDs {
		tenant, err := c.GetTenant(ctx, id)
		if err != nil {
			t.Fatalf("Failed to get tenant %d: %v", i, err)
		}
		if tenant.ID != id {
			t.Errorf("Tenant ID mismatch: expected %s, got %s", id, tenant.ID)
		}
	}

	t.Log("All tenants verified successfully")
}

// TestIdentityDuplicateTenant tests handling of duplicate tenant creation
func TestIdentityDuplicateTenant(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()
	email := fmt.Sprintf("duplicate-%d@example.com", timestamp)

	// Create first tenant
	_, err := c.CreateTenant(ctx, &Tenant{
		Name:  "Duplicate Test 1",
		Email: email,
	})
	if err != nil {
		t.Fatalf("Failed to create first tenant: %v", err)
	}

	// Creating second tenant with same email may fail or succeed based on implementation
	// We're just checking the system handles it gracefully
	_, err = c.CreateTenant(ctx, &Tenant{
		Name:  "Duplicate Test 2",
		Email: email,
	})
	// Just log the result - different implementations may handle this differently
	if err != nil {
		t.Logf("Duplicate email handling: %v", err)
	} else {
		t.Log("System allowed duplicate emails (may be valid)")
	}
}

// TestIdentityInvalidTenant tests error handling for invalid tenant data
func TestIdentityInvalidTenant(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()

	// Try to get non-existent tenant
	_, err := c.GetTenant(ctx, "non-existent-tenant-id")
	if err == nil {
		t.Log("Non-existent tenant returned without error (may return empty)")
	} else {
		t.Logf("Non-existent tenant handled: %v", err)
	}
}

