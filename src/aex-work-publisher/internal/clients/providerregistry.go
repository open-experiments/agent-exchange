package clients

import (
	"context"
	"time"

	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/model"
	"github.com/parlakisik/agent-exchange/internal/httpclient"
)

type ProviderRegistryClient struct {
	baseURL string
	client  *httpclient.Client
}

func NewProviderRegistryClient(baseURL string) *ProviderRegistryClient {
	return &ProviderRegistryClient{
		baseURL: baseURL,
		client:  httpclient.NewClient("provider-registry", 10*time.Second),
	}
}

// GetSubscribedProviders returns providers subscribed to a category
func (c *ProviderRegistryClient) GetSubscribedProviders(ctx context.Context, category string) ([]model.Provider, error) {
	var result struct {
		Category  string           `json:"category"`
		Providers []model.Provider `json:"providers"`
		Count     int              `json:"count"`
	}

	err := httpclient.NewRequest("GET", c.baseURL).
		Path("/internal/providers/subscribed").
		Query("category", category).
		Context(ctx).
		ExecuteJSON(c.client, &result)

	if err != nil {
		return nil, err
	}

	return result.Providers, nil
}
