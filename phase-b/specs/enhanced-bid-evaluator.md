# Enhanced Bid Evaluator Specification (Phase B)

## Overview

Phase B enhances `aex-bid-evaluator` to incorporate ML-based outcome predictions and expected value calculations for CPA-enabled bids. The evaluator now considers predicted bonus/penalty outcomes when ranking bids.

## Changes from Phase A

| Aspect | Phase A | Phase B |
|--------|---------|---------|
| Scoring | price × trust × quality | + predicted outcome, expected value |
| ML Integration | None | Trust scoring predictions |
| Ranking | Simple weighted score | Expected value optimization |
| Output | Ranked bids | + predicted costs, EV breakdown |

## Architecture

```
                    Pub/Sub
              (work.bid_window_closed)
                        │
                        ▼
             ┌──────────────────────┐
             │  aex-bid-evaluator   │◄── THIS SERVICE
             │                      │
             │ • Fetch bids         │
             │ • Get ML predictions │
             │ • Calculate EV       │
             │ • Rank bids          │
             └──────────┬───────────┘
                        │
        ┌───────────────┼───────────────┐
        ▼               ▼               ▼
  aex-bid-gateway  aex-trust-scoring  aex-trust-broker
  (get bids)       (ML predictions)   (historical trust)
```

## Enhanced Scoring Model

### Expected Value Calculation

For CPA-enabled bids, the evaluator calculates expected value:

```
EV = base_price + E[bonus] - E[penalty]

Where:
  E[bonus] = Σ (criterion_bonus × P(criterion_met))
  E[penalty] = penalty_rate × base_price × P(failure)
  P(criterion_met) = f(provider_confidence, ML_prediction, historical_rate)
  P(failure) = 1 - P(all_required_criteria_met)
```

### Scoring Formula

```
score = w1 × price_score + w2 × trust_score + w3 × quality_score + w4 × ev_score

Where (Phase B enhanced):
  price_score = 1 - (price / max_budget)
  trust_score = historical_trust × 0.6 + ml_predicted_success × 0.4
  quality_score = (confidence + mvp_quality + sla_commitment) / 3
  ev_score = (max_ev - expected_cost) / max_ev  # NEW: Expected value score

Weights (configurable):
  Phase A: w1=0.35, w2=0.35, w3=0.30, w4=0
  Phase B: w1=0.25, w2=0.25, w3=0.20, w4=0.30
```

## Core Implementation

### Enhanced Evaluation Flow

