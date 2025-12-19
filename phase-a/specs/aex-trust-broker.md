# aex-trust-broker Service Specification

## Overview

**Purpose:** Manage provider reputation through outcome tracking, calculate trust scores, and enforce trust-based access controls. This is the reputation and compliance layer for the marketplace.

**Language:** Python 3.11+
**Framework:** FastAPI + Event-driven (Pub/Sub)
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

#### POST /internal/trust/batch

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

#### POST /internal/outcomes

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

#### POST /internal/providers/{provider_id}/verify

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

```python
class TrustRecord(BaseModel):
    provider_id: str

    # Current scores
    trust_score: float           # 0.0 - 1.0
    trust_tier: TrustTier
    base_score: float

    # Verification status
    identity_verified: bool
    endpoint_verified: bool
    compliance_verified: bool

    # Statistics
    total_contracts: int
    successful_contracts: int
    failed_contracts: int
    disputed_contracts: int
    disputes_won: int
    disputes_lost: int

    # Timestamps
    registered_at: datetime
    last_contract_at: datetime | None
    last_updated: datetime

class TrustTier(str, Enum):
    UNVERIFIED = "UNVERIFIED"    # New, limited access
    VERIFIED = "VERIFIED"        # Basic verification
    TRUSTED = "TRUSTED"          # Established track record
    PREFERRED = "PREFERRED"      # Top tier external
    INTERNAL = "INTERNAL"        # Enterprise-managed

class ContractOutcome(BaseModel):
    id: str
    contract_id: str
    provider_id: str
    consumer_id: str

    outcome: OutcomeType
    metrics: dict

    agreed_price: float
    final_price: float

    completed_at: datetime
    recorded_at: datetime

class OutcomeType(str, Enum):
    SUCCESS = "SUCCESS"
    SUCCESS_PARTIAL = "SUCCESS_PARTIAL"
    FAILURE_PROVIDER = "FAILURE_PROVIDER"
    FAILURE_EXTERNAL = "FAILURE_EXTERNAL"
    FAILURE_CONSUMER = "FAILURE_CONSUMER"
    DISPUTE_WON = "DISPUTE_WON"
    DISPUTE_LOST = "DISPUTE_LOST"
    EXPIRED = "EXPIRED"

class Dispute(BaseModel):
    id: str
    contract_id: str
    reporter_type: str           # "consumer" or "provider"
    reporter_id: str
    defendant_id: str

    reason: str
    description: str
    evidence: dict

    status: DisputeStatus
    resolution: str | None
    resolved_by: str | None

    created_at: datetime
    resolved_at: datetime | None

class DisputeStatus(str, Enum):
    OPEN = "OPEN"
    INVESTIGATING = "INVESTIGATING"
    RESOLVED_FOR_REPORTER = "RESOLVED_FOR_REPORTER"
    RESOLVED_FOR_DEFENDANT = "RESOLVED_FOR_DEFENDANT"
    RESOLVED_SPLIT = "RESOLVED_SPLIT"
    CLOSED = "CLOSED"
```

## Core Functions

### Calculate Trust Score

