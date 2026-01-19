package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parlakisik/agent-exchange/aex-bid-gateway/internal/httpapi"
	"github.com/parlakisik/agent-exchange/aex-bid-gateway/internal/service"
	"github.com/parlakisik/agent-exchange/aex-bid-gateway/internal/store"
)

func TestSubmitBidAndListInternal(t *testing.T) {
	st := store.NewMemoryBidStore()
	svc := service.New(st, map[string]string{
		"test-api-key": "prov_test",
	})
	ts := httptest.NewServer(httpapi.NewRouter(svc))
	t.Cleanup(ts.Close)

	expires := time.Now().UTC().Add(5 * time.Minute).Format(time.RFC3339Nano)
	reqBody := map[string]any{
		"work_id":              "work_1",
		"price":                0.08,
		"confidence":           0.92,
		"approach":             "test",
		"estimated_latency_ms": 1500,
		"sla": map[string]any{
			"max_latency_ms": 3000,
			"availability":   0.99,
		},
		"a2a_endpoint": "https://agent.example.com/a2a/v1",
		"expires_at":   expires,
	}
	b, _ := json.Marshal(reqBody)

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/v1/bids", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test-api-key")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	listResp, err := http.Get(ts.URL + "/internal/v1/bids?work_id=work_1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listResp.Body.Close() }()
	if listResp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
}
