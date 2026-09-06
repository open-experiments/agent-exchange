// Command toolgate-client replays the harness call set against a live AEX
// deployment and scores what the deployment recorded. It is run 2c of the
// tool-call harness study (docs/TOOLGATE.md):
//
//	A  plain log    calls go straight to the provider's tools (ap-tools)
//	B  scope gate   calls go through aex-gateway, which validates the API key
//	                with aex-identity and enforces the route scope map
//	C  value gate   calls go through aex-gateway to aex-toolgate, which
//	                applies the argument-value rules, forwards allowed calls
//	                to ap-tools and records an artifact per call
//
// Modes:
//
//	toolgate-client run   -config B -calls calls.json -target http://aex-gateway:8080/v1/tools \
//	                      -identity http://aex-identity:8080 -scopes a,b,c -out /tmp/out
//	    Creates a tenant and an API key with the given scopes (when -identity
//	    is set and -api-key is not), replays the calls with the study's
//	    identity headers, writes responses_<config>.jsonl, and with -records
//	    fetches the gate's chain into artifacts_C.jsonl.
//
//	toolgate-client score -calls calls.json -policy policy.json -ap-log ap-tools.log \
//	                      -gw-log aex-gateway.log -gw-tenant tenant_x -records artifacts_C.jsonl -out out
//	    Turns each configuration's own record (the provider's log line, the
//	    gateway's log line, the gate's artifact) into the nine-field artifact
//	    shape and scores the three sets with the run 1 metrics.
package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/parlakisik/agent-exchange/internal/toolgate"
)

type call struct {
	ID   string         `json:"id"`
	Tool string         `json:"tool"`
	Args map[string]any `json:"args"`
	Note string         `json:"note"`
}

type response struct {
	CallID     string  `json:"call_id"`
	Tool       string  `json:"tool"`
	Config     string  `json:"config"`
	Status     int     `json:"status"`
	Decision   string  `json:"decision,omitempty"`
	Rule       string  `json:"rule,omitempty"`
	Hash       string  `json:"hash,omitempty"`
	RequestID  string  `json:"request_id,omitempty"`
	LatencyMs  float64 `json:"latency_ms"`
	Body       string  `json:"body"`
	TenantID   string  `json:"tenant_id,omitempty"`
	Timestamp  string  `json:"timestamp"`
	HoldHash   string  `json:"hold,omitempty"`
	Configured string  `json:"target"`
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: toolgate-client run|score [flags]")
	}
	switch os.Args[1] {
	case "run":
		runMode(os.Args[2:])
	case "score":
		scoreMode(os.Args[2:])
	default:
		log.Fatalf("unknown mode %q", os.Args[1])
	}
}

func loadCalls(path string) []call {
	b, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	var calls []call
	if err := json.Unmarshal(b, &calls); err != nil {
		log.Fatal(err)
	}
	return calls
}

// ---------------------------------------------------------------- run