```python
async def calculate_trust_score(provider_id: str) -> TrustRecord:
    # 1. Fetch outcome history
    outcomes = await firestore.get_outcomes(provider_id, limit=200)

    # 2. Calculate weighted base score
    base_score = calculate_weighted_score(outcomes)

    # 3. Get verification status
    record = await firestore.get_trust_record(provider_id)

    # 4. Calculate modifiers
    modifiers = 0.0
    if record.identity_verified:
        modifiers += 0.05
    if record.endpoint_verified:
        modifiers += 0.05

    # Tenure bonus (months since registration, max 5 months = 0.1)
    tenure_months = min(5, months_since(record.registered_at))
    modifiers += tenure_months * 0.02

    # Dispute penalty
    open_disputes = await firestore.count_open_disputes(provider_id)
    modifiers -= open_disputes * 0.1

    # Compliance penalty
    violations = await firestore.get_compliance_violations(provider_id)
    modifiers -= len(violations) * 0.2

    # 5. Calculate final score
    final_score = max(0.0, min(1.0, base_score + modifiers))

    # 6. Determine tier
    tier = determine_tier(final_score, record)

    # 7. Update record
    record.trust_score = final_score
    record.base_score = base_score
    record.trust_tier = tier
    record.last_updated = datetime.utcnow()

    await firestore.update_trust_record(record)

    return record

def calculate_weighted_score(outcomes: list[ContractOutcome]) -> float:
    if not outcomes:
        return 0.3  # Default for new providers

    weighted_sum = 0.0
    weight_sum = 0.0

    for i, outcome in enumerate(outcomes):
        # Determine weight based on recency
        if i < 10:
            weight = 1.0
        elif i < 50:
            weight = 0.5
        elif i < 100:
            weight = 0.25
        else:
            weight = 0.1

        # Get outcome score
        score = outcome_to_score(outcome.outcome)

        weighted_sum += score * weight
        weight_sum += weight

    return weighted_sum / weight_sum if weight_sum > 0 else 0.3

def outcome_to_score(outcome: OutcomeType) -> float:
    scores = {
        OutcomeType.SUCCESS: 1.0,
        OutcomeType.SUCCESS_PARTIAL: 0.7,
        OutcomeType.FAILURE_PROVIDER: 0.0,
        OutcomeType.FAILURE_EXTERNAL: 0.5,
        OutcomeType.FAILURE_CONSUMER: 0.8,
        OutcomeType.DISPUTE_WON: 0.8,
        OutcomeType.DISPUTE_LOST: 0.0,
        OutcomeType.EXPIRED: 0.2
    }
    return scores.get(outcome, 0.5)

def determine_tier(score: float, record: TrustRecord) -> TrustTier:
    # Internal providers have fixed tier
    if record.trust_tier == TrustTier.INTERNAL:
        return TrustTier.INTERNAL

    # Check tier requirements
    if score >= 0.9 and record.total_contracts >= 100:
        return TrustTier.PREFERRED
    elif score >= 0.7 and record.total_contracts >= 25:
        return TrustTier.TRUSTED
    elif score >= 0.5 and record.total_contracts >= 5:
        return TrustTier.VERIFIED
    else:
        return TrustTier.UNVERIFIED
```

### Record Outcome

```python
async def record_outcome(outcome: ContractOutcome) -> dict:
    # 1. Store outcome
    await firestore.save_outcome(outcome)

    # 2. Update provider stats
    record = await firestore.get_trust_record(outcome.provider_id)
    previous_score = record.trust_score

    record.total_contracts += 1
    if outcome.outcome in [OutcomeType.SUCCESS, OutcomeType.SUCCESS_PARTIAL]:
        record.successful_contracts += 1
    elif outcome.outcome in [OutcomeType.FAILURE_PROVIDER, OutcomeType.DISPUTE_LOST]:
        record.failed_contracts += 1

    record.last_contract_at = outcome.completed_at

    # 3. Recalculate trust score
    updated_record = await calculate_trust_score(outcome.provider_id)

    # 4. Check for tier change
    tier_changed = updated_record.trust_tier != record.trust_tier

    # 5. Publish event if tier changed
    if tier_changed:
        await pubsub.publish("trust.tier_changed", {
            "provider_id": outcome.provider_id,
            "old_tier": record.trust_tier.value,
            "new_tier": updated_record.trust_tier.value,
            "trust_score": updated_record.trust_score
        })

    # 6. Store to BigQuery for analytics
    await bigquery.insert_outcome(outcome)

    return {
        "recorded": True,
        "provider_id": outcome.provider_id,
        "previous_score": previous_score,
        "new_score": updated_record.trust_score,
        "tier_changed": tier_changed
    }
```

### Get Initial Trust Score

```python
async def get_initial_score(provider_type: str = "external") -> float:
    """Get initial trust score for new providers."""
    if provider_type == "internal":
        return 1.0
    return 0.3  # New external providers start at 0.3
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
├── app/
│   ├── __init__.py
│   ├── main.py
│   ├── config.py
│   ├── models/
│   │   ├── trust.py
│   │   ├── outcome.py
│   │   └── dispute.py
│   ├── services/
│   │   ├── trust_calculator.py
│   │   ├── outcome_recorder.py
│   │   ├── dispute_handler.py
│   │   └── tier_manager.py
│   ├── store/
│   │   ├── firestore.py
│   │   └── bigquery.py
│   ├── events/
│   │   └── handlers.py
│   └── api/
│       ├── trust.py
│       ├── outcomes.py
│       └── disputes.py
├── tests/
│   ├── test_trust_calculator.py
│   ├── test_outcome_recorder.py
│   └── test_tier_manager.py
├── Dockerfile
└── requirements.txt
```
