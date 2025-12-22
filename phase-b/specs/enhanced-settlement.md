# Enhanced Settlement Specification (Phase B)

## Overview

Phase B enhances the `aex-settlement` service to support CPA (Cost Per Action) pricing. Settlement now calculates costs based on both CPC (base invocation cost) and CPA (outcome-based bonuses/penalties), integrating with the Outcome Verification Framework.

## Changes from Phase A

| Aspect | Phase A | Phase B |
|--------|---------|---------|
| Pricing | CPC only | CPC + CPA |
| Cost Calculation | Fixed per invocation | Base + bonus - penalty |
| Dependencies | - | Outcome Verification, Governance |
| Billing Records | Simple | Detailed breakdown |
| Ledger Entries | Single per contract | Multiple (base, bonus, penalty) |

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    ENHANCED SETTLEMENT                          │
│                                                                 │
│  ┌──────────────┐                                               │
│  │  Pub/Sub     │─── contract.completed►┌────────────────────┐   │
│  │              │    (with metrics)    │                    │   │
│  └──────────────┘                      │  Settlement Engine │   │
│                                        │                    │   │
│  ┌──────────────┐                      │  1. Verify Outcome │   │
│  │  Outcome     │◄─── verify ──────────│  2. Calculate Cost │   │
│  │  Verification│                      │  3. Update Ledger  │   │
│  └──────────────┘                      │  4. Emit Events    │   │
│                                        │                    │   │
│  ┌──────────────┐                      │                    │   │
│  │  Governance  │◄─── validate ────────│                    │   │
│  │              │                      └────────────────────┘   │
│  └──────────────┘                               │               │
│                                                 ▼               │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                    Cloud SQL (PostgreSQL)                │   │
│  │  executions │ ledger_entries │ tenant_balances │ payouts │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## Enhanced Data Models

### Execution Record

```python
class Execution(BaseModel):
    id: str
    work_id: str
    contract_id: str
    agent_id: str
    consumer_id: str
    provider_id: str
    domain: str
    started_at: datetime
    completed_at: datetime
    status: ExecutionStatus

    # Pricing terms from matching
    cpc_price: float
    cpa_terms: Optional[CPATerms]

    # Outcome data
    success: bool
    outcome_metrics: dict[str, float]
    criteria_results: list[CriterionResult]

    # Cost breakdown
    cost_breakdown: CostBreakdown

class CostBreakdown(BaseModel):
    cpc_base: float              # Base invocation cost
    cpa_bonus: float             # Bonus for meeting criteria
    cpa_penalty: float           # Penalty for failure
    gross_total: float           # cpc + bonus - penalty
    platform_fee: float          # Platform's cut
    provider_payout: float       # Provider receives
    requestor_charge: float      # Requestor pays (gross_total)
```

### CPA Terms (from matching)

```python
class CPATerms(BaseModel):
    criteria: list[CPACriterion]
    max_bonus: float
    penalty_rate: float
    predicted_bonus: float

class CPACriterion(BaseModel):
    metric: str
    threshold: float
    comparison: str  # gte, lte, eq, etc.
    bonus: float
```

### Ledger Entry Types

```python
class LedgerEntryType(str, Enum):
    # Consumer entries
    CONTRACT_BASE_CHARGE = "contract_base_charge"       # CPC charge
    CONTRACT_BONUS_CHARGE = "contract_bonus_charge"     # CPA bonus charge
    CONTRACT_PENALTY_CREDIT = "contract_penalty_credit" # Penalty refund
    DEPOSIT = "deposit"
    REFUND = "refund"

    # Provider entries
    CONTRACT_BASE_EARNING = "contract_base_earning"     # CPC earning
    CONTRACT_BONUS_EARNING = "contract_bonus_earning"   # CPA bonus earning
    CONTRACT_PENALTY_DEBIT = "contract_penalty_debit"   # Penalty deduction
    PLATFORM_FEE = "platform_fee"
    WITHDRAWAL = "withdrawal"
```

