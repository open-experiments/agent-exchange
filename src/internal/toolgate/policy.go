package toolgate

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
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
	// KindTool fires whenever the rule applies, whatever the arguments: the
	// action itself is what the rule governs ("every destructive tool needs a
	// person"). It takes no Arg and must be limited by Tool or Tag, so it can
	// never fire on every tool by accident.
	KindTool = "tool"
	// KindFloor denies when the numeric argument Arg is below Min.
	KindFloor = "floor"
	// KindDenylist denies when the argument Arg is one of Denied (compared
	// without regard to case, so a denied account cannot be dodged by case).
	KindDenylist = "denylist"
	// KindPattern denies when the argument Arg does not match Pattern in full.
	// Patterns are RE2, so a pattern cannot be made to run in exponential time.
	KindPattern = "pattern"
	// KindDomain denies when the host of the argument Arg (an email address, a
	// URL or a bare host name) is not one of Allowed or a subdomain of one.
	KindDomain = "domain"
	// KindMaxItems denies when the argument Arg holds more than Max items (a
	// lone value counts as one): bulk sends, bulk deletes.
	KindMaxItems = "max_items"
	// KindCompare denies unless Arg Op MatchArg holds: lt, le, gt, ge on
	// numbers, eq and ne on numbers or plain values ("a refund may not exceed
	// the original charge").
	KindCompare = "compare"
	// KindPrefix denies when the string argument Arg does not start with one of
	// Allowed. Used for storage destinations.
	KindPrefix = "prefix"
)

// Effects a rule can have when it fires.
const (
	EffectDeny     = "deny"
	EffectEscalate = "escalate"
)

// Rule is one argument-value rule. Tool restricts the rule to one tool and Tag
// to the tools the policy's ToolTags label with that tag; a rule sets at most
// one of them. An empty Tool and Tag applies it to every tool that carries the
// argument.
type Rule struct {
	ID       string   `json:"id"`
	Tool     string   `json:"tool,omitempty"`
	Tag      string   `json:"tag,omitempty"`
	Kind     string   `json:"kind"`
	Arg      string   `json:"arg"`
	Max      float64  `json:"max,omitempty"`
	Allowed  []string `json:"allowed,omitempty"`
	Field    string   `json:"field,omitempty"`
	MatchArg string   `json:"match_arg,omitempty"`
	Effect   string   `json:"effect,omitempty"`
	Message  string   `json:"message,omitempty"`
	// Min is the floor's lower bound; a pointer so that a floor of 0 is
	// distinguishable from a floor nobody set.
	Min     *float64 `json:"min,omitempty"`
	Denied  []string `json:"denied,omitempty"`
	Pattern string   `json:"pattern,omitempty"`
	// Op is the relation a compare rule requires between Arg and MatchArg.
	Op string `json:"op,omitempty"`
	// Each applies a value rule to every element of a list argument. Without
	// it a list cannot be checked as one value and the rule fails closed.
	Each bool `json:"each,omitempty"`
}

// Arg (and MatchArg) name an argument, or a path into one: "recipient.email"
// reads a field of an object argument, and "items[*].amount" the field of
// every element of a list, each of which the rule then checks. An argument
// whose own name contains a dot is still found by its exact name first.

// eachKinds are the kinds that check one value at a time and so can apply to
// each element of a list.
var eachKinds = map[string]bool{
	KindCeiling: true, KindFloor: true, KindAllowlist: true, KindDenylist: true, KindSuffix: true,
	KindPrefix: true, KindPattern: true, KindDomain: true, KindSensitiveField: true,
}

var compareOps = map[string]bool{"lt": true, "le": true, "gt": true, "ge": true, "eq": true, "ne": true}

