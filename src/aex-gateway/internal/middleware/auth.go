package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const TenantIDKey contextKey = "tenant_id"
const RolesKey contextKey = "roles"

// APIKeyValidator validates API keys against the identity service
type APIKeyValidator interface {
	Validate(ctx context.Context, apiKey string) (*APIKeyInfo, error)
}

type APIKeyInfo struct {
	TenantID string   `json:"tenant_id"`
	Scopes   []string `json:"scopes"`
	Status   string   `json:"status"`
}

// InMemoryAPIKeyValidator is a simple in-memory validator.
// Keys must be added explicitly via AddKey; no keys are hardcoded.
type InMemoryAPIKeyValidator struct {
	mu   sync.RWMutex
	keys map[string]*APIKeyInfo
}

func NewInMemoryAPIKeyValidator() *InMemoryAPIKeyValidator {
	return &InMemoryAPIKeyValidator{
		keys: make(map[string]*APIKeyInfo),
	}
}

func (v *InMemoryAPIKeyValidator) Validate(ctx context.Context, apiKey string) (*APIKeyInfo, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if info, ok := v.keys[apiKey]; ok {
		return info, nil
	}
	return nil, nil
}

func (v *InMemoryAPIKeyValidator) AddKey(apiKey string, info *APIKeyInfo) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.keys[apiKey] = info
}

// HTTPAPIKeyValidator validates API keys via HTTP call to identity service.
// Includes a simple circuit breaker to prevent stampeding the identity service
// when it's down. After 5 consecutive failures, requests are rejected for 10s.
type HTTPAPIKeyValidator struct {
	identityURL string
	client      *http.Client
	cache       sync.Map
	cacheTTL    time.Duration

	// Circuit breaker state
	cbMu                sync.Mutex
	consecutiveFailures int
	cbOpenUntil         time.Time
	cbFailureThreshold  int
	cbOpenDuration      time.Duration
}

type cachedKey struct {
	info      *APIKeyInfo
	expiresAt time.Time
}

func NewHTTPAPIKeyValidator(identityURL string) *HTTPAPIKeyValidator {
	return &HTTPAPIKeyValidator{
		identityURL:        identityURL,
		client:             &http.Client{Timeout: 5 * time.Second},
		cacheTTL:           5 * time.Minute,
		cbFailureThreshold: 5,
		cbOpenDuration:     10 * time.Second,
	}
}

