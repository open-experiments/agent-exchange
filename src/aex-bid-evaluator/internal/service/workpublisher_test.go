package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/model"
	"github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/store"
)

// newBidGatewayStub serves two bids: bid_cheap (0.10) and bid_pricey (0.30),
// both with a 500ms SLA.
func newBidGatewayStub(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/internal/v1/bids" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		workID := r.URL.Query().Get("work_id")
		now := time.Now().UTC()
		bid := func(id string, price float64) map[string]any {
			return map[string]any{
				"bid_id":       id,
				"work_id":      workID,
				"provider_id":  "prov_" + id,
				"price":        price,
				"confidence":   0.9,
				"sla":          map[string]any{"max_latency_ms": 500, "availability": 0.99},
				"a2a_endpoint": "https://a2a/" + id,
				"expires_at":   now.Add(5 * time.Minute).Format(time.RFC3339Nano),
				"received_at":  now.Format(time.RFC3339Nano),
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"work_id":    workID,
			"bids":       []map[string]any{bid("bid_cheap", 0.10), bid("bid_pricey", 0.30)},
			"total_bids": 2,
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newWorkPublisherStub mimics GET /v1/work/{work_id} on aex-work-publisher.
// work_1 exists with max_price 0.25 and max_latency_ms 1000; work_err returns
// 500; anything else is 404.
func newWorkPublisherStub(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/v1/work/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch strings.TrimPrefix(r.URL.Path, "/v1/work/") {
		case "work_1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"work_id":     "work_1",
				"consumer_id": "cons_1",
				"category":    "travel",
				"description": "book a flight",
				"constraints": map[string]any{"max_latency_ms": 1000, "internal_only": false},
				"budget":      map[string]any{"max_price": 0.25, "bid_strategy": "lowest_price"},
				"status":      "OPEN",
				"created_at":  time.Now().UTC().Format(time.RFC3339Nano),
			})
		case "work_err":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"code":"INTERNAL_ERROR"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"NOT_FOUND","message":"work not found"}}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func doEvaluate(t *testing.T, svc *Service, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/evaluate", bytes.NewReader(b))
	rec := httptest.NewRecorder()
	svc.HandleEvaluate(rec, req)
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) (string, string) {
	t.Helper()
	var out struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode error body: %v (%s)", err, rec.Body.String())
	}
	return out.Error.Code, out.Error.Message
}

func TestHandleEvaluate_WorkPublisher(t *testing.T) {
	bg := newBidGatewayStub(t)

	tests := []struct {
		name         string
		body         map[string]any
		wantStatus   int
		wantCode     string
		wantFetches  int32
		wantValid    int
		wantDisq     int
		wantDisqWhy  string
		wantFirstBid string
	}{
		{
			name:         "budget missing fetches work spec",
			body:         map[string]any{"work_id": "work_1"},
			wantStatus:   http.StatusOK,
			wantFetches:  1,
			wantValid:    1,
			wantDisq:     1,
			wantDisqWhy:  "Price exceeds budget",
			wantFirstBid: "bid_cheap",
		},
		{
			name: "request budget skips fetch and wins",
			body: map[string]any{
				"work_id": "work_1",
				"budget":  map[string]any{"max_price": 0.50, "bid_strategy": "balanced"},
			},
			wantStatus:  http.StatusOK,
			wantFetches: 0,
			wantValid:   2,
			wantDisq:    0,
		},
		{
			name: "request constraints override fetched ones",
			body: map[string]any{
				"work_id":     "work_1",
				"constraints": map[string]any{"max_latency_ms": 100},
			},
			wantStatus:  http.StatusOK,
			wantFetches: 1,
			wantValid:   0,
			wantDisq:    2,
		},
		{
			name: "negative max_price rejected without fetch",
			body: map[string]any{
				"work_id": "work_1",
				"budget":  map[string]any{"max_price": -1},
			},
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantFetches: 0,
		},
		{
			name:        "unknown work maps to 404",
			body:        map[string]any{"work_id": "work_missing"},
			wantStatus:  http.StatusNotFound,
			wantCode:    "WORK_NOT_FOUND",
			wantFetches: 1,
		},
		{
			name:        "work-publisher failure maps to 502",
			body:        map[string]any{"work_id": "work_err"},
			wantStatus:  http.StatusBadGateway,
			wantCode:    "BAD_GATEWAY",
			wantFetches: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hits atomic.Int32
			wp := newWorkPublisherStub(t, &hits)
			svc, err := New(bg.URL, "", "", wp.URL, store.NewMemoryEvaluationStore())
			if err != nil {
				t.Fatal(err)
			}

			rec := doEvaluate(t, svc, tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if got := hits.Load(); got != tt.wantFetches {
				t.Errorf("work-publisher fetches = %d, want %d", got, tt.wantFetches)
			}
			if tt.wantCode != "" {
				if code, _ := errorCode(t, rec); code != tt.wantCode {
					t.Errorf("error code = %q, want %q", code, tt.wantCode)
				}
				return
			}

			var ev model.BidEvaluation
			if err := json.Unmarshal(rec.Body.Bytes(), &ev); err != nil {
				t.Fatal(err)
			}
			if ev.ValidBids != tt.wantValid {
				t.Errorf("valid bids = %d, want %d", ev.ValidBids, tt.wantValid)
			}
			if len(ev.DisqualifiedBids) != tt.wantDisq {
				t.Fatalf("disqualified bids = %d, want %d (%+v)", len(ev.DisqualifiedBids), tt.wantDisq, ev.DisqualifiedBids)
			}
			if tt.wantDisqWhy != "" && ev.DisqualifiedBids[0].Reason != tt.wantDisqWhy {
				t.Errorf("disqualified reason = %q, want %q", ev.DisqualifiedBids[0].Reason, tt.wantDisqWhy)
			}
			if tt.wantFirstBid != "" && (len(ev.RankedBids) == 0 || ev.RankedBids[0].BidID != tt.wantFirstBid) {
				t.Errorf("first ranked bid = %+v, want %s", ev.RankedBids, tt.wantFirstBid)
			}
		})
	}
}

func TestHandleEvaluate_NoWorkPublisherRequiresBudget(t *testing.T) {
	bg := newBidGatewayStub(t)
	svc, err := New(bg.URL, "", "", "", store.NewMemoryEvaluationStore())
	if err != nil {
		t.Fatal(err)
	}

	rec := doEvaluate(t, svc, map[string]any{"work_id": "work_1"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
	code, msg := errorCode(t, rec)
	if code != "BAD_REQUEST" {
		t.Errorf("error code = %q, want BAD_REQUEST", code)
	}
	if !strings.Contains(msg, "budget.max_price is required") || strings.Contains(msg, "not integrated") {
		t.Errorf("unexpected message %q", msg)
	}

	// Callers that pass a budget still work without work-publisher.
	rec = doEvaluate(t, svc, map[string]any{
		"work_id": "work_1",
		"budget":  map[string]any{"max_price": 0.25},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
}
