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

	CreateSubscription(ctx context.Context, s model.Subscription) error
	ListSubscriptions(ctx context.Context) ([]model.Subscription, error)
}


