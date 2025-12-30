package integration

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestContractAward tests basic contract awarding with auto_award
func TestContractAward(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	// Setup: provider, work, bid
	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Contract Provider %d", timestamp),
		Capabilities: []string{"contract-test"},
		Endpoint:     fmt.Sprintf("https://contract-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "contract-test",
		Description: "Work for contract test",
		Payload:     map[string]any{"data": "test"},
		Budget: &Budget{
			MaxPrice: 100.00,
		},
		ConsumerID:  fmt.Sprintf("contract-consumer-%d", timestamp),
		BidWindowMs: 300000,
	})
	if err != nil {
		t.Fatalf("Failed to submit work: %v", err)
	}

	// Submit bid with provider auth
	bidResp, err := c.SubmitBidWithAuth(ctx, provider.APIKey, &Bid{
		WorkID:      work.ID,
		Price:       50.00,
		Confidence:  0.9,
		Approach:    "standard processing",
		A2AEndpoint: fmt.Sprintf("https://contract-%d.example.com/a2a", timestamp),
		SLA: &SLA{
			MaxLatencyMs: 2000,
			Availability: 0.99,
		},
		ExpiresAt: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("Failed to submit bid: %v", err)
	}

	// Award contract using auto_award
	awardResp, err := c.AwardContract(ctx, work.ID, &AwardRequest{
		AutoAward: true,
	})
	if err != nil {
		t.Fatalf("Failed to award contract: %v", err)
	}

	if awardResp.ContractID == "" {
		t.Error("Contract ID should not be empty")
	}
	if awardResp.ExecutionToken == "" {
		t.Error("Execution token should not be empty")
	}
	t.Logf("Awarded contract: %s (status: %s, bid: %s)", awardResp.ContractID, awardResp.Status, bidResp.BidID)
}

// TestContractAwardWithBidID tests awarding a specific bid
func TestContractAwardWithBidID(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	// Setup
	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("BidID Award Provider %d", timestamp),
		Capabilities: []string{"bidid-award-test"},
		Endpoint:     fmt.Sprintf("https://bidid-award-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "bidid-award-test",
		Description: "Work for bid ID award test",
		Payload:     map[string]any{"data": "test"},
		Budget:      &Budget{MaxPrice: 100.00},
		ConsumerID:  fmt.Sprintf("bidid-award-consumer-%d", timestamp),
		BidWindowMs: 300000,
	})
	if err != nil {
		t.Fatalf("Failed to submit work: %v", err)
	}

	// Submit bid
	bidResp, err := c.SubmitBidWithAuth(ctx, provider.APIKey, &Bid{
		WorkID:      work.ID,
		Price:       60.00,
		Confidence:  0.85,
		Approach:    "premium processing",
		A2AEndpoint: fmt.Sprintf("https://bidid-award-%d.example.com/a2a", timestamp),
		ExpiresAt:   time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("Failed to submit bid: %v", err)
	}

	// Award contract with specific bid ID
	awardResp, err := c.AwardContract(ctx, work.ID, &AwardRequest{
		BidID: bidResp.BidID,
	})
	if err != nil {
		t.Fatalf("Failed to award contract: %v", err)
	}

	t.Logf("Awarded contract %s with specific bid %s", awardResp.ContractID, bidResp.BidID)
}

// TestContractProgress tests progress updates with execution token
func TestContractProgress(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	// Setup
	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Progress Provider %d", timestamp),
		Capabilities: []string{"progress-test"},
		Endpoint:     fmt.Sprintf("https://progress-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "progress-test",
		Description: "Work for progress test",
		Payload:     map[string]any{"data": "test"},
		Budget:      &Budget{MaxPrice: 100.00},
		ConsumerID:  fmt.Sprintf("progress-consumer-%d", timestamp),
		BidWindowMs: 300000,
	})
	if err != nil {
		t.Fatalf("Failed to submit work: %v", err)
	}

	_, err = c.SubmitBidWithAuth(ctx, provider.APIKey, &Bid{
		WorkID:      work.ID,
		Price:       50.00,
		Confidence:  0.9,
		Approach:    "standard",
		A2AEndpoint: fmt.Sprintf("https://progress-%d.example.com/a2a", timestamp),
		ExpiresAt:   time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("Failed to submit bid: %v", err)
	}

	awardResp, err := c.AwardContract(ctx, work.ID, &AwardRequest{AutoAward: true})
	if err != nil {
		t.Fatalf("Failed to award contract: %v", err)
	}

	// Test progress updates with execution token
	progressSteps := []struct {
		status  string
		percent int
		message string
	}{
		{"in_progress", 10, "Starting task"},
		{"in_progress", 25, "Processing data"},
		{"in_progress", 50, "Halfway complete"},
		{"in_progress", 75, "Almost done"},
		{"in_progress", 90, "Finalizing"},
	}

	for _, step := range progressSteps {
		pct := step.percent
		err := c.UpdateProgressWithToken(ctx, awardResp.ContractID, awardResp.ExecutionToken, &ProgressRequest{
			Status:  step.status,
			Percent: &pct,
			Message: step.message,
		})
		if err != nil {
			t.Errorf("Failed to update progress to %d%%: %v", step.percent, err)
			continue
		}
		t.Logf("Updated progress: %d%% - %s", step.percent, step.message)
	}
}

