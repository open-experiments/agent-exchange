# aex-governance Service Specification

## Overview

The `aex-governance` service is the policy engine for the Agent Exchange. It enforces safety rails, compliance rules, and business policies across all platform operations. In Phase B, it focuses on outcome validation, content safety, and basic rate governance.

## Architecture Position

```
┌──────────────────────────────────────────────────────────────────┐
│                         API LAYER                                │
│  aex-gateway ──► aex-governance (policy check before routing)    │
└──────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────────┐
│                      EXCHANGE CORE                               │
│  aex-settlement ──► aex-governance (validate outcome claims)     │
│  aex-matching ──► aex-governance (filter policy-violating agents)│
└──────────────────────────────────────────────────────────────────┘
```

## Tech Stack

- **Runtime:** Cloud Run
- **Language:** Python 3.11+
- **Framework:** FastAPI
- **Policy Engine:** Open Policy Agent (OPA) / Rego
- **Database:** Firestore (policy definitions), Redis (decision cache)

## API Endpoints

### Policy Evaluation

```
POST /v1/policies/evaluate
Authorization: Bearer {service_token}
Content-Type: application/json

Request:
{
  "policy_type": "task_submission",
  "context": {
    "tenant_id": "tenant-123",
    "domain": "nlp.summarization",
    "payload_size_bytes": 50000,
    "success_criteria": [...],
    "budget": {...}
  }
}

Response:
{
  "allowed": true,
  "decision_id": "dec-uuid",
  "evaluated_policies": [
    {"policy": "content_size_limit", "result": "pass"},
    {"policy": "domain_allowed", "result": "pass"},
    {"policy": "budget_reasonable", "result": "pass"}
  ],
  "warnings": [],
  "metadata": {
    "evaluation_time_ms": 12
  }
}
```

### Outcome Validation

```
POST /v1/outcomes/validate
Authorization: Bearer {service_token}
Content-Type: application/json

Request:
{
  "task_id": "task-uuid",
  "agent_id": "agent-uuid",
  "claimed_metrics": {
    "accuracy": 0.92,
    "latency_ms": 850,
    "tokens_generated": 150
  },
  "success_criteria": [
    {"metric": "accuracy", "threshold": 0.90, "comparison": "gte"}
  ],
  "execution_context": {
    "input_hash": "sha256:...",
    "output_hash": "sha256:...",
    "duration_ms": 1200
  }
}

Response:
{
  "valid": true,
  "validation_id": "val-uuid",
  "criteria_results": [
    {
      "criterion": "accuracy",
      "claimed": 0.92,
      "threshold": 0.90,
      "comparison": "gte",
      "met": true
    }
  ],
  "flags": [],
  "confidence": 0.95
}
```

### Content Safety Check

```
POST /v1/safety/check
Authorization: Bearer {service_token}
Content-Type: application/json

Request:
{
  "content_type": "task_payload",
  "content": "...",
  "context": {
    "domain": "nlp.summarization",
    "tenant_id": "tenant-123"
  }
}

Response:
{
  "safe": true,
  "check_id": "chk-uuid",
  "categories": {
    "harmful_content": {"score": 0.01, "flagged": false},
    "pii_detected": {"score": 0.0, "flagged": false},
    "prompt_injection": {"score": 0.05, "flagged": false}
  }
}
```

### Policy Management

```
GET /v1/policies
GET /v1/policies/{policy_id}
POST /v1/policies
PUT /v1/policies/{policy_id}
DELETE /v1/policies/{policy_id}
```

## Policy Types

### 1. Submission Policies

```rego
# policies/submission.rego
package aex.submission

default allow = false

allow {
    valid_domain
    valid_budget
    content_size_ok
    not blocked_tenant
}

valid_domain {
    input.domain in data.allowed_domains
}

valid_budget {
    input.budget.max_cpc <= data.limits.max_cpc
    input.budget.max_cpa_total <= data.limits.max_cpa
}

content_size_ok {
    input.payload_size_bytes <= data.limits.max_payload_bytes
}

blocked_tenant {
    input.tenant_id in data.blocked_tenants
}
```

### 2. Outcome Policies

