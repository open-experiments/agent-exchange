package service

import (
	"context"
	"testing"

	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/model"
	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/store"
)

func TestPublishWork(t *testing.T) {
	tests := []struct {
		name        string
		consumerID  string
		category    string
		description string
		maxPrice    float64
		wantErr     bool
	}{
		{
			name:        "valid work submission",
			consumerID:  "tenant_001",
			category:    "general",
			description: "Test work",
			maxPrice:    100.0,
			wantErr:     false,
		},
		{
			name:        "empty category",
			consumerID:  "tenant_001",
			category:    "",
			description: "Test work",
			maxPrice:    100.0,
			wantErr:     true,
		},
		{
			name:        "negative price",
			consumerID:  "tenant_001",
			category:    "general",
			description: "Test work",
			maxPrice:    -10.0,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := store.NewMemoryStore()
			svc := New(st, "")

			req := model.WorkSubmission{
				Category:    tt.category,
				Description: tt.description,
				Budget: model.Budget{
					MaxPrice:    tt.maxPrice,
					BidStrategy: "balanced",
				},
			}

			ctx := context.Background()
			resp, err := svc.PublishWork(ctx, tt.consumerID, req)

			if tt.wantErr {
				if err == nil {
					t.Errorf("PublishWork() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("PublishWork() unexpected error: %v", err)
			}

			if resp.WorkID == "" {
				t.Error("PublishWork() returned response with empty WorkID")
			}

			if resp.Status != string(model.WorkStateOpen) {
				t.Errorf("PublishWork() status = %v, want %v", resp.Status, model.WorkStateOpen)
			}
		})
	}
}

func TestGetWork(t *testing.T) {
	st := store.NewMemoryStore()
	svc := New(st, "")
	ctx := context.Background()

	req := model.WorkSubmission{
		Category:    "general",
		Description: "Test work",
		Budget: model.Budget{
			MaxPrice:    100.0,
			BidStrategy: "balanced",
		},
	}

	submitted, err := svc.PublishWork(ctx, "tenant_001", req)
	if err != nil {
		t.Fatalf("PublishWork() error: %v", err)
	}

	t.Run("get existing work", func(t *testing.T) {
		spec, err := svc.GetWork(ctx, submitted.WorkID)
		if err != nil {
			t.Fatalf("GetWork() error: %v", err)
		}

		if spec.ID != submitted.WorkID {
			t.Errorf("GetWork() ID = %v, want %v", spec.ID, submitted.WorkID)
		}
	})

	t.Run("get non-existent work", func(t *testing.T) {
		_, err := svc.GetWork(ctx, "work_nonexistent")
		if err == nil {
			t.Error("GetWork() expected error for non-existent work, got nil")
		}
	})
}

func TestCancelWork(t *testing.T) {
	st := store.NewMemoryStore()
	svc := New(st, "")
	ctx := context.Background()

	req := model.WorkSubmission{
		Category:    "general",
		Description: "Test work",
		Budget: model.Budget{
			MaxPrice:    100.0,
			BidStrategy: "balanced",
		},
	}

	submitted, err := svc.PublishWork(ctx, "tenant_001", req)
	if err != nil {
		t.Fatalf("PublishWork() error: %v", err)
	}

	t.Run("cancel open work", func(t *testing.T) {
		spec, err := svc.CancelWork(ctx, submitted.WorkID, "tenant_001")
		if err != nil {
			t.Fatalf("CancelWork() error: %v", err)
		}

		if spec.State != model.WorkStateCancelled {
			t.Errorf("CancelWork() state = %v, want %v", spec.State, model.WorkStateCancelled)
		}
	})

	t.Run("cancel non-existent work", func(t *testing.T) {
		_, err := svc.CancelWork(ctx, "work_nonexistent", "tenant_001")
		if err == nil {
			t.Error("CancelWork() expected error for non-existent work, got nil")
		}
	})
}

func TestOnBidSubmitted(t *testing.T) {
	st := store.NewMemoryStore()
	svc := New(st, "")
	ctx := context.Background()

	req := model.WorkSubmission{
		Category:    "general",
		Description: "Test work",
		Budget: model.Budget{
			MaxPrice:    100.0,
			BidStrategy: "balanced",
		},
	}

	submitted, err := svc.PublishWork(ctx, "tenant_001", req)
	if err != nil {
		t.Fatalf("PublishWork() error: %v", err)
	}

	t.Run("record bid on existing work", func(t *testing.T) {
		err := svc.OnBidSubmitted(ctx, submitted.WorkID, "bid_001")
		if err != nil {
			t.Fatalf("OnBidSubmitted() error: %v", err)
		}

		spec, err := svc.GetWork(ctx, submitted.WorkID)
		if err != nil {
			t.Fatalf("GetWork() error: %v", err)
		}

		if spec.BidsReceived != 1 {
			t.Errorf("OnBidSubmitted() bidsReceived = %v, want 1", spec.BidsReceived)
		}
	})

	t.Run("record bid on non-existent work", func(t *testing.T) {
		err := svc.OnBidSubmitted(ctx, "work_nonexistent", "bid_002")
		if err == nil {
			t.Error("OnBidSubmitted() expected error for non-existent work, got nil")
		}
	})
}

func TestCloseBidWindow(t *testing.T) {
	st := store.NewMemoryStore()
	svc := New(st, "")
	ctx := context.Background()

	req := model.WorkSubmission{
		Category:    "general",
		Description: "Test work",
		Budget: model.Budget{
			MaxPrice:    100.0,
			BidStrategy: "balanced",
		},
	}

	submitted, err := svc.PublishWork(ctx, "tenant_001", req)
	if err != nil {
		t.Fatalf("PublishWork() error: %v", err)
	}

	t.Run("close bid window", func(t *testing.T) {
		err := svc.CloseBidWindow(ctx, submitted.WorkID)
		if err != nil {
			t.Fatalf("CloseBidWindow() error: %v", err)
		}

		spec, err := svc.GetWork(ctx, submitted.WorkID)
		if err != nil {
			t.Fatalf("GetWork() error: %v", err)
		}

		if spec.State != model.WorkStateEvaluating {
			t.Errorf("CloseBidWindow() state = %v, want %v", spec.State, model.WorkStateEvaluating)
		}
	})

	t.Run("close bid window on non-existent work", func(t *testing.T) {
		err := svc.CloseBidWindow(ctx, "work_nonexistent")
		if err == nil {
			t.Error("CloseBidWindow() expected error for non-existent work, got nil")
		}
	})
}

func TestBidWindowDefaults(t *testing.T) {
	st := store.NewMemoryStore()
	svc := New(st, "")
	ctx := context.Background()

	tests := []struct {
		name        string
		bidWindowMs int64
		wantMinimum int64
	}{
		{
			name:        "default bid window",
			bidWindowMs: 0,
			wantMinimum: DefaultBidWindowMs,
		},
		{
			name:        "custom bid window",
			bidWindowMs: 60000,
			wantMinimum: 60000,
		},
		{
			name:        "too short bid window",
			bidWindowMs: 1000,
			wantMinimum: MinBidWindowMs,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := model.WorkSubmission{
				Category:    "general",
				Description: "Test work",
				Budget: model.Budget{
					MaxPrice:    100.0,
					BidStrategy: "balanced",
				},
				BidWindowMs: tt.bidWindowMs,
			}

			resp, err := svc.PublishWork(ctx, "tenant_001", req)
			if err != nil {
				t.Fatalf("PublishWork() error: %v", err)
			}

			spec, _ := svc.GetWork(ctx, resp.WorkID)
			if spec.BidWindowMs < tt.wantMinimum {
				t.Errorf("BidWindowMs = %v, want minimum %v", spec.BidWindowMs, tt.wantMinimum)
			}
		})
	}
}
