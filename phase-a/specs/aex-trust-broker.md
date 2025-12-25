# aex-trust-broker Service Specification

## Overview

**Purpose:** Manage provider reputation through outcome tracking, calculate trust scores, and enforce trust-based access controls. This is the reputation and compliance layer for the marketplace.

**Language:** Go 1.22+
**Framework:** Chi router + Event-driven (Pub/Sub)
**Runtime:** Cloud Run
**Port:** 8080

## Architecture Position

```
     Contract Engine    Bid Evaluator    Provider Registry
           │                 │                  │
           ▼                 ▼                  ▼
    ┌─────────────────────────────────────────────────┐
    │             aex-trust-broker                    │◄── THIS SERVICE
    │                                                 │
    │ • Track contract outcomes                       │
    │ • Calculate rolling trust scores                │
    │ • Manage trust tiers                            │
    │ • Enforce compliance requirements               │
    └─────────────────────────────────────────────────┘
                          │
               ┌──────────┼──────────┐
               ▼          ▼          ▼
          Firestore   BigQuery    Settlement
         (scores)    (history)   (adjustments)
```

## Phase A vs Phase B: Trust Service Responsibilities

In Phase B, the `aex-trust-scoring` service is introduced for ML-based predictions.
The two services have distinct responsibilities:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    TRUST SERVICE SEPARATION                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  aex-trust-broker (Phase A & B)          aex-trust-scoring (Phase B only)  │
│  ─────────────────────────────           ────────────────────────────────  │
│                                                                             │
│  TRUST MANAGEMENT:                       ML PREDICTIONS:                    │
│  • Trust tier assignment                 • Success probability prediction   │
│    (UNVERIFIED → PREFERRED)              • Metric forecasting               │
│  • Verification status tracking          • Feature extraction from BigQuery │
│  • Dispute handling & resolution         • Model training & evaluation      │
│  • Compliance enforcement                                                   │
│  • Historical score calculation          INTEGRATION:                       │
│    (weighted average of outcomes)        • Publishes predictions            │
│                                          • Trust-broker may use for         │
│  SOURCE OF TRUTH:                          tier boundary decisions          │
│  • Provider reputation record                                               │
│  • Contract outcome history                                                 │
│  • Dispute records                                                          │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘

Phase B Integration Flow:

    contract.completed ──► aex-trust-broker ──► Updates trust score & tier
                      └──► aex-trust-scoring ──► Updates ML model features
                                    │
                                    ▼
                           trust.prediction_updated
                                    │
                                    ▼
                           aex-trust-broker (optional: use for tier edge cases)
```

**Key Distinction:**
- `aex-trust-broker`: Deterministic scoring based on historical outcomes
- `aex-trust-scoring`: Probabilistic predictions for future performance

## Core Responsibilities

1. **Trust Score Calculation** - Rolling reputation based on outcomes
2. **Trust Tier Management** - Assign and update trust levels
3. **Outcome Recording** - Track contract success/failure
4. **Compliance Enforcement** - Verify provider meets requirements
5. **Score Querying** - Provide scores for bid evaluation

## Trust Tiers

```
┌─────────────────────────────────────────────────────────────────────┐
│                       TRUST TIER LADDER                              │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  INTERNAL (1.0)      Enterprise-managed agents (same org)           │
│      ▲               • No external verification needed              │
│      │               • Highest trust, internal SLA only             │
│      │                                                              │
│  PREFERRED (0.9+)    Top-tier external providers                    │
│      ▲               • 100+ successful contracts                    │
│      │               • 95%+ success rate                            │
│      │               • Manual review passed                         │
│      │                                                              │
│  TRUSTED (0.7+)      Established providers                          │
│      ▲               • 25+ successful contracts                     │
│      │               • 85%+ success rate                            │
│      │               • Endpoint verified                            │
│      │                                                              │
│  VERIFIED (0.5+)     Basic verification complete                    │
│      ▲               • 5+ successful contracts                      │
│      │               • 70%+ success rate                            │
│      │               • Identity verified                            │
│      │                                                              │
│  UNVERIFIED (0.3)    New providers                                  │
│                      • Just registered                              │
│                      • Limited work access                          │
│                      • Higher escrow requirements                   │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

## Trust Score Algorithm

