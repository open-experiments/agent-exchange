package toolgate

// Scores are the five auditability dimensions of Nian et al. (2026),
// "Auditable agents" (arXiv:2604.05485), computed over a set of records with
// the metrics fixed by the harness study that motivated this package:
//
//   - ActionRecoverability: fraction of records holding the tool, the argument
//     values and the outcome, so the action can be reconstructed.
//   - LifecycleCoverage: phases observable per record out of request, decision,
//     execution and outcome, averaged.
//   - PolicyCheckability: of the policy's rules plus the scope check, the
//     fraction a reader could decide from the record alone.
//   - ResponsibilityAttribution: chain elements present out of principal,
//     agent, session and approval status, averaged.
//   - EvidenceIntegrity: ordinal, 0 plain log, 1 append-only, 2 hash-chained,
//     averaged.
type Scores struct {
	Records                   int     `json:"records"`
	ActionRecoverability      float64 `json:"action_recoverability"`
	LifecycleCoverage         float64 `json:"lifecycle_coverage"`
	PolicyCheckability        float64 `json:"policy_checkability"`
	ResponsibilityAttribution float64 `json:"responsibility_attribution"`
	EvidenceIntegrity         float64 `json:"evidence_integrity_ordinal"`
	FieldsRecoverableOf9      float64 `json:"schema_fields_recoverable_of_9"`
}

// Score computes the five dimensions over records for a policy with
// ruleCount argument-value rules (the scope check is added to that count).
func Score(records []Artifact, ruleCount int) Scores {
	var s Scores
	n := len(records)
	if n == 0 {
		return s
	}
	policies := float64(ruleCount + 1)
	for _, r := range records {
		f := r.FieldsPresent()
		if f["tool_name"] && f["argument_values"] && f["outcome"] {
			s.ActionRecoverability++
		}
		phases := 0.0
		if f["tool_name"] {
			phases++ // request
		}
		if f["decision"] {
			phases++ // decision
		}
		phases++ // execution: a record exists for the call
		if f["outcome"] {
			phases++ // outcome
		}
		s.LifecycleCoverage += phases / 4.0
		checkable := 0.0
		if r.Scope != "" {
			checkable++ // the scope check is decidable when the scope consulted is in the record
		}
		if f["argument_values"] {
			checkable += float64(ruleCount) // every value rule is decidable from recorded values
		}
		s.PolicyCheckability += checkable / policies
		chain := 0.0
		if r.Principal != "" {
			chain++
		}
		if r.AgentID != "" {
			chain++
		}
		if r.SessionID != "" {
			chain++
		}
		if r.Approval != "" {
			chain++
		}
		s.ResponsibilityAttribution += chain / 4.0
		switch r.Integrity {
		case IntegrityAppendOnly:
			s.EvidenceIntegrity += 1
		case IntegrityHashChain:
			s.EvidenceIntegrity += 2
		}
		present := 0.0
		for _, v := range f {
			if v {
				present++
			}
		}
		s.FieldsRecoverableOf9 += present
	}
	fn := float64(n)
	s.Records = n
	s.ActionRecoverability = round2(s.ActionRecoverability / fn)
	s.LifecycleCoverage = round2(s.LifecycleCoverage / fn)
	s.PolicyCheckability = round2(s.PolicyCheckability / fn)
	s.ResponsibilityAttribution = round2(s.ResponsibilityAttribution / fn)
	s.EvidenceIntegrity = round2(s.EvidenceIntegrity / fn)
	s.FieldsRecoverableOf9 = round2(s.FieldsRecoverableOf9 / fn)
	return s
}

func round2(x float64) float64 {
	return float64(int(x*100+0.5)) / 100
}
