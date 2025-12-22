# Enhanced Work Publisher Specification (Phase B)

## Overview

Phase B enhances `aex-work-publisher` to support CPA pricing by accepting success criteria, CPA bonus terms, and outcome requirements in work submissions.

## Changes from Phase A

| Aspect | Phase A | Phase B |
|--------|---------|---------|
| Work Spec | Basic payload + budget | + Success criteria, CPA terms |
| Validation | Structure only | + Criteria validation, metric support |
| Budget | max_price only | + max_cpa_bonus, cpa_terms |
| Broadcast | Basic opportunity | + CPA terms for provider decision |
| Events | work.submitted | + cpa_enabled flag |

## Enhanced API

### POST /v1/work (Enhanced)

```json
// Request (Phase B additions in bold comments)
{
  "category": "travel.booking",
  "description": "Book round-trip flight LAX→JFK, March 15-22, 2 adults",
  "constraints": {
    "max_latency_ms": 5000,
    "required_fields": ["confirmation_number", "total_price", "itinerary"],
    "min_trust_tier": "VERIFIED"  // NEW: Minimum provider trust
  },
  "budget": {
    "max_price": 0.15,
    "bid_strategy": "balanced",
    "max_cpa_bonus": 0.10,         // NEW: Maximum CPA bonus pool
    "accept_cpa_bids": true        // NEW: Accept CPA-enabled bids
  },
  "success_criteria": [            // NEW: Outcome requirements
    {
      "metric": "booking_confirmed",
      "metric_type": "boolean",
      "comparison": "eq",
      "threshold": true,
      "required": true,
      "bonus": 0.05,
      "description": "Flight booking must be confirmed"
    },
    {
      "metric": "response_time_ms",
      "metric_type": "latency",
      "comparison": "lte",
      "threshold": 3000,
      "required": false,
      "bonus": 0.02,
      "description": "Response under 3 seconds"
    },
    {
      "metric": "price_accuracy",
      "metric_type": "percentage",
      "comparison": "gte",
      "threshold": 0.95,
      "required": false,
      "bonus": 0.03,
      "penalty": 0.02,
      "description": "Final price within 5% of quoted"
    }
  ],
  "cpa_terms": {                   // NEW: CPA contract terms
    "verification_method": "automated",
    "dispute_window_hours": 24,
    "evidence_required": ["confirmation_number", "receipt"],
    "penalty_on_failure": true,
    "max_penalty_rate": 0.20
  },
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
  "cpa_enabled": true,
  "max_potential_cost": 0.25,     // max_price + max_cpa_bonus
  "success_criteria_count": 3,
  "created_at": "2025-01-15T10:30:00Z"
}
```

### GET /v1/work/{work_id} (Enhanced Response)

```json
{
  "work_id": "work_550e8400",
  "category": "travel.booking",
  "status": "EVALUATING",
  "bids_received": 5,
  "cpa_bids_received": 3,          // NEW: CPA-enabled bids
  "bid_window_ends_at": "2025-01-15T10:30:30Z",
  "success_criteria": [...],       // NEW: Full criteria
  "cpa_terms": {...},              // NEW: CPA terms
  "budget": {
    "max_price": 0.15,
    "max_cpa_bonus": 0.10,
    "max_potential_cost": 0.25
  },
  "contract": null,
  "created_at": "2025-01-15T10:30:00Z"
}
```

## Enhanced Data Models

