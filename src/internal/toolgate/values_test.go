package toolgate

import (
	"encoding/json"
	"strings"
	"testing"
)

func f64(v float64) *float64 { return &v }

// decideWith builds a one-tool gate around rules and decides one call.
func decideWith(t *testing.T, args string, rules ...Rule) Decision {
	t.Helper()
	p := Policy{Provider: "p", GrantedScopes: []string{"s"}, ToolScopes: map[string]string{"t": "s"}, Rules: rules}
	g, err := New(p)
	if err != nil {
		t.Fatalf("policy rejected: %v", err)
	}
	var a map[string]any
	dec := json.NewDecoder(strings.NewReader(args))
	dec.UseNumber() // as the HTTP service decodes
	if err := dec.Decode(&a); err != nil {
		t.Fatal(err)
	}
	return g.Decide(Call{Tool: "t", Args: a})
}

type valueCase struct {
	name, args string
	want       string // allow, deny, or "unverifiable" (deny with a cannot-verify message)
}

func runCases(t *testing.T, rule Rule, cases []valueCase) {
	t.Helper()
	rule.ID, rule.Tool = "R", "t"
	for _, c := range cases {
		d := decideWith(t, c.args, rule)
		got := d.Outcome
		if d.Outcome == DecisionDeny && strings.HasPrefix(d.Message, "cannot verify:") {
			got = "unverifiable"
		}
		if got != c.want {
			t.Errorf("%s %s: got %s (%s), want %s", rule.Kind, c.name, got, d.Message, c.want)
		}
	}
}

func TestFloor(t *testing.T) {
	runCases(t, Rule{Kind: KindFloor, Arg: "qty", Min: f64(1)}, []valueCase{
		{"at floor", `{"qty": 1}`, DecisionAllow},
		{"below", `{"qty": 0}`, DecisionDeny},
		{"negative", `{"qty": -5}`, DecisionDeny},
		{"quoted number", `{"qty": "7"}`, "unverifiable"},
		{"absent", `{}`, "unverifiable"},
	})
}

func TestDenylistIgnoresCase(t *testing.T) {
	runCases(t, Rule{Kind: KindDenylist, Arg: "acct", Denied: []string{"NEW-ACCT-9911"}}, []valueCase{
		{"other", `{"acct": "ACME-ACCT-001"}`, DecisionAllow},
		{"denied", `{"acct": "NEW-ACCT-9911"}`, DecisionDeny},
		{"denied, other case", `{"acct": "new-acct-9911"}`, DecisionDeny},
		{"padded", `{"acct": " NEW-ACCT-9911"}`, "unverifiable"},
		{"two values", `{"acct": "A,NEW-ACCT-9911"}`, "unverifiable"},
	})
}

func TestPatternMatchesWholeValue(t *testing.T) {
	runCases(t, Rule{Kind: KindPattern, Arg: "id", Pattern: `ACME-ACCT-\d{3}`}, []valueCase{
		{"matches", `{"id": "ACME-ACCT-001"}`, DecisionAllow},
		{"prefix only", `{"id": "ACME-ACCT-0012"}`, DecisionDeny},
		{"embedded", `{"id": "xACME-ACCT-001"}`, DecisionDeny},
		{"list", `{"id": ["ACME-ACCT-001"]}`, "unverifiable"},
	})
}

func TestDomain(t *testing.T) {
	runCases(t, Rule{Kind: KindDomain, Arg: "to", Allowed: []string{"corp.example"}}, []valueCase{
		{"email", `{"to": "cfo@corp.example"}`, DecisionAllow},
		{"email, subdomain", `{"to": "cfo@eu.corp.example"}`, DecisionAllow},
		{"email, upper case", `{"to": "CFO@CORP.EXAMPLE"}`, DecisionAllow},
		{"url", `{"to": "https://files.corp.example/x"}`, DecisionAllow},
		{"bare host", `{"to": "corp.example"}`, DecisionAllow},
		{"lookalike suffix", `{"to": "cfo@evilcorp.example"}`, DecisionDeny},
		{"lookalike parent", `{"to": "cfo@corp.example.evil.example"}`, DecisionDeny},
		{"url userinfo trick", `{"to": "https://corp.example@evil.example/x"}`, DecisionDeny},
		{"two @", `{"to": "a@corp.example@evil.example"}`, "unverifiable"},
		{"homoglyph", `{"to": "cfo@corp.exаmple"}`, "unverifiable"},
		{"url without host", `{"to": "file:///etc/passwd"}`, "unverifiable"},
	})
}

func TestMaxItems(t *testing.T) {
	runCases(t, Rule{Kind: KindMaxItems, Arg: "to", Max: 2}, []valueCase{
		{"two", `{"to": ["a", "b"]}`, DecisionAllow},
		{"three", `{"to": ["a", "b", "c"]}`, DecisionDeny},
		{"lone value", `{"to": "a"}`, DecisionAllow},
		{"null", `{"to": null}`, "unverifiable"},
	})
	runCases(t, Rule{Kind: KindMaxItems, Arg: "items[*].id", Max: 1}, []valueCase{
		{"one via path", `{"items": [{"id": 1}]}`, DecisionAllow},
		{"two via path", `{"items": [{"id": 1}, {"id": 2}]}`, DecisionDeny},
	})
}