// Policy is a provider's authorization policy: the scopes this session holds,
// the scope each tool requires, the argument-value rules, and any lookup
// tables the lookup rules read.
type Policy struct {
	Provider      string                       `json:"provider"`
	GrantedScopes []string                     `json:"granted_scopes"`
	ToolScopes    map[string]string            `json:"tool_scopes"`
	Rules         []Rule                       `json:"rules"`
	Lookups       map[string]map[string]string `json:"lookups,omitempty"`
	// ToolTags labels tools with classes ("destructive", "payment", ...) so
	// one rule can govern many tools. Every tagged tool must be in ToolScopes.
	ToolTags map[string][]string `json:"tool_tags,omitempty"`
}

// hasTag reports whether the policy labels tool with tag.
func (p Policy) hasTag(tool, tag string) bool {
	for _, t := range p.ToolTags[tool] {
		if t == tag {
			return true
		}
	}
	return false
}

// applies reports whether rule r governs calls to tool.
func (p Policy) applies(r Rule, tool string) bool {
	if r.Tool != "" && r.Tool != tool {
		return false
	}
	return r.Tag == "" || p.hasTag(tool, r.Tag)
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
	tags := map[string]bool{}
	for tool, ts := range p.ToolTags {
		// A tag on a tool the policy does not know is a typo that would
		// silently leave the real tool ungoverned.
		if _, ok := p.ToolScopes[tool]; !ok {
			return fmt.Errorf("toolgate: tool_tags names %q, which is not in tool_scopes", tool)
		}
		for _, t := range ts {
			tags[t] = true
		}
	}
	seen := map[string]bool{}
	for _, r := range p.Rules {
		if r.ID == "" {
			return fmt.Errorf("toolgate: rule needs an id")
		}
		if seen[r.ID] {
			return fmt.Errorf("toolgate: duplicate rule id %q", r.ID)
		}
		seen[r.ID] = true
		if r.Tool != "" && r.Tag != "" {
			return fmt.Errorf("toolgate: rule %q sets both tool and tag; use one", r.ID)
		}
		// Same reasoning as for tool_tags: a rule on a tag no tool carries
		// governs nothing, and nobody would notice.
		if r.Tag != "" && !tags[r.Tag] {
			return fmt.Errorf("toolgate: rule %q uses tag %q, which no tool carries", r.ID, r.Tag)
		}
		if r.Kind == KindTool {
			if r.Arg != "" {
				return fmt.Errorf("toolgate: rule %q of kind tool takes no arg", r.ID)
			}
			if r.Tool == "" && r.Tag == "" {
				return fmt.Errorf("toolgate: rule %q of kind tool needs a tool or a tag", r.ID)
			}
		} else if r.Arg == "" {
			return fmt.Errorf("toolgate: rule %q needs an arg", r.ID)
		} else if err := validPath(r.Arg); err != nil {
			return fmt.Errorf("toolgate: rule %q: arg %v", r.ID, err)
		}
		if r.Each && !eachKinds[r.Kind] {
			return fmt.Errorf("toolgate: rule %q of kind %s cannot apply to each element", r.ID, r.Kind)
		}
		switch r.Kind {
		case KindTool:
		case KindFloor:
			if r.Min == nil {
				return fmt.Errorf("toolgate: rule %q needs min", r.ID)
			}
		case KindDenylist:
			if len(r.Denied) == 0 {
				return fmt.Errorf("toolgate: rule %q needs a denied list", r.ID)
			}
		case KindPattern:
			if r.Pattern == "" {
				return fmt.Errorf("toolgate: rule %q needs a pattern", r.ID)
			}
			if _, err := compiledPattern(r.Pattern); err != nil {
				return fmt.Errorf("toolgate: rule %q has a bad pattern: %v", r.ID, err)
			}
		case KindDomain:
			if len(r.Allowed) == 0 {
				return fmt.Errorf("toolgate: rule %q needs an allowed list of domains", r.ID)
			}
			for _, d := range r.Allowed {
				// A domain entry is a bare lower-case ASCII host name: "@corp.example",
				// ".corp.example" or "https://corp.example" would never match.
				if d == "" || d != strings.ToLower(d) || strings.ContainsAny(d, "@/:* ") ||
					strings.HasPrefix(d, ".") || strings.HasSuffix(d, ".") || !isASCII(d) {
					return fmt.Errorf("toolgate: rule %q: %q is not a bare lower-case domain", r.ID, d)
				}
			}
		case KindMaxItems:
			if r.Max < 1 {
				return fmt.Errorf("toolgate: rule %q needs max of at least 1", r.ID)
			}
		case KindCompare:
			if !compareOps[r.Op] {
				return fmt.Errorf("toolgate: rule %q needs op lt, le, gt, ge, eq or ne", r.ID)
			}
			if r.MatchArg == "" {
				return fmt.Errorf("toolgate: rule %q needs match_arg", r.ID)
			}
			if strings.Contains(r.Arg+r.MatchArg, "[*]") {
				return fmt.Errorf("toolgate: rule %q compares single values; [*] is not allowed", r.ID)
			}
			if err := validPath(r.MatchArg); err != nil {
				return fmt.Errorf("toolgate: rule %q: match_arg %v", r.ID, err)
			}
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
	if r.Kind == KindTool {
		return true, effect, ""
	}
	if r.Kind == KindCompare {
		return r.evaluateCompare(call, effect)
	}
	vals, multi, why := resolveArg(call.Args, r.Arg)
	if why != "" {
		// The rule constrains an argument the call did not supply (or a path
		// that does not exist in it), so the constraint cannot be checked.
		// Fail closed: an absent argument must not be a way around the rule
		// (a tool whose upstream accepts a synonym, an alternate casing or its
		// own default would otherwise sail straight past). evaluateLookup
		// does the same.
		return true, effect, unverifiable(why)
	}
	if r.Kind == KindMaxItems {
		n, ok := countItems(vals, multi)
		if !ok {
			return true, effect, unverifiable(r.Arg + " is null, so its items cannot be counted")
		}
		return float64(n) > r.Max, effect, ""
	}
	if r.Each && !multi {
		// A rule marked each checks every element of a list argument, and a
		// lone value as a list of one. Without each, a list still reaches
		// checkValue whole and fails closed there, as before.
		if list, ok := vals[0].([]any); ok {
			vals = list
		}
	}
	for _, v := range vals {
		if fired, reason := r.checkValue(v); fired {
			return true, effect, reason
		}
	}
	return false, "", ""
}

// checkValue applies the rule's value check to one value. It is the single
// place each kind compares; evaluate decides which values reach it (the
// argument, an element of it, or a value found along a nested path).
func (r Rule) checkValue(v any) (fired bool, reason string) {
	switch r.Kind {
	case KindCeiling:
		f, ok := toFloat(v)
		if !ok {
			// Not a number, so it cannot be compared against the ceiling.
			// Fail closed: a quoted "999999" must not read as under the limit.
			return true, unverifiable(r.Arg + " is not a number, so it cannot be compared against the limit")
		}
		return f > r.Max, ""
	case KindAllowlist:
		s, ok := atomicValue(v)
		if !ok {
			return true, unverifiable(r.Arg + " is not a single plain value")
		}
		for _, a := range r.Allowed {
			if s == a {
				return false, ""
			}
		}
		return true, ""
	case KindSuffix:
		s, ok := atomicValue(v)
		if !ok {
			return true, unverifiable(r.Arg + " holds more than one value, or padding, so a suffix match is meaningless")
		}
		for _, a := range r.Allowed {
			if strings.HasSuffix(s, a) {
				return false, ""
			}
		}
		return true, ""
	case KindPrefix:
		s, ok := atomicValue(v)
		if !ok {
			return true, unverifiable(r.Arg + " holds more than one value, or padding, so a prefix match is meaningless")
		}
		// An allowed prefix must not be escapable by walking back up out of
		// it: "corp-internal://../../../s3://attacker/" starts with the
		// allowed prefix and lands somewhere else entirely.
		if strings.Contains(s, "..") {
			return true, unverifiable(r.Arg + " walks back out of the allowed prefix")
		}
		for _, a := range r.Allowed {
			if strings.HasPrefix(s, a) {
				return false, ""
			}
		}
		return true, ""
	case KindFloor:
		f, ok := toFloat(v)
		if !ok {
			return true, unverifiable(r.Arg + " is not a number, so it cannot be compared against the floor")
		}
		return f < *r.Min, ""
	case KindDenylist:
		s, ok := atomicValue(v)
		if !ok {
			return true, unverifiable(r.Arg + " is not a single plain value")
		}
		for _, d := range r.Denied {
			if strings.EqualFold(s, d) {
				return true, ""
			}
		}
		return false, ""
	case KindPattern:
		s, ok := atomicValue(v)
		if !ok {
			return true, unverifiable(r.Arg + " is not a single plain value")
		}
		re, err := compiledPattern(r.Pattern)
		if err != nil {
			return true, unverifiable("the rule's pattern does not compile")
		}
		return !re.MatchString(s), ""
	case KindDomain:
		s, ok := atomicValue(v)
		if !ok {
			return true, unverifiable(r.Arg + " is not a single plain value")
		}
		host, why := hostOf(s)
		if why != "" {
			return true, unverifiable(r.Arg + " " + why)
		}
		for _, d := range r.Allowed {
			if host == d || strings.HasSuffix(host, "."+d) {
				return false, ""
			}
		}
		return true, ""
	case KindSensitiveField:
		s, ok := atomicValue(v)
		if !ok {
			// The argument naming the field is not a lone scalar, so which
			// field is being changed cannot be established. Fail closed.
			return true, unverifiable(r.Arg + " is not a single plain value")
		}
		return normalizeField(s) == normalizeField(r.Field), ""
	}
	return false, ""
}

// evaluateLookup applies a lookup rule using the policy's table.
func (p Policy) evaluateLookup(r Rule, call Call) (fired bool, effect, reason string) {
	if r.Tool != "" && r.Tool != call.Tool {
		return false, "", ""
	}
	v, ok := singleArg(call.Args, r.Arg)
	if !ok {
		// The value this rule checks is absent, so it cannot be checked
		// against the table. Fail closed for the same reason evaluate does:
		// otherwise misspelling recipient_account, or omitting it and letting
		// the upstream tool supply its own default, skips the rule that says
		// where the money is allowed to go.
		return true, effectOrDeny(r), unverifiable(r.Arg + " is required by this rule and was not supplied")
	}
	key, ok := singleArg(call.Args, r.MatchArg)
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

// evaluateCompare applies a compare rule: the rule fires unless Arg Op
// MatchArg holds.
func (r Rule) evaluateCompare(call Call, effect string) (fired bool, eff, reason string) {
	a, ok := singleArg(call.Args, r.Arg)
	if !ok {
		return true, effect, unverifiable(r.Arg + " is required by this rule and was not supplied")
	}
	b, ok := singleArg(call.Args, r.MatchArg)
	if !ok {
		return true, effect, unverifiable(r.MatchArg + " was not supplied, so " + r.Arg + " cannot be compared with it")
	}
	fa, okA := toFloat(a)
	fb, okB := toFloat(b)
	if okA && okB {
		var holds bool
		switch r.Op {
		case "lt":
			holds = fa < fb
		case "le":
			holds = fa <= fb
		case "gt":
			holds = fa > fb
		case "ge":
			holds = fa >= fb
		case "eq":
			holds = fa == fb
		case "ne":
			holds = fa != fb
		}
		return !holds, effect, ""
	}
	if r.Op != "eq" && r.Op != "ne" {
		// Ordering needs numbers; "999" against 1000 must not be decided as text.
		return true, effect, unverifiable(r.Arg + " and " + r.MatchArg + " are not both numbers, so they cannot be ordered")
	}
	sa, okA := atomicValue(a)
	sb, okB := atomicValue(b)
	if !okA || !okB {
		return true, effect, unverifiable(r.Arg + " or " + r.MatchArg + " is not a single plain value")
	}
	return (sa == sb) != (r.Op == "eq"), effect, ""
}

// resolveArg finds the values a rule checks. It returns one value for a plain
// argument or path, and every value found for a path through "[*]" (multi),
// or a reason when the argument or path is absent.
func resolveArg(args map[string]any, path string) (vals []any, multi bool, why string) {
	if v, ok := args[path]; ok {
		return []any{v}, false, ""
	}
	if !strings.ContainsAny(path, ".[") {
		return nil, false, path + " is required by this rule and was not supplied"
	}
	cur := []any{map[string]any(args)}
	for _, seg := range strings.Split(path, ".") {
		name, wild := strings.CutSuffix(seg, "[*]")
		var next []any
		for _, c := range cur {
			obj, ok := c.(map[string]any)
			if !ok {
				return nil, false, path + " does not lead to a value in this call"
			}
			v, ok := obj[name]
			if !ok {
				return nil, false, path + " is required by this rule and was not supplied"
			}
			if !wild {
				next = append(next, v)
				continue
			}
			list, ok := v.([]any)
			if !ok {
				return nil, false, path + " expects a list at " + name
			}
			next = append(next, list...)
			multi = true
		}
		cur = next
	}
	return cur, multi, ""
}

// singleArg resolves a path that must lead to exactly one value.
func singleArg(args map[string]any, path string) (any, bool) {
	vals, multi, why := resolveArg(args, path)
	if why != "" || multi || len(vals) != 1 {
		return nil, false
	}
	return vals[0], true
}

// countItems counts what a max_items rule limits: the values a [*] path
// found, the elements of a list, or one for a lone value. Null is uncountable.
func countItems(vals []any, multi bool) (int, bool) {
	if multi {
		return len(vals), true
	}
	switch v := vals[0].(type) {
	case nil:
		return 0, false
	case []any:
		return len(v), true
	}
	return 1, true
}

// validPath checks an argument path: non-empty segments, [*] only at a
// segment's end.
func validPath(path string) error {
	for _, seg := range strings.Split(path, ".") {
		name, _ := strings.CutSuffix(seg, "[*]")
		if name == "" || strings.ContainsAny(name, "[]*") {
			return fmt.Errorf("%q is not a valid argument path", path)
		}
	}
	return nil
}

// hostOf extracts the host a domain rule checks from an email address, a URL
// or a bare host name, lower-cased. It refuses what it cannot read without
// guessing: several "@", a URL without a host, and non-ASCII (homoglyph)
// hosts.
func hostOf(s string) (host, why string) {
	switch {
	case strings.Contains(s, "://"):
		u, err := url.Parse(s)
		if err != nil || u.Hostname() == "" {
			return "", "is not a URL with a host"
		}
		// url.Parse already takes the host after any userinfo, so
		// "https://corp.example@evil.example" reads as evil.example.
		host = u.Hostname()
	case strings.Contains(s, "@"):
		if strings.Count(s, "@") != 1 {
			return "", "has more than one @, so its domain is ambiguous"
		}
		host = s[strings.Index(s, "@")+1:]
	default:
		host = s
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "" || !isASCII(host) {
		return "", "has no plain ASCII host"
	}
	return host, ""
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 0x7e || s[i] < 0x21 {
			return false
		}
	}
	return true
}

var patterns sync.Map // pattern source -> *regexp.Regexp

// compiledPattern compiles a pattern once, anchored so it must match the
// whole value.
func compiledPattern(p string) (*regexp.Regexp, error) {
	if re, ok := patterns.Load(p); ok {
		return re.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile(`^(?:` + p + `)$`)
	if err != nil {
		return nil, err
	}
	patterns.Store(p, re)
	return re, nil
}
