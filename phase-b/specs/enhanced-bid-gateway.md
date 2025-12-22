# Enhanced Bid Gateway Specification (Phase B)

## Overview

Phase B enhances `aex-bid-gateway` to support CPA bids where providers can accept outcome-based pricing terms, provide performance guarantees, and commit to meeting success criteria.

## Changes from Phase A

| Aspect | Phase A | Phase B |
|--------|---------|---------|
| Bid Structure | Price + confidence | + CPA guarantees, outcome commitments |
| Validation | Basic constraints | + CPA term validation |
| Stored Data | Bid details | + CPA commitments, predicted performance |
| Events | bid.submitted | + cpa_enabled flag, guarantees |

## Enhanced API

### POST /v1/bids (Enhanced)

```json
// Request (Phase B additions)
{
  "work_id": "work_550e8400",
  "price": 0.08,
  "confidence": 0.92,
  "approach": "Using premium booking API with real-time inventory",
  "estimated_latency_ms": 1500,
  "mvp_sample": {
    "sample_input": "Book flight LAX→JFK, March 15-22, 2 adults",
    "sample_output": "Found 23 options. Best: Delta DL123, $299/person",
    "sample_latency_ms": 450
  },
  "sla": {
    "max_latency_ms": 3000,
    "availability": 0.99
  },
  "a2a_endpoint": "https://agent.expedia.com/a2a/v1",

  // NEW: CPA-specific fields
  "cpa_acceptance": {
    "accept_cpa_terms": true,
    "criteria_guarantees": [
      {
        "metric": "booking_confirmed",
        "guarantee": true,
        "confidence": 0.95,
        "evidence_types": ["confirmation_number", "receipt"]
      },
      {
        "metric": "response_time_ms",
        "guarantee": 2500,
        "confidence": 0.90
      },
      {
        "metric": "price_accuracy",
        "guarantee": 0.97,
        "confidence": 0.88
      }
    ],
    "accept_penalty": true,
    "max_penalty_accepted": 0.02,
    "expected_bonus": 0.07
  },
  "historical_performance": {
    "category_success_rate": 0.94,
    "similar_tasks_completed": 1250,
    "avg_criteria_met_rate": 0.91
  },

  "expires_at": "2025-01-15T10:35:00Z"
}

// Response
{
  "bid_id": "bid_abc123",
  "work_id": "work_550e8400",
  "status": "RECEIVED",
  "cpa_enabled": true,
  "expected_total": 0.15,           // price + expected_bonus
  "received_at": "2025-01-15T10:30:15Z"
}
```

## Enhanced Data Models

```go
// Enhanced BidPacket
type BidPacket struct {
    BidID            string            `json:"bid_id"`
    WorkID           string            `json:"work_id"`
    ProviderID       string            `json:"provider_id"`
    AgentID          string            `json:"agent_id"`

    // Base pricing
    Price            float64           `json:"price"`
    PriceBreakdown   map[string]float64 `json:"price_breakdown,omitempty"`

    // Quality signals
    Confidence       float64           `json:"confidence"`
    Approach         string            `json:"approach"`
    EstimatedLatency int64             `json:"estimated_latency_ms"`

    // Proof of competence
    MVPSample        *MVPSample        `json:"mvp_sample,omitempty"`

    // SLA commitment
    SLA              SLACommitment     `json:"sla"`

    // Execution endpoint
    A2AEndpoint      string            `json:"a2a_endpoint"`

    // NEW: CPA fields
    CPAAcceptance    *CPAAcceptance    `json:"cpa_acceptance,omitempty"`
    HistoricalPerf   *HistoricalPerf   `json:"historical_performance,omitempty"`

    // Timestamps
    ExpiresAt        time.Time         `json:"expires_at"`
    ReceivedAt       time.Time         `json:"received_at"`
}

type CPAAcceptance struct {
    AcceptCPATerms     bool                `json:"accept_cpa_terms"`
    CriteriaGuarantees []CriteriaGuarantee `json:"criteria_guarantees"`
    AcceptPenalty      bool                `json:"accept_penalty"`
    MaxPenaltyAccepted float64             `json:"max_penalty_accepted"`
    ExpectedBonus      float64             `json:"expected_bonus"`
}

type CriteriaGuarantee struct {
    Metric        string   `json:"metric"`
    Guarantee     any      `json:"guarantee"`        // bool, float64, or threshold
    Confidence    float64  `json:"confidence"`       // Provider's confidence in meeting
    EvidenceTypes []string `json:"evidence_types,omitempty"`
}

type HistoricalPerf struct {
    CategorySuccessRate   float64 `json:"category_success_rate"`
    SimilarTasksCompleted int     `json:"similar_tasks_completed"`
    AvgCriteriaMetRate    float64 `json:"avg_criteria_met_rate"`
}
```