// TestContractCompletion tests successful contract completion
func TestContractCompletion(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	// Setup
	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Completion Provider %d", timestamp),
		Capabilities: []string{"completion-test"},
		Endpoint:     fmt.Sprintf("https://completion-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "completion-test",
		Description: "Work for completion test",
		Payload:     map[string]any{"data": "test"},
		Budget:      &Budget{MaxPrice: 100.00},
		ConsumerID:  fmt.Sprintf("completion-consumer-%d", timestamp),
		BidWindowMs: 300000,
	})
	if err != nil {
		t.Fatalf("Failed to submit work: %v", err)
	}

	_, err = c.SubmitBidWithAuth(ctx, provider.APIKey, &Bid{
		WorkID:      work.ID,
		Price:       50.00,
		Confidence:  0.9,
		Approach:    "standard",
		A2AEndpoint: fmt.Sprintf("https://completion-%d.example.com/a2a", timestamp),
		ExpiresAt:   time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("Failed to submit bid: %v", err)
	}

	awardResp, err := c.AwardContract(ctx, work.ID, &AwardRequest{AutoAward: true})
	if err != nil {
		t.Fatalf("Failed to award contract: %v", err)
	}

	// Complete contract with execution token
	err = c.CompleteContractWithToken(ctx, awardResp.ContractID, awardResp.ExecutionToken, &CompleteRequest{
		Success:       true,
		ResultSummary: "Task completed successfully",
		Metrics: map[string]any{
			"duration_ms": 1500,
			"tokens":      250,
		},
	})
	if err != nil {
		t.Fatalf("Failed to complete contract: %v", err)
	}

	// Verify completion
	contract, err := c.GetContract(ctx, awardResp.ContractID)
	if err != nil {
		t.Fatalf("Failed to get contract: %v", err)
	}
	if contract.Status != "COMPLETED" {
		t.Errorf("Expected status COMPLETED, got %s", contract.Status)
	}
	t.Logf("Completed contract: %s (status: %s)", contract.ContractID, contract.Status)
}

// TestContractFailure tests contract failure handling
func TestContractFailure(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	// Setup
	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Failure Provider %d", timestamp),
		Capabilities: []string{"failure-test"},
		Endpoint:     fmt.Sprintf("https://failure-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "failure-test",
		Description: "Work for failure test",
		Payload:     map[string]any{"data": "test"},
		Budget:      &Budget{MaxPrice: 100.00},
		ConsumerID:  fmt.Sprintf("failure-consumer-%d", timestamp),
		BidWindowMs: 300000,
	})
	if err != nil {
		t.Fatalf("Failed to submit work: %v", err)
	}

	_, err = c.SubmitBidWithAuth(ctx, provider.APIKey, &Bid{
		WorkID:      work.ID,
		Price:       50.00,
		Confidence:  0.9,
		Approach:    "standard",
		A2AEndpoint: fmt.Sprintf("https://failure-%d.example.com/a2a", timestamp),
		ExpiresAt:   time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("Failed to submit bid: %v", err)
	}

	awardResp, err := c.AwardContract(ctx, work.ID, &AwardRequest{AutoAward: true})
	if err != nil {
		t.Fatalf("Failed to award contract: %v", err)
	}

	// Fail contract with execution token
	err = c.FailContractWithToken(ctx, awardResp.ContractID, awardResp.ExecutionToken, &FailRequest{
		Reason:     "Service unavailable",
		Message:    "External dependency failed",
		ReportedBy: "provider",
	})
	if err != nil {
		t.Fatalf("Failed to fail contract: %v", err)
	}

	// Verify failure
	contract, err := c.GetContract(ctx, awardResp.ContractID)
	if err != nil {
		t.Fatalf("Failed to get contract: %v", err)
	}
	if contract.Status != "FAILED" {
		t.Errorf("Expected status FAILED, got %s", contract.Status)
	}
	t.Logf("Failed contract: %s (status: %s)", contract.ContractID, contract.Status)
}

