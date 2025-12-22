# AEX Event Schemas

This document defines all Pub/Sub event schemas used across AEX services.

## Event Envelope

All events follow this envelope structure:

```json
{
  "event_id": "evt_550e8400-e29b-41d4-a716-446655440000",
  "event_type": "contract.completed",
  "schema_version": "1.0",
  "idempotency_key": "contract_789xyz_completed_1705312200",
  "timestamp": "2025-01-15T10:30:00Z",
  "source": "aex-contract-engine",
  "tenant_id": "tenant_123",
  "data": { ... }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| event_id | string | Yes | Unique event ID (UUID format) |
| event_type | string | Yes | Event type identifier |
| schema_version | string | Yes | Schema version for compatibility |
| idempotency_key | string | Yes | Key for deduplication |
| timestamp | datetime | Yes | ISO 8601 timestamp |
| source | string | Yes | Publishing service |
| tenant_id | string | No | Associated tenant |
| data | object | Yes | Event-specific payload |

---

## Work Events

### work.submitted

Published by `aex-work-publisher` when a new work spec is submitted.

**Topic:** `aex-work-events`

```json
{
  "event_type": "work.submitted",
  "data": {
    "work_id": "work_550e8400",
    "domain": "nlp.summarization",
    "requirements": {
      "max_latency_ms": 5000
    },
    "budget": {
      "max_cpc": 0.10,
      "max_cpa_total": 0.05
    },
    "success_criteria": [
      {
        "metric": "accuracy",
        "threshold": 0.90,
        "comparison": "gte"
      }
    ],
    "bid_window_ms": 5000
  }
}
```

**Consumers:**
- `aex-provider-registry` - Broadcasts to subscribed providers

---

### work.bid_window_closed

Published by `aex-work-publisher` when bid collection window expires.

**Topic:** `aex-work-events`

```json
{
  "event_type": "work.bid_window_closed",
  "data": {
    "work_id": "work_550e8400",
    "bid_count": 5,
    "closed_at": "2025-01-15T10:30:05Z"
  }
}
```

**Consumers:**
- `aex-bid-evaluator` - Triggers bid evaluation

---

## Bid Events

### bid.submitted

Published by `aex-bid-gateway` when a provider submits a bid.

**Topic:** `aex-bid-events`

```json
{
  "event_type": "bid.submitted",
  "data": {
    "bid_id": "bid_abc123",
    "work_id": "work_550e8400",
    "provider_id": "prov_expedia",
    "price": 0.08,
    "confidence": 0.92,
    "a2a_endpoint": "https://agent.expedia.com/a2a/v1"
  }
}
```

**Consumers:**
- `aex-work-publisher` - Updates bid count, notifies consumer via WebSocket

---

### bids.evaluated

Published by `aex-bid-evaluator` after ranking bids.

**Topic:** `aex-bid-events`

```json
{
  "event_type": "bids.evaluated",
  "data": {
    "work_id": "work_550e8400",
    "evaluation_id": "eval_abc123",
    "ranked_bids": [
      {
        "bid_id": "bid_def456",
        "provider_id": "prov_abc123",
        "agent_id": "agent_xyz789",
        "rank": 1,
        "score": 0.92,
        "price": 0.08
      }
    ],
    "winning_bid_id": "bid_def456"
  }
}
```

**Consumers:**
- `aex-contract-engine` - Awards contract to winner

---

## Contract Events

### contract.awarded

Published by `aex-contract-engine` when a contract is awarded.

**Topic:** `aex-contract-events`

```json
{
  "event_type": "contract.awarded",
  "data": {
    "contract_id": "contract_789xyz",
    "work_id": "work_550e8400",
    "bid_id": "bid_def456",
    "provider_id": "prov_abc123",
    "agent_id": "agent_xyz789",
    "requestor_id": "tenant_123",
    "agreed_price": 0.08,
    "cpa_terms": {
      "criteria": [
        {"metric": "accuracy", "threshold": 0.90, "bonus": 0.02}
      ],
      "max_bonus": 0.05
    },
    "a2a_endpoint": "https://provider.example.com/a2a"
  }
}
```

**Consumers:**
- `aex-settlement` - Prepares for settlement
- `aex-trust-broker` - Records contract for provider

---

### contract.completed

Published by `aex-contract-engine` when provider completes work.

**Topic:** `aex-contract-events`

```json
{
  "event_type": "contract.completed",
  "data": {
    "contract_id": "contract_789xyz",
    "work_id": "work_550e8400",
    "agent_id": "agent_xyz789",
    "provider_id": "prov_abc123",
    "requestor_id": "tenant_123",
    "domain": "nlp.summarization",
    "started_at": "2025-01-15T10:30:00Z",
    "completed_at": "2025-01-15T10:30:02Z",
    "duration_ms": 2000,
    "billing": {
      "cost": 0.08
    },
    "metrics": {
      "accuracy": 0.94,
      "latency_ms": 780
    },
    "metadata": {}
  }
}
```

**Consumers:**
- `aex-settlement` - Settles payment
- `aex-trust-broker` - Updates trust score
- `aex-trust-scoring` (Phase B) - Updates ML model

---

### contract.failed

Published by `aex-contract-engine` when execution fails.

**Topic:** `aex-contract-events`

```json
{
  "event_type": "contract.failed",
  "data": {
    "contract_id": "contract_789xyz",
    "work_id": "work_550e8400",
    "agent_id": "agent_xyz789",
    "provider_id": "prov_abc123",
    "requestor_id": "tenant_123",
    "failure_reason": "timeout",
    "error_code": "EXECUTION_TIMEOUT",
    "error_message": "Agent did not respond within SLA"
  }
}
```

**Consumers:**
- `aex-settlement` - Records failure (no charge)
- `aex-trust-broker` - Updates trust score (negative)

---

### contract.settled

Published by `aex-settlement` after payment is processed.

**Topic:** `aex-settlement-events`

```json
{
  "event_type": "contract.settled",
  "data": {
    "contract_id": "contract_789xyz",
    "work_id": "work_550e8400",
    "execution_id": "exec_abc123",
    "provider_id": "prov_abc123",
    "requestor_id": "tenant_123",
    "cost_breakdown": {
      "cpc_base": 0.08,
      "cpa_bonus": 0.02,
      "cpa_penalty": 0.00,
      "gross_total": 0.10,
      "platform_fee": 0.015,
      "provider_payout": 0.085
    }
  }
}
```

**Consumers:**
- `aex-trust-broker` - Confirms settlement for trust
- `aex-telemetry` - Analytics

---

## Trust Events

### trust.score_updated

Published by `aex-trust-broker` when a provider's trust score changes.

**Topic:** `aex-trust-events`

```json
{
  "event_type": "trust.score_updated",
  "data": {
    "provider_id": "prov_abc123",
    "agent_id": "agent_xyz789",
    "previous_score": 0.85,
    "new_score": 0.87,
    "previous_tier": "SILVER",
    "new_tier": "SILVER",
    "reason": "contract_completed"
  }
}
```

**Consumers:**
- `aex-provider-registry` - Updates cached trust score
- `aex-bid-evaluator` - Uses in scoring

---

### trust.tier_changed

Published by `aex-trust-broker` when provider tier changes.

**Topic:** `aex-trust-events`

```json
{
  "event_type": "trust.tier_changed",
  "data": {
    "provider_id": "prov_abc123",
    "previous_tier": "SILVER",
    "new_tier": "GOLD",
    "effective_at": "2025-01-15T00:00:00Z"
  }
}
```

**Consumers:**
- `aex-provider-registry` - Updates tier limits

---

### trust.prediction_updated (Phase B)

Published by `aex-trust-scoring` when ML model updates predictions.

**Topic:** `aex-trust-events`

```json
{
  "event_type": "trust.prediction_updated",
  "data": {
    "agent_id": "agent_xyz789",
    "predicted_success_rate": 0.92,
    "confidence": 0.85,
    "model_version": "v2.1.0",
    "features_hash": "sha256:abc123"
  }
}
```

**Consumers:**
- `aex-trust-broker` - May trigger tier reevaluation

---

## Identity Events

### tenant.created

Published by `aex-identity` when a new tenant is created.

**Topic:** `aex-identity-events`

```json
{
  "event_type": "tenant.created",
  "data": {
    "tenant_id": "tenant_550e8400",
    "external_id": "acme-corp",
    "name": "Acme Corp",
    "type": "BOTH"
  }
}
```

**Consumers:**
- `aex-settlement` - Creates balance record

---

### tenant.suspended

Published by `aex-identity` when a tenant is suspended.

**Topic:** `aex-identity-events`

```json
{
  "event_type": "tenant.suspended",
  "data": {
    "tenant_id": "tenant_550e8400",
    "reason": "billing_overdue"
  }
}
```

**Consumers:**
- `aex-gateway` - Blocks requests

---

### apikey.revoked

Published by `aex-identity` when an API key is revoked.

**Topic:** `aex-identity-events`

```json
{
  "event_type": "apikey.revoked",
  "data": {
    "tenant_id": "tenant_550e8400",
    "key_id": "key_789",
    "prefix": "ak_live_xxxx"
  }
}
```

**Consumers:**
- `aex-gateway` - Invalidates cache

---

## Governance Events (Phase B)

### policy.evaluated

Published by `aex-governance` after policy evaluation.

**Topic:** `aex-governance-events`

```json
{
  "event_type": "policy.evaluated",
  "data": {
    "decision_id": "dec-uuid",
    "policy_type": "submission",
    "allowed": true,
    "evaluation_time_ms": 12
  }
}
```

**Consumers:**
- `aex-telemetry` - Audit logging

---

### safety.violation

Published by `aex-governance` when content fails safety check.

**Topic:** `aex-governance-events`

```json
{
  "event_type": "safety.violation",
  "data": {
    "check_id": "chk-uuid",
    "content_type": "task_payload",
    "category": "prompt_injection",
    "score": 0.85
  }
}
```

**Consumers:**
- `aex-identity` - May flag tenant
- `aex-telemetry` - Security logging

---

## Pub/Sub Topics Summary

| Topic | Publishers | Events |
|-------|------------|--------|
| `aex-work-events` | work-publisher | work.submitted, work.bid_window_closed |
| `aex-bid-events` | bid-gateway, bid-evaluator | bid.submitted, bids.evaluated |
| `aex-contract-events` | contract-engine | contract.awarded, contract.completed, contract.failed |
| `aex-settlement-events` | settlement | contract.settled |
| `aex-trust-events` | trust-broker, trust-scoring | trust.score_updated, trust.tier_changed, trust.prediction_updated |
| `aex-identity-events` | identity | tenant.created, tenant.suspended, apikey.revoked |
| `aex-governance-events` | governance | policy.evaluated, safety.violation |

---

## Idempotency Guidelines

1. **Event ID**: Use UUID v4 for `event_id`
2. **Idempotency Key**: Format as `{entity}_{id}_{action}_{timestamp_epoch}`
3. **Consumer Deduplication**: Store processed `event_id` or `idempotency_key` with TTL
4. **Retry Handling**: Pub/Sub may deliver duplicates; consumers must be idempotent

```python
# Example consumer deduplication
async def process_event(event: Event, redis: Redis):
    key = f"processed:{event.idempotency_key}"

    # Check if already processed
    if await redis.exists(key):
        logger.info("duplicate_event_skipped", event_id=event.event_id)
        return

    # Process event
    await handle_event(event)

    # Mark as processed (24h TTL)
    await redis.setex(key, 86400, "1")
```
