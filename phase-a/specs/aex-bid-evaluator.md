# aex-bid-evaluator Service Specification

## Overview

**Purpose:** Score and rank bids based on price, trust, confidence, and MVP sample quality. This is the "brain" of bid selection.

**Language:** Python 3.11+
**Framework:** FastAPI + Event-driven (Pub/Sub)
**Runtime:** Cloud Run
**Port:** 8080

## Architecture Position

```
        Pub/Sub
   (bid_window_closed)
          │
          ▼
 ┌─────────────────────┐
 │  aex-bid-evaluator  │◄── THIS SERVICE
 │                     │
 │ • Fetch bids        │
 │ • Score each bid    │
 │ • Rank by strategy  │
 │ • LLM evaluation    │
 └──────────┬──────────┘
            │
   ┌────────┼────────┐
   ▼        ▼        ▼
Bid      Trust     Contract
Gateway  Broker    Engine
```

## Core Responsibilities

1. **Bid Scoring** - Calculate composite score for each bid
2. **Trust Integration** - Fetch provider trust scores
3. **MVP Sample Evaluation** - Assess proof of competence
4. **Strategy Ranking** - Rank by consumer's bid strategy
5. **LLM Integration** - Use LLM for semantic evaluation (optional)

## Scoring Algorithm

```
┌─────────────────────────────────────────────────────────────────┐
│                    BID SCORING ALGORITHM                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  INPUTS:                                                        │
│  - Bid packet (price, confidence, MVP sample, SLA)              │
│  - Work spec (budget, criteria, strategy)                       │
│  - Provider trust score (from Trust Broker)                     │
│                                                                 │
│  SCORE COMPONENTS:                                              │
│                                                                 │
│  1. Price Score (0-1)                                           │
│     └─► price_score = 1 - (bid_price / max_price)               │
│                                                                 │
│  2. Trust Score (0-1)                                           │
│     └─► From Trust Broker (rolling reputation)                  │
│                                                                 │
│  3. Confidence Score (0-1)                                      │
│     └─► Direct from bid.confidence                              │
│                                                                 │
│  4. MVP Sample Score (0-1)                                      │
│     └─► LLM evaluation of sample quality                        │
│                                                                 │
│  5. SLA Score (0-1)                                             │
│     └─► How well SLA matches requirements                       │
│                                                                 │
│  WEIGHTED TOTAL by strategy:                                    │
│  - lowest_price:  0.5*price + 0.2*trust + 0.1*conf + 0.1*mvp + 0.1*sla
│  - best_quality:  0.1*price + 0.4*trust + 0.2*conf + 0.2*mvp + 0.1*sla
│  - balanced:      0.3*price + 0.3*trust + 0.15*conf + 0.15*mvp + 0.1*sla
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## API Endpoints

### Evaluate Bids (Internal)

#### POST /internal/v1/evaluate

Evaluate bids for a work spec.

```json
// Request
{
  "work_id": "work_550e8400"
}

// Response
{
  "work_id": "work_550e8400",
  "evaluation_id": "eval_xyz789",
  "total_bids": 5,
  "valid_bids": 4,
  "ranked_bids": [
    {
      "rank": 1,
      "bid_id": "bid_def456",
      "provider_id": "prov_booking",
      "total_score": 0.87,
      "scores": {
        "price": 0.85,
        "trust": 0.91,
        "confidence": 0.88,
        "mvp_sample": 0.82,
        "sla": 0.90
      }
    },
    {
      "rank": 2,
      "bid_id": "bid_abc123",
      "provider_id": "prov_expedia",
      "total_score": 0.79,
      "scores": {...}
    }
  ],
  "disqualified_bids": [
    {
      "bid_id": "bid_ghi789",
      "reason": "Price exceeds budget"
    }
  ],
  "evaluated_at": "2025-01-15T10:31:00Z"
}
```

## Core Functions

### Bid Evaluation

```python
async def evaluate_bids(work_id: str) -> BidEvaluation:
    # 1. Fetch work spec
    work = await work_publisher.get_work(work_id)

    # 2. Fetch all bids
    bids = await bid_gateway.get_bids(work_id)

    # 3. Filter valid bids
    valid_bids, disqualified = filter_valid_bids(bids, work)

    # 4. Score each valid bid
    scored_bids = []
    for bid in valid_bids:
        score = await score_bid(bid, work)
        scored_bids.append(ScoredBid(bid=bid, score=score))

    # 5. Rank by strategy
    ranked = rank_bids(scored_bids, work.budget.bid_strategy)

    # 6. Store evaluation
    evaluation = BidEvaluation(
        work_id=work_id,
        ranked_bids=ranked,
        disqualified_bids=disqualified,
        evaluated_at=datetime.utcnow()
    )
    await firestore.save_evaluation(evaluation)

    # 7. Publish evaluation complete
    await pubsub.publish("bids.evaluated", {
        "work_id": work_id,
        "evaluation_id": evaluation.id,
        "winner_bid_id": ranked[0].bid.bid_id if ranked else None
    })

    return evaluation

def filter_valid_bids(bids: list[BidPacket], work: WorkSpec) -> tuple[list, list]:
    valid = []
    disqualified = []

    for bid in bids:
        # Check price within budget
        if bid.price > work.budget.max_price:
            disqualified.append(DisqualifiedBid(
                bid_id=bid.bid_id,
                reason="Price exceeds budget"
            ))
            continue

        # Check not expired
        if bid.expires_at < datetime.utcnow():
            disqualified.append(DisqualifiedBid(
                bid_id=bid.bid_id,
                reason="Bid expired"
            ))
            continue

        # Check SLA meets requirements
        if work.constraints.max_latency_ms:
            if bid.sla.max_latency_ms > work.constraints.max_latency_ms:
                disqualified.append(DisqualifiedBid(
                    bid_id=bid.bid_id,
                    reason="SLA does not meet latency requirements"
                ))
                continue

        valid.append(bid)

    return valid, disqualified
