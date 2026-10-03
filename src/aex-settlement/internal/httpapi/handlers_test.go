package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parlakisik/agent-exchange/aex-settlement/internal/service"
	"github.com/parlakisik/agent-exchange/aex-settlement/internal/store"
)

func newTestRouter(t *testing.T, st store.SettlementStore) http.Handler {
	t.Helper()
	t.Setenv("PAYMENT_PROVIDER_URLS", "[]")
	t.Setenv("AP2_ENABLED", "false")
	return NewRouter(service.New(st, nil))
}

func postCompletion(t *testing.T, h http.Handler, body map[string]any) (int, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/internal/settlement/complete", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp
}

func completionBody(contractID string) map[string]any {
	now := time.Now().UTC()
	return map[string]any{
		"contract_id":  contractID,
		"work_id":      "work_1",
		"agent_id":     "agent_1",
		"consumer_id":  "consumer",
		"provider_id":  "provider",
		"domain":       "general",
		"started_at":   now.Add(-time.Minute),
		"completed_at": now,
		"success":      true,
		"agreed_price": "10.00",
	}
}

func errorCode(resp map[string]any) string {
	e, _ := resp["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func TestProcessContractCompletionHTTP(t *testing.T) {
	st := store.NewMemoryStore()
	if _, err := st.IncrementBalance(context.Background(), "consumer", 10000, "USD"); err != nil {
		t.Fatal(err)
	}
	h := newTestRouter(t, st)

	code, resp := postCompletion(t, h, completionBody("c1"))
	if code != http.StatusOK || resp["status"] != "settled" || resp["charged"] != true {
		t.Fatalf("settle: %d %v", code, resp)
	}

	code, resp = postCompletion(t, h, completionBody("c1"))
	if code != http.StatusConflict || errorCode(resp) != "EXECUTION_EXISTS" {
		t.Fatalf("duplicate: %d %v, want 409 EXECUTION_EXISTS", code, resp)
	}

	failed := completionBody("c2")
	failed["success"] = false
	code, resp = postCompletion(t, h, failed)
	if code != http.StatusOK || resp["status"] != "recorded" || resp["charged"] != false {
		t.Fatalf("unsuccessful: %d %v, want 200 recorded charged=false", code, resp)
	}
}

func TestProcessContractCompletionHTTPRejections(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(map[string]any)
		wantCode int
		wantErr  string
	}{
		{"empty consumer", func(b map[string]any) { b["consumer_id"] = "" }, http.StatusBadRequest, "INVALID_CONSUMER_ID"},
		{"unknown consumer", func(b map[string]any) { b["consumer_id"] = "unknown" }, http.StatusBadRequest, "INVALID_CONSUMER_ID"},
		{"empty provider", func(b map[string]any) { b["provider_id"] = "" }, http.StatusBadRequest, "PROVIDER_ID_REQUIRED"},
		{"unfunded consumer", func(b map[string]any) { b["consumer_id"] = "never-deposited" }, http.StatusPaymentRequired, "CONSUMER_ACCOUNT_NOT_FOUND"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := store.NewMemoryStore()
			if _, err := st.IncrementBalance(context.Background(), "consumer", 10000, "USD"); err != nil {
				t.Fatal(err)
			}
			h := newTestRouter(t, st)
			body := completionBody("c1")
			tt.mutate(body)
			code, resp := postCompletion(t, h, body)
			if code != tt.wantCode || errorCode(resp) != tt.wantErr {
				t.Fatalf("got %d %v, want %d %s", code, resp, tt.wantCode, tt.wantErr)
			}
		})
	}
}

func TestProcessContractCompletionHTTPFailedThenSuccess(t *testing.T) {
	st := store.NewMemoryStore()
	if _, err := st.IncrementBalance(context.Background(), "consumer", 10000, "USD"); err != nil {
		t.Fatal(err)
	}
	h := newTestRouter(t, st)

	failed := completionBody("c1")
	failed["success"] = false
	if code, resp := postCompletion(t, h, failed); code != http.StatusOK || resp["status"] != "recorded" {
		t.Fatalf("failure: %d %v, want 200 recorded", code, resp)
	}
	if code, resp := postCompletion(t, h, failed); code != http.StatusConflict || errorCode(resp) != "EXECUTION_EXISTS" {
		t.Fatalf("repeated failure: %d %v, want 409 EXECUTION_EXISTS", code, resp)
	}
	if code, resp := postCompletion(t, h, completionBody("c1")); code != http.StatusOK || resp["status"] != "settled" || resp["charged"] != true {
		t.Fatalf("success after failure: %d %v, want 200 settled", code, resp)
	}
	if code, resp := postCompletion(t, h, completionBody("c1")); code != http.StatusConflict {
		t.Fatalf("duplicate success: %d %v, want 409", code, resp)
	}
	bal, _ := st.GetBalance(context.Background(), "consumer")
	if bal.Balance != 9000 {
		t.Errorf("consumer balance = %d, want 9000 (debited once)", bal.Balance)
	}
}
