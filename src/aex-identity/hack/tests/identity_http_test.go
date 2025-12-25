package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	idhttp "github.com/parlakisik/agent-exchange/aex-identity/internal/httpapi"
	idsvc "github.com/parlakisik/agent-exchange/aex-identity/internal/service"
	idst "github.com/parlakisik/agent-exchange/aex-identity/internal/store"
)

func TestTenantCreateAPIKeyAndValidate(t *testing.T) {
	svc := idsvc.New(idst.NewMemoryStore())
	ts := httptest.NewServer(idhttp.NewRouter(svc))
	t.Cleanup(ts.Close)

	// Create tenant (should return initial api key)
	createReq := map[string]any{
		"name":          "tenant-a",
		"type":          "BOTH",
		"contact_email": "a@example.com",
		"billing_email": "bill@example.com",
	}
	b, _ := json.Marshal(createReq)
	resp, err := http.Post(ts.URL+"/v1/tenants", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected %d got %d", http.StatusCreated, resp.StatusCode)
	}
	var created map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	tenantID, _ := created["id"].(string)
	if tenantID == "" {
		t.Fatalf("missing tenant id")
	}
	apiKeyObj, _ := created["api_key"].(map[string]any)
	initialKey, _ := apiKeyObj["key"].(string)
	if initialKey == "" {
		t.Fatalf("missing initial api key")
	}

	// Create second API key
	keyReq := map[string]any{"name": "k2", "scopes": []string{"work:read", "work:write"}}
	b2, _ := json.Marshal(keyReq)
	resp2, err := http.Post(ts.URL+"/v1/tenants/"+tenantID+"/api-keys", "application/json", bytes.NewReader(b2))
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("expected %d got %d", http.StatusCreated, resp2.StatusCode)
	}
	var createdKey map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&createdKey); err != nil {
		t.Fatal(err)
	}
	key2, _ := createdKey["key"].(string)
	if key2 == "" {
		t.Fatalf("missing created api key")
	}

	// Validate initial key (internal)
	valReq := map[string]any{"api_key": initialKey}
	b3, _ := json.Marshal(valReq)
	resp3, err := http.Post(ts.URL+"/internal/v1/apikeys/validate", "application/json", bytes.NewReader(b3))
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("expected %d got %d", http.StatusOK, resp3.StatusCode)
	}

	// Validate second key
	valReq2 := map[string]any{"api_key": key2}
	b4, _ := json.Marshal(valReq2)
	resp4, err := http.Post(ts.URL+"/internal/v1/apikeys/validate", "application/json", bytes.NewReader(b4))
	if err != nil {
		t.Fatal(err)
	}
	defer resp4.Body.Close()
	if resp4.StatusCode != http.StatusOK {
		t.Fatalf("expected %d got %d", http.StatusOK, resp4.StatusCode)
	}
}

