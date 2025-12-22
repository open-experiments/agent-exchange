# Enhanced Trust Broker Specification (Phase B)

## Overview

Phase B enhances `aex-trust-broker` to integrate ML predictions from `aex-trust-scoring`, handle CPA-specific outcomes, and provide enhanced dispute resolution for outcome-based contracts.

## Changes from Phase A

| Aspect | Phase A | Phase B |
|--------|---------|---------|
| Trust Score | Historical outcomes only | + ML prediction integration |
| Outcomes | Binary success/failure | + CPA criteria outcomes |
| Disputes | Basic resolution | + Outcome verification disputes |
| Tier Rules | Contract count + score | + CPA performance factors |
| Caching | None | Prediction cache for evaluator |

## Architecture

```
     Contract Engine    Bid Evaluator    Provider Registry
           │                 │                  │
           ▼                 ▼                  ▼
    ┌─────────────────────────────────────────────────┐
    │         aex-trust-broker (Enhanced)             │◄── THIS SERVICE
    │                                                 │
    │ • Track CPA contract outcomes                   │
    │ • Integrate ML predictions                      │
    │ • Calculate composite trust scores              │
    │ • Handle outcome-based disputes                 │
    └─────────────────────────────────────────────────┘
                          │
               ┌──────────┼──────────────┬────────────┐
               ▼          ▼              ▼            ▼
          Firestore   BigQuery    Trust Scoring   Outcome Oracle
         (scores)    (history)   (ML predictions)  (disputes)
```

## Phase A vs Phase B Trust Service Integration

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    ENHANCED TRUST INTEGRATION (Phase B)                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  contract.completed                                                         │
│        │                                                                    │
│        ├───► aex-trust-broker                                               │
│        │         │                                                          │
│        │         ├─► Update historical trust score                          │
│        │         ├─► Record CPA criteria outcomes                           │
│        │         ├─► Evaluate tier transition                               │
│        │         │                                                          │
│        │         └─► Query aex-trust-scoring for prediction                 │
│        │                      │                                             │
│        │                      ▼                                             │
│        │              ┌─────────────────┐                                   │
│        │              │  Composite Score │                                  │
│        │              │  = 0.6 × hist    │                                  │
│        │              │  + 0.4 × ml_pred │                                  │
│        │              └─────────────────┘                                   │
│        │                                                                    │
│        └───► aex-trust-scoring                                              │
│                  │                                                          │
│                  ├─► Update ML model features                               │
│                  ├─► Retrain prediction models (async)                      │
│                  └─► Publish trust.prediction_updated                       │
│                                                                             │
│  aex-bid-evaluator                                                          │
│        │                                                                    │
│        └───► aex-trust-broker.get_composite_scores()                        │
│                  │                                                          │
│                  ├─► Historical score (from Firestore)                      │
│                  ├─► ML prediction (from cache/trust-scoring)               │
│                  └─► Return blended score for bid ranking                   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Enhanced Data Models

