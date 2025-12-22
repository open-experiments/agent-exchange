# Enhanced Contract Engine Specification (Phase B)

## Overview

Phase B enhances `aex-contract-engine` to support CPA-based contracts with outcome-based pricing, success criteria tracking, and integration with the Outcome Oracle for verification.

## Changes from Phase A

| Aspect | Phase A | Phase B |
|--------|---------|---------|
| Contract Structure | Fixed price | + CPA terms, success criteria |
| Completion | Provider reports metrics | + Outcome verification |
| Settlement Trigger | On completion | After outcome evaluation |
| Tracking | Execution updates | + Criteria progress |
| Disputes | Basic failure handling | + Outcome-based disputes |

## Architecture

```
       Bid Evaluator        Consumer/Provider
            │                     │
            ▼                     ▼
   ┌─────────────────────────────────────┐
   │    aex-contract-engine (Enhanced)   │◄── THIS SERVICE
   │                                     │
   │ • Award contracts with CPA terms    │
   │ • Track criteria progress           │
   │ • Trigger outcome verification      │
   │ • Handle CPA settlement             │
   └──────────────┬──────────────────────┘
                  │
        ┌─────────┼─────────────┬─────────────┐
        ▼         ▼             ▼             ▼
   Firestore  Settlement   Trust Broker  Outcome Oracle
  (contracts)  (CPA pay)   (reputation)  (verification)
```

## Enhanced Data Models

```python
class Contract(BaseModel):
    # Phase A fields
    id: str
    work_id: str
    consumer_id: str
    provider_id: str
    agent_id: str
    bid_id: str
    agreed_price: float
    sla: SLACommitment
    provider_endpoint: str
    execution_token: str
    consumer_token: str
    status: ContractStatus
    expires_at: datetime
    awarded_at: datetime
    started_at: datetime | None
    completed_at: datetime | None
    failed_at: datetime | None
    execution_updates: list[ExecutionUpdate]
    outcome: OutcomeReport | None

    # NEW: Phase B CPA fields
    cpa_enabled: bool = False
    cpa_terms: CPAContractTerms | None = None
    criteria_tracking: list[CriteriaProgress] = []
    outcome_verification: OutcomeVerification | None = None
    settlement_breakdown: SettlementBreakdown | None = None

class CPAContractTerms(BaseModel):
    """CPA terms agreed in the contract."""
    success_criteria: list[ContractCriterion]
    max_bonus: float
    max_penalty_rate: float
    provider_guarantees: list[CriteriaGuarantee]
    verification_required: bool = True

class ContractCriterion(BaseModel):
    metric: str
    target_value: Any
    comparison: str  # gte, lte, eq, gt, lt
    bonus: float | None
    required: bool
    verification_type: str  # self_reported, oracle_verified, third_party

class CriteriaProgress(BaseModel):
    """Track progress toward each criterion during execution."""
    metric: str
    current_value: Any | None
    status: str  # pending, in_progress, met, not_met, verification_pending
    updated_at: datetime
    evidence: list[EvidenceItem] = []

class EvidenceItem(BaseModel):
    type: str  # confirmation_number, receipt, screenshot, api_response
    value: str
    timestamp: datetime
    verified: bool = False

class OutcomeVerification(BaseModel):
    """Results from Outcome Oracle."""
    verification_id: str
    status: str  # pending, verified, failed, disputed
    criteria_results: list[CriteriaResult]
    verified_at: datetime | None
    confidence: float
    verifier: str  # oracle, self, third_party

class CriteriaResult(BaseModel):
    metric: str
    reported_value: Any
    verified_value: Any | None
    met: bool
    confidence: float
    evidence_verified: bool
    bonus_eligible: bool
    notes: str | None

class SettlementBreakdown(BaseModel):
    """CPA settlement details."""
    base_price: float
    criteria_bonuses: list[CriteriaBonus]
    total_bonus: float
    penalty_applied: float
    penalty_reason: str | None
    final_amount: float
    consumer_pays: float
    provider_receives: float
    platform_fee: float

class CriteriaBonus(BaseModel):
    metric: str
    met: bool
    bonus_amount: float
    verification_status: str

class ContractStatus(str, Enum):
    AWARDED = "AWARDED"
    EXECUTING = "EXECUTING"
    COMPLETED = "COMPLETED"
    VERIFICATION_PENDING = "VERIFICATION_PENDING"  # NEW
    VERIFIED = "VERIFIED"                          # NEW
    SETTLEMENT_PENDING = "SETTLEMENT_PENDING"      # NEW
    SETTLED = "SETTLED"                            # NEW
    FAILED = "FAILED"
    EXPIRED = "EXPIRED"
    DISPUTED = "DISPUTED"
```

