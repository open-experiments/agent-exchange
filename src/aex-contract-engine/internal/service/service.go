package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/parlakisik/agent-exchange/aex-contract-engine/internal/clients"
	"github.com/parlakisik/agent-exchange/aex-contract-engine/internal/model"
	"github.com/parlakisik/agent-exchange/aex-contract-engine/internal/store"
)

// SettlementTimeout bounds one settlement notification, including the
// shared HTTP client's retries. Shutdown waits at least this long for
// in-flight notifications.
const SettlementTimeout = 60 * time.Second

// consumerLookupTimeout bounds the work-publisher lookup during award so an
// unavailable work-publisher cannot hold the award request past the server's
// write timeout.
const consumerLookupTimeout = 5 * time.Second

// unknownConsumerID is recorded when the work's consumer cannot be resolved.
// Contracts with this consumer are never sent to settlement.
const unknownConsumerID = "unknown"

type Service struct {
	store         store.ContractStore
	bg            *clients.BidGatewayClient
	workPublisher *clients.WorkPublisherClient // nil when WORK_PUBLISHER_URL is unset
	settlement    *clients.SettlementClient    // nil when SETTLEMENT_URL is unset

	settlements sync.WaitGroup // in-flight settlement notifications
}

// New builds the service. workPublisherURL is optional: when empty, awarded
// contracts record an unknown consumer. settlementURL is optional: when empty,
// completed contracts are not sent to aex-settlement.
func New(store store.ContractStore, bidGatewayURL string, workPublisherURL string, settlementURL string) (*Service, error) {
	if strings.TrimSpace(bidGatewayURL) == "" {
		return nil, errors.New("BID_GATEWAY_URL is required")
	}
	s := &Service{
		store: store,
		bg:    clients.NewBidGatewayClient(bidGatewayURL),
	}
	if strings.TrimSpace(workPublisherURL) != "" {
		s.workPublisher = clients.NewWorkPublisherClient(workPublisherURL)
	}
	if strings.TrimSpace(settlementURL) != "" {
		s.settlement = clients.NewSettlementClient(settlementURL)
	}
	return s, nil
}

// WaitForSettlements blocks until in-flight settlement notifications finish
// or ctx is done.
func (s *Service) WaitForSettlements(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.settlements.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) HandleAward(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workID := pathParam(r.URL.Path, "/v1/work/", "/award")
	if workID == "" {
		respondError(w, http.StatusBadRequest, "WORK_ID_REQUIRED", "work_id is required")
		return
	}

	var req model.AwardRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "bad request")
		return
	}

	bids, err := s.bg.ListBids(ctx, workID)
	if err != nil {
		respondError(w, http.StatusBadGateway, "BAD_GATEWAY", "failed to fetch bids")
		return
	}

	now := time.Now().UTC()
	var chosen *clients.Bid
	if req.AutoAward {
		// Simplest policy for local use: choose the lowest price among unexpired bids.
		for i := range bids {
			if bids[i].ExpiresAt.Before(now) {
				continue
			}
			if chosen == nil || bids[i].Price < chosen.Price {
				chosen = &bids[i]
			}
		}
		if chosen == nil {
			respondError(w, http.StatusBadRequest, "NO_VALID_BIDS", "no valid bids to award")
			return
		}
		req.BidID = chosen.BidID
	} else {
		for i := range bids {
			if bids[i].BidID == req.BidID {
				chosen = &bids[i]
				break
			}
		}
		if chosen == nil {
			respondError(w, http.StatusBadRequest, "INVALID_BID_ID", "invalid bid_id")
			return
		}
		if chosen.ExpiresAt.Before(now) {
			respondError(w, http.StatusConflict, "BID_EXPIRED", "bid expired")
			return
		}
	}

	contractID := generateID("contract_")
	execToken := generateID("exec_")
	consumerToken := generateID("cons_")
	expiresAt := now.Add(1 * time.Hour)

	contract := model.Contract{
		ContractID:       contractID,
		WorkID:           workID,
		ConsumerID:       s.lookupConsumerID(ctx, workID),
		ProviderID:       chosen.ProviderID,
		BidID:            chosen.BidID,
		AgreedPrice:      chosen.Price,
		SLA:              model.SLACommitment{},
		ProviderEndpoint: chosen.A2AEndpoint,
		ExecutionToken:   execToken,
		ConsumerToken:    consumerToken,
		Status:           model.ContractStatusAwarded,
		ExpiresAt:        expiresAt,
		AwardedAt:        now,
	}

	if err := s.store.Save(ctx, contract); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to save contract")
		return
	}

	resp := model.AwardResponse{
		ContractID:       contract.ContractID,
		WorkID:           contract.WorkID,
		ProviderID:       contract.ProviderID,
		AgreedPrice:      contract.AgreedPrice,
		Status:           contract.Status,
		ProviderEndpoint: contract.ProviderEndpoint,
		ExecutionToken:   contract.ExecutionToken,
		ExpiresAt:        contract.ExpiresAt,
		AwardedAt:        contract.AwardedAt,
	}
	writeJSON(w, http.StatusOK, resp)
}

