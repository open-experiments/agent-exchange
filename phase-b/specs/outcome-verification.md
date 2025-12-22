# Outcome Verification Framework Specification

## Overview

The Outcome Verification Framework provides the infrastructure for defining, measuring, and verifying task outcomes in the AEX platform. This is the foundation for CPA (Cost Per Action) pricing, enabling agents to earn bonuses based on verified performance.

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                    OUTCOME VERIFICATION FLOW                     │
│                                                                  │
│  Task Definition    Execution       Measurement     Verification │
│  ┌─────────────┐   ┌──────────┐   ┌────────────┐   ┌──────────┐  │
│  │  Success    │──►│  Agent   │──►│  Metric    │──►│  Verify  │  │
│  │  Criteria   │   │  Output  │   │  Extraction│   │  Against │  │
│  └─────────────┘   └──────────┘   └────────────┘   │  Criteria│  │
│                                                    └──────────┘  │
│                                          │                       │
│                                          ▼                       │
│                              ┌──────────────────────┐            │
│                              │  Settlement Engine   │            │
│                              │  (CPC + CPA calc)    │            │
│                              └──────────────────────┘            │
└──────────────────────────────────────────────────────────────────┘
```

## Success Criteria Definition

### Supported Metric Types

```python
from enum import Enum
from pydantic import BaseModel
from typing import Union, Optional

class MetricType(str, Enum):
    # Numeric metrics
    NUMERIC = "numeric"           # Any float value
    PERCENTAGE = "percentage"     # 0-100 or 0-1
    LATENCY = "latency"           # Milliseconds
    COUNT = "count"               # Integer count

    # Quality metrics
    ACCURACY = "accuracy"         # Model accuracy
    BLEU_SCORE = "bleu_score"     # Text similarity
    ROUGE_SCORE = "rouge_score"   # Summarization quality
    F1_SCORE = "f1_score"         # Classification quality

    # Boolean metrics
    BOOLEAN = "boolean"           # True/False
    CONTAINS = "contains"         # Contains keywords
    MATCHES_SCHEMA = "matches_schema"  # Output matches schema

    # Custom metrics
    CUSTOM = "custom"             # User-defined evaluation

class ComparisonOperator(str, Enum):
    GTE = "gte"                   # Greater than or equal
    GT = "gt"                     # Greater than
    LTE = "lte"                   # Less than or equal
    LT = "lt"                     # Less than
    EQ = "eq"                     # Equal
    NEQ = "neq"                   # Not equal
    IN_RANGE = "in_range"         # Between min and max
    CONTAINS_ALL = "contains_all" # Contains all values
    CONTAINS_ANY = "contains_any" # Contains any value

class SuccessCriterion(BaseModel):
    metric: str                   # Metric name
    metric_type: MetricType
    comparison: ComparisonOperator
    threshold: Union[float, bool, list, dict]
    weight: float = 1.0           # For weighted scoring
    required: bool = True         # Must pass for task success
    bonus: Optional[float] = None # CPA bonus if met
    penalty: Optional[float] = None # Penalty if not met

class SuccessCriteria(BaseModel):
    criteria: list[SuccessCriterion]
    aggregation: str = "all"      # "all", "any", "weighted"
    minimum_weighted_score: Optional[float] = None  # For weighted
```

### Example Success Criteria

```yaml
# NLP Summarization Task
success_criteria:
  criteria:
    - metric: "accuracy"
      metric_type: "percentage"
      comparison: "gte"
      threshold: 0.90
      required: true
      bonus: 0.03

    - metric: "latency_ms"
      metric_type: "latency"
      comparison: "lte"
      threshold: 2000
      required: false
      bonus: 0.02

    - metric: "output_length"
      metric_type: "count"
      comparison: "in_range"
      threshold: {"min": 100, "max": 500}
      required: true

    - metric: "contains_keywords"
      metric_type: "contains"
      comparison: "contains_all"
      threshold: ["summary", "conclusion"]
      required: false

  aggregation: "all"

# Image Classification Task
success_criteria:
  criteria:
    - metric: "f1_score"
      metric_type: "f1_score"
      comparison: "gte"
      threshold: 0.85
      weight: 0.6
      bonus: 0.05

    - metric: "confidence"
      metric_type: "percentage"
      comparison: "gte"
      threshold: 0.80
      weight: 0.4

  aggregation: "weighted"
  minimum_weighted_score: 0.75
```

## Metric Extractors

### Base Extractor Interface

```python
from abc import ABC, abstractmethod
from typing import Any

