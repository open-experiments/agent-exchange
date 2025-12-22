# aex-trust-scoring Service Specification

## Overview

The `aex-trust-scoring` service provides ML-based trust score calculation for agents. It uses historical performance data to compute trust scores and predict task success probability. In Phase B, this enables CPA-aware matching by predicting which agents are most likely to meet outcome criteria.

## Architecture Position

```
┌─────────────────────────────────────────────────────────────────┐
│                      EXCHANGE CORE                              │
│                                                                 │
│  aex-matching ──► aex-trust-scoring (get predicted success)     │
│                                                                 │
│  aex-settlement ──► aex-trust-scoring (update after execution)  │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    DATA & ML LAYER                              │
│                                                                 │
│  BigQuery (historical data) ──► BigQuery ML (model training)    │
│                                                                 │
│  Vertex AI (optional) ──► Online prediction                     │
└─────────────────────────────────────────────────────────────────┘
```

## Tech Stack

- **Runtime:** Cloud Run
- **Language:** Python 3.11+
- **Framework:** FastAPI
- **ML:** BigQuery ML, scikit-learn (fallback)
- **Feature Store:** Redis (real-time features), BigQuery (historical)
- **Model Serving:** Vertex AI Endpoints (optional)

## API Endpoints

### Get Trust Score

```
GET /v1/agents/{agent_id}/trust-score
Authorization: Bearer {service_token}

Response:
{
  "agent_id": "agent-uuid",
  "trust_score": 0.92,
  "components": {
    "success_rate": 0.95,
    "latency_score": 0.88,
    "consistency_score": 0.91,
    "recency_weight": 0.98
  },
  "confidence": 0.85,
  "sample_size": 1500,
  "last_updated": "2024-01-15T10:30:00Z"
}
```

### Predict Task Success

```
POST /v1/predictions/task-success
Authorization: Bearer {service_token}
Content-Type: application/json

Request:
{
  "agent_id": "agent-uuid",
  "task": {
    "domain": "nlp.summarization",
    "payload_size_bytes": 50000,
    "success_criteria": [
      {"metric": "accuracy", "threshold": 0.90}
    ],
    "max_latency_ms": 2000
  }
}

Response:
{
  "prediction_id": "pred-uuid",
  "agent_id": "agent-uuid",
  "predicted_success_probability": 0.87,
  "predicted_metrics": {
    "accuracy": {"mean": 0.92, "std": 0.05},
    "latency_ms": {"mean": 850, "std": 200}
  },
  "criteria_meet_probability": [
    {"metric": "accuracy", "probability": 0.89}
  ],
  "confidence": 0.82,
  "model_version": "v2.1.0"
}
```

### Batch Trust Scores

```
POST /v1/agents/trust-scores/batch
Authorization: Bearer {service_token}
Content-Type: application/json

Request:
{
  "agent_ids": ["agent-1", "agent-2", "agent-3"]
}

Response:
{
  "scores": [
    {"agent_id": "agent-1", "trust_score": 0.92, "confidence": 0.85},
    {"agent_id": "agent-2", "trust_score": 0.78, "confidence": 0.90},
    {"agent_id": "agent-3", "trust_score": 0.95, "confidence": 0.75}
  ]
}
```

### Update Score (After Execution)

```
POST /v1/agents/{agent_id}/executions
Authorization: Bearer {service_token}
Content-Type: application/json

Request:
{
  "task_id": "task-uuid",
  "execution_id": "exec-uuid",
  "success": true,
  "metrics": {
    "accuracy": 0.94,
    "latency_ms": 780
  },
  "criteria_results": [
    {"metric": "accuracy", "threshold": 0.90, "met": true}
  ]
}

Response:
{
  "agent_id": "agent-uuid",
  "previous_score": 0.91,
  "new_score": 0.92,
  "score_delta": 0.01
}
```

## Trust Score Model

### Score Components

