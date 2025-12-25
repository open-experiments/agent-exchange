package store

import (
	"context"

	"github.com/parlakisik/agent-exchange/aex-identity/internal/model"
)

type Store interface {
	CreateTenant(ctx context.Context, t model.Tenant) error
	GetTenant(ctx context.Context, tenantID string) (*model.Tenant, error)
	UpdateTenant(ctx context.Context, t model.Tenant) error

	CreateAPIKey(ctx context.Context, k model.APIKey) error
	ListAPIKeys(ctx context.Context, tenantID string) ([]model.APIKey, error)
	GetAPIKey(ctx context.Context, tenantID string, keyID string) (*model.APIKey, error)
	UpdateAPIKey(ctx context.Context, k model.APIKey) error

	FindAPIKeyByHash(ctx context.Context, keyHash string) (*model.APIKey, error)
}

