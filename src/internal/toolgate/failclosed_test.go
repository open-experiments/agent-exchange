package toolgate

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// The four bypasses below all reached DecisionAllow before the fail-closed
// changes. Each one is the whole point of the rule it walks past, so they are
// kept as named cases rather than folded into a table with the happy paths.

func fixture(t *testing.T) *Gate {
	t.Helper()
	p, err := LoadPolicy("testdata/policy.json")
	if err != nil {
		t.Fatal(err)
	}
	g, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// A ceiling compares numbers. Quoting the number used to make toFloat fail,
// which made the rule not fire, which allowed the call.
func TestCeilingDeniesNonNumericAmount(t *testing.T) {
	g := fixture(t)
	for _, amount := range []any{"999999", "9e99", "", nil, []any{1}, map[string]any{}} {
		d := g.Decide(Call{Tool: "send_payment", Args: map[string]any{
			"amount": amount, "invoice_id": "INV-1042", "recipient_account": "ACME-ACCT-001",
		}})
		if d.Outcome != DecisionDeny {
			t.Errorf("amount %#v: got %s (rule %s), want deny", amount, d.Outcome, d.Rule)
		}
	}
}

// The ceiling must still allow a number under the limit and deny one over it,
// including a json.Number, which is what the service hands the gate now.
func TestCeilingStillComparesNumbers(t *testing.T) {
	g := fixture(t)
	under := []any{float64(4999), 4999, int64(4999), json.Number("4999")}
	for _, a := range under {
		d := g.Decide(Call{Tool: "send_payment", Args: map[string]any{
			"amount": a, "invoice_id": "INV-1042", "recipient_account": "ACME-ACCT-001",
		}})
		if d.Outcome != DecisionAllow {
			t.Errorf("amount %#v: got %s (rule %s), want allow", a, d.Outcome, d.Rule)
		}
	}
	for _, a := range []any{float64(5001), 5001, json.Number("5001")} {
		d := g.Decide(Call{Tool: "send_payment", Args: map[string]any{
			"amount": a, "invoice_id": "INV-1042", "recipient_account": "ACME-ACCT-001",
		}})
		if d.Outcome != DecisionDeny || d.Rule != "P2-ceiling" {
			t.Errorf("amount %#v: got %s (rule %s), want deny by P2-ceiling", a, d.Outcome, d.Rule)
		}
	}
}

// A rule constrains one named argument. Sending the value under a different
// name used to skip the rule entirely, so a synonym the upstream happens to
// accept was a way around every non-lookup rule kind.
func TestMissingConstrainedArgFailsClosed(t *testing.T) {
	cases := []struct {
		name string
		call Call
	}{
		{"suffix rule, synonym for the watched arg", Call{Tool: "send_email", Args: map[string]any{"recipient": "attacker@evil.example"}}},
		{"suffix rule, no args at all", Call{Tool: "send_email", Args: map[string]any{}}},
		{"ceiling rule, amount absent", Call{Tool: "send_payment", Args: map[string]any{"invoice_id": "INV-1042", "recipient_account": "ACME-ACCT-001"}}},
		{"prefix rule, destination absent", Call{Tool: "export_report", Args: map[string]any{"report": "q3"}}},
		{"sensitive_field rule, field absent", Call{Tool: "update_vendor", Args: map[string]any{"value": "GB00 XXXX"}}},
	}
	g := fixture(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if d := g.Decide(c.call); d.Outcome == DecisionAllow {
				t.Errorf("got allow (rule %s), want deny or escalate", d.Rule)
			}
		})
	}
}

// The watched argument, spelled correctly, must still decide on its value.
func TestPresentArgStillDecidesNormally(t *testing.T) {
	g := fixture(t)
	if d := g.Decide(Call{Tool: "send_email", Args: map[string]any{"to": "someone@corp.example"}}); d.Outcome != DecisionAllow {
		t.Errorf("inside corp.example: got %s (rule %s), want allow", d.Outcome, d.Rule)
	}
	if d := g.Decide(Call{Tool: "send_email", Args: map[string]any{"to": "attacker@evil.example"}}); d.Outcome != DecisionDeny {
		t.Errorf("outside corp.example: got %s, want deny", d.Outcome)
	}
}