## Enhanced Validation

```go
func (h *BidHandler) validateBid(ctx context.Context, bid *BidPacket) error {
    // Phase A validations
    if bid.WorkID == "" || bid.Price <= 0 || bid.A2AEndpoint == "" {
        return errors.New("missing required fields")
    }

    if bid.Confidence < 0 || bid.Confidence > 1 {
        return errors.New("confidence must be between 0 and 1")
    }

    if bid.ExpiresAt.Before(time.Now()) {
        return errors.New("bid already expired")
    }

    // Phase B: CPA validations
    if bid.CPAAcceptance != nil {
        if err := h.validateCPAAcceptance(ctx, bid); err != nil {
            return err
        }
    }

    return nil
}

func (h *BidHandler) validateCPAAcceptance(ctx context.Context, bid *BidPacket) error {
    cpa := bid.CPAAcceptance

    // Get work spec to validate against criteria
    work, err := h.workStore.Get(ctx, bid.WorkID)
    if err != nil {
        return errors.New("unable to fetch work spec")
    }

    if !work.CPAEnabled {
        if cpa.AcceptCPATerms {
            return errors.New("work does not accept CPA bids")
        }
        return nil
    }

    // Validate guarantees match work criteria
    workCriteria := make(map[string]SuccessCriterion)
    for _, c := range work.SuccessCriteria {
        workCriteria[c.Metric] = c
    }

    for _, g := range cpa.CriteriaGuarantees {
        criterion, exists := workCriteria[g.Metric]
        if !exists {
            return fmt.Errorf("guarantee for unknown metric: %s", g.Metric)
        }

        // Validate guarantee type matches criterion type
        if err := validateGuaranteeType(g.Guarantee, criterion.MetricType); err != nil {
            return fmt.Errorf("invalid guarantee for %s: %v", g.Metric, err)
        }

        // Confidence must be positive
        if g.Confidence <= 0 || g.Confidence > 1 {
            return fmt.Errorf("invalid confidence for %s: must be 0-1", g.Metric)
        }
    }

    // Validate expected bonus is reasonable
    maxPossibleBonus := calculateMaxBonus(work.SuccessCriteria)
    if cpa.ExpectedBonus > maxPossibleBonus {
        return fmt.Errorf("expected bonus %.2f exceeds maximum possible %.2f",
            cpa.ExpectedBonus, maxPossibleBonus)
    }

    // Validate penalty acceptance
    if cpa.AcceptPenalty && cpa.MaxPenaltyAccepted < 0 {
        return errors.New("max_penalty_accepted must be non-negative")
    }

    return nil
}

func validateGuaranteeType(guarantee any, metricType string) error {
    switch metricType {
    case "boolean":
        if _, ok := guarantee.(bool); !ok {
            return errors.New("boolean metric requires bool guarantee")
        }
    case "numeric", "latency", "percentage", "count":
        switch guarantee.(type) {
        case float64, int:
            // OK
        default:
            return errors.New("numeric metric requires numeric guarantee")
        }
    }
    return nil
}
```

## Enhanced Bid Storage

```go
func (h *BidHandler) storeBid(ctx context.Context, bid *BidPacket) error {
    // Store in Redis for fast access during evaluation
    key := fmt.Sprintf("bids:%s:%s", bid.WorkID, bid.BidID)
    data, _ := json.Marshal(bid)

    if err := h.redis.Set(ctx, key, data, 1*time.Hour).Err(); err != nil {
        return err
    }

    // Add to work's bid list
    listKey := fmt.Sprintf("bids:%s", bid.WorkID)
    h.redis.SAdd(ctx, listKey, bid.BidID)

    // NEW: Track CPA bid count separately
    if bid.CPAAcceptance != nil && bid.CPAAcceptance.AcceptCPATerms {
        cpaKey := fmt.Sprintf("bids:cpa:%s", bid.WorkID)
        h.redis.SAdd(ctx, cpaKey, bid.BidID)
    }

    // Store in Firestore for durability
    go func() {
        _ = h.firestore.SaveBid(context.Background(), bid)
    }()

    return nil
}
```

## Enhanced Consumer Notification

