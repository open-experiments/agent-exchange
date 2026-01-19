package store

import (
	"context"
	"sync"

	"github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/model"
)

type MemoryEvaluationStore struct {
	mu     sync.RWMutex
	latest map[string]model.BidEvaluation
}

func NewMemoryEvaluationStore() *MemoryEvaluationStore {
	return &MemoryEvaluationStore{latest: map[string]model.BidEvaluation{}}
}

func (s *MemoryEvaluationStore) Save(ctx context.Context, ev model.BidEvaluation) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latest[ev.WorkID] = ev
	return nil
}

func (s *MemoryEvaluationStore) GetLatest(ctx context.Context, workID string) (*model.BidEvaluation, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	ev, ok := s.latest[workID]
	if !ok {
		return nil, nil
	}
	out := ev
	return &out, nil
}
