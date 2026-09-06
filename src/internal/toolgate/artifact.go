package toolgate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// Decisions a gate can reach for one call.
const (
	DecisionAllow    = "allow"
	DecisionDeny     = "deny"
	DecisionEscalate = "escalate"
	// DecisionNone is recorded when the gate is off: no stage was consulted.
	DecisionNone = "none"
)

// Approval states.
const (
	ApprovalAutomatic    = "automatic"
	ApprovalPendingHuman = "pending-human"
	ApprovalGranted      = "human-approved"
	ApprovalRejected     = "human-rejected"
)

// Integrity levels of a record, in increasing strength.
const (
	IntegrityPlainLog   = "plain-log"
	IntegrityAppendOnly = "append-only"
	IntegrityHashChain  = "hash-chained"
)

// GenesisHash is the prev_hash of the first record in a provider's chain.
const GenesisHash = "0000000000000000"

// Artifact is the record the gate emits for one tool call. The first nine
// fields are the ones an approver can ask for; the ids after them join the
// record to the rest of AEX: the tenant, the contract, the agent's ACA
// certificate and the gateway request that carried the call.
type Artifact struct {
	Timestamp  time.Time      `json:"ts"`
	AgentID    string         `json:"agent_id"`
	Tool       string         `json:"tool"`
	Args       map[string]any `json:"args,omitempty"`
	Decision   string         `json:"decision"`
	Rule       string         `json:"rule,omitempty"`
	Scope      string         `json:"scope,omitempty"`
	Approval   string         `json:"approval,omitempty"`
	Outcome    string         `json:"outcome,omitempty"`
	Integrity  string         `json:"integrity"`
	Principal  string         `json:"principal,omitempty"`
	SessionID  string         `json:"session_id,omitempty"`
	TenantID   string         `json:"tenant_id,omitempty"`
	ContractID string         `json:"contract_id,omitempty"`
	CertID     string         `json:"cert_id,omitempty"`
	TraceID    string         `json:"trace_id,omitempty"`
	CallID     string         `json:"call_id,omitempty"`
	Provider   string         `json:"provider"`
	Approver   string         `json:"approver,omitempty"`
	PrevHash   string         `json:"prev_hash,omitempty"`
	Hash       string         `json:"hash,omitempty"`
}

// hashInput is the artifact without its own hash, serialized with sorted keys
// by encoding/json (struct fields are emitted in declaration order, which is
// stable), so the hash is reproducible from the record itself.
func (a Artifact) hashInput() []byte {
	c := a
	c.Hash = ""
	b, _ := json.Marshal(c)
	return b
}

// computeHash returns the first 16 hex characters of SHA-256 over the previous
// hash and the record, which is what the harness study used and enough to
// detect edits; a deployment can switch to the full digest without changing
// the verification rule.
func computeHash(prev string, a Artifact) string {
	h := sha256.New()
	h.Write([]byte(prev))
	h.Write(a.hashInput())
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// VerifyChain walks a provider's records in order and reports whether every
// record's hash matches its content and its predecessor. On failure it returns
// the index of the first record that does not verify.
func VerifyChain(records []Artifact) (ok bool, brokenAt int) {
	prev := GenesisHash
	for i, r := range records {
		if r.PrevHash != prev {
			return false, i
		}
		if computeHash(prev, r) != r.Hash {
			return false, i
		}
		prev = r.Hash
	}
	return true, -1
}

// FieldsPresent reports which of the nine approver-facing fields a reader can
// recover from the record. It is the basis of the Score metrics.
func (a Artifact) FieldsPresent() map[string]bool {
	return map[string]bool{
		"timestamp":        !a.Timestamp.IsZero(),
		"agent_identity":   a.AgentID != "",
		"tool_name":        a.Tool != "",
		"argument_values":  len(a.Args) > 0,
		"decision":         a.Decision != "" && a.Decision != DecisionNone,
		"rule_or_scope":    a.Rule != "" || a.Scope != "",
		"approval_status":  a.Approval != "",
		"outcome":          a.Outcome != "",
		"record_integrity": a.Integrity == IntegrityAppendOnly || a.Integrity == IntegrityHashChain,
	}
}

// ToEventData converts the artifact into the map the AEX event publisher
// takes as an event payload.
func (a Artifact) ToEventData() map[string]any {
	b, _ := json.Marshal(a)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}
