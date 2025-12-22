# Phase B: Outcome Economics

**Objective:** Add CPA (Cost Per Action) pricing with outcome verification, policy governance, advanced trust scoring, and protocol adapters for external providers.

## Overview

Phase B builds on the broker-based marketplace foundation to introduce outcome-based economics. External providers can earn bonuses (or face penalties) based on verified task outcomes. This requires a robust outcome verification framework, ML-based trust scoring for bid evaluation, and a governance layer to enforce policies.

**Key Principle:** AEX remains a broker. Phase B enhances the marketplace with outcome economics while maintaining the separation between AEX (broker) and external providers (execution).

## Service Architecture

```
┌───────────────────────────────────────────────────────────────────────┐
│                              API LAYER                                │
│  ┌──────────────┐    ┌────────────────────┐  ┌──────────────────────┐ │
│  │ aex-gateway  │───►│ aex-work-publisher │  │ aex-provider-registry│ │
│  │   (Go)       │    │     (Python)       │  │      (Python)        │ │
│  │              │    │    [ENHANCED]      │  │     [ENHANCED]       │ │
│  └──────────────┘    └─────────┬──────────┘  └──────────┬───────────┘ │
├────────────────────────────────┼────────────────────────┼─────────────┤
│                         EVENT BUS (Pub/Sub)             │             │
│                                │                        │             │
├────────────────────────────────┼────────────────────────┼─────────────┤
│                         EXCHANGE CORE                   │             │
│  ┌──────────────────┐  ┌───────┴──────┐   ┌─────────────┴───────────┐ │
│  │  aex-bid-gateway │  │   Pub/Sub    │   │  aex-contract-engine    │ │
│  │       (Go)       │  │              │   │       (Python)          │ │
│  │   [ENHANCED]     │  └──────────────┘   │      [ENHANCED]         │ │
│  └────────┬─────────┘                     └─────────────┬───────────┘ │
│           │                                             │             │
│           ▼                                             ▼             │
│  ┌──────────────────┐                     ┌─────────────────────────┐ │
│  │ aex-bid-evaluator│                     │    aex-settlement       │ │
│  │     (Python)     │                     │       (Python)          │ │
│  │   [ENHANCED]     │                     │      [ENHANCED]         │ │
│  └──────────────────┘                     └─────────────────────────┘ │
├───────────────────────────────────────────────────────────────────────┤
│                           NEW SERVICES                                │
│  ┌──────────────────┐  ┌───────────────────┐  ┌────────────────────┐  │
│  │  aex-governance  │  │ aex-trust-scoring │  │ aex-outcome-oracle │  │
│  │     (Python)     │  │     (Python)      │  │     (Python)       │  │
│  │      [*NEW]      │  │      [*NEW]       │  │      [*NEW]        │  │
│  └──────────────────┘  └───────────────────┘  └────────────────────┘  │
├───────────────────────────────────────────────────────────────────────┤
│                           SHARED SERVICES                             │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────────┐         │
│  │aex-telemetry │    │ aex-identity │    │  aex-trust-broker│         │
│  │    (Go)      │    │   (Python)   │    │     (Python)     │         │
│  │              │    │              │    │    [ENHANCED]    │         │
│  └──────────────┘    └──────────────┘    └──────────────────┘         │
│                                                                       │
│        Firestore │ Redis │ Cloud SQL │ BigQuery ML │ Vertex AI        │
└───────────────────────────────────────────────────────────────────────┘
                                │
                         EXTERNAL WORLD
                                │
        ┌───────────────────────┼───────────────────────┐
        ▼                       ▼                       ▼
┌───────────────┐     ┌───────────────┐     ┌───────────────┐
│  Provider A   │     │  Provider B   │     │  Provider C   │
│  (External)   │     │  (External)   │     │  (Internal)   │
│ A2A Endpoint  │     │ A2A Endpoint  │     │ A2A Endpoint  │
│ MCP Toolbox   │     │ MCP Toolbox   │     │ MCP Toolbox   │
└───────────────┘     └───────────────┘     └───────────────┘
```

## New Services

Phase B introduces 3 new services:

| Service | Spec | Language | Runtime | Purpose |
|---------|------|----------|---------|---------|
| aex-governance | [spec](./specs/aex-governance.md) | Python | Cloud Run | Policy engine, safety rails, compliance |
| aex-trust-scoring | [spec](./specs/aex-trust-scoring.md) | Python | Cloud Run | ML-based trust score predictions |
| aex-outcome-oracle | TODO | Python | Cloud Run | Advanced outcome verification |

## Enhanced Services

