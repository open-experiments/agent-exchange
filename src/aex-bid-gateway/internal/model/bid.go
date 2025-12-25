package model

import "time"

type MVPSample struct {
	SampleInput   string `json:"sample_input"`
	SampleOutput  string `json:"sample_output"`
	SampleLatency int64  `json:"sample_latency_ms"`
}

type SLACommitment struct {
	MaxLatencyMs int64   `json:"max_latency_ms"`
	Availability float64 `json:"availability"`
}

type BidPacket struct {
	BidID      string `json:"bid_id"`
	WorkID     string `json:"work_id"`
	ProviderID string `json:"provider_id"`

	Price          float64            `json:"price"`
	PriceBreakdown map[string]float64 `json:"price_breakdown,omitempty"`

	Confidence       float64 `json:"confidence"`
	Approach         string  `json:"approach"`
	EstimatedLatency int64   `json:"estimated_latency_ms"`

	MVPSample *MVPSample `json:"mvp_sample,omitempty"`

	SLA SLACommitment `json:"sla"`

	A2AEndpoint string    `json:"a2a_endpoint"`
	ExpiresAt   time.Time `json:"expires_at"`
	ReceivedAt  time.Time `json:"received_at"`
}

type SubmitBidRequest struct {
	WorkID           string             `json:"work_id"`
	Price            float64            `json:"price"`
	PriceBreakdown   map[string]float64 `json:"price_breakdown,omitempty"`
	Confidence       float64            `json:"confidence"`
	Approach         string             `json:"approach"`
	EstimatedLatency int64              `json:"estimated_latency_ms"`
	MVPSample        *MVPSample         `json:"mvp_sample,omitempty"`
	SLA              SLACommitment      `json:"sla"`
	A2AEndpoint      string             `json:"a2a_endpoint"`
	ExpiresAt        time.Time          `json:"expires_at"`
}

type SubmitBidResponse struct {
	BidID      string    `json:"bid_id"`
	WorkID     string    `json:"work_id"`
	Status     string    `json:"status"`
	ReceivedAt time.Time `json:"received_at"`
}

