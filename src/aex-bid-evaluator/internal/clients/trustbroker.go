package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type TrustBrokerClient struct {
	baseURL string
	http    *http.Client
}

func NewTrustBrokerClient(baseURL string) *TrustBrokerClient {
	return &TrustBrokerClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *TrustBrokerClient) GetScore(ctx context.Context, providerID string) (float64, error) {
	if c.baseURL == "" {
		return 0.5, nil
	}
	u, err := url.Parse(c.baseURL + "/v1/providers/" + providerID + "/trust")
	if err != nil {
		return 0.5, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0.5, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0.5, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0.5, fmt.Errorf("trust-broker returned %d", resp.StatusCode)
	}
	var out struct {
		TrustScore float64 `json:"trust_score"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0.5, err
	}
	return out.TrustScore, nil
}

