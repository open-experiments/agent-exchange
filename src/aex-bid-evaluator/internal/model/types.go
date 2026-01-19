package model

import "time"

type WorkConstraints struct {
	MaxLatencyMs *int64 `json:"max_latency_ms,omitempty"`
}

type WorkBudget struct {
	MaxPrice    float64 `json:"max_price"`
	BidStrategy string  `json:"bid_strategy"` // lowest_price|best_quality|balanced
}

type WorkSpec struct {
	WorkID      string          `json:"work_id"`
	Budget      WorkBudget      `json:"budget"`
	Constraints WorkConstraints `json:"constraints"`
	Description string          `json:"description,omitempty"`
}

type SLACommitment struct {
	MaxLatencyMs int64   `json:"max_latency_ms"`
	Availability float64 `json:"availability"`
}

type MVPSample struct {
	SampleInput   string `json:"sample_input"`
	SampleOutput  string `json:"sample_output"`
	SampleLatency int64  `json:"sample_latency_ms"`
}

type BidPacket struct {
	BidID      string `json:"bid_id"`
	WorkID     string `json:"work_id"`
	ProviderID string `json:"provider_id"`

	Price      float64 `json:"price"`
	Confidence float64 `json:"confidence"`

	MVPSample *MVPSample    `json:"mvp_sample,omitempty"`
	SLA       SLACommitment `json:"sla"`

	A2AEndpoint string    `json:"a2a_endpoint"`
	ExpiresAt   time.Time `json:"expires_at"`
	ReceivedAt  time.Time `json:"received_at"`
}

type DisqualifiedBid struct {
	BidID  string `json:"bid_id"`
	Reason string `json:"reason"`
}

type BidScore struct {
	Price      float64 `json:"price"`
	Trust      float64 `json:"trust"`
	Confidence float64 `json:"confidence"`
	MVPSample  float64 `json:"mvp_sample"`
	SLA        float64 `json:"sla"`
}

type RankedBid struct {
	Rank       int      `json:"rank"`
	BidID      string   `json:"bid_id"`
	ProviderID string   `json:"provider_id"`
	TotalScore float64  `json:"total_score"`
	Scores     BidScore `json:"scores"`
}

type BidEvaluation struct {
	EvaluationID     string            `json:"evaluation_id"`
	WorkID           string            `json:"work_id"`
	TotalBids        int               `json:"total_bids"`
	ValidBids        int               `json:"valid_bids"`
	RankedBids       []RankedBid       `json:"ranked_bids"`
	DisqualifiedBids []DisqualifiedBid `json:"disqualified_bids"`
	EvaluatedAt      time.Time         `json:"evaluated_at"`
}

type EvaluateRequest struct {
	WorkID string `json:"work_id"`

	// Optional override if you don't have a work-publisher yet.
	Budget      *WorkBudget      `json:"budget,omitempty"`
	Constraints *WorkConstraints `json:"constraints,omitempty"`
	Description *string          `json:"description,omitempty"`
}