## Enhanced Settlement Logic

```python
class EnhancedSettlementEngine:
    PLATFORM_FEE_RATE = 0.15  # 15%

    def __init__(
        self,
        db: Database,
        outcome_verifier: OutcomeVerifier,
        governance: GovernanceClient,
        pubsub: PubSubClient
    ):
        self.db = db
        self.verifier = outcome_verifier
        self.governance = governance
        self.pubsub = pubsub

    async def settle_execution(
        self,
        event: ContractCompletedEvent
    ) -> SettlementResult:
        # 1. Get execution and contract details
        execution = await self.db.get_execution(event.execution_id)
        contract = await self.db.get_contract(event.contract_id)

        # 2. Verify outcome (if CPA)
        verification = None
        if execution.cpa_terms:
            verification = await self.verifier.verify(
                contract_id=contract.id,
                work_id=contract.work_id,
                contract_input=contract.payload,
                contract_output=event.result,
                success_criteria=contract.success_criteria,
                execution_metadata={
                    "duration_ms": event.duration_ms,
                    "agent_id": execution.agent_id
                }
            )

            # Governance validation
            gov_result = await self.governance.validate_outcome(
                contract_id=contract.id,
                agent_id=execution.agent_id,
                claimed_metrics=event.metrics,
                execution_context=event.metadata
            )

            if not gov_result.valid:
                # Flag for review, use conservative settlement
                verification.governance_flags = gov_result.flags

        # 3. Calculate cost breakdown
        cost = self._calculate_cost(execution, verification)

        # 4. Update ledger (atomic transaction)
        async with self.db.transaction() as tx:
            # Charge consumer
            await self._charge_consumer(tx, execution, cost)

            # Pay provider
            await self._pay_provider(tx, execution, cost)

            # Record execution with cost
            execution.cost_breakdown = cost
            execution.outcome_metrics = event.metrics
            execution.criteria_results = (
                verification.criterion_results if verification else []
            )
            await tx.update_execution(execution)

        # 5. Emit settlement event
        await self._emit_settlement_event(execution, cost, verification)

        return SettlementResult(
            execution_id=execution.id,
            cost_breakdown=cost,
            verification=verification
        )

    def _calculate_cost(
        self,
        execution: Execution,
        verification: Optional[VerificationResult]
    ) -> CostBreakdown:
        # Base CPC cost (always charged)
        cpc_base = execution.cpc_price

        # CPA calculations
        cpa_bonus = 0.0
        cpa_penalty = 0.0

        if execution.cpa_terms and verification:
            # Calculate bonus for met criteria
            for criterion in execution.cpa_terms.criteria:
                result = next(
                    (r for r in verification.criterion_results
                     if r.metric == criterion.metric),
                    None
                )
                if result and result.met:
                    cpa_bonus += criterion.bonus

            # Cap bonus at max
            cpa_bonus = min(cpa_bonus, execution.cpa_terms.max_bonus)

            # Calculate penalty for failure
            if not verification.success:
                cpa_penalty = cpc_base * execution.cpa_terms.penalty_rate

        # Totals
        gross_total = cpc_base + cpa_bonus - cpa_penalty
        gross_total = max(gross_total, 0)  # Never negative

        platform_fee = gross_total * self.PLATFORM_FEE_RATE
        provider_payout = gross_total - platform_fee

        return CostBreakdown(
            cpc_base=cpc_base,
            cpa_bonus=cpa_bonus,
            cpa_penalty=cpa_penalty,
            gross_total=gross_total,
            platform_fee=platform_fee,
            provider_payout=provider_payout,
            requestor_charge=gross_total
        )

    async def _charge_consumer(
        self,
        tx: Transaction,
        execution: Execution,
        cost: CostBreakdown
    ):
        entries = []

        # Base CPC charge
        entries.append(LedgerEntry(
            tenant_id=execution.consumer_id,
            entry_type=LedgerEntryType.CONTRACT_BASE_CHARGE,
            amount=-cost.cpc_base,  # Negative = debit
            reference_id=execution.id,
            description=f"Contract {execution.contract_id} - Base"
        ))

        # CPA bonus charge (if any)
        if cost.cpa_bonus > 0:
            entries.append(LedgerEntry(
                tenant_id=execution.consumer_id,
                entry_type=LedgerEntryType.CONTRACT_BONUS_CHARGE,
                amount=-cost.cpa_bonus,
                reference_id=execution.id,
                description=f"Contract {execution.contract_id} - CPA Bonus"
            ))

        # Penalty credit/refund (if any)
        if cost.cpa_penalty > 0:
            entries.append(LedgerEntry(
                tenant_id=execution.consumer_id,
                entry_type=LedgerEntryType.CONTRACT_PENALTY_CREDIT,
                amount=cost.cpa_penalty,  # Positive = credit
                reference_id=execution.id,
                description=f"Contract {execution.contract_id} - Penalty Refund"
            ))

        # Apply entries
        for entry in entries:
            await tx.add_ledger_entry(entry)
            await tx.update_balance(
                entry.tenant_id,
                entry.amount
            )

    async def _pay_provider(
        self,
        tx: Transaction,
        execution: Execution,
        cost: CostBreakdown
    ):
        entries = []

        # Base earning (minus platform fee portion)
        base_earning = cost.cpc_base * (1 - self.PLATFORM_FEE_RATE)
        entries.append(LedgerEntry(
            tenant_id=execution.provider_id,
            entry_type=LedgerEntryType.CONTRACT_BASE_EARNING,
            amount=base_earning,
            reference_id=execution.id,
            description=f"Contract {execution.contract_id} - Base Earning"
        ))

        # Bonus earning (if any)
        if cost.cpa_bonus > 0:
            bonus_earning = cost.cpa_bonus * (1 - self.PLATFORM_FEE_RATE)
            entries.append(LedgerEntry(
                tenant_id=execution.provider_id,
                entry_type=LedgerEntryType.CONTRACT_BONUS_EARNING,
                amount=bonus_earning,
                reference_id=execution.id,
                description=f"Contract {execution.contract_id} - CPA Bonus"
            ))

        # Penalty deduction (if any)
        if cost.cpa_penalty > 0:
            penalty_deduction = cost.cpa_penalty * (1 - self.PLATFORM_FEE_RATE)
            entries.append(LedgerEntry(
                tenant_id=execution.provider_id,
                entry_type=LedgerEntryType.CONTRACT_PENALTY_DEBIT,
                amount=-penalty_deduction,
                reference_id=execution.id,
                description=f"Contract {execution.contract_id} - Penalty"
            ))

        # Platform fee entry
        entries.append(LedgerEntry(
            tenant_id=execution.provider_id,
            entry_type=LedgerEntryType.PLATFORM_FEE,
            amount=-cost.platform_fee,
            reference_id=execution.id,
            description=f"Contract {execution.contract_id} - Platform Fee (15%)"
        ))

        # Apply entries
        for entry in entries:
            await tx.add_ledger_entry(entry)
            await tx.update_balance(
                entry.tenant_id,
                entry.amount
            )
```

