# aex-outcome-oracle Service Specification

## Overview

**Purpose:** Advanced outcome verification service that validates provider-reported metrics, detects anomalies, and provides verified outcomes for CPA settlement calculations.

**Language:** Python 3.11+
**Framework:** FastAPI
**Runtime:** Cloud Run
**Port:** 8080

## Architecture Position

```
                    aex-contract-engine
                           │
                           │ verify outcome
                           ▼
                ┌─────────────────────┐
                │  aex-outcome-oracle │◄── THIS SERVICE
                │                     │
                │ • Extract metrics   │
                │ • Verify claims     │
                │ • Detect anomalies  │
                │ • Validate outcomes │
                └──────────┬──────────┘
                           │
          ┌────────────────┼────────────────┐
          ▼                ▼                ▼
    aex-governance   aex-trust-scoring   BigQuery
    (policy check)   (anomaly baseline)  (audit log)
```

## Core Responsibilities

1. **Metric Extraction** - Extract metrics from task outputs using the Outcome Verification Framework
2. **Claim Verification** - Verify provider-reported metrics against extracted values
3. **Anomaly Detection** - Detect statistically anomalous outcomes
4. **Evidence Collection** - Collect and store evidence for disputes
5. **Outcome Validation** - Final pass/fail determination for CPA terms

## API Endpoints

### Verify Outcome

```
POST /internal/v1/outcomes/verify
Authorization: Bearer {service_token}
Content-Type: application/json

Request:
{
  "work_id": "work_550e8400",
  "contract_id": "contract_789xyz",
  "agent_id": "agent_abc123",
  "provider_id": "prov_expedia",
  "execution_context": {
    "started_at": "2025-01-15T10:30:00Z",
    "completed_at": "2025-01-15T10:30:02Z",
    "duration_ms": 2000
  },
  "task_input": {
    "category": "travel.booking",
    "payload": {
      "origin": "LAX",
      "destination": "JFK",
      "date": "2025-03-15"
    }
  },
  "task_output": {
    "confirmation_number": "AA12345",
    "total_price": 599.00,
    "itinerary": {...}
  },
  "claimed_metrics": {
    "booking_confirmed": true,
    "response_time_ms": 1800,
    "price_accuracy": 0.98
  },
  "success_criteria": [
    {
      "metric": "booking_confirmed",
      "metric_type": "boolean",
      "comparison": "eq",
      "threshold": true,
      "required": true,
      "bonus": 0.05
    },
    {
      "metric": "response_time_ms",
      "metric_type": "latency",
      "comparison": "lte",
      "threshold": 3000,
      "required": false,
      "bonus": 0.02
    }
  ]
}

Response:
{
  "verification_id": "ver_abc123",
  "work_id": "work_550e8400",
  "contract_id": "contract_789xyz",
  "success": true,
  "extracted_metrics": {
    "booking_confirmed": true,
    "response_time_ms": 2000,
    "has_confirmation_number": true
  },
  "criteria_results": [
    {
      "metric": "booking_confirmed",
      "claimed_value": true,
      "extracted_value": true,
      "threshold": true,
      "comparison": "eq",
      "met": true,
      "bonus": 0.05,
      "discrepancy": null
    },
    {
      "metric": "response_time_ms",
      "claimed_value": 1800,
      "extracted_value": 2000,
      "threshold": 3000,
      "comparison": "lte",
      "met": true,
      "bonus": 0.02,
      "discrepancy": {
        "type": "minor_deviation",
        "claimed": 1800,
        "actual": 2000,
        "deviation_pct": 11.1
      }
    }
  ],
  "total_bonus": 0.07,
  "total_penalty": 0.00,
  "anomaly_flags": [],
  "governance_check": {
    "passed": true,
    "flags": []
  },
  "evidence": {
    "output_hash": "sha256:abc123...",
    "timestamp_verified": true,
    "evidence_stored": true
  },
  "verified_at": "2025-01-15T10:30:05Z"
}
```

### Get Verification Details

