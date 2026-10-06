package config

import (
	"errors"
	"strings"
)

// MinJWTSecretLength is the shortest JWT_SECRET accepted outside development.
// HS256 keys shorter than the 32-byte hash output are brute-forceable.
const MinJWTSecretLength = 32

// placeholderJWTSecrets are values copied from examples and manifests; a
// gateway signing with one accepts tokens anyone can forge.
var placeholderJWTSecrets = map[string]bool{
	"replace_me":           true,
	"change_me":            true,
	"changeme":             true,
	"changeit":             true,
	"secret":               true,
	"your-secret":          true,
	"your-jwt-secret":      true,
	"your-jwt-signing-key": true,
	"dev-secret":           true,
	"test":                 true,
	"password":             true,
}

// placeholderJWTSecretMarkers flag a placeholder anywhere in the value, such
// as "REPLACE_ME_WITH_A_REAL_KEY" or the local compose stack's
// "dev-jwt-secret-do-not-use-in-production".
var placeholderJWTSecretMarkers = []string{"replace_me", "change_me", "do-not-use-in-production"}

// ValidateJWTSecret checks the JWT_SECRET the gateway verifies tokens with.
// An empty, short or placeholder secret is an error outside development; in
// development it is returned as a warning so local stacks keep starting.
// Neither the error nor the warning includes the secret.
func ValidateJWTSecret(environment, secret string) (warning string, err error) {
	problem := jwtSecretProblem(secret)
	if problem == "" {
		return "", nil
	}
	if environment == "development" {
		return problem + "; acceptable only in development", nil
	}
	return "", errors.New(problem + " (ENVIRONMENT=" + environment + "); set JWT_SECRET to a random value of at least 32 bytes, e.g. from `openssl rand -base64 48`")
}

// jwtSecretProblem describes why secret is unfit to sign tokens, or returns
// "" when it is usable.
func jwtSecretProblem(secret string) string {
	normalized := strings.ToLower(strings.TrimSpace(secret))
	switch {
	case normalized == "":
		return "JWT_SECRET is empty, so JWT authentication is disabled"
	case isPlaceholderJWTSecret(normalized):
		return "JWT_SECRET is a known placeholder value, so anyone can forge tokens"
	case len(normalized) < MinJWTSecretLength:
		return "JWT_SECRET is shorter than 32 bytes"
	}
	return ""
}

func isPlaceholderJWTSecret(normalized string) bool {
	if placeholderJWTSecrets[normalized] {
		return true
	}
	// Template values such as "<generate: openssl rand -base64 48>".
	if strings.HasPrefix(normalized, "<") && strings.HasSuffix(normalized, ">") {
		return true
	}
	for _, marker := range placeholderJWTSecretMarkers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
