package clients

import (
	"context"
	"time"

	"github.com/parlakisik/agent-exchange/internal/httpclient"
)

// BidPacket represents a bid from bid-gateway
type BidPacket struct {
	ID         string                 `json:"id"`
	WorkID     string                 `json:"work_id"`
	ProviderID string                 `json:"provider_id"`
	AgentID    string                 `json:"agent_id"`
	Price      float64                `json:"price"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	ReceivedAt string                 `json:"received_at"`
}

type BidGatewayClient struct {
	baseURL string
	client  *httpclient.Client
}

func NewBidGatewayClient(baseURL string) *BidGatewayClient {
	return &BidGatewayClient{
		baseURL: baseURL,
		client:  httpclient.NewClient("bid-gateway", 10*time.Second),
	}
}

// GetBidsForWork retrieves all bids for a work specification
func (c *BidGatewayClient) GetBidsForWork(ctx context.Context, workID string) ([]BidPacket, error) {
	var response struct {
		Bids  []BidPacket `json:"bids"`
		Count int         `json:"count"`
	}

	err := httpclient.NewRequest("GET", c.baseURL).
		Path("/internal/bids/work/"+workID).
		Context(ctx).
		ExecuteJSON(c.client, &response)

	if err != nil {
		return nil, err
	}

	return response.Bids, nil
}
