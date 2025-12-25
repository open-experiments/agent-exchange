package store

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/model"
)

// MemoryStore is an in-memory implementation of WorkStore for development
type MemoryStore struct {
	mu    sync.RWMutex
	works map[string]model.WorkSpec
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		works: make(map[string]model.WorkSpec),
	}
}

func (s *MemoryStore) SaveWork(ctx context.Context, work model.WorkSpec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.works[work.ID] = work
	return nil
}

func (s *MemoryStore) GetWork(ctx context.Context, workID string) (model.WorkSpec, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	work, ok := s.works[workID]
	if !ok {
		return model.WorkSpec{}, errors.New("work not found")
	}
	return work, nil
}

func (s *MemoryStore) UpdateWork(ctx context.Context, work model.WorkSpec) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.works[work.ID]; !ok {
		return errors.New("work not found")
	}

	s.works[work.ID] = work
	return nil
}

func (s *MemoryStore) ListWork(ctx context.Context, consumerID string, limit int) ([]model.WorkSpec, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var works []model.WorkSpec
	for _, work := range s.works {
		if work.ConsumerID == consumerID {
			works = append(works, work)
		}
	}

	// Sort by created_at descending
	sort.Slice(works, func(i, j int) bool {
		return works[i].CreatedAt.After(works[j].CreatedAt)
	})

	if limit > 0 && len(works) > limit {
		works = works[:limit]
	}

	return works, nil
}

func (s *MemoryStore) Close() error {
	return nil
}