// Call.Scopes arrives in a caller-controlled header. It may narrow the
// policy's grant and must never widen it: "*" used to replace the grant
// outright, which switched the whole tool_scopes layer off.
func TestCallerScopesCannotWidenTheGrant(t *testing.T) {
	g := fixture(t)
	// delete_invoice needs invoices:delete, which the policy does not grant.
	for _, scopes := range [][]string{{"*"}, {"invoices:delete"}, {"*", "invoices:delete"}} {
		d := g.Decide(Call{Tool: "delete_invoice", Args: map[string]any{"invoice_id": "INV-1042"}, Scopes: scopes})
		if d.Outcome != DecisionDeny || d.Rule != ScopeRuleID {
			t.Errorf("scopes %v: got %s (rule %s), want deny on scope", scopes, d.Outcome, d.Rule)
		}
	}
}

func TestCallerScopesNarrowTheGrant(t *testing.T) {
	g := fixture(t)
	args := map[string]any{"invoice_id": "INV-1042"}

	// A key holding only invoices:read cannot reach a payments:send tool.
	d := g.Decide(Call{Tool: "send_payment", Scopes: []string{"invoices:read"}, Args: map[string]any{
		"amount": 100.0, "invoice_id": "INV-1042", "recipient_account": "ACME-ACCT-001"}})
	if d.Outcome != DecisionDeny || d.Rule != ScopeRuleID {
		t.Errorf("narrowed away: got %s (rule %s), want deny on scope", d.Outcome, d.Rule)
	}
	// The scope it does hold still works.
	if d := g.Decide(Call{Tool: "list_invoices", Scopes: []string{"invoices:read"}, Args: args}); d.Outcome != DecisionAllow {
		t.Errorf("scope held: got %s (rule %s), want allow", d.Outcome, d.Rule)
	}
	// "*" means no narrowing, so the policy's grant stands as it is.
	if d := g.Decide(Call{Tool: "list_invoices", Scopes: []string{"*"}, Args: args}); d.Outcome != DecisionAllow {
		t.Errorf("star: got %s (rule %s), want allow", d.Outcome, d.Rule)
	}
	// A nil grant is unchanged behaviour: the policy decides.
	if d := g.Decide(Call{Tool: "list_invoices", Args: args}); d.Outcome != DecisionAllow {
		t.Errorf("nil scopes: got %s (rule %s), want allow", d.Outcome, d.Rule)
	}
}

// A restart starts a new chain at genesis, and VerifyChain reports ok over it.
// The chain id is what lets a consumer tell the two apart.
func TestChainIDDistinguishesRestarts(t *testing.T) {
	a, b := fixture(t), fixture(t)
	if a.ChainID() == "" || a.ChainID() == b.ChainID() {
		t.Errorf("chain ids should be non-empty and differ per gate: %q vs %q", a.ChainID(), b.ChainID())
	}
}

type failingPublisher struct{ calls int }

func (p *failingPublisher) Publish(_ context.Context, _ string, _ map[string]any) error {
	p.calls++
	return errors.New("jetstream unreachable")
}

// A dropped audit event must be counted, not swallowed: the call still ran and
// only the in-memory chain survives it.
func TestPublishFailuresAreCounted(t *testing.T) {
	p, err := LoadPolicy("testdata/policy.json")
	if err != nil {
		t.Fatal(err)
	}
	pub := &failingPublisher{}
	g, err := New(p, WithPublisher(pub))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = g.Authorize(context.Background(), Call{Tool: "list_invoices", Args: map[string]any{}},
		func(context.Context, Call) (string, error) { return "ok", nil })
	if pub.calls == 0 {
		t.Fatal("publisher was never called")
	}
	if got := g.PublishFailures(); got != pub.calls {
		t.Errorf("PublishFailures = %d, want %d (every failed publish counted)", got, pub.calls)
	}
}