```python
class TrustScoreCalculator:
    """
    Trust Score = weighted combination of:
    - Success Rate (40%): Historical task success percentage
    - Latency Score (20%): Meeting latency SLAs
    - Consistency Score (20%): Low variance in performance
    - Recency Weight (20%): Recent performance weighted higher
    """

    WEIGHTS = {
        "success_rate": 0.40,
        "latency_score": 0.20,
        "consistency_score": 0.20,
        "recency_weight": 0.20
    }

    def calculate(self, agent_id: str, history: list[Execution]) -> TrustScore:
        if len(history) < 10:
            return self._cold_start_score(agent_id, history)

        # Success Rate: successful / total executions
        success_rate = sum(1 for e in history if e.success) / len(history)

        # Latency Score: percentage meeting SLA
        latency_score = sum(
            1 for e in history
            if e.latency_ms <= e.sla_latency_ms
        ) / len(history)

        # Consistency: inverse of coefficient of variation
        metrics = [e.primary_metric for e in history if e.primary_metric]
        if metrics:
            mean = sum(metrics) / len(metrics)
            std = (sum((m - mean) ** 2 for m in metrics) / len(metrics)) ** 0.5
            cv = std / mean if mean > 0 else 1.0
            consistency_score = max(0, 1 - cv)
        else:
            consistency_score = 0.5

        # Recency: exponential decay weighting
        recency_weights = self._calculate_recency_weights(history)
        recency_score = sum(
            w * (1 if e.success else 0)
            for e, w in zip(history, recency_weights)
        ) / sum(recency_weights)

        # Combine with weights
        trust_score = (
            self.WEIGHTS["success_rate"] * success_rate +
            self.WEIGHTS["latency_score"] * latency_score +
            self.WEIGHTS["consistency_score"] * consistency_score +
            self.WEIGHTS["recency_weight"] * recency_score
        )

        return TrustScore(
            agent_id=agent_id,
            score=trust_score,
            components={
                "success_rate": success_rate,
                "latency_score": latency_score,
                "consistency_score": consistency_score,
                "recency_weight": recency_score
            },
            confidence=self._calculate_confidence(len(history)),
            sample_size=len(history)
        )

    def _calculate_recency_weights(self, history: list) -> list[float]:
        """Exponential decay: recent executions weighted higher"""
        decay = 0.95
        now = datetime.utcnow()
        weights = []
        for e in history:
            days_ago = (now - e.completed_at).days
            weights.append(decay ** days_ago)
        return weights

    def _calculate_confidence(self, sample_size: int) -> float:
        """Confidence increases with sample size, plateaus at 1000"""
        return min(1.0, sample_size / 1000)

    def _cold_start_score(self, agent_id: str, history: list) -> TrustScore:
        """For new agents with <10 executions"""
        if not history:
            return TrustScore(
                agent_id=agent_id,
                score=0.5,  # Neutral score
                components={},
                confidence=0.0,
                sample_size=0
            )

        # Simple average with low confidence
        success_rate = sum(1 for e in history if e.success) / len(history)
        return TrustScore(
            agent_id=agent_id,
            score=0.5 + (success_rate - 0.5) * 0.5,  # Regress toward 0.5
            components={"success_rate": success_rate},
            confidence=len(history) / 100,
            sample_size=len(history)
        )
```

### Success Prediction Model

```python
class SuccessPredictor:
    """
    ML model to predict task success probability.
    Uses BigQuery ML for training, serves predictions via API.
    """

    def __init__(self, bq_client, model_name: str):
        self.bq = bq_client
        self.model_name = model_name
        self.feature_extractor = FeatureExtractor()

    async def predict(
        self,
        agent_id: str,
        task: TaskRequest
    ) -> SuccessPrediction:
        # Extract features
        features = await self.feature_extractor.extract(agent_id, task)

        # Get prediction from BigQuery ML
        query = f"""
        SELECT
            predicted_success_probability,
            predicted_accuracy,
            predicted_latency_ms
        FROM ML.PREDICT(
            MODEL `{self.model_name}`,
            (SELECT @features AS features)
        )
        """

        result = await self.bq.query(query, features=features)

        # Calculate criteria meet probabilities
        criteria_probs = []
        for criterion in task.success_criteria:
            prob = self._criterion_probability(
                criterion,
                result.predicted_metrics
            )
            criteria_probs.append({
                "metric": criterion.metric,
                "probability": prob
            })

        return SuccessPrediction(
            agent_id=agent_id,
            predicted_success_probability=result.success_prob,
            predicted_metrics=result.predicted_metrics,
            criteria_meet_probability=criteria_probs,
            confidence=self._model_confidence(features)
        )

    def _criterion_probability(
        self,
        criterion: dict,
        predicted: dict
    ) -> float:
        """
        Calculate probability of meeting criterion using
        predicted metric distribution (mean, std).
        """
        metric = criterion["metric"]
        threshold = criterion["threshold"]
        comparison = criterion.get("comparison", "gte")

        pred = predicted.get(metric)
        if not pred:
            return 0.5  # Unknown

        mean, std = pred["mean"], pred["std"]

        # Use normal distribution CDF
        from scipy import stats

        if comparison in ("gte", "gt"):
            return 1 - stats.norm.cdf(threshold, mean, std)
        elif comparison in ("lte", "lt"):
            return stats.norm.cdf(threshold, mean, std)
        else:  # eq
            return stats.norm.pdf(threshold, mean, std) * std
```

