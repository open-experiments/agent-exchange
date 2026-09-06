package toolgate

import (
	"context"
	"testing"
	"time"
)

// The expected scores are the run-1 results of the harness study for the same
// twelve calls and six policies: configuration B (scope gate) and C (value gate).
func TestScoresMatchHarnessRun1(t *testing.T) {
	p := loadPolicy(t)
	ruleCount := len(p.Rules)
	ctx := context.Background()
	exec := func(context.Context, Call) (string, error) { return "ok", nil }

	full, _ := New(p, WithClock(fixedClock()))
	for _, tc := range loadCalls(t) {
		_, _ = full.Authorize(ctx, asCall(tc), exec)
	}
	c := Score(full.Records(), ruleCount)
	if c.ActionRecoverability != 1 || c.LifecycleCoverage != 1 || c.PolicyCheckability != 1 || c.ResponsibilityAttribution != 1 || c.EvidenceIntegrity != 2 || c.FieldsRecoverableOf9 != 9 {
		t.Fatalf("value gate should score 1/1/1/1/2 and 9 fields, got %+v", c)
	}

	scope, _ := New(p, WithMode(ModeScope), WithClock(fixedClock()))
	for _, tc := range loadCalls(t) {
		_, _ = scope.Authorize(ctx, asCall(tc), exec)
	}
	// The gate's scope-mode record is richer than the harness's mock B (it
	// carries session, approval status and outcome), so it scores above the
	// mock B on lifecycle, attribution and recoverability of everything but
	// the argument values. What it cannot score is the checkability of any
	// value rule, which is the point.
	b := Score(scope.Records(), ruleCount)
	if b.ActionRecoverability != 0 {
		t.Fatalf("scope mode records no values, so no action is recoverable; got %v", b.ActionRecoverability)
	}
	if b.PolicyCheckability != round2(1.0/float64(ruleCount+1)) {
		t.Fatalf("scope mode should make only the scope check decidable, got %v", b.PolicyCheckability)
	}
	if b.EvidenceIntegrity != 2 {
		t.Fatalf("scope mode still hash-chains its records, got %v", b.EvidenceIntegrity)
	}
}

func TestPlainLogScoresZeroOnEverythingButRequest(t *testing.T) {
	// A framework-style log line: timestamp and tool name, nothing else.
	var recs []Artifact
	for i := 0; i < 12; i++ {
		recs = append(recs, Artifact{Timestamp: time.Now(), Tool: "send_payment", Decision: DecisionNone, Integrity: IntegrityPlainLog})
	}
	s := Score(recs, 5)
	if s.ActionRecoverability != 0 || s.LifecycleCoverage != 0.5 || s.PolicyCheckability != 0 || s.ResponsibilityAttribution != 0 || s.EvidenceIntegrity != 0 || s.FieldsRecoverableOf9 != 2 {
		t.Fatalf("plain log should score 0/0.5/0/0/0 and 2 fields, got %+v", s)
	}
}