```
GET /internal/v1/outcomes/{verification_id}
Authorization: Bearer {service_token}

Response:
{
  "verification_id": "ver_abc123",
  "work_id": "work_550e8400",
  "contract_id": "contract_789xyz",
  "success": true,
  "criteria_results": [...],
  "evidence": {...},
  "audit_trail": [
    {
      "step": "metric_extraction",
      "timestamp": "2025-01-15T10:30:03Z",
      "result": "success"
    },
    {
      "step": "anomaly_check",
      "timestamp": "2025-01-15T10:30:04Z",
      "result": "pass"
    },
    {
      "step": "governance_check",
      "timestamp": "2025-01-15T10:30:05Z",
      "result": "pass"
    }
  ]
}
```

### Batch Verify (for historical analysis)

```
POST /internal/v1/outcomes/verify/batch
Authorization: Bearer {service_token}

Request:
{
  "verifications": [
    { "work_id": "work_1", "contract_id": "contract_1", ... },
    { "work_id": "work_2", "contract_id": "contract_2", ... }
  ]
}

Response:
{
  "results": [
    { "work_id": "work_1", "success": true, ... },
    { "work_id": "work_2", "success": false, ... }
  ],
  "summary": {
    "total": 2,
    "passed": 1,
    "failed": 1
  }
}
```

## Data Models

```python
from pydantic import BaseModel
from typing import Optional
from datetime import datetime
from enum import Enum

class DiscrepancyType(str, Enum):
    NONE = "none"
    MINOR_DEVIATION = "minor_deviation"  # <20% difference
    MAJOR_DEVIATION = "major_deviation"  # >20% difference
    METRIC_MISSING = "metric_missing"    # Claimed but not extractable
    VALUE_MISMATCH = "value_mismatch"    # Boolean/categorical mismatch
    SUSPICIOUS = "suspicious"            # Anomaly detected

class CriterionVerification(BaseModel):
    metric: str
    claimed_value: Optional[float | bool]
    extracted_value: Optional[float | bool]
    threshold: float | bool | dict
    comparison: str
    met: bool
    bonus: Optional[float]
    penalty: Optional[float]
    discrepancy: Optional[dict]

class AnomalyFlag(BaseModel):
    type: str                          # "statistical", "behavioral", "temporal"
    metric: str
    description: str
    severity: str                      # "low", "medium", "high"
    baseline: Optional[float]
    observed: float
    z_score: Optional[float]

class GovernanceCheckResult(BaseModel):
    passed: bool
    flags: list[str]
    policy_violations: list[dict]

class Evidence(BaseModel):
    output_hash: str
    input_hash: str
    timestamp_verified: bool
    execution_trace_id: Optional[str]
    evidence_stored: bool
    storage_location: Optional[str]

class VerificationResult(BaseModel):
    verification_id: str
    work_id: str
    contract_id: str
    agent_id: str
    provider_id: str
    success: bool
    extracted_metrics: dict[str, float | bool]
    criteria_results: list[CriterionVerification]
    total_bonus: float
    total_penalty: float
    anomaly_flags: list[AnomalyFlag]
    governance_check: GovernanceCheckResult
    evidence: Evidence
    verified_at: datetime
```

## Core Implementation

### Verification Engine