## Feature Engineering

```python
class FeatureExtractor:
    """Extract features for ML model"""

    async def extract(self, agent_id: str, task: TaskRequest) -> dict:
        # Agent features
        agent_stats = await self.get_agent_stats(agent_id)

        # Task features
        task_features = {
            "domain": task.domain,
            "payload_size_bytes": len(json.dumps(task.payload)),
            "num_criteria": len(task.success_criteria),
            "max_latency_ms": task.requirements.max_latency_ms,
            "budget_cpc": task.budget.max_cpc
        }

        # Cross features
        cross_features = {
            "agent_domain_match": task.domain in agent_stats.domains,
            "agent_domain_success_rate": agent_stats.domain_success.get(
                task.domain, 0.5
            ),
            "sla_headroom": (
                task.requirements.max_latency_ms -
                agent_stats.avg_latency_ms
            ) / task.requirements.max_latency_ms
        }

        return {
            **agent_stats.to_dict(),
            **task_features,
            **cross_features
        }

    async def get_agent_stats(self, agent_id: str) -> AgentStats:
        """Get aggregated agent statistics from BigQuery"""
        query = """
        SELECT
            agent_id,
            COUNT(*) as total_executions,
            AVG(CASE WHEN success THEN 1 ELSE 0 END) as success_rate,
            AVG(latency_ms) as avg_latency_ms,
            STDDEV(latency_ms) as std_latency_ms,
            ARRAY_AGG(DISTINCT domain) as domains,
            -- Domain-specific success rates
            ARRAY(
                SELECT AS STRUCT domain, AVG(CASE WHEN success THEN 1 ELSE 0 END) as rate
                FROM UNNEST(executions) GROUP BY domain
            ) as domain_success
        FROM `aex.executions`
        WHERE agent_id = @agent_id
        AND completed_at > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
        GROUP BY agent_id
        """
        return await self.bq.query(query, agent_id=agent_id)
```

## BigQuery ML Model

### Model Training

```sql
-- Create training dataset
CREATE OR REPLACE TABLE `aex.ml_training_data` AS
SELECT
    e.agent_id,
    e.task_id,
    e.domain,
    e.payload_size_bytes,
    e.num_criteria,
    e.max_latency_ms,

    -- Agent historical features (as of task time)
    a.total_executions_before,
    a.success_rate_30d,
    a.avg_latency_30d,
    a.domain_success_rate,

    -- Target
    e.success as label,
    e.accuracy as outcome_accuracy,
    e.latency_ms as outcome_latency
FROM `aex.executions` e
JOIN `aex.agent_features_snapshot` a
    ON e.agent_id = a.agent_id
    AND e.created_at BETWEEN a.snapshot_start AND a.snapshot_end;

-- Train logistic regression model for success prediction
CREATE OR REPLACE MODEL `aex.success_predictor`
OPTIONS(
    model_type = 'LOGISTIC_REG',
    input_label_cols = ['label'],
    auto_class_weights = TRUE,
    l2_reg = 0.01,
    max_iterations = 50
) AS
SELECT
    * EXCEPT(task_id, outcome_accuracy, outcome_latency)
FROM `aex.ml_training_data`;

-- Train linear regression for accuracy prediction
CREATE OR REPLACE MODEL `aex.accuracy_predictor`
OPTIONS(
    model_type = 'LINEAR_REG',
    input_label_cols = ['outcome_accuracy'],
    l2_reg = 0.01
) AS
SELECT
    * EXCEPT(task_id, label, outcome_latency)
FROM `aex.ml_training_data`
WHERE outcome_accuracy IS NOT NULL;
```

### Model Evaluation

```sql
-- Evaluate success predictor
SELECT *
FROM ML.EVALUATE(
    MODEL `aex.success_predictor`,
    (SELECT * FROM `aex.ml_test_data`)
);

-- Feature importance
SELECT *
FROM ML.FEATURE_IMPORTANCE(MODEL `aex.success_predictor`);
```

## Data Models