```python
class TrustRecord(BaseModel):
    # Phase A fields
    provider_id: str
    trust_score: float
    trust_tier: TrustTier
    base_score: float
    identity_verified: bool
    endpoint_verified: bool
    compliance_verified: bool
    total_contracts: int
    successful_contracts: int
    failed_contracts: int
    disputed_contracts: int
    disputes_won: int
    disputes_lost: int
    registered_at: datetime
    last_contract_at: datetime | None
    last_updated: datetime

    # NEW: Phase B fields
    cpa_enabled: bool = False
    cpa_stats: CPAStats | None = None
    composite_score: float | None = None    # Blended hist + ML
    ml_prediction: MLPredictionCache | None = None
    outcome_disputes: list[OutcomeDispute] = []


class CPAStats(BaseModel):
    """CPA-specific performance statistics."""
    total_cpa_contracts: int = 0
    cpa_success_rate: float = 0.0

    # Criteria performance
    criteria_met_rate: float = 0.0          # % of criteria met
    criteria_performance: dict[str, float] = {}  # metric -> success rate

    # Financial outcomes
    total_bonuses_earned: float = 0.0
    total_penalties_incurred: float = 0.0
    net_cpa_earnings: float = 0.0
    avg_bonus_rate: float = 0.0             # Actual / potential bonus

    # Prediction accuracy
    confidence_calibration: float = 0.0     # How well confidence predicts outcome
    prediction_accuracy: float = 0.0        # ML predictions vs actual

    # Time windows
    last_30_days: CPAWindow | None = None
    last_90_days: CPAWindow | None = None


class CPAWindow(BaseModel):
    """CPA performance for a time window."""
    contracts: int
    success_rate: float
    criteria_met_rate: float
    bonus_rate: float
    penalty_rate: float


class MLPredictionCache(BaseModel):
    """Cached ML prediction from trust-scoring."""
    predicted_success_rate: float
    confidence: float
    criteria_predictions: dict[str, float]  # metric -> P(success)
    model_version: str
    computed_at: datetime
    expires_at: datetime


class OutcomeDispute(BaseModel):
    """CPA-specific outcome dispute."""
    dispute_id: str
    contract_id: str
    verification_id: str | None
    disputed_criteria: list[str]
    disputed_by: str                        # consumer or provider
    reason: str
    evidence: list[dict]
    status: str                             # open, investigating, resolved
    resolution: OutcomeDisputeResolution | None


class OutcomeDisputeResolution(BaseModel):
    """Resolution of an outcome dispute."""
    resolved_at: datetime
    resolved_by: str
    outcome: str                            # upheld, overturned, partial
    criteria_adjustments: list[CriteriaAdjustment]
    settlement_adjustment: float
    notes: str


class CriteriaAdjustment(BaseModel):
    """Adjustment to a criterion result after dispute."""
    metric: str
    original_met: bool
    adjusted_met: bool
    bonus_adjusted: float
    reason: str


class CompositeScore(BaseModel):
    """Blended trust score for bid evaluation."""
    provider_id: str
    historical_score: float
    ml_prediction: float
    composite_score: float
    weights: dict[str, float]               # hist, ml weights used
    confidence: float
    computed_at: datetime


class ContractOutcome(BaseModel):
    # Phase A fields
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

    # NEW: Phase B CPA fields
    cpa_enabled: bool = False
    cpa_outcome: CPAOutcome | None = None


class CPAOutcome(BaseModel):
    """CPA-specific outcome details."""
    verification_id: str | None
    verification_status: str                # verified, failed, disputed
    criteria_results: list[CriteriaResult]
    base_price: float
    bonus_earned: float
    penalty_applied: float
    final_amount: float
    provider_confidence: float              # Original confidence from bid
```

## Enhanced API Endpoints

### Get Trust Score (Enhanced)

#### GET /v1/providers/{provider_id}/trust

```json
// Response (Enhanced)
{
  "provider_id": "prov_abc123",
  "trust_score": 0.87,
  "trust_tier": "TRUSTED",
  "composite_score": 0.89,                   // NEW: Blended score
  "components": {
    "base_score": 0.82,
    "historical_weight": 0.6,
    "ml_prediction": 0.92,                   // NEW
    "ml_weight": 0.4,                        // NEW
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
  "cpa_stats": {                             // NEW
    "total_cpa_contracts": 85,
    "cpa_success_rate": 0.94,
    "criteria_met_rate": 0.89,
    "total_bonuses_earned": 42.50,
    "total_penalties_incurred": 3.20,
    "net_cpa_earnings": 39.30,
    "confidence_calibration": 0.91
  },
  "ml_prediction": {                         // NEW
    "predicted_success_rate": 0.92,
    "confidence": 0.85,
    "model_version": "v2.1.0",
    "expires_at": "2025-01-15T11:00:00Z"
  },
  "last_updated": "2025-01-15T10:00:00Z"
}
```

### Get Composite Scores (Batch - Internal)

#### POST /internal/v1/trust/composite-batch

Used by Bid Evaluator for scoring bids.