```
┌─────────────────────────────────────────────────────────────────────┐
│                    TRUST SCORE CALCULATION                          │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  BASE SCORE = weighted_average(recent_outcomes)                     │
│                                                                     │
│  Weights by recency:                                                │
│  - Last 10 contracts:  weight = 1.0                                 │
│  - 11-50 contracts:    weight = 0.5                                 │
│  - 51-100 contracts:   weight = 0.25                                │
│  - 100+ contracts:     weight = 0.1                                 │
│                                                                     │
│  Outcome scores:                                                    │
│  - SUCCESS:            1.0                                          │
│  - SUCCESS_PARTIAL:    0.7                                          │
│  - FAILURE_PROVIDER:   0.0 (provider fault)                         │
│  - FAILURE_EXTERNAL:   0.5 (not provider's fault)                   │
│  - DISPUTE_LOST:       0.0                                          │
│  - DISPUTE_WON:        0.8                                          │
│                                                                     │
│  MODIFIERS:                                                         │
│  + 0.05 for verified identity                                       │
│  + 0.05 for verified endpoint (TLS, A2A compliant)                  │
│  + 0.02 per month of good standing (max +0.1)                       │
│  - 0.1 per unresolved dispute                                       │
│  - 0.2 for any compliance violation                                 │
│                                                                     │
│  FINAL = min(1.0, max(0.0, BASE + MODIFIERS))                       │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

## API Endpoints

### Get Trust Score

#### GET /v1/providers/{provider_id}/trust

Get current trust score and tier for a provider.

```json
// Response
{
  "provider_id": "prov_abc123",
  "trust_score": 0.87,
  "trust_tier": "TRUSTED",
  "components": {
    "base_score": 0.82,
    "identity_verified": true,
    "endpoint_verified": true,
    "tenure_bonus": 0.06,
    "dispute_penalty": 0.0,
    "compliance_penalty": 0.0
  },
  "stats": {
    "total_contracts": 145,
    "successful_contracts": 132,
    "failed_contracts": 8,
    "disputed_contracts": 5,
    "success_rate": 0.91
  },
  "last_updated": "2025-01-15T10:00:00Z"
}
```

### Get Trust Score (Internal - Batch)

#### POST /internal/v1/trust/batch

Get trust scores for multiple providers (used by Bid Evaluator).

```json
// Request
{
  "provider_ids": ["prov_abc123", "prov_def456", "prov_ghi789"]
}

// Response
{
  "scores": {
    "prov_abc123": 0.87,
    "prov_def456": 0.72,
    "prov_ghi789": 0.45
  }
}
```

### Record Outcome

#### POST /internal/v1/outcomes

Record a contract outcome (called by Contract Engine).

```json
// Request
{
  "contract_id": "contract_789xyz",
  "provider_id": "prov_abc123",
  "consumer_id": "tenant_123",
  "outcome": "SUCCESS",
  "metrics": {
    "latency_ms": 2300,
    "sla_met": true,
    "consumer_satisfied": true
  },
  "agreed_price": 0.08,
  "final_price": 0.10,  // Including bonuses
  "completed_at": "2025-01-15T10:32:00Z"
}

// Response
{
  "recorded": true,
  "provider_id": "prov_abc123",
  "previous_score": 0.85,
  "new_score": 0.87,
  "tier_changed": false
}
```

### Get Provider History

#### GET /v1/providers/{provider_id}/history

Get contract history for a provider.

```json
// Response
{
  "provider_id": "prov_abc123",
  "outcomes": [
    {
      "contract_id": "contract_789xyz",
      "outcome": "SUCCESS",
      "consumer_id": "tenant_***",  // Masked
      "category": "travel.booking",
      "completed_at": "2025-01-15T10:32:00Z"
    },
    {
      "contract_id": "contract_456abc",
      "outcome": "FAILURE_EXTERNAL",
      "consumer_id": "tenant_***",
      "category": "travel.search",
      "completed_at": "2025-01-14T15:20:00Z"
    }
  ],
  "pagination": {
    "total": 145,
    "offset": 0,
    "limit": 20
  }
}
```

### Report Dispute

#### POST /v1/disputes

Report a dispute against a provider.

```json
// Request
{
  "contract_id": "contract_789xyz",
  "reporter_type": "consumer",  // or "provider"
  "reporter_id": "tenant_123",
  "reason": "service_not_delivered",
  "description": "Provider claimed success but no booking confirmation received",
  "evidence": {
    "expected_fields": ["confirmation_number"],
    "received_fields": []
  }
}

