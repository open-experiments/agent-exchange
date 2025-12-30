package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	prhttp "github.com/parlakisik/agent-exchange/aex-provider-registry/internal/httpapi"
	prsvc "github.com/parlakisik/agent-exchange/aex-provider-registry/internal/service"
	prstore "github.com/parlakisik/agent-exchange/aex-provider-registry/internal/store"
)

func TestRegisterSubscribeAndInternalLookup(t *testing.T) {
	svc := prsvc.New(prstore.NewMemoryStore())
	ts := httptest.NewServer(prhttp.NewRouter(svc))
	t.Cleanup(ts.Close)

	regReq := map[string]any{
		"name":          "Test Provider",
		"description":   "desc",
		"endpoint":      "https://agent.example.com/a2a",
		"bid_webhook":   "https://agent.example.com/aex/work",
		"capabilities":  []string{"travel.booking"},
		"contact_email": "agents@example.com",
		"metadata":      map[string]any{"region": "global"},
	}
	b, _ := json.Marshal(regReq)
	resp, err := http.Post(ts.URL+"/v1/providers", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var regOut struct {
		ProviderID string `json:"provider_id"`
		APIKey     string `json:"api_key"`
		APISecret  string `json:"api_secret"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&regOut)
	if regOut.ProviderID == "" || regOut.APIKey == "" || regOut.APISecret == "" {
		t.Fatalf("missing provider credentials in response")
	}

	subReq := map[string]any{
		"provider_id": regOut.ProviderID,
		"categories":  []string{"travel.*"},
		"filters":     map[string]any{},
		"delivery": map[string]any{
			"method":         "webhook",
			"webhook_url":    "https://agent.example.com/aex/work",
			"webhook_secret": "whsec_test",
		},
	}
	sb, _ := json.Marshal(subReq)
	resp2, err := http.Post(ts.URL+"/v1/subscriptions", "application/json", bytes.NewReader(sb))
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}

	// Internal lookup should return the provider for travel.booking
	resp3, err := http.Get(ts.URL + "/internal/v1/providers/subscribed?category=travel.booking")
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp3.StatusCode)
	}
	var out struct {
		Providers []struct {
			ProviderID string `json:"provider_id"`
		} `json:"providers"`
	}
	_ = json.NewDecoder(resp3.Body).Decode(&out)
	if len(out.Providers) != 1 || out.Providers[0].ProviderID != regOut.ProviderID {
		t.Fatalf("expected provider %s, got %+v", regOut.ProviderID, out.Providers)
	}
}