Phase B enhances 7 existing Phase A services with CPA support:

| Service | Enhancement Spec | Changes |
|---------|-----------------|---------|
| aex-work-publisher | TODO | Success criteria definition, CPA bonus terms |
| aex-bid-gateway | TODO | CPA bid terms, outcome guarantees |
| aex-bid-evaluator | TODO | ML-based outcome prediction, expected value ranking |
| aex-contract-engine | TODO | CPA tracking, outcome verification triggers |
| aex-settlement | [spec](./specs/enhanced-settlement.md) | CPA calculation, bonus/penalty processing |
| aex-provider-registry | TODO | CPA pricing tiers, outcome history |
| aex-trust-broker | TODO | ML model integration, prediction caching |

## Frameworks

| Framework | Spec | Purpose |
|-----------|------|---------|
| Outcome Verification | [spec](./specs/outcome-verification.md) | Verify task outcomes against success criteria |

## Infrastructure Additions

| Component | Spec | GCP Service | Purpose |
|-----------|------|-------------|---------|
| ML Pipeline | [spec](./specs/infrastructure.md) | BigQuery ML | Trust score & outcome prediction model training |
| Prediction Service | [spec](./specs/infrastructure.md) | Vertex AI | Real-time outcome prediction |
| Enhanced Monitoring | [spec](./specs/infrastructure.md) | Cloud Monitoring | CPA dashboards, outcome metrics |

## Data Flow

### CPA Contract Flow

```
1.  Consumer → aex-gateway: POST /v1/work {category, payload, budget, success_criteria, cpa_bonus}
2.  aex-gateway → aex-governance: Validate policy compliance
3.  aex-gateway → aex-work-publisher: Validate, persist work with CPA terms
4.  aex-work-publisher → Pub/Sub: Publish "work.published"
5.  aex-work-publisher → External Providers: Broadcast work opportunity with CPA terms
6.  Provider decides: Can I meet success criteria? What bonus can I earn?
7.  Provider → aex-bid-gateway: Submit bid with CPA guarantees
8.  aex-bid-gateway → Pub/Sub: Publish "bid.received"
9.  (Bid window closes)
10. Pub/Sub → aex-bid-evaluator: Receive "work.bid_window_closed"
11. aex-bid-evaluator → aex-trust-scoring: Get ML-predicted success probability
12. aex-bid-evaluator: Calculate expected value (price vs predicted success × bonus)
13. aex-bid-evaluator → Pub/Sub: Publish "bids.evaluated" with expected value ranking
14. Consumer (or auto) → aex-contract-engine: Award contract
15. aex-contract-engine → Provider: Notify contract awarded with CPA terms
16. Consumer ↔ Provider: Direct A2A communication (AEX exits)
17. Provider → aex-contract-engine: Report completion with outcome metrics
18. aex-contract-engine → aex-outcome-oracle: Verify outcome against criteria
19. aex-outcome-oracle → aex-governance: Validate outcome claims
20. aex-outcome-oracle: Return verified outcome (success/partial/failure)
21. aex-contract-engine → Pub/Sub: Publish "contract.completed" with verified outcome
22. Pub/Sub → aex-settlement: Calculate base price + CPA bonus/penalty
23. aex-settlement: Process payment (provider payout - platform fee)
24. Pub/Sub → aex-trust-broker: Update provider trust score based on outcome
25. Pub/Sub → aex-trust-scoring: Update ML model training data
```

**Key Insight:** AEX still exits the execution path after step 15. Steps 16-25 occur when the provider reports back.

## Pricing Model (Base + CPA)

```yaml
# Work submission with CPA terms
work:
  budget:
    max_base_price: 0.10        # Maximum base price willing to pay
    bid_strategy: "best_quality"
  success_criteria:
    - metric: "booking_confirmed"
      type: "boolean"
      threshold: true
    - metric: "response_time_ms"
      type: "numeric"
      comparison: "lte"
      threshold: 3000
  cpa_bonus:
    max_total: 0.10             # Maximum bonus pool
    criteria:
      - metric: "booking_confirmed"
        bonus: 0.05              # +$0.05 if booking confirmed
      - metric: "response_time_ms"
        comparison: "lte"
        threshold: 2000
        bonus: 0.02              # +$0.02 if under 2s

# Provider bid with CPA guarantees
bid:
  base_price: 0.08              # $0.08 base cost
  confidence: 0.92              # 92% confident can complete
  cpa_acceptance:
    - metric: "booking_confirmed"
      guarantee: true            # I guarantee booking or forfeit bonus
    - metric: "response_time_ms"
      guarantee: 2500            # I guarantee under 2.5s
  penalty_accepted: true         # Accept -10% base if task fails

# Settlement calculation
settlement:
  base_cost: 0.08                # Provider's bid price
  cpa_earned:
    booking_confirmed: 0.05      # Outcome verified ✓
    response_time: 0.02          # 1800ms < 2000ms ✓
  penalty: 0.00                  # No failure
  total_provider: 0.15           # $0.08 + $0.05 + $0.02
  platform_fee: 0.0225           # 15% of $0.15
  provider_payout: 0.1275        # $0.15 - $0.0225
```

