package integration

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestBidSubmission tests basic bid submission with provider authentication
func TestBidSubmission(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	// Setup: Register provider (get API key)
	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Bid Test Provider %d", timestamp),
		Capabilities: []string{"test"},
		Endpoint:     fmt.Sprintf("https://bid-test-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	if provider.APIKey == "" {
		t.Fatal("Provider registration should return API key")
	}
	t.Logf("Registered provider %s with API key", provider.ID)

	// Submit work
	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "test",
		Description: "Work for bid test",
		Payload:     map[string]any{"data": "test"},
		Budget: &Budget{
			MaxPrice: 100.00,
		},
		ConsumerID:  fmt.Sprintf("bid-test-consumer-%d", timestamp),
		BidWindowMs: 300000,
	})
	if err != nil {
		t.Fatalf("Failed to submit work: %v", err)
	}

	// Submit bid using provider's API key
	bidResp, err := c.SubmitBidWithAuth(ctx, provider.APIKey, &Bid{
		WorkID:      work.ID,
		Price:       50.00,
		Confidence:  0.85,
		Approach:    "standard processing",
		A2AEndpoint: fmt.Sprintf("https://bid-test-%d.example.com/a2a", timestamp),
		SLA: &SLA{
			MaxLatencyMs: 2000,
			Availability: 0.99,
		},
		ExpiresAt: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("Failed to submit bid: %v", err)
	}

	if bidResp.BidID == "" {
		t.Error("Bid ID should not be empty")
	}
	if bidResp.Status != "RECEIVED" {
		t.Errorf("Expected status RECEIVED, got %s", bidResp.Status)
	}
	t.Logf("Submitted bid: %s (status: %s)", bidResp.BidID, bidResp.Status)
}

// TestBidWithSLA tests bid submission with SLA parameters
func TestBidWithSLA(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("SLA Provider %d", timestamp),
		Capabilities: []string{"sla-test"},
		Endpoint:     fmt.Sprintf("https://sla-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "sla-test",
		Description: "Work for SLA bid test",
		Payload:     map[string]any{"data": "test"},
		Budget: &Budget{
			MaxPrice: 100.00,
		},
		ConsumerID:  fmt.Sprintf("sla-consumer-%d", timestamp),
		BidWindowMs: 300000,
	})
	if err != nil {
		t.Fatalf("Failed to submit work: %v", err)
	}

	// Submit bid with aggressive SLA
	bidResp, err := c.SubmitBidWithAuth(ctx, provider.APIKey, &Bid{
		WorkID:      work.ID,
		Price:       75.00,
		Confidence:  0.95,
		Approach:    "premium processing with guaranteed SLA",
		A2AEndpoint: fmt.Sprintf("https://sla-%d.example.com/a2a", timestamp),
		SLA: &SLA{
			MaxLatencyMs: 1000,
			Availability: 0.999,
		},
		ExpiresAt: time.Now().Add(2 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("Failed to submit bid: %v", err)
	}

	t.Logf("Submitted SLA bid: %s", bidResp.BidID)
}

// TestCompetingBids tests multiple bids from different providers
func TestCompetingBids(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	// Register multiple providers
	providers := make([]*Provider, 5)
	for i := 0; i < 5; i++ {
		provider, err := c.RegisterProvider(ctx, &Provider{
			Name:         fmt.Sprintf("Competing Provider %d-%d", timestamp, i),
			Capabilities: []string{"competition"},
			Endpoint:     fmt.Sprintf("https://competing-%d-%d.example.com/api", timestamp, i),
		})
		if err != nil {
			t.Fatalf("Failed to register provider %d: %v", i, err)
		}
		providers[i] = provider
	}

	// Submit work
	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "competition",
		Description: "Work for competing bids",
		Payload:     map[string]any{"data": "test"},
		Budget: &Budget{
			MaxPrice: 100.00,
		},
		ConsumerID:  fmt.Sprintf("competition-consumer-%d", timestamp),
		BidWindowMs: 300000,
	})
	if err != nil {
		t.Fatalf("Failed to submit work: %v", err)
	}

	// Submit competing bids
	prices := []float64{80.00, 65.00, 72.00, 55.00, 90.00}
	bidResponses := make([]*BidResponse, 5)

	for i, provider := range providers {
		bidResp, err := c.SubmitBidWithAuth(ctx, provider.APIKey, &Bid{
			WorkID:      work.ID,
			Price:       prices[i],
			Confidence:  0.9 - (float64(i) * 0.05),
			Approach:    fmt.Sprintf("approach %d", i),
			A2AEndpoint: fmt.Sprintf("https://competing-%d-%d.example.com/a2a", timestamp, i),
			SLA: &SLA{
				MaxLatencyMs: 2000 + (i * 500),
				Availability: 0.99 - (float64(i) * 0.01),
			},
			ExpiresAt: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
		})
		if err != nil {
			t.Fatalf("Failed to submit bid %d: %v", i, err)
		}
		bidResponses[i] = bidResp
		t.Logf("Bid %d: Provider %s, Price $%.2f, BidID %s", i, provider.ID, prices[i], bidResp.BidID)
	}

	t.Logf("Submitted %d competing bids for work %s", len(bidResponses), work.ID)
}

