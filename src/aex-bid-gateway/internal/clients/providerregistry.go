package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ProviderRegistryClient validates provider API keys against the provider registry
type ProviderRegistryClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewProviderRegistryClient creates a new provider registry client
func NewProviderRegistryClient(baseURL string) *ProviderRegistryClient {
	return &ProviderRegistryClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// ValidateAPIKeyResponse is the response from the provider registry
type ValidateAPIKeyResponse struct {
	ProviderID string `json:"provider_id"`
	Valid      bool   `json:"valid"`
	Status     string `json:"status"`
}

// ValidateAPIKey validates an API key against the provider registry
func (c *ProviderRegistryClient) ValidateAPIKey(ctx context.Context, apiKey string) (string, error) {
	url := fmt.Sprintf("%s/internal/v1/providers/validate-key?api_key=%s", c.baseURL, apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("invalid API key: status %d", resp.StatusCode)
	}

	var result ValidateAPIKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if !result.Valid {
		return "", fmt.Errorf("invalid API key")
	}

	return result.ProviderID, nil
}

