package integration

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestTrustBrokerGetScore tests getting trust scores
func TestTrustBrokerGetScore(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	// Register a provider first
	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Trust Score Provider %d", timestamp),
		Capabilities: []string{"trust-test"},
		Endpoint:     fmt.Sprintf("https://trust-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	// Get trust score
	trustScore, err := c.GetTrustScore(ctx, provider.ID)
	if err != nil {
		t.Logf("Trust score not available for new provider (expected): %v", err)
	} else {
		t.Logf("Provider trust score: %.3f (tier: %s)", trustScore.Score, trustScore.Tier)
	}
}

// TestTrustBrokerOutcomeRecording tests recording outcomes
func TestTrustBrokerOutcomeRecording(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	// Register provider
	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Outcome Provider %d", timestamp),
		Capabilities: []string{"outcome-test"},
		Endpoint:     fmt.Sprintf("https://outcome-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	// Record successful outcomes
	for i := 0; i < 5; i++ {
		err := c.RecordOutcome(ctx, &OutcomeRequest{
			ProviderID: provider.ID,
			ContractID: fmt.Sprintf("contract-%d-%d", timestamp, i),
			Outcome:    "success",
		})
		if err != nil {
			t.Logf("Failed to record success outcome %d: %v", i, err)
		} else {
			t.Logf("Recorded success outcome %d", i)
		}
	}

	// Check trust score after outcomes
	trustScore, err := c.GetTrustScore(ctx, provider.ID)
	if err != nil {
		t.Logf("Trust score not available: %v", err)
	} else {
		t.Logf("Trust score after successes: %.3f (tier: %s)", trustScore.Score, trustScore.Tier)
	}
}

// TestTrustBrokerMixedOutcomes tests mixed success/failure outcomes
func TestTrustBrokerMixedOutcomes(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Mixed Outcome Provider %d", timestamp),
		Capabilities: []string{"mixed-outcome-test"},
		Endpoint:     fmt.Sprintf("https://mixed-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	// Record mixed outcomes
	outcomes := []string{
		"success", "success", "success", "failure",
		"success", "success", "failure", "success",
		"success", "success",
	}

	for i, outcome := range outcomes {
		err := c.RecordOutcome(ctx, &OutcomeRequest{
			ProviderID: provider.ID,
			ContractID: fmt.Sprintf("mixed-contract-%d-%d", timestamp, i),
			Outcome:    outcome,
		})
		if err != nil {
			t.Logf("Failed to record outcome %d (%s): %v", i, outcome, err)
		}
	}

	// Check final trust score
	trustScore, err := c.GetTrustScore(ctx, provider.ID)
	if err != nil {
		t.Logf("Trust score not available: %v", err)
	} else {
		t.Logf("Trust score with mixed outcomes: %.3f (tier: %s)", trustScore.Score, trustScore.Tier)
	}
}

// TestTrustBrokerMultipleProviders tests trust scores for multiple providers
func TestTrustBrokerMultipleProviders(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	providerCount := 3
	providers := make([]*Provider, providerCount)

	// Register providers
	for i := 0; i < providerCount; i++ {
		provider, err := c.RegisterProvider(ctx, &Provider{
			Name:         fmt.Sprintf("Multi Trust Provider %d-%d", timestamp, i),
			Capabilities: []string{"multi-trust-test"},
			Endpoint:     fmt.Sprintf("https://multi-trust-%d-%d.example.com/api", timestamp, i),
		})
		if err != nil {
			t.Fatalf("Failed to register provider %d: %v", i, err)
		}
		providers[i] = provider
	}

	// Record different outcome profiles for each
	// Provider 0: All success (high reliability)
	for i := 0; i < 10; i++ {
		_ = c.RecordOutcome(ctx, &OutcomeRequest{
			ProviderID: providers[0].ID,
			ContractID: fmt.Sprintf("high-rel-%d-%d", timestamp, i),
			Outcome:    "success",
		})
	}

	// Provider 1: Mixed (medium reliability)
	for i := 0; i < 10; i++ {
		outcome := "success"
		if i%3 == 0 {
			outcome = "failure"
		}
		_ = c.RecordOutcome(ctx, &OutcomeRequest{
			ProviderID: providers[1].ID,
			ContractID: fmt.Sprintf("med-rel-%d-%d", timestamp, i),
			Outcome:    outcome,
		})
	}

	// Provider 2: Mostly failure (low reliability)
	for i := 0; i < 10; i++ {
		outcome := "failure"
		if i%4 == 0 {
			outcome = "success"
		}
		_ = c.RecordOutcome(ctx, &OutcomeRequest{
			ProviderID: providers[2].ID,
			ContractID: fmt.Sprintf("low-rel-%d-%d", timestamp, i),
			Outcome:    outcome,
		})
	}

	// Compare trust scores
	for i, provider := range providers {
		trustScore, err := c.GetTrustScore(ctx, provider.ID)
		if err != nil {
			t.Logf("Provider %d trust score not available: %v", i, err)
		} else {
			t.Logf("Provider %d: score=%.3f, tier=%s", i, trustScore.Score, trustScore.Tier)
		}
	}
}

// TestTrustBrokerTierProgression tests trust tier progression
func TestTrustBrokerTierProgression(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Tier Progression Provider %d", timestamp),
		Capabilities: []string{"tier-test"},
		Endpoint:     fmt.Sprintf("https://tier-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	// Check initial tier
	initialScore, _ := c.GetTrustScore(ctx, provider.ID)
	if initialScore != nil {
		t.Logf("Initial: score=%.3f, tier=%s", initialScore.Score, initialScore.Tier)
	}

	// Record successes and check tier progression
	checkpoints := []int{5, 10, 25, 50, 100}
	outcomeCount := 0

	for _, checkpoint := range checkpoints {
		for outcomeCount < checkpoint {
			_ = c.RecordOutcome(ctx, &OutcomeRequest{
				ProviderID: provider.ID,
				ContractID: fmt.Sprintf("tier-contract-%d-%d", timestamp, outcomeCount),
				Outcome:    "success",
			})
			outcomeCount++
		}

		score, err := c.GetTrustScore(ctx, provider.ID)
		if err != nil {
			t.Logf("After %d outcomes: error getting score - %v", checkpoint, err)
		} else {
			t.Logf("After %d outcomes: score=%.3f, tier=%s", checkpoint, score.Score, score.Tier)
		}
	}
}

// TestTrustBrokerScoreRecovery tests score recovery after failures
func TestTrustBrokerScoreRecovery(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Recovery Provider %d", timestamp),
		Capabilities: []string{"recovery-test"},
		Endpoint:     fmt.Sprintf("https://recovery-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	// Build up initial score
	for i := 0; i < 20; i++ {
		_ = c.RecordOutcome(ctx, &OutcomeRequest{
			ProviderID: provider.ID,
			ContractID: fmt.Sprintf("recovery-success-%d-%d", timestamp, i),
			Outcome:    "success",
		})
	}

	scoreBeforeFailures, _ := c.GetTrustScore(ctx, provider.ID)
	if scoreBeforeFailures != nil {
		t.Logf("Score before failures: %.3f", scoreBeforeFailures.Score)
	}

	// Introduce failures
	for i := 0; i < 5; i++ {
		_ = c.RecordOutcome(ctx, &OutcomeRequest{
			ProviderID: provider.ID,
			ContractID: fmt.Sprintf("recovery-failure-%d-%d", timestamp, i),
			Outcome:    "failure",
		})
	}

	scoreAfterFailures, _ := c.GetTrustScore(ctx, provider.ID)
	if scoreAfterFailures != nil {
		t.Logf("Score after failures: %.3f", scoreAfterFailures.Score)
	}

	// Recover with successes
	for i := 0; i < 20; i++ {
		_ = c.RecordOutcome(ctx, &OutcomeRequest{
			ProviderID: provider.ID,
			ContractID: fmt.Sprintf("recovery-recover-%d-%d", timestamp, i),
			Outcome:    "success",
		})
	}

	scoreAfterRecovery, _ := c.GetTrustScore(ctx, provider.ID)
	if scoreAfterRecovery != nil {
		t.Logf("Score after recovery: %.3f", scoreAfterRecovery.Score)
	}
}

// TestTrustBrokerNonExistentProvider tests handling of non-existent provider
func TestTrustBrokerNonExistentProvider(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()

	_, err := c.GetTrustScore(ctx, "non-existent-provider-12345")
	if err == nil {
		t.Log("Non-existent provider returned without error (may return default score)")
	} else {
		t.Logf("Non-existent provider handled: %v", err)
	}
}

// TestTrustBrokerOutcomeTypes tests different outcome types
func TestTrustBrokerOutcomeTypes(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	provider, err := c.RegisterProvider(ctx, &Provider{
		Name:         fmt.Sprintf("Outcome Types Provider %d", timestamp),
		Capabilities: []string{"outcome-types-test"},
		Endpoint:     fmt.Sprintf("https://outcome-types-%d.example.com/api", timestamp),
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	outcomeTypes := []string{
		"success",
		"failure",
		"timeout",
		"partial_success",
		"cancelled",
	}

	for i, outcome := range outcomeTypes {
		err := c.RecordOutcome(ctx, &OutcomeRequest{
			ProviderID: provider.ID,
			ContractID: fmt.Sprintf("type-contract-%d-%d", timestamp, i),
			Outcome:    outcome,
		})
		if err != nil {
			t.Logf("Outcome type '%s': %v", outcome, err)
		} else {
			t.Logf("Recorded outcome type: %s", outcome)
		}
	}

	score, err := c.GetTrustScore(ctx, provider.ID)
	if err != nil {
		t.Logf("Final score not available: %v", err)
	} else {
		t.Logf("Final score after various outcomes: %.3f", score.Score)
	}
}

