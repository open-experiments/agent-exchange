package service

import (
	"testing"
	"time"

	"github.com/parlakisik/agent-exchange/aex-trust-broker/internal/model"
)

func TestOutcomeToScore(t *testing.T) {
	tests := []struct {
		name    string
		outcome model.OutcomeType
		want    float64
	}{
		{"success", model.OutcomeSuccess, 1.0},
		{"partial success", model.OutcomeSuccessPartial, 0.7},
		{"provider failure", model.OutcomeFailureProvider, 0.0},
		{"external failure", model.OutcomeFailureExternal, 0.5},
		{"consumer failure", model.OutcomeFailureConsumer, 0.8},
		{"dispute won", model.OutcomeDisputeWon, 0.8},
		{"dispute lost", model.OutcomeDisputeLost, 0.0},
		{"expired", model.OutcomeExpired, 0.2},
		{"unknown outcome", model.OutcomeType("unknown"), 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := outcomeToScore(tt.outcome)
			if got != tt.want {
				t.Errorf("outcomeToScore(%v) = %v, want %v", tt.outcome, got, tt.want)
			}
		})
	}
}

func TestCalculateWeightedScore(t *testing.T) {
	tests := []struct {
		name      string
		outcomes  []model.ContractOutcome
		wantScore float64
	}{
		{
			name:      "no outcomes - default score",
			outcomes:  []model.ContractOutcome{},
			wantScore: 0.3,
		},
		{
			name: "all successes - perfect score",
			outcomes: []model.ContractOutcome{
				{Outcome: model.OutcomeSuccess},
				{Outcome: model.OutcomeSuccess},
				{Outcome: model.OutcomeSuccess},
			},
			wantScore: 1.0,
		},
		{
			name: "all failures - zero score",
			outcomes: []model.ContractOutcome{
				{Outcome: model.OutcomeFailureProvider},
				{Outcome: model.OutcomeFailureProvider},
				{Outcome: model.OutcomeFailureProvider},
			},
			wantScore: 0.0,
		},
		{
			name: "mixed outcomes - weighted average",
			outcomes: []model.ContractOutcome{
				{Outcome: model.OutcomeSuccess},            // 1.0 * 1.0 = 1.0
				{Outcome: model.OutcomeFailureProvider},    // 0.0 * 1.0 = 0.0
			},
			wantScore: 0.5, // (1.0 + 0.0) / 2
		},
		{
			name: "recent outcomes weighted more heavily",
			outcomes: []model.ContractOutcome{
				// First 10 have weight 1.0
				{Outcome: model.OutcomeSuccess},
				{Outcome: model.OutcomeSuccess},
				{Outcome: model.OutcomeSuccess},
				{Outcome: model.OutcomeSuccess},
				{Outcome: model.OutcomeSuccess},
				{Outcome: model.OutcomeSuccess},
				{Outcome: model.OutcomeSuccess},
				{Outcome: model.OutcomeSuccess},
				{Outcome: model.OutcomeSuccess},
				{Outcome: model.OutcomeSuccess},
				// 11-50 have weight 0.5
				{Outcome: model.OutcomeFailureProvider},
			},
			// (10 * 1.0 * 1.0 + 1 * 0.0 * 0.5) / (10 * 1.0 + 1 * 0.5) = 10.0 / 10.5 = 0.952
			wantScore: 0.952, // Approximately
		},
		{
			name: "partial success scores as 0.7",
			outcomes: []model.ContractOutcome{
				{Outcome: model.OutcomeSuccessPartial},
				{Outcome: model.OutcomeSuccessPartial},
			},
			wantScore: 0.7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateWeightedScore(tt.outcomes)

			// Allow small floating point error
			if !floatNear(got, tt.wantScore, 0.01) {
				t.Errorf("calculateWeightedScore() = %v, want %v", got, tt.wantScore)
			}

			// Verify score is always in [0, 1]
			if got < 0 || got > 1 {
				t.Errorf("Score %v is outside valid range [0, 1]", got)
			}
		})
	}
}