```json
// Request
{
  "provider_ids": ["prov_abc123", "prov_def456"],
  "include_criteria_predictions": true,
  "work_category": "travel.booking",
  "criteria_metrics": ["booking_confirmed", "response_time_ms"]
}

// Response
{
  "scores": {
    "prov_abc123": {
      "historical_score": 0.87,
      "ml_prediction": 0.92,
      "composite_score": 0.89,
      "confidence": 0.85,
      "criteria_predictions": {              // NEW
        "booking_confirmed": 0.95,
        "response_time_ms": 0.88
      }
    },
    "prov_def456": {
      "historical_score": 0.72,
      "ml_prediction": 0.78,
      "composite_score": 0.74,
      "confidence": 0.72,
      "criteria_predictions": {
        "booking_confirmed": 0.80,
        "response_time_ms": 0.75
      }
    }
  },
  "cache_status": "partial_hit",
  "computed_at": "2025-01-15T10:30:00Z"
}
```

### Record CPA Outcome (Internal)

#### POST /internal/v1/outcomes

```json
// Request (Enhanced)
{
  "contract_id": "contract_789xyz",
  "provider_id": "prov_abc123",
  "agent_id": "agent_xyz789",
  "consumer_id": "tenant_123",
  "work_category": "travel.booking",
  "outcome": "SUCCESS",
  "metrics": {
    "latency_ms": 2300,
    "sla_met": true
  },
  "agreed_price": 0.08,
  "final_price": 0.13,
  "completed_at": "2025-01-15T10:32:00Z",
  "cpa_outcome": {                           // NEW
    "verification_id": "ver_abc123",
    "verification_status": "verified",
    "criteria_results": [
      {
        "metric": "booking_confirmed",
        "target": true,
        "actual": true,
        "met": true,
        "bonus_earned": 0.05
      },
      {
        "metric": "response_time_ms",
        "target": 3000,
        "actual": 2300,
        "met": true,
        "bonus_earned": 0.02
      }
    ],
    "base_price": 0.08,
    "bonus_earned": 0.07,
    "penalty_applied": 0.0,
    "final_amount": 0.15,
    "provider_confidence": 0.92
  }
}

// Response
{
  "recorded": true,
  "provider_id": "prov_abc123",
  "previous_score": 0.85,
  "new_score": 0.87,
  "previous_composite": 0.88,
  "new_composite": 0.89,
  "tier_changed": false,
  "cpa_stats_updated": true,
  "ml_features_queued": true
}
```

### Report Outcome Dispute

#### POST /v1/disputes/outcome

Dispute CPA outcome verification.

```json
// Request
{
  "contract_id": "contract_789xyz",
  "verification_id": "ver_abc123",
  "disputed_by": "consumer",
  "disputed_criteria": ["booking_confirmed"],
  "reason": "verification_incorrect",
  "description": "Booking was not actually confirmed despite verification passing",
  "evidence": [
    {
      "type": "screenshot",
      "url": "https://storage.example.com/evidence/123.png",
      "description": "Screenshot showing booking not found"
    },
    {
      "type": "api_response",
      "value": "{\"status\": \"not_found\", \"confirmation\": null}",
      "description": "API response when querying booking"
    }
  ]
}

// Response
{
  "dispute_id": "disp_abc123",
  "contract_id": "contract_789xyz",
  "status": "OPEN",
  "escalation_level": "outcome_verification",
  "estimated_resolution": "2025-01-16T10:00:00Z",
  "created_at": "2025-01-15T11:00:00Z"
}
```

### Get Outcome Dispute Status

#### GET /v1/disputes/{dispute_id}

```json
{
  "dispute_id": "disp_abc123",
  "contract_id": "contract_789xyz",
  "verification_id": "ver_abc123",
  "disputed_by": "consumer",
  "disputed_criteria": ["booking_confirmed"],
  "status": "INVESTIGATING",
  "investigation": {
    "started_at": "2025-01-15T12:00:00Z",
    "investigator": "system_oracle",
    "evidence_collected": 3,
    "preliminary_finding": null
  },
  "timeline": [
    {
      "action": "dispute_opened",
      "timestamp": "2025-01-15T11:00:00Z",
      "actor": "consumer"
    },
    {
      "action": "investigation_started",
      "timestamp": "2025-01-15T12:00:00Z",
      "actor": "system"
    }
  ]
}
```