```python
class OutcomeOracle:
    def __init__(
        self,
        extractor_registry: ExtractorRegistry,
        anomaly_detector: AnomalyDetector,
        governance_client: GovernanceClient,
        trust_scoring_client: TrustScoringClient,
        evidence_store: EvidenceStore
    ):
        self.extractors = extractor_registry
        self.anomaly_detector = anomaly_detector
        self.governance = governance_client
        self.trust_scoring = trust_scoring_client
        self.evidence = evidence_store

    async def verify(
        self,
        request: VerificationRequest
    ) -> VerificationResult:
        verification_id = generate_verification_id()
        audit_trail = []

        # 1. Extract metrics from task output
        extracted_metrics = await self._extract_metrics(
            request.task_input,
            request.task_output,
            request.execution_context,
            request.success_criteria
        )
        audit_trail.append(AuditEntry("metric_extraction", "success"))

        # 2. Compare claimed vs extracted
        criteria_results = self._evaluate_criteria(
            request.claimed_metrics,
            extracted_metrics,
            request.success_criteria
        )
        audit_trail.append(AuditEntry("criteria_evaluation", "success"))

        # 3. Anomaly detection
        anomaly_flags = await self._detect_anomalies(
            request.agent_id,
            request.work_id,
            extracted_metrics,
            criteria_results
        )
        audit_trail.append(AuditEntry(
            "anomaly_check",
            "flagged" if anomaly_flags else "pass"
        ))

        # 4. Governance policy check
        governance_result = await self.governance.validate_outcome(
            work_id=request.work_id,
            contract_id=request.contract_id,
            agent_id=request.agent_id,
            claimed_metrics=request.claimed_metrics,
            extracted_metrics=extracted_metrics,
            anomaly_flags=anomaly_flags
        )
        audit_trail.append(AuditEntry(
            "governance_check",
            "pass" if governance_result.passed else "fail"
        ))

        # 5. Calculate totals
        total_bonus = sum(
            cr.bonus or 0 for cr in criteria_results if cr.met
        )
        total_penalty = sum(
            cr.penalty or 0 for cr in criteria_results if not cr.met
        )

        # 6. Determine overall success
        required_met = all(
            cr.met for cr in criteria_results
            if cr.metric in [
                sc.metric for sc in request.success_criteria if sc.required
            ]
        )
        success = required_met and governance_result.passed

        # 7. Store evidence
        evidence = await self._store_evidence(
            verification_id,
            request,
            extracted_metrics,
            criteria_results
        )
        audit_trail.append(AuditEntry("evidence_stored", "success"))

        # 8. Record for trust scoring
        await self.trust_scoring.record_execution(
            agent_id=request.agent_id,
            work_id=request.work_id,
            contract_id=request.contract_id,
            success=success,
            metrics=extracted_metrics,
            criteria_results=criteria_results
        )

        return VerificationResult(
            verification_id=verification_id,
            work_id=request.work_id,
            contract_id=request.contract_id,
            agent_id=request.agent_id,
            provider_id=request.provider_id,
            success=success,
            extracted_metrics=extracted_metrics,
            criteria_results=criteria_results,
            total_bonus=total_bonus,
            total_penalty=total_penalty,
            anomaly_flags=anomaly_flags,
            governance_check=governance_result,
            evidence=evidence,
            verified_at=datetime.utcnow()
        )

    async def _extract_metrics(
        self,
        task_input: dict,
        task_output: dict,
        execution_context: dict,
        success_criteria: list[SuccessCriterion]
    ) -> dict[str, float | bool]:
        required_metrics = set(sc.metric for sc in success_criteria)
        extracted = {}

        # Always extract execution context metrics
        extracted["response_time_ms"] = execution_context.get("duration_ms", 0)

        # Extract metrics based on criteria
        for extractor in self.extractors.get_applicable(required_metrics):
            metrics = await extractor.extract(
                task_input, task_output, execution_context
            )
            extracted.update(metrics)

        return extracted

    def _evaluate_criteria(
        self,
        claimed: dict,
        extracted: dict,
        criteria: list[SuccessCriterion]
    ) -> list[CriterionVerification]:
        results = []

        for criterion in criteria:
            claimed_value = claimed.get(criterion.metric)
            extracted_value = extracted.get(criterion.metric)

            # Determine if criterion is met
            met = self._evaluate_criterion(
                extracted_value, criterion.threshold, criterion.comparison
            )

            # Check for discrepancy
            discrepancy = self._check_discrepancy(
                claimed_value, extracted_value, criterion
            )

            results.append(CriterionVerification(
                metric=criterion.metric,
                claimed_value=claimed_value,
                extracted_value=extracted_value,
                threshold=criterion.threshold,
                comparison=criterion.comparison,
                met=met,
                bonus=criterion.bonus if met else None,
                penalty=criterion.penalty if not met and criterion.required else None,
                discrepancy=discrepancy
            ))

        return results

    def _check_discrepancy(
        self,
        claimed: Optional[float | bool],
        extracted: Optional[float | bool],
        criterion: SuccessCriterion
    ) -> Optional[dict]:
        if claimed is None or extracted is None:
            if claimed is not None:
                return {
                    "type": DiscrepancyType.METRIC_MISSING,
                    "claimed": claimed,
                    "actual": None
                }
            return None

        # Boolean comparison
        if isinstance(claimed, bool) and isinstance(extracted, bool):
            if claimed != extracted:
                return {
                    "type": DiscrepancyType.VALUE_MISMATCH,
                    "claimed": claimed,
                    "actual": extracted
                }
            return None

        # Numeric comparison
        if claimed != extracted:
            deviation_pct = abs(claimed - extracted) / max(abs(claimed), 0.0001) * 100

            if deviation_pct > 20:
                return {
                    "type": DiscrepancyType.MAJOR_DEVIATION,
                    "claimed": claimed,
                    "actual": extracted,
                    "deviation_pct": round(deviation_pct, 1)
                }
            elif deviation_pct > 5:
                return {
                    "type": DiscrepancyType.MINOR_DEVIATION,
                    "claimed": claimed,
                    "actual": extracted,
                    "deviation_pct": round(deviation_pct, 1)
                }

        return None
```