```python
class EnhancedBidEvaluator:
    def __init__(
        self,
        bid_gateway: BidGatewayClient,
        trust_broker: TrustBrokerClient,
        trust_scoring: TrustScoringClient,
        work_publisher: WorkPublisherClient
    ):
        self.bid_gateway = bid_gateway
        self.trust_broker = trust_broker
        self.trust_scoring = trust_scoring
        self.work_publisher = work_publisher

    async def evaluate(self, work_id: str) -> EvaluationResult:
        # 1. Fetch work spec and bids
        work = await self.work_publisher.get_work(work_id)
        bids = await self.bid_gateway.get_bids(work_id)

        if not bids:
            return EvaluationResult(
                work_id=work_id,
                status="NO_BIDS",
                ranked_bids=[]
            )

        # 2. Get trust scores and ML predictions in parallel
        provider_ids = list(set(b.provider_id for b in bids))
        agent_ids = list(set(b.agent_id for b in bids))

        trust_scores, ml_predictions = await asyncio.gather(
            self.trust_broker.get_batch_scores(provider_ids),
            self._get_ml_predictions(agent_ids, work) if work.cpa_enabled else {}
        )

        # 3. Score each bid
        scored_bids = []
        for bid in bids:
            scored = await self._score_bid(
                bid, work, trust_scores, ml_predictions
            )
            scored_bids.append(scored)

        # 4. Rank by score (descending)
        scored_bids.sort(key=lambda b: b.score, reverse=True)

        # 5. Determine winner
        winning_bid = scored_bids[0] if scored_bids else None

        return EvaluationResult(
            work_id=work_id,
            status="EVALUATED",
            ranked_bids=scored_bids,
            winning_bid_id=winning_bid.bid_id if winning_bid else None,
            evaluation_metadata={
                "total_bids": len(bids),
                "cpa_bids": len([b for b in bids if b.cpa_acceptance]),
                "ml_predictions_used": work.cpa_enabled
            }
        )

    async def _get_ml_predictions(
        self,
        agent_ids: list[str],
        work: WorkSpec
    ) -> dict[str, MLPrediction]:
        """Get ML predictions for outcome success."""
        predictions = {}

        for agent_id in agent_ids:
            try:
                pred = await self.trust_scoring.predict_success(
                    agent_id=agent_id,
                    category=work.category,
                    criteria=[c.metric for c in work.success_criteria]
                )
                predictions[agent_id] = pred
            except Exception as e:
                # Fallback to no prediction
                predictions[agent_id] = None

        return predictions

    async def _score_bid(
        self,
        bid: BidPacket,
        work: WorkSpec,
        trust_scores: dict[str, TrustScore],
        ml_predictions: dict[str, MLPrediction]
    ) -> ScoredBid:
        # Base scores (Phase A)
        price_score = self._calculate_price_score(bid.price, work.budget.max_price)
        trust_score = self._calculate_trust_score(
            bid.provider_id,
            trust_scores,
            ml_predictions.get(bid.agent_id)
        )
        quality_score = self._calculate_quality_score(bid)

        # Phase B: Expected value calculation for CPA bids
        ev_score = 0.0
        expected_cost = bid.price
        cost_breakdown = None

        if work.cpa_enabled and bid.cpa_acceptance:
            cost_breakdown = self._calculate_expected_cost(
                bid, work, trust_scores, ml_predictions
            )
            expected_cost = cost_breakdown.expected_total
            max_cost = work.budget.max_price + work.budget.max_cpa_bonus
            ev_score = (max_cost - expected_cost) / max_cost

        # Weighted final score
        if work.cpa_enabled:
            # Phase B weights
            final_score = (
                0.25 * price_score +
                0.25 * trust_score +
                0.20 * quality_score +
                0.30 * ev_score
            )
        else:
            # Phase A weights
            final_score = (
                0.35 * price_score +
                0.35 * trust_score +
                0.30 * quality_score
            )

        return ScoredBid(
            bid_id=bid.bid_id,
            provider_id=bid.provider_id,
            agent_id=bid.agent_id,
            price=bid.price,
            score=final_score,
            score_breakdown=ScoreBreakdown(
                price_score=price_score,
                trust_score=trust_score,
                quality_score=quality_score,
                ev_score=ev_score
            ),
            expected_cost=expected_cost,
            cost_breakdown=cost_breakdown,
            cpa_enabled=bid.cpa_acceptance is not None
        )

    def _calculate_expected_cost(
        self,
        bid: BidPacket,
        work: WorkSpec,
        trust_scores: dict,
        ml_predictions: dict
    ) -> CostBreakdown:
        """Calculate expected cost including CPA bonuses/penalties."""
        base_price = bid.price
        cpa = bid.cpa_acceptance
        ml_pred = ml_predictions.get(bid.agent_id)

        # Calculate expected bonus
        expected_bonus = 0.0
        criteria_predictions = []

        for criterion in work.success_criteria:
            # Find corresponding guarantee
            guarantee = next(
                (g for g in cpa.criteria_guarantees if g.metric == criterion.metric),
                None
            )

            # Calculate P(criterion_met)
            if guarantee:
                provider_confidence = guarantee.confidence
            else:
                provider_confidence = bid.confidence * 0.8  # Discount if no guarantee

            ml_confidence = ml_pred.criteria_success.get(criterion.metric, 0.7) if ml_pred else 0.7

            # Weighted combination
            p_met = 0.4 * provider_confidence + 0.6 * ml_confidence

            criteria_predictions.append({
                "metric": criterion.metric,
                "p_met": p_met,
                "bonus_if_met": criterion.bonus or 0
            })

            if criterion.bonus:
                expected_bonus += criterion.bonus * p_met

        # Calculate expected penalty
        expected_penalty = 0.0
        if cpa.accept_penalty:
            # P(failure) = 1 - P(all required criteria met)
            required_success_probs = [
                cp["p_met"] for cp in criteria_predictions
                if next(
                    (c for c in work.success_criteria if c.metric == cp["metric"]),
                    None
                ).required
            ]

            if required_success_probs:
                p_all_required_met = 1.0
                for p in required_success_probs:
                    p_all_required_met *= p

                p_failure = 1 - p_all_required_met
                penalty_rate = min(cpa.max_penalty_accepted, work.cpa_terms.max_penalty_rate)
                expected_penalty = base_price * penalty_rate * p_failure

        expected_total = base_price + expected_bonus - expected_penalty

        return CostBreakdown(
            base_price=base_price,
            expected_bonus=expected_bonus,
            expected_penalty=expected_penalty,
            expected_total=expected_total,
            criteria_predictions=criteria_predictions
        )

    def _calculate_trust_score(
        self,
        provider_id: str,
        trust_scores: dict,
        ml_prediction: Optional[MLPrediction]
    ) -> float:
        historical = trust_scores.get(provider_id, TrustScore(score=0.5))

        if ml_prediction:
            # Phase B: Combine historical and ML
            return 0.6 * historical.score + 0.4 * ml_prediction.overall_success_rate
        else:
            # Phase A: Historical only
            return historical.score
```

## Data Models

```python
class ScoredBid(BaseModel):
    bid_id: str
    provider_id: str
    agent_id: str
    price: float
    score: float
    rank: int = 0
    score_breakdown: ScoreBreakdown
    expected_cost: float
    cost_breakdown: Optional[CostBreakdown]
    cpa_enabled: bool

class ScoreBreakdown(BaseModel):
    price_score: float
    trust_score: float
    quality_score: float
    ev_score: float              # NEW: Expected value score

class CostBreakdown(BaseModel):     # NEW
    base_price: float
    expected_bonus: float
    expected_penalty: float
    expected_total: float
    criteria_predictions: list[dict]

class MLPrediction(BaseModel):       # NEW
    agent_id: str
    overall_success_rate: float
    criteria_success: dict[str, float]  # metric -> P(success)
    confidence: float
    model_version: str

class EvaluationResult(BaseModel):
    work_id: str
    status: str
    ranked_bids: list[ScoredBid]
    winning_bid_id: Optional[str]
    evaluation_metadata: dict
```

