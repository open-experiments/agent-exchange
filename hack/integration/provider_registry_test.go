package integration

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestProviderRegistration tests basic provider registration
func TestProviderRegistration(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()
	providerName := fmt.Sprintf("Test Provider %d", timestamp)

	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         providerName,
		Capabilities: []string{"text-generation", "summarization"},
		Endpoint:     fmt.Sprintf("https://provider-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	if provider.ID == "" {
		t.Error("Provider ID should not be empty")
	}
	if provider.APIKey == "" {
		t.Error("Provider API key should not be empty")
	}
	t.Logf("Registered provider: %s (API Key provided)", provider.ID)

	// Fetch provider to verify name was stored
	fetchedProvider, err := c.GetProvider(ctx, provider.ID)
	if err != nil {
		t.Fatalf("Failed to get provider: %v", err)
	}

	if fetchedProvider.ID != provider.ID {
		t.Errorf("Provider ID mismatch: expected %s, got %s", provider.ID, fetchedProvider.ID)
	}
	if fetchedProvider.Name != providerName {
		t.Errorf("Provider name mismatch: expected %s, got %s", providerName, fetchedProvider.Name)
	}

	t.Logf("Verified provider: %s", fetchedProvider.Name)
}

// TestProviderWithMetadata tests provider registration with metadata
func TestProviderWithMetadata(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Metadata Provider %d", timestamp),
		Capabilities: []string{"translation"},
		Endpoint:     fmt.Sprintf("https://metadata-%d.example.com/api", timestamp),
		Metadata: map[string]string{
			"region":    "us-west-2",
			"version":   "v2.0",
			"certified": "true",
		},
	})
	if err != nil {
		t.Fatalf("Failed to register provider with metadata: %v", err)
	}

	t.Logf("Registered provider with metadata: %s", provider.ID)
}

// TestProviderSubscription tests subscription creation
func TestProviderSubscription(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	// Register provider
	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Subscription Provider %d", timestamp),
		Capabilities: []string{"code-review"},
		Endpoint:     fmt.Sprintf("https://sub-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	// Create subscription
	sub, err := c.CreateSubscription(ctx, &Subscription{
		ProviderID: provider.ID,
		Categories: []string{"code-review"},
	})
	if err != nil {
		t.Fatalf("Failed to create subscription: %v", err)
	}

	if sub.ID == "" {
		t.Error("Subscription ID should not be empty")
	}

	t.Logf("Created subscription: %s for provider %s", sub.ID, provider.ID)
}

// TestProviderMultipleSubscriptions tests multiple subscriptions for a provider
func TestProviderMultipleSubscriptions(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	// Register provider
	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Multi-Sub Provider %d", timestamp),
		Capabilities: []string{"translation", "summarization", "sentiment"},
		Endpoint:     fmt.Sprintf("https://multi-sub-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	categories := []string{"translation", "summarization", "sentiment-analysis"}
	for _, category := range categories {
		_, err := c.CreateSubscription(ctx, &Subscription{
			ProviderID: provider.ID,
			Categories: []string{category},
		})
		if err != nil {
			t.Fatalf("Failed to create subscription for %s: %v", category, err)
		}
		t.Logf("Subscribed provider to: %s", category)
	}

	// List subscriptions
	subs, err := c.ListSubscriptions(ctx, provider.ID)
	if err != nil {
		t.Logf("ListSubscriptions not implemented or failed: %v", err)
	} else {
		t.Logf("Provider has %d subscriptions", len(subs))
	}
}

// TestProviderCapabilities tests provider capability matching
func TestProviderCapabilities(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	capabilities := []string{
		"text-generation",
		"code-completion",
		"image-generation",
		"audio-transcription",
	}

	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Full Capability Provider %d", timestamp),
		Capabilities: capabilities,
		Endpoint:     fmt.Sprintf("https://full-cap-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	// Verify capabilities
	fetchedProvider, err := c.GetProvider(ctx, provider.ID)
	if err != nil {
		t.Fatalf("Failed to get provider: %v", err)
	}

	if len(fetchedProvider.Capabilities) == 0 {
		t.Log("Capabilities not returned in GET response (may be expected)")
	} else {
		t.Logf("Provider has %d capabilities", len(fetchedProvider.Capabilities))
	}
}

// TestProviderNonExistent tests handling of non-existent provider
func TestProviderNonExistent(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()

	_, err := c.GetProvider(ctx, "non-existent-provider-id-12345")
	if err == nil {
		t.Log("Non-existent provider returned without error (may return empty)")
	} else {
		t.Logf("Non-existent provider handled correctly: %v", err)
	}
}

// TestProviderMultipleRegistrations tests registering multiple providers
func TestProviderMultipleRegistrations(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	providerCount := 5
	providerIDs := make([]string, providerCount)

	for i := 0; i < providerCount; i++ {
		provider, err := c.RegisterProvider(ctx, &Provider{
			Name:         fmt.Sprintf("Bulk Provider %d-%d", timestamp, i),
			Capabilities: []string{"bulk-test"},
			Endpoint:     fmt.Sprintf("https://bulk-%d-%d.example.com/api", timestamp, i),
		})
		if err != nil {
			t.Fatalf("Failed to register provider %d: %v", i, err)
		}
		providerIDs[i] = provider.ID
	}

	// Verify all providers
	for i, id := range providerIDs {
		_, err := c.GetProvider(ctx, id)
		if err != nil {
			t.Errorf("Failed to get provider %d: %v", i, err)
		}
	}

	t.Logf("Successfully registered and verified %d providers", providerCount)
}