func TestDetermineTier(t *testing.T) {
	tests := []struct {
		name          string
		score         float64
		currentTier   model.TrustTier
		totalContracts int
		wantTier      model.TrustTier
	}{
		{
			name:          "internal tier always stays internal",
			score:         0.5,
			currentTier:   model.TrustTierInternal,
			totalContracts: 0,
			wantTier:      model.TrustTierInternal,
		},
		{
			name:          "preferred tier requires high score and volume",
			score:         0.95,
			currentTier:   model.TrustTierTrusted,
			totalContracts: 100,
			wantTier:      model.TrustTierPreferred,
		},
		{
			name:          "preferred tier not reached without enough contracts",
			score:         0.95,
			currentTier:   model.TrustTierTrusted,
			totalContracts: 99,
			wantTier:      model.TrustTierTrusted,
		},
		{
			name:          "trusted tier requires 0.7+ score and 25+ contracts",
			score:         0.75,
			currentTier:   model.TrustTierVerified,
			totalContracts: 25,
			wantTier:      model.TrustTierTrusted,
		},
		{
			name:          "verified tier requires 0.5+ score and 5+ contracts",
			score:         0.6,
			currentTier:   model.TrustTierUnverified,
			totalContracts: 5,
			wantTier:      model.TrustTierVerified,
		},
		{
			name:          "new provider starts unverified",
			score:         0.3,
			currentTier:   model.TrustTierUnverified,
			totalContracts: 0,
			wantTier:      model.TrustTierUnverified,
		},
		{
			name:          "low score returns to unverified",
			score:         0.4,
			currentTier:   model.TrustTierVerified,
			totalContracts: 10,
			wantTier:      model.TrustTierUnverified,
		},
		{
			name:          "boundary case - exactly 0.9 score and 100 contracts",
			score:         0.9,
			currentTier:   model.TrustTierTrusted,
			totalContracts: 100,
			wantTier:      model.TrustTierPreferred,
		},
		{
			name:          "boundary case - just below preferred threshold",
			score:         0.89,
			currentTier:   model.TrustTierTrusted,
			totalContracts: 100,
			wantTier:      model.TrustTierTrusted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := determineTier(tt.score, tt.currentTier, tt.totalContracts)
			if got != tt.wantTier {
				t.Errorf("determineTier(%v, %v, %v) = %v, want %v",
					tt.score, tt.currentTier, tt.totalContracts, got, tt.wantTier)
			}
		})
	}
}

func TestMonthsSince(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		registered time.Time
		now        time.Time
		want       int
	}{
		{
			name:       "0 months - just registered",
			registered: now,
			now:        now,
			want:       0,
		},
		{
			name:       "1 month",
			registered: now.AddDate(0, -1, 0),
			now:        now,
			want:       1,
		},
		{
			name:       "6 months",
			registered: now.AddDate(0, -6, 0),
			now:        now,
			want:       6,
		},
		{
			name:       "12 months",
			registered: now.AddDate(-1, 0, 0),
			now:        now,
			want:       12,
		},
		{
			name:       "future registration time",
			registered: now.AddDate(0, 1, 0),
			now:        now,
			want:       0,
		},
		{
			name:       "approximately 2.5 months",
			registered: now.AddDate(0, 0, -75), // 75 days
			now:        now,
			want:       2, // 75 days / 30 = 2.5, truncated to 2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := monthsSince(tt.registered, tt.now)
			if got != tt.want {
				t.Errorf("monthsSince(%v, %v) = %v, want %v", tt.registered, tt.now, got, tt.want)
			}
		})
	}
}

func TestClamp01(t *testing.T) {
	tests := []struct {
		name  string
		input float64
		want  float64
	}{
		{"value in range", 0.5, 0.5},
		{"value at zero", 0.0, 0.0},
		{"value at one", 1.0, 1.0},
		{"negative value clamped to 0", -0.5, 0.0},
		{"value above 1 clamped to 1", 1.5, 1.0},
		{"large negative", -100.0, 0.0},
		{"large positive", 100.0, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clamp01(tt.input)
			if got != tt.want {
				t.Errorf("clamp01(%v) = %v, want %v", tt.input, got, tt.want)
			}

			// Verify output is always in [0, 1]
			if got < 0 || got > 1 {
				t.Errorf("clamp01(%v) = %v, which is outside [0, 1]", tt.input, got)
			}
		})
	}
}

