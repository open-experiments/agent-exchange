package store

import (
	"context"
	"sort"
	"sync"

	"github.com/parlakisik/agent-exchange/aex-trust-broker/internal/model"
)

type MemoryStore struct {
	mu       sync.RWMutex
	trust    map[string]model.TrustRecord
	outcomes map[string][]model.ContractOutcome
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		trust:    map[string]model.TrustRecord{},
		outcomes: map[string][]model.ContractOutcome{},
	}
}

func (s *MemoryStore) UpsertTrustRecord(ctx context.Context, rec model.TrustRecord) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trust[rec.ProviderID] = rec
	return nil
}

func (s *MemoryStore) GetTrustRecord(ctx context.Context, providerID string) (*model.TrustRecord, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.trust[providerID]
	if !ok {
		return nil, nil
	}
	out := rec
	return &out, nil
}

func (s *MemoryStore) SaveOutcome(ctx context.Context, out model.ContractOutcome) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outcomes[out.ProviderID] = append(s.outcomes[out.ProviderID], out)
	// keep most recent first
	sort.Slice(s.outcomes[out.ProviderID], func(i, j int) bool {
		return s.outcomes[out.ProviderID][i].CompletedAt.After(s.outcomes[out.ProviderID][j].CompletedAt)
	})
	return nil
}

func (s *MemoryStore) ListOutcomes(ctx context.Context, providerID string, limit int) ([]model.ContractOutcome, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	outs := s.outcomes[providerID]
	if limit > 0 && len(outs) > limit {
		outs = outs[:limit]
	}
	out := make([]model.ContractOutcome, len(outs))
	copy(out, outs)
	return out, nil
}