### Anomaly Detection

```python
class AnomalyDetector:
    def __init__(self, bigquery_client: BigQueryClient):
        self.bq = bigquery_client

    async def detect(
        self,
        agent_id: str,
        work_id: str,
        metrics: dict,
        criteria_results: list[CriterionVerification]
    ) -> list[AnomalyFlag]:
        flags = []

        # Get agent's historical baseline
        baseline = await self._get_agent_baseline(agent_id)

        if not baseline:
            return flags  # New agent, no baseline

        # Statistical anomaly detection
        for metric, value in metrics.items():
            if metric in baseline:
                z_score = self._calculate_z_score(
                    value,
                    baseline[metric]["mean"],
                    baseline[metric]["std"]
                )

                if abs(z_score) > 3:  # 3 sigma
                    flags.append(AnomalyFlag(
                        type="statistical",
                        metric=metric,
                        description=f"{metric} is {abs(z_score):.1f} std devs from mean",
                        severity="high" if abs(z_score) > 4 else "medium",
                        baseline=baseline[metric]["mean"],
                        observed=value,
                        z_score=z_score
                    ))

        # Success rate anomaly (sudden improvement)
        success_rate = await self._get_recent_success_rate(agent_id)
        current_success = all(cr.met for cr in criteria_results if cr.bonus)

        if current_success and success_rate < 0.5:
            flags.append(AnomalyFlag(
                type="behavioral",
                metric="success_rate",
                description="Sudden success after pattern of failures",
                severity="medium",
                baseline=success_rate,
                observed=1.0,
                z_score=None
            ))

        return flags

    async def _get_agent_baseline(self, agent_id: str) -> dict:
        query = """
        SELECT
            metric,
            AVG(value) as mean,
            STDDEV(value) as std,
            COUNT(*) as sample_size
        FROM aex_analytics.outcome_metrics
        WHERE agent_id = @agent_id
        AND timestamp > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
        GROUP BY metric
        HAVING COUNT(*) >= 10
        """
        results = await self.bq.query(query, {"agent_id": agent_id})
        return {r["metric"]: r for r in results}

    def _calculate_z_score(
        self, value: float, mean: float, std: float
    ) -> float:
        if std == 0:
            return 0
        return (value - mean) / std
```

## Events

### Published Events

```python
# outcome.verified - Published after verification completes
{
    "event_type": "outcome.verified",
    "event_id": "evt_abc123",
    "verification_id": "ver_abc123",
    "work_id": "work_550e8400",
    "contract_id": "contract_789xyz",
    "agent_id": "agent_abc123",
    "provider_id": "prov_expedia",
    "success": true,
    "total_bonus": 0.07,
    "total_penalty": 0.00,
    "anomaly_count": 0,
    "timestamp": "2025-01-15T10:30:05Z"
}

# outcome.anomaly_detected - Published when anomalies found
{
    "event_type": "outcome.anomaly_detected",
    "event_id": "evt_def456",
    "verification_id": "ver_abc123",
    "work_id": "work_550e8400",
    "agent_id": "agent_abc123",
    "anomaly_flags": [
        {
            "type": "statistical",
            "metric": "response_time_ms",
            "severity": "high"
        }
    ],
    "timestamp": "2025-01-15T10:30:05Z"
}
```

### Consumed Events

```python
# contract.completed - Triggers verification
{
    "event_type": "contract.completed",
    "contract_id": "contract_789xyz",
    "work_id": "work_550e8400",
    "agent_id": "agent_abc123"
}
```

## Storage

### Firestore Collections

