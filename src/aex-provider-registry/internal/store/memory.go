package store

import (
	"context"
	"sync"
	"time"

	"github.com/parlakisik/agent-exchange/aex-provider-registry/internal/model"
)

type MemoryStore struct {
	mu            sync.RWMutex
	providers     map[string]model.Provider
	subscriptions map[string]model.Subscription
	agentCards    map[string]model.AgentCard
	a2aEndpoints  map[string]string
	skillIndex    map[string][]model.SkillIndex // tag -> skills
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		providers:     map[string]model.Provider{},
		subscriptions: map[string]model.Subscription{},
		agentCards:    map[string]model.AgentCard{},
		a2aEndpoints:  map[string]string{},
		skillIndex:    map[string][]model.SkillIndex{},
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

func (s *MemoryStore) GetProviderByAPIKeyHash(ctx context.Context, apiKeyHash string) (*model.Provider, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.providers {
		if p.APIKeyHash == apiKeyHash {
			out := p
			return &out, nil
		}
	}
	return nil, nil
}

func (s *MemoryStore) GetProviderByName(ctx context.Context, name string) (*model.Provider, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.providers {
		if p.Name == name {
			out := p
			return &out, nil
		}
	}
	return nil, nil
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

func (s *MemoryStore) ListAllProviders(ctx context.Context) ([]model.Provider, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Provider, 0, len(s.providers))
	for _, p := range s.providers {
		out = append(out, p)
	}
	return out, nil
}

func (s *MemoryStore) UpdateProvider(ctx context.Context, p model.Provider) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.providers[p.ProviderID]; !ok {
		return nil
	}
	s.providers[p.ProviderID] = p
	return nil
}

func (s *MemoryStore) SaveAgentCard(ctx context.Context, providerID string, card model.AgentCard, a2aEndpoint string) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentCards[providerID] = card
	s.a2aEndpoints[providerID] = a2aEndpoint
	return nil
}

func (s *MemoryStore) GetProviderWithA2A(ctx context.Context, providerID string) (*model.ProviderWithA2A, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.providers[providerID]
	if !ok {
		return nil, nil
	}
	result := &model.ProviderWithA2A{
		Provider:    p,
		A2AEndpoint: s.a2aEndpoints[providerID],
	}
	if card, ok := s.agentCards[providerID]; ok {
		result.AgentCard = &card
	}
	return result, nil
}

func (s *MemoryStore) IndexSkills(ctx context.Context, providerID string, skills []model.SkillIndex) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove old skills for this provider
	for tag, indexedSkills := range s.skillIndex {
		filtered := make([]model.SkillIndex, 0)
		for _, skill := range indexedSkills {
			if skill.ProviderID != providerID {
				filtered = append(filtered, skill)
			}
		}
		s.skillIndex[tag] = filtered
	}

	// Add new skills
	now := time.Now()
	for _, skill := range skills {
		skill.CreatedAt = now
		// Index by each tag
		for _, tag := range skill.Tags {
			s.skillIndex[tag] = append(s.skillIndex[tag], skill)
		}
		// Also index by skill ID
		s.skillIndex[skill.SkillID] = append(s.skillIndex[skill.SkillID], skill)
	}
	return nil
}

func (s *MemoryStore) SearchBySkillTags(ctx context.Context, tags []string, minTrust float64, limit int) ([]model.ProviderSearchResult, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	// Find providers matching tags
	providerMatches := make(map[string]map[string]bool) // providerID -> matched tags
	providerSkills := make(map[string][]string)         // providerID -> skill IDs

	for _, tag := range tags {
		if skills, ok := s.skillIndex[tag]; ok {
			for _, skill := range skills {
				if providerMatches[skill.ProviderID] == nil {
					providerMatches[skill.ProviderID] = make(map[string]bool)
				}
				providerMatches[skill.ProviderID][tag] = true
				// Track skills
				found := false
				for _, sid := range providerSkills[skill.ProviderID] {
					if sid == skill.SkillID {
						found = true
						break
					}
				}
				if !found {
					providerSkills[skill.ProviderID] = append(providerSkills[skill.ProviderID], skill.SkillID)
				}
			}
		}
	}

	// Build results
	results := make([]model.ProviderSearchResult, 0)
	for providerID, matchedTags := range providerMatches {
		p, ok := s.providers[providerID]
		if !ok || p.Status != model.ProviderStatusActive {
			continue
		}
		if p.TrustScore < minTrust {
			continue
		}

		tags := make([]string, 0, len(matchedTags))
		for tag := range matchedTags {
			tags = append(tags, tag)
		}

		results = append(results, model.ProviderSearchResult{
			ProviderID:  p.ProviderID,
			Name:        p.Name,
			Description: p.Description,
			Endpoint:    p.Endpoint,
			A2AEndpoint: s.a2aEndpoints[providerID],
			TrustScore:  p.TrustScore,
			TrustTier:   string(p.TrustTier),
			Skills:      providerSkills[providerID],
			MatchedTags: tags,
		})

		if len(results) >= limit {
			break
		}
	}

	return results, nil
}