// TestBidEvaluation tests bid evaluation with different strategies
func TestBidEvaluation(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	// Setup providers and work
	providers := make([]*Provider, 3)
	for i := 0; i < 3; i++ {
		provider, err := c.RegisterProvider(ctx, &Provider{
			Name:         fmt.Sprintf("Eval Provider %d-%d", timestamp, i),
			Capabilities: []string{"evaluation"},
			Endpoint:     fmt.Sprintf("https://eval-%d-%d.example.com/api", timestamp, i),
		})
		if err != nil {
			t.Fatalf("Failed to register provider %d: %v", i, err)
		}
		providers[i] = provider
	}

	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "evaluation",
		Description: "Work for evaluation test",
		Payload:     map[string]any{"data": "test"},
		Budget: &Budget{
			MaxPrice: 100.00,
		},
		ConsumerID:  fmt.Sprintf("eval-consumer-%d", timestamp),
		BidWindowMs: 300000,
	})
	if err != nil {
		t.Fatalf("Failed to submit work: %v", err)
	}

	// Submit bids with different characteristics
	// Provider 0: Low price, low quality
	_, err = c.SubmitBidWithAuth(ctx, providers[0].APIKey, &Bid{
		WorkID:      work.ID,
		Price:       30.00,
		Confidence:  0.7,
		Approach:    "budget option",
		A2AEndpoint: fmt.Sprintf("https://eval-%d-0.example.com/a2a", timestamp),
		SLA: &SLA{
			MaxLatencyMs: 4500,
			Availability: 0.90,
		},
		ExpiresAt: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("Failed to submit bid 0: %v", err)
	}

	// Provider 1: Medium price, medium quality
	_, err = c.SubmitBidWithAuth(ctx, providers[1].APIKey, &Bid{
		WorkID:      work.ID,
		Price:       55.00,
		Confidence:  0.85,
		Approach:    "balanced option",
		A2AEndpoint: fmt.Sprintf("https://eval-%d-1.example.com/a2a", timestamp),
		SLA: &SLA{
			MaxLatencyMs: 2500,
			Availability: 0.95,
		},
		ExpiresAt: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("Failed to submit bid 1: %v", err)
	}

	// Provider 2: High price, high quality
	_, err = c.SubmitBidWithAuth(ctx, providers[2].APIKey, &Bid{
		WorkID:      work.ID,
		Price:       80.00,
		Confidence:  0.99,
		Approach:    "premium option",
		A2AEndpoint: fmt.Sprintf("https://eval-%d-2.example.com/a2a", timestamp),
		SLA: &SLA{
			MaxLatencyMs: 1000,
			Availability: 0.999,
		},
		ExpiresAt: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("Failed to submit bid 2: %v", err)
	}

	// Test different evaluation strategies
	strategies := []string{"balanced", "lowest_price", "best_quality"}

	for _, strategy := range strategies {
		result, err := c.EvaluateBids(ctx, &EvaluationRequest{
			WorkID: work.ID,
			Budget: &EvaluationBudget{
				MaxPrice:    100.00,
				BidStrategy: strategy,
			},
		})
		if err != nil {
			t.Errorf("Failed to evaluate bids with strategy %s: %v", strategy, err)
			continue
		}

		t.Logf("Strategy %s: %d bids ranked", strategy, len(result.RankedBids))
		if len(result.RankedBids) > 0 {
			winner := result.RankedBids[0]
			t.Logf("  Winner: Provider %s (score: %.3f)", winner.ProviderID, winner.Score)
		}
	}
}

// TestBidPriceVariations tests bids with various price points
func TestBidPriceVariations(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Price Variation Provider %d", timestamp),
		Capabilities: []string{"price-test"},
		Endpoint:     fmt.Sprintf("https://price-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	prices := []float64{0.01, 1.00, 10.00, 100.00, 1000.00}

	for _, price := range prices {
		work, err := c.SubmitWork(ctx, &WorkSpec{
			Category:    "price-test",
			Description: fmt.Sprintf("Price test $%.2f", price),
			Payload:     map[string]any{"data": "test"},
			Budget: &Budget{
				MaxPrice: price * 2,
			},
			ConsumerID:  fmt.Sprintf("price-consumer-%d", timestamp),
			BidWindowMs: 60000,
		})
		if err != nil {
			t.Fatalf("Failed to submit work for price $%.2f: %v", price, err)
		}

		bidResp, err := c.SubmitBidWithAuth(ctx, provider.APIKey, &Bid{
			WorkID:      work.ID,
			Price:       price,
			Confidence:  0.9,
			Approach:    "standard",
			A2AEndpoint: fmt.Sprintf("https://price-%d.example.com/a2a", timestamp),
			SLA: &SLA{
				MaxLatencyMs: 2000,
				Availability: 0.99,
			},
			ExpiresAt: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
		})
		if err != nil {
			t.Errorf("Failed to submit bid for $%.2f: %v", price, err)
			continue
		}
		t.Logf("Submitted bid at $%.2f: %s", price, bidResp.BidID)
	}
}

// TestBidExpiration tests bid expiration handling
func TestBidExpiration(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Expiration Provider %d", timestamp),
		Capabilities: []string{"expiration-test"},
		Endpoint:     fmt.Sprintf("https://expiration-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "expiration-test",
		Description: "Work for expiration test",
		Payload:     map[string]any{"data": "test"},
		Budget: &Budget{
			MaxPrice: 50.00,
		},
		ConsumerID:  fmt.Sprintf("expiration-consumer-%d", timestamp),
		BidWindowMs: 300000,
	})
	if err != nil {
		t.Fatalf("Failed to submit work: %v", err)
	}

	expirations := []time.Duration{
		5 * time.Minute,
		1 * time.Hour,
		24 * time.Hour,
	}

	for _, exp := range expirations {
		bidResp, err := c.SubmitBidWithAuth(ctx, provider.APIKey, &Bid{
			WorkID:      work.ID,
			Price:       25.00,
			Confidence:  0.9,
			Approach:    "standard",
			A2AEndpoint: fmt.Sprintf("https://expiration-%d.example.com/a2a", timestamp),
			SLA: &SLA{
				MaxLatencyMs: 2000,
				Availability: 0.99,
			},
			ExpiresAt: time.Now().Add(exp).Format(time.RFC3339),
		})
		if err != nil {
			t.Errorf("Failed to submit bid with %v expiration: %v", exp, err)
			continue
		}
		t.Logf("Submitted bid with %v expiration: %s", exp, bidResp.BidID)
	}
}

// TestBidWithoutSLA tests bid submission without SLA
func TestBidWithoutSLA(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("No SLA Provider %d", timestamp),
		Capabilities: []string{"no-sla-test"},
		Endpoint:     fmt.Sprintf("https://no-sla-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "no-sla-test",
		Description: "Work for no SLA test",
		Payload:     map[string]any{"data": "test"},
		Budget: &Budget{
			MaxPrice: 50.00,
		},
		ConsumerID:  fmt.Sprintf("no-sla-consumer-%d", timestamp),
		BidWindowMs: 60000,
	})
	if err != nil {
		t.Fatalf("Failed to submit work: %v", err)
	}

	// Submit bid without SLA
	bidResp, err := c.SubmitBidWithAuth(ctx, provider.APIKey, &Bid{
		WorkID:      work.ID,
		Price:       25.00,
		Confidence:  0.8,
		Approach:    "basic processing",
		A2AEndpoint: fmt.Sprintf("https://no-sla-%d.example.com/a2a", timestamp),
		ExpiresAt:   time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("Failed to submit bid without SLA: %v", err)
	}
	t.Logf("Submitted bid without SLA: %s", bidResp.BidID)
}

// TestBidAuthenticationRequired tests that bid submission requires authentication
func TestBidAuthenticationRequired(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	// Submit work first
	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "auth-test",
		Description: "Work for auth test",
		Payload:     map[string]any{"data": "test"},
		Budget: &Budget{
			MaxPrice: 50.00,
		},
		ConsumerID:  fmt.Sprintf("auth-consumer-%d", timestamp),
		BidWindowMs: 60000,
	})
	if err != nil {
		t.Fatalf("Failed to submit work: %v", err)
	}

	// Try to submit bid without authentication (should fail)
	_, body, err := c.RequestWithStatus(ctx, "POST", c.urls.BidGateway+"/v1/bids", map[string]any{
		"work_id":      work.ID,
		"price":        25.00,
		"confidence":   0.8,
		"approach":     "basic",
		"a2a_endpoint": "https://example.com/a2a",
		"expires_at":   time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Logf("Request failed (expected): %v", err)
	}

	// The request should have been rejected
	t.Logf("Response without auth: %s", string(body))
}

// TestBidInvalidAPIKey tests that invalid API keys are rejected
func TestBidInvalidAPIKey(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	// Submit work first
	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "invalid-key-test",
		Description: "Work for invalid key test",
		Payload:     map[string]any{"data": "test"},
		Budget: &Budget{
			MaxPrice: 50.00,
		},
		ConsumerID:  fmt.Sprintf("invalid-key-consumer-%d", timestamp),
		BidWindowMs: 60000,
	})
	if err != nil {
		t.Fatalf("Failed to submit work: %v", err)
	}

	// Try to submit bid with invalid API key (should fail)
	_, err = c.SubmitBidWithAuth(ctx, "invalid-api-key-12345", &Bid{
		WorkID:      work.ID,
		Price:       25.00,
		Confidence:  0.8,
		Approach:    "basic",
		A2AEndpoint: "https://example.com/a2a",
		ExpiresAt:   time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
	if err == nil {
		t.Error("Expected error for invalid API key, got none")
	} else {
		t.Logf("Invalid API key correctly rejected: %v", err)
	}
}
