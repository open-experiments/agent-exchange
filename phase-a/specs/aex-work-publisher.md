# aex-work-publisher Service Specification

## Overview

**Purpose:** Accept work specifications from consumer agents, validate them, and broadcast to subscribed providers. This is the entry point for consumers seeking agent services.

**Language:** Go 1.22+
**Framework:** Chi router (net/http)
**Runtime:** Cloud Run
**Port:** 8080

## Architecture Position

```
        Consumer Agents
              │
              ▼
    ┌────────────────────┐
    │ aex-work-publisher │◄── THIS SERVICE
    │                    │
    │ • Accept work specs│
    │ • Validate & store │
    │ • Broadcast to     │
    │   providers        │
    └─────────┬──────────┘
              │
    ┌─────────┼─────────┐
    ▼         ▼         ▼
Firestore  Pub/Sub   Provider
(work)   (broadcast)  Registry
```

## Core Responsibilities

1. **Work Submission** - Accept and validate work specifications
2. **Semantic Validation** - Ensure work specs are well-formed
3. **Provider Broadcast** - Notify subscribed providers of work opportunities
4. **Bid Window Management** - Track bid collection timing
5. **Work Lifecycle** - Manage work state transitions

## API Endpoints

### Work Submission

#### POST /v1/work

Submit a new work specification.

```json
// Request
{
  "category": "travel.booking",
  "description": "Book a round-trip flight from LAX to JFK for 2 adults, departing March 15, returning March 22",
  "constraints": {
    "max_latency_ms": 5000,
    "required_fields": ["confirmation_number", "total_price", "itinerary"]
  },
  "budget": {
    "max_price": 0.25,
    "bid_strategy": "balanced",
    "max_cpa_bonus": 0.10
  },
  "success_criteria": [
    {
      "metric": "booking_confirmed",
      "type": "boolean",
      "threshold": true,
      "bonus": 0.05
    },
    {
      "metric": "response_time_ms",
      "type": "numeric",
      "comparison": "lte",
      "threshold": 3000,
      "bonus": 0.02
    }
  ],
  "bid_window_ms": 30000,
  "payload": {
    "origin": "LAX",
    "destination": "JFK",
    "departure_date": "2025-03-15",
    "return_date": "2025-03-22",
    "passengers": 2
  }
}

// Response
{
  "work_id": "work_550e8400",
  "status": "OPEN",
  "bid_window_ends_at": "2025-01-15T10:30:30Z",
  "providers_notified": 12,
  "created_at": "2025-01-15T10:30:00Z"
}
```

#### GET /v1/work/{work_id}

Get work specification and current status.

```json
{
  "work_id": "work_550e8400",
  "category": "travel.booking",
  "description": "Book a round-trip flight...",
  "status": "EVALUATING",
  "bids_received": 5,
  "bid_window_ends_at": "2025-01-15T10:30:30Z",
  "contract": null,
  "created_at": "2025-01-15T10:30:00Z"
}
```

#### POST /v1/work/{work_id}/cancel

Cancel work request (only if not yet awarded).

```json
// Response
{
  "work_id": "work_550e8400",
  "status": "CANCELLED",
  "cancelled_at": "2025-01-15T10:31:00Z"
}
```

### Streaming (for consumers to watch bids)

#### WebSocket /v1/work/{work_id}/stream

Stream bid updates in real-time.

```json
// Messages sent to client
{"type": "bid_received", "bid_id": "bid_123", "provider": "Expedia", "price": 0.12}
{"type": "bid_received", "bid_id": "bid_124", "provider": "Booking.com", "price": 0.08}
{"type": "bid_window_closing", "seconds_remaining": 10}
{"type": "bid_window_closed", "total_bids": 5}
{"type": "evaluation_complete", "winner": "bid_124"}
```

## Data Models

### WorkSpec