```go
func (h *BidHandler) notifyConsumer(ctx context.Context, bid *BidPacket) {
    work, _ := h.workStore.Get(ctx, bid.WorkID)
    if work == nil {
        return
    }

    // Enhanced notification with CPA info
    notification := BidNotification{
        Type:       "bid",
        BidID:      bid.BidID,
        Provider:   h.getProviderName(bid.ProviderID),
        Price:      bid.Price,
        Confidence: bid.Confidence,
        // NEW: CPA fields
        CPAEnabled:    bid.CPAAcceptance != nil && bid.CPAAcceptance.AcceptCPATerms,
        ExpectedBonus: 0,
        ExpectedTotal: bid.Price,
    }

    if notification.CPAEnabled {
        notification.ExpectedBonus = bid.CPAAcceptance.ExpectedBonus
        notification.ExpectedTotal = bid.Price + bid.CPAAcceptance.ExpectedBonus
        notification.GuaranteesCount = len(bid.CPAAcceptance.CriteriaGuarantees)
    }

    h.wsHub.Broadcast(bid.WorkID, notification)
}
```

## Events

### bid.submitted (Enhanced)

```json
{
  "event_type": "bid.submitted",
  "event_id": "evt_abc123",
  "bid_id": "bid_abc123",
  "work_id": "work_550e8400",
  "provider_id": "prov_expedia",
  "agent_id": "agent_xyz789",
  "price": 0.08,
  "confidence": 0.92,
  "cpa_enabled": true,                        // NEW
  "expected_bonus": 0.07,                     // NEW
  "expected_total": 0.15,                     // NEW
  "guarantees_count": 3,                      // NEW
  "accepts_penalty": true,                    // NEW
  "received_at": "2025-01-15T10:30:15Z"
}
```

## Internal API Enhancements

### GET /internal/bids?work_id={id} (Enhanced Response)

```json
{
  "work_id": "work_550e8400",
  "bids": [
    {
      "bid_id": "bid_abc123",
      "provider_id": "prov_expedia",
      "agent_id": "agent_xyz789",
      "price": 0.08,
      "confidence": 0.92,
      "cpa_acceptance": {
        "accept_cpa_terms": true,
        "criteria_guarantees": [
          {
            "metric": "booking_confirmed",
            "guarantee": true,
            "confidence": 0.95
          }
        ],
        "expected_bonus": 0.07
      },
      "historical_performance": {
        "category_success_rate": 0.94,
        "similar_tasks_completed": 1250
      },
      "received_at": "2025-01-15T10:30:15Z"
    }
  ],
  "total_bids": 5,
  "cpa_bids": 3,                              // NEW
  "summary": {                                // NEW
    "avg_price": 0.085,
    "avg_expected_total": 0.14,
    "avg_confidence": 0.89
  }
}
```

## Configuration Changes

```yaml
# config/bid-gateway.yaml (Phase B additions)
bid_gateway:
  # Phase A config unchanged...

  # Phase B additions
  cpa:
    enabled: true
    max_guarantees_per_bid: 10
    min_guarantee_confidence: 0.5
    validate_historical_claims: true

    # Penalty constraints
    max_penalty_rate: 0.50
    default_penalty_rate: 0.20

  validation:
    require_historical_for_cpa: false    # Require historical_performance for CPA bids
    min_tasks_for_history: 10
```

## Backward Compatibility

1. **Optional Fields**: All CPA fields are optional
2. **Non-CPA Works**: If work doesn't enable CPA, CPA fields in bid are ignored
3. **Non-CPA Bids**: Providers can submit traditional bids without CPA acceptance
4. **Event Compatibility**: New fields added, consumers ignore unknown fields

## Storage Changes

### Redis Keys

```
bids:{work_id}           # Set of all bid IDs for a work
bids:{work_id}:{bid_id}  # Full bid JSON
bids:cpa:{work_id}       # NEW: Set of CPA-enabled bid IDs
```

### Firestore Document (bids collection)

```json
{
  "id": "bid_abc123",
  "work_id": "work_550e8400",
  "provider_id": "prov_expedia",
  "agent_id": "agent_xyz789",
  "price": 0.08,
  "confidence": 0.92,
  "approach": "...",
  "mvp_sample": {...},
  "sla": {...},
  "a2a_endpoint": "https://...",
  "cpa_acceptance": {                    // NEW
    "accept_cpa_terms": true,
    "criteria_guarantees": [...],
    "accept_penalty": true,
    "expected_bonus": 0.07
  },
  "historical_performance": {...},       // NEW
  "received_at": "2025-01-15T10:30:15Z",
  "expires_at": "2025-01-15T10:35:00Z"
}
```