func runMode(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	cfg := fs.String("config", "C", "A, B or C (labels the output; the target decides the path)")
	callsPath := fs.String("calls", "calls.json", "call set JSON")
	target := fs.String("target", "http://aex-gateway:8080/v1/tools", "base URL the calls are posted to (tool name appended)")
	identity := fs.String("identity", "", "aex-identity base URL; when set and -api-key is empty, a tenant and key are created")
	apiKey := fs.String("api-key", "", "API key to send as X-API-Key")
	scopes := fs.String("scopes", "", "comma-separated scopes for the created key (default: the seven of the study)")
	tenantName := fs.String("tenant-name", "", "name of the created tenant (default purdue-run2c-<config>)")
	records := fs.String("records", "", "aex-toolgate base URL; when set, the chain is fetched after the run into artifacts_<config>.jsonl")
	outDir := fs.String("out", "out", "output directory")
	approve := fs.Bool("approve-held", false, "after the run, approve the held call through the gate's API (C only)")
	dump := fs.Bool("dump", false, "print the output files to stdout at the end (for a Job whose filesystem is discarded)")
	_ = fs.Parse(args)

	calls := loadCalls(*callsPath)
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatal(err)
	}
	if *scopes == "" {
		*scopes = "invoices:read,invoices:approve,payments:send,vendors:update,email:send,reports:export,documents:read"
	}
	if *tenantName == "" {
		*tenantName = "purdue-run2c-" + *cfg
	}
	client := &http.Client{Timeout: 30 * time.Second}

	tenantID := ""
	if *identity != "" && *apiKey == "" {
		tenantID, *apiKey = bootstrap(client, *identity, *tenantName, strings.Split(*scopes, ","))
		fmt.Printf("tenant created: id=%s name=%s key_prefix=%s scopes=%s\n", tenantID, *tenantName, (*apiKey)[:12], *scopes)
	}

	f, err := os.Create(filepath.Join(*outDir, "responses_"+*cfg+".jsonl"))
	if err != nil {
		log.Fatal(err)
	}
	enc := json.NewEncoder(f)
	fmt.Printf("config %s: %d calls to %s\n", *cfg, len(calls), *target)
	fmt.Println("call  tool             status decision rule            latency_ms")
	var total float64
	holds := map[string]string{}
	for _, c := range calls {
		body, _ := json.Marshal(c.Args)
		req, _ := http.NewRequest(http.MethodPost, strings.TrimRight(*target, "/")+"/"+c.Tool, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if *apiKey != "" {
			req.Header.Set("X-API-Key", *apiKey)
		}
		// The study's fixed identities, the same as runs 1, 2a and 2b.
		req.Header.Set("X-AEX-Call-ID", c.ID)
		req.Header.Set("X-AEX-Agent-ID", "ap-assistant-v2")
		req.Header.Set("X-AEX-Principal", "user:j.doe@corp.example")
		req.Header.Set("X-AEX-Session-ID", "sess-7f3a")
		req.Header.Set("X-AEX-Contract-ID", "contract_789")
		req.Header.Set("X-AEX-Cert-ID", "cert_ap_assistant_v2")
		t0 := time.Now()
		resp, err := client.Do(req)
		lat := float64(time.Since(t0).Microseconds()) / 1000.0
		total += lat
		r := response{CallID: c.ID, Tool: c.Tool, Config: *cfg, LatencyMs: lat, Timestamp: t0.UTC().Format(time.RFC3339Nano), Configured: *target, TenantID: tenantID}
		if err != nil {
			r.Status = -1
			r.Body = err.Error()
		} else {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			r.Status = resp.StatusCode
			r.Body = strings.TrimSpace(string(b))
			r.Decision = resp.Header.Get("X-Toolgate-Decision")
			r.Rule = resp.Header.Get("X-Toolgate-Rule")
			r.Hash = resp.Header.Get("X-Toolgate-Hash")
			r.RequestID = resp.Header.Get("X-Request-ID")
			if r.Status == http.StatusAccepted {
				var m map[string]any
				_ = json.Unmarshal(b, &m)
				r.HoldHash, _ = m["hold"].(string)
				holds[c.ID] = r.HoldHash
			}
			if r.Decision == "" {
				// Not the gate's answer: read the gateway's or the tool's status.
				switch {
				case r.Status == http.StatusForbidden:
					r.Decision = "deny"
					var m map[string]any
					_ = json.Unmarshal(b, &m)
					if e, ok := m["error"].(map[string]any); ok {
						r.Rule, _ = e["required_scope"].(string)
					}
				case r.Status >= 200 && r.Status < 300:
					if *cfg == "A" {
						r.Decision = "none"
					} else {
						r.Decision = "allow"
					}
				}
			}
		}
		_ = enc.Encode(r)
		fmt.Printf("%-5s %-16s %6d %-8s %-15s %8.1f\n", c.ID, c.Tool, r.Status, r.Decision, r.Rule, r.LatencyMs)
	}
	_ = f.Close()
	fmt.Printf("mean end-to-end latency: %.1f ms over %d calls\n", total/float64(len(calls)), len(calls))

	if *approve && *records != "" {
		for id, h := range holds {
			body, _ := json.Marshal(map[string]any{"approver": "user:ap.manager@corp.example", "approved": true})
			resp, err := client.Post(strings.TrimRight(*records, "/")+"/v1/holds/"+h, "application/json", bytes.NewReader(body))
			if err != nil {
				log.Printf("approve %s: %v", id, err)
				continue
			}
			b, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			fmt.Printf("held call %s approved through the gate: status=%d %s\n", id, resp.StatusCode, truncate(string(b), 160))
		}
	}
	if *records != "" {
		resp, err := client.Get(strings.TrimRight(*records, "/") + "/v1/records")
		if err != nil {
			log.Fatalf("records: %v", err)
		}
		var recs []toolgate.Artifact
		if err := json.NewDecoder(resp.Body).Decode(&recs); err != nil {
			log.Fatalf("records: %v", err)
		}
		_ = resp.Body.Close()
		rf, _ := os.Create(filepath.Join(*outDir, "artifacts_"+*cfg+".jsonl"))
		e := json.NewEncoder(rf)
		for _, a := range recs {
			_ = e.Encode(a)
		}
		_ = rf.Close()
		ok, at := toolgate.VerifyChain(recs)
		fmt.Printf("gate chain fetched: %d records, verified=%v (failed_at=%d)\n", len(recs), ok, at)
		vr, err := client.Get(strings.TrimRight(*records, "/") + "/v1/records/verify")
		if err == nil {
			b, _ := io.ReadAll(vr.Body)
			_ = vr.Body.Close()
			fmt.Printf("gate's own verification: %s\n", strings.TrimSpace(string(b)))
		}
	}
	if *dump {
		for _, name := range []string{"responses_" + *cfg + ".jsonl", "artifacts_" + *cfg + ".jsonl"} {
			b, err := os.ReadFile(filepath.Join(*outDir, name))
			if err != nil {
				continue
			}
			fmt.Printf("----- BEGIN %s -----\n%s----- END %s -----\n", name, b, name)
		}
	}
}

