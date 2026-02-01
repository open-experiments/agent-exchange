package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// Context key type for storing agent ID
type contextKey string

// AgentIDKey is the context key for the authenticated agent ID
const AgentIDKey contextKey = "agent_id"

// AgentAuthenticator provides methods for authenticating agents
type AgentAuthenticator interface {
	GetAgentIDByTokenHash(tokenHash string) (string, error)
}

// AuthMiddleware creates a middleware that validates Bearer tokens
func AuthMiddleware(auth AgentAuthenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract Bearer token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			if !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, `{"error":"invalid authorization header format"}`, http.StatusUnauthorized)
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == "" {
				http.Error(w, `{"error":"empty token"}`, http.StatusUnauthorized)
				return
			}

			// Hash the token and lookup agent
			tokenHash := SHA256Hex(token)
			agentID, err := auth.GetAgentIDByTokenHash(tokenHash)
			if err != nil {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}

			// Add agent_id to request context
			ctx := context.WithValue(r.Context(), AgentIDKey, agentID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetAuthenticatedAgentID extracts the agent_id from the request context
func GetAuthenticatedAgentID(r *http.Request) string {
	if id, ok := r.Context().Value(AgentIDKey).(string); ok {
		return id
	}
	return ""
}

// SHA256Hex returns the SHA256 hash of a string as a hex-encoded string
func SHA256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// OptionalAuthMiddleware creates a middleware that validates Bearer tokens if present
// but allows requests without auth to pass through (for backwards compatibility)
func OptionalAuthMiddleware(auth AgentAuthenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract Bearer token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
				token := strings.TrimPrefix(authHeader, "Bearer ")
				if token != "" {
					// Hash the token and lookup agent
					tokenHash := SHA256Hex(token)
					agentID, err := auth.GetAgentIDByTokenHash(tokenHash)
					if err == nil {
						// Add agent_id to request context
						ctx := context.WithValue(r.Context(), AgentIDKey, agentID)
						r = r.WithContext(ctx)
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
