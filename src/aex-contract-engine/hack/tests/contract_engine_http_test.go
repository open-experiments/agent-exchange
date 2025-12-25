package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cehttp "github.com/parlakisik/agent-exchange/aex-contract-engine/internal/httpapi"
	cesvc "github.com/parlakisik/agent-exchange/aex-contract-engine/internal/service"
	cestore "github.com/parlakisik/agent-exchange/aex-contract-engine/internal/store"
)

func TestAwardProgressCompleteFlow(t *testing.T) {
	// Bid-gateway stub (real HTTP server).
	bg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/internal/bids" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		workID := r.URL.Query().Get("work_id")
		now := time.Now().UTC()
		resp := map[string]any{
			"work_id": workID,
			"bids": []map[string]any{
				{
					"bid_id":       "bid_1",
					"work_id":      workID,
					"provider_id":  "prov_a",
					"price":        0.10,
					"a2a_endpoint": "https://a2a/a",
					"expires_at":   now.Add(10 * time.Minute).Format(time.RFC3339Nano),
					"received_at":  now.Format(time.RFC3339Nano),
				},
			},
			"total_bids": 1,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(bg.Close)

	svc, err := cesvc.New(cestore.NewMemoryContractStore(), bg.URL)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(cehttp.NewRouter(svc))
	t.Cleanup(ts.Close)

	awardReq := map[string]any{"bid_id": "bid_1", "auto_award": false}
	awardBody, _ := json.Marshal(awardReq)
	awardResp, err := http.Post(ts.URL+"/v1/work/work_1/award", "application/json", bytes.NewReader(awardBody))
	if err != nil {
		t.Fatal(err)
	}
	defer awardResp.Body.Close()
	if awardResp.StatusCode != 200 {
		t.Fatalf("award expected 200, got %d", awardResp.StatusCode)
	}
	var awardOut struct {
		ContractID     string `json:"contract_id"`
		ExecutionToken string `json:"execution_token"`
	}
	_ = json.NewDecoder(awardResp.Body).Decode(&awardOut)
	if awardOut.ContractID == "" || awardOut.ExecutionToken == "" {
		t.Fatalf("missing contract_id or execution_token")
	}

	// progress
	progressReq := map[string]any{"status": "progress", "percent": 50, "message": "half"}
	progressBody, _ := json.Marshal(progressReq)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/contracts/"+awardOut.ContractID+"/progress", bytes.NewReader(progressBody))
	req.Header.Set("Authorization", "Bearer "+awardOut.ExecutionToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("progress expected 200, got %d", resp.StatusCode)
	}

	// complete
	completeReq := map[string]any{"success": true, "result_summary": "ok", "metrics": map[string]any{"x": 1}}
	completeBody, _ := json.Marshal(completeReq)
	req2, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/contracts/"+awardOut.ContractID+"/complete", bytes.NewReader(completeBody))
	req2.Header.Set("Authorization", "Bearer "+awardOut.ExecutionToken)
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("complete expected 200, got %d", resp2.StatusCode)
	}
}