## Outcome Verification

Phase B introduces robust outcome verification through the Outcome Oracle:

```yaml
outcome_verification:
  sources:
    - provider_report           # What provider claims
    - consumer_confirmation     # Consumer can dispute within window
    - automated_checks          # Programmatic verification

  verification_methods:
    boolean:
      # Require evidence
      - provider_must_provide: ["confirmation_number", "timestamp"]
      - consumer_dispute_window: 3600  # 1 hour to dispute

    numeric:
      # Trust provider metrics with spot checks
      - trust_provider_if_tier: ["TRUSTED", "PREFERRED"]
      - spot_check_rate: 0.1    # Verify 10% randomly

    evidence_based:
      # Require cryptographic proof
      - signature_required: true
      - artifact_hash: true

  dispute_resolution:
    auto_resolve_if:
      - provider_tier: "PREFERRED"
      - consumer_tier: "VERIFIED"
      - amount_under: 0.10
    human_review_if:
      - amount_over: 1.00
      - repeated_disputes: 3
```

## Trust Scoring ML Model

Phase B introduces ML-based trust scoring:

```sql
-- BigQuery ML model for outcome prediction
CREATE OR REPLACE MODEL aex_analytics.outcome_predictor
OPTIONS(
  model_type='LOGISTIC_REG',
  input_label_cols=['outcome_success']
) AS
SELECT
  provider_trust_score,
  provider_success_rate,
  provider_contracts_count,
  category_match_score,
  bid_confidence,
  bid_price_ratio,  -- bid_price / max_budget
  work_complexity_score,
  CASE WHEN outcome = 'SUCCESS' THEN 1 ELSE 0 END as outcome_success
FROM aex_analytics.contract_outcomes
WHERE completed_at > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 90 DAY)
```

### Trust Score Enhanced Algorithm

```python
# Phase B trust score combines:
# 1. Historical performance (from Phase A trust broker)
# 2. ML-predicted success probability
# 3. CPA performance history

def calculate_phase_b_trust(provider_id: str, work: WorkSpec) -> TrustScore:
    # Base trust from Phase A
    base_trust = trust_broker.get_score(provider_id)

    # ML prediction for this specific work type
    ml_prediction = trust_scoring.predict_success(
        provider_id=provider_id,
        category=work.category,
        complexity=work.complexity_score
    )

    # CPA performance history
    cpa_history = trust_broker.get_cpa_stats(provider_id)
    cpa_bonus_rate = cpa_history.bonuses_earned / cpa_history.bonuses_possible

    # Weighted combination
    return TrustScore(
        overall=0.4 * base_trust + 0.4 * ml_prediction + 0.2 * cpa_bonus_rate,
        base=base_trust,
        predicted=ml_prediction,
        cpa_rate=cpa_bonus_rate
    )
```

## Governance Layer

The governance service enforces policies across the marketplace:

```yaml
policies:
  work_submission:
    - max_budget_per_work: 10.00
    - max_cpa_bonus_ratio: 2.0     # CPA bonus cannot exceed 2x base
    - required_success_criteria: true
    - banned_categories: ["illegal.*", "adult.*"]

  bidding:
    - min_confidence: 0.5          # Must be at least 50% confident
    - max_price_to_budget_ratio: 1.0
    - require_mvp_sample: true

  provider:
    - min_trust_for_high_value: 0.7  # Work >$1 requires TRUSTED+
    - max_concurrent_contracts: 100
    - cooldown_after_failures: 3600   # 1 hour after 3 failures

  outcome:
    - dispute_window_seconds: 3600
    - max_disputes_per_day: 10
    - auto_fail_on_timeout: true
```

## Success Criteria

Per Phase B objectives:

- [ ] All 10 Phase A services remain operational and healthy
- [ ] 3 new services deployed:
  - [ ] aex-governance (policy engine)
  - [ ] aex-trust-scoring (ML predictions)
  - [ ] aex-outcome-oracle (verification)
