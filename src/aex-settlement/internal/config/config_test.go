package config

import (
	"strings"
	"testing"
)

func TestLoadSettlementDurations(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{name: "defaults"},
		{
			name: "consistent overrides",
			env: map[string]string{
				"SETTLEMENT_RESUME_INTERVAL":   "10s",
				"SETTLEMENT_RESUME_GRACE":      "30s",
				"SETTLEMENT_PENDING_ALERT_AGE": "5m",
				"SETTLEMENT_OP_RETENTION":      "24h",
			},
		},
		{
			name:    "invalid duration",
			env:     map[string]string{"SETTLEMENT_RESUME_GRACE": "soon"},
			wantErr: "SETTLEMENT_RESUME_GRACE",
		},
		{
			name: "retention too short for grace",
			env: map[string]string{
				"SETTLEMENT_RESUME_GRACE":      "1h",
				"SETTLEMENT_PENDING_ALERT_AGE": "1m",
				"SETTLEMENT_OP_RETENTION":      "2h",
			},
			wantErr: "SETTLEMENT_OP_RETENTION/2",
		},
		{
			name: "retention too short for alert age",
			env: map[string]string{
				"SETTLEMENT_PENDING_ALERT_AGE": "2h",
				"SETTLEMENT_OP_RETENTION":      "3h",
			},
			wantErr: "SETTLEMENT_OP_RETENTION/2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{
				"SETTLEMENT_RESUME_INTERVAL", "SETTLEMENT_RESUME_GRACE",
				"SETTLEMENT_PENDING_ALERT_AGE", "SETTLEMENT_OP_RETENTION",
			} {
				t.Setenv(key, tt.env[key])
			}
			_, err := Load()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Load() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Load() error = %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}