```python
from pydantic import BaseModel, validator
from typing import Optional
from enum import Enum

class MetricType(str, Enum):
    BOOLEAN = "boolean"
    NUMERIC = "numeric"
    PERCENTAGE = "percentage"
    LATENCY = "latency"
    COUNT = "count"
    ACCURACY = "accuracy"
    CUSTOM = "custom"

class ComparisonOperator(str, Enum):
    EQ = "eq"
    NEQ = "neq"
    GT = "gt"
    GTE = "gte"
    LT = "lt"
    LTE = "lte"
    IN_RANGE = "in_range"

class SuccessCriterion(BaseModel):
    metric: str
    metric_type: MetricType
    comparison: ComparisonOperator
    threshold: float | bool | dict
    required: bool = True
    bonus: Optional[float] = None
    penalty: Optional[float] = None
    weight: float = 1.0
    description: Optional[str] = None

    @validator('bonus', 'penalty')
    def validate_incentives(cls, v):
        if v is not None and v < 0:
            raise ValueError("Incentives must be non-negative")
        return v

class CPATerms(BaseModel):
    verification_method: str = "automated"  # automated, consumer_confirm, evidence
    dispute_window_hours: int = 24
    evidence_required: list[str] = []
    penalty_on_failure: bool = False
    max_penalty_rate: float = 0.20

    @validator('max_penalty_rate')
    def validate_penalty_rate(cls, v):
        if v < 0 or v > 0.5:
            raise ValueError("Penalty rate must be 0-50%")
        return v

class EnhancedBudget(BaseModel):
    max_price: float
    bid_strategy: str = "balanced"
    max_cpa_bonus: Optional[float] = None
    accept_cpa_bids: bool = True

    @property
    def max_potential_cost(self) -> float:
        return self.max_price + (self.max_cpa_bonus or 0)

class EnhancedWorkSpec(BaseModel):
    id: str
    consumer_id: str
    category: str
    description: str
    constraints: WorkConstraints
    budget: EnhancedBudget
    success_criteria: list[SuccessCriterion] = []
    cpa_terms: Optional[CPATerms] = None
    bid_window_ms: int
    payload: dict

    # State
    state: WorkState
    providers_notified: int
    bids_received: int
    cpa_bids_received: int = 0
    contract_id: Optional[str] = None

    # Timestamps
    created_at: datetime
    bid_window_ends_at: datetime

    @property
    def cpa_enabled(self) -> bool:
        return len(self.success_criteria) > 0 and self.budget.accept_cpa_bids

    @property
    def max_potential_bonus(self) -> float:
        return sum(c.bonus or 0 for c in self.success_criteria)
```

## Enhanced Validation

```python
class WorkValidator:
    SUPPORTED_METRICS = {
        "latency_ms", "response_time_ms", "processing_time",
        "accuracy", "precision", "recall", "f1_score",
        "booking_confirmed", "task_completed", "has_output",
        "price_accuracy", "output_length", "word_count",
        "custom"
    }

    MAX_CRITERIA_PER_WORK = 10
    MAX_BONUS_MULTIPLIER = 3.0  # Max bonus = 3x base price

    async def validate(self, work: EnhancedWorkSpec) -> list[str]:
        errors = []

        # Validate success criteria
        if work.success_criteria:
            errors.extend(self._validate_criteria(work))

        # Validate CPA terms
        if work.cpa_terms:
            errors.extend(self._validate_cpa_terms(work))

        # Validate budget constraints
        errors.extend(self._validate_budget(work))

        return errors

    def _validate_criteria(self, work: EnhancedWorkSpec) -> list[str]:
        errors = []

        if len(work.success_criteria) > self.MAX_CRITERIA_PER_WORK:
            errors.append(f"Maximum {self.MAX_CRITERIA_PER_WORK} criteria allowed")

        total_bonus = 0
        for criterion in work.success_criteria:
            # Check metric is supported
            if criterion.metric not in self.SUPPORTED_METRICS:
                if criterion.metric_type != MetricType.CUSTOM:
                    errors.append(f"Unsupported metric: {criterion.metric}")

            # Validate threshold type matches metric type
            if criterion.metric_type == MetricType.BOOLEAN:
                if not isinstance(criterion.threshold, bool):
                    errors.append(f"Boolean metric {criterion.metric} requires bool threshold")

            if criterion.metric_type == MetricType.PERCENTAGE:
                if not (0 <= criterion.threshold <= 1):
                    errors.append(f"Percentage metric {criterion.metric} threshold must be 0-1")

            if criterion.bonus:
                total_bonus += criterion.bonus

        # Check total bonus doesn't exceed cap
        if work.budget.max_cpa_bonus and total_bonus > work.budget.max_cpa_bonus:
            errors.append(f"Total bonus ({total_bonus}) exceeds max_cpa_bonus ({work.budget.max_cpa_bonus})")

        return errors

    def _validate_cpa_terms(self, work: EnhancedWorkSpec) -> list[str]:
        errors = []

        if work.cpa_terms.verification_method not in ["automated", "consumer_confirm", "evidence"]:
            errors.append(f"Invalid verification method: {work.cpa_terms.verification_method}")

        if work.cpa_terms.dispute_window_hours < 1:
            errors.append("Dispute window must be at least 1 hour")

        if work.cpa_terms.dispute_window_hours > 168:  # 1 week
            errors.append("Dispute window cannot exceed 168 hours")

        return errors

    def _validate_budget(self, work: EnhancedWorkSpec) -> list[str]:
        errors = []

        # Check bonus isn't excessive relative to base
        if work.budget.max_cpa_bonus:
            bonus_ratio = work.budget.max_cpa_bonus / work.budget.max_price
            if bonus_ratio > self.MAX_BONUS_MULTIPLIER:
                errors.append(f"CPA bonus cannot exceed {self.MAX_BONUS_MULTIPLIER}x base price")

        return errors
```

