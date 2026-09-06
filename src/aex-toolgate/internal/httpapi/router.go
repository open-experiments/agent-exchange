// Package httpapi is the service form of internal/toolgate: an authorizing
// reverse proxy a provider puts in front of its tool endpoint. Every call the
// provider's agent makes arrives as POST /v1/tools/{tool} with the argument
// values as the JSON body and the caller's identity in headers. The gate
// decides, forwards the allowed calls to the upstream tool, records one
// artifact per call on the provider's hash chain, and publishes the
// toolcall.* events. Held calls are settled through POST /v1/holds/{hash}.
//
// Identity headers, all optional except where noted:
//
//	X-Tenant-ID       the tenant the gateway validated (attribution)
//	X-Request-ID      the gateway's request id; becomes the artifact's trace_id
//	X-Scopes          comma-separated validated scopes; they intersect with
//	                  the policy's granted_scopes and can only narrow it. The
//	                  header is caller-controlled, so it never widens a grant:
//	                  "*" leaves the policy's grant as it stands.
//	X-AEX-Agent-ID    the acting agent
//	X-AEX-Principal   the person the agent acts for
//	X-AEX-Session-ID  the agent session
//	X-AEX-Contract-ID the AEX contract the work belongs to
//	X-AEX-Cert-ID     the agent's ACA certificate
//	X-AEX-Call-ID     the caller's own id for the call (else the request id)
//
// Those headers are attribution, not authentication: they are whatever the
// caller sent, and none of them widens an authorization decision.
//
// The operator endpoints (POST /v1/holds/{hash}, GET /v1/records and
// /v1/records/verify) are outside that model. They belong to the provider's
// operator, not to the agent being gated, and require
// Authorization: Bearer <OPERATOR_TOKEN>. With no token configured they are
// refused rather than left open.
package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/parlakisik/agent-exchange/internal/toolgate"
)

// Server holds the gate and the upstream it forwards to.
type Server struct {
	gate           *toolgate.Gate
	upstreamURL    string
	upstreamPrefix string
	client         *http.Client
	started        time.Time
	operatorToken  string
	exposeArgs     bool
}

// Option configures the server.
type Option func(*Server)

// WithOperatorToken sets the bearer token the operator endpoints require:
// resolving a held call and reading the record chain. Those endpoints are not
// for the agent being gated — an agent that can settle its own holds defeats
// the escalate effect, and one that can read the chain reads every other
// call's argument values. Without a token they are refused outright.
func WithOperatorToken(t string) Option {
	return func(s *Server) { s.operatorToken = strings.TrimSpace(t) }
}

// WithExposeArgs serves full argument values on GET /v1/records. Off by
// default: in ModeFull an artifact carries the values verbatim, which for the
// shipped policy means payment amounts and recipient account numbers.
func WithExposeArgs(v bool) Option { return func(s *Server) { s.exposeArgs = v } }

// New builds the handler.
func New(gate *toolgate.Gate, upstreamURL, upstreamPrefix string, timeout time.Duration, opts ...Option) http.Handler {
	s := &Server{gate: gate, upstreamURL: strings.TrimRight(upstreamURL, "/"), upstreamPrefix: upstreamPrefix,
		client: &http.Client{Timeout: timeout}, started: time.Now()}
	for _, o := range opts {
		o(s)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /ready", s.health)
	mux.HandleFunc("POST /v1/tools/{tool}", s.authorize)
	mux.HandleFunc("POST /v1/holds/{hash}", s.resolve)
	mux.HandleFunc("GET /v1/records", s.records)
	mux.HandleFunc("GET /v1/records/verify", s.verify)
	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "healthy", "uptime_s": int(time.Since(s.started).Seconds())})
}

// upstreamResult is what the executor captured from the tool.
type upstreamResult struct {
	status int
	body   []byte
	ctype  string
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) {
	tool := r.PathValue("tool")
	if r.Header.Get("X-Request-ID") == "" {
		// A call that did not come through the gateway still gets a trace id.
		r.Header.Set("X-Request-ID", newRequestID())
	}
	w.Header().Set("X-Request-ID", r.Header.Get("X-Request-ID"))
	var args map[string]any
	if r.Body != nil {
		b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "cannot read body")
			return
		}
		if len(bytes.TrimSpace(b)) > 0 {
			// UseNumber keeps integers exact. Decoding into float64 rewrites
			// anything past 2^53 (an account or invoice number), so the tool
			// would receive a different value than the agent sent and the
			// artifact would record the rewritten one.
			dec := json.NewDecoder(bytes.NewReader(b))
			dec.UseNumber()
			if err := dec.Decode(&args); err != nil {
				writeError(w, http.StatusBadRequest, "bad_request", "body must be a JSON object of argument values")
				return
			}
		}
	}
	call := callFromRequest(r, tool, args)

	var res upstreamResult
	exec := func(ctx context.Context, c toolgate.Call) (string, error) {
		out, err := s.forward(ctx, r, c)
		if err != nil {
			return "", err
		}
		res = out
		summary := strings.TrimSpace(string(out.body))
		if len(summary) > 120 {
			summary = summary[:120] + "..."
		}
		if out.status >= 400 {
			return fmt.Sprintf("http %d: %s", out.status, summary), fmt.Errorf("upstream returned %d", out.status)
		}
		return fmt.Sprintf("http %d: %s", out.status, summary), nil
	}

	a, err := s.gate.Authorize(r.Context(), call, exec)
	w.Header().Set("X-Toolgate-Decision", a.Decision)
	w.Header().Set("X-Toolgate-Rule", a.Rule)
	w.Header().Set("X-Toolgate-Hash", a.Hash)
	switch {
	case errors.Is(err, toolgate.ErrDenied):
		writeJSON(w, http.StatusForbidden, map[string]any{"decision": a.Decision, "rule": a.Rule, "scope": a.Scope,
			"message": s.gate.Decide(call).Message, "hash": a.Hash, "call_id": a.CallID})
	case errors.Is(err, toolgate.ErrEscalated):
		writeJSON(w, http.StatusAccepted, map[string]any{"decision": a.Decision, "rule": a.Rule, "scope": a.Scope,
			"approval": a.Approval, "message": s.gate.Decide(call).Message, "hold": a.Hash, "call_id": a.CallID})
	case err != nil && res.status == 0:
		writeJSON(w, http.StatusBadGateway, map[string]any{"decision": a.Decision, "rule": a.Rule, "hash": a.Hash,
			"error": "upstream unreachable: " + err.Error()})
	default:
		// The tool ran (or returned an error status of its own); hand its answer back.
		if res.ctype != "" {
			w.Header().Set("Content-Type", res.ctype)
		}
		w.WriteHeader(res.status)
		_, _ = w.Write(res.body)
	}
}