## Enhanced API Endpoints

### Award Contract (Enhanced)

#### POST /v1/work/{work_id}/award

```json
// Request
{
  "bid_id": "bid_def456",
  "auto_award": false
}

// Response (Enhanced)
{
  "contract_id": "contract_789xyz",
  "work_id": "work_550e8400",
  "provider_id": "prov_booking",
  "agent_id": "agent_xyz789",
  "agreed_price": 0.08,
  "status": "AWARDED",
  "provider_endpoint": "https://agent.booking.com/a2a/v1",
  "execution_token": "exec_token_abc...",
  "expires_at": "2025-01-15T11:30:00Z",
  "awarded_at": "2025-01-15T10:31:00Z",
  "cpa_enabled": true,                              // NEW
  "cpa_terms": {                                    // NEW
    "success_criteria": [
      {
        "metric": "booking_confirmed",
        "target_value": true,
        "comparison": "eq",
        "bonus": 0.05,
        "required": true,
        "verification_type": "oracle_verified"
      },
      {
        "metric": "response_time_ms",
        "target_value": 3000,
        "comparison": "lte",
        "bonus": 0.02,
        "required": false,
        "verification_type": "self_reported"
      }
    ],
    "max_bonus": 0.07,
    "max_penalty_rate": 0.20,
    "verification_required": true
  },
  "expected_payout": {                              // NEW
    "min": 0.064,                                   // base - max penalty
    "base": 0.08,
    "max": 0.15                                     // base + max bonus
  }
}
```

### Report Criteria Progress (Provider)

#### POST /v1/contracts/{contract_id}/criteria-progress

Provider reports progress toward success criteria during execution.

```json
// Headers
Authorization: Bearer {execution_token}

// Request
{
  "updates": [
    {
      "metric": "booking_confirmed",
      "current_value": null,
      "status": "in_progress",
      "message": "Searching for available flights"
    },
    {
      "metric": "response_time_ms",
      "current_value": 1200,
      "status": "in_progress"
    }
  ]
}

// Response
{
  "contract_id": "contract_789xyz",
  "acknowledged": true,
  "criteria_tracking": [
    {"metric": "booking_confirmed", "status": "in_progress"},
    {"metric": "response_time_ms", "status": "in_progress", "on_track": true}
  ]
}
```

### Report Completion with Evidence (Enhanced)

#### POST /v1/contracts/{contract_id}/complete

```json
// Headers
Authorization: Bearer {execution_token}

// Request (Enhanced)
{
  "success": true,
  "result_summary": "Flight booked successfully",
  "metrics": {
    "booking_confirmed": true,
    "response_time_ms": 2300,
    "total_price": 598.00,
    "options_found": 23
  },
  "evidence": {                                     // NEW
    "booking_confirmed": [
      {
        "type": "confirmation_number",
        "value": "ABC123XYZ",
        "timestamp": "2025-01-15T10:31:55Z"
      },
      {
        "type": "receipt",
        "value": "https://storage.example.com/receipts/abc123.pdf",
        "timestamp": "2025-01-15T10:31:56Z"
      }
    ]
  },
  "result_location": "https://agent.booking.com/results/abc123"
}

// Response (Enhanced)
{
  "contract_id": "contract_789xyz",
  "status": "VERIFICATION_PENDING",                 // NEW: Not completed until verified
  "verification_initiated": true,                   // NEW
  "verification_id": "ver_abc123",                  // NEW
  "criteria_preliminary": [                         // NEW
    {
      "metric": "booking_confirmed",
      "reported_value": true,
      "verification_status": "pending"
    },
    {
      "metric": "response_time_ms",
      "reported_value": 2300,
      "verification_status": "auto_verified",
      "met": true
    }
  ],
  "completed_at": "2025-01-15T10:32:00Z"
}
```

### Get Verification Status

#### GET /v1/contracts/{contract_id}/verification

```json
{
  "contract_id": "contract_789xyz",
  "verification_id": "ver_abc123",
  "status": "verified",
  "criteria_results": [
    {
      "metric": "booking_confirmed",
      "reported_value": true,
      "verified_value": true,
      "met": true,
      "confidence": 0.98,
      "bonus_eligible": true
    },
    {
      "metric": "response_time_ms",
      "reported_value": 2300,
      "verified_value": 2300,
      "met": true,
      "confidence": 1.0,
      "bonus_eligible": true
    }
  ],
  "overall_success": true,
  "verified_at": "2025-01-15T10:32:30Z"
}
```

