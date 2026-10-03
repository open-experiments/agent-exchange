package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/clients"
	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/model"
	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/store"
	"github.com/parlakisik/agent-exchange/internal/events"
)

var (
	ErrInvalidWorkSpec = errors.New("invalid work specification")
	ErrWorkNotFound    = errors.New("work not found")
	ErrInvalidState    = errors.New("invalid work state")
	ErrVersionConflict = errors.New("version conflict: concurrent modification")
	ErrNotAuthorized   = errors.New("not authorized")
	DefaultBidWindowMs = int64(30000)  // 30 seconds
	MaxBidWindowMs     = int64(300000) // 5 minutes
	MinBidWindowMs     = int64(5000)   // 5 seconds
)

type Service struct {
	store            store.WorkStore
	providerRegistry *clients.ProviderRegistryClient
	events           *events.Publisher
}

func New(st store.WorkStore, providerRegistryURL string, pub *events.Publisher) *Service {
	if pub == nil {
		pub = events.NewPublisher("aex-work-publisher")
	}
	return &Service{
		store:            st,
		providerRegistry: clients.NewProviderRegistryClient(providerRegistryURL),
		events:           pub,
	}
}

// PublishWork submits a new work specification
func (s *Service) PublishWork(ctx context.Context, consumerID string, req model.WorkSubmission) (model.WorkResponse, error) {
	// 1. Validate work spec
	if err := s.validateWorkSpec(req); err != nil {
		return model.WorkResponse{}, fmt.Errorf("%w: %v", ErrInvalidWorkSpec, err)
	}

	// 2. Set defaults
	if req.BidWindowMs == 0 {
		req.BidWindowMs = DefaultBidWindowMs
	}
	if req.BidWindowMs < MinBidWindowMs {
		req.BidWindowMs = MinBidWindowMs
	}
	if req.BidWindowMs > MaxBidWindowMs {
		req.BidWindowMs = MaxBidWindowMs
	}
	if req.Budget.BidStrategy == "" {
		req.Budget.BidStrategy = "balanced"
	}

	// 3. Create work record
	now := time.Now().UTC()
	workID := generateWorkID()

	work := model.WorkSpec{
		ID:              workID,
		ConsumerID:      consumerID,
		Category:        req.Category,
		Description:     req.Description,
		Constraints:     req.Constraints,
		Budget:          req.Budget,
		SuccessCriteria: req.SuccessCriteria,
		BidWindowMs:     req.BidWindowMs,
		Payload:         req.Payload,
		State:           model.WorkStateOpen,
		CreatedAt:       now,
		BidWindowEndsAt: now.Add(time.Duration(req.BidWindowMs) * time.Millisecond),
	}

	// 4. Get subscribed providers
	providers, err := s.providerRegistry.GetSubscribedProviders(ctx, req.Category)
	if err != nil {
		slog.WarnContext(ctx, "failed to get providers", "error", err)
		providers = []model.Provider{} // Continue even if provider lookup fails
	}

	work.ProvidersNotified = len(providers)

	// 5. Persist to Firestore
	if err := s.store.SaveWork(ctx, work); err != nil {
		return model.WorkResponse{}, fmt.Errorf("save work: %w", err)
	}

	// 6. Broadcast work opportunity (via event for now, webhooks later)
	_ = s.events.Publish(ctx, events.EventWorkSubmitted, map[string]any{
		"work_id":            work.ID,
		"domain":             work.Category,
		"consumer_id":        work.ConsumerID,
		"providers_notified": len(providers),
		"bid_window_ends_at": work.BidWindowEndsAt.Format(time.RFC3339Nano),
		"budget":             work.Budget,
	})

	slog.InfoContext(ctx, "work_published",
		"work_id", work.ID,
		"category", work.Category,
		"providers_notified", len(providers),
	)

	return model.WorkResponse{
		WorkID:            work.ID,
		Status:            string(work.State),
		BidWindowEndsAt:   work.BidWindowEndsAt,
		ProvidersNotified: len(providers),
		CreatedAt:         work.CreatedAt,
	}, nil
}

// GetWork retrieves a work specification
func (s *Service) GetWork(ctx context.Context, workID string) (model.WorkSpec, error) {
	work, err := s.store.GetWork(ctx, workID)
	if err != nil {
		if errors.Is(err, store.ErrWorkNotFound) {
			return model.WorkSpec{}, ErrWorkNotFound
		}
		return model.WorkSpec{}, fmt.Errorf("get work: %w", err)
	}
	return work, nil
}

