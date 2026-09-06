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

// narrow intersects the policy's grant with the caller's scopes. The caller's
// scopes arrive in a request header and are therefore untrusted: they may only
// reduce what the policy granted, never add to it. A caller sending "*" gets
// the policy's grant unchanged rather than everything, so the header cannot be
// used to switch the tool_scopes layer off. Nil caller scopes mean "no
// narrowing" and the policy's grant stands.
// The two sets are not always the same vocabulary: the gateway forwards its
// own route scopes (tools:invoke, read) alongside the per-tool scopes a policy
// names (payments:send). A working least-privilege key therefore carries both,
// and the unrecognised ones simply fail to match anything here.
func narrow(granted, callerScopes []string) []string {
	if callerScopes == nil {
		return granted
	}
	// A policy that grants "*" has named no scopes to intersect with, so the
	// caller's own scopes are the narrower set. Without this the intersection
	// is empty and such a policy denies every call the moment a caller
	// narrows honestly.
	if hasScope(granted, "*") {
		return callerScopes
	}
	out := make([]string, 0, len(granted))
	for _, g := range granted {
		if hasScope(callerScopes, g) {
			out = append(out, g)
		}
	}
	return out
}

// atomicValue reports whether v is a single scalar value safe to match a
// prefix or suffix against, and returns it as a string.
//
// A suffix or prefix check only means something when the value is one thing.
// "attacker@evil.example,someone@corp.example" ends in "@corp.example" and
// passes an allowed-domain check, and any upstream that splits on commas then
// mails the attacker. The same holds for newlines (header injection) and for
// composite JSON values, whose fmt.Sprint form ("[a b]") matches by accident
// rather than by rule. Anything that is not a lone scalar fails closed.
func atomicValue(v any) (string, bool) {
	switch v.(type) {
	case string, json.Number, float64, float32, int, int64, bool:
	default:
		return "", false
	}
	s := fmt.Sprint(v)
	if s != strings.TrimSpace(s) {
		return "", false
	}
	if strings.ContainsAny(s, ",;<>\"'\\ \t\r\n\v\f\x00") {
		return "", false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return "", false
		}
	}
	return s, true
}

// normalizeField folds the spelling differences that would otherwise let a
// sensitive-field rule be dodged: case, surrounding space, and hyphen for
// underscore. It cannot fold homoglyphs, so a policy's Field should stay ASCII
// and callers should treat a non-ASCII value as suspect.
func normalizeField(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "-", "_")
}

// evaluate applies one rule to a call. It returns whether the rule fired and
// the effect to apply.
func (r Rule) evaluate(call Call) (fired bool, effect, reason string) {
	if r.Tool != "" && r.Tool != call.Tool {
		return false, "", ""
	}
	effect = effectOrDeny(r)
	v, ok := call.Args[r.Arg]
	if !ok {
		// The rule constrains an argument the call did not supply, so the
		// constraint cannot be checked. Fail closed: an absent argument must
		// not be a way around the rule (a tool whose upstream accepts a
		// synonym, an alternate casing or its own default would otherwise
		// sail straight past). evaluateLookup does the same.
		return true, effect, unverifiable(r.Arg + " is required by this rule and was not supplied")
	}
	switch r.Kind {
	case KindCeiling:
		f, ok := toFloat(v)
		if !ok {
			// Not a number, so it cannot be compared against the ceiling.
			// Fail closed: a quoted "999999" must not read as under the limit.
			return true, effect, unverifiable(r.Arg + " is not a number, so it cannot be compared against the limit")
		}
		return f > r.Max, effect, ""
	case KindAllowlist:
		s, ok := atomicValue(v)
		if !ok {
			return true, effect, unverifiable(r.Arg + " is not a single plain value")
		}
		for _, a := range r.Allowed {
			if s == a {
				return false, "", ""
			}
		}
		return true, effect, ""
	case KindSuffix:
		s, ok := atomicValue(v)
		if !ok {
			return true, effect, unverifiable(r.Arg + " holds more than one value, or padding, so a suffix match is meaningless")
		}
		for _, a := range r.Allowed {
			if strings.HasSuffix(s, a) {
				return false, "", ""
			}
		}
		return true, effect, ""
	case KindPrefix:
		s, ok := atomicValue(v)
		if !ok {
			return true, effect, unverifiable(r.Arg + " holds more than one value, or padding, so a prefix match is meaningless")
		}
		// An allowed prefix must not be escapable by walking back up out of
		// it: "corp-internal://../../../s3://attacker/" starts with the
		// allowed prefix and lands somewhere else entirely.
		if strings.Contains(s, "..") {
			return true, effect, unverifiable(r.Arg + " walks back out of the allowed prefix")
		}
		for _, a := range r.Allowed {
			if strings.HasPrefix(s, a) {
				return false, "", ""
			}
		}
		return true, effect, ""
	case KindSensitiveField:
		s, ok := atomicValue(v)
		if !ok {
			// The argument naming the field is not a lone scalar, so which
			// field is being changed cannot be established. Fail closed.
			return true, effect, unverifiable(r.Arg + " is not a single plain value")
		}
		return normalizeField(s) == normalizeField(r.Field), effect, ""
	}
	return false, "", ""
}

// evaluateLookup applies a lookup rule using the policy's table.
func (p Policy) evaluateLookup(r Rule, call Call) (fired bool, effect, reason string) {
	if r.Tool != "" && r.Tool != call.Tool {
		return false, "", ""
	}
	v, ok := call.Args[r.Arg]
	if !ok {
		// The value this rule checks is absent, so it cannot be checked
		// against the table. Fail closed for the same reason evaluate does:
		// otherwise misspelling recipient_account, or omitting it and letting
		// the upstream tool supply its own default, skips the rule that says
		// where the money is allowed to go.
		return true, effectOrDeny(r), unverifiable(r.Arg + " is required by this rule and was not supplied")
	}
	key, ok := call.Args[r.MatchArg]
	if !ok {
		return true, effectOrDeny(r), unverifiable(r.MatchArg + " was not supplied, so " + r.Arg + " cannot be checked against the table")
	}
	expected, ok := p.Lookups[r.ID][fmt.Sprint(key)]
	if !ok {
		return true, effectOrDeny(r), unverifiable("no entry on file for " + fmt.Sprint(key))
	}
	return fmt.Sprint(v) != expected, effectOrDeny(r), ""
}

// unverifiable marks a rule that fired because the gate could not check it,
// rather than because the call broke it. The distinction matters to whoever
// reads the decision: "bank account changes need a person" on a call that
// never mentioned a bank account sends them looking for the wrong thing.
func unverifiable(detail string) string {
	return "cannot verify: " + detail
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