```rego
# policies/outcome.rego
package aex.outcome

default valid = false

valid {
    all_metrics_reasonable
    no_gaming_detected
    timing_consistent
}

all_metrics_reasonable {
    every m in input.claimed_metrics {
        m.value >= 0
        m.value <= data.metric_limits[m.name].max
    }
}

no_gaming_detected {
    not latency_suspiciously_low
    not accuracy_always_perfect
}

latency_suspiciously_low {
    input.claimed_metrics.latency_ms < 10
    input.execution_context.duration_ms > 1000
}

accuracy_always_perfect {
    input.claimed_metrics.accuracy == 1.0
    count(data.agent_history[input.agent_id].perfect_scores) > 10
}
```

### 3. Safety Policies

```rego
# policies/safety.rego
package aex.safety

default safe = true

safe = false {
    harmful_content_detected
}

safe = false {
    pii_without_consent
}

safe = false {
    prompt_injection_detected
}

harmful_content_detected {
    input.categories.harmful_content.score > 0.7
}

pii_without_consent {
    input.categories.pii_detected.score > 0.5
    not input.context.pii_consent
}
```

## Data Models

### Policy Definition

```python
from pydantic import BaseModel
from typing import Optional
from datetime import datetime
from enum import Enum

class PolicyType(str, Enum):
    SUBMISSION = "submission"
    OUTCOME = "outcome"
    SAFETY = "safety"
    RATE_LIMIT = "rate_limit"
    AGENT_ELIGIBILITY = "agent_eligibility"

class PolicyScope(str, Enum):
    GLOBAL = "global"
    TENANT = "tenant"
    DOMAIN = "domain"
    AGENT = "agent"

class Policy(BaseModel):
    id: str
    name: str
    description: str
    type: PolicyType
    scope: PolicyScope
    scope_id: Optional[str]  # tenant_id, domain, or agent_id
    rego_code: str           # OPA Rego policy
    enabled: bool
    priority: int            # Lower = higher priority
    created_at: datetime
    updated_at: datetime
    created_by: str

class PolicyDecision(BaseModel):
    id: str
    policy_ids: list[str]
    input_hash: str
    allowed: bool
    results: list[dict]
    warnings: list[str]
    evaluated_at: datetime
    evaluation_time_ms: int
```

### Outcome Validation

```python
class OutcomeValidation(BaseModel):
    id: str
    task_id: str
    agent_id: str
    claimed_metrics: dict[str, float]
    verified_metrics: dict[str, float]
    criteria_results: list[CriterionResult]
    valid: bool
    flags: list[str]
    confidence: float
    validated_at: datetime

class CriterionResult(BaseModel):
    criterion: str
    claimed: float
    threshold: float
    comparison: str  # gte, lte, eq, gt, lt
    met: bool
    verified_value: Optional[float]
```

## Core Logic

### Policy Evaluation Engine

```python
import httpx
from functools import lru_cache

class PolicyEngine:
    def __init__(self, opa_url: str, redis: Redis, firestore: Firestore):
        self.opa_url = opa_url
        self.redis = redis
        self.firestore = firestore

    async def evaluate(
        self,
        policy_type: PolicyType,
        context: dict,
        tenant_id: str
    ) -> PolicyDecision:
        # Get applicable policies
        policies = await self._get_policies(policy_type, tenant_id)

        # Check cache for identical context
        cache_key = self._cache_key(policy_type, context)
        cached = await self.redis.get(cache_key)
        if cached:
            return PolicyDecision.parse_raw(cached)

        # Evaluate with OPA
        results = []
        for policy in sorted(policies, key=lambda p: p.priority):
            result = await self._evaluate_opa(policy, context)
            results.append(result)

            # Short-circuit on deny for high-priority policies
            if not result["allowed"] and policy.priority < 10:
                break

        decision = PolicyDecision(
            id=str(uuid.uuid4()),
            policy_ids=[p.id for p in policies],
            input_hash=hashlib.sha256(json.dumps(context).encode()).hexdigest(),
            allowed=all(r["allowed"] for r in results),
            results=results,
            warnings=self._collect_warnings(results),
            evaluated_at=datetime.utcnow(),
            evaluation_time_ms=int((time.time() - start) * 1000)
        )

        # Cache decision (short TTL for dynamic policies)
        await self.redis.setex(cache_key, 60, decision.json())

        return decision

    async def _evaluate_opa(self, policy: Policy, context: dict) -> dict:
        async with httpx.AsyncClient() as client:
            response = await client.post(
                f"{self.opa_url}/v1/data/{policy.type.value}",
                json={
                    "input": context,
                    "policies": {policy.name: policy.rego_code}
                }
            )
            return response.json()["result"]
```

