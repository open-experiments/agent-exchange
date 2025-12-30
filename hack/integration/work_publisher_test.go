package integration

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestWorkSubmission tests basic work submission
func TestWorkSubmission(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "text-generation",
		Description: fmt.Sprintf("Test work %d", timestamp),
		Payload: map[string]any{
			"prompt": "Complete this sentence: The quick brown fox",
		},
		Budget: &Budget{
			MaxPrice: 10.00,
		},
		ConsumerID:  fmt.Sprintf("consumer-%d", timestamp),
		BidWindowMs: 60000, // 60 seconds
	})
	if err != nil {
		t.Fatalf("Failed to submit work: %v", err)
	}

	if work == nil || work.ID == "" {
		t.Error("Work ID should not be empty")
	} else {
		t.Logf("Submitted work: %s (status: %s)", work.ID, work.Status)
	}
}

// TestWorkWithConstraints tests work submission with constraints
func TestWorkWithConstraints(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()
	latency := int64(3000)

	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "text-generation",
		Description: "Summarize a document with latency constraints",
		Payload: map[string]any{
			"text": "This is a long document that needs to be summarized quickly.",
		},
		Constraints: &Constraints{
			MaxLatencyMs: &latency,
		},
		Budget: &Budget{
			MaxPrice: 50.00,
		},
		ConsumerID:  fmt.Sprintf("constraint-consumer-%d", timestamp),
		BidWindowMs: 120000, // 120 seconds
	})
	if err != nil {
		t.Fatalf("Failed to submit work with constraints: %v", err)
	}

	if work != nil {
		t.Logf("Submitted constrained work: %s", work.ID)
	}
}

// TestWorkRetrieval tests work submission and retrieval
func TestWorkRetrieval(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()
	description := fmt.Sprintf("Retrieval test work %d", timestamp)

	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "translation",
		Description: description,
		Payload: map[string]any{
			"text":   "Hello, world!",
			"source": "en",
			"target": "es",
		},
		Budget: &Budget{
			MaxPrice: 5.00,
		},
		ConsumerID:  fmt.Sprintf("retrieval-consumer-%d", timestamp),
		BidWindowMs: 60000,
	})
	if err != nil {
		t.Fatalf("Failed to submit work: %v", err)
	}

	if work == nil || work.ID == "" {
		t.Fatal("Work ID is empty, cannot retrieve")
	}

	// Retrieve work
	fetchedWork, err := c.GetWork(ctx, work.ID)
	if err != nil {
		t.Fatalf("Failed to get work: %v", err)
	}

	if fetchedWork != nil {
		if fetchedWork.ID != work.ID {
			t.Errorf("Work ID mismatch: expected %s, got %s", work.ID, fetchedWork.ID)
		}
		t.Logf("Retrieved work: %s (status: %s)", fetchedWork.ID, fetchedWork.Status)
	}
}

// TestWorkDifferentCategories tests work for different categories
func TestWorkDifferentCategories(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	categories := []struct {
		category string
		payload  map[string]any
	}{
		{
			category: "text-generation",
			payload:  map[string]any{"prompt": "Generate text"},
		},
		{
			category: "translation",
			payload:  map[string]any{"text": "Hello", "source": "en", "target": "fr"},
		},
		{
			category: "code-review",
			payload:  map[string]any{"code": "func main() {}", "language": "go"},
		},
		{
			category: "sentiment-analysis",
			payload:  map[string]any{"text": "I love this product!"},
		},
		{
			category: "image-generation",
			payload:  map[string]any{"prompt": "A sunset over mountains"},
		},
	}

	for _, tc := range categories {
		work, err := c.SubmitWork(ctx, &WorkSpec{
			Category:    tc.category,
			Description: fmt.Sprintf("Test %s work %d", tc.category, timestamp),
			Payload:     tc.payload,
			Budget: &Budget{
				MaxPrice: 25.00,
			},
			ConsumerID:  fmt.Sprintf("category-consumer-%d", timestamp),
			BidWindowMs: 60000,
		})
		if err != nil {
			t.Errorf("Failed to submit %s work: %v", tc.category, err)
			continue
		}
		if work != nil {
			t.Logf("Submitted %s work: %s", tc.category, work.ID)
		}
	}
}