// lookupConsumerID resolves the consumer that submitted workID from
// aex-work-publisher. A failed lookup does not fail the award: the contract
// records an unknown consumer and is not sent to settlement on completion.
func (s *Service) lookupConsumerID(ctx context.Context, workID string) string {
	if s.workPublisher == nil {
		slog.WarnContext(ctx, "consumer lookup skipped: WORK_PUBLISHER_URL not configured",
			"work_id", workID,
		)
		return unknownConsumerID
	}
	lookupCtx, cancel := context.WithTimeout(ctx, consumerLookupTimeout)
	defer cancel()
	work, err := s.workPublisher.GetWork(lookupCtx, workID)
	if err != nil {
		slog.WarnContext(ctx, "consumer lookup failed; contract will not be settled",
			"error", err,
			"work_id", workID,
		)
		return unknownConsumerID
	}
	if strings.TrimSpace(work.ConsumerID) == "" {
		slog.WarnContext(ctx, "work has no consumer_id; contract will not be settled",
			"work_id", workID,
		)
		return unknownConsumerID
	}
	return work.ConsumerID
}

func (s *Service) HandleGetContract(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	contractID := pathParam(r.URL.Path, "/v1/contracts/", "")
	if contractID == "" {
		respondError(w, http.StatusBadRequest, "CONTRACT_ID_REQUIRED", "contract_id is required")
		return
	}
	c, err := s.store.Get(ctx, contractID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	if c == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Service) HandleProgress(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	contractID := pathParam(r.URL.Path, "/v1/contracts/", "/progress")
	if contractID == "" {
		respondError(w, http.StatusBadRequest, "CONTRACT_ID_REQUIRED", "contract_id is required")
		return
	}
	token := bearerToken(r)
	if token == "" {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	var req model.ProgressRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "bad request")
		return
	}

	c, err := s.store.Get(ctx, contractID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	if c == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	if c.ExecutionToken != token {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}

	now := time.Now().UTC()
	if now.After(c.ExpiresAt) {
		c.Status = model.ContractStatusExpired
		_ = s.store.Update(ctx, *c)
		respondError(w, http.StatusGone, "CONTRACT_EXPIRED", "contract expired")
		return
	}
	c.ExecutionUpdates = append(c.ExecutionUpdates, model.ExecutionUpdate{
		Status:    req.Status,
		Percent:   req.Percent,
		Message:   req.Message,
		Timestamp: now,
	})
	if c.Status == model.ContractStatusAwarded {
		c.Status = model.ContractStatusExecuting
		c.StartedAt = &now
	}
	if err := s.store.Update(ctx, *c); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"acknowledged": true, "contract_id": contractID})
}

func (s *Service) HandleComplete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	contractID := pathParam(r.URL.Path, "/v1/contracts/", "/complete")
	if contractID == "" {
		respondError(w, http.StatusBadRequest, "CONTRACT_ID_REQUIRED", "contract_id is required")
		return
	}
	token := bearerToken(r)
	if token == "" {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	var req model.CompleteRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "bad request")
		return
	}

	c, err := s.store.Get(ctx, contractID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	if c == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	if c.ExecutionToken != token {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}

	if c.Status != model.ContractStatusAwarded && c.Status != model.ContractStatusExecuting {
		respondError(w, http.StatusConflict, "INVALID_CONTRACT_STATE", "contract cannot be completed in status "+string(c.Status))
		return
	}

	now := time.Now().UTC()
	if now.After(c.ExpiresAt) {
		c.Status = model.ContractStatusExpired
		_ = s.store.Update(ctx, *c)
		respondError(w, http.StatusGone, "CONTRACT_EXPIRED", "contract expired")
		return
	}
	c.Status = model.ContractStatusCompleted
	c.CompletedAt = &now
	c.Outcome = &model.OutcomeReport{
		Success:        req.Success,
		ResultSummary:  req.ResultSummary,
		Metrics:        req.Metrics,
		ResultLocation: req.ResultLocation,
		ReportedAt:     now,
	}
	if err := s.store.Update(ctx, *c); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	settlementInitiated := s.triggerSettlement(ctx, *c)
	writeJSON(w, http.StatusOK, map[string]any{
		"contract_id":          contractID,
		"status":               c.Status,
		"settlement_initiated": settlementInitiated,
		"completed_at":         now,
	})
}

