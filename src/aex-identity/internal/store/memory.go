package store

import (
	"context"
	"sync"

	"github.com/parlakisik/agent-exchange/aex-identity/internal/model"
)

type MemoryStore struct {
	mu      sync.RWMutex
	tenants map[string]model.Tenant
	apiKeys map[string]map[string]model.APIKey // tenantID -> keyID -> key
	byHash  map[string]model.APIKey            // keyHash -> key
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		tenants: map[string]model.Tenant{},
		apiKeys: map[string]map[string]model.APIKey{},
		byHash:  map[string]model.APIKey{},
	}
}

func (s *MemoryStore) CreateTenant(ctx context.Context, t model.Tenant) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tenants[t.ID] = t
	return nil
}

func (s *MemoryStore) GetTenant(ctx context.Context, tenantID string) (*model.Tenant, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tenants[tenantID]
	if !ok {
		return nil, nil
	}
	out := t
	return &out, nil
}

func (s *MemoryStore) UpdateTenant(ctx context.Context, t model.Tenant) error {
	return s.CreateTenant(ctx, t)
}

func (s *MemoryStore) CreateAPIKey(ctx context.Context, k model.APIKey) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.apiKeys[k.TenantID]; !ok {
		s.apiKeys[k.TenantID] = map[string]model.APIKey{}
	}
	s.apiKeys[k.TenantID][k.ID] = k
	s.byHash[k.KeyHash] = k
	return nil
}

func (s *MemoryStore) ListAPIKeys(ctx context.Context, tenantID string) ([]model.APIKey, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.apiKeys[tenantID]
	out := make([]model.APIKey, 0, len(m))
	for _, k := range m {
		out = append(out, k)
	}
	return out, nil
}

func (s *MemoryStore) GetAPIKey(ctx context.Context, tenantID string, keyID string) (*model.APIKey, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	if m2, ok := s.apiKeys[tenantID]; ok {
		if k, ok := m2[keyID]; ok {
			out := k
			return &out, nil
		}
	}
	return nil, nil
}

func (s *MemoryStore) UpdateAPIKey(ctx context.Context, k model.APIKey) error {
	return s.CreateAPIKey(ctx, k)
}

func (s *MemoryStore) FindAPIKeyByHash(ctx context.Context, keyHash string) (*model.APIKey, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	k, ok := s.byHash[keyHash]
	if !ok {
		return nil, nil
	}
	out := k
	return &out, nil
}
