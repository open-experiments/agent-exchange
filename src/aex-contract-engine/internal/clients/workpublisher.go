package clients

import (
	"context"
	"net/url"
	"time"

	"github.com/parlakisik/agent-exchange/internal/httpclient"
)

// Work holds the fields of work-publisher's GET /v1/work/{work_id} response
// (aex-work-publisher/internal/model.WorkSpec) that the contract engine uses.
type Work struct {
	ID         string `json:"work_id"`
	ConsumerID string `json:"consumer_id"`
	State      string `json:"status"`
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
// returned as *httpclient.HTTPError.
func (c *WorkPublisherClient) GetWork(ctx context.Context, workID string) (*Work, error) {
	var work Work
	err := httpclient.NewRequest("GET", c.baseURL).
		Path("/v1/work/"+url.PathEscape(workID)).
		Context(ctx).
		ExecuteJSON(c.client, &work)
	if err != nil {
		return nil, err
	}
	return &work, nil
}
