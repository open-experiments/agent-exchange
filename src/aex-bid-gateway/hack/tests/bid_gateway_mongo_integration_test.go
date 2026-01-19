//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestBidGatewayMongoIntegration(t *testing.T) {
	baseURL := os.Getenv("AEX_BID_GATEWAY_URL")
	if baseURL == "" {
		t.Skip("set AEX_BID_GATEWAY_URL (e.g. http://localhost:8081) to run integration test")
	}
	apiKey := os.Getenv("AEX_PROVIDER_API_KEY")
	if apiKey == "" {
		t.Skip("set AEX_PROVIDER_API_KEY to run integration test")
	}

	expires := time.Now().UTC().Add(5 * time.Minute).Format(time.RFC3339Nano)
	reqBody := map[string]any{
		"work_id":              "work_mongo_1",
		"price":                0.08,
		"confidence":           0.92,
		"approach":             "mongo integration test",
		"estimated_latency_ms": 1500,
		"sla": map[string]any{
			"max_latency_ms": 3000,
			"availability":   0.99,
		},
		"a2a_endpoint": "https://agent.example.com/a2a/v1",
		"expires_at":   expires,
	}
	b, _ := json.Marshal(reqBody)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/bids", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	listResp, err := http.Get(baseURL + "/internal/bids?work_id=work_mongo_1")
	if err != nil {
		t.Fatal(err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
}