## Enhanced Work Publishing

```python
async def publish_work(consumer_id: str, req: WorkSubmission) -> WorkResponse:
    # 1. Validate work spec (Phase B enhanced)
    validator = WorkValidator()
    errors = await validator.validate(req)
    if errors:
        raise HTTPException(400, detail={"errors": errors})

    # 2. Check consumer budget (max_potential_cost)
    max_cost = req.budget.max_potential_cost
    balance = await billing.get_balance(consumer_id)
    if balance < max_cost:
        raise HTTPException(402, f"Insufficient balance. Need {max_cost}, have {balance}")

    # 3. Governance policy check (Phase B)
    if req.success_criteria:
        policy_result = await governance.evaluate_policy(
            policy_type="work_submission",
            context={
                "category": req.category,
                "budget": req.budget.dict(),
                "cpa_terms": req.cpa_terms.dict() if req.cpa_terms else None,
                "criteria_count": len(req.success_criteria),
                "consumer_id": consumer_id
            }
        )
        if not policy_result.allowed:
            raise HTTPException(403, f"Policy violation: {policy_result.reason}")

    # 4. Create enhanced work record
    work = EnhancedWorkSpec(
        id=generate_work_id(),
        consumer_id=consumer_id,
        category=req.category,
        description=req.description,
        constraints=req.constraints,
        budget=req.budget,
        success_criteria=req.success_criteria,
        cpa_terms=req.cpa_terms,
        bid_window_ms=req.bid_window_ms,
        payload=req.payload,
        state=WorkState.OPEN,
        created_at=datetime.utcnow(),
        bid_window_ends_at=datetime.utcnow() + timedelta(milliseconds=req.bid_window_ms)
    )

    # 5. Persist to Firestore
    await firestore.save_work(work)

    # 6. Get subscribed providers (filter by trust tier if required)
    providers = await provider_registry.get_subscribed_providers(
        category=req.category,
        min_trust_tier=req.constraints.min_trust_tier,
        cpa_capable=work.cpa_enabled
    )

    # 7. Broadcast work opportunity (with CPA info)
    await broadcast_work(work, providers)
    work.providers_notified = len(providers)

    # 8. Schedule bid window close
    await schedule_bid_window_close(work.id, work.bid_window_ends_at)

    # 9. Publish event (Phase B enhanced)
    await pubsub.publish("work.submitted", {
        "work_id": work.id,
        "category": work.category,
        "consumer_id": consumer_id,
        "cpa_enabled": work.cpa_enabled,
        "max_price": work.budget.max_price,
        "max_cpa_bonus": work.budget.max_cpa_bonus,
        "criteria_count": len(work.success_criteria),
        "providers_notified": len(providers)
    })

    return WorkResponse(
        work_id=work.id,
        status=work.state,
        bid_window_ends_at=work.bid_window_ends_at,
        providers_notified=len(providers),
        cpa_enabled=work.cpa_enabled,
        max_potential_cost=work.budget.max_potential_cost,
        success_criteria_count=len(work.success_criteria)
    )
```

## Enhanced Broadcast