func bootstrap(client *http.Client, identity, name string, scopes []string) (tenantID, key string) {
	body, _ := json.Marshal(map[string]any{"name": name, "type": "PROVIDER", "contact_email": "purdue-run2c@narlabs.io",
		"metadata": map[string]any{"purpose": "TCM 55000 harness run 2c"}})
	resp, err := client.Post(strings.TrimRight(identity, "/")+"/v1/tenants", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Fatalf("identity: create tenant: %v", err)
	}
	var t map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&t)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		log.Fatalf("identity: create tenant: status %d %v", resp.StatusCode, t)
	}
	tenantID, _ = t["id"].(string)
	body, _ = json.Marshal(map[string]any{"name": "harness-" + name, "scopes": scopes})
	resp, err = client.Post(strings.TrimRight(identity, "/")+"/v1/tenants/"+tenantID+"/api-keys", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Fatalf("identity: create key: %v", err)
	}
	var k map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&k)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		log.Fatalf("identity: create key: status %d %v", resp.StatusCode, k)
	}
	key, _ = k["key"].(string)
	if key == "" {
		log.Fatal("identity returned no key")
	}
	return tenantID, key
}

// ---------------------------------------------------------------- score

func scoreMode(args []string) {
	fs := flag.NewFlagSet("score", flag.ExitOnError)
	callsPath := fs.String("calls", "calls.json", "call set JSON")
	policyPath := fs.String("policy", "policy.json", "policy JSON, for the rule count")
	apLog := fs.String("ap-log", "", "ap-tools log captured after the A run (configuration A's records)")
	gwLog := fs.String("gw-log", "", "aex-gateway log captured after the B run (configuration B's records, JSON lines)")
	gwTenant := fs.String("gw-tenant", "", "keep only gateway lines for this tenant id (the B run's tenant)")
	recPath := fs.String("records", "", "artifacts_C.jsonl fetched from aex-toolgate (configuration C's records)")
	gwLogC := fs.String("gw-log-c", "", "aex-gateway log captured after the C run; calls the gateway refused before the gate are shown in the matrix from it")
	gwTenantC := fs.String("gw-tenant-c", "", "keep only gateway lines for this tenant id (the C run's tenant)")
	outDir := fs.String("out", "out", "output directory")
	_ = fs.Parse(args)

	calls := loadCalls(*callsPath)
	policy, err := toolgate.LoadPolicy(*policyPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatal(err)
	}

	recA := parseAPLog(*apLog, calls)
	recB := parseGatewayLog(*gwLog, *gwTenant, calls)
	recC := parseRecords(*recPath, calls)
	sets := map[string][]toolgate.Artifact{"A": recA, "B": recB, "C": recC}
	names := map[string]string{"A": "plain-log", "B": "scope-gate-live-gateway", "C": "value-gate-service"}
	for k, recs := range sets {
		f, _ := os.Create(filepath.Join(*outDir, "artifacts_"+k+".jsonl"))
		e := json.NewEncoder(f)
		for _, r := range recs {
			_ = e.Encode(r)
		}
		_ = f.Close()
	}

	dm, _ := os.Create(filepath.Join(*outDir, "decision_matrix.csv"))
	w := csv.NewWriter(dm)
	_ = w.Write([]string{"call_id", "tool", "note", "A_plain_log", "B_scope_gate", "C_value_gate", "C_rule"})
	byID := func(recs []toolgate.Artifact) map[string]toolgate.Artifact {
		m := map[string]toolgate.Artifact{}
		for _, r := range recs {
			if _, seen := m[r.CallID]; !seen {
				m[r.CallID] = r
			}
		}
		return m
	}
	mA, mB, mC := byID(recA), byID(recB), byID(recC)
	// In the layered deployment the gateway's scope check runs before the gate,
	// so a call it refuses never reaches the gate; the gateway's line is that
	// call's only record on the C path.
	mGC := byID(parseGatewayLog(*gwLogC, *gwTenantC, calls))
	pathExec, pathRefused, pathHeld := 0, 0, 0
	for _, c := range calls {
		cDecision, cRule := decisionOr(mC[c.ID], "missing"), mC[c.ID].Rule
		if cDecision == "missing" {
			if g, ok := mGC[c.ID]; ok && g.Decision == toolgate.DecisionDeny {
				cDecision, cRule = "deny (gateway)", g.Scope
			}
		}
		switch {
		case strings.HasPrefix(cDecision, "deny"):
			pathRefused++
		case cDecision == toolgate.DecisionEscalate:
			pathHeld++
		case cDecision == toolgate.DecisionAllow:
			pathExec++
		}
		_ = w.Write([]string{c.ID, c.Tool, c.Note, decisionOr(mA[c.ID], "none"), decisionOr(mB[c.ID], "missing"), cDecision, cRule})
	}
	w.Flush()
	_ = dm.Close()
	fmt.Printf("C path (gateway then gate) over the %d calls: executed %d, refused %d, held %d\n", len(calls), pathExec, pathRefused, pathHeld)

	sc, _ := os.Create(filepath.Join(*outDir, "nian_scores.csv"))
	w = csv.NewWriter(sc)
	_ = w.Write([]string{"config", "config_name", "records", "action_recoverability", "lifecycle_coverage", "policy_checkability",
		"responsibility_attribution", "evidence_integrity_ordinal", "schema_fields_recoverable_of_9", "calls_executed", "calls_refused", "calls_escalated"})
	fmt.Println("config  records executed refused escalated  recover lifecycle checkable attrib integrity fields")
	for _, k := range []string{"A", "B", "C"} {
		recs := sets[k]
		s := toolgate.Score(recs, len(policy.Rules))
		ex, rf, es := 0, 0, 0
		for _, r := range recs {
			switch r.Decision {
			case toolgate.DecisionAllow, toolgate.DecisionNone:
				ex++
			case toolgate.DecisionDeny:
				rf++
			case toolgate.DecisionEscalate:
				es++
			}
		}
		_ = w.Write([]string{k, names[k], fmt.Sprint(s.Records), f2(s.ActionRecoverability), f2(s.LifecycleCoverage), f2(s.PolicyCheckability),
			f2(s.ResponsibilityAttribution), f2(s.EvidenceIntegrity), f2(s.FieldsRecoverableOf9), fmt.Sprint(ex), fmt.Sprint(rf), fmt.Sprint(es)})
		fmt.Printf("%s %-24s %3d %8d %7d %9d   %5.2f %9.2f %9.2f %6.2f %9.2f %6.2f\n", k, names[k], s.Records, ex, rf, es,
			s.ActionRecoverability, s.LifecycleCoverage, s.PolicyCheckability, s.ResponsibilityAttribution, s.EvidenceIntegrity, s.FieldsRecoverableOf9)
	}
	w.Flush()
	_ = sc.Close()
	if ok, at := toolgate.VerifyChain(recC); ok {
		fmt.Printf("value-gate chain verified over %d records\n", len(recC))
	} else {
		fmt.Printf("value-gate chain FAILED at record %d\n", at)
	}
	fmt.Printf("outputs in %s\n", *outDir)
}