func (v *HTTPAPIKeyValidator) Validate(ctx context.Context, apiKey string) (*APIKeyInfo, error) {
	// Check cache first
	if cached, ok := v.cache.Load(apiKey); ok {
		c := cached.(*cachedKey)
		if time.Now().Before(c.expiresAt) {
			return c.info, nil
		}
		v.cache.Delete(apiKey)
	}

	// Check circuit breaker - fast-fail if identity service is down
	if v.isBreakerOpen() {
		return nil, fmt.Errorf("identity service circuit breaker open")
	}

	// Call identity service
	reqBody, _ := json.Marshal(map[string]string{"api_key": apiKey})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.identityURL+"/internal/v1/apikeys/validate", strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		v.recordFailure()
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 500 {
		v.recordFailure()
		return nil, nil
	}

	// Success or 4xx - reset breaker
	v.recordSuccess()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	// The identity service answers 401 for an invalid key and 200 with the
	// tenant for a valid one; "valid" is optional in the body so either shape
	// of the contract (docs/AUTHENTICATION.md) is accepted.
	var result struct {
		Valid    *bool    `json:"valid"`
		TenantID string   `json:"tenant_id"`
		Scopes   []string `json:"scopes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if (result.Valid != nil && !*result.Valid) || result.TenantID == "" {
		return nil, nil
	}

	info := &APIKeyInfo{
		TenantID: result.TenantID,
		Scopes:   result.Scopes,
		Status:   "ACTIVE",
	}

	// Cache the result
	v.cache.Store(apiKey, &cachedKey{
		info:      info,
		expiresAt: time.Now().Add(v.cacheTTL),
	})

	return info, nil
}

func (v *HTTPAPIKeyValidator) isBreakerOpen() bool {
	v.cbMu.Lock()
	defer v.cbMu.Unlock()
	if v.consecutiveFailures >= v.cbFailureThreshold {
		if time.Now().Before(v.cbOpenUntil) {
			return true
		}
		// Half-open: allow one request through
		v.consecutiveFailures = v.cbFailureThreshold - 1
	}
	return false
}

func (v *HTTPAPIKeyValidator) recordFailure() {
	v.cbMu.Lock()
	defer v.cbMu.Unlock()
	v.consecutiveFailures++
	if v.consecutiveFailures >= v.cbFailureThreshold {
		v.cbOpenUntil = time.Now().Add(v.cbOpenDuration)
	}
}

func (v *HTTPAPIKeyValidator) recordSuccess() {
	v.cbMu.Lock()
	defer v.cbMu.Unlock()
	v.consecutiveFailures = 0
}

// jwtConfig holds the configuration for JWT validation.
type jwtConfig struct {
	secret []byte
	issuer string
}

// Auth returns an HTTP middleware that validates requests via API key or JWT bearer token.
// jwtSecret is the HMAC-SHA256 signing key used to verify bearer tokens.
func Auth(validator APIKeyValidator, jwtSecret string) func(http.Handler) http.Handler {
	cfg := &jwtConfig{
		secret: []byte(jwtSecret),
		issuer: "aex-identity",
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. Check API Key header
			if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
				info, err := validator.Validate(r.Context(), apiKey)
				if err != nil {
					respondError(w, http.StatusInternalServerError, "auth_error", "Authentication service unavailable", r)
					return
				}
				if info == nil {
					respondError(w, http.StatusUnauthorized, "invalid_api_key", "Invalid API key", r)
					return
				}
				if ri := GetRequestInfo(r.Context()); ri != nil {
					ri.TenantID = info.TenantID
				}
				ctx := context.WithValue(r.Context(), TenantIDKey, info.TenantID)
				ctx = context.WithValue(ctx, RolesKey, info.Scopes)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// 2. Check Bearer token (JWT)
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				tokenStr := strings.TrimPrefix(auth, "Bearer ")
				if tokenStr == "" {
					respondError(w, http.StatusUnauthorized, "invalid_token", "Bearer token is empty", r)
					return
				}

				claims, err := validateJWT(tokenStr, cfg)
				if err != nil {
					respondError(w, http.StatusUnauthorized, "invalid_token", fmt.Sprintf("Invalid bearer token: %v", err), r)
					return
				}

				if ri := GetRequestInfo(r.Context()); ri != nil {
					ri.TenantID = claims.TenantID
				}
				ctx := context.WithValue(r.Context(), TenantIDKey, claims.TenantID)
				ctx = context.WithValue(ctx, RolesKey, claims.Scopes)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			respondError(w, http.StatusUnauthorized, "authentication_required", "Authentication required", r)
		})
	}
}

// AEXClaims represents the custom JWT claims used by Agent Exchange.
type AEXClaims struct {
	jwt.RegisteredClaims
	TenantID string   `json:"tenant_id"`
	Scopes   []string `json:"scopes"`
}

// validateJWT parses and validates a JWT token string using HMAC-SHA256.
// It checks the signature, expiration, and issuer.
func validateJWT(tokenStr string, cfg *jwtConfig) (*AEXClaims, error) {
	claims := &AEXClaims{}

	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (any, error) {
		// Ensure the signing method is HMAC (HS256/HS384/HS512).
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return cfg.secret, nil
	},
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithExpirationRequired(),
		jwt.WithIssuer(cfg.issuer),
	)
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, fmt.Errorf("token is not valid")
	}

	if claims.TenantID == "" {
		return nil, fmt.Errorf("token missing required tenant_id claim")
	}

	return claims, nil
}

func GetTenantID(ctx context.Context) string {
	if id, ok := ctx.Value(TenantIDKey).(string); ok {
		return id
	}
	return ""
}

func GetRoles(ctx context.Context) []string {
	if roles, ok := ctx.Value(RolesKey).([]string); ok {
		return roles
	}
	return nil
}

func respondError(w http.ResponseWriter, status int, code, message string, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":       code,
			"message":    message,
			"request_id": GetRequestID(r.Context()),
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
		},
	})
}