### Outcome Validator

```python
class OutcomeValidator:
    def __init__(self, policy_engine: PolicyEngine, firestore: Firestore):
        self.policy_engine = policy_engine
        self.firestore = firestore

    async def validate(
        self,
        task_id: str,
        agent_id: str,
        claimed_metrics: dict[str, float],
        success_criteria: list[dict],
        execution_context: dict
    ) -> OutcomeValidation:
        # Get agent history for anomaly detection
        agent_history = await self._get_agent_history(agent_id)

        # Evaluate outcome policies
        policy_result = await self.policy_engine.evaluate(
            PolicyType.OUTCOME,
            {
                "task_id": task_id,
                "agent_id": agent_id,
                "claimed_metrics": claimed_metrics,
                "execution_context": execution_context,
                "agent_history": agent_history
            },
            tenant_id=execution_context.get("tenant_id")
        )

        # Check each criterion
        criteria_results = []
        for criterion in success_criteria:
            metric_name = criterion["metric"]
            claimed_value = claimed_metrics.get(metric_name, 0)
            threshold = criterion["threshold"]
            comparison = criterion.get("comparison", "gte")

            met = self._compare(claimed_value, threshold, comparison)
            criteria_results.append(CriterionResult(
                criterion=metric_name,
                claimed=claimed_value,
                threshold=threshold,
                comparison=comparison,
                met=met
            ))

        # Detect gaming/fraud patterns
        flags = await self._detect_anomalies(
            agent_id, claimed_metrics, agent_history
        )

        # Calculate confidence based on verification depth
        confidence = self._calculate_confidence(
            criteria_results, flags, execution_context
        )

        validation = OutcomeValidation(
            id=str(uuid.uuid4()),
            task_id=task_id,
            agent_id=agent_id,
            claimed_metrics=claimed_metrics,
            verified_metrics=claimed_metrics,  # Phase B: trust claimed values
            criteria_results=criteria_results,
            valid=policy_result.allowed and all(c.met for c in criteria_results),
            flags=flags,
            confidence=confidence,
            validated_at=datetime.utcnow()
        )

        # Store for audit
        await self._store_validation(validation)

        return validation

    def _compare(self, value: float, threshold: float, comparison: str) -> bool:
        comparisons = {
            "gte": lambda v, t: v >= t,
            "gt": lambda v, t: v > t,
            "lte": lambda v, t: v <= t,
            "lt": lambda v, t: v < t,
            "eq": lambda v, t: abs(v - t) < 0.0001
        }
        return comparisons[comparison](value, threshold)

    async def _detect_anomalies(
        self,
        agent_id: str,
        metrics: dict,
        history: list
    ) -> list[str]:
        flags = []

        # Check for statistical anomalies
        for metric, value in metrics.items():
            historical = [h.get(metric) for h in history if metric in h]
            if historical:
                mean = sum(historical) / len(historical)
                std = (sum((x - mean) ** 2 for x in historical) / len(historical)) ** 0.5
                if std > 0 and abs(value - mean) > 3 * std:
                    flags.append(f"anomaly:{metric}:3sigma")

        # Check for gaming patterns
        if metrics.get("accuracy", 0) == 1.0:
            perfect_count = sum(1 for h in history if h.get("accuracy") == 1.0)
            if perfect_count > 5:
                flags.append("pattern:perfect_accuracy_streak")

        return flags
```

## Events

### Published Events

