package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/parlakisik/agent-exchange/internal/toolgate"
)

// The operator endpoints are not part of the gated agent's surface. An agent
// that can settle its own holds defeats the escalate effect outright, and one
// that can read the chain reads every other call's argument values.
func TestOperatorEndpointsRefuseTheGatedAgent(t *testing.T) {
	srv, upstream, calls, _ := setup(t)
	defer srv.Close()
	defer upstream.Close()

	// Drive C07 (a bank_account change) to a hold and take its hash, exactly
	// as the agent would see it in the 202 body.
	var hold string
	for _, c := range calls {
		if c.ID != "C07" {
			continue
		}
		resp, body := post(t, srv.URL+"/v1/tools/"+c.Tool, c.Args, map[string]string{"X-AEX-Call-ID": c.ID})
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("C07 should be held, got %d %v", resp.StatusCode, body)
		}
		hold, _ = body["hold"].(string)
	}
	if hold == "" {
		t.Fatal("no hold hash in the 202 body")
	}

	t.Run("self-approval with no credential", func(t *testing.T) {
		resp, _ := post(t, srv.URL+"/v1/holds/"+hold, map[string]any{"approver": "anyone", "approved": true}, nil)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("holding agent settled its own hold: got %d, want 401", resp.StatusCode)
		}
	})

	t.Run("self-approval with a guessed credential", func(t *testing.T) {
		resp, _ := post(t, srv.URL+"/v1/holds/"+hold,
			map[string]any{"approver": "anyone", "approved": true},
			map[string]string{"Authorization": "Bearer not-the-token"})
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", resp.StatusCode)
		}
	})

	t.Run("records without a credential", func(t *testing.T) {
		for _, path := range []string{"/v1/records", "/v1/records/verify"} {
			resp, body := getAs(t, srv.URL+path, nil)
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("%s: got %d, want 401", path, resp.StatusCode)
			}
			if strings.Contains(string(body), "ACME-ACCT") {
				t.Errorf("%s leaked argument values to an unauthenticated caller", path)
			}
		}
	})

	t.Run("the operator still gets through", func(t *testing.T) {
		resp, _ := post(t, srv.URL+"/v1/holds/"+hold,
			map[string]any{"approver": "user:ap.manager@corp.example", "approved": true}, operatorHeaders())
		if resp.StatusCode != http.StatusOK {
			t.Errorf("operator resolve: got %d, want 200", resp.StatusCode)
		}
	})
}

// With no token configured the operator endpoints are unavailable rather than
// open, so a deployment that forgot to set one does not quietly expose them.
func TestOperatorEndpointsFailClosedWithoutAToken(t *testing.T) {
	policy, err := toolgate.LoadPolicy("../../../internal/toolgate/testdata/policy.json")
	if err != nil {
		t.Fatal(err)
	}
	gate, err := toolgate.New(policy)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(gate, "http://127.0.0.1:1", "/tools", time.Second))
	defer srv.Close()

	resp, _ := getAs(t, srv.URL+"/v1/records", operatorHeaders())
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("got %d, want 503 when no OPERATOR_TOKEN is configured", resp.StatusCode)
	}
}

// Argument values are off the records response unless the deployment asks for
// them: in ModeFull an artifact carries payment amounts and account numbers.
func TestRecordsRedactArgsByDefault(t *testing.T) {
	policy, err := toolgate.LoadPolicy("../../../internal/toolgate/testdata/policy.json")
	if err != nil {
		t.Fatal(err)
	}
	gate, err := toolgate.New(policy)
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()
	srv := httptest.NewServer(New(gate, upstream.URL, "/tools", 5*time.Second, WithOperatorToken(testOperatorToken)))
	defer srv.Close()

	post(t, srv.URL+"/v1/tools/send_payment", map[string]any{
		"amount": 100, "invoice_id": "INV-1042", "recipient_account": "ACME-ACCT-001"}, nil)

	_, body := getAs(t, srv.URL+"/v1/records", operatorHeaders())
	if strings.Contains(string(body), "ACME-ACCT-001") {
		t.Errorf("records served argument values with EXPOSE_ARGS off: %s", body)
	}
	var recs []toolgate.Artifact
	if err := json.Unmarshal(body, &recs); err != nil || len(recs) == 0 {
		t.Fatalf("records should still be readable: %v %s", err, body)
	}
	if recs[0].Hash == "" || recs[0].Tool != "send_payment" {
		t.Errorf("redaction should drop only the args: %+v", recs[0])
	}
}

// The gate re-marshals arguments before forwarding, so decoding has to keep
// integers exact. Through float64 an account number past 2^53 arrives at the
// tool as a different number than the agent sent.
func TestLargeIntegersReachTheToolUnchanged(t *testing.T) {
	policy, err := toolgate.LoadPolicy("../../../internal/toolgate/testdata/policy.json")
	if err != nil {
		t.Fatal(err)
	}
	gate, err := toolgate.New(policy)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 512)
		n, _ := r.Body.Read(b)
		got = string(b[:n])
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()
	srv := httptest.NewServer(New(gate, upstream.URL, "/tools", 5*time.Second))
	defer srv.Close()

	const acct = "12345678901234567890"
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/tools/list_invoices",
		strings.NewReader(`{"account":`+acct+`}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if !strings.Contains(got, acct) {
		t.Errorf("upstream received %s, want the account number %s unchanged", got, acct)
	}
}