### Resolve Outcome Dispute (Internal)

#### POST /internal/v1/disputes/{dispute_id}/resolve

```json
// Request
{
  "resolution": "partial_upheld",
  "resolved_by": "admin_user",
  "criteria_adjustments": [
    {
      "metric": "booking_confirmed",
      "original_met": true,
      "adjusted_met": false,
      "bonus_adjusted": -0.05,
      "reason": "Evidence shows booking was not confirmed"
    }
  ],
  "settlement_adjustment": -0.05,
  "notes": "Consumer evidence confirmed booking was not actually completed"
}

// Response
{
  "dispute_id": "disp_abc123",
  "status": "RESOLVED",
  "resolution": {
    "outcome": "partial_upheld",
    "criteria_adjustments": [...],
    "settlement_adjustment": -0.05,
    "provider_trust_impact": -0.02
  },
  "resolved_at": "2025-01-16T09:00:00Z"
}
```

## Core Implementation

### Composite Score Calculation

```python
async def get_composite_score(
    provider_id: str,
    work_category: str | None = None,
    criteria_metrics: list[str] | None = None
) -> CompositeScore:
    """Calculate blended historical + ML trust score."""
    # 1. Get historical trust record
    record = await firestore.get_trust_record(provider_id)
    historical_score = record.trust_score

    # 2. Get ML prediction (from cache or trust-scoring)
    ml_prediction = await get_ml_prediction(
        provider_id,
        work_category,
        criteria_metrics
    )

    # 3. Calculate composite score
    # Default weights: 60% historical, 40% ML
    # Adjust based on confidence and data availability
    weights = calculate_weights(record, ml_prediction)

    composite = (
        weights["historical"] * historical_score +
        weights["ml"] * ml_prediction.predicted_success_rate
    )

    return CompositeScore(
        provider_id=provider_id,
        historical_score=historical_score,
        ml_prediction=ml_prediction.predicted_success_rate,
        composite_score=composite,
        weights=weights,
        confidence=ml_prediction.confidence,
        computed_at=datetime.utcnow()
    )


async def get_ml_prediction(
    provider_id: str,
    work_category: str | None,
    criteria_metrics: list[str] | None
) -> MLPredictionCache:
    """Get ML prediction from cache or trust-scoring service."""
    # Check cache first
    cache_key = f"ml_pred:{provider_id}:{work_category or 'general'}"
    cached = await redis.get(cache_key)

    if cached:
        prediction = MLPredictionCache(**json.loads(cached))
        if prediction.expires_at > datetime.utcnow():
            return prediction

    # Fetch from trust-scoring service
    try:
        prediction = await trust_scoring.predict_success(
            agent_id=await get_agent_id(provider_id),
            category=work_category,
            criteria=criteria_metrics or []
        )

        # Cache with 5-minute TTL
        cache_entry = MLPredictionCache(
            predicted_success_rate=prediction.overall_success_rate,
            confidence=prediction.confidence,
            criteria_predictions=prediction.criteria_success,
            model_version=prediction.model_version,
            computed_at=datetime.utcnow(),
            expires_at=datetime.utcnow() + timedelta(minutes=5)
        )
        await redis.setex(cache_key, 300, cache_entry.json())

        return cache_entry
    except Exception as e:
        # Fallback to historical-only
        logger.warning("ML prediction unavailable", provider_id=provider_id, error=str(e))
        return MLPredictionCache(
            predicted_success_rate=0.7,  # Default fallback
            confidence=0.0,              # Low confidence
            criteria_predictions={},
            model_version="fallback",
            computed_at=datetime.utcnow(),
            expires_at=datetime.utcnow() + timedelta(minutes=1)
        )


def calculate_weights(
    record: TrustRecord,
    ml_prediction: MLPredictionCache
) -> dict[str, float]:
    """Calculate dynamic weights based on data quality."""
    # Base weights
    hist_weight = 0.6
    ml_weight = 0.4

    # Adjust based on contract history
    if record.total_contracts < 10:
        # New provider: rely more on ML
        hist_weight = 0.3
        ml_weight = 0.7
    elif record.total_contracts < 50:
        # Moderate history: balanced
        hist_weight = 0.5
        ml_weight = 0.5
    elif record.total_contracts >= 200:
        # Strong history: rely more on historical
        hist_weight = 0.7
        ml_weight = 0.3

    # Adjust based on ML confidence
    if ml_prediction.confidence < 0.5:
        # Low ML confidence: favor historical
        ml_weight *= 0.5
        hist_weight = 1 - ml_weight

    # Normalize
    total = hist_weight + ml_weight
    return {
        "historical": hist_weight / total,
        "ml": ml_weight / total
    }
```

