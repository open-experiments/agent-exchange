package clients

import (
	"context"
	"time"

	"github.com/parlakisik/agent-exchange/internal/httpclient"
)

// WorkSpec represents a work specification from work-publisher
type WorkSpec struct {
	ID              string                 `json:"id"`
	ConsumerID      string                 `json:"consumer_id"`
	Category        string                 `json:"category"`
	Description     string                 `json:"description"`
	Constraints     map[string]interface{} `json:"constraints"`
	Budget          Budget                 `json:"budget"`
	SuccessCriteria []SuccessCriterion     `json:"success_criteria"`
	BidWindowMs     int64                  `json:"bid_window_ms"`
	State           string                 `json:"state"`
	CreatedAt       string                 `json:"created_at"`
	BidWindowEndsAt string                 `json:"bid_window_ends_at"`
}

type Budget struct {
	MaxPrice    float64 `json:"max_price"`
	MaxCPABonus float64 `json:"max_cpa_bonus,omitempty"`
	BidStrategy string  `json:"bid_strategy,omitempty"`
}

type SuccessCriterion struct {
	Metric     string      `json:"metric"`
	Threshold  interface{} `json:"threshold"`
	Comparison string      `json:"comparison,omitempty"`
	Bonus      float64     `json:"bonus,omitempty"`
}

type WorkPublisherClient struct {
	baseURL string
	client  *httpclient.Client
}

func NewWorkPublisherClient(baseURL string) *WorkPublisherClient {
	return &WorkPublisherClient{
		baseURL: baseURL,
		client:  httpclient.NewClient("work-publisher", 10*time.Second),
	}
}

// GetWork retrieves a work specification by ID
func (c *WorkPublisherClient) GetWork(ctx context.Context, workID string) (*WorkSpec, error) {
	var work WorkSpec
	err := httpclient.NewRequest("GET", c.baseURL).
		Path("/v1/work/"+workID).
		Context(ctx).
		ExecuteJSON(c.client, &work)

	if err != nil {
		return nil, err
	}

	return &work, nil
}

// CloseBidWindow notifies work-publisher to close the bid window
func (c *WorkPublisherClient) CloseBidWindow(ctx context.Context, workID string) error {
	return httpclient.NewRequest("POST", c.baseURL).
		Path("/internal/work/"+workID+"/close-bids").
		Context(ctx).
		ExecuteJSON(c.client, nil)
}