// Response
{
  "dispute_id": "dispute_abc123",
  "contract_id": "contract_789xyz",
  "status": "OPEN",
  "created_at": "2025-01-15T11:00:00Z"
}
```

### Verify Provider

#### POST /internal/v1/providers/{provider_id}/verify

Verify provider identity (admin action).

```json
// Request
{
  "verification_type": "identity",  // or "endpoint", "compliance"
  "verified": true,
  "notes": "Domain ownership verified via DNS TXT record"
}

// Response
{
  "provider_id": "prov_abc123",
  "verification_type": "identity",
  "verified": true,
  "previous_tier": "UNVERIFIED",
  "new_tier": "VERIFIED"
}
```

## Data Models

### TrustRecord

```go
type TrustTier string

const (
	TrustTierUnverified TrustTier = "UNVERIFIED"
	TrustTierVerified   TrustTier = "VERIFIED"
	TrustTierTrusted    TrustTier = "TRUSTED"
	TrustTierPreferred  TrustTier = "PREFERRED"
	TrustTierInternal   TrustTier = "INTERNAL"
)

type OutcomeType string

const (
	OutcomeSuccess        OutcomeType = "SUCCESS"
	OutcomeSuccessPartial OutcomeType = "SUCCESS_PARTIAL"
	OutcomeFailureProvider OutcomeType = "FAILURE_PROVIDER"
	OutcomeFailureExternal OutcomeType = "FAILURE_EXTERNAL"
	OutcomeFailureConsumer OutcomeType = "FAILURE_CONSUMER"
	OutcomeDisputeWon     OutcomeType = "DISPUTE_WON"
	OutcomeDisputeLost    OutcomeType = "DISPUTE_LOST"
	OutcomeExpired        OutcomeType = "EXPIRED"
)

type DisputeStatus string

const (
	DisputeOpen               DisputeStatus = "OPEN"
	DisputeInvestigating      DisputeStatus = "INVESTIGATING"
	DisputeResolvedForReporter DisputeStatus = "RESOLVED_FOR_REPORTER"
	DisputeResolvedForDefendant DisputeStatus = "RESOLVED_FOR_DEFENDANT"
	DisputeResolvedSplit      DisputeStatus = "RESOLVED_SPLIT"
	DisputeClosed             DisputeStatus = "CLOSED"
)

type TrustRecord struct {
	ProviderID string `json:"provider_id"`

	TrustScore float64  `json:"trust_score"`
	TrustTier  TrustTier `json:"trust_tier"`
	BaseScore  float64  `json:"base_score"`

	IdentityVerified   bool `json:"identity_verified"`
	EndpointVerified   bool `json:"endpoint_verified"`
	ComplianceVerified bool `json:"compliance_verified"`

	TotalContracts      int `json:"total_contracts"`
	SuccessfulContracts int `json:"successful_contracts"`
	FailedContracts     int `json:"failed_contracts"`
	DisputedContracts   int `json:"disputed_contracts"`
	DisputesWon         int `json:"disputes_won"`
	DisputesLost        int `json:"disputes_lost"`

	RegisteredAt   time.Time  `json:"registered_at"`
	LastContractAt *time.Time `json:"last_contract_at,omitempty"`
	LastUpdated    time.Time  `json:"last_updated"`
}

type ContractOutcome struct {
	ID         string `json:"id"`
	ContractID string `json:"contract_id"`
	ProviderID string `json:"provider_id"`
	ConsumerID string `json:"consumer_id"`

	Outcome  OutcomeType     `json:"outcome"`
	Metrics  map[string]any  `json:"metrics"`

	AgreedPrice float64 `json:"agreed_price"`
	FinalPrice  float64 `json:"final_price"`

	CompletedAt time.Time `json:"completed_at"`
	RecordedAt  time.Time `json:"recorded_at"`
}

type Dispute struct {
	ID           string `json:"dispute_id"`
	ContractID   string `json:"contract_id"`
	ReporterType string `json:"reporter_type"` // "consumer" or "provider"
	ReporterID   string `json:"reporter_id"`
	DefendantID  string `json:"defendant_id"`

	Reason      string         `json:"reason"`
	Description string         `json:"description"`
	Evidence    map[string]any `json:"evidence"`

	Status     DisputeStatus `json:"status"`
	Resolution *string       `json:"resolution,omitempty"`
	ResolvedBy *string       `json:"resolved_by,omitempty"`

	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}
