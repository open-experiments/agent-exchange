package store

import (
	"context"
	"sync"

	"github.com/parlakisik/agent-exchange/aex-contract-engine/internal/model"
)

type MemoryContractStore struct {
	mu   sync.RWMutex
	byID map[string]model.Contract
}

func NewMemoryContractStore() *MemoryContractStore {
	return &MemoryContractStore{byID: map[string]model.Contract{}}
}

func (s *MemoryContractStore) Save(ctx context.Context, c model.Contract) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[c.ContractID] = c
	return nil
}

func (s *MemoryContractStore) Get(ctx context.Context, contractID string) (*model.Contract, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.byID[contractID]
	if !ok {
		return nil, nil
	}
	out := c
	return &out, nil
}

func (s *MemoryContractStore) Update(ctx context.Context, c model.Contract) error {
	return s.Save(ctx, c)
}