### Enhanced Outcome Recording

```python
async def record_cpa_outcome(outcome: ContractOutcome) -> dict:
    """Record CPA contract outcome with criteria details."""
    # 1. Store outcome (Phase A)
    await firestore.save_outcome(outcome)

    # 2. Get current trust record
    record = await firestore.get_trust_record(outcome.provider_id)
    previous_score = record.trust_score
    previous_composite = record.composite_score

    # 3. Update Phase A stats
    record.total_contracts += 1
    if outcome.outcome in [OutcomeType.SUCCESS, OutcomeType.SUCCESS_PARTIAL]:
        record.successful_contracts += 1
    elif outcome.outcome in [OutcomeType.FAILURE_PROVIDER, OutcomeType.DISPUTE_LOST]:
        record.failed_contracts += 1

    record.last_contract_at = outcome.completed_at

    # 4. NEW: Update CPA stats
    if outcome.cpa_enabled and outcome.cpa_outcome:
        if not record.cpa_stats:
            record.cpa_stats = CPAStats()

        cpa = outcome.cpa_outcome
        stats = record.cpa_stats

        stats.total_cpa_contracts += 1
        stats.total_bonuses_earned += cpa.bonus_earned
        stats.total_penalties_incurred += cpa.penalty_applied
        stats.net_cpa_earnings = stats.total_bonuses_earned - stats.total_penalties_incurred

        # Update criteria performance
        for result in cpa.criteria_results:
            metric = result.metric
            if metric not in stats.criteria_performance:
                stats.criteria_performance[metric] = 0.0

            # Running average update
            current_rate = stats.criteria_performance[metric]
            n = stats.total_cpa_contracts
            stats.criteria_performance[metric] = (
                current_rate * (n - 1) + (1.0 if result.met else 0.0)
            ) / n

        # Update overall criteria met rate
        total_criteria = sum(len(o.cpa_outcome.criteria_results)
                            for o in await get_recent_outcomes(outcome.provider_id, 100)
                            if o.cpa_outcome)
        met_criteria = sum(sum(1 for r in o.cpa_outcome.criteria_results if r.met)
                          for o in await get_recent_outcomes(outcome.provider_id, 100)
                          if o.cpa_outcome)
        stats.criteria_met_rate = met_criteria / total_criteria if total_criteria > 0 else 0

        # Update confidence calibration
        stats.confidence_calibration = update_calibration(
            stats.confidence_calibration,
            cpa.provider_confidence,
            1.0 if outcome.outcome == OutcomeType.SUCCESS else 0.0,
            stats.total_cpa_contracts
        )

        record.cpa_enabled = True
        record.cpa_stats = stats

    # 5. Recalculate trust score
    updated_record = await calculate_trust_score(outcome.provider_id)

    # 6. Recalculate composite score
    composite = await get_composite_score(outcome.provider_id)
    updated_record.composite_score = composite.composite_score

    # 7. Check tier change with CPA factors
    tier_changed = updated_record.trust_tier != record.trust_tier
    if not tier_changed and record.cpa_stats:
        # Check if CPA performance warrants tier change
        tier_changed = check_cpa_tier_impact(record, updated_record)

    # 8. Publish events
    await pubsub.publish("trust.score_updated", {
        "provider_id": outcome.provider_id,
        "previous_score": previous_score,
        "new_score": updated_record.trust_score,
        "previous_composite": previous_composite,
        "new_composite": updated_record.composite_score,
        "reason": "contract_completed"
    })

    if tier_changed:
        await pubsub.publish("trust.tier_changed", {
            "provider_id": outcome.provider_id,
            "old_tier": record.trust_tier.value,
            "new_tier": updated_record.trust_tier.value,
            "trust_score": updated_record.trust_score
        })

    # 9. Notify trust-scoring for ML update
    await pubsub.publish("trust.outcome_recorded", {
        "provider_id": outcome.provider_id,
        "contract_id": outcome.contract_id,
        "category": outcome.work_category,
        "cpa_enabled": outcome.cpa_enabled,
        "success": outcome.outcome == OutcomeType.SUCCESS
    })

    # 10. Store to BigQuery
    await bigquery.insert_outcome(outcome)

    return {
        "recorded": True,
        "provider_id": outcome.provider_id,
        "previous_score": previous_score,
        "new_score": updated_record.trust_score,
        "previous_composite": previous_composite,
        "new_composite": updated_record.composite_score,
        "tier_changed": tier_changed,
        "cpa_stats_updated": outcome.cpa_enabled,
        "ml_features_queued": True
    }


def check_cpa_tier_impact(old_record: TrustRecord, new_record: TrustRecord) -> bool:
    """Check if CPA performance warrants tier change."""
    if not new_record.cpa_stats or new_record.cpa_stats.total_cpa_contracts < 20:
        return False

    stats = new_record.cpa_stats

    # Exceptional CPA performance can accelerate tier upgrade
    if (stats.cpa_success_rate >= 0.95 and
        stats.criteria_met_rate >= 0.90 and
        stats.confidence_calibration >= 0.85):
        # May qualify for accelerated PREFERRED tier
        if new_record.trust_tier == TrustTier.TRUSTED:
            # Check if they meet relaxed contract count for CPA excellence
            if new_record.total_contracts >= 75:  # vs normal 100
                return True

    # Poor CPA performance can trigger tier downgrade
    if stats.cpa_success_rate < 0.70 or stats.criteria_met_rate < 0.60:
        if new_record.trust_tier in [TrustTier.TRUSTED, TrustTier.PREFERRED]:
            return True  # Trigger review

    return False
```

