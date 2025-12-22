# Enhanced Provider Registry Specification (Phase B)

## Overview

Phase B enhances `aex-provider-registry` to support CPA-capable providers, outcome history tracking, and ML-ready provider profiling for trust predictions.

## Changes from Phase A

| Aspect | Phase A | Phase B |
|--------|---------|---------|
| Provider Profile | Basic capabilities | + CPA eligibility, outcome history |
| Trust Data | Score + tier | + ML features, prediction data |
| Subscriptions | Category filters | + CPA preference, pricing tiers |
| Validation | Endpoint check | + CPA capability verification |
| Provider Stats | Basic metrics | + Outcome-based performance |

## Architecture

```
                External Providers
                      │
                      ▼
         ┌─────────────────────────────┐
         │ aex-provider-registry       │◄── THIS SERVICE
         │     (Enhanced)              │
         │                             │
         │ • Register CPA providers    │
         │ • Track outcome history     │
         │ • Manage pricing tiers      │
         │ • ML feature collection     │
         └───────────┬─────────────────┘
                     │
          ┌──────────┼──────────────┬───────────┐
          ▼          ▼              ▼           ▼
     Firestore    Pub/Sub     Trust Broker  Trust Scoring
    (providers)  (events)    (tier mgmt)    (ML features)
```

## Enhanced Data Models

```python
class Provider(BaseModel):
    # Phase A fields
    id: str
    name: str
    description: str
    endpoint: str
    bid_webhook: str | None
    capabilities: list[str]
    contact_email: str
    metadata: dict
    api_key_hash: str
    api_secret_hash: str
    status: ProviderStatus
    trust_score: float
    trust_tier: TrustTier
    created_at: datetime
    updated_at: datetime
    verified_at: datetime | None

    # NEW: Phase B CPA fields
    cpa_enabled: bool = False
    cpa_profile: CPAProfile | None = None
    outcome_history: OutcomeHistory | None = None
    pricing_tiers: list[PricingTier] = []
    ml_features: MLProviderFeatures | None = None


class CPAProfile(BaseModel):
    """Provider's CPA capability profile."""
    accepts_cpa_terms: bool = True
    max_penalty_accepted: float = 0.20        # Max penalty rate willing to accept
    preferred_bonus_structure: str = "per_criterion"  # per_criterion, flat, percentage

    # Capability assessment
    verification_support: list[str] = []      # confirmation_number, receipt, api_callback
    supported_metrics: list[str] = []         # booking_confirmed, response_time_ms, etc.

    # Risk preferences
    min_confidence_threshold: float = 0.7     # Won't bid if confidence below this
    penalty_buffer_percentage: float = 10     # Extra margin built into bids

    # Certification
    cpa_certified: bool = False
    certification_date: datetime | None = None
    certification_expires: datetime | None = None


class OutcomeHistory(BaseModel):
    """Aggregated outcome performance history."""
    total_cpa_contracts: int = 0
    total_bonuses_earned: float = 0.0
    total_penalties_incurred: float = 0.0

    # Success rates by metric type
    criteria_performance: dict[str, CriteriaPerformance] = {}

    # Category performance
    category_performance: dict[str, CategoryPerformance] = {}

    # Time-based trends
    last_30_days: PerformanceWindow | None = None
    last_90_days: PerformanceWindow | None = None
    lifetime: PerformanceWindow | None = None

    # Consistency metrics
    prediction_accuracy: float = 0.0          # How accurate provider's confidence was
    volatility: float = 0.0                   # Variance in performance


class CriteriaPerformance(BaseModel):
    """Performance for a specific criterion type."""
    metric: str
    total_attempts: int
    times_met: int
    success_rate: float
    avg_margin: float                         # How much above/below threshold
    last_updated: datetime


class CategoryPerformance(BaseModel):
    """Performance in a work category."""
    category: str
    contracts_completed: int
    success_rate: float
    avg_bonus_rate: float
    avg_penalty_rate: float
    last_contract: datetime


class PerformanceWindow(BaseModel):
    """Performance metrics for a time window."""
    contracts: int
    success_rate: float
    bonus_rate: float                         # Bonus earned / potential bonus
    penalty_rate: float                       # Penalties / base price
    avg_confidence_accuracy: float


class PricingTier(BaseModel):
    """Provider's pricing preferences by category."""
    category: str
    base_price_range: tuple[float, float]     # Min-max base price
    accepts_cpa: bool
    min_bonus_for_cpa: float                  # Min bonus to accept CPA terms
    max_penalty_for_cpa: float                # Max penalty willing to accept
    priority: int                             # Preference order


class MLProviderFeatures(BaseModel):
    """Features used by ML model for predictions."""
    # Behavioral features
    avg_bid_price: float
    bid_win_rate: float
    avg_confidence: float
    confidence_calibration: float             # Actual vs predicted success

    # Performance features
    completion_rate: float
    avg_latency_vs_sla: float                 # Ratio of actual to SLA
    criteria_meet_rate: float

    # Stability features
    active_days: int
    contracts_per_day: float
    consistency_score: float

    # Risk features
    dispute_rate: float
    penalty_frequency: float
    recent_failures: int

    # Computed at
    computed_at: datetime
    model_version: str
```