```go
type WorkState string

const (
	WorkStateOpen       WorkState = "OPEN"
	WorkStateEvaluating WorkState = "EVALUATING"
	WorkStateAwarded    WorkState = "AWARDED"
	WorkStateExecuting  WorkState = "EXECUTING"
	WorkStateCompleted  WorkState = "COMPLETED"
	WorkStateFailed     WorkState = "FAILED"
	WorkStateCancelled  WorkState = "CANCELLED"
)

type Budget struct {
	MaxPrice    float64  `json:"max_price"`
	BidStrategy string   `json:"bid_strategy"`           // "lowest_price" | "best_quality" | "balanced"
	MaxCPABonus *float64 `json:"max_cpa_bonus,omitempty"`
}

type WorkConstraints struct {
	MaxLatencyMs   *int64   `json:"max_latency_ms,omitempty"`
	RequiredFields []string `json:"required_fields,omitempty"`
	MinTrustTier   *string  `json:"min_trust_tier,omitempty"`
	InternalOnly   bool     `json:"internal_only"`
	Regions        []string `json:"regions,omitempty"`
}

type SuccessCriterion struct {
	Metric     string   `json:"metric"`
	Type       string   `json:"type"` // "boolean" | "numeric"
	Comparison *string  `json:"comparison,omitempty"`
	Threshold  any      `json:"threshold"`
	Bonus      *float64 `json:"bonus,omitempty"`
}

type WorkSpec struct {
	ID             string `json:"work_id"`
	ConsumerID     string `json:"consumer_id"` // From JWT
	Category       string `json:"category"`
	Description    string `json:"description"`
	Constraints    WorkConstraints `json:"constraints"`
	Budget         Budget          `json:"budget"`
	SuccessCriteria []SuccessCriterion `json:"success_criteria"`
	BidWindowMs    int64           `json:"bid_window_ms"`
	Payload        map[string]any  `json:"payload"`

	State           WorkState `json:"status"`
	ProvidersNotified int     `json:"providers_notified"`
	BidsReceived      int     `json:"bids_received"`
	ContractID        *string `json:"contract_id,omitempty"`

	CreatedAt        time.Time  `json:"created_at"`
	BidWindowEndsAt  time.Time  `json:"bid_window_ends_at"`
	AwardedAt        *time.Time `json:"awarded_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}
