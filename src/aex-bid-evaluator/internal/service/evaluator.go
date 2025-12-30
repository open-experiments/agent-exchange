package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/clients"
	"github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/model"
	"github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/store"
)

type Service struct {
	bidGateway  *clients.BidGatewayClient
	trustBroker *clients.TrustBrokerClient
	store       store.EvaluationStore
}

func New(bidGatewayURL string, trustBrokerURL string, st store.EvaluationStore) (*Service, error) {
	if strings.TrimSpace(bidGatewayURL) == "" {
		return nil, errors.New("BID_GATEWAY_URL is required")
	}
	return &Service{
		bidGateway:  clients.NewBidGatewayClient(bidGatewayURL),
		trustBroker: clients.NewTrustBrokerClient(trustBrokerURL),
		store:       st,
	}, nil
}

func (s *Service) HandleEvaluate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req model.EvaluateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req.WorkID = strings.TrimSpace(req.WorkID)
	if req.WorkID == "" {
		http.Error(w, "work_id is required", http.StatusBadRequest)
		return
	}

	work := model.WorkSpec{
		WorkID:      req.WorkID,
		Budget:      model.WorkBudget{MaxPrice: 0, BidStrategy: "balanced"},
		Constraints: model.WorkConstraints{},
	}
	if req.Budget != nil {
		work.Budget = *req.Budget
	}
	if req.Constraints != nil {
		work.Constraints = *req.Constraints
	}
	if req.Description != nil {
		work.Description = *req.Description
	}
	if work.Budget.MaxPrice <= 0 {
		http.Error(w, "budget.max_price is required (work-publisher not integrated yet)", http.StatusBadRequest)
		return
	}

	ev, err := s.evaluate(ctx, work)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

func (s *Service) evaluate(ctx context.Context, work model.WorkSpec) (model.BidEvaluation, error) {
	bids, err := s.bidGateway.GetBids(ctx, work.WorkID)
	if err != nil {
		return model.BidEvaluation{}, err
	}

	now := time.Now().UTC()
	valid, disq := filterValidBids(bids, work, now)

	weights := weightsForStrategy(work.Budget.BidStrategy)
	type scored struct {
		bid        model.BidPacket
		score      model.BidScore
		totalScore float64
	}
	scoredBids := make([]scored, 0, len(valid))
	for _, bid := range valid {
		trust, _ := s.trustBroker.GetScore(ctx, bid.ProviderID)
		priceScore := clamp01(1 - (bid.Price / work.Budget.MaxPrice))
		confScore := clamp01(bid.Confidence)
		mvpScore := 0.5
		if bid.MVPSample != nil {
			mvpScore = 0.5
		}
		slaScore := calculateSLAScore(bid.SLA, work.Constraints)

		scr := model.BidScore{
			Price:      priceScore,
			Trust:      clamp01(trust),
			Confidence: confScore,
			MVPSample:  clamp01(mvpScore),
			SLA:        clamp01(slaScore),
		}
		total := weights.Price*scr.Price +
			weights.Trust*scr.Trust +
			weights.Confidence*scr.Confidence +
			weights.MVPSample*scr.MVPSample +
			weights.SLA*scr.SLA
		scoredBids = append(scoredBids, scored{bid: bid, score: scr, totalScore: total})
	}

	sort.Slice(scoredBids, func(i, j int) bool { return scoredBids[i].totalScore > scoredBids[j].totalScore })

	ranked := make([]model.RankedBid, 0, len(scoredBids))
	for i, sb := range scoredBids {
		ranked = append(ranked, model.RankedBid{
			Rank:       i + 1,
			BidID:      sb.bid.BidID,
			ProviderID: sb.bid.ProviderID,
			TotalScore: sb.totalScore,
			Scores:     sb.score,
		})
	}

	ev := model.BidEvaluation{
		EvaluationID:     generateEvalID(),
		WorkID:           work.WorkID,
		TotalBids:        len(bids),
		ValidBids:        len(valid),
		RankedBids:       ranked,
		DisqualifiedBids: disq,
		EvaluatedAt:      now,
	}
	_ = s.store.Save(ctx, ev)
	return ev, nil
}

type strategyWeights struct {
	Price      float64
	Trust      float64
	Confidence float64
	MVPSample  float64
	SLA        float64
}

func weightsForStrategy(strategy string) strategyWeights {
	switch strategy {
	case "lowest_price":
		return strategyWeights{Price: 0.5, Trust: 0.2, Confidence: 0.1, MVPSample: 0.1, SLA: 0.1}
	case "best_quality":
		return strategyWeights{Price: 0.1, Trust: 0.4, Confidence: 0.2, MVPSample: 0.2, SLA: 0.1}
	default:
		return strategyWeights{Price: 0.3, Trust: 0.3, Confidence: 0.15, MVPSample: 0.15, SLA: 0.1}
	}
}

func filterValidBids(bids []model.BidPacket, work model.WorkSpec, now time.Time) (valid []model.BidPacket, disq []model.DisqualifiedBid) {
	for _, bid := range bids {
		if bid.Price > work.Budget.MaxPrice {
			disq = append(disq, model.DisqualifiedBid{BidID: bid.BidID, Reason: "Price exceeds budget"})
			continue
		}
		if bid.ExpiresAt.Before(now) {
			disq = append(disq, model.DisqualifiedBid{BidID: bid.BidID, Reason: "Bid expired"})
			continue
		}
		if work.Constraints.MaxLatencyMs != nil && bid.SLA.MaxLatencyMs > *work.Constraints.MaxLatencyMs {
			disq = append(disq, model.DisqualifiedBid{BidID: bid.BidID, Reason: "SLA does not meet latency requirements"})
			continue
		}
		valid = append(valid, bid)
	}
	return valid, disq
}

func calculateSLAScore(sla model.SLACommitment, c model.WorkConstraints) float64 {
	if c.MaxLatencyMs == nil || *c.MaxLatencyMs <= 0 {
		return 0.8
	}
	req := float64(*c.MaxLatencyMs)
	got := float64(sla.MaxLatencyMs)
	if got <= 0 {
		return 0.0
	}
	// 1.0 if within requirement, linearly drop after that.
	if got <= req {
		return 1.0
	}
	over := (got - req) / req
	return clamp01(1.0 - over)
}

func clamp01(v float64) float64 {
	if math.IsNaN(v) {
		return 0.0
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func generateEvalID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "eval_" + hex.EncodeToString(b[:8])
}



