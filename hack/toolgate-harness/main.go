// Command toolgate-harness replays a fixed set of agent tool calls through
// three authorization configurations and records what each one emits:
//
//	A  plain log      no gate; a framework-style log line per call
//	B  scope gate     toolgate in ModeScope (tool name against granted scopes)
//	C  value gate     toolgate in ModeFull  (scope, then argument-value rules)
//
// It writes the emitted records, a decision matrix and the five auditability
// scores of Nian et al. (2026) per configuration, in the same shape as run 1
// of the harness study (see docs/TOOLGATE.md), so the two runs compare.
//
// With -nats the gate publishes toolcall.* events through the AEX event
// publisher into the TOOLCALL JetStream stream; without it, records stay in
// memory and on disk.
package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/parlakisik/agent-exchange/internal/events"
	aexnats "github.com/parlakisik/agent-exchange/internal/nats"
	"github.com/parlakisik/agent-exchange/internal/toolgate"
)

type call struct {
	ID   string         `json:"id"`
	Tool string         `json:"tool"`
	Args map[string]any `json:"args"`
	Note string         `json:"note"`
}

func main() {
	policyPath := flag.String("policy", "../../src/internal/toolgate/testdata/policy.json", "policy JSON")
	callsPath := flag.String("calls", "../../src/internal/toolgate/testdata/calls.json", "call set JSON")
	outDir := flag.String("out", "out", "output directory")
	natsURL := flag.String("nats", "", "NATS URL; when set, events are published to JetStream")
	verify := flag.Bool("verify", false, "after the run, read the TOOLCALL stream back and print its state (needs -nats)")
	printOut := flag.Bool("print", false, "print the decision matrix and scores to stdout as well")
	flag.Parse()

	policy, err := toolgate.LoadPolicy(*policyPath)
	if err != nil {
		log.Fatal(err)
	}
	b, err := os.ReadFile(*callsPath)
	if err != nil {
		log.Fatal(err)
	}
	var calls []call
	if err := json.Unmarshal(b, &calls); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatal(err)
	}

	var publisher toolgate.Publisher
	var nc *aexnats.Client
	if *natsURL != "" {
		cfg := aexnats.DefaultConfig()
		cfg.URL = *natsURL
		cfg.Name = "toolgate-harness"
		var err error
		nc, err = aexnats.Connect(cfg)
		if err != nil {
			log.Fatal(err)
		}
		defer nc.Close()
		if err := nc.EnsureStreams(); err != nil {
			log.Fatal(err)
		}
		publisher = events.NewPublisherWithNATS("toolgate-harness", nc)
	}

	ctx := context.Background()
	exec := func(_ context.Context, c toolgate.Call) (string, error) {
		return "side effect applied", nil
	}
	mk := func(c call) toolgate.Call {
		return toolgate.Call{ID: c.ID, Tool: c.Tool, Args: c.Args, Principal: "user:j.doe@corp.example",
			AgentID: "ap-assistant-v2", SessionID: "sess-7f3a", TenantID: "tenant_123",
			ContractID: "contract_789", CertID: "cert_ap_assistant_v2", TraceID: "req-" + c.ID}
	}

	// A: no gate. A framework-style log line: timestamp, tool, truncated input.
	var recA []toolgate.Artifact
	for _, c := range calls {
		recA = append(recA, toolgate.Artifact{Timestamp: time.Now().UTC(), Tool: c.Tool,
			Decision: toolgate.DecisionNone, Integrity: toolgate.IntegrityPlainLog, CallID: c.ID, Provider: policy.Provider})
	}
	// B and C.
	run := func(mode toolgate.Mode) ([]toolgate.Artifact, time.Duration) {
		opts := []toolgate.Option{toolgate.WithMode(mode)}
		if publisher != nil {
			opts = append(opts, toolgate.WithPublisher(publisher))
		}
		g, err := toolgate.New(policy, opts...)
		if err != nil {
			log.Fatal(err)
		}
		var total time.Duration
		for _, c := range calls {
			t0 := time.Now()
			_, _ = g.Authorize(ctx, mk(c), exec)
			total += time.Since(t0)
		}
		return g.Records(), total / time.Duration(len(calls))
	}
	recB, latB := run(toolgate.ModeScope)
	recC, latC := run(toolgate.ModeFull)

	sets := map[string][]toolgate.Artifact{"A": recA, "B": recB, "C": recC}
	names := map[string]string{"A": "plain-log", "B": "scope-gate", "C": "value-gate"}
	for k, recs := range sets {
		f, err := os.Create(filepath.Join(*outDir, "artifacts_"+k+".jsonl"))
		if err != nil {
			log.Fatal(err)
		}
		enc := json.NewEncoder(f)
		for _, r := range recs {
			_ = enc.Encode(r)
		}
		_ = f.Close()
	}

	// Decision matrix.
	dm, _ := os.Create(filepath.Join(*outDir, "decision_matrix.csv"))
	w := csv.NewWriter(dm)
	_ = w.Write([]string{"call_id", "tool", "note", "A_plain_log", "B_scope_gate", "C_value_gate", "C_rule"})
	for i, c := range calls {
		_ = w.Write([]string{c.ID, c.Tool, c.Note, "none", recB[i].Decision, recC[i].Decision, recC[i].Rule})
	}
	w.Flush()
	_ = dm.Close()

	// Scores.
	sc, _ := os.Create(filepath.Join(*outDir, "nian_scores.csv"))
	w = csv.NewWriter(sc)
	_ = w.Write([]string{"config", "config_name", "action_recoverability", "lifecycle_coverage", "policy_checkability",
		"responsibility_attribution", "evidence_integrity_ordinal", "schema_fields_recoverable_of_9", "mean_gate_latency_us",
		"calls_executed", "calls_refused", "calls_escalated"})
	lat := map[string]time.Duration{"A": 0, "B": latB, "C": latC}
	fmt.Println("config  executed refused escalated  recover lifecycle checkable attrib integrity  latency_us")
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
		_ = w.Write([]string{k, names[k], f(s.ActionRecoverability), f(s.LifecycleCoverage), f(s.PolicyCheckability),
			f(s.ResponsibilityAttribution), f(s.EvidenceIntegrity), f(s.FieldsRecoverableOf9),
			fmt.Sprintf("%.1f", float64(lat[k].Nanoseconds())/1000.0), fmt.Sprint(ex), fmt.Sprint(rf), fmt.Sprint(es)})
		fmt.Printf("%s %-11s %3d %7d %9d   %5.2f %9.2f %9.2f %6.2f %9.0f   %.1f\n", k, names[k], ex, rf, es,
			s.ActionRecoverability, s.LifecycleCoverage, s.PolicyCheckability, s.ResponsibilityAttribution, s.EvidenceIntegrity,
			float64(lat[k].Nanoseconds())/1000.0)
	}
	w.Flush()
	_ = sc.Close()

	if ok, at := toolgate.VerifyChain(recC); !ok {
		log.Fatalf("value-gate chain failed to verify at record %d", at)
	}
	fmt.Printf("\nvalue-gate chain verified over %d records; outputs in %s\n", len(recC), *outDir)

	if *printOut {
		for _, name := range []string{"decision_matrix.csv", "nian_scores.csv"} {
			b, _ := os.ReadFile(filepath.Join(*outDir, name))
			fmt.Printf("\n--- %s ---\n%s", name, b)
		}
	}

	if *verify && nc != nil {
		js := nc.JetStream()
		info, err := js.StreamInfo("TOOLCALL")
		if err != nil {
			log.Fatalf("TOOLCALL stream info: %v", err)
		}
		fmt.Printf("\n--- TOOLCALL stream ---\nmessages=%d bytes=%d first_seq=%d last_seq=%d subjects=%v retention=%s max_age=%s\n",
			info.State.Msgs, info.State.Bytes, info.State.FirstSeq, info.State.LastSeq, info.Config.Subjects, info.Config.Retention, info.Config.MaxAge)
		for _, subj := range []string{"toolcall.decided", "toolcall.refused", "toolcall.escalated"} {
			m, err := js.GetLastMsg("TOOLCALL", subj)
			if err != nil {
				fmt.Printf("last %s: %v\n", subj, err)
				continue
			}
			var env map[string]any
			_ = json.Unmarshal(m.Data, &env)
			data, _ := env["data"].(map[string]any)
			fmt.Printf("last %s: seq=%d event_id=%v source=%v call_id=%v tool=%v decision=%v rule=%v hash=%v\n",
				subj, m.Sequence, env["event_id"], env["source"], data["call_id"], data["tool"], data["decision"], data["rule"], data["hash"])
		}
		// Every event the run published should be in the stream: per configuration B and C,
		// 12 calls x (requested + decided + one outcome event) = 36 events each.
		want := uint64(2 * len(calls) * 3)
		fmt.Printf("expected at least %d events from this run; stream holds %d\n", want, info.State.Msgs)
	}
}

func f(x float64) string { return fmt.Sprintf("%.2f", x) }