class MetricExtractor(ABC):
    """Base class for extracting metrics from task outputs"""

    @abstractmethod
    async def extract(
        self,
        task_input: dict,
        task_output: dict,
        metadata: dict
    ) -> dict[str, float]:
        """Extract metrics from task input/output"""
        pass

    @abstractmethod
    def supported_metrics(self) -> list[str]:
        """List of metrics this extractor can compute"""
        pass
```

### Built-in Extractors

```python
class LatencyExtractor(MetricExtractor):
    """Extract latency-based metrics"""

    def supported_metrics(self) -> list[str]:
        return ["latency_ms", "time_to_first_byte", "processing_time"]

    async def extract(
        self,
        task_input: dict,
        task_output: dict,
        metadata: dict
    ) -> dict[str, float]:
        return {
            "latency_ms": metadata.get("duration_ms", 0),
            "time_to_first_byte": metadata.get("ttfb_ms", 0),
            "processing_time": metadata.get("processing_ms", 0)
        }


class TextQualityExtractor(MetricExtractor):
    """Extract text quality metrics using NLP"""

    def __init__(self):
        from rouge_score import rouge_scorer
        from sacrebleu import corpus_bleu
        self.rouge = rouge_scorer.RougeScorer(['rouge1', 'rouge2', 'rougeL'])

    def supported_metrics(self) -> list[str]:
        return [
            "bleu_score", "rouge1", "rouge2", "rougeL",
            "output_length", "word_count"
        ]

    async def extract(
        self,
        task_input: dict,
        task_output: dict,
        metadata: dict
    ) -> dict[str, float]:
        output_text = task_output.get("text", "")
        reference = task_input.get("reference", "")

        metrics = {
            "output_length": len(output_text),
            "word_count": len(output_text.split())
        }

        if reference:
            # BLEU score
            metrics["bleu_score"] = corpus_bleu(
                [output_text], [[reference]]
            ).score / 100

            # ROUGE scores
            rouge_scores = self.rouge.score(reference, output_text)
            metrics["rouge1"] = rouge_scores["rouge1"].fmeasure
            metrics["rouge2"] = rouge_scores["rouge2"].fmeasure
            metrics["rougeL"] = rouge_scores["rougeL"].fmeasure

        return metrics


class ClassificationExtractor(MetricExtractor):
    """Extract classification quality metrics"""

    def supported_metrics(self) -> list[str]:
        return [
            "accuracy", "precision", "recall", "f1_score",
            "confidence", "num_predictions"
        ]

    async def extract(
        self,
        task_input: dict,
        task_output: dict,
        metadata: dict
    ) -> dict[str, float]:
        predictions = task_output.get("predictions", [])
        ground_truth = task_input.get("ground_truth", [])

        metrics = {
            "num_predictions": len(predictions),
            "confidence": task_output.get("confidence", 0)
        }

        if ground_truth and predictions:
            from sklearn.metrics import (
                accuracy_score, precision_score,
                recall_score, f1_score
            )

            pred_labels = [p["label"] for p in predictions]
            true_labels = [g["label"] for g in ground_truth]

            metrics["accuracy"] = accuracy_score(true_labels, pred_labels)
            metrics["precision"] = precision_score(
                true_labels, pred_labels, average="weighted"
            )
            metrics["recall"] = recall_score(
                true_labels, pred_labels, average="weighted"
            )
            metrics["f1_score"] = f1_score(
                true_labels, pred_labels, average="weighted"
            )

        return metrics


class ContainsExtractor(MetricExtractor):
    """Extract content-based metrics"""

    def supported_metrics(self) -> list[str]:
        return [
            "contains_keywords", "matches_pattern",
            "matches_schema", "has_required_fields"
        ]

    async def extract(
        self,
        task_input: dict,
        task_output: dict,
        metadata: dict
    ) -> dict[str, float]:
        output_text = str(task_output.get("text", task_output))
        output_lower = output_text.lower()

        metrics = {}

        # Keyword containment
        keywords = task_input.get("required_keywords", [])
        if keywords:
            found = sum(1 for k in keywords if k.lower() in output_lower)
            metrics["contains_keywords"] = found / len(keywords)

        # Schema validation
        schema = task_input.get("output_schema")
        if schema:
            import jsonschema
            try:
                jsonschema.validate(task_output, schema)
                metrics["matches_schema"] = 1.0
            except jsonschema.ValidationError:
                metrics["matches_schema"] = 0.0

        # Required fields
        required_fields = task_input.get("required_fields", [])
        if required_fields:
            found = sum(1 for f in required_fields if f in task_output)
            metrics["has_required_fields"] = found / len(required_fields)

        return metrics