### Outcome Dispute Resolution

```python
async def handle_outcome_dispute(dispute: OutcomeDispute) -> None:
    """Handle CPA outcome dispute."""
    # 1. Get contract and verification
    contract = await contract_engine.get_contract(dispute.contract_id)
    verification = await outcome_oracle.get_verification(dispute.verification_id)

    # 2. Create dispute record
    dispute_record = await firestore.save_dispute(dispute)

    # 3. Publish event
    await pubsub.publish("trust.outcome_dispute_opened", {
        "dispute_id": dispute.dispute_id,
        "contract_id": dispute.contract_id,
        "provider_id": contract.provider_id,
        "disputed_by": dispute.disputed_by,
        "disputed_criteria": dispute.disputed_criteria
    })

    # 4. Request re-verification from Outcome Oracle
    if dispute.reason in ["verification_incorrect", "evidence_invalid"]:
        await outcome_oracle.request_reverification(
            verification_id=dispute.verification_id,
            disputed_criteria=dispute.disputed_criteria,
            additional_evidence=dispute.evidence
        )

    # 5. Temporarily hold settlement if pending
    await settlement.hold_for_dispute(dispute.contract_id)


async def resolve_outcome_dispute(
    dispute_id: str,
    resolution: OutcomeDisputeResolution
) -> dict:
    """Resolve an outcome dispute with adjustments."""
    dispute = await firestore.get_dispute(dispute_id)
    contract = await contract_engine.get_contract(dispute.contract_id)

    # 1. Apply criteria adjustments
    for adjustment in resolution.criteria_adjustments:
        # Update the verification record
        await outcome_oracle.update_criteria_result(
            dispute.verification_id,
            adjustment.metric,
            adjustment.adjusted_met
        )

    # 2. Update contract settlement
    if resolution.settlement_adjustment != 0:
        await settlement.adjust_settlement(
            contract.id,
            resolution.settlement_adjustment,
            reason=f"dispute_{dispute_id}_resolution"
        )

    # 3. Update provider trust based on outcome
    provider_id = contract.provider_id
    trust_impact = calculate_dispute_trust_impact(resolution)

    if trust_impact != 0:
        await adjust_trust_score(provider_id, trust_impact, reason="dispute_resolution")

    # 4. Update dispute record
    dispute.status = "RESOLVED"
    dispute.resolution = resolution
    await firestore.update_dispute(dispute)

    # 5. Publish resolution event
    await pubsub.publish("trust.outcome_dispute_resolved", {
        "dispute_id": dispute_id,
        "contract_id": dispute.contract_id,
        "provider_id": provider_id,
        "outcome": resolution.outcome,
        "settlement_adjustment": resolution.settlement_adjustment,
        "trust_impact": trust_impact
    })

    # 6. Release settlement hold
    await settlement.release_dispute_hold(dispute.contract_id)

    return {
        "dispute_id": dispute_id,
        "status": "RESOLVED",
        "resolution": resolution.dict(),
        "provider_trust_impact": trust_impact
    }


def calculate_dispute_trust_impact(resolution: OutcomeDisputeResolution) -> float:
    """Calculate trust score impact from dispute resolution."""
    if resolution.outcome == "upheld":
        # Dispute against provider was upheld
        return -0.05  # Significant negative impact
    elif resolution.outcome == "overturned":
        # Provider was vindicated
        return 0.01   # Small positive impact
    elif resolution.outcome == "partial_upheld":
        # Mixed result
        return -0.02  # Moderate negative impact
    return 0.0
```