```

## Core Functions

### Calculate Trust Score

```go
func (s *Service) CalculateTrustScore(ctx context.Context, providerID string) (TrustRecord, error) {
	// 1. Fetch outcome history
	outcomes, err := s.store.GetOutcomes(ctx, providerID, 200)
	if err != nil {
		return TrustRecord{}, err
	}

	// 2. Calculate weighted base score
	baseScore := calculateWeightedScore(outcomes)

	// 3. Get verification status + current record
	record, err := s.store.GetTrustRecord(ctx, providerID)
	if err != nil {
		return TrustRecord{}, err
	}

	// 4. Calculate modifiers
	modifiers := 0.0
	if record.IdentityVerified {
		modifiers += 0.05
	}
	if record.EndpointVerified {
		modifiers += 0.05
	}

	tenureMonths := monthsSince(record.RegisteredAt)
	if tenureMonths > 5 {
		tenureMonths = 5
	}
	modifiers += float64(tenureMonths) * 0.02

	openDisputes, err := s.store.CountOpenDisputes(ctx, providerID)
	if err != nil {
		return TrustRecord{}, err
	}
	modifiers -= float64(openDisputes) * 0.1

	violations, err := s.store.GetComplianceViolations(ctx, providerID)
	if err != nil {
		return TrustRecord{}, err
	}
	modifiers -= float64(len(violations)) * 0.2

	// 5. Calculate final score
	finalScore := clamp01(baseScore + modifiers)

	// 6. Determine tier
	tier := determineTier(finalScore, record)

	// 7. Update record
	record.TrustScore = finalScore
	record.BaseScore = baseScore
	record.TrustTier = tier
	record.LastUpdated = time.Now().UTC()
	if err := s.store.UpdateTrustRecord(ctx, record); err != nil {
		return TrustRecord{}, err
	}

	return record, nil
}

func calculateWeightedScore(outcomes []ContractOutcome) float64 {
	if len(outcomes) == 0 {
		return 0.3
	}
	weightedSum := 0.0
	weightSum := 0.0
	for i, o := range outcomes {
		weight := 0.1
		switch {
		case i < 10:
			weight = 1.0
		case i < 50:
			weight = 0.5
		case i < 100:
			weight = 0.25
		}
		score := outcomeToScore(o.Outcome)
		weightedSum += score * weight
		weightSum += weight
	}
	if weightSum == 0 {
		return 0.3
	}
	return weightedSum / weightSum
}

func outcomeToScore(outcome OutcomeType) float64 {
	switch outcome {
	case OutcomeSuccess:
		return 1.0
	case OutcomeSuccessPartial:
		return 0.7
	case OutcomeFailureProvider:
		return 0.0
	case OutcomeFailureExternal:
		return 0.5
	case OutcomeFailureConsumer:
		return 0.8
	case OutcomeDisputeWon:
		return 0.8
	case OutcomeDisputeLost:
		return 0.0
	case OutcomeExpired:
		return 0.2
	default:
		return 0.5
	}
}