### Dispute Outcome

#### POST /v1/contracts/{contract_id}/dispute

Consumer or provider can dispute outcome verification.

```json
// Request
{
  "disputed_by": "consumer",
  "disputed_criteria": ["booking_confirmed"],
  "reason": "Booking was not actually confirmed",
  "evidence": [
    {
      "type": "screenshot",
      "value": "https://storage.example.com/evidence/dispute123.png"
    }
  ]
}

// Response
{
  "contract_id": "contract_789xyz",
  "dispute_id": "disp_abc123",
  "status": "DISPUTED",
  "under_review": true,
  "estimated_resolution": "2025-01-16T10:00:00Z"
}
```

## Core Implementation

### Enhanced Contract Award

```python
async def award_contract(work_id: str, bid_id: str, auto: bool = False) -> Contract:
    # Phase A: Get work, evaluation, determine winner
    work = await work_publisher.get_work(work_id)
    evaluation = await bid_evaluator.get_evaluation(work_id)

    if auto:
        if not evaluation.ranked_bids:
            raise HTTPException(400, "No valid bids to award")
        winning_bid = evaluation.ranked_bids[0]
        bid_id = winning_bid.bid_id
    else:
        winning_bid = next(
            (b for b in evaluation.ranked_bids if b.bid_id == bid_id),
            None
        )
        if not winning_bid:
            raise HTTPException(400, "Invalid bid ID")

    bid = await bid_gateway.get_bid(bid_id)

    # Phase A: Create base contract
    contract = Contract(
        id=generate_contract_id(),
        work_id=work_id,
        consumer_id=work.consumer_id,
        provider_id=bid.provider_id,
        agent_id=bid.agent_id,
        bid_id=bid_id,
        agreed_price=bid.price,
        sla=bid.sla,
        provider_endpoint=bid.a2a_endpoint,
        execution_token=generate_execution_token(),
        consumer_token=generate_consumer_token(),
        status=ContractStatus.AWARDED,
        expires_at=datetime.utcnow() + timedelta(hours=1),
        awarded_at=datetime.utcnow()
    )

    # NEW Phase B: Add CPA terms if enabled
    if work.cpa_enabled and bid.cpa_acceptance:
        contract.cpa_enabled = True
        contract.cpa_terms = build_cpa_contract_terms(work, bid)
        contract.criteria_tracking = initialize_criteria_tracking(work.success_criteria)

    # Persist and publish
    await firestore.save_contract(contract)
    await work_publisher.update_work_status(work_id, "AWARDED", contract.id)
    await notify_provider_awarded(contract, bid)

    # Enhanced event with CPA info
    await pubsub.publish("contract.awarded", {
        "contract_id": contract.id,
        "work_id": work_id,
        "provider_id": contract.provider_id,
        "agent_id": contract.agent_id,
        "consumer_id": contract.consumer_id,
        "agreed_price": contract.agreed_price,
        "cpa_enabled": contract.cpa_enabled,
        "cpa_terms": contract.cpa_terms.dict() if contract.cpa_terms else None,
        "provider_endpoint": contract.provider_endpoint
    })

    return contract


def build_cpa_contract_terms(work: WorkSpec, bid: BidPacket) -> CPAContractTerms:
    """Build CPA contract terms from work spec and bid guarantees."""
    criteria = []
    for wc in work.success_criteria:
        # Find matching guarantee from bid
        guarantee = next(
            (g for g in bid.cpa_acceptance.criteria_guarantees if g.metric == wc.metric),
            None
        )

        criteria.append(ContractCriterion(
            metric=wc.metric,
            target_value=wc.threshold,
            comparison=wc.comparison,
            bonus=wc.bonus,
            required=wc.required,
            verification_type=determine_verification_type(wc, guarantee)
        ))

    return CPAContractTerms(
        success_criteria=criteria,
        max_bonus=sum(c.bonus or 0 for c in criteria),
        max_penalty_rate=min(
            bid.cpa_acceptance.max_penalty_accepted,
            work.cpa_terms.max_penalty_rate
        ),
        provider_guarantees=bid.cpa_acceptance.criteria_guarantees,
        verification_required=any(c.verification_type == "oracle_verified" for c in criteria)
    )


def initialize_criteria_tracking(success_criteria: list) -> list[CriteriaProgress]:
    """Initialize tracking for each success criterion."""
    return [
        CriteriaProgress(
            metric=c.metric,
            current_value=None,
            status="pending",
            updated_at=datetime.utcnow()
        )
        for c in success_criteria
    ]
```