## Events

### Published Events (Enhanced)

```json
// Trust score updated (enhanced)
{
  "event_type": "trust.score_updated",
  "provider_id": "prov_abc123",
  "previous_score": 0.85,
  "new_score": 0.87,
  "previous_composite": 0.88,               // NEW
  "new_composite": 0.89,                    // NEW
  "reason": "contract_completed",
  "timestamp": "2025-01-15T10:35:00Z"
}

// Trust tier changed
{
  "event_type": "trust.tier_changed",
  "provider_id": "prov_abc123",
  "old_tier": "TRUSTED",
  "new_tier": "PREFERRED",
  "trust_score": 0.91,
  "cpa_accelerated": true,                  // NEW
  "timestamp": "2025-01-15T10:35:00Z"
}

// NEW: Outcome recorded for ML
{
  "event_type": "trust.outcome_recorded",
  "provider_id": "prov_abc123",
  "contract_id": "contract_789xyz",
  "category": "travel.booking",
  "cpa_enabled": true,
  "success": true,
  "timestamp": "2025-01-15T10:35:00Z"
}

// NEW: Outcome dispute opened
{
  "event_type": "trust.outcome_dispute_opened",
  "dispute_id": "disp_abc123",
  "contract_id": "contract_789xyz",
  "provider_id": "prov_abc123",
  "disputed_by": "consumer",
  "disputed_criteria": ["booking_confirmed"],
  "timestamp": "2025-01-15T11:00:00Z"
}

// NEW: Outcome dispute resolved
{
  "event_type": "trust.outcome_dispute_resolved",
  "dispute_id": "disp_abc123",
  "contract_id": "contract_789xyz",
  "provider_id": "prov_abc123",
  "outcome": "partial_upheld",
  "settlement_adjustment": -0.05,
  "trust_impact": -0.02,
  "timestamp": "2025-01-16T09:00:00Z"
}
```

### Consumed Events

```json
// Contract completed (enhanced)
{
  "event_type": "contract.completed",
  "contract_id": "contract_789xyz",
  "provider_id": "prov_abc123",
  "cpa_details": {...}
}

// NEW: ML prediction updated (from trust-scoring)
{
  "event_type": "trust.prediction_updated",
  "agent_id": "agent_xyz789",
  "predicted_success_rate": 0.93,
  "confidence": 0.88,
  "model_version": "v2.2.0"
}

// NEW: Outcome verified (from outcome-oracle)
{
  "event_type": "outcome.verified",
  "contract_id": "contract_789xyz",
  "verification_id": "ver_abc123",
  "status": "verified",
  "criteria_results": [...]
}
```

