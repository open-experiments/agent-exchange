package clients

import (
	"context"
	"time"

	"github.com/parlakisik/agent-exchange/internal/httpclient"
)

// ContractCompletedEvent represents the payload sent to settlement service
type ContractCompletedEvent struct {
	ContractID  string                 `json:"contract_id"`
	WorkID      string                 `json:"work_id"`
	AgentID     string                 `json:"agent_id"`
	ConsumerID  string                 `json:"consumer_id"`
	ProviderID  string                 `json:"provider_id"`
	Domain      string                 `json:"domain"`
	StartedAt   time.Time              `json:"started_at"`
	CompletedAt time.Time              `json:"completed_at"`
	Success     bool                   `json:"success"`
	AgreedPrice string                 `json:"agreed_price"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type SettlementClient struct {
	baseURL string
	client  *httpclient.Client
}

func NewSettlementClient(baseURL string) *SettlementClient {
	return &SettlementClient{
		baseURL: baseURL,
		client:  httpclient.NewClient("settlement", 10*time.Second),
	}
}

// ProcessContractCompletion sends contract completion to settlement service
func (c *SettlementClient) ProcessContractCompletion(ctx context.Context, event ContractCompletedEvent) error {
	var response struct {
		Status string `json:"status"`
	}

	err := httpclient.NewRequest("POST", c.baseURL).
		Path("/internal/settlement/complete").
		JSON(event).
		Context(ctx).
		ExecuteJSON(c.client, &response)

	return err
}
