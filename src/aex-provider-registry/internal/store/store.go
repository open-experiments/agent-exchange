package store

import (
	"context"

	"github.com/parlakisik/agent-exchange/aex-provider-registry/internal/model"
)

type Store interface {
	CreateProvider(ctx context.Context, p model.Provider) error
	GetProvider(ctx context.Context, providerID string) (*model.Provider, error)
	GetProviderByAPIKeyHash(ctx context.Context, apiKeyHash string) (*model.Provider, error)
	ListProviders(ctx context.Context, providerIDs []string) ([]model.Provider, error)
	ListAllProviders(ctx context.Context) ([]model.Provider, error)
	UpdateProvider(ctx context.Context, p model.Provider) error

	CreateSubscription(ctx context.Context, s model.Subscription) error
	ListSubscriptions(ctx context.Context) ([]model.Subscription, error)

	// A2A support
	SaveAgentCard(ctx context.Context, providerID string, card model.AgentCard, a2aEndpoint string) error
	GetProviderWithA2A(ctx context.Context, providerID string) (*model.ProviderWithA2A, error)
	IndexSkills(ctx context.Context, providerID string, skills []model.SkillIndex) error
	SearchBySkillTags(ctx context.Context, tags []string, minTrust float64, limit int) ([]model.ProviderSearchResult, error)
}