```python
# verifications collection
{
    "id": "ver_abc123",
    "work_id": "work_550e8400",
    "contract_id": "contract_789xyz",
    "agent_id": "agent_abc123",
    "provider_id": "prov_expedia",
    "success": true,
    "extracted_metrics": {...},
    "criteria_results": [...],
    "anomaly_flags": [],
    "governance_check": {...},
    "evidence": {
        "output_hash": "sha256:abc123...",
        "gcs_path": "gs://aex-evidence/ver_abc123/"
    },
    "verified_at": "2025-01-15T10:30:05Z"
}
```

### BigQuery Tables

```sql
-- Outcome metrics for analytics and ML
CREATE TABLE aex_analytics.outcome_metrics (
    verification_id STRING,
    work_id STRING,
    contract_id STRING,
    agent_id STRING,
    provider_id STRING,
    metric STRING,
    claimed_value FLOAT64,
    extracted_value FLOAT64,
    threshold FLOAT64,
    met BOOL,
    discrepancy_type STRING,
    timestamp TIMESTAMP
)
PARTITION BY DATE(timestamp);

-- Anomaly log for investigation
CREATE TABLE aex_analytics.outcome_anomalies (
    verification_id STRING,
    agent_id STRING,
    anomaly_type STRING,
    metric STRING,
    severity STRING,
    baseline FLOAT64,
    observed FLOAT64,
    z_score FLOAT64,
    timestamp TIMESTAMP
);
```

## Configuration

```bash
# Server
PORT=8080
ENV=production

# Service clients
GOVERNANCE_URL=https://aex-governance-xxx.run.app
TRUST_SCORING_URL=https://aex-trust-scoring-xxx.run.app

# Storage
FIRESTORE_PROJECT_ID=aex-prod
FIRESTORE_COLLECTION_VERIFICATIONS=verifications
GCS_EVIDENCE_BUCKET=aex-evidence

# BigQuery
BQ_PROJECT_ID=aex-prod
BQ_DATASET=aex_analytics

# Anomaly detection
ANOMALY_Z_SCORE_THRESHOLD=3.0
ANOMALY_MIN_SAMPLE_SIZE=10
ANOMALY_LOOKBACK_DAYS=30

# Verification
VERIFICATION_TIMEOUT_MS=5000
MAX_BATCH_SIZE=100

# Observability
LOG_LEVEL=info
```

## Directory Structure

```
aex-outcome-oracle/
├── app/
│   ├── __init__.py
│   ├── main.py
│   ├── config.py
│   ├── models/
│   │   ├── verification.py
│   │   ├── criteria.py
│   │   └── anomaly.py
│   ├── services/
│   │   ├── oracle.py
│   │   ├── extractor_registry.py
│   │   └── anomaly_detector.py
│   ├── clients/
│   │   ├── governance.py
│   │   └── trust_scoring.py
│   ├── store/
│   │   ├── firestore.py
│   │   ├── bigquery.py
│   │   └── gcs.py
│   └── api/
│       └── verification.py
├── extractors/
│   ├── __init__.py
│   ├── base.py
│   ├── latency.py
│   ├── text_quality.py
│   ├── classification.py
│   └── domain/
│       ├── travel.py
│       └── legal.py
├── tests/
│   ├── test_oracle.py
│   ├── test_anomaly.py
│   └── fixtures/
├── Dockerfile
└── requirements.txt
```

## Integration Points

| Service | Direction | Purpose |
|---------|-----------|---------|
| aex-contract-engine | Inbound | Verification requests |
| aex-governance | Outbound | Policy validation |
| aex-trust-scoring | Outbound | Record execution, get baseline |
| aex-settlement | Indirect | Settlement uses verification results |

## Metrics

```python
verification_requests = Counter(
    "outcome_verification_requests_total",
    "Total verification requests",
    ["success", "has_anomaly"]
)

verification_latency = Histogram(
    "outcome_verification_latency_seconds",
    "Verification processing time"
)

anomaly_detections = Counter(
    "outcome_anomalies_detected_total",
    "Anomalies detected",
    ["type", "severity"]
)

discrepancy_rate = Gauge(
    "outcome_discrepancy_rate",
    "Rate of claimed vs extracted discrepancies",
    ["type"]
)
```