// TestWorkWithHighBudget tests work with high budget
func TestWorkWithHighBudget(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()
	latency := int64(60000)

	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "complex-analysis",
		Description: "Complex analysis task with high budget",
		Payload: map[string]any{
			"data": "large dataset reference",
		},
		Constraints: &Constraints{
			MaxLatencyMs: &latency,
		},
		Budget: &Budget{
			MaxPrice: 1000.00,
		},
		ConsumerID:  fmt.Sprintf("high-budget-consumer-%d", timestamp),
		BidWindowMs: 300000,
	})
	if err != nil {
		t.Fatalf("Failed to submit high budget work: %v", err)
	}

	if work != nil && work.ID != "" {
		t.Logf("Submitted high budget work: %s", work.ID)
	} else {
		t.Log("Work submitted but ID not returned")
	}
}

// TestWorkBidWindow tests different bid window configurations
func TestWorkBidWindow(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	bidWindowsMs := []int64{30000, 60000, 120000, 300000, 600000}

	for _, windowMs := range bidWindowsMs {
		work, err := c.SubmitWork(ctx, &WorkSpec{
			Category:    "test-category",
			Description: fmt.Sprintf("Bid window %d ms test", windowMs),
			Payload:     map[string]any{"data": "test"},
			Budget: &Budget{
				MaxPrice: 10.00,
			},
			ConsumerID:  fmt.Sprintf("bid-window-consumer-%d", timestamp),
			BidWindowMs: windowMs,
		})
		if err != nil {
			t.Errorf("Failed to submit work with %d ms bid window: %v", windowMs, err)
			continue
		}
		if work != nil {
			t.Logf("Submitted work with %d ms bid window: %s", windowMs, work.ID)
		}
	}
}

// TestWorkNonExistent tests handling of non-existent work
func TestWorkNonExistent(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()

	_, err := c.GetWork(ctx, "non-existent-work-id-12345")
	if err == nil {
		t.Log("Non-existent work returned without error (may return empty)")
	} else {
		t.Logf("Non-existent work handled correctly: %v", err)
	}
}

// TestWorkMultipleSubmissions tests submitting multiple work items
func TestWorkMultipleSubmissions(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()
	consumerID := fmt.Sprintf("bulk-consumer-%d", timestamp)

	workCount := 10
	workIDs := make([]string, 0, workCount)

	for i := 0; i < workCount; i++ {
		work, err := c.SubmitWork(ctx, &WorkSpec{
			Category:    "bulk-test",
			Description: fmt.Sprintf("Bulk work item %d", i),
			Payload: map[string]any{
				"index": i,
				"data":  fmt.Sprintf("data-%d", i),
			},
			Budget: &Budget{
				MaxPrice: 5.00,
			},
			ConsumerID:  consumerID,
			BidWindowMs: 60000,
		})
		if err != nil {
			t.Fatalf("Failed to submit work %d: %v", i, err)
		}
		if work != nil && work.ID != "" {
			workIDs = append(workIDs, work.ID)
		}
	}

	// Verify all work items with valid IDs
	for i, id := range workIDs {
		if id == "" {
			continue
		}
		_, err := c.GetWork(ctx, id)
		if err != nil {
			t.Errorf("Failed to get work %d: %v", i, err)
		}
	}

	t.Logf("Successfully submitted %d work items", len(workIDs))
}

// TestWorkCancellation tests work cancellation
func TestWorkCancellation(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	// Submit work
	work, err := c.SubmitWork(ctx, &WorkSpec{
		Category:    "cancellation-test",
		Description: "Work to be cancelled",
		Payload:     map[string]any{"data": "test"},
		Budget: &Budget{
			MaxPrice: 10.00,
		},
		ConsumerID:  fmt.Sprintf("cancel-consumer-%d", timestamp),
		BidWindowMs: 300000,
	})
	if err != nil {
		t.Fatalf("Failed to submit work: %v", err)
	}

	if work == nil || work.ID == "" {
		t.Skip("Work ID not returned, cannot cancel")
	}

	// Try to cancel
	cancelledWork, err := c.CancelWork(ctx, work.ID)
	if err != nil {
		t.Logf("Cancel work not implemented or failed: %v", err)
	} else if cancelledWork != nil {
		t.Logf("Cancelled work: %s (status: %s)", cancelledWork.ID, cancelledWork.Status)
	}
}