```

## Core Functions

### Work Submission

```go
func (s *Service) PublishWork(ctx context.Context, consumerID string, req WorkSubmission) (WorkResponse, error) {
	// 1. Validate work spec
	if err := validateWorkSpec(req); err != nil {
		return WorkResponse{}, err
	}

	// 2. Check consumer budget/credits (Settlement/Identity integration)
	balance, err := s.billing.GetBalance(ctx, consumerID)
	if err != nil {
		return WorkResponse{}, err
	}
	if balance < req.Budget.MaxPrice {
		return WorkResponse{}, ErrInsufficientBalance
	}

	// 3. Create work record
	now := time.Now().UTC()
	workID := generateWorkID()
	work := WorkSpec{
		ID:             workID,
		ConsumerID:     consumerID,
		Category:       req.Category,
		Description:    req.Description,
		Constraints:    req.Constraints,
		Budget:         req.Budget,
		SuccessCriteria: req.SuccessCriteria,
		BidWindowMs:    req.BidWindowMs,
		Payload:        req.Payload,
		State:          WorkStateOpen,
		CreatedAt:      now,
		BidWindowEndsAt: now.Add(time.Duration(req.BidWindowMs) * time.Millisecond),
	}

	// 4. Persist to Firestore
	if err := s.store.SaveWork(ctx, work); err != nil {
		return WorkResponse{}, err
	}

	// 5. Get subscribed providers
	providers, err := s.providerRegistry.GetSubscribedProviders(ctx, req.Category)
	if err != nil {
		return WorkResponse{}, err
	}

	// 6. Broadcast work opportunity
	if err := s.broadcastWork(ctx, work, providers); err != nil {
		return WorkResponse{}, err
	}

	// 7. Schedule bid window close (Cloud Tasks)
	if err := s.scheduler.ScheduleBidWindowClose(ctx, work.ID, work.BidWindowEndsAt); err != nil {
		return WorkResponse{}, err
	}

	// 8. Publish event
	_ = s.events.Publish(ctx, "work.submitted", map[string]any{
		"work_id":            work.ID,
		"domain":             work.Category,
		"providers_notified": len(providers),
		"bid_window_ends_at": work.BidWindowEndsAt.Format(time.RFC3339Nano),
	})

	return WorkResponse{
		WorkID:          work.ID,
		Status:          string(work.State),
		BidWindowEndsAt: work.BidWindowEndsAt,
		ProvidersNotified: len(providers),
		CreatedAt:       work.CreatedAt,
	}, nil
}
```

### Provider Broadcast

```go
func (s *Service) broadcastWork(ctx context.Context, work WorkSpec, providers []Provider) error {
	opportunity := WorkOpportunity{
		WorkID:       work.ID,
		Category:     work.Category,
		Description:  work.Description,
		Constraints:  work.Constraints,
		Budget: BudgetInfo{
			MaxPrice: work.Budget.MaxPrice,
			Strategy: work.Budget.BidStrategy,
		},
		BidDeadline:    work.BidWindowEndsAt,
		PayloadPreview: truncatePayload(work.Payload, 500),
	}

	for _, p := range providers {
		if p.BidWebhook != "" {
			if err := s.webhooks.Send(ctx, p.BidWebhook, opportunity, p.WebhookSecret); err != nil {
				// Decide retry policy: fail-fast vs best-effort
				return err
			}
		}
		_ = s.events.Publish(ctx, "work.opportunity."+p.ID, opportunity)
	}
	return nil
}
```

### Bid Window Management

```go
func (s *Service) CloseBidWindow(ctx context.Context, workID string) error {
	work, err := s.store.GetWork(ctx, workID)
	if err != nil {
		return err
	}
	if work.State != WorkStateOpen {
		return nil // Already closed
	}

	work.State = WorkStateEvaluating
	if err := s.store.UpdateWork(ctx, work); err != nil {
		return err
	}

	_ = s.events.Publish(ctx, "work.bid_window_closed", map[string]any{
		"work_id":        work.ID,
		"bids_received":  work.BidsReceived,
		"evaluating_at":  time.Now().UTC().Format(time.RFC3339Nano),
	})
	return nil
}
```

## Events

### Published Events

```json
{
  "event_type": "work.submitted",
  "work_id": "work_550e8400",
  "domain": "travel.booking",
  "consumer_id": "tenant_123",
  "providers_notified": 12,
  "bid_window_ends_at": "2025-01-15T10:30:30Z"
}
{
  "event_type": "work.bid_window_closed",
  "work_id": "work_550e8400",
  "bids_received": 5
}
{
  "event_type": "work.cancelled",
  "work_id": "work_550e8400",
  "reason": "consumer_requested"
}
```

### Consumed Events

```json
{
  "event_type": "bid.submitted",
  "work_id": "work_550e8400",
  "bid_id": "bid_123"
}
{
  "event_type": "contract.awarded",
  "work_id": "work_550e8400",
  "contract_id": "contract_789"
}
```

## Configuration

```bash
# Server
PORT=8080
ENV=production

# Firestore
FIRESTORE_PROJECT_ID=aex-prod
FIRESTORE_COLLECTION_WORK=work_specs

# Provider Registry
PROVIDER_REGISTRY_URL=https://aex-provider-registry-xxx.run.app

# Pub/Sub
PUBSUB_PROJECT_ID=aex-prod
PUBSUB_TOPIC_WORK_EVENTS=aex-work-events
PUBSUB_TOPIC_OPPORTUNITIES=aex-work-opportunities

# Cloud Tasks (for scheduling)
CLOUD_TASKS_QUEUE=aex-bid-window
CLOUD_TASKS_LOCATION=us-central1

# Defaults
DEFAULT_BID_WINDOW_MS=30000
MAX_BID_WINDOW_MS=300000
MIN_BID_WINDOW_MS=5000

# Observability
LOG_LEVEL=info
```

## Directory Structure

```
aex-work-publisher/
├── cmd/
│   └── work-publisher/
│       └── main.go
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── api/
│   │   ├── http.go           # routes + handlers
│   │   └── websocket.go      # WS endpoint (or SSE/polling)
│   ├── model/
│   │   ├── work.go
│   │   └── opportunity.go
│   ├── service/
│   │   ├── publisher.go
│   │   ├── broadcaster.go
│   │   └── bidwindow.go
│   ├── clients/
│   │   └── providerregistry.go
│   └── store/
│       └── firestore.go
├── hack/
│   └── tests/
├── Dockerfile
├── go.mod
└── go.sum
```