// CancelWork cancels a work request. Uses optimistic concurrency.
func (s *Service) CancelWork(ctx context.Context, workID, consumerID string) (model.WorkSpec, error) {
	const maxRetries = 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		work, err := s.store.GetWork(ctx, workID)
		if err != nil {
			if errors.Is(err, store.ErrWorkNotFound) {
				return model.WorkSpec{}, ErrWorkNotFound
			}
			return model.WorkSpec{}, fmt.Errorf("get work: %w", err)
		}

		if work.ConsumerID != consumerID {
			return model.WorkSpec{}, ErrNotAuthorized
		}

		if work.State != model.WorkStateOpen && work.State != model.WorkStateEvaluating {
			return model.WorkSpec{}, fmt.Errorf("%w: cannot cancel work in state %s", ErrInvalidState, work.State)
		}

		now := time.Now().UTC()
		work.State = model.WorkStateCancelled
		work.CompletedAt = &now

		if err := s.store.UpdateWork(ctx, work); err != nil {
			if errors.Is(err, store.ErrVersionConflict) {
				continue
			}
			return model.WorkSpec{}, fmt.Errorf("update work: %w", err)
		}

		_ = s.events.Publish(ctx, events.EventWorkCancelled, map[string]any{
			"work_id":      work.ID,
			"consumer_id":  work.ConsumerID,
			"reason":       "consumer_requested",
			"cancelled_at": now.Format(time.RFC3339Nano),
		})

		slog.InfoContext(ctx, "work_cancelled", "work_id", work.ID)
		return work, nil
	}
	return model.WorkSpec{}, ErrVersionConflict
}

// OnBidSubmitted handles bid submission notification.
// Uses optimistic concurrency with retry to prevent lost updates from
// concurrent bid submissions.
func (s *Service) OnBidSubmitted(ctx context.Context, workID, bidID string) error {
	const maxRetries = 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		work, err := s.store.GetWork(ctx, workID)
		if err != nil {
			return err
		}

		work.BidsReceived++

		if err := s.store.UpdateWork(ctx, work); err != nil {
			if errors.Is(err, store.ErrVersionConflict) {
				slog.DebugContext(ctx, "version conflict on bid submit, retrying",
					"work_id", workID, "attempt", attempt+1)
				continue
			}
			return err
		}

		slog.InfoContext(ctx, "bid_received",
			"work_id", workID,
			"bid_id", bidID,
			"total_bids", work.BidsReceived,
		)
		return nil
	}
	return ErrVersionConflict
}

// CloseBidWindow closes the bid window and transitions to evaluation.
// Uses optimistic concurrency with retry to prevent race with concurrent
// bid submissions.
func (s *Service) CloseBidWindow(ctx context.Context, workID string) error {
	const maxRetries = 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		work, err := s.store.GetWork(ctx, workID)
		if err != nil {
			return err
		}

		if work.State != model.WorkStateOpen {
			return nil // Already closed
		}

		work.State = model.WorkStateEvaluating

		if err := s.store.UpdateWork(ctx, work); err != nil {
			if errors.Is(err, store.ErrVersionConflict) {
				slog.DebugContext(ctx, "version conflict on close bids, retrying",
					"work_id", workID, "attempt", attempt+1)
				continue
			}
			return err
		}

		// Publish bid window closed event
		_ = s.events.Publish(ctx, events.EventWorkBidWindowClosed, map[string]any{
			"work_id":   work.ID,
			"bid_count": work.BidsReceived,
			"closed_at": time.Now().UTC().Format(time.RFC3339Nano),
		})

		slog.InfoContext(ctx, "bid_window_closed",
			"work_id", workID,
			"bids_received", work.BidsReceived,
		)
		return nil
	}
	return ErrVersionConflict
}

func (s *Service) validateWorkSpec(req model.WorkSubmission) error {
	if strings.TrimSpace(req.Category) == "" {
		return errors.New("category is required")
	}
	if strings.TrimSpace(req.Description) == "" {
		return errors.New("description is required")
	}
	if req.Budget.MaxPrice <= 0 {
		return errors.New("budget.max_price must be positive")
	}
	return nil
}

func generateWorkID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "work_" + hex.EncodeToString(b[:8])
}