## Enhanced API Endpoints

### Register Provider (Enhanced)

#### POST /v1/providers

```json
// Request (Enhanced)
{
  "name": "Expedia Travel Agent",
  "description": "Full-service travel booking and search",
  "endpoint": "https://agent.expedia.com/a2a",
  "bid_webhook": "https://agent.expedia.com/aex/work",
  "capabilities": ["travel.booking", "travel.search", "hospitality.hotels"],
  "contact_email": "agents@expedia.com",
  "metadata": {
    "company": "Expedia Group",
    "region": "global"
  },
  "cpa_preferences": {                          // NEW
    "accepts_cpa_terms": true,
    "max_penalty_accepted": 0.25,
    "verification_support": ["confirmation_number", "receipt", "api_callback"],
    "supported_metrics": ["booking_confirmed", "response_time_ms", "price_accuracy"]
  },
  "pricing_tiers": [                            // NEW
    {
      "category": "travel.booking",
      "base_price_range": [0.05, 0.15],
      "accepts_cpa": true,
      "min_bonus_for_cpa": 0.02,
      "max_penalty_for_cpa": 0.25
    }
  ]
}

// Response (Enhanced)
{
  "provider_id": "prov_abc123",
  "api_key": "aex_pk_live_...",
  "api_secret": "aex_sk_live_...",
  "status": "PENDING_VERIFICATION",
  "trust_tier": "UNVERIFIED",
  "cpa_enabled": true,                          // NEW
  "cpa_status": "PENDING_CERTIFICATION",        // NEW
  "created_at": "2025-01-15T10:00:00Z"
}
```

### Get Provider (Enhanced)

#### GET /v1/providers/{provider_id}

```json
{
  "provider_id": "prov_abc123",
  "name": "Expedia Travel Agent",
  "endpoint": "https://agent.expedia.com/a2a",
  "status": "ACTIVE",
  "trust_score": 0.87,
  "trust_tier": "TRUSTED",
  "capabilities": ["travel.booking", "travel.search"],
  "subscriptions": [...],
  "stats": {
    "total_contracts": 1250,
    "success_rate": 0.94,
    "avg_response_time_ms": 1200
  },
  "cpa_profile": {                              // NEW
    "accepts_cpa_terms": true,
    "max_penalty_accepted": 0.25,
    "cpa_certified": true,
    "certification_expires": "2026-01-15T00:00:00Z"
  },
  "outcome_summary": {                          // NEW
    "cpa_contracts": 450,
    "bonus_earned_30d": 28.50,
    "penalty_incurred_30d": 3.20,
    "net_cpa_performance": 25.30,
    "top_criteria": [
      {"metric": "booking_confirmed", "success_rate": 0.96},
      {"metric": "response_time_ms", "success_rate": 0.89}
    ]
  },
  "ml_prediction": {                            // NEW
    "predicted_success_rate": 0.92,
    "confidence": 0.85,
    "risk_level": "low"
  }
}
```

### Update CPA Profile

#### PUT /v1/providers/{provider_id}/cpa-profile