```python
async def broadcast_work(work: EnhancedWorkSpec, providers: list[Provider]):
    """Broadcast work opportunity with CPA terms."""

    # Build enhanced opportunity message
    opportunity = WorkOpportunity(
        work_id=work.id,
        category=work.category,
        description=work.description,
        constraints=work.constraints,
        budget=BudgetInfo(
            max_price=work.budget.max_price,
            strategy=work.budget.bid_strategy,
            max_cpa_bonus=work.budget.max_cpa_bonus,
            accept_cpa_bids=work.budget.accept_cpa_bids
        ),
        # Phase B: Include success criteria for provider decision
        success_criteria=[
            CriteriaSummary(
                metric=c.metric,
                metric_type=c.metric_type,
                threshold=c.threshold,
                comparison=c.comparison,
                required=c.required,
                bonus=c.bonus,
                penalty=c.penalty
            ) for c in work.success_criteria
        ],
        cpa_terms=CPATermsSummary(
            verification_method=work.cpa_terms.verification_method,
            dispute_window_hours=work.cpa_terms.dispute_window_hours,
            penalty_on_failure=work.cpa_terms.penalty_on_failure
        ) if work.cpa_terms else None,
        bid_deadline=work.bid_window_ends_at,
        payload_preview=truncate_payload(work.payload, max_chars=500)
    )

    # Send to each provider
    for provider in providers:
        if provider.bid_webhook:
            await send_webhook(
                url=provider.bid_webhook,
                payload=opportunity.dict(),
                signature=sign_webhook(opportunity, provider.webhook_secret)
            )
        await pubsub.publish(f"work.opportunity.{provider.id}", opportunity)
```

## Events

### work.submitted (Enhanced)

```python
{
    "event_type": "work.submitted",
    "event_id": "evt_abc123",
    "work_id": "work_550e8400",
    "category": "travel.booking",
    "consumer_id": "tenant_123",
    "cpa_enabled": true,                    # NEW
    "max_price": 0.15,
    "max_cpa_bonus": 0.10,                  # NEW
    "max_potential_cost": 0.25,             # NEW
    "criteria_count": 3,                    # NEW
    "providers_notified": 12,
    "bid_window_ends_at": "2025-01-15T10:30:30Z",
    "timestamp": "2025-01-15T10:30:00Z"
}
```

## Database Schema Changes

```python
# Firestore document (work_specs collection)
{
    "id": "work_550e8400",
    "consumer_id": "tenant_123",
    "category": "travel.booking",
    "description": "...",
    "constraints": {...},
    "budget": {
        "max_price": 0.15,
        "bid_strategy": "balanced",
        "max_cpa_bonus": 0.10,           # NEW
        "accept_cpa_bids": true          # NEW
    },
    "success_criteria": [                 # NEW
        {
            "metric": "booking_confirmed",
            "metric_type": "boolean",
            "comparison": "eq",
            "threshold": true,
            "required": true,
            "bonus": 0.05
        }
    ],
    "cpa_terms": {                        # NEW
        "verification_method": "automated",
        "dispute_window_hours": 24,
        "evidence_required": ["confirmation_number"],
        "penalty_on_failure": true,
        "max_penalty_rate": 0.20
    },
    "state": "OPEN",
    "bids_received": 5,
    "cpa_bids_received": 3,               # NEW
    "created_at": "2025-01-15T10:30:00Z",
    "bid_window_ends_at": "2025-01-15T10:30:30Z"
}
```

## Configuration Changes

```yaml
# config/work-publisher.yaml (Phase B additions)
work_publisher:
  # Phase A config unchanged...

  # Phase B additions
  cpa:
    enabled: true
    max_criteria_per_work: 10
    max_bonus_multiplier: 3.0      # Max bonus = 3x base
    max_penalty_rate: 0.50         # Max penalty = 50% of base
    min_dispute_window_hours: 1
    max_dispute_window_hours: 168

    # Supported metrics
    supported_metrics:
      - latency_ms
      - response_time_ms
      - accuracy
      - booking_confirmed
      - task_completed
      - price_accuracy
      - output_length

  governance:
    policy_check_enabled: true
    policy_check_timeout_ms: 500
```

## Backward Compatibility

1. **Optional Fields**: All CPA fields (`success_criteria`, `cpa_terms`, `max_cpa_bonus`) are optional
2. **Default Behavior**: If no CPA fields, works exactly like Phase A
3. **Event Compatibility**: New fields added to events, consumers ignore unknown fields
4. **API Versioning**: Same `/v1/work` endpoint, enhanced response includes new fields only when CPA enabled

## Migration Notes

- No database migration needed - new fields are optional in Firestore
- Existing work specs continue to work unchanged
- Feature flag `cpa.enabled` can disable CPA features if needed