```python
# Pub/Sub topic: aex-governance-events
{
    "event_type": "policy.evaluated",
    "decision_id": "dec-uuid",
    "policy_type": "submission",
    "allowed": true,
    "tenant_id": "tenant-123",
    "timestamp": "2024-01-15T10:30:00Z"
}

{
    "event_type": "outcome.validated",
    "validation_id": "val-uuid",
    "task_id": "task-uuid",
    "agent_id": "agent-uuid",
    "valid": true,
    "flags": [],
    "timestamp": "2024-01-15T10:30:00Z"
}

{
    "event_type": "safety.violation",
    "check_id": "chk-uuid",
    "content_type": "task_payload",
    "category": "prompt_injection",
    "tenant_id": "tenant-123",
    "timestamp": "2024-01-15T10:30:00Z"
}
```

## Configuration

```yaml
# config/governance.yaml
service:
  name: aex-governance
  port: 8080

opa:
  url: "http://localhost:8181"
  bundle_path: "/policies"
  decision_log_enabled: true

redis:
  host: ${REDIS_HOST}
  port: 6379
  decision_cache_ttl: 60

firestore:
  project_id: ${GCP_PROJECT_ID}
  collection_prefix: "governance_"

safety:
  harmful_content_threshold: 0.7
  pii_threshold: 0.5
  prompt_injection_threshold: 0.6

rate_limits:
  evaluations_per_minute: 10000
  validations_per_minute: 5000

logging:
  level: INFO
  decision_audit: true
```

## Environment Variables

```bash
# Required
GCP_PROJECT_ID=aex-prod
REDIS_HOST=10.0.0.5
OPA_URL=http://opa-sidecar:8181
PUBSUB_TOPIC=aex-governance-events

# Optional
LOG_LEVEL=INFO
DECISION_CACHE_TTL=60
ENABLE_SAFETY_CHECKS=true
```

## Observability

### Metrics

```python
# Prometheus metrics
governance_evaluations_total = Counter(
    "governance_evaluations_total",
    "Total policy evaluations",
    ["policy_type", "result"]
)

governance_evaluation_duration = Histogram(
    "governance_evaluation_duration_seconds",
    "Policy evaluation duration",
    ["policy_type"]
)

governance_validations_total = Counter(
    "governance_validations_total",
    "Total outcome validations",
    ["result", "has_flags"]
)

governance_safety_violations = Counter(
    "governance_safety_violations_total",
    "Safety violations detected",
    ["category"]
)
```

### Health Check

```
GET /health
{
  "status": "healthy",
  "opa_connected": true,
  "redis_connected": true,
  "firestore_connected": true,
  "policies_loaded": 45
}
```

## Directory Structure

```
aex-governance/
├── cmd/
│   └── main.py
├── internal/
│   ├── api/
│   │   ├── routes.py
│   │   ├── handlers.py
│   │   └── middleware.py
│   ├── engine/
│   │   ├── policy_engine.py
│   │   ├── outcome_validator.py
│   │   └── safety_checker.py
│   ├── models/
│   │   ├── policy.py
│   │   ├── decision.py
│   │   └── validation.py
│   ├── store/
│   │   ├── firestore.py
│   │   └── redis.py
│   └── events/
│       └── publisher.py
├── policies/
│   ├── submission.rego
│   ├── outcome.rego
│   ├── safety.rego
│   └── rate_limit.rego
├── config/
│   └── governance.yaml
├── tests/
│   ├── test_policy_engine.py
│   ├── test_outcome_validator.py
│   └── test_policies/
├── Dockerfile
├── requirements.txt
└── README.md
```

## Deployment

```yaml
# Cloud Run service
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: aex-governance
spec:
  template:
    metadata:
      annotations:
        autoscaling.knative.dev/minScale: "1"
        autoscaling.knative.dev/maxScale: "20"
    spec:
      containerConcurrency: 100
      containers:
        - image: gcr.io/PROJECT/aex-governance:latest
          ports:
            - containerPort: 8080
          resources:
            limits:
              cpu: "2"
              memory: "1Gi"
          env:
            - name: GCP_PROJECT_ID
              value: "aex-prod"
            - name: REDIS_HOST
              valueFrom:
                secretKeyRef:
                  name: redis-config
                  key: host
```