```json
// Request
{
  "accepts_cpa_terms": true,
  "max_penalty_accepted": 0.30,
  "verification_support": ["confirmation_number", "receipt", "api_callback", "screenshot"],
  "supported_metrics": ["booking_confirmed", "response_time_ms", "price_accuracy", "options_count"],
  "min_confidence_threshold": 0.75
}

// Response
{
  "provider_id": "prov_abc123",
  "cpa_profile": {
    "accepts_cpa_terms": true,
    "max_penalty_accepted": 0.30,
    "verification_support": [...],
    "cpa_certified": true
  },
  "updated_at": "2025-01-15T12:00:00Z"
}
```

### Get Outcome History

#### GET /v1/providers/{provider_id}/outcome-history

```json
{
  "provider_id": "prov_abc123",
  "outcome_history": {
    "total_cpa_contracts": 450,
    "total_bonuses_earned": 285.50,
    "total_penalties_incurred": 32.20,
    "net_cpa_earnings": 253.30,
    "criteria_performance": {
      "booking_confirmed": {
        "total_attempts": 420,
        "times_met": 403,
        "success_rate": 0.96,
        "avg_margin": 0.12
      },
      "response_time_ms": {
        "total_attempts": 380,
        "times_met": 338,
        "success_rate": 0.89,
        "avg_margin": 450
      }
    },
    "category_performance": {
      "travel.booking": {
        "contracts_completed": 350,
        "success_rate": 0.94,
        "avg_bonus_rate": 0.65,
        "avg_penalty_rate": 0.08
      }
    },
    "last_30_days": {
      "contracts": 45,
      "success_rate": 0.96,
      "bonus_rate": 0.72,
      "penalty_rate": 0.04,
      "avg_confidence_accuracy": 0.91
    }
  }
}
```

### Enhanced Subscription with CPA Preferences

#### POST /v1/subscriptions

```json
// Request (Enhanced)
{
  "provider_id": "prov_abc123",
  "categories": ["travel.*", "hospitality.hotels"],
  "filters": {
    "min_budget": 0.05,
    "max_latency_ms": 5000,
    "regions": ["us", "eu"]
  },
  "cpa_preferences": {                          // NEW
    "only_cpa_enabled": false,                  // Only receive CPA-enabled work
    "min_bonus_opportunity": 0.02,              // Min potential bonus
    "max_penalty_exposure": 0.25,               // Max penalty rate
    "preferred_criteria": ["booking_confirmed", "response_time_ms"]
  },
  "delivery": {
    "method": "webhook",
    "webhook_url": "https://agent.expedia.com/aex/work",
    "webhook_secret": "whsec_..."
  }
}

// Response
{
  "subscription_id": "sub_xyz789",
  "provider_id": "prov_abc123",
  "categories": ["travel.*", "hospitality.hotels"],
  "cpa_filter_active": true,
  "status": "ACTIVE",
  "created_at": "2025-01-15T10:05:00Z"
}
```

### Internal APIs (Enhanced)

#### GET /internal/v1/providers/subscribed

```json
// Request: GET /internal/v1/providers/subscribed?category=travel.booking&cpa_required=true

// Response (Enhanced)
{
  "category": "travel.booking",
  "cpa_required": true,
  "providers": [
    {
      "provider_id": "prov_abc123",
      "webhook_url": "https://agent.expedia.com/aex/work",
      "trust_score": 0.87,
      "trust_tier": "TRUSTED",
      "cpa_profile": {                          // NEW
        "cpa_certified": true,
        "max_penalty_accepted": 0.25,
        "supported_metrics": ["booking_confirmed", "response_time_ms"]
      },
      "outcome_summary": {                      // NEW
        "cpa_success_rate": 0.94,
        "avg_bonus_rate": 0.65
      },
      "ml_prediction": {                        // NEW
        "predicted_success_rate": 0.92,
        "confidence": 0.85
      }
    }
  ]
}
```

#### GET /internal/v1/providers/{provider_id}/ml-features

```json
// Response
{
  "provider_id": "prov_abc123",
  "features": {
    "avg_bid_price": 0.085,
    "bid_win_rate": 0.32,
    "avg_confidence": 0.88,
    "confidence_calibration": 0.94,
    "completion_rate": 0.97,
    "avg_latency_vs_sla": 0.78,
    "criteria_meet_rate": 0.91,
    "active_days": 245,
    "contracts_per_day": 5.2,
    "consistency_score": 0.89,
    "dispute_rate": 0.02,
    "penalty_frequency": 0.08,
    "recent_failures": 2
  },
  "computed_at": "2025-01-15T10:00:00Z",
  "model_version": "v2.1.0"
}
```

