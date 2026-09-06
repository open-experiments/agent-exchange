package toolgate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
)

type testCall struct {
	ID   string         `json:"id"`
	Tool string         `json:"tool"`
	Args map[string]any `json:"args"`
	Note string         `json:"note"`
}

func loadCalls(t *testing.T) []testCall {
	t.Helper()
	b, err := os.ReadFile("testdata/calls.json")
	if err != nil {
		t.Fatal(err)
	}
	var calls []testCall
	if err := json.Unmarshal(b, &calls); err != nil {
		t.Fatal(err)
	}
	return calls
}

func loadPolicy(t *testing.T) Policy {
	t.Helper()
	p, err := LoadPolicy("testdata/policy.json")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func asCall(tc testCall) Call {
	return Call{ID: tc.ID, Tool: tc.Tool, Args: tc.Args, Principal: "user:j.doe@corp.example",
		AgentID: "ap-assistant-v2", SessionID: "sess-7f3a", TenantID: "tenant_123",
		ContractID: "contract_789", CertID: "cert_ap_assistant_v2", TraceID: "req-" + tc.ID}
}

type memPublisher struct {
	mu     sync.Mutex
	events []string
}

func (m *memPublisher) Publish(_ context.Context, eventType string, _ map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, eventType)
	return nil
}

func fixedClock() func() time.Time {
	t0 := time.Date(2026, 9, 5, 21, 39, 30, 0, time.UTC)
	i := 0
	return func() time.Time { i++; return t0.Add(time.Duration(i) * time.Millisecond) }
}

// The expected decisions are the run-1 decision matrix of the harness study.
var expectFull = map[string][2]string{
	"C01": {DecisionAllow, "P1-scope"}, "C02": {DecisionAllow, "P1-scope"}, "C03": {DecisionAllow, "P1-scope"},
	"C04": {DecisionAllow, "P1-scope"}, "C05": {DecisionDeny, "P2-ceiling"}, "C06": {DecisionDeny, "P3-recipient"},
	"C07": {DecisionEscalate, "P4-sensitive"}, "C08": {DecisionAllow, "P1-scope"}, "C09": {DecisionDeny, "P5-email-domain"},
	"C10": {DecisionDeny, "P6-export-dest"}, "C11": {DecisionDeny, "P1-scope"}, "C12": {DecisionDeny, "P1-scope"},
}