func determineTier(score float64, record TrustRecord) TrustTier {
	if record.TrustTier == TrustTierInternal {
		return TrustTierInternal
	}
	switch {
	case score >= 0.9 && record.TotalContracts >= 100:
		return TrustTierPreferred
	case score >= 0.7 && record.TotalContracts >= 25:
		return TrustTierTrusted
	case score >= 0.5 && record.TotalContracts >= 5:
		return TrustTierVerified
	default:
		return TrustTierUnverified
	}
}
```

### Record Outcome

```go
func (s *Service) RecordOutcome(ctx context.Context, outcome ContractOutcome) (map[string]any, error) {
	// 1. Store outcome
	if err := s.store.SaveOutcome(ctx, outcome); err != nil {
		return nil, err
	}

	// 2. Update provider stats (and capture old tier/score)
	record, err := s.store.GetTrustRecord(ctx, outcome.ProviderID)
	if err != nil {
		return nil, err
	}
	previousScore := record.TrustScore
	previousTier := record.TrustTier

	record.TotalContracts++
	switch outcome.Outcome {
	case OutcomeSuccess, OutcomeSuccessPartial:
		record.SuccessfulContracts++
	case OutcomeFailureProvider, OutcomeDisputeLost:
		record.FailedContracts++
	}
	record.LastContractAt = &outcome.CompletedAt
	if err := s.store.UpdateTrustRecord(ctx, record); err != nil {
		return nil, err
	}

	// 3. Recalculate trust score
	updated, err := s.CalculateTrustScore(ctx, outcome.ProviderID)
	if err != nil {
		return nil, err
	}

	// 4. Check for tier change
	tierChanged := updated.TrustTier != previousTier
	if tierChanged {
		_ = s.events.Publish(ctx, "trust.tier_changed", map[string]any{
			"provider_id": outcome.ProviderID,
			"old_tier":    string(previousTier),
			"new_tier":    string(updated.TrustTier),
			"trust_score": updated.TrustScore,
		})
	}

	// 5. Store to BigQuery for analytics
	_ = s.bigquery.InsertOutcome(ctx, outcome)

	return map[string]any{
		"recorded":       true,
		"provider_id":    outcome.ProviderID,
		"previous_score": previousScore,
		"new_score":      updated.TrustScore,
		"tier_changed":   tierChanged,
	}, nil
}
```

### Get Initial Trust Score

```go
func getInitialScore(providerType string) float64 {
	if providerType == "internal" {
		return 1.0
	}
	return 0.3
}
```

## Events

### Consumed Events

```json
// Contract completed
{
  "event_type": "contract.completed",
  "contract_id": "contract_789xyz",
  "provider_id": "prov_abc123",
  "consumer_id": "tenant_123",
  "outcome": {...}
}

// Contract failed
{
  "event_type": "contract.failed",
  "contract_id": "contract_789xyz",
  "provider_id": "prov_abc123",
  "reason": "provider_timeout"
}

// Provider registered
{
  "event_type": "provider.registered",
  "provider_id": "prov_abc123"
}
```

### Published Events

```json
// Trust tier changed
{
  "event_type": "trust.tier_changed",
  "provider_id": "prov_abc123",
  "old_tier": "VERIFIED",
  "new_tier": "TRUSTED",
  "trust_score": 0.75,
  "timestamp": "2025-01-15T10:00:00Z"
}

// Dispute opened
{
  "event_type": "trust.dispute_opened",
  "dispute_id": "dispute_abc123",
  "contract_id": "contract_789xyz",
  "provider_id": "prov_abc123",
  "reporter_id": "tenant_123"
}

// Dispute resolved
{
  "event_type": "trust.dispute_resolved",
  "dispute_id": "dispute_abc123",
  "resolution": "RESOLVED_FOR_REPORTER",
  "provider_id": "prov_abc123"
}
```

## Configuration

```bash
# Server
PORT=8080
ENV=production

# Firestore
FIRESTORE_PROJECT_ID=aex-prod
FIRESTORE_COLLECTION_TRUST=trust_records
FIRESTORE_COLLECTION_OUTCOMES=contract_outcomes
FIRESTORE_COLLECTION_DISPUTES=disputes

# BigQuery (for analytics)
BIGQUERY_PROJECT_ID=aex-prod
BIGQUERY_DATASET=aex_analytics
BIGQUERY_TABLE_OUTCOMES=outcomes

# Pub/Sub
PUBSUB_PROJECT_ID=aex-prod
PUBSUB_SUBSCRIPTION=aex-trust-broker-sub
PUBSUB_TOPIC_EVENTS=aex-trust-events

# Trust Settings
DEFAULT_INITIAL_SCORE=0.3
INTERNAL_PROVIDER_SCORE=1.0
SCORE_RECALC_BATCH_SIZE=100

# Observability
LOG_LEVEL=info
```

## Directory Structure

```
aex-trust-broker/
├── cmd/
│   └── trust-broker/
│       └── main.go
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── api/
│   │   ├── trust.go
│   │   ├── outcomes.go
│   │   └── disputes.go
│   ├── model/
│   │   ├── trust.go
│   │   ├── outcome.go
│   │   └── dispute.go
│   ├── service/
│   │   ├── calculator.go
│   │   ├── recorder.go
│   │   └── tiers.go
│   ├── store/
│   │   ├── firestore.go
│   │   └── bigquery.go
│   └── events/
│       └── handler.go
├── hack/
│   └── tests/
├── Dockerfile
├── go.mod
└── go.sum
```
