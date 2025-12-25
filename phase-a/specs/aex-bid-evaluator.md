# aex-bid-evaluator Service Specification

## Overview

**Purpose:** Score and rank bids based on price, trust, confidence, and MVP sample quality. This is the "brain" of bid selection.

**Language:** Go 1.22+
**Framework:** Chi router + Event-driven (Pub/Sub)
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

```go
func (s *Service) EvaluateBids(ctx context.Context, workID string) (BidEvaluation, error) {
	// 1. Fetch work spec
	work, err := s.workPublisher.GetWork(ctx, workID)
	if err != nil {
		return BidEvaluation{}, err
	}

	// 2. Fetch all bids
	bids, err := s.bidGateway.GetBids(ctx, workID)
	if err != nil {
		return BidEvaluation{}, err
	}

	// 3. Filter valid bids
	valid, disqualified := filterValidBids(bids, work, time.Now().UTC())

	// 4. Score each valid bid
	scored := make([]ScoredBid, 0, len(valid))
	for _, bid := range valid {
		score, err := s.scoreBid(ctx, bid, work)
		if err != nil {
			return BidEvaluation{}, err
		}
		scored = append(scored, ScoredBid{Bid: bid, Score: score})
	}

	// 5. Rank by strategy
	ranked := rankBids(scored, work.Budget.BidStrategy)

	// 6. Store evaluation
	eval := BidEvaluation{
		ID:               generateEvaluationID(),
		WorkID:           workID,
		RankedBids:       ranked,
		DisqualifiedBids: disqualified,
		EvaluatedAt:      time.Now().UTC(),
	}
	if err := s.store.SaveEvaluation(ctx, eval); err != nil {
		return BidEvaluation{}, err
	}

	// 7. Publish evaluation complete
	var winner *string
	if len(ranked) > 0 {
		winner = &ranked[0].BidID
	}
	_ = s.events.Publish(ctx, "bids.evaluated", map[string]any{
		"work_id":        workID,
		"evaluation_id":  eval.ID,
		"winner_bid_id":  winner,
		"total_bids":     len(bids),
		"valid_bids":     len(valid),
		"evaluated_at":   eval.EvaluatedAt.Format(time.RFC3339Nano),
	})

	return eval, nil
}

func filterValidBids(bids []BidPacket, work WorkSpec, now time.Time) (valid []BidPacket, disqualified []DisqualifiedBid) {
	for _, bid := range bids {
		if bid.Price > work.Budget.MaxPrice {
			disqualified = append(disqualified, DisqualifiedBid{BidID: bid.BidID, Reason: "Price exceeds budget"})
			continue
		}
		if bid.ExpiresAt.Before(now) {
			disqualified = append(disqualified, DisqualifiedBid{BidID: bid.BidID, Reason: "Bid expired"})
			continue
		}
		if work.Constraints.MaxLatencyMs != nil && bid.SLA.MaxLatencyMs > *work.Constraints.MaxLatencyMs {
			disqualified = append(disqualified, DisqualifiedBid{BidID: bid.BidID, Reason: "SLA does not meet latency requirements"})
			continue
		}
		valid = append(valid, bid)
	}
	return valid, disqualified
}
```

### Bid Scoring

```go
func (s *Service) scoreBid(ctx context.Context, bid BidPacket, work WorkSpec) (BidScore, error) {
	// 1. Price score (inverse - lower price = higher score)
	priceScore := 1 - (bid.Price / work.Budget.MaxPrice)

	// 2. Trust score from Trust Broker
	trustScore, err := s.trustBroker.GetScore(ctx, bid.ProviderID)
	if err != nil {
		return BidScore{}, err
	}

	// 3. Confidence score (direct)
	confidenceScore := bid.Confidence

	// 4. MVP sample score (LLM evaluation)
	mvpScore := 0.5
	if bid.MVPSample != nil && s.llm.Enabled() {
		if v, err := s.evaluateMVPSample(ctx, *bid.MVPSample, work); err == nil {
			mvpScore = v
		}
	}

	// 5. SLA score
	slaScore := calculateSLAScore(bid.SLA, work.Constraints)

	return BidScore{
		Price:      clamp01(priceScore),
		Trust:      clamp01(trustScore),
		Confidence: clamp01(confidenceScore),
		MVPSample:  clamp01(mvpScore),
		SLA:        clamp01(slaScore),
	}, nil
}

func (s *Service) evaluateMVPSample(ctx context.Context, sample MVPSample, work WorkSpec) (float64, error) {
	prompt := buildMVPPrompt(work.Description, sample, work.SuccessCriteria)
	raw, err := s.llm.Complete(ctx, prompt)
	if err != nil {
		return 0.5, err
	}
	score, err := parseScore01(raw)
	if err != nil {
		return 0.5, err
	}
	return score, nil
}
```

### Ranking by Strategy

```go
func rankBids(scored []ScoredBid, strategy string) []RankedBid {
	weights := map[string]map[string]float64{
		"lowest_price": {"price": 0.5, "trust": 0.2, "confidence": 0.1, "mvp_sample": 0.1, "sla": 0.1},
		"best_quality": {"price": 0.1, "trust": 0.4, "confidence": 0.2, "mvp_sample": 0.2, "sla": 0.1},
		"balanced":     {"price": 0.3, "trust": 0.3, "confidence": 0.15, "mvp_sample": 0.15, "sla": 0.1},
	}
	w, ok := weights[strategy]
	if !ok {
		w = weights["balanced"]
	}

	type scoredTotal struct {
		sb    ScoredBid
		total float64
	}
	tmp := make([]scoredTotal, 0, len(scored))
	for _, sb := range scored {
		total := w["price"]*sb.Score.Price +
			w["trust"]*sb.Score.Trust +
			w["confidence"]*sb.Score.Confidence +
			w["mvp_sample"]*sb.Score.MVPSample +
			w["sla"]*sb.Score.SLA
		tmp = append(tmp, scoredTotal{sb: sb, total: total})
	}

	sort.Slice(tmp, func(i, j int) bool { return tmp[i].total > tmp[j].total })

	ranked := make([]RankedBid, 0, len(tmp))
	for i, it := range tmp {
		ranked = append(ranked, RankedBid{
			Rank:       i + 1,
			BidID:      it.sb.Bid.BidID,
			ProviderID: it.sb.Bid.ProviderID,
			TotalScore: it.total,
			Scores:     it.sb.Score,
		})
	}
	return ranked
}
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
├── cmd/
│   └── bid-evaluator/
│       └── main.go
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── model/
│   │   ├── bid.go
│   │   ├── score.go
│   │   └── evaluation.go
│   ├── service/
│   │   ├── evaluator.go
│   │   ├── scorer.go
│   │   ├── ranker.go
│   │   └── mvp.go
│   ├── clients/
│   │   ├── bidgateway.go
│   │   ├── workpublisher.go
│   │   ├── trustbroker.go
│   │   └── llm.go
│   ├── events/
│   │   └── handler.go          # Pub/Sub push handlers
│   └── store/
│       └── firestore.go
├── hack/
│   └── tests/
├── Dockerfile
├── go.mod
└── go.sum
```