## Database Schema Changes

```sql
-- Enhanced executions table
ALTER TABLE executions ADD COLUMN cpa_terms JSONB;
ALTER TABLE executions ADD COLUMN criteria_results JSONB;
ALTER TABLE executions ADD COLUMN cost_cpa_bonus DECIMAL(10,6) DEFAULT 0;
ALTER TABLE executions ADD COLUMN cost_cpa_penalty DECIMAL(10,6) DEFAULT 0;

-- Rename for clarity
ALTER TABLE executions RENAME COLUMN cost_base TO cost_cpc_base;
ALTER TABLE executions RENAME COLUMN cost_total TO cost_gross_total;

-- Add columns for breakdown
ALTER TABLE executions ADD COLUMN cost_platform_fee DECIMAL(10,6);
ALTER TABLE executions ADD COLUMN cost_provider_payout DECIMAL(10,6);

-- Enhanced ledger entries
ALTER TABLE ledger_entries
    ALTER COLUMN entry_type TYPE VARCHAR(30);

-- Add index for CPA analytics
CREATE INDEX idx_executions_cpa ON executions (agent_id, created_at)
    WHERE cpa_terms IS NOT NULL;

-- View for provider earnings breakdown
CREATE VIEW provider_earnings_breakdown AS
SELECT
    provider_id,
    DATE_TRUNC('day', created_at) as date,
    COUNT(*) as total_contracts,
    SUM(cost_cpc_base) as total_cpc,
    SUM(cost_cpa_bonus) as total_bonus,
    SUM(cost_cpa_penalty) as total_penalty,
    SUM(cost_provider_payout) as total_payout
FROM executions
WHERE status = 'COMPLETED'
GROUP BY provider_id, DATE_TRUNC('day', created_at);
```

