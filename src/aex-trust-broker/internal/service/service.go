package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/parlakisik/agent-exchange/aex-trust-broker/internal/model"
	"github.com/parlakisik/agent-exchange/aex-trust-broker/internal/store"
)

type Service struct {
	store store.Store
}

func New(st store.Store) *Service {
	return &Service{store: st}
}

func (s *Service) HandleGetTrust(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	providerID := pathParam(r.URL.Path, "/v1/providers/", "/trust")
	if providerID == "" {
		http.Error(w, "provider_id is required", http.StatusBadRequest)
		return
	}

	rec, err := s.store.GetTrustRecord(ctx, providerID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if rec == nil {
		now := time.Now().UTC()
		r := model.TrustRecord{
			ProviderID:   providerID,
			TrustScore:   0.3,
			BaseScore:    0.3,
			TrustTier:    model.TrustTierUnverified,
			RegisteredAt: now,
			LastUpdated:  now,
		}
		_ = s.store.UpsertTrustRecord(ctx, r)
		rec = &r
	}

	writeJSON(w, http.StatusOK, rec)
}

func (s *Service) HandleBatchTrust(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req model.BatchTrustRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	out := model.BatchTrustResponse{Scores: map[string]float64{}}
	for _, id := range req.ProviderIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		rec, err := s.store.GetTrustRecord(ctx, id)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if rec == nil {
			out.Scores[id] = 0.3
		} else {
			out.Scores[id] = rec.TrustScore
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Service) HandleRecordOutcome(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var out model.ContractOutcome
	if err := decodeJSON(r, &out); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if out.ProviderID == "" || out.ContractID == "" || out.Outcome == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}
	if out.ID == "" {
		out.ID = generateID("out_")
	}
	if out.RecordedAt.IsZero() {
		out.RecordedAt = time.Now().UTC()
	}
	if out.CompletedAt.IsZero() {
		out.CompletedAt = out.RecordedAt
	}

	if err := s.store.SaveOutcome(ctx, out); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	updated, prevScore, prevTier, err := s.recalculate(ctx, out.ProviderID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	resp := map[string]any{
		"recorded":       true,
		"provider_id":    out.ProviderID,
		"previous_score": prevScore,
		"new_score":      updated.TrustScore,
		"tier_changed":   prevTier != updated.TrustTier,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Service) recalculate(ctx context.Context, providerID string) (model.TrustRecord, float64, model.TrustTier, error) {
	now := time.Now().UTC()
	rec, err := s.store.GetTrustRecord(ctx, providerID)
	if err != nil {
		return model.TrustRecord{}, 0, "", err
	}
	if rec == nil {
		r := model.TrustRecord{
			ProviderID:   providerID,
			TrustScore:   0.3,
			BaseScore:    0.3,
			TrustTier:    model.TrustTierUnverified,
			RegisteredAt: now,
			LastUpdated:  now,
		}
		rec = &r
	}

	prevScore := rec.TrustScore
	prevTier := rec.TrustTier

	outcomes, err := s.store.ListOutcomes(ctx, providerID, 200)
	if err != nil {
		return model.TrustRecord{}, 0, "", err
	}
	base := calculateWeightedScore(outcomes)
	mod := 0.0
	if rec.IdentityVerified {
		mod += 0.05
	}
	if rec.EndpointVerified {
		mod += 0.05
	}
	tenureMonths := monthsSince(rec.RegisteredAt, now)
	if tenureMonths > 5 {
		tenureMonths = 5
	}
	mod += float64(tenureMonths) * 0.02

	final := clamp01(base + mod)
	rec.BaseScore = base
	rec.TrustScore = final
	rec.LastUpdated = now

	// derive stats from outcomes
	rec.TotalContracts = len(outcomes)
	rec.SuccessfulContracts, rec.FailedContracts, rec.DisputedContracts = 0, 0, 0
	for _, o := range outcomes {
		switch o.Outcome {
		case model.OutcomeSuccess, model.OutcomeSuccessPartial:
			rec.SuccessfulContracts++
		case model.OutcomeDisputeWon, model.OutcomeDisputeLost:
			rec.DisputedContracts++
			if o.Outcome == model.OutcomeDisputeWon {
				rec.DisputesWon++
			} else {
				rec.DisputesLost++
			}
		case model.OutcomeFailureProvider, model.OutcomeFailureExternal, model.OutcomeFailureConsumer, model.OutcomeExpired:
			rec.FailedContracts++
		}
	}
	if len(outcomes) > 0 {
		t := outcomes[0].CompletedAt
		rec.LastContractAt = &t
	}

	rec.TrustTier = determineTier(rec.TrustScore, rec.TrustTier, rec.TotalContracts)

	if err := s.store.UpsertTrustRecord(ctx, *rec); err != nil {
		return model.TrustRecord{}, 0, "", err
	}
	return *rec, prevScore, prevTier, nil
}

func determineTier(score float64, current model.TrustTier, total int) model.TrustTier {
	if current == model.TrustTierInternal {
		return model.TrustTierInternal
	}
	switch {
	case score >= 0.9 && total >= 100:
		return model.TrustTierPreferred
	case score >= 0.7 && total >= 25:
		return model.TrustTierTrusted
	case score >= 0.5 && total >= 5:
		return model.TrustTierVerified
	default:
		return model.TrustTierUnverified
	}
}

func calculateWeightedScore(outcomes []model.ContractOutcome) float64 {
	if len(outcomes) == 0 {
		return 0.3
	}
	weightedSum := 0.0
	weightSum := 0.0
	for i, o := range outcomes {
		weight := 0.1
		switch {
		case i < 10:
			weight = 1.0
		case i < 50:
			weight = 0.5
		case i < 100:
			weight = 0.25
		}
		score := outcomeToScore(o.Outcome)
		weightedSum += score * weight
		weightSum += weight
	}
	if weightSum == 0 {
		return 0.3
	}
	return weightedSum / weightSum
}

func outcomeToScore(out model.OutcomeType) float64 {
	switch out {
	case model.OutcomeSuccess:
		return 1.0
	case model.OutcomeSuccessPartial:
		return 0.7
	case model.OutcomeFailureProvider:
		return 0.0
	case model.OutcomeFailureExternal:
		return 0.5
	case model.OutcomeFailureConsumer:
		return 0.8
	case model.OutcomeDisputeWon:
		return 0.8
	case model.OutcomeDisputeLost:
		return 0.0
	case model.OutcomeExpired:
		return 0.2
	default:
		return 0.5
	}
}

func monthsSince(t time.Time, now time.Time) int {
	d := now.Sub(t)
	if d <= 0 {
		return 0
	}
	return int(d.Hours() / (24 * 30))
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

func decodeJSON(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	defer func() { _ = r.Body.Close() }()
	return json.Unmarshal(body, v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func generateID(prefix string) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:8])
}

func pathParam(path string, prefix string, suffix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(path, prefix)
	if suffix != "" {
		if !strings.HasSuffix(rest, suffix) {
			return ""
		}
		rest = strings.TrimSuffix(rest, suffix)
	}
	rest = strings.Trim(rest, "/")
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	return strings.TrimSpace(rest)
}
