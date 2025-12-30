package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
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

// InMemoryAPIKeyValidator is a simple in-memory validator for development
type InMemoryAPIKeyValidator struct {
	mu   sync.RWMutex
	keys map[string]*APIKeyInfo
}

func NewInMemoryAPIKeyValidator() *InMemoryAPIKeyValidator {
	return &InMemoryAPIKeyValidator{
		keys: map[string]*APIKeyInfo{
			"dev-api-key": {
				TenantID: "tenant_dev",
				Scopes:   []string{"*"},
				Status:   "ACTIVE",
			},
			"test-api-key": {
				TenantID: "tenant_test",
				Scopes:   []string{"*"},
				Status:   "ACTIVE",
			},
		},
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

// HTTPAPIKeyValidator validates API keys via HTTP call to identity service
type HTTPAPIKeyValidator struct {
	identityURL string
	client      *http.Client
	cache       sync.Map
	cacheTTL    time.Duration
}

type cachedKey struct {
	info      *APIKeyInfo
	expiresAt time.Time
}

func NewHTTPAPIKeyValidator(identityURL string) *HTTPAPIKeyValidator {
	return &HTTPAPIKeyValidator{
		identityURL: identityURL,
		client:      &http.Client{Timeout: 5 * time.Second},
		cacheTTL:    5 * time.Minute,
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

	// Call identity service
	reqBody, _ := json.Marshal(map[string]string{"api_key": apiKey})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.identityURL+"/internal/v1/apikeys/validate", strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	var result struct {
		Valid    bool     `json:"valid"`
		TenantID string   `json:"tenant_id"`
		Scopes   []string `json:"scopes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if !result.Valid {
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

func Auth(validator APIKeyValidator) func(http.Handler) http.Handler {
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
				ctx := context.WithValue(r.Context(), TenantIDKey, info.TenantID)
				ctx = context.WithValue(ctx, RolesKey, info.Scopes)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// 2. Check Bearer token
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				token := strings.TrimPrefix(auth, "Bearer ")
				// For Phase A, we accept any non-empty bearer token with a development tenant
				// In production, this would validate against Firebase Auth
				if token != "" {
					ctx := context.WithValue(r.Context(), TenantIDKey, "tenant_bearer")
					ctx = context.WithValue(ctx, RolesKey, []string{"*"})
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				respondError(w, http.StatusUnauthorized, "invalid_token", "Invalid bearer token", r)
				return
			}

			respondError(w, http.StatusUnauthorized, "authentication_required", "Authentication required", r)
		})
	}
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
		},
	})
}