```python
from pydantic import BaseModel
from datetime import datetime

class TrustScore(BaseModel):
    agent_id: str
    score: float                    # 0.0 - 1.0
    components: dict[str, float]
    confidence: float               # 0.0 - 1.0
    sample_size: int
    last_updated: datetime

class SuccessPrediction(BaseModel):
    prediction_id: str
    agent_id: str
    predicted_success_probability: float
    predicted_metrics: dict[str, dict]  # metric -> {mean, std}
    criteria_meet_probability: list[dict]
    confidence: float
    model_version: str

class Execution(BaseModel):
    execution_id: str
    agent_id: str
    task_id: str
    domain: str
    success: bool
    latency_ms: int
    sla_latency_ms: int
    primary_metric: Optional[float]
    all_metrics: dict[str, float]
    completed_at: datetime

class AgentStats(BaseModel):
    agent_id: str
    total_executions: int
    success_rate: float
    avg_latency_ms: float
    std_latency_ms: float
    domains: list[str]
    domain_success: dict[str, float]
```

## Events

### Consumed Events

```python
# From aex-settlement
{
    "event_type": "task.execution_completed",
    "task_id": "task-uuid",
    "agent_id": "agent-uuid",
    "success": true,
    "metrics": {"accuracy": 0.94, "latency_ms": 780},
    "criteria_results": [...],
    "timestamp": "2024-01-15T10:30:00Z"
}
```

### Published Events

```python
# To aex-matching, aex-agent-registry
{
    "event_type": "trust_score.updated",
    "agent_id": "agent-uuid",
    "previous_score": 0.91,
    "new_score": 0.92,
    "timestamp": "2024-01-15T10:30:05Z"
}
```

## Configuration

```yaml
# config/trust-scoring.yaml
service:
  name: aex-trust-scoring
  port: 8080

scoring:
  weights:
    success_rate: 0.40
    latency_score: 0.20
    consistency_score: 0.20
    recency_weight: 0.20
  recency_decay: 0.95
  min_samples_for_confidence: 10
  cold_start_score: 0.5

prediction:
  model_name: "aex.success_predictor"
  accuracy_model: "aex.accuracy_predictor"
  cache_ttl_seconds: 300
  batch_size: 100

bigquery:
  project_id: ${GCP_PROJECT_ID}
  dataset: aex
  location: US

redis:
  host: ${REDIS_HOST}
  port: 6379
  score_cache_ttl: 60
  feature_cache_ttl: 300

logging:
  level: INFO
```

## Environment Variables

```bash
# Required
GCP_PROJECT_ID=aex-prod
REDIS_HOST=10.0.0.5
BIGQUERY_DATASET=aex

# Optional
LOG_LEVEL=INFO
SCORE_CACHE_TTL=60
PREDICTION_CACHE_TTL=300
MODEL_VERSION=v2.1.0
```

## Observability

### Metrics

```python
trust_score_requests = Counter(
    "trust_score_requests_total",
    "Total trust score requests",
    ["agent_id", "cache_hit"]
)

trust_score_value = Histogram(
    "trust_score_value",
    "Distribution of trust scores",
    buckets=[0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0]
)

prediction_requests = Counter(
    "prediction_requests_total",
    "Total prediction requests",
    ["domain"]
)

prediction_latency = Histogram(
    "prediction_latency_seconds",
    "Prediction latency",
    buckets=[0.01, 0.05, 0.1, 0.25, 0.5, 1.0]
)

model_accuracy = Gauge(
    "model_accuracy",
    "Current model accuracy",
    ["model_name"]
)
```

## Directory Structure

```
aex-trust-scoring/
├── cmd/
│   └── main.py
├── internal/
│   ├── api/
│   │   ├── routes.py
│   │   └── handlers.py
│   ├── scoring/
│   │   ├── calculator.py
│   │   └── components.py
│   ├── prediction/
│   │   ├── predictor.py
│   │   ├── features.py
│   │   └── model.py
│   ├── store/
│   │   ├── bigquery.py
│   │   └── redis.py
│   └── events/
│       ├── consumer.py
│       └── publisher.py
├── ml/
│   ├── training/
│   │   ├── train_success.sql
│   │   ├── train_accuracy.sql
│   │   └── evaluate.sql
│   └── features/
│       └── feature_engineering.sql
├── config/
│   └── trust-scoring.yaml
├── tests/
│   ├── test_calculator.py
│   ├── test_predictor.py
│   └── test_features.py
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
  name: aex-trust-scoring
spec:
  template:
    metadata:
      annotations:
        autoscaling.knative.dev/minScale: "1"
        autoscaling.knative.dev/maxScale: "10"
    spec:
      containerConcurrency: 80
      containers:
        - image: gcr.io/PROJECT/aex-trust-scoring:latest
          ports:
            - containerPort: 8080
          resources:
            limits:
              cpu: "2"
              memory: "2Gi"
          env:
            - name: GCP_PROJECT_ID
              value: "aex-prod"
            - name: REDIS_HOST
              valueFrom:
                secretKeyRef:
                  name: redis-config
                  key: host
```