## Integration with Trust Scoring

```python
class TrustScoringClient:
    """Client for aex-trust-scoring service."""

    def __init__(self, base_url: str, service_token: str):
        self.base_url = base_url
        self.service_token = service_token

    async def predict_success(
        self,
        agent_id: str,
        category: str | None,
        criteria: list[str]
    ) -> MLPrediction:
        """Get ML prediction for agent's success probability."""
        response = await self.http.post(
            f"{self.base_url}/internal/v1/predictions/task-success",
            headers={"Authorization": f"Bearer {self.service_token}"},
            json={
                "agent_id": agent_id,
                "category": category,
                "criteria_metrics": criteria
            },
            timeout=0.5  # 500ms timeout
        )

        data = response.json()
        return MLPrediction(
            agent_id=agent_id,
            overall_success_rate=data["predicted_success_rate"],
            criteria_success=data["criteria_predictions"],
            confidence=data["confidence"],
            model_version=data["model_version"]
        )

    async def get_cached_prediction(self, agent_id: str) -> MLPrediction | None:
        """Get cached prediction without compute."""
        response = await self.http.get(
            f"{self.base_url}/internal/v1/predictions/{agent_id}/cached",
            headers={"Authorization": f"Bearer {self.service_token}"}
        )

        if response.status_code == 404:
            return None

        return MLPrediction(**response.json())
```

## Configuration

```yaml
# config/trust-broker.yaml (Phase B additions)
trust_broker:
  # Phase A config unchanged...

  # Phase B additions
  composite_scoring:
    enabled: true
    default_historical_weight: 0.6
    default_ml_weight: 0.4
    min_contracts_for_ml: 5

  ml_integration:
    trust_scoring_url: ${TRUST_SCORING_URL}
    prediction_cache_ttl_seconds: 300
    fallback_on_timeout: true
    timeout_ms: 500

  cpa:
    track_criteria_performance: true
    cpa_tier_acceleration: true
    min_cpa_contracts_for_acceleration: 20
    excellence_threshold:
      success_rate: 0.95
      criteria_met_rate: 0.90
      calibration: 0.85

  outcome_disputes:
    enabled: true
    auto_reverify: true
    settlement_hold_on_dispute: true
    max_resolution_days: 7

  prediction_cache:
    redis_url: ${REDIS_URL}
    ttl_seconds: 300
    refresh_threshold_seconds: 60
```

## Metrics

```python
# Phase B metrics
trust_composite_score = Histogram(
    "trust_composite_score",
    "Composite trust scores distribution",
    buckets=[0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0]
)

trust_ml_prediction_latency = Histogram(
    "trust_ml_prediction_latency_seconds",
    "ML prediction fetch latency",
    buckets=[0.01, 0.05, 0.1, 0.2, 0.5, 1.0]
)

trust_ml_cache_hit_rate = Counter(
    "trust_ml_cache_total",
    "ML prediction cache access",
    ["status"]  # hit, miss, expired
)

trust_cpa_outcomes = Counter(
    "trust_cpa_outcomes_total",
    "CPA outcome recordings",
    ["success", "bonus_earned", "penalty_applied"]
)

trust_outcome_disputes = Counter(
    "trust_outcome_disputes_total",
    "Outcome disputes",
    ["disputed_by", "status"]
)

trust_dispute_resolution = Histogram(
    "trust_dispute_resolution_hours",
    "Time to resolve outcome disputes",
    buckets=[1, 4, 12, 24, 48, 72, 168]
)

trust_tier_acceleration = Counter(
    "trust_tier_acceleration_total",
    "CPA-accelerated tier upgrades"
)
```

## Backward Compatibility

1. **Non-CPA Contracts**: Processed as Phase A, no CPA stats updated
2. **Composite Score**: Falls back to historical if ML unavailable
3. **Legacy Disputes**: Handled by Phase A dispute flow
4. **API Compatibility**: New fields added, old responses unchanged
5. **Events**: New fields added, consumers ignore unknown