func decisionOr(a toolgate.Artifact, def string) string {
	if a.Decision == "" {
		return def
	}
	return a.Decision
}

// parseAPLog reads the mock provider's default log lines in order and pairs
// them with the call set by position: the line holds no call id, which is
// itself part of what configuration A fails to record.
func parseAPLog(path string, calls []call) []toolgate.Artifact {
	var out []toolgate.Artifact
	if path == "" {
		return out
	}
	i := 0
	for _, line := range readLines(path) {
		if !strings.Contains(line, "agent.tools invoking ") {
			continue
		}
		parts := strings.Fields(line)
		ts, _ := time.Parse(time.RFC3339Nano, parts[0])
		tool := ""
		for j, p := range parts {
			if p == "invoking" && j+1 < len(parts) {
				tool = parts[j+1]
			}
		}
		id := ""
		if i < len(calls) {
			id = calls[i].ID
		}
		out = append(out, toolgate.Artifact{Timestamp: ts, Tool: tool, Decision: toolgate.DecisionNone, Integrity: toolgate.IntegrityPlainLog, CallID: id, Provider: "ap-tools"})
		i++
	}
	return out
}

// parseGatewayLog reads aex-gateway's JSON request lines for /v1/tools/ and
// turns each into the artifact shape with exactly the fields the line holds.
func parseGatewayLog(path, tenant string, calls []call) []toolgate.Artifact {
	var out []toolgate.Artifact
	if path == "" {
		return out
	}
	i := 0
	for _, line := range readLines(path) {
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil || m["msg"] != "request" {
			continue
		}
		tool, _ := m["tool"].(string)
		if tool == "" {
			continue
		}
		if tenant != "" && m["tenant_id"] != tenant {
			continue
		}
		ts, _ := time.Parse(time.RFC3339Nano, fmt.Sprint(m["time"]))
		status := int(m["status"].(float64))
		a := toolgate.Artifact{Timestamp: ts, Tool: tool, Integrity: toolgate.IntegrityPlainLog, Provider: "aex-gateway"}
		a.TenantID, _ = m["tenant_id"].(string)
		a.TraceID, _ = m["request_id"].(string)
		a.Scope, _ = m["scope_required"].(string)
		switch fmt.Sprint(m["scope_decision"]) {
		case "allow":
			a.Decision = toolgate.DecisionAllow
			a.Outcome = fmt.Sprintf("http %d", status)
		case "deny":
			a.Decision = toolgate.DecisionDeny
			a.Outcome = "refused"
		}
		if i < len(calls) {
			a.CallID = calls[i].ID
		}
		out = append(out, a)
		i++
	}
	return out
}

func parseRecords(path string, calls []call) []toolgate.Artifact {
	var out []toolgate.Artifact
	if path == "" {
		return out
	}
	for _, line := range readLines(path) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var a toolgate.Artifact
		if err := json.Unmarshal([]byte(line), &a); err != nil {
			log.Fatalf("records: %v", err)
		}
		out = append(out, a)
	}
	return out
}

func readLines(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func f2(x float64) string { return fmt.Sprintf("%.2f", x) }
