package toolgate

import (
	"strings"
	"testing"
)

func taggedPolicy() Policy {
	return Policy{
		Provider:      "p",
		GrantedScopes: []string{"files", "pay"},
		ToolScopes:    map[string]string{"read_file": "files", "delete_file": "files", "wipe_disk": "files", "refund": "pay", "payout": "pay"},
		ToolTags: map[string][]string{
			"delete_file": {"destructive"}, "wipe_disk": {"destructive"},
			"refund": {"money"}, "payout": {"money"},
		},
		Rules: []Rule{
			{ID: "T1-destructive", Tag: "destructive", Kind: KindTool, Effect: EffectEscalate, Message: "destructive actions need a person"},
			{ID: "T2-money-ceiling", Tag: "money", Kind: KindCeiling, Arg: "amount", Max: 100, Message: "over the per-call limit"},
		},
	}
}

// One rule governs every tool carrying its tag, and no other tool.
func TestTagRulesGovernTaggedToolsOnly(t *testing.T) {
	g, err := New(taggedPolicy())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		tool string
		args map[string]any
		want string
		rule string
	}{
		{"read_file", map[string]any{"path": "/tmp/x"}, DecisionAllow, ScopeRuleID},
		{"delete_file", map[string]any{"path": "/tmp/x"}, DecisionEscalate, "T1-destructive"},
		{"wipe_disk", nil, DecisionEscalate, "T1-destructive"},
		{"refund", map[string]any{"amount": 50}, DecisionAllow, ScopeRuleID},
		{"payout", map[string]any{"amount": 500}, DecisionDeny, "T2-money-ceiling"},
		// A tagged tool must still carry the argument its tag's rule checks.
		{"refund", map[string]any{}, DecisionDeny, "T2-money-ceiling"},
	}
	for _, c := range cases {
		d := g.Decide(Call{Tool: c.tool, Args: c.args})
		if d.Outcome != c.want || d.Rule != c.rule {
			t.Errorf("%s %v: got %s/%s, want %s/%s", c.tool, c.args, d.Outcome, d.Rule, c.want, c.rule)
		}
	}
}

// Tags and tool rules fail validation on the mistakes that would otherwise
// leave a tool silently ungoverned or a rule firing everywhere.
func TestTagPolicyValidation(t *testing.T) {
	bad := []struct {
		name  string
		edit  func(*Policy)
		error string
	}{
		{"tag on unknown tool", func(p *Policy) { p.ToolTags["delet_file"] = []string{"destructive"} }, "not in tool_scopes"},
		{"rule on unused tag", func(p *Policy) { p.Rules[0].Tag = "destrutive" }, "no tool carries"},
		{"tool and tag", func(p *Policy) { p.Rules[0].Tool = "delete_file" }, "both tool and tag"},
		{"tool rule with arg", func(p *Policy) { p.Rules[0].Arg = "path" }, "takes no arg"},
		{"tool rule unscoped", func(p *Policy) { p.Rules[0].Tag = "" }, "needs a tool or a tag"},
		{"arg rule without arg", func(p *Policy) { p.Rules[1].Arg = "" }, "needs an arg"},
	}
	for _, b := range bad {
		p := taggedPolicy()
		b.edit(&p)
		err := p.Validate()
		if err == nil || !strings.Contains(err.Error(), b.error) {
			t.Errorf("%s: got %v, want an error containing %q", b.name, err, b.error)
		}
	}
	if err := taggedPolicy().Validate(); err != nil {
		t.Errorf("valid tagged policy rejected: %v", err)
	}
}

// A tool rule scoped to one tool works without tags at all.
func TestToolRuleByName(t *testing.T) {
	p := taggedPolicy()
	p.ToolTags = nil
	p.Rules = []Rule{{ID: "T3", Tool: "wipe_disk", Kind: KindTool}}
	g, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	if d := g.Decide(Call{Tool: "wipe_disk"}); d.Outcome != DecisionDeny || d.Rule != "T3" {
		t.Errorf("wipe_disk: got %s/%s, want deny/T3", d.Outcome, d.Rule)
	}
	if d := g.Decide(Call{Tool: "delete_file"}); d.Outcome != DecisionAllow {
		t.Errorf("delete_file: got %s, want allow", d.Outcome)
	}
}
