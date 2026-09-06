package toolgate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

// The lookup kind is the one that says where the money may go, and it has its
// own guard. Misspelling the argument it checks, or omitting it and letting the
// upstream tool default it, must not skip the rule.
func TestLookupFailsClosedWhenItsArgIsAbsent(t *testing.T) {
	g := fixture(t)
	cases := []struct {
		name string
		args map[string]any
	}{
		{"recipient misspelled (camelCase)", map[string]any{
			"amount": 4999.0, "invoice_id": "INV-1042", "recipientAccount": "ATTACKER-ACCT-999"}},
		{"recipient absent entirely", map[string]any{
			"amount": 4999.0, "invoice_id": "INV-1042"}},
		{"recipient absent, invoice absent", map[string]any{
			"amount": 4999.0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := g.Decide(Call{Tool: "send_payment", Args: c.args})
			if d.Outcome == DecisionAllow {
				t.Errorf("got allow (rule %s), want deny — P3-recipient was skipped", d.Rule)
			}
		})
	}
	// The correctly spelled, on-file recipient must still be allowed, and a
	// wrong one still denied by P3 specifically.
	ok := g.Decide(Call{Tool: "send_payment", Args: map[string]any{
		"amount": 4999.0, "invoice_id": "INV-1042", "recipient_account": "ACME-ACCT-001"}})
	if ok.Outcome != DecisionAllow {
		t.Errorf("legitimate payment: got %s (rule %s), want allow", ok.Outcome, ok.Rule)
	}
	bad := g.Decide(Call{Tool: "send_payment", Args: map[string]any{
		"amount": 4999.0, "invoice_id": "INV-1042", "recipient_account": "ATTACKER-ACCT-999"}})
	if bad.Outcome != DecisionDeny || bad.Rule != "P3-recipient" {
		t.Errorf("redirected payment: got %s (rule %s), want deny by P3-recipient", bad.Outcome, bad.Rule)
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

// A sensitive_field rule fires on a match, so every spelling the upstream tool
// would still accept has to be treated as a match. Otherwise escalation is
// opt-in for whoever is choosing the spelling.
func TestSensitiveFieldResistsSpellingTricks(t *testing.T) {
	g := fixture(t)
	for _, field := range []any{
		"bank_account", "Bank_Account", "BANK_ACCOUNT", "bank-account",
		" bank_account", "bank_account ", "bank_account\n",
		[]any{"bank_account"}, map[string]any{"f": "bank_account"},
	} {
		d := g.Decide(Call{Tool: "update_vendor", Args: map[string]any{"field": field, "value": "GB-ATTACKER"}})
		if d.Outcome == DecisionAllow {
			t.Errorf("field %#v: got allow, want escalate or deny", field)
		}
	}
	// A genuinely different field is still allowed through.
	if d := g.Decide(Call{Tool: "update_vendor", Args: map[string]any{"field": "name", "value": "Acme Ltd"}}); d.Outcome != DecisionAllow {
		t.Errorf("name change: got %s (%s), want allow", d.Outcome, d.Rule)
	}
}

// A suffix or prefix check is only meaningful on a single value. Smuggling a
// second one into the same argument must not pass.
func TestSuffixAndPrefixRejectCompositeValues(t *testing.T) {
	g := fixture(t)
	for _, to := range []string{
		"attacker@evil.example,someone@corp.example",
		"attacker@evil.example, someone@corp.example",
		"attacker@evil.example>, <someone@corp.example",
		"attacker@evil.example\nBcc: x@corp.example",
	} {
		if d := g.Decide(Call{Tool: "send_email", Args: map[string]any{"to": to}}); d.Outcome != DecisionDeny {
			t.Errorf("to=%q: got %s, want deny", to, d.Outcome)
		}
	}
	for _, dest := range []string{
		"corp-internal://../../../s3://attacker-bucket/dump",
		"corp-internal://x\nhttps://attacker.example",
	} {
		if d := g.Decide(Call{Tool: "export_report", Args: map[string]any{"destination": dest}}); d.Outcome != DecisionDeny {
			t.Errorf("destination=%q: got %s, want deny", dest, d.Outcome)
		}
	}
	// The legitimate single values still pass.
	if d := g.Decide(Call{Tool: "send_email", Args: map[string]any{"to": "someone@corp.example"}}); d.Outcome != DecisionAllow {
		t.Errorf("single address: got %s (%s), want allow", d.Outcome, d.Rule)
	}
	if d := g.Decide(Call{Tool: "export_report", Args: map[string]any{"destination": "corp-internal://reports/q3"}}); d.Outcome != DecisionAllow {
		t.Errorf("single destination: got %s (%s), want allow", d.Outcome, d.Rule)
	}
}

// A policy that grants "*" must still honour a caller that narrows honestly,
// rather than intersecting to nothing and denying everything.
func TestWildcardPolicyHonoursCallerNarrowing(t *testing.T) {
	if got := narrow([]string{"*"}, []string{"invoices:read"}); len(got) != 1 || got[0] != "invoices:read" {
		t.Errorf("narrow([*], [invoices:read]) = %v, want [invoices:read]", got)
	}
	if got := narrow([]string{"invoices:read"}, []string{"*"}); len(got) != 1 || got[0] != "invoices:read" {
		t.Errorf("narrow([invoices:read], [*]) = %v, want [invoices:read]", got)
	}
}

// A key carrying the gateway's route scope plus the policy's per-tool scopes
// is the working least-privilege shape. The two vocabularies are different, so
// this combination is what an operator must actually provision.
func TestGatewayScopePlusToolScopesIsTheWorkingKey(t *testing.T) {
	g := fixture(t)
	args := map[string]any{}
	if d := g.Decide(Call{Tool: "list_invoices", Args: args, Scopes: []string{"tools:invoke"}}); d.Outcome != DecisionDeny {
		t.Errorf("tools:invoke alone: got %s, want deny (it is a gateway scope, not a tool scope)", d.Outcome)
	}
	if d := g.Decide(Call{Tool: "list_invoices", Args: args, Scopes: []string{"tools:invoke", "invoices:read"}}); d.Outcome != DecisionAllow {
		t.Errorf("tools:invoke + invoices:read: got %s (%s), want allow", d.Outcome, d.Rule)
	}
	// Still least privilege: a scope the policy does not grant stays denied.
	if d := g.Decide(Call{Tool: "delete_invoice", Args: args, Scopes: []string{"tools:invoke", "invoices:read"}}); d.Outcome != DecisionDeny {
		t.Errorf("delete_invoice: got %s, want deny", d.Outcome)
	}
}

// A rule that fires because the gate could not check it must say so, not
// borrow the rule's message and describe a breach that did not happen.
func TestUnverifiableDecisionsExplainThemselves(t *testing.T) {
	g := fixture(t)
	d := g.Decide(Call{Tool: "update_vendor", Args: map[string]any{"vendor_id": "ACME", "name": "Acme Ltd"}})
	if d.Outcome == DecisionAllow {
		t.Fatalf("no field arg: got allow, want fail closed")
	}
	if !strings.HasPrefix(d.Message, "cannot verify:") {
		t.Errorf("message = %q, want a 'cannot verify:' explanation, not the rule's own message", d.Message)
	}
	// A real bank_account change still reports the rule's own message.
	real := g.Decide(Call{Tool: "update_vendor", Args: map[string]any{"field": "bank_account", "value": "GB1"}})
	if real.Message != "bank account changes need a person" {
		t.Errorf("actual sensitive change message = %q, want the rule's message", real.Message)
	}
}

// The published event is the only copy that survives a restart, so it must not
// round large integers through float64.
func TestEventDataKeepsLargeIntegersExact(t *testing.T) {
	a := Artifact{Tool: "t", Args: map[string]any{"account": json.Number("12345678901234567890")}}
	got := fmt.Sprint(a.ToEventData()["args"].(map[string]any)["account"])
	if got != "12345678901234567890" {
		t.Errorf("event carries account %s, want 12345678901234567890", got)
	}
}