#### POST /internal/v1/providers/{provider_id}/outcome

Record outcome for ML feature updates.

```json
// Request
{
  "contract_id": "contract_789xyz",
  "work_category": "travel.booking",
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
  "provider_confidence": 0.92,
  "actual_success": true,
  "penalty_applied": 0.0
}

// Response
{
  "acknowledged": true,
  "outcome_history_updated": true,
  "ml_features_queued": true
}
```

## Core Implementation

### Enhanced Provider Registration

```python
async def register_provider(req: EnhancedProviderRegistration) -> Provider:
    # Phase A: Validate endpoint
    endpoint_valid = await validate_endpoint(req.endpoint)
    if not endpoint_valid:
        raise HTTPException(400, "Endpoint not accessible")

    # NEW Phase B: Validate CPA capabilities if declared
    cpa_profile = None
    if req.cpa_preferences:
        cpa_profile = await validate_cpa_capabilities(req)

    # Generate credentials
    api_key = generate_api_key()
    api_secret = generate_api_secret()

    # Get initial trust score
    trust_score = await trust_broker.get_initial_score()

    # Create provider
    provider = Provider(
        id=generate_provider_id(),
        name=req.name,
        endpoint=req.endpoint,
        capabilities=req.capabilities,
        api_key_hash=hash_key(api_key),
        api_secret_hash=hash_key(api_secret),
        status=ProviderStatus.PENDING_VERIFICATION,
        trust_score=trust_score,
        trust_tier=TrustTier.UNVERIFIED,
        created_at=datetime.utcnow(),
        # Phase B fields
        cpa_enabled=cpa_profile is not None,
        cpa_profile=cpa_profile,
        outcome_history=OutcomeHistory(),
        pricing_tiers=req.pricing_tiers or [],
        ml_features=None  # Computed after first contracts
    )

    await firestore.save_provider(provider)

    # Publish enhanced event
    await pubsub.publish("provider.registered", {
        "provider_id": provider.id,
        "name": provider.name,
        "cpa_enabled": provider.cpa_enabled,
        "capabilities": provider.capabilities,
        "timestamp": datetime.utcnow().isoformat()
    })

    return ProviderResponse(
        provider_id=provider.id,
        api_key=api_key,
        api_secret=api_secret,
        status=provider.status,
        cpa_enabled=provider.cpa_enabled,
        cpa_status="PENDING_CERTIFICATION" if provider.cpa_enabled else None
    )


async def validate_cpa_capabilities(req: EnhancedProviderRegistration) -> CPAProfile:
    """Validate and build CPA profile."""
    prefs = req.cpa_preferences

    # Validate supported metrics are known
    known_metrics = await get_known_metrics()
    for metric in prefs.supported_metrics:
        if metric not in known_metrics:
            raise HTTPException(400, f"Unknown metric: {metric}")

    # Validate verification support
    valid_verification = {"confirmation_number", "receipt", "api_callback", "screenshot"}
    for v in prefs.verification_support:
        if v not in valid_verification:
            raise HTTPException(400, f"Invalid verification type: {v}")

    return CPAProfile(
        accepts_cpa_terms=prefs.accepts_cpa_terms,
        max_penalty_accepted=min(prefs.max_penalty_accepted, 0.50),  # Cap at 50%
        verification_support=prefs.verification_support,
        supported_metrics=prefs.supported_metrics,
        min_confidence_threshold=prefs.get("min_confidence_threshold", 0.7),
        cpa_certified=False,  # Requires certification process
        certification_date=None
    )
```

### Outcome History Update