### Enhanced Completion with Verification

```python
async def complete_contract(
    contract_id: str,
    outcome: EnhancedOutcomeReport,
    execution_token: str
) -> Contract:
    # Validate
    contract = await firestore.get_contract(contract_id)
    if contract.execution_token != execution_token:
        raise HTTPException(401, "Invalid execution token")

    if contract.status != ContractStatus.EXECUTING:
        raise HTTPException(400, f"Cannot complete contract in {contract.status} status")

    # Update outcome
    contract.outcome = outcome
    contract.completed_at = datetime.utcnow()

    if contract.cpa_enabled and contract.cpa_terms.verification_required:
        # NEW Phase B: Trigger outcome verification
        contract.status = ContractStatus.VERIFICATION_PENDING

        verification = await initiate_verification(contract, outcome)
        contract.outcome_verification = OutcomeVerification(
            verification_id=verification.id,
            status="pending",
            criteria_results=[],
            verified_at=None,
            confidence=0,
            verifier="oracle"
        )

        await firestore.update_contract(contract)

        # Publish verification pending event
        await pubsub.publish("contract.verification_pending", {
            "contract_id": contract.id,
            "work_id": contract.work_id,
            "verification_id": verification.id,
            "criteria_count": len(contract.cpa_terms.success_criteria)
        })
    else:
        # Phase A flow: Direct completion
        contract.status = ContractStatus.COMPLETED
        await firestore.update_contract(contract)
        await trigger_settlement(contract)

    return contract


async def initiate_verification(
    contract: Contract,
    outcome: EnhancedOutcomeReport
) -> VerificationRequest:
    """Request outcome verification from Outcome Oracle."""
    # Prepare verification request
    criteria_to_verify = []
    for criterion in contract.cpa_terms.success_criteria:
        if criterion.verification_type == "oracle_verified":
            criteria_to_verify.append({
                "metric": criterion.metric,
                "reported_value": outcome.metrics.get(criterion.metric),
                "target_value": criterion.target_value,
                "comparison": criterion.comparison,
                "evidence": outcome.evidence.get(criterion.metric, [])
            })
        elif criterion.verification_type == "self_reported":
            # Auto-verify self-reported metrics
            await auto_verify_criterion(contract, criterion, outcome)

    if criteria_to_verify:
        # Call Outcome Oracle
        verification = await outcome_oracle.verify_outcome(
            contract_id=contract.id,
            work_id=contract.work_id,
            provider_id=contract.provider_id,
            criteria=criteria_to_verify
        )
        return verification
    else:
        # All criteria self-reported, create synthetic verification
        return VerificationRequest(
            id=f"self_{contract.id}",
            status="auto_verified"
        )
```

### Handle Verification Result

