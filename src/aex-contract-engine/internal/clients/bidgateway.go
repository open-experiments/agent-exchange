package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Bid struct {
	BidID       string    `json:"bid_id"`
	WorkID      string    `json:"work_id"`
	ProviderID  string    `json:"provider_id"`
	Price       float64   `json:"price"`
	A2AEndpoint string    `json:"a2a_endpoint"`
	ExpiresAt   time.Time `json:"expires_at"`
	ReceivedAt  time.Time `json:"received_at"`
	SLA         any       `json:"sla"`
}

type BidGatewayClient struct {
	baseURL string
	http    *http.Client
}

func NewBidGatewayClient(baseURL string) *BidGatewayClient {
	return &BidGatewayClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *BidGatewayClient) ListBids(ctx context.Context, workID string) ([]Bid, error) {
	u, err := url.Parse(c.baseURL + "/internal/v1/bids")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("work_id", workID)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bid-gateway returned %d", resp.StatusCode)
	}
	var out struct {
		Bids []Bid `json:"bids"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Bids, nil
}


