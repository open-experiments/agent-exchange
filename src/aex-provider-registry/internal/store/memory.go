package store

import (
	"context"
	"sync"

	"github.com/parlakisik/agent-exchange/aex-provider-registry/internal/model"
)

type MemoryStore struct {
	mu            sync.RWMutex
	providers     map[string]model.Provider
	subscriptions map[string]model.Subscription
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		providers:     map[string]model.Provider{},
		subscriptions: map[string]model.Subscription{},
	}
}

func (s *MemoryStore) CreateProvider(ctx context.Context, p model.Provider) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers[p.ProviderID] = p
	return nil
}

func (s *MemoryStore) GetProvider(ctx context.Context, providerID string) (*model.Provider, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.providers[providerID]
	if !ok {
		return nil, nil
	}
	out := p
	return &out, nil
}

func (s *MemoryStore) ListProviders(ctx context.Context, providerIDs []string) ([]model.Provider, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Provider, 0, len(providerIDs))
	for _, id := range providerIDs {
		if p, ok := s.providers[id]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *MemoryStore) CreateSubscription(ctx context.Context, sub model.Subscription) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscriptions[sub.SubscriptionID] = sub
	return nil
}

func (s *MemoryStore) ListSubscriptions(ctx context.Context) ([]model.Subscription, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Subscription, 0, len(s.subscriptions))
	for _, sub := range s.subscriptions {
		out = append(out, sub)
	}
	return out, nil
}