// TestContractRetrieval tests contract retrieval
func TestContractRetrieval(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	// Setup
	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Retrieval Provider %d", timestamp),
		Capabilities: []string{"retrieval-test"},
		Endpoint:     fmt.Sprintf("https://retrieval-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "retrieval-test",
		Description: "Work for retrieval test",
		Payload:     map[string]any{"data": "test"},
		Budget:      &Budget{MaxPrice: 100.00},
		ConsumerID:  fmt.Sprintf("retrieval-consumer-%d", timestamp),
		BidWindowMs: 300000,
	})
	if err != nil {
		t.Fatalf("Failed to submit work: %v", err)
	}

	_, err = c.SubmitBidWithAuth(ctx, provider.APIKey, &Bid{
		WorkID:      work.ID,
		Price:       50.00,
		Confidence:  0.9,
		Approach:    "standard",
		A2AEndpoint: fmt.Sprintf("https://retrieval-%d.example.com/a2a", timestamp),
		ExpiresAt:   time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("Failed to submit bid: %v", err)
	}

	awardResp, err := c.AwardContract(ctx, work.ID, &AwardRequest{AutoAward: true})
	if err != nil {
		t.Fatalf("Failed to award contract: %v", err)
	}

	// Retrieve contract
	fetched, err := c.GetContract(ctx, awardResp.ContractID)
	if err != nil {
		t.Fatalf("Failed to get contract: %v", err)
	}

	if fetched.ContractID != awardResp.ContractID {
		t.Errorf("Contract ID mismatch: expected %s, got %s", awardResp.ContractID, fetched.ContractID)
	}
	t.Logf("Retrieved contract: %s (status: %s)", fetched.ContractID, fetched.Status)
}

// TestContractFullLifecycle tests complete contract lifecycle
func TestContractFullLifecycle(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	t.Log("=== Contract Full Lifecycle Test ===")

	// Step 1: Setup
	t.Log("Step 1: Setting up provider and work...")
	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Lifecycle Provider %d", timestamp),
		Capabilities: []string{"lifecycle-test"},
		Endpoint:     fmt.Sprintf("https://lifecycle-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "lifecycle-test",
		Description: "Work for lifecycle test",
		Payload:     map[string]any{"data": "important data"},
		Budget:      &Budget{MaxPrice: 100.00},
		ConsumerID:  fmt.Sprintf("lifecycle-consumer-%d", timestamp),
		BidWindowMs: 300000,
	})
	if err != nil {
		t.Fatalf("Failed to submit work: %v", err)
	}
	t.Logf("  Work created: %s", work.ID)

	// Step 2: Bid
	t.Log("Step 2: Submitting bid...")
	bidResp, err := c.SubmitBidWithAuth(ctx, provider.APIKey, &Bid{
		WorkID:      work.ID,
		Price:       50.00,
		Confidence:  0.95,
		Approach:    "comprehensive processing",
		A2AEndpoint: fmt.Sprintf("https://lifecycle-%d.example.com/a2a", timestamp),
		SLA:         &SLA{MaxLatencyMs: 2000, Availability: 0.99},
		ExpiresAt:   time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("Failed to submit bid: %v", err)
	}
	t.Logf("  Bid created: %s", bidResp.BidID)

	// Step 3: Award
	t.Log("Step 3: Awarding contract...")
	awardResp, err := c.AwardContract(ctx, work.ID, &AwardRequest{AutoAward: true})
	if err != nil {
		t.Fatalf("Failed to award contract: %v", err)
	}
	t.Logf("  Contract awarded: %s (status: %s)", awardResp.ContractID, awardResp.Status)

	// Step 4: Progress updates
	t.Log("Step 4: Updating progress...")
	for i := 25; i <= 75; i += 25 {
		pct := i
		err := c.UpdateProgressWithToken(ctx, awardResp.ContractID, awardResp.ExecutionToken, &ProgressRequest{
			Status:  "in_progress",
			Percent: &pct,
			Message: fmt.Sprintf("Progress at %d%%", i),
		})
		if err != nil {
			t.Logf("  Progress update to %d%% failed: %v", i, err)
		} else {
			t.Logf("  Progress: %d%%", i)
		}
	}

	// Step 5: Verify contract state
	t.Log("Step 5: Verifying contract state...")
	fetched, err := c.GetContract(ctx, awardResp.ContractID)
	if err != nil {
		t.Logf("  Contract retrieval failed: %v", err)
	} else {
		t.Logf("  Contract state: %s", fetched.Status)
	}

	// Step 6: Complete
	t.Log("Step 6: Completing contract...")
	err = c.CompleteContractWithToken(ctx, awardResp.ContractID, awardResp.ExecutionToken, &CompleteRequest{
		Success:       true,
		ResultSummary: "Lifecycle test completed successfully",
		Metrics: map[string]any{
			"duration_ms": 2500,
			"processed":   true,
		},
	})
	if err != nil {
		t.Fatalf("Failed to complete contract: %v", err)
	}

	// Verify final state
	final, err := c.GetContract(ctx, awardResp.ContractID)
	if err != nil {
		t.Fatalf("Failed to get final contract state: %v", err)
	}
	t.Logf("  Contract completed: %s (status: %s)", final.ContractID, final.Status)

	t.Log("=== Lifecycle Test Complete ===")
}