```python
async def handle_verification_result(verification_event: dict):
    """Process verification result from Outcome Oracle."""
    contract_id = verification_event["contract_id"]
    contract = await firestore.get_contract(contract_id)

    if contract.status != ContractStatus.VERIFICATION_PENDING:
        logger.warning("Unexpected verification for contract", contract_id=contract_id)
        return

    # Update verification results
    contract.outcome_verification = OutcomeVerification(
        verification_id=verification_event["verification_id"],
        status=verification_event["status"],
        criteria_results=[
            CriteriaResult(**r) for r in verification_event["criteria_results"]
        ],
        verified_at=datetime.utcnow(),
        confidence=verification_event["confidence"],
        verifier="oracle"
    )

    # Calculate settlement
    if verification_event["status"] == "verified":
        contract.status = ContractStatus.VERIFIED
        contract.settlement_breakdown = calculate_settlement(contract)
        await firestore.update_contract(contract)
        await trigger_cpa_settlement(contract)
    elif verification_event["status"] == "failed":
        contract.status = ContractStatus.FAILED
        contract.failed_at = datetime.utcnow()
        await firestore.update_contract(contract)
        await handle_verification_failure(contract)
    elif verification_event["status"] == "disputed":
        contract.status = ContractStatus.DISPUTED
        await firestore.update_contract(contract)
        await notify_dispute(contract, verification_event)


def calculate_settlement(contract: Contract) -> SettlementBreakdown:
    """Calculate CPA settlement based on verified outcomes."""
    base_price = contract.agreed_price
    total_bonus = 0.0
    penalty = 0.0
    criteria_bonuses = []

    verification = contract.outcome_verification
    required_criteria_met = True

    for result in verification.criteria_results:
        criterion = next(
            (c for c in contract.cpa_terms.success_criteria if c.metric == result.metric),
            None
        )

        if not criterion:
            continue

        bonus_amount = 0.0
        if result.met and result.bonus_eligible and criterion.bonus:
            bonus_amount = criterion.bonus
            total_bonus += bonus_amount

        criteria_bonuses.append(CriteriaBonus(
            metric=result.metric,
            met=result.met,
            bonus_amount=bonus_amount,
            verification_status=result.verification_status if hasattr(result, 'verification_status') else "verified"
        ))

        if criterion.required and not result.met:
            required_criteria_met = False

    # Apply penalty if required criteria not met
    if not required_criteria_met:
        penalty = base_price * contract.cpa_terms.max_penalty_rate

    # Cap bonus at max
    total_bonus = min(total_bonus, contract.cpa_terms.max_bonus)

    final_amount = base_price + total_bonus - penalty
    platform_fee = final_amount * 0.15  # 15% platform fee

    return SettlementBreakdown(
        base_price=base_price,
        criteria_bonuses=criteria_bonuses,
        total_bonus=total_bonus,
        penalty_applied=penalty,
        penalty_reason="Required criteria not met" if penalty > 0 else None,
        final_amount=final_amount,
        consumer_pays=final_amount,
        provider_receives=final_amount - platform_fee,
        platform_fee=platform_fee
    )


async def trigger_cpa_settlement(contract: Contract):
    """Trigger settlement with CPA breakdown."""
    settlement = contract.settlement_breakdown

    await pubsub.publish("contract.completed", {
        "contract_id": contract.id,
        "work_id": contract.work_id,
        "agent_id": contract.agent_id,
        "provider_id": contract.provider_id,
        "consumer_id": contract.consumer_id,
        "domain": contract.domain,
        "started_at": contract.started_at.isoformat(),
        "completed_at": contract.completed_at.isoformat(),
        "duration_ms": (contract.completed_at - contract.started_at).total_seconds() * 1000,
        "billing": {
            "base_price": settlement.base_price,
            "cpa_bonus": settlement.total_bonus,
            "cpa_penalty": settlement.penalty_applied,
            "final_amount": settlement.final_amount
        },
        "metrics": contract.outcome.metrics,
        "verification": {
            "verification_id": contract.outcome_verification.verification_id,
            "status": contract.outcome_verification.status,
            "confidence": contract.outcome_verification.confidence
        },
        "cpa_details": {
            "criteria_results": [
                {
                    "metric": b.metric,
                    "met": b.met,
                    "bonus": b.bonus_amount
                }
                for b in settlement.criteria_bonuses
            ]
        }
    })

    contract.status = ContractStatus.SETTLEMENT_PENDING
    await firestore.update_contract(contract)
```

## Events

### Published Events (Enhanced)

```json
// Contract awarded (enhanced)
{
  "event_type": "contract.awarded",
  "contract_id": "contract_789xyz",
  "work_id": "work_550e8400",
  "provider_id": "prov_booking",
  "agent_id": "agent_xyz789",
  "consumer_id": "tenant_123",
  "agreed_price": 0.08,
  "cpa_enabled": true,
  "cpa_terms": {
    "success_criteria": [...],
    "max_bonus": 0.07,
    "max_penalty_rate": 0.20
  },
  "provider_endpoint": "https://agent.booking.com/a2a/v1"
}

// NEW: Verification pending
{
  "event_type": "contract.verification_pending",
  "contract_id": "contract_789xyz",
  "work_id": "work_550e8400",
  "verification_id": "ver_abc123",
  "criteria_count": 3
}

// Contract completed (enhanced with CPA)
{
  "event_type": "contract.completed",
  "contract_id": "contract_789xyz",
  "work_id": "work_550e8400",
  "agent_id": "agent_xyz789",
  "provider_id": "prov_abc123",
  "consumer_id": "tenant_123",
  "domain": "travel.booking",
  "started_at": "2025-01-15T10:30:00Z",
  "completed_at": "2025-01-15T10:30:02Z",
  "duration_ms": 2000,
  "billing": {
    "base_price": 0.08,
    "cpa_bonus": 0.05,
    "cpa_penalty": 0.00,
    "final_amount": 0.13
  },
  "metrics": {
    "booking_confirmed": true,
    "response_time_ms": 2300
  },
  "verification": {
    "verification_id": "ver_abc123",
    "status": "verified",
    "confidence": 0.98
  },
  "cpa_details": {
    "criteria_results": [
      {"metric": "booking_confirmed", "met": true, "bonus": 0.05},
      {"metric": "response_time_ms", "met": true, "bonus": 0.02}
    ]
  }
}

// Contract failed
{
  "event_type": "contract.failed",
  "contract_id": "contract_789xyz",
  "work_id": "work_550e8400",
  "agent_id": "agent_xyz789",
  "provider_id": "prov_abc123",
  "reason": "verification_failed",
  "error_code": "CRITERIA_NOT_MET",
  "failed_criteria": ["booking_confirmed"]
}
```