## Events

### contract.settled Event

```python
# Pub/Sub topic: aex-settlement-events
{
    "event_type": "contract.settled",
    "event_id": "evt-uuid",
    "work_id": "work-uuid",
    "contract_id": "contract-uuid",
    "execution_id": "exec-uuid",
    "agent_id": "agent-uuid",
    "consumer_id": "consumer-uuid",
    "provider_id": "prov-uuid",
    "settlement": {
        "cost_breakdown": {
            "cpc_base": 0.05,
            "cpa_bonus": 0.03,
            "cpa_penalty": 0.00,
            "gross_total": 0.08,
            "platform_fee": 0.012,
            "provider_payout": 0.068,
            "requestor_charge": 0.08
        },
        "verification": {
            "success": true,
            "criteria_results": [
                {
                    "metric": "accuracy",
                    "value": 0.94,
                    "threshold": 0.90,
                    "met": true,
                    "bonus": 0.03
                }
            ]
        }
    },
    "timestamp": "2024-01-15T10:30:00Z"
}
```

## API Endpoints

### Get Execution Details

```
GET /v1/executions/{execution_id}
Authorization: Bearer {token}

Response:
{
  "id": "exec-uuid",
  "work_id": "work-uuid",
  "contract_id": "contract-uuid",
  "agent_id": "agent-uuid",
  "status": "COMPLETED",
  "started_at": "2024-01-15T10:29:00Z",
  "completed_at": "2024-01-15T10:29:02Z",
  "cost_breakdown": {
    "cpc_base": 0.05,
    "cpa_bonus": 0.03,
    "cpa_penalty": 0.00,
    "gross_total": 0.08,
    "platform_fee": 0.012,
    "provider_payout": 0.068,
    "requestor_charge": 0.08
  },
  "outcome_metrics": {
    "accuracy": 0.94,
    "latency_ms": 780
  },
  "criteria_results": [
    {
      "metric": "accuracy",
      "value": 0.94,
      "threshold": 0.90,
      "comparison": "gte",
      "met": true
    }
  ]
}
```

### Get Provider Earnings

```
GET /v1/providers/{provider_id}/earnings
Authorization: Bearer {token}
Query: ?from=2024-01-01&to=2024-01-31

Response:
{
  "provider_id": "prov-uuid",
  "period": {
    "from": "2024-01-01",
    "to": "2024-01-31"
  },
  "summary": {
    "total_contracts": 1500,
    "total_cpc": 75.00,
    "total_bonus": 22.50,
    "total_penalty": 3.75,
    "total_platform_fee": 14.06,
    "total_payout": 79.69
  },
  "by_day": [
    {
      "date": "2024-01-15",
      "contracts": 50,
      "cpc": 2.50,
      "bonus": 0.75,
      "penalty": 0.10,
      "payout": 2.68
    }
  ],
  "by_agent": [
    {
      "agent_id": "agent-uuid",
      "agent_name": "summarizer-v2",
      "contracts": 500,
      "cpc": 25.00,
      "bonus": 10.00,
      "penalty": 1.00,
      "payout": 28.90
    }
  ]
}
```