// TestContractMultipleAwards tests multiple contract awards
func TestContractMultipleAwards(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Multi-Award Provider %d", timestamp),
		Capabilities: []string{"multi-award-test"},
		Endpoint:     fmt.Sprintf("https://multi-award-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	contractCount := 5
	for i := 0; i < contractCount; i++ {
		work, err := c.SubmitWork(ctx, &WorkSpec{
			Category:    "multi-award-test",
			Description: fmt.Sprintf("Work %d for multi-award test", i),
			Payload:     map[string]any{"index": i},
			Budget:      &Budget{MaxPrice: 100.00},
			ConsumerID:  fmt.Sprintf("multi-award-consumer-%d", timestamp),
			BidWindowMs: 300000,
		})
		if err != nil {
			t.Fatalf("Failed to create work %d: %v", i, err)
		}

		_, err = c.SubmitBidWithAuth(ctx, provider.APIKey, &Bid{
			WorkID:      work.ID,
			Price:       25.00 + float64(i*10),
			Confidence:  0.9,
			Approach:    "standard",
			A2AEndpoint: fmt.Sprintf("https://multi-award-%d.example.com/a2a", timestamp),
			ExpiresAt:   time.Now().Add(1 * time.Hour).Format(time.RFC3339),
		})
		if err != nil {
			t.Fatalf("Failed to submit bid %d: %v", i, err)
		}

		awardResp, err := c.AwardContract(ctx, work.ID, &AwardRequest{AutoAward: true})
		if err != nil {
			t.Errorf("Failed to award contract %d: %v", i, err)
			continue
		}
		t.Logf("Contract %d: %s", i, awardResp.ContractID)
	}

	t.Logf("Created %d contracts", contractCount)
}

// TestContractNonExistent tests handling of non-existent contract
func TestContractNonExistent(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()

	_, err := c.GetContract(ctx, "non-existent-contract-id-12345")
	if err == nil {
		t.Log("Non-existent contract returned without error (may return empty)")
	} else {
		t.Logf("Non-existent contract handled correctly: %v", err)
	}
}

// TestContractUnauthorizedProgress tests that progress requires execution token
func TestContractUnauthorizedProgress(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	// Setup
	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Unauth Progress Provider %d", timestamp),
		Capabilities: []string{"unauth-progress-test"},
		Endpoint:     fmt.Sprintf("https://unauth-progress-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "unauth-progress-test",
		Description: "Work for unauthorized progress test",
		Payload:     map[string]any{"data": "test"},
		Budget:      &Budget{MaxPrice: 100.00},
		ConsumerID:  fmt.Sprintf("unauth-progress-consumer-%d", timestamp),
		BidWindowMs: 300000,
	})
	if err != nil {
		t.Fatalf("Failed to submit work: %v", err)
	}

	_, err = c.SubmitBidWithAuth(ctx, provider.APIKey, &Bid{
		WorkID:      work.ID,
		Price:       50.00,
		Confidence:  0.9,
		Approach:    "standard",
		A2AEndpoint: fmt.Sprintf("https://unauth-progress-%d.example.com/a2a", timestamp),
		ExpiresAt:   time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("Failed to submit bid: %v", err)
	}

	awardResp, err := c.AwardContract(ctx, work.ID, &AwardRequest{AutoAward: true})
	if err != nil {
		t.Fatalf("Failed to award contract: %v", err)
	}

	// Try to update progress with wrong token
	pct := 50
	err = c.UpdateProgressWithToken(ctx, awardResp.ContractID, "wrong-token-12345", &ProgressRequest{
		Status:  "in_progress",
		Percent: &pct,
		Message: "This should fail",
	})
	if err == nil {
		t.Error("Expected error for unauthorized progress update")
	} else {
		t.Logf("Unauthorized progress correctly rejected: %v", err)
	}
}