class CustomExtractor(MetricExtractor):
    """Execute custom metric extraction code"""

    def __init__(self, code: str, sandbox: Sandbox):
        self.code = code
        self.sandbox = sandbox

    def supported_metrics(self) -> list[str]:
        return ["custom"]

    async def extract(
        self,
        task_input: dict,
        task_output: dict,
        metadata: dict
    ) -> dict[str, float]:
        # Execute custom code in sandbox
        result = await self.sandbox.execute(
            self.code,
            context={
                "input": task_input,
                "output": task_output,
                "metadata": metadata
            },
            timeout_ms=5000
        )
        return result.get("metrics", {})
```

## Verification Engine

```python
class VerificationEngine:
    """Core engine for verifying task outcomes"""

    def __init__(
        self,
        extractors: dict[str, MetricExtractor],
        governance: GovernanceClient
    ):
        self.extractors = extractors
        self.governance = governance

    async def verify(
        self,
        contract_id: str,
        work_input: dict,
        work_output: dict,
        success_criteria: SuccessCriteria,
        execution_metadata: dict
    ) -> VerificationResult:
        # 1. Extract all required metrics
        metrics = await self._extract_metrics(
            work_input, work_output, execution_metadata,
            success_criteria.criteria
        )

        # 2. Evaluate each criterion
        criterion_results = []
        for criterion in success_criteria.criteria:
            result = self._evaluate_criterion(criterion, metrics)
            criterion_results.append(result)

        # 3. Aggregate results
        success, weighted_score = self._aggregate_results(
            criterion_results,
            success_criteria.aggregation,
            success_criteria.minimum_weighted_score
        )

        # 4. Governance check (fraud detection)
        governance_result = await self.governance.validate_outcome(
            contract_id=contract_id,
            claimed_metrics=metrics,
            execution_context=execution_metadata
        )

        # 5. Calculate bonuses/penalties
        bonus, penalty = self._calculate_incentives(criterion_results)

        return VerificationResult(
            contract_id=contract_id,
            success=success and governance_result.valid,
            metrics=metrics,
            criterion_results=criterion_results,
            weighted_score=weighted_score,
            bonus=bonus,
            penalty=penalty,
            governance_flags=governance_result.flags,
            verified_at=datetime.utcnow()
        )

    async def _extract_metrics(
        self,
        task_input: dict,
        task_output: dict,
        metadata: dict,
        criteria: list[SuccessCriterion]
    ) -> dict[str, float]:
        required_metrics = set(c.metric for c in criteria)
        extracted = {}

        for extractor in self.extractors.values():
            supported = set(extractor.supported_metrics())
            needed = required_metrics & supported

            if needed:
                metrics = await extractor.extract(
                    task_input, task_output, metadata
                )
                extracted.update(metrics)

        return extracted

    def _evaluate_criterion(
        self,
        criterion: SuccessCriterion,
        metrics: dict[str, float]
    ) -> CriterionResult:
        value = metrics.get(criterion.metric)

        if value is None:
            return CriterionResult(
                metric=criterion.metric,
                value=None,
                threshold=criterion.threshold,
                comparison=criterion.comparison,
                met=False,
                error="Metric not found"
            )

        met = self._compare(value, criterion.threshold, criterion.comparison)

        return CriterionResult(
            metric=criterion.metric,
            value=value,
            threshold=criterion.threshold,
            comparison=criterion.comparison,
            met=met,
            weight=criterion.weight,
            bonus=criterion.bonus if met else None,
            penalty=criterion.penalty if not met else None
        )

    def _compare(
        self,
        value: float,
        threshold: Union[float, dict, list],
        comparison: ComparisonOperator
    ) -> bool:
        comparisons = {
            ComparisonOperator.GTE: lambda v, t: v >= t,
            ComparisonOperator.GT: lambda v, t: v > t,
            ComparisonOperator.LTE: lambda v, t: v <= t,
            ComparisonOperator.LT: lambda v, t: v < t,
            ComparisonOperator.EQ: lambda v, t: abs(v - t) < 0.0001,
            ComparisonOperator.NEQ: lambda v, t: abs(v - t) >= 0.0001,
            ComparisonOperator.IN_RANGE: lambda v, t: t["min"] <= v <= t["max"],
            ComparisonOperator.CONTAINS_ALL: lambda v, t: v >= 1.0,  # All found
            ComparisonOperator.CONTAINS_ANY: lambda v, t: v > 0,     # Any found
        }

        return comparisons[comparison](value, threshold)

    def _aggregate_results(
        self,
        results: list[CriterionResult],
        aggregation: str,
        min_weighted_score: Optional[float]
    ) -> tuple[bool, Optional[float]]:
        if aggregation == "all":
            success = all(
                r.met for r in results
                if r.error is None and results[results.index(r)].required
            )
            return success, None

        elif aggregation == "any":
            success = any(r.met for r in results if r.error is None)
            return success, None

        elif aggregation == "weighted":
            total_weight = sum(r.weight for r in results if r.error is None)
            weighted_sum = sum(
                r.weight * (1.0 if r.met else 0.0)
                for r in results if r.error is None
            )
            score = weighted_sum / total_weight if total_weight > 0 else 0
            success = score >= (min_weighted_score or 0.5)
            return success, score

        return False, None

    def _calculate_incentives(
        self,
        results: list[CriterionResult]
    ) -> tuple[float, float]:
        bonus = sum(r.bonus or 0 for r in results if r.met and r.bonus)
        penalty = sum(r.penalty or 0 for r in results if not r.met and r.penalty)
        return bonus, penalty
