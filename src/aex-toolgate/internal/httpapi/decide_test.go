package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/parlakisik/agent-exchange/internal/toolgate"
)

// The twelve calls through /v1/decide reach the same decisions as through
// /v1/tools, but nothing is forwarded, nothing is held, and every record says
// the gate did not act.
func TestDecideOnlyRulesWithoutExecuting(t *testing.T) {
	srv, upstream, calls, executed := setup(t)
	defer srv.Close()
	defer upstream.Close()

	want := map[string]string{
		"C01": "allow", "C02": "allow", "C03": "allow", "C04": "allow", "C05": "deny", "C06": "deny",
		"C07": "escalate", "C08": "allow", "C09": "deny", "C10": "deny", "C11": "deny", "C12": "deny",
	}
	var heldHash string
	for _, c := range calls {
		resp, body := post(t, srv.URL+"/v1/decide/"+c.Tool, c.Args,
			map[string]string{"Authorization": "Bearer " + testOperatorToken, "X-AEX-Call-ID": c.ID})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: got %d, want 200 whatever the decision", c.ID, resp.StatusCode)
		}
		if got := body["decision"]; got != want[c.ID] || resp.Header.Get("X-Toolgate-Decision") != want[c.ID] {
			t.Errorf("%s: decision %v (header %q), want %s", c.ID, got, resp.Header.Get("X-Toolgate-Decision"), want[c.ID])
		}
		if body["outcome"] != toolgate.OutcomeDecideOnly {
			t.Errorf("%s: outcome %v, want the decide-only outcome", c.ID, body["outcome"])
		}
		if c.ID == "C07" {
			heldHash, _ = body["hash"].(string)
		}
	}
	if len(*executed) != 0 {
		t.Errorf("decide-only forwarded calls to the tool: %v", *executed)
	}

	// The escalated call left no hold behind to settle.
	resp, _ := post(t, srv.URL+"/v1/holds/"+heldHash, map[string]any{"approver": "user:x", "approved": true}, operatorHeaders())
	if resp.StatusCode == http.StatusOK {
		t.Error("a decide-only escalation must not create a hold")
	}

	_, raw := getAs(t, srv.URL+"/v1/records/verify", operatorHeaders())
	var v map[string]any
	_ = json.Unmarshal(raw, &v)
	if v["ok"] != true || v["records"] != float64(12) {
		t.Errorf("chain: %v, want ok with 12 records", v)
	}
}

// Decide-only is an operator endpoint: the gated agent must not be able to ask
// the gate what would pass.
func TestDecideOnlyRefusesTheGatedAgent(t *testing.T) {
	srv, upstream, _, _ := setup(t)
	defer srv.Close()
	defer upstream.Close()

	resp, _ := post(t, srv.URL+"/v1/decide/send_payment", map[string]any{"invoice_id": "INV-1042", "amount": 1.0}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no credential: got %d, want 401", resp.StatusCode)
	}
	_, raw := getAs(t, srv.URL+"/v1/records/verify", operatorHeaders())
	var v map[string]any
	_ = json.Unmarshal(raw, &v)
	if v["records"] != float64(0) {
		t.Errorf("a refused decide request must leave no record, got %v", v["records"])
	}
}
