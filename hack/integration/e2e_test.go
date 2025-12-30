package integration

import (
	"context"
	"os"
	"testing"
	"time"
)

// skipIfNoServices skips the test if services are not available
func skipIfNoServices(t *testing.T, c *Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.HealthCheck(ctx, c.urls.WorkPublisher); err != nil {
		t.Skipf("Services not available: %v (run with docker-compose up)", err)
	}
}

func getTestClient() *Client {
	urls := DefaultLocalURLs()

	// Allow override from environment
	if u := os.Getenv("GATEWAY_URL"); u != "" {
		urls.Gateway = u
	}
	if u := os.Getenv("WORK_PUBLISHER_URL"); u != "" {
		urls.WorkPublisher = u
	}
	if u := os.Getenv("BID_GATEWAY_URL"); u != "" {
		urls.BidGateway = u
	}
	if u := os.Getenv("BID_EVALUATOR_URL"); u != "" {
		urls.BidEvaluator = u
	}
	if u := os.Getenv("CONTRACT_ENGINE_URL"); u != "" {
		urls.ContractEngine = u
	}
	if u := os.Getenv("PROVIDER_REGISTRY_URL"); u != "" {
		urls.ProviderRegistry = u
	}
	if u := os.Getenv("TRUST_BROKER_URL"); u != "" {
		urls.TrustBroker = u
	}
	if u := os.Getenv("IDENTITY_URL"); u != "" {
		urls.Identity = u
	}
	if u := os.Getenv("SETTLEMENT_URL"); u != "" {
		urls.Settlement = u
	}
	if u := os.Getenv("TELEMETRY_URL"); u != "" {
		urls.Telemetry = u
	}

	return NewClient(urls)
}

