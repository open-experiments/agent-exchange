package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	evalhttp "github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/httpapi"
	evalsvc "github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/service"
	evalstore "github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/store"
)

func TestEvaluateOverHTTPUsingRealBidGatewayHTTP(t *testing.T) {
	// Spin up a minimal bid-gateway stub with real HTTP server (loopback).
	bg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/internal/v1/bids" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		workID := r.URL.Query().Get("work_id")
		if workID == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		now := time.Now().UTC()
		resp := map[string]any{
			"work_id": workID,
			"bids": []map[string]any{
				{
					"bid_id":       "bid_1",
					"work_id":      workID,
					"provider_id":  "prov_a",
					"price":        0.10,
					"confidence":   0.9,
					"sla":          map[string]any{"max_latency_ms": 2000, "availability": 0.99},
					"a2a_endpoint": "https://a2a/a",
					"expires_at":   now.Add(5 * time.Minute).Format(time.RFC3339Nano),
					"received_at":  now.Format(time.RFC3339Nano),
				},
				{
					"bid_id":       "bid_2",
					"work_id":      workID,
					"provider_id":  "prov_b",
					"price":        0.30, // should be disqualified if max_price=0.25
					"confidence":   0.9,
					"sla":          map[string]any{"max_latency_ms": 2000, "availability": 0.99},
					"a2a_endpoint": "https://a2a/b",
					"expires_at":   now.Add(5 * time.Minute).Format(time.RFC3339Nano),
					"received_at":  now.Format(time.RFC3339Nano),
				},
			},
			"total_bids": 2,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(bg.Close)

	svc, err := evalsvc.New(bg.URL, "", evalstore.NewMemoryEvaluationStore())
	if err != nil {
		t.Fatal(err)
	}
	ev := httptest.NewServer(evalhttp.NewRouter(svc))
	t.Cleanup(ev.Close)

	reqBody := map[string]any{
		"work_id": "work_1",
		"budget": map[string]any{
			"max_price":    0.25,
			"bid_strategy": "balanced",
		},
	}
	b, _ := json.Marshal(reqBody)
	resp, err := http.Post(ev.URL+"/internal/v1/evaluate", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}


