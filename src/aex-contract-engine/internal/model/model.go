package model

import "time"

type ContractStatus string

const (
	ContractStatusAwarded   ContractStatus = "AWARDED"
	ContractStatusExecuting ContractStatus = "EXECUTING"
	ContractStatusCompleted ContractStatus = "COMPLETED"
	ContractStatusFailed    ContractStatus = "FAILED"
	ContractStatusExpired   ContractStatus = "EXPIRED"
	ContractStatusDisputed  ContractStatus = "DISPUTED"
)

type SLACommitment struct {
	MaxLatencyMs int64   `json:"max_latency_ms"`
	Availability float64 `json:"availability"`
}

type ExecutionUpdate struct {
	Status    string    `json:"status"`
	Percent   *int      `json:"percent,omitempty"`
	Message   *string   `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type OutcomeReport struct {
	Success        bool           `json:"success"`
	ResultSummary  string         `json:"result_summary"`
	Metrics        map[string]any `json:"metrics"`
	ResultLocation *string        `json:"result_location,omitempty"`
	ReportedAt     time.Time      `json:"reported_at"`
}

type Contract struct {
	ContractID string `json:"contract_id" bson:"contract_id"`
	WorkID     string `json:"work_id" bson:"work_id"`
	ConsumerID string `json:"consumer_id" bson:"consumer_id"`
	ProviderID string `json:"provider_id" bson:"provider_id"`
	BidID      string `json:"bid_id" bson:"bid_id"`

	AgreedPrice      float64       `json:"agreed_price" bson:"agreed_price"`
	SLA              SLACommitment `json:"sla" bson:"sla"`
	ProviderEndpoint string        `json:"provider_endpoint" bson:"provider_endpoint"`

	ExecutionToken string `json:"execution_token" bson:"execution_token"`
	ConsumerToken  string `json:"consumer_token" bson:"consumer_token"`

	Status    ContractStatus `json:"status" bson:"status"`
	ExpiresAt time.Time      `json:"expires_at" bson:"expires_at"`

	AwardedAt   time.Time  `json:"awarded_at" bson:"awarded_at"`
	StartedAt   *time.Time `json:"started_at,omitempty" bson:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty" bson:"completed_at,omitempty"`
	FailedAt    *time.Time `json:"failed_at,omitempty" bson:"failed_at,omitempty"`

	ExecutionUpdates []ExecutionUpdate `json:"execution_updates,omitempty" bson:"execution_updates,omitempty"`
	Outcome          *OutcomeReport    `json:"outcome,omitempty" bson:"outcome,omitempty"`
	FailureReason    *string           `json:"failure_reason,omitempty" bson:"failure_reason,omitempty"`
}

type AwardRequest struct {
	BidID     string `json:"bid_id"`
	AutoAward bool   `json:"auto_award"`
}

type AwardResponse struct {
	ContractID       string         `json:"contract_id"`
	WorkID           string         `json:"work_id"`
	ProviderID       string         `json:"provider_id"`
	AgreedPrice      float64        `json:"agreed_price"`
	Status           ContractStatus `json:"status"`
	ProviderEndpoint string         `json:"provider_endpoint"`
	ExecutionToken   string         `json:"execution_token"`
	ExpiresAt        time.Time      `json:"expires_at"`
	AwardedAt        time.Time      `json:"awarded_at"`
}

type ProgressRequest struct {
	Status  string  `json:"status"`
	Percent *int    `json:"percent,omitempty"`
	Message *string `json:"message,omitempty"`
}

type CompleteRequest struct {
	Success        bool           `json:"success"`
	ResultSummary  string         `json:"result_summary"`
	Metrics        map[string]any `json:"metrics"`
	ResultLocation *string        `json:"result_location,omitempty"`
}

type FailRequest struct {
	Reason     string `json:"reason"`
	Message    string `json:"message"`
	ReportedBy string `json:"reported_by"` // "provider" or "consumer"
}