// forward sends the allowed call to the upstream tool endpoint.
func (s *Server) forward(ctx context.Context, in *http.Request, c toolgate.Call) (upstreamResult, error) {
	body, _ := json.Marshal(c.Args)
	if c.Args == nil {
		body = []byte("{}")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.upstreamURL+s.upstreamPrefix+"/"+c.Tool, bytes.NewReader(body))
	if err != nil {
		return upstreamResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	for _, h := range []string{"X-Tenant-ID", "X-Request-ID", "X-AEX-Agent-ID", "X-AEX-Principal", "X-AEX-Session-ID", "X-AEX-Contract-ID", "X-AEX-Cert-ID", "X-AEX-Call-ID"} {
		if v := in.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return upstreamResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return upstreamResult{status: resp.StatusCode, body: b, ctype: resp.Header.Get("Content-Type")}, nil
}

// requireOperator gates the endpoints that are not for the gated agent. It
// fails closed: with no token configured the endpoint is unavailable rather
// than open, because the alternative is that a deployment which forgot to set
// one lets the agent settle its own holds and read every record.
func (s *Server) requireOperator(w http.ResponseWriter, r *http.Request) bool {
	if s.operatorToken == "" {
		writeError(w, http.StatusServiceUnavailable, "operator_token_not_configured",
			"this endpoint needs OPERATOR_TOKEN set on the gate")
		return false
	}
	got := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if subtle.ConstantTimeCompare([]byte(got), []byte(s.operatorToken)) != 1 {
		writeError(w, http.StatusUnauthorized, "unauthorized", "operator credential required")
		return false
	}
	return true
}

func (s *Server) resolve(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperator(w, r) {
		return
	}
	hash := r.PathValue("hash")
	var body struct {
		Approver string `json:"approver"`
		Approved bool   `json:"approved"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Approver) == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "body needs approver and approved")
		return
	}
	a, err := s.gate.Resolve(r.Context(), hash, body.Approver, body.Approved)
	if err != nil && !errors.Is(err, toolgate.ErrDenied) {
		if strings.Contains(err.Error(), "no held call") {
			writeError(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
		// The approver allowed it and the tool failed: the record says so.
		writeJSON(w, http.StatusBadGateway, a)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) records(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperator(w, r) {
		return
	}
	recs := s.gate.Records()
	if !s.exposeArgs {
		for i := range recs {
			recs[i].Args = nil
		}
	}
	writeJSON(w, http.StatusOK, recs)
}

func (s *Server) verify(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperator(w, r) {
		return
	}
	recs := s.gate.Records()
	ok, at := toolgate.VerifyChain(recs)
	// chain_id changes on every restart, so a consumer can tell a verified
	// chain that continues the previous one from a verified chain that
	// started over at genesis. publish_failures being non-zero means the
	// durable copy of this chain is incomplete.
	writeJSON(w, http.StatusOK, map[string]any{"ok": ok, "records": len(recs), "failed_at": at,
		"chain_id": s.gate.ChainID(), "publish_failures": s.gate.PublishFailures()})
}

func callFromRequest(r *http.Request, tool string, args map[string]any) toolgate.Call {
	h := r.Header
	callID := h.Get("X-AEX-Call-ID")
	if callID == "" {
		callID = h.Get("X-Request-ID")
	}
	c := toolgate.Call{ID: callID, Tool: tool, Args: args, Principal: h.Get("X-AEX-Principal"), AgentID: h.Get("X-AEX-Agent-ID"),
		SessionID: h.Get("X-AEX-Session-ID"), TenantID: h.Get("X-Tenant-ID"), ContractID: h.Get("X-AEX-Contract-ID"),
		CertID: h.Get("X-AEX-Cert-ID"), TraceID: h.Get("X-Request-ID")}
	if v, ok := h["X-Scopes"]; ok && len(v) > 0 {
		c.Scopes = []string{}
		for _, s := range strings.Split(v[0], ",") {
			if s = strings.TrimSpace(s); s != "" {
				c.Scopes = append(c.Scopes, s)
			}
		}
	}
	return c
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": msg}})
}

func newRequestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