```

## Data Models

```python
class CriterionResult(BaseModel):
    metric: str
    value: Optional[float]
    threshold: Union[float, dict, list]
    comparison: ComparisonOperator
    met: bool
    weight: float = 1.0
    bonus: Optional[float] = None
    penalty: Optional[float] = None
    error: Optional[str] = None

class VerificationResult(BaseModel):
    contract_id: str
    success: bool
    metrics: dict[str, float]
    criterion_results: list[CriterionResult]
    weighted_score: Optional[float]
    bonus: float
    penalty: float
    governance_flags: list[str]
    verified_at: datetime

class OutcomeRecord(BaseModel):
    """Persisted outcome for audit and ML training"""
    id: str
    contract_id: str
    agent_id: str
    consumer_id: str
    domain: str
    success_criteria: SuccessCriteria
    verification_result: VerificationResult
    execution_metadata: dict
    billing_impact: dict  # CPC base, bonus, penalty, total
    created_at: datetime
```

## API Integration

### Work Publisher Integration

```python
# In aex-work-publisher: Accept success criteria
@router.post("/v1/work")
async def submit_work(work: WorkSubmission):
    # Validate success criteria
    if work.success_criteria:
        for criterion in work.success_criteria.criteria:
            # Verify metric is extractable
            if not extractor_registry.can_extract(criterion.metric):
                raise HTTPException(
                    400,
                    f"Unknown metric: {criterion.metric}"
                )

            # Verify threshold type matches metric type
            if not validate_threshold(criterion):
                raise HTTPException(
                    400,
                    f"Invalid threshold for metric: {criterion.metric}"
                )

    # Store work with criteria
    work_doc = await firestore.create_work(work)

    # Publish event
    await pubsub.publish("work.submitted", {
        "work_id": work_doc.id,
        "has_cpa": work.success_criteria is not None
    })

    return work_doc
```

### Settlement Integration

```python
# In aex-settlement: Calculate CPA cost
async def calculate_cost(
    execution: Execution,
    verification: VerificationResult
) -> Cost:
    # Base CPC
    base_cost = execution.agent.pricing.cpc_rate

    # CPA adjustments
    bonus = verification.bonus
    penalty = verification.penalty

    # Apply caps
    max_bonus = execution.task.budget.max_cpa_total or float('inf')
    bonus = min(bonus, max_bonus)

    total = base_cost + bonus - penalty

    return Cost(
        base=base_cost,
        bonus=bonus,
        penalty=penalty,
        total=max(total, 0),  # Never negative
        breakdown={
            "cpc": base_cost,
            "cpa_bonus": bonus,
            "cpa_penalty": penalty
        }
    )
```

## Configuration

```yaml
# config/outcome-verification.yaml
extractors:
  latency:
    enabled: true
    class: "LatencyExtractor"

  text_quality:
    enabled: true
    class: "TextQualityExtractor"
    config:
      rouge_types: ["rouge1", "rouge2", "rougeL"]

  classification:
    enabled: true
    class: "ClassificationExtractor"

  contains:
    enabled: true
    class: "ContainsExtractor"

  custom:
    enabled: true
    class: "CustomExtractor"
    sandbox:
      timeout_ms: 5000
      memory_limit_mb: 256

verification:
  governance_check: true
  store_outcomes: true
  max_criteria_per_task: 10

caching:
  extractor_results_ttl: 300
```

## Directory Structure

```
outcome-verification/
├── extractors/
│   ├── __init__.py
│   ├── base.py
│   ├── latency.py
│   ├── text_quality.py
│   ├── classification.py
│   ├── contains.py
│   └── custom.py
├── engine/
│   ├── __init__.py
│   ├── verification.py
│   └── aggregation.py
├── models/
│   ├── __init__.py
│   ├── criteria.py
│   └── results.py
├── integration/
│   ├── task_intake.py
│   └── settlement.py
├── tests/
│   ├── test_extractors.py
│   ├── test_verification.py
│   └── fixtures/
└── README.md
```

This framework is used as a shared library by `aex-task-intake`, `aex-settlement`, and `aex-governance` services.