// triggerSettlement notifies aex-settlement of a completed contract in the
// background so the provider's completion response does not depend on
// settlement availability. Only successful outcomes with a known consumer are
// settled. It reports whether a notification was started.
func (s *Service) triggerSettlement(ctx context.Context, c model.Contract) bool {
	if c.Outcome == nil || !c.Outcome.Success {
		slog.InfoContext(ctx, "settlement skipped: contract completed with an unsuccessful outcome",
			"contract_id", c.ContractID,
			"work_id", c.WorkID,
		)
		return false
	}
	if c.ConsumerID == "" || c.ConsumerID == unknownConsumerID {
		slog.WarnContext(ctx, "settlement skipped: contract has no known consumer",
			"contract_id", c.ContractID,
			"work_id", c.WorkID,
			"consumer_id", c.ConsumerID,
		)
		return false
	}
	if s.settlement == nil {
		slog.WarnContext(ctx, "settlement skipped: SETTLEMENT_URL not configured",
			"contract_id", c.ContractID,
			"work_id", c.WorkID,
		)
		return false
	}
	event := settlementEvent(c)
	// Detach from the request so the notification outlives the response,
	// keeping trace values for log correlation.
	bgCtx := context.WithoutCancel(ctx)
	s.settlements.Add(1)
	go func() {
		defer s.settlements.Done()
		ctx, cancel := context.WithTimeout(bgCtx, SettlementTimeout)
		defer cancel()
		_ = s.notifySettlement(ctx, event)
	}()
	return true
}

// notifySettlement sends the completion event to aex-settlement. A duplicate
// (409) is treated as success by the client. Failures are logged with the
// full event so the settlement can be replayed.
func (s *Service) notifySettlement(ctx context.Context, event clients.ContractCompletedEvent) error {
	if err := s.settlement.ProcessContractCompletion(ctx, event); err != nil {
		slog.ErrorContext(ctx, "settlement notification failed; contract completed but not settled",
			"error", err,
			"contract_id", event.ContractID,
			"work_id", event.WorkID,
			"consumer_id", event.ConsumerID,
			"provider_id", event.ProviderID,
			"agreed_price", event.AgreedPrice,
			"settlement_event", event,
		)
		return err
	}
	slog.InfoContext(ctx, "settlement notified",
		"contract_id", event.ContractID,
		"provider_id", event.ProviderID,
		"agreed_price", event.AgreedPrice,
	)
	return nil
}

// settlementEvent maps a completed contract to aex-settlement's
// ContractCompletedEvent.
func settlementEvent(c model.Contract) clients.ContractCompletedEvent {
	startedAt := c.AwardedAt
	if c.StartedAt != nil {
		startedAt = *c.StartedAt
	}
	var completedAt time.Time
	if c.CompletedAt != nil {
		completedAt = *c.CompletedAt
	}
	metadata := map[string]any{"bid_id": c.BidID}
	success := false
	if c.Outcome != nil {
		success = c.Outcome.Success
		if c.Outcome.ResultSummary != "" {
			metadata["result_summary"] = c.Outcome.ResultSummary
		}
		if c.Outcome.ResultLocation != nil {
			metadata["result_location"] = *c.Outcome.ResultLocation
		}
	}
	return clients.ContractCompletedEvent{
		ContractID: c.ContractID,
		WorkID:     c.WorkID,
		// Contracts do not track a separate agent; the provider is the agent.
		AgentID:     c.ProviderID,
		ConsumerID:  c.ConsumerID,
		ProviderID:  c.ProviderID,
		StartedAt:   startedAt,
		CompletedAt: completedAt,
		Success:     success,
		AgreedPrice: strconv.FormatFloat(c.AgreedPrice, 'f', -1, 64),
		Metadata:    metadata,
	}
}

func (s *Service) HandleFail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	contractID := pathParam(r.URL.Path, "/v1/contracts/", "/fail")
	if contractID == "" {
		respondError(w, http.StatusBadRequest, "CONTRACT_ID_REQUIRED", "contract_id is required")
		return
	}
	// For local: allow either execution token or consumer token; both are Bearer.
	token := bearerToken(r)
	if token == "" {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	var req model.FailRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "bad request")
		return
	}

	c, err := s.store.Get(ctx, contractID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	if c == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	if c.ExecutionToken != token && c.ConsumerToken != token {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}

	now := time.Now().UTC()
	if now.After(c.ExpiresAt) {
		c.Status = model.ContractStatusExpired
		_ = s.store.Update(ctx, *c)
		respondError(w, http.StatusGone, "CONTRACT_EXPIRED", "contract expired")
		return
	}
	c.Status = model.ContractStatusFailed
	c.FailedAt = &now
	c.FailureReason = &req.Reason
	if err := s.store.Update(ctx, *c); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"contract_id":    contractID,
		"status":         c.Status,
		"failure_reason": req.Reason,
		"failed_at":      now,
	})
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

func bearerToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
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
	// take first segment
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	return strings.TrimSpace(rest)
}
