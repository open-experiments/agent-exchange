package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/parlakisik/agent-exchange/aex-identity/internal/model"
	"github.com/parlakisik/agent-exchange/aex-identity/internal/store"
)

type Service struct {
	store store.Store
}

func New(st store.Store) *Service {
	return &Service{store: st}
}

func (s *Service) HandleCreateTenant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req model.CreateTenantRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "bad request")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		respondError(w, http.StatusBadRequest, "NAME_REQUIRED", "name is required")
		return
	}
	if req.Type == "" {
		req.Type = model.TenantTypeBoth
	}
	now := time.Now().UTC()
	tenantID := generateID("tenant_")
	externalID := generateID("ext_")

	t := model.Tenant{
		ID:           tenantID,
		ExternalID:   externalID,
		Name:         strings.TrimSpace(req.Name),
		Type:         req.Type,
		Status:       model.TenantStatusActive,
		ContactEmail: strings.TrimSpace(req.ContactEmail),
		BillingEmail: strings.TrimSpace(req.BillingEmail),
		Quotas:       defaultQuotas(),
		Metadata:     req.Metadata,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if t.Metadata == nil {
		t.Metadata = map[string]any{}
	}
	if err := s.store.CreateTenant(ctx, t); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}

	keyPlain, keyHash, prefix := generateAPIKey("aexk_")
	k := model.APIKey{
		ID:        generateID("key_"),
		TenantID:  tenantID,
		Name:      "default",
		KeyHash:   keyHash,
		Prefix:    prefix,
		Scopes:    []string{"*"},
		Status:    model.APIKeyStatusActive,
		CreatedAt: now,
	}
	if err := s.store.CreateAPIKey(ctx, k); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}

	var resp model.CreateTenantResponse
	resp.ID = t.ID
	resp.ExternalID = t.ExternalID
	resp.Name = t.Name
	resp.Type = t.Type
	resp.Status = t.Status
	resp.CreatedAt = t.CreatedAt
	resp.Quotas = t.Quotas
	resp.APIKey.ID = k.ID
	resp.APIKey.Key = keyPlain
	resp.APIKey.Prefix = k.Prefix

	writeJSON(w, http.StatusCreated, resp)
}

func (s *Service) HandleGetTenant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := pathParam(r.URL.Path, "/v1/tenants/", "")
	if tenantID == "" {
		respondError(w, http.StatusBadRequest, "TENANT_ID_REQUIRED", "tenant_id is required")
		return
	}
	t, err := s.store.GetTenant(ctx, tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	if t == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Service) HandleSuspendTenant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := pathParam(r.URL.Path, "/v1/tenants/", "/suspend")
	if tenantID == "" {
		respondError(w, http.StatusBadRequest, "TENANT_ID_REQUIRED", "tenant_id is required")
		return
	}
	t, err := s.store.GetTenant(ctx, tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	if t == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	now := time.Now().UTC()
	t.Status = model.TenantStatusSuspended
	t.SuspendedAt = &now
	t.UpdatedAt = now
	if err := s.store.UpdateTenant(ctx, *t); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Service) HandleActivateTenant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := pathParam(r.URL.Path, "/v1/tenants/", "/activate")
	if tenantID == "" {
		respondError(w, http.StatusBadRequest, "TENANT_ID_REQUIRED", "tenant_id is required")
		return
	}
	t, err := s.store.GetTenant(ctx, tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	if t == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	now := time.Now().UTC()
	t.Status = model.TenantStatusActive
	t.SuspendedAt = nil
	t.SuspensionReason = nil
	t.UpdatedAt = now
	if err := s.store.UpdateTenant(ctx, *t); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Service) HandleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := pathParam(r.URL.Path, "/v1/tenants/", "/api-keys")
	if tenantID == "" {
		respondError(w, http.StatusBadRequest, "TENANT_ID_REQUIRED", "tenant_id is required")
		return
	}
	t, err := s.store.GetTenant(ctx, tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	if t == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}

	var req model.CreateAPIKeyRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "bad request")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "key"
	}
	now := time.Now().UTC()

	plain, hash, prefix := generateAPIKey("aexk_")
	k := model.APIKey{
		ID:        generateID("key_"),
		TenantID:  tenantID,
		Name:      name,
		KeyHash:   hash,
		Prefix:    prefix,
		Scopes:    normalizeScopes(req.Scopes),
		Status:    model.APIKeyStatusActive,
		CreatedAt: now,
		ExpiresAt: req.ExpiresAt,
	}
	if err := s.store.CreateAPIKey(ctx, k); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}

	resp := model.CreateAPIKeyResponse{
		ID:         k.ID,
		Name:       k.Name,
		Key:        plain,
		Prefix:     k.Prefix,
		Scopes:     k.Scopes,
		CreatedAt:  k.CreatedAt,
		ExpiresAt:  k.ExpiresAt,
		LastUsedAt: nil,
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Service) HandleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := pathParam(r.URL.Path, "/v1/tenants/", "/api-keys")
	if tenantID == "" {
		respondError(w, http.StatusBadRequest, "TENANT_ID_REQUIRED", "tenant_id is required")
		return
	}
	keys, err := s.store.ListAPIKeys(ctx, tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}

	// Apply pagination
	limit, offset := 100, 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, _ := strconv.Atoi(v); n > 0 && n <= 100 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, _ := strconv.Atoi(v); n >= 0 {
			offset = n
		}
	}
	total := len(keys)
	if offset >= len(keys) {
		keys = nil
	} else {
		end := offset + limit
		if end > len(keys) {
			end = len(keys)
		}
		keys = keys[offset:end]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"api_keys": keys,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
	})
}

