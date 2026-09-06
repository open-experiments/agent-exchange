package toolgate

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Rule kinds. Rules are evaluated in the order they appear in the policy,
// after the scope check, and the first rule that fires decides the call.
const (
	// KindCeiling denies when the numeric argument Arg exceeds Max.
	KindCeiling = "ceiling"
	// KindAllowlist denies when the argument Arg is not one of Allowed.
	KindAllowlist = "allowlist"
	// KindLookup denies when the argument Arg differs from the value the
	// policy's Lookups table maps the argument MatchArg to. This is the
	// "recipient must be the account on file for this invoice" rule.
	KindLookup = "lookup"
	// KindSensitiveField escalates (or denies, per Effect) when the argument
	// Arg equals Field. This is the "changing a bank account needs a person" rule.
	KindSensitiveField = "sensitive_field"
	// KindSuffix denies when the string argument Arg does not end with one of
	// Allowed. Used for email domains.
	KindSuffix = "suffix"
	// KindPrefix denies when the string argument Arg does not start with one of
	// Allowed. Used for storage destinations.
	KindPrefix = "prefix"
)

// Effects a rule can have when it fires.
const (
	EffectDeny     = "deny"
	EffectEscalate = "escalate"
)

// Rule is one argument-value rule. Tool restricts the rule to one tool; an
// empty Tool applies it to every tool that carries the argument.
type Rule struct {
	ID       string   `json:"id"`
	Tool     string   `json:"tool,omitempty"`
	Kind     string   `json:"kind"`
	Arg      string   `json:"arg"`
	Max      float64  `json:"max,omitempty"`
	Allowed  []string `json:"allowed,omitempty"`
	Field    string   `json:"field,omitempty"`
	MatchArg string   `json:"match_arg,omitempty"`
	Effect   string   `json:"effect,omitempty"`
	Message  string   `json:"message,omitempty"`
}

// Policy is a provider's authorization policy: the scopes this session holds,
// the scope each tool requires, the argument-value rules, and any lookup
// tables the lookup rules read.
type Policy struct {
	Provider      string                       `json:"provider"`
	GrantedScopes []string                     `json:"granted_scopes"`
	ToolScopes    map[string]string            `json:"tool_scopes"`
	Rules         []Rule                       `json:"rules"`
	Lookups       map[string]map[string]string `json:"lookups,omitempty"`
}

// ScopeRuleID is the rule id recorded when the scope check decides a call.
const ScopeRuleID = "P1-scope"

// LoadPolicy reads a policy from a JSON file.
func LoadPolicy(path string) (Policy, error) {
	var p Policy
	b, err := os.ReadFile(path)
	if err != nil {
		return p, fmt.Errorf("toolgate: read policy: %w", err)
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return p, fmt.Errorf("toolgate: parse policy: %w", err)
	}
	return p, p.Validate()
}

// Validate checks that every rule is well formed.
func (p Policy) Validate() error {
	if p.Provider == "" {
		return fmt.Errorf("toolgate: policy has no provider")
	}
	if len(p.ToolScopes) == 0 {
		return fmt.Errorf("toolgate: policy maps no tools to scopes")
	}
	seen := map[string]bool{}
	for _, r := range p.Rules {
		if r.ID == "" || r.Arg == "" {
			return fmt.Errorf("toolgate: rule %q needs an id and an arg", r.ID)
		}
		if seen[r.ID] {
			return fmt.Errorf("toolgate: duplicate rule id %q", r.ID)
		}
		seen[r.ID] = true
		switch r.Kind {
		case KindCeiling:
		case KindAllowlist, KindSuffix, KindPrefix:
			if len(r.Allowed) == 0 {
				return fmt.Errorf("toolgate: rule %q needs an allowed list", r.ID)
			}
		case KindLookup:
			if r.MatchArg == "" {
				return fmt.Errorf("toolgate: rule %q needs match_arg", r.ID)
			}
			if _, ok := p.Lookups[r.ID]; !ok {
				return fmt.Errorf("toolgate: rule %q has no lookup table", r.ID)
			}
		case KindSensitiveField:
			if r.Field == "" {
				return fmt.Errorf("toolgate: rule %q needs a field", r.ID)
			}
		default:
			return fmt.Errorf("toolgate: rule %q has unknown kind %q", r.ID, r.Kind)
		}
		switch r.Effect {
		case "", EffectDeny, EffectEscalate:
		default:
			return fmt.Errorf("toolgate: rule %q has unknown effect %q", r.ID, r.Effect)
		}
	}
	return nil
}

// hasScope reports whether a grant covers the scope. A grant of "*" covers
// everything, matching the gateway's default key scope.
func hasScope(granted []string, scope string) bool {
	for _, s := range granted {
		if s == "*" || s == scope {
			return true
		}
	}
	return false
}

// evaluate applies one rule to a call. It returns whether the rule fired and
// the effect to apply.
func (r Rule) evaluate(call Call) (fired bool, effect string) {
	if r.Tool != "" && r.Tool != call.Tool {
		return false, ""
	}
	v, ok := call.Args[r.Arg]
	if !ok {
		return false, ""
	}
	effect = r.Effect
	if effect == "" {
		effect = EffectDeny
	}
	switch r.Kind {
	case KindCeiling:
		f, ok := toFloat(v)
		return ok && f > r.Max, effect
	case KindAllowlist:
		s := fmt.Sprint(v)
		for _, a := range r.Allowed {
			if s == a {
				return false, ""
			}
		}
		return true, effect
	case KindSuffix:
		s := fmt.Sprint(v)
		for _, a := range r.Allowed {
			if strings.HasSuffix(s, a) {
				return false, ""
			}
		}
		return true, effect
	case KindPrefix:
		s := fmt.Sprint(v)
		for _, a := range r.Allowed {
			if strings.HasPrefix(s, a) {
				return false, ""
			}
		}
		return true, effect
	case KindSensitiveField:
		return fmt.Sprint(v) == r.Field, effect
	}
	return false, ""
}

// evaluateLookup applies a lookup rule using the policy's table.
func (p Policy) evaluateLookup(r Rule, call Call) (fired bool, effect string) {
	if r.Tool != "" && r.Tool != call.Tool {
		return false, ""
	}
	v, ok := call.Args[r.Arg]
	if !ok {
		return false, ""
	}
	key, ok := call.Args[r.MatchArg]
	if !ok {
		return true, effectOrDeny(r) // cannot verify: fail closed
	}
	expected, ok := p.Lookups[r.ID][fmt.Sprint(key)]
	if !ok {
		return true, effectOrDeny(r) // nothing on file: fail closed
	}
	return fmt.Sprint(v) != expected, effectOrDeny(r)
}

func effectOrDeny(r Rule) string {
	if r.Effect == "" {
		return EffectDeny
	}
	return r.Effect
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}
