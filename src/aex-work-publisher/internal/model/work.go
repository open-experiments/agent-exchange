package model

import "time"

// WorkState represents the current state of work
type WorkState string

const (
	WorkStateOpen       WorkState = "OPEN"
	WorkStateEvaluating WorkState = "EVALUATING"
	WorkStateAwarded    WorkState = "AWARDED"
	WorkStateExecuting  WorkState = "EXECUTING"
	WorkStateCompleted  WorkState = "COMPLETED"
	WorkStateFailed     WorkState = "FAILED"
	WorkStateCancelled  WorkState = "CANCELLED"
)

// Budget represents the pricing constraints for work
type Budget struct {
	MaxPrice    float64  `json:"max_price" firestore:"max_price"`
	BidStrategy string   `json:"bid_strategy" firestore:"bid_strategy"` // "lowest_price" | "best_quality" | "balanced"
	MaxCPABonus *float64 `json:"max_cpa_bonus,omitempty" firestore:"max_cpa_bonus,omitempty"`
}

// WorkConstraints defines execution constraints
type WorkConstraints struct {
	MaxLatencyMs   *int64   `json:"max_latency_ms,omitempty" firestore:"max_latency_ms,omitempty"`
	RequiredFields []string `json:"required_fields,omitempty" firestore:"required_fields,omitempty"`
	MinTrustTier   *string  `json:"min_trust_tier,omitempty" firestore:"min_trust_tier,omitempty"`
	InternalOnly   bool     `json:"internal_only" firestore:"internal_only"`
	Regions        []string `json:"regions,omitempty" firestore:"regions,omitempty"`
}

// SuccessCriterion defines a success metric
type SuccessCriterion struct {
	Metric     string   `json:"metric" firestore:"metric"`
	Type       string   `json:"type" firestore:"type"` // "boolean" | "numeric"
	Comparison *string  `json:"comparison,omitempty" firestore:"comparison,omitempty"`
	Threshold  any      `json:"threshold" firestore:"threshold"`
	Bonus      *float64 `json:"bonus,omitempty" firestore:"bonus,omitempty"`
}

// WorkSpec represents a complete work specification
type WorkSpec struct {
	ID              string             `json:"work_id" firestore:"work_id"`
	ConsumerID      string             `json:"consumer_id" firestore:"consumer_id"`
	Category        string             `json:"category" firestore:"category"`
	Description     string             `json:"description" firestore:"description"`
	Constraints     WorkConstraints    `json:"constraints" firestore:"constraints"`
	Budget          Budget             `json:"budget" firestore:"budget"`
	SuccessCriteria []SuccessCriterion `json:"success_criteria" firestore:"success_criteria"`
	BidWindowMs     int64              `json:"bid_window_ms" firestore:"bid_window_ms"`
	Payload         map[string]any     `json:"payload" firestore:"payload"`

	State             WorkState `json:"status" firestore:"status"`
	ProvidersNotified int       `json:"providers_notified" firestore:"providers_notified"`
	BidsReceived      int       `json:"bids_received" firestore:"bids_received"`
	ContractID        *string   `json:"contract_id,omitempty" firestore:"contract_id,omitempty"`

	CreatedAt       time.Time  `json:"created_at" firestore:"created_at"`
	BidWindowEndsAt time.Time  `json:"bid_window_ends_at" firestore:"bid_window_ends_at"`
	AwardedAt       *time.Time `json:"awarded_at,omitempty" firestore:"awarded_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty" firestore:"completed_at,omitempty"`
}

// WorkSubmission is the request to submit work
type WorkSubmission struct {
	Category        string             `json:"category"`
	Description     string             `json:"description"`
	Constraints     WorkConstraints    `json:"constraints"`
	Budget          Budget             `json:"budget"`
	SuccessCriteria []SuccessCriterion `json:"success_criteria"`
	BidWindowMs     int64              `json:"bid_window_ms"`
	Payload         map[string]any     `json:"payload"`
}

// WorkResponse is returned after submitting work
type WorkResponse struct {
	WorkID            string    `json:"work_id"`
	Status            string    `json:"status"`
	BidWindowEndsAt   time.Time `json:"bid_window_ends_at"`
	ProvidersNotified int       `json:"providers_notified"`
	CreatedAt         time.Time `json:"created_at"`
}

// Provider represents a service provider
type Provider struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Capabilities  []string `json:"capabilities"`
	BidWebhook    string   `json:"bid_webhook,omitempty"`
	WebhookSecret string   `json:"webhook_secret,omitempty"`
}