```

### Bid Scoring

```python
async def score_bid(bid: BidPacket, work: WorkSpec) -> BidScore:
    # 1. Price score (inverse - lower price = higher score)
    price_score = 1 - (bid.price / work.budget.max_price)

    # 2. Trust score from Trust Broker
    trust_score = await trust_broker.get_score(bid.provider_id)

    # 3. Confidence score (direct)
    confidence_score = bid.confidence

    # 4. MVP sample score (LLM evaluation)
    mvp_score = 0.5  # Default if no sample
    if bid.mvp_sample:
        mvp_score = await evaluate_mvp_sample(bid.mvp_sample, work)

    # 5. SLA score
    sla_score = calculate_sla_score(bid.sla, work.constraints)

    return BidScore(
        price=price_score,
        trust=trust_score,
        confidence=confidence_score,
        mvp_sample=mvp_score,
        sla=sla_score
    )

async def evaluate_mvp_sample(sample: MVPSample, work: WorkSpec) -> float:
    """Use LLM to evaluate MVP sample quality."""
    prompt = f"""
    Evaluate this provider's sample work for the following task:

    TASK: {work.description}

    SAMPLE INPUT: {sample.sample_input}
    SAMPLE OUTPUT: {sample.sample_output}
    LATENCY: {sample.sample_latency_ms}ms

    SUCCESS CRITERIA: {work.success_criteria}

    Score from 0.0 to 1.0 how well this sample demonstrates:
    1. Understanding of the task
    2. Quality of the output
    3. Relevance to the actual work
    4. Completeness

    Return only a number between 0.0 and 1.0.
    """

    response = await llm_client.complete(prompt)
    try:
        score = float(response.strip())
        return max(0.0, min(1.0, score))
    except:
        return 0.5  # Default on parse error
```

### Ranking by Strategy

```python
def rank_bids(scored_bids: list[ScoredBid], strategy: str) -> list[RankedBid]:
    # Define weights by strategy
    weights = {
        "lowest_price": {
            "price": 0.5, "trust": 0.2, "confidence": 0.1,
            "mvp_sample": 0.1, "sla": 0.1
        },
        "best_quality": {
            "price": 0.1, "trust": 0.4, "confidence": 0.2,
            "mvp_sample": 0.2, "sla": 0.1
        },
        "balanced": {
            "price": 0.3, "trust": 0.3, "confidence": 0.15,
            "mvp_sample": 0.15, "sla": 0.1
        }
    }

    w = weights.get(strategy, weights["balanced"])

    # Calculate total score for each bid
    for sb in scored_bids:
        sb.total_score = (
            w["price"] * sb.score.price +
            w["trust"] * sb.score.trust +
            w["confidence"] * sb.score.confidence +
            w["mvp_sample"] * sb.score.mvp_sample +
            w["sla"] * sb.score.sla
        )

    # Sort by total score descending
    scored_bids.sort(key=lambda x: x.total_score, reverse=True)

    # Create ranked list
    return [
        RankedBid(
            rank=i + 1,
            bid_id=sb.bid.bid_id,
            provider_id=sb.bid.provider_id,
            total_score=sb.total_score,
            scores=sb.score
        )
        for i, sb in enumerate(scored_bids)
    ]
```

## Events

### Consumed Events

```json
// Bid window closed - triggers evaluation
{
  "event_type": "work.bid_window_closed",
  "work_id": "work_550e8400",
  "bids_received": 5
}
```

### Published Events

```json
// Evaluation complete
{
  "event_type": "bids.evaluated",
  "work_id": "work_550e8400",
  "evaluation_id": "eval_xyz789",
  "winner_bid_id": "bid_def456",
  "total_bids": 5,
  "valid_bids": 4
}
```

## Configuration

```bash
# Server
PORT=8080
ENV=production

# Bid Gateway
BID_GATEWAY_URL=https://aex-bid-gateway-xxx.run.app

# Work Publisher
WORK_PUBLISHER_URL=https://aex-work-publisher-xxx.run.app

# Trust Broker
TRUST_BROKER_URL=https://aex-trust-broker-xxx.run.app

# LLM (for MVP evaluation)
LLM_PROVIDER=vertex_ai
LLM_MODEL=gemini-pro
LLM_ENABLED=true

# Pub/Sub
PUBSUB_PROJECT_ID=aex-prod
PUBSUB_SUBSCRIPTION=aex-bid-evaluator-sub
PUBSUB_TOPIC_EVENTS=aex-bid-events

# Firestore
FIRESTORE_PROJECT_ID=aex-prod

# Observability
LOG_LEVEL=info
```

## Directory Structure

```
aex-bid-evaluator/
├── app/
│   ├── __init__.py
│   ├── main.py
│   ├── config.py
│   ├── models/
│   │   ├── bid.py
│   │   ├── score.py
│   │   └── evaluation.py
│   ├── services/
│   │   ├── evaluator.py
│   │   ├── scorer.py
│   │   ├── ranker.py
│   │   └── mvp_evaluator.py
│   ├── clients/
│   │   ├── bid_gateway.py
│   │   ├── work_publisher.py
│   │   ├── trust_broker.py
│   │   └── llm.py
│   └── events/
│       └── handlers.py
├── tests/
│   ├── test_scorer.py
│   ├── test_ranker.py
│   └── test_mvp_evaluator.py
├── Dockerfile
└── requirements.txt
```