## Metrics

```python
# New Phase B metrics
settlement_cpa_enabled = Counter(
    "settlement_cpa_enabled_total",
    "Settlements with CPA terms",
    ["has_bonus", "has_penalty"]
)

settlement_bonus_amount = Histogram(
    "settlement_bonus_amount_dollars",
    "CPA bonus amounts",
    buckets=[0.01, 0.02, 0.05, 0.10, 0.20, 0.50, 1.00]
)

settlement_penalty_amount = Histogram(
    "settlement_penalty_amount_dollars",
    "CPA penalty amounts",
    buckets=[0.01, 0.02, 0.05, 0.10, 0.20, 0.50, 1.00]
)

settlement_criteria_met_rate = Gauge(
    "settlement_criteria_met_rate",
    "Rate of criteria being met",
    ["metric"]
)

settlement_verification_latency = Histogram(
    "settlement_verification_latency_seconds",
    "Outcome verification latency"
)
```

## Configuration

```yaml
# config/settlement.yaml (Phase B additions)
settlement:
  platform_fee_rate: 0.15

  cpa:
    enabled: true
    max_bonus_multiplier: 3.0    # Max bonus = 3x CPC
    max_penalty_rate: 0.50       # Max penalty = 50% of CPC
    verification_timeout_ms: 5000

  verification:
    governance_check: true
    store_detailed_results: true

  ledger:
    batch_size: 100
    flush_interval_ms: 1000
```

## Provider Dashboard Data

```python
class ProviderDashboardService:
    """Service for provider earnings dashboard"""

    async def get_earnings_summary(
        self,
        provider_id: str,
        period: DateRange
    ) -> EarningsSummary:
        query = """
        SELECT
            COUNT(*) as total_contracts,
            SUM(cost_cpc_base) as total_cpc,
            SUM(cost_cpa_bonus) as total_bonus,
            SUM(cost_cpa_penalty) as total_penalty,
            SUM(cost_platform_fee) as total_fees,
            SUM(cost_provider_payout) as total_payout,
            AVG(CASE WHEN cost_cpa_bonus > 0 THEN 1 ELSE 0 END) as bonus_rate
        FROM executions
        WHERE provider_id = $1
        AND created_at BETWEEN $2 AND $3
        AND status = 'COMPLETED'
        """
        return await self.db.fetch_one(query, provider_id, period.start, period.end)

    async def get_cpa_performance(
        self,
        provider_id: str,
        agent_id: Optional[str] = None
    ) -> CPAPerformance:
        """Get CPA criteria performance stats"""
        query = """
        SELECT
            jsonb_array_elements(criteria_results)->>'metric' as metric,
            AVG(CASE
                WHEN (jsonb_array_elements(criteria_results)->>'met')::boolean
                THEN 1 ELSE 0
            END) as success_rate,
            COUNT(*) as total_evaluations
        FROM executions
        WHERE provider_id = $1
        AND cpa_terms IS NOT NULL
        AND ($2::text IS NULL OR agent_id = $2)
        GROUP BY metric
        """
        return await self.db.fetch_all(query, provider_id, agent_id)
```

## Migration from Phase A

1. **Database Migration**:
   - Add new columns with defaults
   - Existing records get cpa_bonus=0, cpa_penalty=0
   - No data migration needed

2. **Code Compatibility**:
   - CPA terms are optional
   - If no CPA terms, settlement works exactly like Phase A
   - All new fields have sensible defaults

3. **Event Compatibility**:
   - New fields in contract.settled event
   - Consumers should ignore unknown fields
