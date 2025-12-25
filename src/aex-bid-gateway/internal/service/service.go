package service

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/parlakisik/agent-exchange/aex-bid-gateway/internal/model"
	"github.com/parlakisik/agent-exchange/aex-bid-gateway/internal/store"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrBadRequest   = errors.New("bad_request")
	ErrInvalidBid   = errors.New("invalid_bid")
)

type Service struct {
	store store.BidStore

	// apiKey -> providerID
	providerKeys map[string]string
}

func New(store store.BidStore, providerKeys map[string]string) *Service {
	return &Service{
		store:        store,
		providerKeys: providerKeys,
	}
}

func (s *Service) HandleSubmitBid(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	providerID, err := s.validateProviderAuth(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req model.SubmitBidRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid bid format", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	bid := model.BidPacket{
		BidID:            generateBidID(),
		WorkID:           req.WorkID,
		ProviderID:       providerID,
		Price:            req.Price,
		PriceBreakdown:   req.PriceBreakdown,
		Confidence:       req.Confidence,
		Approach:         req.Approach,
		EstimatedLatency: req.EstimatedLatency,
		MVPSample:        req.MVPSample,
		SLA:              req.SLA,
		A2AEndpoint:      req.A2AEndpoint,
		ExpiresAt:        req.ExpiresAt,
		ReceivedAt:       now,
	}

	if err := validateBid(now, bid); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.store.Save(ctx, bid); err != nil {
		http.Error(w, "Failed to store bid", http.StatusInternalServerError)
		return
	}

	resp := model.SubmitBidResponse{
		BidID:      bid.BidID,
		WorkID:     bid.WorkID,
		Status:     "RECEIVED",
		ReceivedAt: bid.ReceivedAt,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Service) HandleInternalListBids(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workID := strings.TrimSpace(r.URL.Query().Get("work_id"))
	if workID == "" {
		http.Error(w, "work_id is required", http.StatusBadRequest)
		return
	}

	bids, err := s.store.ListByWorkID(ctx, workID)
	if err != nil {
		http.Error(w, "Failed to load bids", http.StatusInternalServerError)
		return
	}

	out := map[string]any{
		"work_id":    workID,
		"bids":       bids,
		"total_bids": len(bids),
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Service) validateProviderAuth(r *http.Request) (string, error) {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(auth, "Bearer ") {
		return "", ErrUnauthorized
	}
	apiKey := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	if apiKey == "" {
		return "", ErrUnauthorized
	}
	if providerID, ok := s.providerKeys[apiKey]; ok && providerID != "" {
		return providerID, nil
	}
	return "", ErrUnauthorized
}

func validateBid(now time.Time, bid model.BidPacket) error {
	if bid.WorkID == "" || bid.Price <= 0 || bid.A2AEndpoint == "" {
		return errors.New("missing required fields")
	}
	if bid.Confidence < 0 || bid.Confidence > 1 {
		return errors.New("confidence must be between 0 and 1")
	}
	if bid.ExpiresAt.IsZero() {
		return errors.New("expires_at is required")
	}
	if bid.ExpiresAt.Before(now) {
		return errors.New("bid already expired")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func generateBidID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "bid_" + hex.EncodeToString(b[:8])
}
