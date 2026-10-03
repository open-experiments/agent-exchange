package clients

import (
	"context"
	"net/url"
	"time"

	"github.com/parlakisik/agent-exchange/internal/httpclient"
)

// WorkSpec mirrors the work specification returned by work-publisher's
// GET /v1/work/{work_id} (aex-work-publisher/internal/model.WorkSpec).
type WorkSpec struct {
	ID              string             `json:"work_id"`
	ConsumerID      string             `json:"consumer_id"`
	Category        string             `json:"category"`
	Description     string             `json:"description"`
	Constraints     WorkConstraints    `json:"constraints"`
	Budget          Budget             `json:"budget"`
	SuccessCriteria []SuccessCriterion `json:"success_criteria"`
	BidWindowMs     int64              `json:"bid_window_ms"`
	State           string             `json:"status"`
	CreatedAt       time.Time          `json:"created_at"`
	BidWindowEndsAt time.Time          `json:"bid_window_ends_at"`
}

type WorkConstraints struct {
	MaxLatencyMs   *int64   `json:"max_latency_ms,omitempty"`
	RequiredFields []string `json:"required_fields,omitempty"`
	MinTrustTier   *string  `json:"min_trust_tier,omitempty"`
	InternalOnly   bool     `json:"internal_only"`
	Regions        []string `json:"regions,omitempty"`
}

type Budget struct {
	MaxPrice    float64  `json:"max_price"`
	BidStrategy string   `json:"bid_strategy"`
	MaxCPABonus *float64 `json:"max_cpa_bonus,omitempty"`
}

type SuccessCriterion struct {
	Metric     string   `json:"metric"`
	Type       string   `json:"type"`
	Comparison *string  `json:"comparison,omitempty"`
	Threshold  any      `json:"threshold"`
	Bonus      *float64 `json:"bonus,omitempty"`
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

// GetWork retrieves a work specification by ID. A non-2xx response is
// returned as *httpclient.HTTPError so callers can inspect the status code.
func (c *WorkPublisherClient) GetWork(ctx context.Context, workID string) (*WorkSpec, error) {
	var work WorkSpec
	err := httpclient.NewRequest("GET", c.baseURL).
		Path("/v1/work/"+url.PathEscape(workID)).
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
		Path("/internal/work/"+url.PathEscape(workID)+"/close-bids").
		Context(ctx).
		ExecuteJSON(c.client, nil)
}
