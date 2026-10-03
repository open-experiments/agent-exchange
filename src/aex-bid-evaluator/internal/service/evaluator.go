package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/clients"
	"github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/model"
	"github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/store"
	"github.com/parlakisik/agent-exchange/internal/httpclient"
)

type Service struct {
	bidGateway    *clients.BidGatewayClient
	trustBroker   *clients.TrustBrokerClient
	certAuth      *clients.CertAuthClient
	workPublisher *clients.WorkPublisherClient // nil when WORK_PUBLISHER_URL is unset
	store         store.EvaluationStore
}

func New(bidGatewayURL string, trustBrokerURL string, certAuthURL string, workPublisherURL string, st store.EvaluationStore) (*Service, error) {
	if strings.TrimSpace(bidGatewayURL) == "" {
		return nil, errors.New("BID_GATEWAY_URL is required")
	}
	svc := &Service{
		bidGateway:  clients.NewBidGatewayClient(bidGatewayURL),
		trustBroker: clients.NewTrustBrokerClient(trustBrokerURL),
		certAuth:    clients.NewCertAuthClient(certAuthURL),
		store:       st,
	}
	if strings.TrimSpace(workPublisherURL) != "" {
		svc.workPublisher = clients.NewWorkPublisherClient(workPublisherURL)
	}
	return svc, nil
}

func (s *Service) HandleEvaluate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "bad request")
		return
	}
	defer func() { _ = r.Body.Close() }()

	var req model.EvaluateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "bad request")
		return
	}
	req.WorkID = strings.TrimSpace(req.WorkID)
	if req.WorkID == "" {
		respondError(w, http.StatusBadRequest, "WORK_ID_REQUIRED", "work_id is required")
		return
	}
	if req.Budget != nil && req.Budget.MaxPrice < 0 {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "budget.max_price must not be negative")
		return
	}

	work := model.WorkSpec{
		WorkID:      req.WorkID,
		Budget:      model.WorkBudget{MaxPrice: 0, BidStrategy: "balanced"},
		Constraints: model.WorkConstraints{},
	}

	// Callers that supply budget.max_price keep the request-only path. Otherwise
	// (absent or 0) the work spec is fetched from work-publisher and request
	// fields override it.
	if (req.Budget == nil || req.Budget.MaxPrice == 0) && s.workPublisher != nil {
		fetched, err := s.workPublisher.GetWork(ctx, req.WorkID)
		if err != nil {
			var httpErr *httpclient.HTTPError
			if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
				respondError(w, http.StatusNotFound, "WORK_NOT_FOUND", "work not found")
				return
			}
			slog.ErrorContext(ctx, "failed to fetch work from work-publisher",
				"work_id", req.WorkID,
				"error", err,
			)
			respondError(w, http.StatusBadGateway, "BAD_GATEWAY", "failed to fetch work")
			return
		}
		work = workSpecFromPublisher(req.WorkID, fetched)
	}
	applyRequestOverrides(&work, req)

	if work.Budget.MaxPrice <= 0 {
		msg := "budget.max_price is required"
		if s.workPublisher == nil {
			msg = "budget.max_price is required when WORK_PUBLISHER_URL is not configured"
		}
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", msg)
		return
	}

	ev, err := s.evaluate(ctx, work)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// workSpecFromPublisher converts a work-publisher work spec into the subset
// the evaluator scores against.
func workSpecFromPublisher(workID string, w *clients.WorkSpec) model.WorkSpec {
	strategy := w.Budget.BidStrategy
	if strategy == "" {
		strategy = "balanced"
	}
	return model.WorkSpec{
		WorkID: workID,
		Budget: model.WorkBudget{
			MaxPrice:    w.Budget.MaxPrice,
			BidStrategy: strategy,
		},
		Constraints: model.WorkConstraints{
			MaxLatencyMs: w.Constraints.MaxLatencyMs,
		},
		Description: w.Description,
	}
}

// applyRequestOverrides lets explicit request fields win over the base work
// spec. A supplied budget replaces max_price/bid_strategy only where the
// request sets them, so a caller may override just the strategy.
func applyRequestOverrides(work *model.WorkSpec, req model.EvaluateRequest) {
	if req.Budget != nil {
		if req.Budget.MaxPrice > 0 {
			work.Budget.MaxPrice = req.Budget.MaxPrice
		}
		if req.Budget.BidStrategy != "" {
			work.Budget.BidStrategy = req.Budget.BidStrategy
		}
	}
	if req.Constraints != nil {
		work.Constraints = *req.Constraints
	}
	if req.Description != nil {
		work.Description = *req.Description
	}
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
		trust, err := s.trustBroker.GetScore(ctx, bid.ProviderID)
		if err != nil {
			slog.WarnContext(ctx, "failed to fetch trust score, skipping trust component",
				"provider_id", bid.ProviderID,
				"error", err,
			)
			// Leave trust at 0 so the trust component contributes nothing
			// rather than silently assuming a default score.
			trust = 0
		}

		certScore, err := s.certAuth.GetCertScore(ctx, bid.ProviderID)
		if err != nil {
			slog.WarnContext(ctx, "failed to get cert score",
				"provider_id", bid.ProviderID,
				"error", err,
			)
			certScore = 0 // graceful degradation
		}

		priceScore := clamp01(1 - (bid.Price / work.Budget.MaxPrice))
		confScore := clamp01(bid.Confidence)
		mvpScore := 0.5 // no sample provided
		if bid.MVPSample != nil {
			mvpScore = 0.8 // sample provided, gets scoring boost
		}
		slaScore := calculateSLAScore(bid.SLA, work.Constraints)

		scr := model.BidScore{
			Price:         priceScore,
			Trust:         clamp01(trust),
			Confidence:    confScore,
			MVPSample:     clamp01(mvpScore),
			SLA:           clamp01(slaScore),
			Certification: clamp01(certScore),
		}
		total := weights.Price*scr.Price +
			weights.Trust*scr.Trust +
			weights.Confidence*scr.Confidence +
			weights.MVPSample*scr.MVPSample +
			weights.SLA*scr.SLA +
			weights.Certification*scr.Certification
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
	Price         float64
	Trust         float64
	Confidence    float64
	MVPSample     float64
	SLA           float64
	Certification float64
}

func weightsForStrategy(strategy string) strategyWeights {
	switch strategy {
	case "lowest_price":
		return strategyWeights{Price: 0.45, Trust: 0.15, Confidence: 0.1, MVPSample: 0.05, SLA: 0.1, Certification: 0.15}
	case "best_quality":
		return strategyWeights{Price: 0.1, Trust: 0.3, Confidence: 0.2, MVPSample: 0.1, SLA: 0.1, Certification: 0.2}
	default:
		return strategyWeights{Price: 0.25, Trust: 0.25, Confidence: 0.15, MVPSample: 0.1, SLA: 0.1, Certification: 0.15}
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

func respondError(w http.ResponseWriter, statusCode int, code string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":      code,
			"message":   message,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	})
}

func generateEvalID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "eval_" + hex.EncodeToString(b[:])
}