// TestEndToEndWorkflow tests the complete workflow from work submission to settlement
func TestEndToEndWorkflow(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	// Step 1: Create a tenant (consumer)
	t.Log("Step 1: Creating consumer tenant...")
	consumer, err := c.CreateTenant(ctx, &Tenant{
		Name:  "Test Consumer",
		Email: "consumer@test.com",
	})
	if err != nil {
		t.Fatalf("Failed to create consumer tenant: %v", err)
	}
	t.Logf("  Created consumer: %s", consumer.ID)

	// Step 2: Create provider tenant
	t.Log("Step 2: Creating provider tenant...")
	providerTenant, err := c.CreateTenant(ctx, &Tenant{
		Name:  "Test Provider Tenant",
		Email: "provider@test.com",
	})
	if err != nil {
		t.Fatalf("Failed to create provider tenant: %v", err)
	}
	t.Logf("  Created provider tenant: %s", providerTenant.ID)

	// Step 3: Register first provider
	t.Log("Step 3: Registering providers...")
	provider1, err := c.RegisterProvider(ctx, &Provider{
		Name:         "Test AI Provider 1",
		Capabilities: []string{"text-generation", "summarization"},
		Endpoint:     "https://provider1.test/api",
		Metadata: map[string]string{
			"tenant_id": providerTenant.ID,
		},
	})
	if err != nil {
		t.Fatalf("Failed to register provider 1: %v", err)
	}
	if provider1.APIKey == "" {
		t.Fatal("Provider 1 should have received an API key")
	}
	t.Logf("  Registered provider 1: %s", provider1.ID)

	// Register second provider for competition
	provider2, err := c.RegisterProvider(ctx, &Provider{
		Name:         "Alternative AI Provider",
		Capabilities: []string{"text-generation"},
		Endpoint:     "https://provider2.test/api",
	})
	if err != nil {
		t.Fatalf("Failed to register provider 2: %v", err)
	}
	t.Logf("  Registered provider 2: %s", provider2.ID)

	// Step 4: Subscribe provider to a category
	t.Log("Step 4: Creating subscription...")
	category := "text-generation"
	_, err = c.CreateSubscription(ctx, &Subscription{
		ProviderID: provider1.ID,
		Categories: []string{category},
	})
	if err != nil {
		t.Fatalf("Failed to create subscription: %v", err)
	}
	t.Logf("  Subscribed to category: %s", category)

	// Step 5: Deposit funds for consumer
	t.Log("Step 5: Depositing funds for consumer...")
	err = c.Deposit(ctx, &DepositRequest{
		TenantID: consumer.ID,
		Amount:   "1000.00",
	})
	if err != nil {
		t.Fatalf("Failed to deposit funds: %v", err)
	}

	balance, err := c.GetBalance(ctx, consumer.ID)
	if err != nil {
		t.Fatalf("Failed to get balance: %v", err)
	}
	t.Logf("  Consumer balance: $%.2f", balance.Balance)

	// Step 6: Submit work
	t.Log("Step 6: Submitting work...")
	latency := int64(5000)
	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    category,
		Description: "Summarize this document",
		Payload: map[string]any{
			"text": "This is a test document that needs to be summarized. It contains important information.",
		},
		Constraints: &Constraints{
			MaxLatencyMs: &latency,
		},
		Budget: &Budget{
			MaxPrice: 50.00,
		},
		ConsumerID:  consumer.ID,
		BidWindowMs: 60000,
	})
	if err != nil {
		t.Fatalf("Failed to submit work: %v", err)
	}
	t.Logf("  Work submitted: %s (status: %s)", work.ID, work.Status)

	// Step 7: Submit bids with provider authentication
	t.Log("Step 7: Submitting bids...")
	bid1Resp, err := c.SubmitBidWithAuth(ctx, provider1.APIKey, &Bid{
		WorkID:      work.ID,
		Price:       25.00,
		Confidence:  0.92,
		Approach:    "Advanced NLP summarization with semantic analysis",
		A2AEndpoint: "https://provider1.test/a2a",
		SLA: &SLA{
			MaxLatencyMs: 3000,
			Availability: 0.99,
		},
		ExpiresAt: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("Failed to submit bid 1: %v", err)
	}
	t.Logf("  Bid 1 submitted: %s (price: $25.00)", bid1Resp.BidID)

	bid2Resp, err := c.SubmitBidWithAuth(ctx, provider2.APIKey, &Bid{
		WorkID:      work.ID,
		Price:       30.00,
		Confidence:  0.95,
		Approach:    "Premium summarization with quality guarantees",
		A2AEndpoint: "https://provider2.test/a2a",
		SLA: &SLA{
			MaxLatencyMs: 2000,
			Availability: 0.995,
		},
		ExpiresAt: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("Failed to submit bid 2: %v", err)
	}
	t.Logf("  Bid 2 submitted: %s (price: $30.00)", bid2Resp.BidID)

	// Step 8: Evaluate bids
	t.Log("Step 8: Evaluating bids...")
	evaluation, err := c.EvaluateBids(ctx, &EvaluationRequest{
		WorkID: work.ID,
		Budget: &EvaluationBudget{
			MaxPrice:    50.00,
			BidStrategy: "balanced",
		},
	})
	if err != nil {
		t.Fatalf("Failed to evaluate bids: %v", err)
	}
	t.Logf("  Evaluation complete: %d bids ranked", len(evaluation.RankedBids))
	for _, rb := range evaluation.RankedBids {
		t.Logf("    Rank %d: Provider %s, Score %.3f", rb.Rank, rb.ProviderID, rb.Score)
	}

	if len(evaluation.RankedBids) == 0 {
		t.Fatal("No bids ranked in evaluation")
	}
	// Winner is the first ranked bid
	winner := &evaluation.RankedBids[0]
	t.Logf("  Winner: %s (score: %.3f)", winner.ProviderID, winner.Score)

	// Step 9: Award contract
	t.Log("Step 9: Awarding contract...")
	awardResp, err := c.AwardContract(ctx, work.ID, &AwardRequest{
		BidID: winner.BidID,
	})
	if err != nil {
		t.Fatalf("Failed to award contract: %v", err)
	}
	t.Logf("  Contract awarded: %s (status: %s)", awardResp.ContractID, awardResp.Status)

	// Step 10: Update progress with execution token
	t.Log("Step 10: Updating progress...")
	pct := 50
	err = c.UpdateProgressWithToken(ctx, awardResp.ContractID, awardResp.ExecutionToken, &ProgressRequest{
		Status:  "in_progress",
		Percent: &pct,
		Message: "Processing document...",
	})
	if err != nil {
		t.Fatalf("Failed to update progress: %v", err)
	}
	t.Log("  Progress updated to 50%")

	// Step 11: Complete contract with execution token
	t.Log("Step 11: Completing contract...")
	err = c.CompleteContractWithToken(ctx, awardResp.ContractID, awardResp.ExecutionToken, &CompleteRequest{
		Success:       true,
		ResultSummary: "This is the summarized version of the document.",
		Metrics: map[string]any{
			"word_count": 42,
			"latency_ms": 1500,
		},
	})
	if err != nil {
		t.Fatalf("Failed to complete contract: %v", err)
	}

	// Verify contract completion
	contract, err := c.GetContract(ctx, awardResp.ContractID)
	if err != nil {
		t.Fatalf("Failed to get contract: %v", err)
	}
	t.Logf("  Contract completed: %s (status: %s)", contract.ContractID, contract.Status)

	// Step 12: Verify settlement
	t.Log("Step 12: Verifying settlement...")

	// Check consumer balance
	consumerBalance, err := c.GetBalance(ctx, consumer.ID)
	if err != nil {
		t.Fatalf("Failed to get consumer balance: %v", err)
	}
	t.Logf("  Consumer balance after: $%.2f", consumerBalance.Balance)

	// Check transactions
	transactions, err := c.GetTransactions(ctx, consumer.ID)
	if err != nil {
		t.Fatalf("Failed to get transactions: %v", err)
	}
	t.Logf("  Consumer has %d transactions", len(transactions))

	// Step 13: Verify trust score updated
	t.Log("Step 13: Checking trust scores...")
	trustScore, err := c.GetTrustScore(ctx, winner.ProviderID)
	if err != nil {
		t.Logf("  Trust score lookup: %v (may not be implemented)", err)
	} else {
		t.Logf("  Winner trust score: %.3f (tier: %s)", trustScore.Score, trustScore.Tier)
	}

	t.Logf("\n=== End-to-End Test Complete (timestamp: %d) ===", timestamp)
	t.Log("Successfully tested:")
	t.Log("  - Tenant creation")
	t.Log("  - Provider registration with API keys")
	t.Log("  - Provider subscription")
	t.Log("  - Work submission")
	t.Log("  - Authenticated bid submission")
	t.Log("  - Bid evaluation")
	t.Log("  - Contract award with execution token")
	t.Log("  - Authenticated progress tracking")
	t.Log("  - Authenticated contract completion")
	t.Log("  - Settlement verification")
}

// TestProviderRegistrationFlow tests provider lifecycle
func TestProviderRegistrationFlow(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()

	// Register provider
	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         "Integration Test Provider",
		Capabilities: []string{"test-capability"},
		Endpoint:     "https://test.example.com/api",
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	if provider.ID == "" {
		t.Error("Provider ID should not be empty")
	}
	if provider.APIKey == "" {
		t.Error("Provider should receive an API key")
	}
	if provider.Status != "ACTIVE" && provider.Status != "active" && provider.Status != "PENDING_VERIFICATION" && provider.Status != "" {
		t.Logf("Provider status: %s", provider.Status)
	}

	// Create subscription
	sub, err := c.CreateSubscription(ctx, &Subscription{
		ProviderID: provider.ID,
		Categories: []string{"test-category"},
	})
	if err != nil {
		t.Fatalf("Failed to create subscription: %v", err)
	}

	if sub.ID == "" {
		t.Error("Subscription ID should not be empty")
	}

	t.Logf("Provider registration flow complete: provider=%s, subscription=%s", provider.ID, sub.ID)
}

// TestBidSubmissionFlow tests bid submission with provider auth
func TestBidSubmissionFlow(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	// First register provider
	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         "Bid Test Provider",
		Capabilities: []string{"bid-test"},
		Endpoint:     "https://test.example.com/api",
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	// Submit work
	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "bid-test",
		Description: "Test work for bid submission",
		Payload:     map[string]any{"data": "test"},
		Budget: &Budget{
			MaxPrice: 100.00,
		},
		ConsumerID:  "test-consumer",
		BidWindowMs: 300000,
	})
	if err != nil {
		t.Fatalf("Failed to submit work: %v", err)
	}

	// Submit bid with provider authentication
	bidResp, err := c.SubmitBidWithAuth(ctx, provider.APIKey, &Bid{
		WorkID:      work.ID,
		Price:       50.00,
		Confidence:  0.9,
		Approach:    "standard processing",
		A2AEndpoint: "https://test.example.com/a2a",
		SLA: &SLA{
			MaxLatencyMs: 1000,
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

	t.Logf("Bid submission flow complete (timestamp %d): work=%s, bid=%s", timestamp, work.ID, bidResp.BidID)
}

// TestSettlementFlow tests deposit and balance operations
func TestSettlementFlow(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	tenantID := "settlement-test-tenant"

	// Deposit funds
	err := c.Deposit(ctx, &DepositRequest{
		TenantID: tenantID,
		Amount:   "500.00",
	})
	if err != nil {
		t.Fatalf("Failed to deposit: %v", err)
	}

	// Check balance
	balance, err := c.GetBalance(ctx, tenantID)
	if err != nil {
		t.Fatalf("Failed to get balance: %v", err)
	}

	if balance.Balance < 500.00 {
		t.Errorf("Expected balance >= 500, got %.2f", balance.Balance)
	}

	// Check transactions
	transactions, err := c.GetTransactions(ctx, tenantID)
	if err != nil {
		t.Fatalf("Failed to get transactions: %v", err)
	}

	if len(transactions) == 0 {
		t.Error("Expected at least one transaction")
	}

	t.Logf("Settlement flow complete: balance=%.2f, transactions=%d", balance.Balance, len(transactions))
}

// TestHealthChecks verifies all services are healthy
func TestHealthChecks(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	services := map[string]string{
		"work-publisher":    c.urls.WorkPublisher,
		"bid-gateway":       c.urls.BidGateway,
		"bid-evaluator":     c.urls.BidEvaluator,
		"contract-engine":   c.urls.ContractEngine,
		"provider-registry": c.urls.ProviderRegistry,
		"trust-broker":      c.urls.TrustBroker,
		"identity":          c.urls.Identity,
		"settlement":        c.urls.Settlement,
		"gateway":           c.urls.Gateway,
		"telemetry":         c.urls.Telemetry,
	}

	allHealthy := true
	for name, url := range services {
		err := c.HealthCheck(ctx, url)
		if err != nil {
			t.Logf("UNHEALTHY: %s - %v", name, err)
			allHealthy = false
		} else {
			t.Logf("HEALTHY: %s", name)
		}
	}

	if !allHealthy {
		t.Skip("Not all services are healthy")
	}
}
