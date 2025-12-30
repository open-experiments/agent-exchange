package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tbhttp "github.com/parlakisik/agent-exchange/aex-trust-broker/internal/httpapi"
	tbsvc "github.com/parlakisik/agent-exchange/aex-trust-broker/internal/service"
	tbst "github.com/parlakisik/agent-exchange/aex-trust-broker/internal/store"
)

func TestRecordOutcomeAndGetTrustAndBatch(t *testing.T) {
	svc := tbsvc.New(tbst.NewMemoryStore())
	ts := httptest.NewServer(tbhttp.NewRouter(svc))
	t.Cleanup(ts.Close)

	outcome := map[string]any{
		"contract_id":  "contract_1",
		"provider_id":  "prov_a",
		"consumer_id":  "tenant_1",
		"outcome":      "SUCCESS",
		"metrics":      map[string]any{"latency_ms": 1200},
		"agreed_price": 0.1,
		"final_price":  0.1,
		"completed_at": time.Now().UTC().Format(time.RFC3339Nano),
	}
	b, _ := json.Marshal(outcome)
	resp, err := http.Post(ts.URL+"/internal/v1/outcomes", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Get trust
	resp2, err := http.Get(ts.URL + "/v1/providers/prov_a/trust")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}

	// Batch trust
	batchReq := map[string]any{"provider_ids": []string{"prov_a", "prov_b"}}
	b2, _ := json.Marshal(batchReq)
	resp3, err := http.Post(ts.URL+"/internal/v1/trust/batch", "application/json", bytes.NewReader(b2))
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp3.StatusCode)
	}
}


