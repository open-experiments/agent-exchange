package toolgate

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Event types published by the gate. They belong to the TOOLCALL stream.
const (
	EventRequested = "toolcall.requested"
	EventDecided   = "toolcall.decided"
	EventExecuted  = "toolcall.executed"
	EventRefused   = "toolcall.refused"
	EventEscalated = "toolcall.escalated"
	EventApproved  = "toolcall.approved"
)

// Mode selects how much of the policy the gate applies. ModeScope is the
// gateway's model moved to the call (tool name against granted scopes, values
// never examined); ModeFull adds the argument-value rules. The harness study
// compared the two as configurations B and C.
type Mode int

const (
	ModeScope Mode = iota + 1
	ModeFull
)

// Call is one tool invocation an agent wants to make, with the identities
// that will be written into its artifact.
type Call struct {
	ID         string
	Tool       string
	Args       map[string]any
	Principal  string
	AgentID    string
	SessionID  string
	TenantID   string
	ContractID string
	CertID     string
	TraceID    string
	// Scopes, when set, are the caller's validated scopes for this call (for
	// example the API-key scopes the gateway forwards) and replace the
	// policy's GrantedScopes for the scope check. Nil means use the policy's.
	Scopes []string
}

// Decision is the gate's answer for one call before execution.
type Decision struct {
	Outcome  string
	Rule     string
	Scope    string
	Approval string
	Message  string
}

// Publisher is what the gate needs from an event publisher. The AEX events
// package's Publisher satisfies it, so a gate can publish straight into
// JetStream; tests use an in-memory one.
type Publisher interface {
	Publish(ctx context.Context, eventType string, data map[string]any) error
}

// Executor runs the tool once the gate allows it and returns a short outcome
// string for the record.
type Executor func(ctx context.Context, call Call) (outcome string, err error)

// ErrDenied is returned by Authorize when the gate refuses the call.
var ErrDenied = errors.New("toolgate: call denied")

// ErrEscalated is returned by Authorize when the call is held for a person.
var ErrEscalated = errors.New("toolgate: call held for approval")

// Gate authorizes tool calls against a Policy and records an artifact per call.
type Gate struct {
	policy    Policy
	mode      Mode
	publisher Publisher
	now       func() time.Time
	mu        sync.Mutex
	prevHash  string
	records   []Artifact
	held      map[string]held
}

type held struct {
	call     Call
	artifact Artifact
	exec     Executor
}

// Option configures a Gate.
type Option func(*Gate)

// WithMode sets the gate mode; the default is ModeFull.
func WithMode(m Mode) Option { return func(g *Gate) { g.mode = m } }

// WithPublisher attaches an event publisher. Without one the gate keeps
// records in memory only.
func WithPublisher(p Publisher) Option { return func(g *Gate) { g.publisher = p } }

// WithClock replaces the clock, for tests.
func WithClock(now func() time.Time) Option { return func(g *Gate) { g.now = now } }

// New builds a gate for a policy.
func New(policy Policy, opts ...Option) (*Gate, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	g := &Gate{policy: policy, mode: ModeFull, now: func() time.Time { return time.Now().UTC() },
		prevHash: GenesisHash, held: map[string]held{}}
	for _, o := range opts {
		o(g)
	}
	return g, nil
}

// Decide evaluates a call without executing or recording anything.
func (g *Gate) Decide(call Call) Decision {
	scope, known := g.policy.ToolScopes[call.Tool]
	if !known {
		return Decision{Outcome: DecisionDeny, Rule: ScopeRuleID, Scope: "", Approval: ApprovalAutomatic, Message: "tool is not in the provider's inventory"}
	}
	granted := g.policy.GrantedScopes
	if call.Scopes != nil {
		granted = call.Scopes
	}
	if !hasScope(granted, scope) {
		return Decision{Outcome: DecisionDeny, Rule: ScopeRuleID, Scope: scope, Approval: ApprovalAutomatic, Message: "scope not granted"}
	}
	if g.mode == ModeScope {
		return Decision{Outcome: DecisionAllow, Rule: ScopeRuleID, Scope: scope, Approval: ApprovalAutomatic}
	}
	for _, r := range g.policy.Rules {
		var fired bool
		var effect string
		if r.Kind == KindLookup {
			fired, effect = g.policy.evaluateLookup(r, call)
		} else {
			fired, effect = r.evaluate(call)
		}
		if !fired {
			continue
		}
		if effect == EffectEscalate {
			return Decision{Outcome: DecisionEscalate, Rule: r.ID, Scope: scope, Approval: ApprovalPendingHuman, Message: r.Message}
		}
		return Decision{Outcome: DecisionDeny, Rule: r.ID, Scope: scope, Approval: ApprovalAutomatic, Message: r.Message}
	}
	return Decision{Outcome: DecisionAllow, Rule: ScopeRuleID, Scope: scope, Approval: ApprovalAutomatic}
}

