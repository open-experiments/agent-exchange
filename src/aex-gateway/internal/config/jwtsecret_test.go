package config

import (
	"strings"
	"testing"
)

func TestValidateJWTSecret(t *testing.T) {
	const strong = "k7Qm2vX9pL4sR8tW1yB6nD3fH5jZ0cGa"
	cases := []struct {
		name        string
		environment string
		secret      string
		wantErr     bool
		wantWarning bool
	}{
		{"strong in production", "production", strong, false, false},
		{"strong in development", "development", strong, false, false},
		{"dev compose secret warns in development", "development", "dev-jwt-secret-do-not-use-in-production", false, true},
		{"dev compose secret in production", "production", "dev-jwt-secret-do-not-use-in-production", true, false},
		{"template value in production", "production", "<generate: openssl rand -base64 48>", true, false},
		{"template value in development warns", "development", "<generate: openssl rand -base64 48>", false, true},
		{"empty in production", "production", "", true, false},
		{"blank in staging", "staging", "   ", true, false},
		{"short in production", "production", "k7Qm2vX9pL4sR8tW1yB6nD3fH5jZ0cG", true, false},
		{"REPLACE_ME in production", "production", "REPLACE_ME", true, false},
		{"placeholder case-insensitive and trimmed", "production", "  Replace_Me ", true, false},
		{"CHANGE_ME", "production", "CHANGE_ME", true, false},
		{"changeme", "production", "changeme", true, false},
		{"changeit", "production", "changeit", true, false},
		{"secret", "production", "secret", true, false},
		{"your-secret", "production", "your-secret", true, false},
		{"your-jwt-secret", "production", "your-jwt-secret", true, false},
		{"your-jwt-signing-key", "production", "your-jwt-signing-key", true, false},
		{"dev-secret", "production", "dev-secret", true, false},
		{"test", "production", "test", true, false},
		{"password", "production", "password", true, false},
		{"long value containing replace_me", "production", "REPLACE_ME_WITH_A_LONG_RANDOM_SECRET_VALUE", true, false},
		{"long value containing change_me", "production", "please-change_me-before-deploying-this-service", true, false},
		{"unknown environment is not development", "", "REPLACE_ME", true, false},
		{"empty in development warns", "development", "", false, true},
		{"placeholder in development warns", "development", "REPLACE_ME", false, true},
		{"short in development warns", "development", "abc123", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			warning, err := ValidateJWTSecret(c.environment, c.secret)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if (warning != "") != c.wantWarning {
				t.Fatalf("warning = %q, wantWarning %v", warning, c.wantWarning)
			}
			if trimmed := strings.TrimSpace(c.secret); trimmed != "" {
				if err != nil && strings.Contains(err.Error(), trimmed) {
					t.Errorf("error leaks the secret: %v", err)
				}
				if strings.Contains(warning, trimmed) {
					t.Errorf("warning leaks the secret: %q", warning)
				}
			}
		})
	}
}
