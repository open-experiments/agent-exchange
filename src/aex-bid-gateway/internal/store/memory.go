package store

import (
	"context"
	"sync"

	"github.com/parlakisik/agent-exchange/aex-bid-gateway/internal/model"
)

type BidStore interface {
	Save(ctx context.Context, bid model.BidPacket) error
	ListByWorkID(ctx context.Context, workID string) ([]model.BidPacket, error)
}

type MemoryBidStore struct {
	mu       sync.RWMutex
	byWorkID map[string][]model.BidPacket
}

func NewMemoryBidStore() *MemoryBidStore {
	return &MemoryBidStore{
		byWorkID: make(map[string][]model.BidPacket),
	}
}

func (s *MemoryBidStore) Save(ctx context.Context, bid model.BidPacket) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byWorkID[bid.WorkID] = append(s.byWorkID[bid.WorkID], bid)
	return nil
}

func (s *MemoryBidStore) ListByWorkID(ctx context.Context, workID string) ([]model.BidPacket, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	bids := s.byWorkID[workID]
	out := make([]model.BidPacket, len(bids))
	copy(out, bids)
	return out, nil
}