// Authorize decides the call, executes it when allowed, records the artifact,
// and publishes the toolcall events. It returns the artifact in every case;
// the error is ErrDenied or ErrEscalated when the tool did not run, or the
// executor's error.
func (g *Gate) Authorize(ctx context.Context, call Call, exec Executor) (Artifact, error) {
	d := g.Decide(call)
	a := g.newArtifact(call, d)
	var execErr error
	switch d.Outcome {
	case DecisionAllow:
		out, err := exec(ctx, call)
		if err != nil {
			a.Outcome = "failed: " + err.Error()
			execErr = err
		} else {
			a.Outcome = "executed: " + out
		}
	case DecisionDeny:
		a.Outcome = "refused"
		execErr = ErrDenied
	case DecisionEscalate:
		a.Outcome = "held: awaiting human approval"
		execErr = ErrEscalated
	}
	a = g.commit(a)
	if d.Outcome == DecisionEscalate {
		g.mu.Lock()
		g.held[a.Hash] = held{call: call, artifact: a, exec: exec}
		g.mu.Unlock()
	}
	g.publish(ctx, EventRequested, a)
	g.publish(ctx, EventDecided, a)
	switch d.Outcome {
	case DecisionAllow:
		g.publish(ctx, EventExecuted, a)
	case DecisionDeny:
		g.publish(ctx, EventRefused, a)
	case DecisionEscalate:
		g.publish(ctx, EventEscalated, a)
	}
	return a, execErr
}

// Resolve settles a held call. approver is the identity of the person who
// decided; approved true runs the executor. A second artifact is appended so
// the chain carries both the hold and the human decision.
func (g *Gate) Resolve(ctx context.Context, holdHash, approver string, approved bool) (Artifact, error) {
	g.mu.Lock()
	h, ok := g.held[holdHash]
	if ok {
		delete(g.held, holdHash)
	}
	g.mu.Unlock()
	if !ok {
		return Artifact{}, fmt.Errorf("toolgate: no held call with hash %s", holdHash)
	}
	a := h.artifact
	a.Approver = approver
	a.Timestamp = g.now()
	a.PrevHash, a.Hash = "", ""
	var execErr error
	if approved {
		a.Approval = ApprovalGranted
		a.Decision = DecisionAllow
		out, err := h.exec(ctx, h.call)
		if err != nil {
			a.Outcome = "failed: " + err.Error()
			execErr = err
		} else {
			a.Outcome = "executed: " + out
		}
	} else {
		a.Approval = ApprovalRejected
		a.Decision = DecisionDeny
		a.Outcome = "refused by approver"
		execErr = ErrDenied
	}
	a = g.commit(a)
	if approved {
		g.publish(ctx, EventApproved, a)
		g.publish(ctx, EventExecuted, a)
	} else {
		g.publish(ctx, EventRefused, a)
	}
	return a, execErr
}

// Records returns a copy of every artifact this gate has committed, in order.
func (g *Gate) Records() []Artifact {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]Artifact, len(g.records))
	copy(out, g.records)
	return out
}

func (g *Gate) newArtifact(call Call, d Decision) Artifact {
	a := Artifact{
		Timestamp: g.now(), AgentID: call.AgentID, Tool: call.Tool, Decision: d.Outcome, Rule: d.Rule,
		Scope: d.Scope, Approval: d.Approval, Integrity: IntegrityHashChain, Principal: call.Principal,
		SessionID: call.SessionID, TenantID: call.TenantID, ContractID: call.ContractID, CertID: call.CertID,
		TraceID: call.TraceID, CallID: call.ID, Provider: g.policy.Provider,
	}
	if g.mode == ModeFull {
		a.Args = call.Args
	}
	return a
}

// commit appends the artifact to the provider's chain.
func (g *Gate) commit(a Artifact) Artifact {
	g.mu.Lock()
	defer g.mu.Unlock()
	a.PrevHash = g.prevHash
	a.Hash = computeHash(g.prevHash, a)
	g.prevHash = a.Hash
	g.records = append(g.records, a)
	return a
}

func (g *Gate) publish(ctx context.Context, eventType string, a Artifact) {
	if g.publisher == nil {
		return
	}
	data := a.ToEventData()
	data["idempotency_key"] = eventType + "_" + a.Provider + "_" + a.Hash
	_ = g.publisher.Publish(ctx, eventType, data)
}