- [ ] 7 Phase A services enhanced with CPA support:
  - [ ] aex-work-publisher (success criteria, CPA terms)
  - [ ] aex-bid-gateway (CPA bids)
  - [ ] aex-bid-evaluator (expected value ranking)
  - [ ] aex-contract-engine (CPA tracking)
  - [ ] aex-settlement (CPA calculation)
  - [ ] aex-provider-registry (CPA pricing tiers)
  - [ ] aex-trust-broker (ML integration)
- [ ] Outcome verification framework operational
- [ ] CPA pricing end-to-end: work → bid → contract → outcome → settlement with bonus/penalty
- [ ] Trust score ML model trained in BigQuery ML
- [ ] Governance policies enforced on all requests
- [ ] Provider dashboard showing base + CPA earnings breakdown
- [ ] <100ms added latency for CPA processing (vs Phase A baseline)
- [ ] Backward compatible: Base-price-only work requests work unchanged

## Build Order

Phase B builds incrementally on Phase A:

```
Week 1:     [Infrastructure] BigQuery ML setup, Vertex AI prediction endpoint
Week 1:     [Framework] Outcome verification framework
Week 1:     [Enhanced] aex-work-publisher (success criteria, CPA terms)
Week 2:     [New] aex-governance (policy engine)
Week 2:     [Enhanced] aex-provider-registry (CPA pricing tiers)
Week 2:     [Enhanced] aex-bid-gateway (CPA bids)
Week 3:     [New] aex-trust-scoring (ML predictions)
Week 3:     [Enhanced] aex-bid-evaluator (expected value ranking)
Week 3:     [Enhanced] aex-trust-broker (ML integration)
Week 4:     [New] aex-outcome-oracle (verification service)
Week 4:     [Enhanced] aex-contract-engine (CPA tracking)
Week 4:     [Enhanced] aex-settlement (CPA calculation)
Week 5:     [Integration] End-to-end CPA flow testing
Week 5:     [Dashboard] Provider earnings dashboard
```

## Dependencies on Phase A

Phase B requires all Phase A services to be deployed and functional. Each enhanced service builds directly on its Phase A implementation:

| Phase A Service | Phase A Spec | Phase B Enhancement |
|-----------------|--------------|---------------------|
| aex-gateway | [phase-a/specs/aex-gateway.md](/phase-a/specs/aex-gateway.md) | + Routes to governance for policy checks |
| aex-work-publisher | [phase-a/specs/aex-work-publisher.md](/phase-a/specs/aex-work-publisher.md) | + Success criteria, CPA bonus terms |
| aex-provider-registry | [phase-a/specs/aex-provider-registry.md](/phase-a/specs/aex-provider-registry.md) | + CPA pricing tiers, outcome history |
| aex-bid-gateway | [phase-a/specs/aex-bid-gateway.md](/phase-a/specs/aex-bid-gateway.md) | + CPA bid terms, guarantees |
| aex-bid-evaluator | [phase-a/specs/aex-bid-evaluator.md](/phase-a/specs/aex-bid-evaluator.md) | + ML prediction, expected value |
| aex-contract-engine | [phase-a/specs/aex-contract-engine.md](/phase-a/specs/aex-contract-engine.md) | + CPA tracking, outcome oracle calls |
| aex-settlement | [phase-a/specs/aex-settlement.md](/phase-a/specs/aex-settlement.md) | + CPA calculation, bonus/penalty |
| aex-trust-broker | [phase-a/specs/aex-trust-broker.md](/phase-a/specs/aex-trust-broker.md) | + ML integration, CPA stats |
| aex-telemetry | [phase-a/specs/aex-telemetry.md](/phase-a/specs/aex-telemetry.md) | + CPA metrics export |
| aex-identity | [phase-a/specs/aex-identity.md](/phase-a/specs/aex-identity.md) | No changes |

### Incremental Development Approach

Phase B follows strict incremental development:

1. **No Breaking Changes**: All Phase A APIs remain backward compatible
2. **Optional Fields**: New fields (success_criteria, cpa_terms) are optional
3. **Feature Flags**: CPA features can be disabled to maintain Phase A behavior
4. **Database Migrations**: Additive only, no schema breaking changes
5. **Event Compatibility**: New event fields, consumers ignore unknown fields

### Phase A Infrastructure Reused

| Component | Phase A Setup | Phase B Addition |
|-----------|---------------|------------------|
| Firestore | Work specs, providers, contracts | + Policies, outcome records |
| Cloud SQL | Billing ledger | + CPA breakdown columns |
| Redis | Rate limits, cache | + Prediction cache, policy cache |
| Pub/Sub | Event bus | + New topics (governance, oracle) |
| BigQuery | Analytics | + ML training data, model storage |