```python
async def record_outcome(
    provider_id: str,
    outcome: ContractOutcome
) -> None:
    """Record contract outcome for provider history."""
    provider = await firestore.get_provider(provider_id)

    if not provider.outcome_history:
        provider.outcome_history = OutcomeHistory()

    history = provider.outcome_history

    # Update totals
    history.total_cpa_contracts += 1
    history.total_bonuses_earned += sum(
        r.bonus_earned for r in outcome.criteria_results if r.met
    )
    history.total_penalties_incurred += outcome.penalty_applied

    # Update criteria performance
    for result in outcome.criteria_results:
        metric = result.metric
        if metric not in history.criteria_performance:
            history.criteria_performance[metric] = CriteriaPerformance(
                metric=metric,
                total_attempts=0,
                times_met=0,
                success_rate=0.0,
                avg_margin=0.0,
                last_updated=datetime.utcnow()
            )

        perf = history.criteria_performance[metric]
        perf.total_attempts += 1
        if result.met:
            perf.times_met += 1
        perf.success_rate = perf.times_met / perf.total_attempts
        perf.last_updated = datetime.utcnow()

    # Update category performance
    category = outcome.work_category
    if category not in history.category_performance:
        history.category_performance[category] = CategoryPerformance(
            category=category,
            contracts_completed=0,
            success_rate=0.0,
            avg_bonus_rate=0.0,
            avg_penalty_rate=0.0,
            last_contract=datetime.utcnow()
        )

    cat_perf = history.category_performance[category]
    cat_perf.contracts_completed += 1
    cat_perf.last_contract = datetime.utcnow()
    # Recalculate running averages...

    # Update prediction accuracy
    if outcome.provider_confidence:
        predicted = outcome.provider_confidence
        actual = 1.0 if outcome.actual_success else 0.0
        # Update calibration score
        history.prediction_accuracy = update_calibration(
            history.prediction_accuracy,
            predicted,
            actual,
            history.total_cpa_contracts
        )

    await firestore.update_provider(provider)

    # Queue ML feature update
    await pubsub.publish("provider.outcome_recorded", {
        "provider_id": provider_id,
        "contract_id": outcome.contract_id,
        "category": category,
        "success": outcome.actual_success
    })
```

### ML Feature Computation

```python
async def compute_ml_features(provider_id: str) -> MLProviderFeatures:
    """Compute ML features for provider."""
    provider = await firestore.get_provider(provider_id)

    # Get historical data
    contracts = await firestore.get_provider_contracts(
        provider_id,
        limit=1000,
        order_by="completed_at"
    )
    bids = await firestore.get_provider_bids(
        provider_id,
        limit=1000,
        order_by="submitted_at"
    )

    if not contracts:
        return None

    # Compute behavioral features
    avg_bid_price = sum(b.price for b in bids) / len(bids) if bids else 0
    bid_win_rate = len(contracts) / len(bids) if bids else 0
    avg_confidence = sum(b.confidence for b in bids) / len(bids) if bids else 0

    # Compute performance features
    successful = [c for c in contracts if c.status == "COMPLETED"]
    completion_rate = len(successful) / len(contracts) if contracts else 0

    # Compute stability features
    first_contract = min(c.completed_at for c in contracts)
    active_days = (datetime.utcnow() - first_contract).days
    contracts_per_day = len(contracts) / max(active_days, 1)

    # Compute risk features
    disputed = [c for c in contracts if c.status == "DISPUTED"]
    dispute_rate = len(disputed) / len(contracts) if contracts else 0

    penalized = [c for c in contracts if c.settlement_breakdown and c.settlement_breakdown.penalty_applied > 0]
    penalty_frequency = len(penalized) / len(contracts) if contracts else 0

    recent_failures = len([
        c for c in contracts
        if c.status == "FAILED" and
        c.failed_at and
        (datetime.utcnow() - c.failed_at).days <= 30
    ])

    # Confidence calibration
    calibration = compute_confidence_calibration(bids, contracts)

    features = MLProviderFeatures(
        avg_bid_price=avg_bid_price,
        bid_win_rate=bid_win_rate,
        avg_confidence=avg_confidence,
        confidence_calibration=calibration,
        completion_rate=completion_rate,
        avg_latency_vs_sla=compute_latency_ratio(contracts),
        criteria_meet_rate=compute_criteria_rate(contracts),
        active_days=active_days,
        contracts_per_day=contracts_per_day,
        consistency_score=compute_consistency(contracts),
        dispute_rate=dispute_rate,
        penalty_frequency=penalty_frequency,
        recent_failures=recent_failures,
        computed_at=datetime.utcnow(),
        model_version="v2.1.0"
    )

    # Store and publish
    provider.ml_features = features
    await firestore.update_provider(provider)

    await pubsub.publish("provider.ml_features_updated", {
        "provider_id": provider_id,
        "features_hash": hash_features(features),
        "model_version": features.model_version
    })

    return features
```

