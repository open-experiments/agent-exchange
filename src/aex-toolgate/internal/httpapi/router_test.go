package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/parlakisik/agent-exchange/internal/toolgate"
)

// testOperatorToken stands in for the credential the provider's operator
// holds. The gated agent never has it.
const testOperatorToken = "test-operator-token"

// operatorHeaders authorizes the operator-only endpoints.
func operatorHeaders() map[string]string {
	return map[string]string{"Authorization": "Bearer " + testOperatorToken}
}

// getAs performs a GET with the given headers.
func getAs(t *testing.T, url string, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

type testCall struct {
	ID   string         `json:"id"`
	Tool string         `json:"tool"`
	Args map[string]any `json:"args"`
	Note string         `json:"note"`
}

func setup(t *testing.T) (*httptest.Server, *httptest.Server, []testCall, *[]string) {
	t.Helper()
	var executed []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tool := strings.TrimPrefix(r.URL.Path, "/tools/")
		executed = append(executed, tool)
		if r.Header.Get("X-Request-ID") == "" {
			t.Error("upstream should receive X-Request-ID")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"tool": tool, "result": "side effect applied"})
	}))
	policy, err := toolgate.LoadPolicy("../../../internal/toolgate/testdata/policy.json")
	if err != nil {
		t.Fatal(err)
	}
	gate, err := toolgate.New(policy)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(gate, upstream.URL, "/tools", 5*time.Second,
		WithOperatorToken(testOperatorToken), WithExposeArgs(true)))
	b, err := os.ReadFile("../../../internal/toolgate/testdata/calls.json")
	if err != nil {
		t.Fatal(err)
	}
	var calls []testCall
	if err := json.Unmarshal(b, &calls); err != nil {
		t.Fatal(err)
	}
	return srv, upstream, calls, &executed
}

func post(t *testing.T, url string, body any, headers map[string]string) (*http.Response, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func TestTwelveCallsThroughTheService(t *testing.T) {
	srv, upstream, calls, executed := setup(t)
	defer srv.Close()
	defer upstream.Close()

	headers := map[string]string{"X-Tenant-ID": "tenant_123", "X-Request-ID": "req-1", "X-AEX-Agent-ID": "ap-assistant-v2",
		"X-AEX-Principal": "user:j.doe@corp.example", "X-AEX-Session-ID": "sess-7f3a",
		"X-Scopes": "invoices:read,invoices:approve,payments:send,vendors:update,email:send,reports:export,documents:read"}
	want := map[string]int{"C01": 200, "C02": 200, "C03": 200, "C04": 200, "C05": 403, "C06": 403, "C07": 202,
		"C08": 200, "C09": 403, "C10": 403, "C11": 403, "C12": 403}
	var hold string
	for _, c := range calls {
		headers["X-AEX-Call-ID"] = c.ID
		headers["X-Request-ID"] = "req-" + c.ID
		resp, body := post(t, srv.URL+"/v1/tools/"+c.Tool, c.Args, headers)
		if resp.StatusCode != want[c.ID] {
			t.Errorf("%s %s: expected %d, got %d (%v)", c.ID, c.Tool, want[c.ID], resp.StatusCode, body)
		}
		if resp.Header.Get("X-Toolgate-Hash") == "" {
			t.Errorf("%s: every answer carries the artifact hash", c.ID)
		}
		if c.ID == "C07" {
			hold, _ = body["hold"].(string)
		}
	}
	if len(*executed) != 5 {
		t.Fatalf("upstream should have run 5 tools, ran %d: %v", len(*executed), *executed)
	}

	// The held vendor change is approved by a person through the API and then runs.
	resp, body := post(t, srv.URL+"/v1/holds/"+hold, map[string]any{"approver": "user:ap.manager@corp.example", "approved": true}, operatorHeaders())
	if resp.StatusCode != http.StatusOK || body["approval"] != toolgate.ApprovalGranted {
		t.Fatalf("resolve: expected 200 human-approved, got %d %v", resp.StatusCode, body)
	}
	if len(*executed) != 6 || (*executed)[5] != "update_vendor" {
		t.Fatalf("the approved call should have run upstream, executed=%v", *executed)
	}
	if resp, _ := post(t, srv.URL+"/v1/holds/"+hold, map[string]any{"approver": "x", "approved": true}, operatorHeaders()); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("a settled hold cannot be settled twice, got %d", resp.StatusCode)
	}

	// Records: 13 on the chain (12 calls plus the approval), chain verifies.
	_, vb := getAs(t, srv.URL+"/v1/records/verify", operatorHeaders())
	var v map[string]any
	_ = json.Unmarshal(vb, &v)
	if v["ok"] != true || v["records"].(float64) != 13 {
		t.Fatalf("verify: %v", v)
	}
	if v["chain_id"] == "" || v["chain_id"] == nil {
		t.Errorf("verify should name the chain so a restart is visible: %v", v)
	}
	_, rb := getAs(t, srv.URL+"/v1/records", operatorHeaders())
	var recs []toolgate.Artifact
	_ = json.Unmarshal(rb, &recs)
	if recs[5].TraceID != "req-C06" || recs[5].Rule != "P3-recipient" || recs[5].TenantID != "tenant_123" {
		t.Fatalf("C06 record should carry the gateway request id, the rule and the tenant: %+v", recs[5])
	}
	if ok, _ := toolgate.VerifyChain(recs); !ok {
		t.Fatal("chain from the API should verify")
	}
}

func TestScopesHeaderGovernsTheScopeCheck(t *testing.T) {
	srv, upstream, _, executed := setup(t)
	defer srv.Close()
	defer upstream.Close()

	// A caller whose validated scopes lack invoices:read is refused on the scope rule even though the policy file grants it.
	resp, body := post(t, srv.URL+"/v1/tools/list_invoices", map[string]any{"status": "open"}, map[string]string{"X-Scopes": "email:send"})
	if resp.StatusCode != http.StatusForbidden || body["rule"] != toolgate.ScopeRuleID {
		t.Fatalf("expected 403 on %s, got %d %v", toolgate.ScopeRuleID, resp.StatusCode, body)
	}
	// Without the header the policy's own grant applies.
	resp, _ = post(t, srv.URL+"/v1/tools/list_invoices", map[string]any{"status": "open"}, nil)
	if resp.StatusCode != http.StatusOK || len(*executed) != 1 {
		t.Fatalf("expected the policy grant to allow, got %d", resp.StatusCode)
	}
}

func TestUpstreamDownIsRecorded(t *testing.T) {
	srv, upstream, _, _ := setup(t)
	defer srv.Close()
	upstream.Close()
	resp, body := post(t, srv.URL+"/v1/tools/list_invoices", map[string]any{}, nil)
	if resp.StatusCode != http.StatusBadGateway || body["hash"] == "" {
		t.Fatalf("expected 502 with a recorded artifact, got %d %v", resp.StatusCode, body)
	}
}