func (s *Service) HandleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := pathParam(r.URL.Path, "/v1/tenants/", "/api-keys/")
	if tenantID == "" {
		respondError(w, http.StatusBadRequest, "TENANT_ID_REQUIRED", "tenant_id is required")
		return
	}
	keyID := lastPathSegment(r.URL.Path)
	if keyID == "" {
		respondError(w, http.StatusBadRequest, "KEY_ID_REQUIRED", "key_id is required")
		return
	}
	k, err := s.store.GetAPIKey(ctx, tenantID, keyID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	if k == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	now := time.Now().UTC()
	k.Status = model.APIKeyStatusRevoked
	k.RevokedAt = &now
	if err := s.store.UpdateAPIKey(ctx, *k); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revoked": true, "id": k.ID})
}

func (s *Service) HandleValidateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req model.ValidateAPIKeyRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "bad request")
		return
	}
	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		respondError(w, http.StatusBadRequest, "API_KEY_REQUIRED", "api_key is required")
		return
	}
	keyHash := hashAPIKey(apiKey)
	k, err := s.store.FindAPIKeyByHash(ctx, keyHash)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	if k == nil {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	if k.Status != model.APIKeyStatusActive {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	if k.ExpiresAt != nil && time.Now().UTC().After(*k.ExpiresAt) {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	t, err := s.store.GetTenant(ctx, k.TenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	if t == nil || t.Status != model.TenantStatusActive {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	now := time.Now().UTC()
	k.LastUsedAt = &now
	_ = s.store.UpdateAPIKey(ctx, *k)

	resp := model.ValidateAPIKeyResponse{
		Valid:        true,
		TenantID:     t.ID,
		TenantStatus: t.Status,
		Scopes:       k.Scopes,
		Quotas:       t.Quotas,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Service) HandleGetQuotas(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := pathParam(r.URL.Path, "/internal/v1/tenants/", "/quotas")
	if tenantID == "" {
		respondError(w, http.StatusBadRequest, "TENANT_ID_REQUIRED", "tenant_id is required")
		return
	}
	t, err := s.store.GetTenant(ctx, tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	if t == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	writeJSON(w, http.StatusOK, t.Quotas)
}

func defaultQuotas() model.Quotas {
	return model.Quotas{
		RequestsPerMinute:   60,
		RequestsPerDay:      10000,
		MaxAgents:           10,
		MaxConcurrentTasks:  5,
		MaxTaskPayloadBytes: 256 * 1024,
	}
}

func normalizeScopes(in []string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		return []string{"*"}
	}
	return out
}

func decodeJSON(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	defer func() { _ = r.Body.Close() }()
	return json.Unmarshal(body, v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func respondError(w http.ResponseWriter, statusCode int, code string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":      code,
			"message":   message,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	})
}

func generateID(prefix string) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:8])
}

func generateAPIKey(prefix string) (plain string, hash string, keyPrefix string) {
	var b [32]byte
	_, _ = rand.Read(b[:])
	raw := prefix + hex.EncodeToString(b[:])
	return raw, hashAPIKey(raw), raw[:min(10, len(raw))]
}

func hashAPIKey(k string) string {
	sum := sha256.Sum256([]byte(k))
	return hex.EncodeToString(sum[:])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func pathParam(path string, prefix string, suffix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(path, prefix)
	if suffix != "" {
		if !strings.HasSuffix(rest, suffix) {
			return ""
		}
		rest = strings.TrimSuffix(rest, suffix)
	}
	rest = strings.Trim(rest, "/")
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	return strings.TrimSpace(rest)
}

func lastPathSegment(path string) string {
	path = strings.Trim(path, "/")
	if path == "" {
		return ""
	}
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		return path[i+1:]
	}
	return path
}