## Events

### Published Events (Enhanced)

```json
// Provider registered (enhanced)
{
  "event_type": "provider.registered",
  "provider_id": "prov_abc123",
  "name": "Expedia Travel Agent",
  "cpa_enabled": true,
  "capabilities": ["travel.booking", "travel.search"],
  "timestamp": "2025-01-15T10:00:00Z"
}

// NEW: Outcome recorded
{
  "event_type": "provider.outcome_recorded",
  "provider_id": "prov_abc123",
  "contract_id": "contract_789xyz",
  "category": "travel.booking",
  "success": true,
  "bonus_earned": 0.07,
  "penalty_applied": 0.0,
  "timestamp": "2025-01-15T10:35:00Z"
}

// NEW: ML features updated
{
  "event_type": "provider.ml_features_updated",
  "provider_id": "prov_abc123",
  "features_hash": "sha256:abc123",
  "model_version": "v2.1.0",
  "timestamp": "2025-01-15T11:00:00Z"
}

// NEW: CPA certification status
{
  "event_type": "provider.cpa_certified",
  "provider_id": "prov_abc123",
  "certified": true,
  "expires_at": "2026-01-15T00:00:00Z",
  "timestamp": "2025-01-15T10:30:00Z"
}
```

### Consumed Events

```json
// Contract completed (update outcome history)
{
  "event_type": "contract.completed",
  "contract_id": "contract_789xyz",
  "provider_id": "prov_abc123",
  "cpa_details": {...}
}

// Trust score updated
{
  "event_type": "trust.score_updated",
  "provider_id": "prov_abc123",
  "new_score": 0.89
}

// Trust tier changed
{
  "event_type": "trust.tier_changed",
  "provider_id": "prov_abc123",
  "new_tier": "PREFERRED"
}
```

## Configuration

```yaml
# config/provider-registry.yaml (Phase B additions)
provider_registry:
  # Phase A config unchanged...

  # Phase B additions
  cpa:
    enabled: true
    certification_required: true
    certification_validity_days: 365
    max_penalty_cap: 0.50

  outcome_tracking:
    enabled: true
    history_retention_days: 365
    window_sizes: [30, 90, 365]

  ml_features:
    enabled: true
    computation_interval_hours: 6
    min_contracts_for_features: 10
    trust_scoring_url: ${TRUST_SCORING_URL}

  subscriptions:
    cpa_filter_enabled: true
    default_cpa_preference: false
```

## Metrics

```python
# Phase B metrics
provider_cpa_registrations = Counter(
    "provider_cpa_registrations_total",
    "Providers registered with CPA capability"
)

provider_outcome_recorded = Counter(
    "provider_outcome_recorded_total",
    "Outcomes recorded",
    ["category", "success"]
)

provider_bonus_earned = Histogram(
    "provider_bonus_earned",
    "Bonuses earned per contract",
    buckets=[0.01, 0.02, 0.05, 0.10, 0.20, 0.50]
)

provider_penalty_applied = Histogram(
    "provider_penalty_applied",
    "Penalties applied per contract",
    buckets=[0.01, 0.02, 0.05, 0.10, 0.20]
)

ml_features_computation_duration = Histogram(
    "ml_features_computation_seconds",
    "Time to compute ML features",
    buckets=[0.1, 0.5, 1, 2, 5, 10]
)

provider_cpa_certification = Gauge(
    "provider_cpa_certified",
    "Number of CPA-certified providers"
)
```

## Backward Compatibility

1. **Non-CPA Providers**: All CPA fields optional, default to disabled
2. **Existing Providers**: Can enable CPA via profile update
3. **Subscription Filters**: CPA filters additive, don't break existing
4. **Internal APIs**: New endpoints, existing ones unchanged
5. **Events**: New fields added, consumers ignore unknown