## Events

### bids.evaluated (Enhanced)

```json
{
  "event_type": "bids.evaluated",
  "event_id": "evt_abc123",
  "work_id": "work_550e8400",
  "evaluation_id": "eval_def456",
  "status": "EVALUATED",
  "total_bids": 5,
  "cpa_bids": 3,                           // NEW
  "ml_predictions_used": true,             // NEW
  "ranked_bids": [
    {
      "bid_id": "bid_abc123",
      "provider_id": "prov_expedia",
      "agent_id": "agent_xyz789",
      "rank": 1,
      "score": 0.92,
      "price": 0.08,
      "expected_cost": 0.12,               // NEW
      "score_breakdown": {
        "price_score": 0.47,
        "trust_score": 0.94,
        "quality_score": 0.90,
        "ev_score": 0.85                   // NEW
      }
    }
  ],
  "winning_bid_id": "bid_abc123",
  "timestamp": "2025-01-15T10:30:35Z"
}
```

## API Enhancements

### Internal: Get Evaluation Details

```
GET /internal/v1/evaluations/{work_id}
Authorization: Bearer {service_token}

Response:
{
  "work_id": "work_550e8400",
  "evaluation_id": "eval_def456",
  "status": "EVALUATED",
  "evaluated_at": "2025-01-15T10:30:35Z",
  "config": {
    "weights": {
      "price": 0.25,
      "trust": 0.25,
      "quality": 0.20,
      "ev": 0.30
    },
    "ml_predictions_enabled": true
  },
  "ranked_bids": [
    {
      "bid_id": "bid_abc123",
      "rank": 1,
      "score": 0.92,
      "price": 0.08,
      "expected_cost": 0.12,
      "score_breakdown": {...},
      "cost_breakdown": {
        "base_price": 0.08,
        "expected_bonus": 0.045,
        "expected_penalty": 0.005,
        "expected_total": 0.12,
        "criteria_predictions": [
          {
            "metric": "booking_confirmed",
            "p_met": 0.92,
            "bonus_if_met": 0.05
          },
          {
            "metric": "response_time_ms",
            "p_met": 0.85,
            "bonus_if_met": 0.02
          }
        ]
      }
    }
  ]
}
```

## Integration with Trust Scoring

```python
class TrustScoringClient:
    async def predict_success(
        self,
        agent_id: str,
        category: str,
        criteria: list[str]
    ) -> MLPrediction:
        """Get ML prediction for agent's success probability."""
        response = await self.http.post(
            f"{self.base_url}/internal/v1/predictions/task-success",
            json={
                "agent_id": agent_id,
                "category": category,
                "criteria_metrics": criteria
            }
        )

        data = response.json()
        return MLPrediction(
            agent_id=agent_id,
            overall_success_rate=data["predicted_success_rate"],
            criteria_success=data["criteria_predictions"],
            confidence=data["confidence"],
            model_version=data["model_version"]
        )
```

## Configuration

```yaml
# config/bid-evaluator.yaml (Phase B additions)
bid_evaluator:
  # Phase A config unchanged...

  # Phase B additions
  scoring:
    cpa_weights:
      price: 0.25
      trust: 0.25
      quality: 0.20
      expected_value: 0.30

    non_cpa_weights:
      price: 0.35
      trust: 0.35
      quality: 0.30

  ml_integration:
    enabled: true
    trust_scoring_timeout_ms: 500
    fallback_on_timeout: true
    fallback_success_rate: 0.7

  expected_value:
    provider_confidence_weight: 0.4
    ml_confidence_weight: 0.6
    no_guarantee_discount: 0.8
```

## Metrics

```python
# Phase B metrics
evaluation_ml_predictions = Counter(
    "evaluation_ml_predictions_total",
    "Evaluations using ML predictions",
    ["status"]  # success, timeout, error
)

evaluation_ev_score = Histogram(
    "evaluation_ev_score",
    "Expected value scores distribution",
    buckets=[0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0]
)

evaluation_cpa_bid_ratio = Gauge(
    "evaluation_cpa_bid_ratio",
    "Ratio of CPA bids to total bids"
)

prediction_accuracy = Gauge(
    "evaluation_prediction_accuracy",
    "Accuracy of EV predictions vs actual outcomes",
    ["metric"]
)
```

## Backward Compatibility

1. **Non-CPA Works**: Uses Phase A scoring weights, ev_score=0
2. **Non-CPA Bids**: Evaluated normally, no cost breakdown
3. **ML Timeout**: Falls back to historical trust only
4. **Event Compatibility**: New fields added, consumers ignore unknown
