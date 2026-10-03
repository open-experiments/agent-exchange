package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/parlakisik/agent-exchange/aex-contract-engine/internal/model"
	"github.com/parlakisik/agent-exchange/aex-contract-engine/internal/store"
)

// settlementCompletedEvent mirrors aex-settlement's model.ContractCompletedEvent
// (src/aex-settlement/internal/model/model.go) so decoding with
// DisallowUnknownFields catches JSON tag drift between the two services.
type settlementCompletedEvent struct {
	ContractID    string         `json:"contract_id"`
	WorkID        string         `json:"work_id"`
	AgentID       string         `json:"agent_id"`
	ConsumerID    string         `json:"consumer_id"`
	ProviderID    string         `json:"provider_id"`
	Domain        string         `json:"domain"`
	Description   string         `json:"description,omitempty"`
	StartedAt     time.Time      `json:"started_at"`
	CompletedAt   time.Time      `json:"completed_at"`
	Success       bool           `json:"success"`
	AgreedPrice   string         `json:"agreed_price"`
	Currency      string         `json:"currency,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	UseAP2        bool           `json:"use_ap2,omitempty"`
	PaymentMethod string         `json:"payment_method,omitempty"`
	WorkCategory  string         `json:"work_category,omitempty"`
}

// settlementStub stands in for aex-settlement's /internal/settlement/complete.
type settlementStub struct {
	t      *testing.T
	status int

	mu     sync.Mutex
	events []settlementCompletedEvent
}

func newSettlementStub(t *testing.T, status int) (*settlementStub, *httptest.Server) {
	t.Helper()
	stub := &settlementStub{t: t, status: status}
	srv := httptest.NewServer(http.HandlerFunc(stub.serve))
	t.Cleanup(srv.Close)
	return stub, srv
}

func (s *settlementStub) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/internal/settlement/complete" {
		s.t.Errorf("unexpected settlement request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var ev settlementCompletedEvent
	if err := dec.Decode(&ev); err != nil {
		s.t.Errorf("settlement could not decode event: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.events = append(s.events, ev)
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(s.status)
	switch s.status {
	case http.StatusOK:
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "settled"})
	case http.StatusConflict:
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "EXECUTION_EXISTS", "message": "execution already recorded"}})
	default:
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "INTERNAL_ERROR", "message": "internal error"}})
	}
}

func (s *settlementStub) received() []settlementCompletedEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]settlementCompletedEvent(nil), s.events...)
}

func newBidGatewayStub(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		workID := r.URL.Query().Get("work_id")
		now := time.Now().UTC()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"bids": []map[string]any{{
				"bid_id":       "bid_1",
				"work_id":      workID,
				"provider_id":  "prov_a",
				"price":        12.5,
				"a2a_endpoint": "https://a2a/a",
				"expires_at":   now.Add(10 * time.Minute).Format(time.RFC3339Nano),
				"received_at":  now.Format(time.RFC3339Nano),
			}},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// workPublisherStub stands in for aex-work-publisher's GET /v1/work/{work_id}.
type workPublisherStub struct {
	mu    sync.Mutex
	paths []string
}

func (s *workPublisherStub) requested() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.paths...)
}

// newWorkPublisherStub answers every work lookup with consumerID, or with
// status when it is not 200.
func newWorkPublisherStub(t *testing.T, status int, consumerID string) (*workPublisherStub, *httptest.Server) {
	t.Helper()
	stub := &workPublisherStub{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stub.mu.Lock()
		stub.paths = append(stub.paths, r.Method+" "+r.URL.Path)
		stub.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status != http.StatusOK {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "INTERNAL_ERROR", "message": "failed to get work"}})
			return
		}
		workID := strings.TrimPrefix(r.URL.Path, "/v1/work/")
		// Shape of aex-work-publisher's model.WorkSpec.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"work_id":            workID,
			"consumer_id":        consumerID,
			"category":           "general",
			"description":        "test work",
			"constraints":        map[string]any{"internal_only": false},
			"budget":             map[string]any{"max_price": 20, "bid_strategy": "lowest_price"},
			"success_criteria":   []any{},
			"bid_window_ms":      30000,
			"payload":            map[string]any{},
			"status":             "AWARDED",
			"providers_notified": 1,
			"bids_received":      1,
			"version":            2,
			"created_at":         time.Now().UTC().Format(time.RFC3339Nano),
			"bid_window_ends_at": time.Now().UTC().Format(time.RFC3339Nano),
		})
	}))
	t.Cleanup(srv.Close)
	return stub, srv
}

func doJSON(t *testing.T, h http.HandlerFunc, path, token string, body any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	var out map[string]any
	raw, _ := io.ReadAll(rec.Body)
	_ = json.Unmarshal(raw, &out)
	return rec.Code, out
}

// award awards bid_1 on work_1 and returns the contract ID and execution token.
func award(t *testing.T, svc *Service) (string, string) {
	t.Helper()
	code, out := doJSON(t, svc.HandleAward, "/v1/work/work_1/award", "", map[string]any{"bid_id": "bid_1"})
	if code != http.StatusOK {
		t.Fatalf("award expected 200, got %d: %v", code, out)
	}
	contractID, _ := out["contract_id"].(string)
	token, _ := out["execution_token"].(string)
	return contractID, token
}

func complete(t *testing.T, svc *Service, contractID, token string, success bool, summary string) (int, map[string]any) {
	t.Helper()
	return doJSON(t, svc.HandleComplete, "/v1/contracts/"+contractID+"/complete", token, map[string]any{
		"success":         success,
		"result_summary":  summary,
		"metrics":         map[string]any{"x": 1},
		"result_location": "https://results/1",
	})
}

// awardAndComplete runs award -> progress -> complete (success) and returns
// the contract ID and the completion response.
func awardAndComplete(t *testing.T, svc *Service) (string, int, map[string]any) {
	t.Helper()
	contractID, token := award(t, svc)

	code, _ := doJSON(t, svc.HandleProgress, "/v1/contracts/"+contractID+"/progress", token, map[string]any{"status": "running"})
	if code != http.StatusOK {
		t.Fatalf("progress expected 200, got %d", code)
	}

	code, out := complete(t, svc, contractID, token, true, "done")
	return contractID, code, out
}

func errorCode(out map[string]any) string {
	e, _ := out["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func waitSettlements(t *testing.T, svc *Service) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := svc.WaitForSettlements(ctx); err != nil {
		t.Fatalf("settlement notification did not finish: %v", err)
	}
}

func TestCompleteNotifiesSettlement(t *testing.T) {
	stub, settlement := newSettlementStub(t, http.StatusOK)
	_, wp := newWorkPublisherStub(t, http.StatusOK, "tenant_1")
	st := store.NewMemoryContractStore()
	svc, err := New(st, newBidGatewayStub(t).URL, wp.URL, settlement.URL)
	if err != nil {
		t.Fatal(err)
	}

	contractID, code, out := awardAndComplete(t, svc)
	if code != http.StatusOK {
		t.Fatalf("complete expected 200, got %d: %v", code, out)
	}
	if out["settlement_initiated"] != true {
		t.Errorf("settlement_initiated = %v, want true", out["settlement_initiated"])
	}
	waitSettlements(t, svc)

	events := stub.received()
	if len(events) != 1 {
		t.Fatalf("settlement received %d events, want 1", len(events))
	}
	ev := events[0]

	c, err := st.Get(context.Background(), contractID)
	if err != nil || c == nil {
		t.Fatalf("get contract: %v", err)
	}
	if c.Status != model.ContractStatusCompleted {
		t.Errorf("contract status = %s, want COMPLETED", c.Status)
	}
	if ev.ContractID != contractID || ev.WorkID != "work_1" {
		t.Errorf("ids = %q/%q, want %q/work_1", ev.ContractID, ev.WorkID, contractID)
	}
	if ev.ProviderID != "prov_a" || ev.AgentID != "prov_a" {
		t.Errorf("provider/agent = %q/%q, want prov_a", ev.ProviderID, ev.AgentID)
	}
	if c.ConsumerID != "tenant_1" || ev.ConsumerID != "tenant_1" {
		t.Errorf("consumer_id contract/event = %q/%q, want tenant_1", c.ConsumerID, ev.ConsumerID)
	}
	if ev.AgreedPrice != "12.5" {
		t.Errorf("agreed_price = %q, want 12.5", ev.AgreedPrice)
	}
	if !ev.Success {
		t.Error("success = false, want true")
	}
	if c.StartedAt == nil || !ev.StartedAt.Equal(*c.StartedAt) {
		t.Errorf("started_at = %v, want %v", ev.StartedAt, c.StartedAt)
	}
	if c.CompletedAt == nil || !ev.CompletedAt.Equal(*c.CompletedAt) {
		t.Errorf("completed_at = %v, want %v", ev.CompletedAt, c.CompletedAt)
	}
	if ev.Metadata["bid_id"] != "bid_1" || ev.Metadata["result_summary"] != "done" || ev.Metadata["result_location"] != "https://results/1" {
		t.Errorf("metadata = %v", ev.Metadata)
	}
}

func TestSettlementConflictIsSuccess(t *testing.T) {
	stub, settlement := newSettlementStub(t, http.StatusConflict)
	svc, err := New(store.NewMemoryContractStore(), newBidGatewayStub(t).URL, "", settlement.URL)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	c := model.Contract{
		ContractID:  "contract_1",
		WorkID:      "work_1",
		ConsumerID:  "tenant_1",
		ProviderID:  "prov_a",
		AgreedPrice: 3,
		AwardedAt:   now,
		CompletedAt: &now,
		Outcome:     &model.OutcomeReport{Success: true},
	}
	if err := svc.notifySettlement(context.Background(), settlementEvent(c)); err != nil {
		t.Fatalf("409 from settlement should be treated as success, got %v", err)
	}
	if n := len(stub.received()); n != 1 {
		t.Fatalf("settlement received %d events, want 1", n)
	}
}

func TestSettlementFailureDoesNotFailCompletion(t *testing.T) {
	stub, settlement := newSettlementStub(t, http.StatusInternalServerError)
	_, wp := newWorkPublisherStub(t, http.StatusOK, "tenant_1")
	st := store.NewMemoryContractStore()
	svc, err := New(st, newBidGatewayStub(t).URL, wp.URL, settlement.URL)
	if err != nil {
		t.Fatal(err)
	}

	contractID, code, out := awardAndComplete(t, svc)
	if code != http.StatusOK {
		t.Fatalf("complete expected 200 despite settlement failure, got %d: %v", code, out)
	}
	waitSettlements(t, svc)
	if n := len(stub.received()); n == 0 {
		t.Fatal("settlement was not called")
	}

	c, err := st.Get(context.Background(), contractID)
	if err != nil || c == nil {
		t.Fatalf("get contract: %v", err)
	}
	if c.Status != model.ContractStatusCompleted {
		t.Errorf("contract status = %s, want COMPLETED", c.Status)
	}

	if err := svc.notifySettlement(context.Background(), settlementEvent(*c)); err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("notifySettlement error = %v, want HTTP 500 error", err)
	}
}

func TestSettlementSkippedWhenURLUnset(t *testing.T) {
	_, wp := newWorkPublisherStub(t, http.StatusOK, "tenant_1")
	svc, err := New(store.NewMemoryContractStore(), newBidGatewayStub(t).URL, wp.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	if svc.settlement != nil {
		t.Fatal("settlement client should be nil when URL is unset")
	}

	_, code, out := awardAndComplete(t, svc)
	if code != http.StatusOK {
		t.Fatalf("complete expected 200, got %d: %v", code, out)
	}
	if out["settlement_initiated"] != false {
		t.Errorf("settlement_initiated = %v, want false", out["settlement_initiated"])
	}
	waitSettlements(t, svc)
}

func TestAwardRecordsConsumerFromWorkPublisher(t *testing.T) {
	wpStub, wp := newWorkPublisherStub(t, http.StatusOK, "tenant_42")
	st := store.NewMemoryContractStore()
	svc, err := New(st, newBidGatewayStub(t).URL, wp.URL, "")
	if err != nil {
		t.Fatal(err)
	}

	contractID, _ := award(t, svc)
	if got := wpStub.requested(); len(got) != 1 || got[0] != "GET /v1/work/work_1" {
		t.Errorf("work-publisher requests = %v, want [GET /v1/work/work_1]", got)
	}
	c, err := st.Get(context.Background(), contractID)
	if err != nil || c == nil {
		t.Fatalf("get contract: %v", err)
	}
	if c.ConsumerID != "tenant_42" {
		t.Errorf("consumer_id = %q, want tenant_42", c.ConsumerID)
	}
}

func TestAwardFallsBackToUnknownConsumer(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		consumerID string
		unsetURL   bool
	}{
		{name: "work-publisher error", status: http.StatusInternalServerError},
		{name: "work not found", status: http.StatusNotFound},
		{name: "empty consumer_id", status: http.StatusOK, consumerID: ""},
		{name: "WORK_PUBLISHER_URL unset", unsetURL: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub, settlement := newSettlementStub(t, http.StatusOK)
			wpURL := ""
			if !tt.unsetURL {
				_, wp := newWorkPublisherStub(t, tt.status, tt.consumerID)
				wpURL = wp.URL
			}
			st := store.NewMemoryContractStore()
			svc, err := New(st, newBidGatewayStub(t).URL, wpURL, settlement.URL)
			if err != nil {
				t.Fatal(err)
			}
			if tt.unsetURL && svc.workPublisher != nil {
				t.Fatal("work-publisher client should be nil when URL is unset")
			}

			contractID, code, out := awardAndComplete(t, svc)
			if code != http.StatusOK {
				t.Fatalf("complete expected 200, got %d: %v", code, out)
			}
			if out["settlement_initiated"] != false {
				t.Errorf("settlement_initiated = %v, want false", out["settlement_initiated"])
			}
			waitSettlements(t, svc)
			if n := len(stub.received()); n != 0 {
				t.Errorf("settlement received %d events, want 0 for unknown consumer", n)
			}

			c, err := st.Get(context.Background(), contractID)
			if err != nil || c == nil {
				t.Fatalf("get contract: %v", err)
			}
			if c.ConsumerID != unknownConsumerID {
				t.Errorf("consumer_id = %q, want %q", c.ConsumerID, unknownConsumerID)
			}
			if c.Status != model.ContractStatusCompleted {
				t.Errorf("contract status = %s, want COMPLETED", c.Status)
			}
		})
	}
}

func TestUnsuccessfulOutcomeIsNotSettled(t *testing.T) {
	stub, settlement := newSettlementStub(t, http.StatusOK)
	_, wp := newWorkPublisherStub(t, http.StatusOK, "tenant_1")
	st := store.NewMemoryContractStore()
	svc, err := New(st, newBidGatewayStub(t).URL, wp.URL, settlement.URL)
	if err != nil {
		t.Fatal(err)
	}

	contractID, token := award(t, svc)
	code, out := complete(t, svc, contractID, token, false, "could not finish")
	if code != http.StatusOK {
		t.Fatalf("complete expected 200, got %d: %v", code, out)
	}
	if out["settlement_initiated"] != false {
		t.Errorf("settlement_initiated = %v, want false", out["settlement_initiated"])
	}
	waitSettlements(t, svc)
	if n := len(stub.received()); n != 0 {
		t.Errorf("settlement received %d events, want 0 for unsuccessful outcome", n)
	}

	c, err := st.Get(context.Background(), contractID)
	if err != nil || c == nil {
		t.Fatalf("get contract: %v", err)
	}
	if c.Status != model.ContractStatusCompleted {
		t.Errorf("contract status = %s, want COMPLETED", c.Status)
	}
	if c.Outcome == nil || c.Outcome.Success || c.Outcome.ResultSummary != "could not finish" {
		t.Errorf("outcome = %+v, want recorded unsuccessful outcome", c.Outcome)
	}
}

func TestRecompleteCompletedContractConflicts(t *testing.T) {
	stub, settlement := newSettlementStub(t, http.StatusOK)
	_, wp := newWorkPublisherStub(t, http.StatusOK, "tenant_1")
	st := store.NewMemoryContractStore()
	svc, err := New(st, newBidGatewayStub(t).URL, wp.URL, settlement.URL)
	if err != nil {
		t.Fatal(err)
	}

	contractID, token := award(t, svc)
	if code, out := complete(t, svc, contractID, token, true, "first"); code != http.StatusOK {
		t.Fatalf("first complete expected 200, got %d: %v", code, out)
	}
	waitSettlements(t, svc)
	before, err := st.Get(context.Background(), contractID)
	if err != nil || before == nil {
		t.Fatalf("get contract: %v", err)
	}

	code, out := complete(t, svc, contractID, token, true, "second")
	if code != http.StatusConflict {
		t.Fatalf("second complete expected 409, got %d: %v", code, out)
	}
	if got := errorCode(out); got != "INVALID_CONTRACT_STATE" {
		t.Errorf("error code = %q, want INVALID_CONTRACT_STATE", got)
	}
	waitSettlements(t, svc)
	if n := len(stub.received()); n != 1 {
		t.Errorf("settlement received %d events, want 1", n)
	}

	after, err := st.Get(context.Background(), contractID)
	if err != nil || after == nil {
		t.Fatalf("get contract: %v", err)
	}
	if after.Outcome == nil || after.Outcome.ResultSummary != "first" {
		t.Errorf("outcome = %+v, want first outcome kept", after.Outcome)
	}
	if after.CompletedAt == nil || !after.CompletedAt.Equal(*before.CompletedAt) {
		t.Errorf("completed_at = %v, want %v", after.CompletedAt, before.CompletedAt)
	}
}

func TestCompleteAfterFailConflicts(t *testing.T) {
	stub, settlement := newSettlementStub(t, http.StatusOK)
	_, wp := newWorkPublisherStub(t, http.StatusOK, "tenant_1")
	st := store.NewMemoryContractStore()
	svc, err := New(st, newBidGatewayStub(t).URL, wp.URL, settlement.URL)
	if err != nil {
		t.Fatal(err)
	}

	contractID, token := award(t, svc)
	code, out := doJSON(t, svc.HandleFail, "/v1/contracts/"+contractID+"/fail", token, map[string]any{"reason": "crashed"})
	if code != http.StatusOK {
		t.Fatalf("fail expected 200, got %d: %v", code, out)
	}

	code, out = complete(t, svc, contractID, token, true, "done")
	if code != http.StatusConflict {
		t.Fatalf("complete after fail expected 409, got %d: %v", code, out)
	}
	if got := errorCode(out); got != "INVALID_CONTRACT_STATE" {
		t.Errorf("error code = %q, want INVALID_CONTRACT_STATE", got)
	}
	waitSettlements(t, svc)
	if n := len(stub.received()); n != 0 {
		t.Errorf("settlement received %d events, want 0", n)
	}

	c, err := st.Get(context.Background(), contractID)
	if err != nil || c == nil {
		t.Fatalf("get contract: %v", err)
	}
	if c.Status != model.ContractStatusFailed || c.Outcome != nil || c.CompletedAt != nil {
		t.Errorf("contract = status %s outcome %+v completed_at %v, want FAILED with no outcome", c.Status, c.Outcome, c.CompletedAt)
	}
}
