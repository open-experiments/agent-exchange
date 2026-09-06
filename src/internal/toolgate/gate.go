package toolgate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
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
	// Scopes, when set, are the caller's scopes for this call (for example the
	// API-key scopes the gateway forwards in X-Scopes). They intersect with
	// the policy's GrantedScopes: they can only narrow the grant, never widen
	// it, because they arrive in a header the caller controls. Nil means use
	// the policy's grant unchanged.
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
	chainID   string
	mu        sync.Mutex
	prevHash  string
	records   []Artifact
	held      map[string]held

	publishFailures int
	recordsDropped  bool
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
		prevHash: GenesisHash, held: map[string]held{}, chainID: newChainID()}
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
	granted := narrow(g.policy.GrantedScopes, call.Scopes)
	if !hasScope(granted, scope) {
		return Decision{Outcome: DecisionDeny, Rule: ScopeRuleID, Scope: scope, Approval: ApprovalAutomatic, Message: "scope not granted"}
	}
	if g.mode == ModeScope {
		return Decision{Outcome: DecisionAllow, Rule: ScopeRuleID, Scope: scope, Approval: ApprovalAutomatic}
	}
	for _, r := range g.policy.Rules {
		var fired bool
		var effect, reason string
		if r.Kind == KindLookup {
			fired, effect, reason = g.policy.evaluateLookup(r, call)
		} else {
			fired, effect, reason = r.evaluate(call)
		}
		if !fired {
			continue
		}
		// A rule that fired because the gate could not check it reports that,
		// rather than the rule's own message, which would describe a breach
		// that may not have happened.
		msg := r.Message
		if reason != "" {
			msg = reason
		}
		if effect == EffectEscalate {
			return Decision{Outcome: DecisionEscalate, Rule: r.ID, Scope: scope, Approval: ApprovalPendingHuman, Message: msg}
		}
		return Decision{Outcome: DecisionDeny, Rule: r.ID, Scope: scope, Approval: ApprovalAutomatic, Message: msg}
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
		if len(g.held) < MaxHeld {
			g.held[a.Hash] = held{call: call, artifact: a, exec: exec}
		} else {
			slog.Warn("toolgate: hold table full, escalated call cannot be settled",
				"provider", a.Provider, "hash", a.Hash, "max_held", MaxHeld)
		}
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
// MaxRecords caps the in-memory chain. The gated agent drives how many
// artifacts are created, so an unbounded slice is a denial of service it can
// trigger on its own. Older entries are dropped once the cap is reached; the
// durable copy is the published event stream, not this slice.
const MaxRecords = 10000

// MaxHeld caps calls awaiting human approval, for the same reason.
const MaxHeld = 1000

func (g *Gate) commit(a Artifact) Artifact {
	g.mu.Lock()
	defer g.mu.Unlock()
	a.PrevHash = g.prevHash
	a.Hash = computeHash(g.prevHash, a)
	g.prevHash = a.Hash
	g.records = append(g.records, a)
	if len(g.records) > MaxRecords {
		g.records = g.records[len(g.records)-MaxRecords:]
		g.recordsDropped = true
	}
	return a
}

func (g *Gate) publish(ctx context.Context, eventType string, a Artifact) {
	if g.publisher == nil {
		return
	}
	data := a.ToEventData()
	data["idempotency_key"] = eventType + "_" + a.Provider + "_" + a.Hash
	if err := g.publisher.Publish(ctx, eventType, data); err != nil {
		// The durable record is the point of this component, so a dropped
		// event is not a silent condition: the call has already been decided
		// (and possibly executed) and only the in-memory chain survives.
		g.mu.Lock()
		g.publishFailures++
		n := g.publishFailures
		g.mu.Unlock()
		slog.Error("toolgate: audit event not published",
			"event_type", eventType, "provider", a.Provider, "hash", a.Hash,
			"chain_id", g.chainID, "publish_failures", n, "error", err)
	}
}

func newChainID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "chain-unknown"
	}
	return hex.EncodeToString(b)
}

// ChainID identifies this gate's chain. It is generated when the gate is
// built, so a restart produces a new id: a consumer that sees the id change
// knows the chain below it is a fresh one starting at GenesisHash, and that
// the previous records live only in whatever consumed the events. Without it
// a restarted gate is indistinguishable from one whose history was discarded,
// because VerifyChain reports ok over any self-consistent chain.
func (g *Gate) ChainID() string { return g.chainID }

// RecordsDropped reports whether the in-memory chain has been truncated at
// MaxRecords, in which case VerifyChain covers only the surviving tail.
func (g *Gate) RecordsDropped() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.recordsDropped
}

// PublishFailures is the number of audit events the gate could not publish.
// Non-zero means the durable record is incomplete.
func (g *Gate) PublishFailures() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.publishFailures
}