### Consumed Events

```json
// Bids evaluated (for auto-award)
{
  "event_type": "bids.evaluated",
  "work_id": "work_550e8400",
  "winning_bid_id": "bid_def456"
}

// NEW: Outcome verified (from Outcome Oracle)
{
  "event_type": "outcome.verified",
  "contract_id": "contract_789xyz",
  "verification_id": "ver_abc123",
  "status": "verified",
  "criteria_results": [...],
  "confidence": 0.98
}

// Contract settled (from Settlement)
{
  "event_type": "contract.settled",
  "contract_id": "contract_789xyz",
  "settlement_status": "completed"
}
```

## Integration with Outcome Oracle

```python
class OutcomeOracleClient:
    def __init__(self, base_url: str, service_token: str):
        self.base_url = base_url
        self.service_token = service_token

    async def verify_outcome(
        self,
        contract_id: str,
        work_id: str,
        provider_id: str,
        criteria: list[dict]
    ) -> VerificationRequest:
        """Request outcome verification from Oracle."""
        response = await self.http.post(
            f"{self.base_url}/internal/v1/verify",
            headers={"Authorization": f"Bearer {self.service_token}"},
            json={
                "contract_id": contract_id,
                "work_id": work_id,
                "provider_id": provider_id,
                "criteria": criteria
            }
        )

        data = response.json()
        return VerificationRequest(
            id=data["verification_id"],
            status=data["status"],
            estimated_completion=data.get("estimated_completion")
        )

    async def get_verification_status(
        self,
        verification_id: str
    ) -> VerificationStatus:
        """Check verification status."""
        response = await self.http.get(
            f"{self.base_url}/internal/v1/verifications/{verification_id}",
            headers={"Authorization": f"Bearer {self.service_token}"}
        )
        return VerificationStatus(**response.json())
```

## Configuration

```yaml
# config/contract-engine.yaml (Phase B additions)
contract_engine:
  # Phase A config unchanged...

  # Phase B additions
  cpa:
    enabled: true
    verification_timeout_ms: 30000
    auto_verify_self_reported: true

  verification:
    oracle_url: ${OUTCOME_ORACLE_URL}
    retry_attempts: 3
    retry_delay_ms: 1000

  settlement:
    delay_after_verification_ms: 5000
    dispute_window_hours: 24

  penalties:
    apply_on_required_failure: true
    apply_on_verification_failure: false
```

## Metrics

```python
# Phase B metrics
contract_cpa_enabled = Counter(
    "contract_cpa_enabled_total",
    "Contracts with CPA enabled"
)

contract_verification_duration = Histogram(
    "contract_verification_duration_seconds",
    "Time to complete outcome verification",
    buckets=[0.5, 1, 2, 5, 10, 30, 60]
)

contract_criteria_met = Counter(
    "contract_criteria_met_total",
    "Criteria outcomes",
    ["metric", "met"]
)

contract_bonus_paid = Histogram(
    "contract_bonus_paid",
    "CPA bonuses paid",
    buckets=[0.01, 0.02, 0.05, 0.10, 0.20, 0.50]
)

contract_penalty_applied = Counter(
    "contract_penalty_applied_total",
    "Contracts with penalty applied",
    ["reason"]
)

contract_disputes = Counter(
    "contract_disputes_total",
    "Contract disputes",
    ["disputed_by", "reason"]
)
```

## Backward Compatibility

1. **Non-CPA Contracts**: Flow unchanged, `cpa_enabled=false`
2. **Non-CPA Bids**: Standard completion without verification
3. **Verification Timeout**: Falls back to self-reported metrics
4. **Event Compatibility**: New fields added, old consumers ignore unknown
5. **Settlement**: Works with both CPA and non-CPA billing structures