// each applies a single-value rule to every element; without it a list fails
// closed as before.
func TestEachElement(t *testing.T) {
	rule := Rule{Kind: KindDomain, Arg: "to", Allowed: []string{"corp.example"}, Each: true}
	runCases(t, rule, []valueCase{
		{"all inside", `{"to": ["a@corp.example", "b@eu.corp.example"]}`, DecisionAllow},
		{"one outside", `{"to": ["a@corp.example", "x@evil.example"]}`, DecisionDeny},
		{"lone value", `{"to": "a@corp.example"}`, DecisionAllow},
		{"empty list", `{"to": []}`, DecisionAllow},
	})
	rule.Each = false
	runCases(t, rule, []valueCase{{"list without each", `{"to": ["a@corp.example"]}`, "unverifiable"}})
}

func TestNestedPaths(t *testing.T) {
	runCases(t, Rule{Kind: KindCeiling, Arg: "payment.amount", Max: 100}, []valueCase{
		{"inside", `{"payment": {"amount": 50}}`, DecisionAllow},
		{"over", `{"payment": {"amount": 500}}`, DecisionDeny},
		{"missing field", `{"payment": {"currency": "USD"}}`, "unverifiable"},
		{"not an object", `{"payment": 500}`, "unverifiable"},
	})
	runCases(t, Rule{Kind: KindCeiling, Arg: "items[*].amount", Max: 100}, []valueCase{
		{"every item inside", `{"items": [{"amount": 10}, {"amount": 90}]}`, DecisionAllow},
		{"one item over", `{"items": [{"amount": 10}, {"amount": 900}]}`, DecisionDeny},
		{"one item without it", `{"items": [{"amount": 10}, {"note": "x"}]}`, "unverifiable"},
		{"not a list", `{"items": {"amount": 10}}`, "unverifiable"},
	})
	// An argument literally named with a dot is found by its exact name first.
	runCases(t, Rule{Kind: KindCeiling, Arg: "a.b", Max: 1}, []valueCase{
		{"exact name", `{"a.b": 5}`, DecisionDeny},
	})
}

func TestCompare(t *testing.T) {
	runCases(t, Rule{Kind: KindCompare, Arg: "refund", Op: "le", MatchArg: "charge"}, []valueCase{
		{"less", `{"refund": 10, "charge": 20}`, DecisionAllow},
		{"equal", `{"refund": 20, "charge": 20}`, DecisionAllow},
		{"more", `{"refund": 30, "charge": 20}`, DecisionDeny},
		{"text", `{"refund": "30", "charge": 20}`, "unverifiable"},
		{"other absent", `{"refund": 30}`, "unverifiable"},
	})
	runCases(t, Rule{Kind: KindCompare, Arg: "from", Op: "ne", MatchArg: "to"}, []valueCase{
		{"different", `{"from": "A-1", "to": "B-2"}`, DecisionAllow},
		{"same", `{"from": "A-1", "to": "A-1"}`, DecisionDeny},
	})
	runCases(t, Rule{Kind: KindCompare, Arg: "order.total", Op: "eq", MatchArg: "payment.amount"}, []valueCase{
		{"nested equal", `{"order": {"total": 5}, "payment": {"amount": 5}}`, DecisionAllow},
		{"nested differ", `{"order": {"total": 5}, "payment": {"amount": 6}}`, DecisionDeny},
	})
}

func TestValueKindValidation(t *testing.T) {
	bad := []struct {
		rule  Rule
		error string
	}{
		{Rule{Kind: KindFloor, Arg: "x"}, "needs min"},
		{Rule{Kind: KindDenylist, Arg: "x"}, "needs a denied list"},
		{Rule{Kind: KindPattern, Arg: "x"}, "needs a pattern"},
		{Rule{Kind: KindPattern, Arg: "x", Pattern: "("}, "bad pattern"},
		{Rule{Kind: KindDomain, Arg: "x"}, "allowed list of domains"},
		{Rule{Kind: KindDomain, Arg: "x", Allowed: []string{"@corp.example"}}, "not a bare lower-case domain"},
		{Rule{Kind: KindDomain, Arg: "x", Allowed: []string{"Corp.example"}}, "not a bare lower-case domain"},
		{Rule{Kind: KindMaxItems, Arg: "x"}, "max of at least 1"},
		{Rule{Kind: KindCompare, Arg: "x", MatchArg: "y", Op: "lte"}, "needs op"},
		{Rule{Kind: KindCompare, Arg: "x", Op: "lt"}, "needs match_arg"},
		{Rule{Kind: KindCompare, Arg: "x[*].a", Op: "lt", MatchArg: "y"}, "[*] is not allowed"},
		{Rule{Kind: KindMaxItems, Arg: "x", Max: 2, Each: true}, "cannot apply to each"},
		{Rule{Kind: KindCeiling, Arg: "a..b", Max: 1}, "not a valid argument path"},
		{Rule{Kind: KindCeiling, Arg: "a[*]b", Max: 1}, "not a valid argument path"},
	}
	for _, b := range bad {
		b.rule.ID, b.rule.Tool = "R", "t"
		p := Policy{Provider: "p", GrantedScopes: []string{"s"}, ToolScopes: map[string]string{"t": "s"}, Rules: []Rule{b.rule}}
		err := p.Validate()
		if err == nil || !strings.Contains(err.Error(), b.error) {
			t.Errorf("%s %+v: got %v, want an error containing %q", b.rule.Kind, b.rule, err, b.error)
		}
	}
}