func TestTrustScoreModifiers(t *testing.T) {
	tests := []struct {
		name              string
		identityVerified  bool
		endpointVerified  bool
		tenureMonths      int
		wantModifier      float64
	}{
		{
			name:              "no verification, new provider",
			identityVerified:  false,
			endpointVerified:  false,
			tenureMonths:      0,
			wantModifier:      0.0,
		},
		{
			name:              "identity verified only",
			identityVerified:  true,
			endpointVerified:  false,
			tenureMonths:      0,
			wantModifier:      0.05,
		},
		{
			name:              "endpoint verified only",
			identityVerified:  false,
			endpointVerified:  true,
			tenureMonths:      0,
			wantModifier:      0.05,
		},
		{
			name:              "both verified",
			identityVerified:  true,
			endpointVerified:  true,
			tenureMonths:      0,
			wantModifier:      0.10,
		},
		{
			name:              "both verified + 1 month tenure",
			identityVerified:  true,
			endpointVerified:  true,
			tenureMonths:      1,
			wantModifier:      0.12, // 0.05 + 0.05 + 0.02
		},
		{
			name:              "both verified + 5 months tenure (max)",
			identityVerified:  true,
			endpointVerified:  true,
			tenureMonths:      5,
			wantModifier:      0.20, // 0.05 + 0.05 + 0.10
		},
		{
			name:              "both verified + 10 months tenure (capped at 5)",
			identityVerified:  true,
			endpointVerified:  true,
			tenureMonths:      10,
			wantModifier:      0.20, // 0.05 + 0.05 + 0.10 (capped)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modifier := 0.0
			if tt.identityVerified {
				modifier += 0.05
			}
			if tt.endpointVerified {
				modifier += 0.05
			}

			tenureMonths := tt.tenureMonths
			if tenureMonths > 5 {
				tenureMonths = 5
			}
			modifier += float64(tenureMonths) * 0.02

			if !floatNear(modifier, tt.wantModifier, 0.001) {
				t.Errorf("Modifier = %v, want %v", modifier, tt.wantModifier)
			}
		})
	}
}

func TestTrustScoreIntegration(t *testing.T) {
	// Test complete trust score calculation with base + modifiers
	tests := []struct {
		name              string
		outcomes          []model.ContractOutcome
		identityVerified  bool
		endpointVerified  bool
		tenureMonths      int
		wantMinScore      float64
		wantMaxScore      float64
	}{
		{
			name: "perfect provider",
			outcomes: []model.ContractOutcome{
				{Outcome: model.OutcomeSuccess},
				{Outcome: model.OutcomeSuccess},
				{Outcome: model.OutcomeSuccess},
				{Outcome: model.OutcomeSuccess},
				{Outcome: model.OutcomeSuccess},
			},
			identityVerified: true,
			endpointVerified: true,
			tenureMonths:     5,
			wantMinScore:     1.0,
			wantMaxScore:     1.0, // Clamped at 1.0
		},
		{
			name: "new provider with no contracts",
			outcomes:         []model.ContractOutcome{},
			identityVerified: false,
			endpointVerified: false,
			tenureMonths:     0,
			wantMinScore:     0.3,
			wantMaxScore:     0.3,
		},
		{
			name: "verified provider with mixed results",
			outcomes: []model.ContractOutcome{
				{Outcome: model.OutcomeSuccess},
				{Outcome: model.OutcomeFailureProvider},
			},
			identityVerified: true,
			endpointVerified: true,
			tenureMonths:     3,
			wantMinScore:     0.66, // 0.5 base + 0.16 modifiers
			wantMaxScore:     0.67,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := calculateWeightedScore(tt.outcomes)
			modifier := 0.0
			if tt.identityVerified {
				modifier += 0.05
			}
			if tt.endpointVerified {
				modifier += 0.05
			}
			tenureMonths := tt.tenureMonths
			if tenureMonths > 5 {
				tenureMonths = 5
			}
			modifier += float64(tenureMonths) * 0.02

			finalScore := clamp01(base + modifier)

			if finalScore < tt.wantMinScore || finalScore > tt.wantMaxScore {
				t.Errorf("Final score = %v, want between %v and %v", finalScore, tt.wantMinScore, tt.wantMaxScore)
			}

			// Verify score is always in [0, 1]
			if finalScore < 0 || finalScore > 1 {
				t.Errorf("Final score %v is outside valid range [0, 1]", finalScore)
			}
		})
	}
}

// Helper function to compare floats with tolerance
func floatNear(a, b, tolerance float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}