func TestDecideFullMode(t *testing.T) {
	g, err := New(loadPolicy(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range loadCalls(t) {
		d := g.Decide(asCall(tc))
		want := expectFull[tc.ID]
		if d.Outcome != want[0] || d.Rule != want[1] {
			t.Errorf("%s (%s): got %s/%s, want %s/%s", tc.ID, tc.Note, d.Outcome, d.Rule, want[0], want[1])
		}
	}
}

func TestDecideScopeMode(t *testing.T) {
	g, err := New(loadPolicy(t), WithMode(ModeScope))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range loadCalls(t) {
		d := g.Decide(asCall(tc))
		want := DecisionAllow
		if tc.ID == "C11" || tc.ID == "C12" {
			want = DecisionDeny
		}
		if d.Outcome != want {
			t.Errorf("%s: scope mode got %s, want %s", tc.ID, d.Outcome, want)
		}
		if d.Rule != ScopeRuleID {
			t.Errorf("%s: scope mode should record the scope rule, got %q", tc.ID, d.Rule)
		}
	}
}

func TestScopeModeRecordsNoValues(t *testing.T) {
	// The finding the component exists for: a scope gate's record of the
	// redirected payment (C06) is identical, field for field, to the record of
	// the legitimate payment (C04), because it never sees argument values.
	g, _ := New(loadPolicy(t), WithMode(ModeScope), WithClock(fixedClock()))
	ctx := context.Background()
	exec := func(context.Context, Call) (string, error) { return "ok", nil }
	var recs = map[string]Artifact{}
	for _, tc := range loadCalls(t) {
		if tc.ID == "C04" || tc.ID == "C06" {
			a, _ := g.Authorize(ctx, asCall(tc), exec)
			recs[tc.ID] = a
		}
	}
	a, b := recs["C04"], recs["C06"]
	if a.Args != nil || b.Args != nil {
		t.Fatalf("scope mode must not record argument values")
	}
	if a.Decision != b.Decision || a.Rule != b.Rule || a.Scope != b.Scope || a.Tool != b.Tool || a.Outcome != b.Outcome {
		t.Fatalf("scope-mode records of C04 and C06 should match on every field but time and hash")
	}
}

func TestAuthorizeExecutesOnlyAllowedCalls(t *testing.T) {
	g, _ := New(loadPolicy(t), WithClock(fixedClock()))
	ctx := context.Background()
	executed := map[string]bool{}
	exec := func(_ context.Context, c Call) (string, error) {
		executed[c.ID] = true
		return "side effect applied", nil
	}
	for _, tc := range loadCalls(t) {
		_, err := g.Authorize(ctx, asCall(tc), exec)
		switch expectFull[tc.ID][0] {
		case DecisionAllow:
			if err != nil {
				t.Errorf("%s: unexpected error %v", tc.ID, err)
			}
		case DecisionDeny:
			if !errors.Is(err, ErrDenied) {
				t.Errorf("%s: want ErrDenied, got %v", tc.ID, err)
			}
		case DecisionEscalate:
			if !errors.Is(err, ErrEscalated) {
				t.Errorf("%s: want ErrEscalated, got %v", tc.ID, err)
			}
		}
	}
	if n := len(executed); n != 5 {
		t.Fatalf("expected 5 executed calls, got %d: %v", n, executed)
	}
	if executed["C06"] || executed["C05"] || executed["C11"] {
		t.Fatalf("a refused call executed")
	}
}

func TestChainVerifiesAndDetectsTampering(t *testing.T) {
	g, _ := New(loadPolicy(t), WithClock(fixedClock()))
	ctx := context.Background()
	exec := func(context.Context, Call) (string, error) { return "ok", nil }
	for _, tc := range loadCalls(t) {
		_, _ = g.Authorize(ctx, asCall(tc), exec)
	}
	recs := g.Records()
	if ok, at := VerifyChain(recs); !ok {
		t.Fatalf("fresh chain failed to verify at %d", at)
	}
	// Edit the redirected payment's record to look like an allow.
	recs[5].Decision = DecisionAllow
	recs[5].Outcome = "executed: ok"
	if ok, at := VerifyChain(recs); ok || at != 5 {
		t.Fatalf("tampered record not detected (ok=%v at=%d)", ok, at)
	}
	// Remove a record.
	recs = g.Records()
	recs = append(recs[:4], recs[5:]...)
	if ok, at := VerifyChain(recs); ok || at != 4 {
		t.Fatalf("removed record not detected (ok=%v at=%d)", ok, at)
	}
}

func TestEscalationResolvesWithApprover(t *testing.T) {
	pub := &memPublisher{}
	g, _ := New(loadPolicy(t), WithPublisher(pub), WithClock(fixedClock()))
	ctx := context.Background()
	ran := false
	exec := func(context.Context, Call) (string, error) { ran = true; return "vendor updated", nil }
	var c07 testCall
	for _, tc := range loadCalls(t) {
		if tc.ID == "C07" {
			c07 = tc
		}
	}
	held, err := g.Authorize(ctx, asCall(c07), exec)
	if !errors.Is(err, ErrEscalated) || ran {
		t.Fatalf("C07 should be held, err=%v ran=%v", err, ran)
	}
	if held.Approval != ApprovalPendingHuman {
		t.Fatalf("held record should be pending-human, got %q", held.Approval)
	}
	res, err := g.Resolve(ctx, held.Hash, "user:cfo@corp.example", true)
	if err != nil || !ran {
		t.Fatalf("approval should execute: err=%v ran=%v", err, ran)
	}
	if res.Approval != ApprovalGranted || res.Approver != "user:cfo@corp.example" || res.PrevHash != held.Hash {
		t.Fatalf("resolved record should carry the approver and chain to the hold: %+v", res)
	}
	if ok, _ := VerifyChain(g.Records()); !ok {
		t.Fatalf("chain with a resolved hold should verify")
	}
	want := []string{EventRequested, EventDecided, EventEscalated, EventApproved, EventExecuted}
	if len(pub.events) != len(want) {
		t.Fatalf("events: got %v, want %v", pub.events, want)
	}
	for i := range want {
		if pub.events[i] != want[i] {
			t.Fatalf("events: got %v, want %v", pub.events, want)
		}
	}
	if _, err := g.Resolve(ctx, held.Hash, "user:cfo@corp.example", true); err == nil {
		t.Fatalf("a hold must resolve only once")
	}
}

func TestUnknownToolIsDenied(t *testing.T) {
	g, _ := New(loadPolicy(t))
	d := g.Decide(Call{Tool: "wire_transfer", Args: map[string]any{"amount": 1.0}})
	if d.Outcome != DecisionDeny || d.Rule != ScopeRuleID {
		t.Fatalf("unknown tool should fail closed, got %+v", d)
	}
}

func TestPolicyValidation(t *testing.T) {
	p := loadPolicy(t)
	p.Rules = append(p.Rules, Rule{ID: "P2-ceiling", Kind: KindCeiling, Arg: "x"})
	if err := p.Validate(); err == nil {
		t.Fatal("duplicate rule id should fail validation")
	}
	p = loadPolicy(t)
	p.Rules = append(p.Rules, Rule{ID: "P9", Kind: "regex", Arg: "x"})
	if err := p.Validate(); err == nil {
		t.Fatal("unknown rule kind should fail validation")
	}
	p = loadPolicy(t)
	p.Rules = append(p.Rules, Rule{ID: "P9", Kind: KindLookup, Arg: "x", MatchArg: "y"})
	if err := p.Validate(); err == nil {
		t.Fatal("lookup rule without a table should fail validation")
	}
}

func TestCallScopesOverridePolicyGrant(t *testing.T) {
	g, err := New(loadPolicy(t))
	if err != nil {
		t.Fatal(err)
	}
	call := Call{ID: "X1", Tool: "list_invoices", Args: map[string]any{"status": "open"}}
	if d := g.Decide(call); d.Outcome != DecisionAllow {
		t.Fatalf("policy grant should allow list_invoices, got %s", d.Outcome)
	}
	call.Scopes = []string{"email:send"}
	if d := g.Decide(call); d.Outcome != DecisionDeny || d.Rule != ScopeRuleID {
		t.Fatalf("caller scopes without invoices:read should deny on the scope rule, got %s/%s", d.Outcome, d.Rule)
	}
	call.Scopes = []string{"*"}
	if d := g.Decide(call); d.Outcome != DecisionAllow {
		t.Fatalf("a caller grant of * should allow, got %s", d.Outcome)
	}
	call.Scopes = []string{}
	if d := g.Decide(call); d.Outcome != DecisionDeny {
		t.Fatalf("an empty caller grant should deny, got %s", d.Outcome)
	}
}
