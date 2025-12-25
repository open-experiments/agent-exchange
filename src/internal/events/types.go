package events

import "time"

// Event envelope for all events
type Envelope struct {
	EventID        string         `json:"event_id"`
	EventType      string         `json:"event_type"`
	SchemaVersion  string         `json:"schema_version"`
	IdempotencyKey string         `json:"idempotency_key"`
	Timestamp      time.Time      `json:"timestamp"`
	Source         string         `json:"source"`
	TenantID       string         `json:"tenant_id,omitempty"`
	Data           map[string]any `json:"data"`
}

// Work Events
type WorkSubmittedData struct {
	WorkID            string         `json:"work_id"`
	Domain            string         `json:"domain"`
	Requirements      map[string]any `json:"requirements"`
	Budget            Budget         `json:"budget"`
	SuccessCriteria   []Criterion    `json:"success_criteria"`
	BidWindowMs       int64          `json:"bid_window_ms"`
	ProvidersNotified int            `json:"providers_notified,omitempty"`
	BidWindowEndsAt   string         `json:"bid_window_ends_at,omitempty"`
}

type Budget struct {
	MaxPrice    float64 `json:"max_price"`
	MaxCPABonus float64 `json:"max_cpa_bonus,omitempty"`
	BidStrategy string  `json:"bid_strategy,omitempty"`
}

type Criterion struct {
	Metric     string  `json:"metric"`
	Threshold  any     `json:"threshold"`
	Comparison string  `json:"comparison,omitempty"`
	Bonus      float64 `json:"bonus,omitempty"`
}

type WorkBidWindowClosedData struct {
	WorkID   string    `json:"work_id"`
	BidCount int       `json:"bid_count"`
	ClosedAt time.Time `json:"closed_at"`
}

type WorkCancelledData struct {
	WorkID      string    `json:"work_id"`
	ConsumerID  string    `json:"consumer_id"`
	Reason      string    `json:"reason"`
	CancelledAt time.Time `json:"cancelled_at"`
}

// Bid Events
type BidSubmittedData struct {
	BidID       string  `json:"bid_id"`
	WorkID      string  `json:"work_id"`
	ProviderID  string  `json:"provider_id"`
	AgentID     string  `json:"agent_id"`
	Price       float64 `json:"price"`
	Confidence  float64 `json:"confidence"`
	A2AEndpoint string  `json:"a2a_endpoint"`
}

type BidsEvaluatedData struct {
	WorkID       string      `json:"work_id"`
	EvaluationID string      `json:"evaluation_id"`
	RankedBids   []RankedBid `json:"ranked_bids"`
	WinningBidID string      `json:"winning_bid_id"`
}

type RankedBid struct {
	BidID      string  `json:"bid_id"`
	ProviderID string  `json:"provider_id"`
	AgentID    string  `json:"agent_id"`
	Rank       int     `json:"rank"`
	Score      float64 `json:"score"`
	Price      float64 `json:"price"`
}

// Contract Events
type ContractAwardedData struct {
	ContractID  string   `json:"contract_id"`
	WorkID      string   `json:"work_id"`
	BidID       string   `json:"bid_id"`
	ProviderID  string   `json:"provider_id"`
	AgentID     string   `json:"agent_id"`
	ConsumerID  string   `json:"consumer_id"`
	AgreedPrice float64  `json:"agreed_price"`
	CPATerms    CPATerms `json:"cpa_terms,omitempty"`
	A2AEndpoint string   `json:"a2a_endpoint"`
}

type CPATerms struct {
	SuccessCriteria []Criterion `json:"success_criteria"`
	MaxBonus        float64     `json:"max_bonus"`
	MaxPenaltyRate  float64     `json:"max_penalty_rate"`
}

type ContractCompletedData struct {
	ContractID  string         `json:"contract_id"`
	WorkID      string         `json:"work_id"`
	AgentID     string         `json:"agent_id"`
	ProviderID  string         `json:"provider_id"`
	ConsumerID  string         `json:"consumer_id"`
	Domain      string         `json:"domain"`
	StartedAt   time.Time      `json:"started_at"`
	CompletedAt time.Time      `json:"completed_at"`
	DurationMs  int64          `json:"duration_ms"`
	Billing     Billing        `json:"billing"`
	Metrics     map[string]any `json:"metrics"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type Billing struct {
	Cost float64 `json:"cost"`
}

type ContractFailedData struct {
	ContractID    string `json:"contract_id"`
	WorkID        string `json:"work_id"`
	AgentID       string `json:"agent_id"`
	ProviderID    string `json:"provider_id"`
	ConsumerID    string `json:"consumer_id"`
	FailureReason string `json:"failure_reason"`
	ErrorCode     string `json:"error_code"`
	ErrorMessage  string `json:"error_message"`
}

// Trust Events
type TrustScoreUpdatedData struct {
	ProviderID    string  `json:"provider_id"`
	AgentID       string  `json:"agent_id"`
	PreviousScore float64 `json:"previous_score"`
	NewScore      float64 `json:"new_score"`
	PreviousTier  string  `json:"previous_tier"`
	NewTier       string  `json:"new_tier"`
	Reason        string  `json:"reason"`
}

type TrustTierChangedData struct {
	ProviderID   string    `json:"provider_id"`
	PreviousTier string    `json:"previous_tier"`
	NewTier      string    `json:"new_tier"`
	EffectiveAt  time.Time `json:"effective_at"`
}

// Identity Events
type TenantCreatedData struct {
	TenantID   string `json:"tenant_id"`
	ExternalID string `json:"external_id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
}

type TenantSuspendedData struct {
	TenantID string `json:"tenant_id"`
	Reason   string `json:"reason"`
}

type APIKeyRevokedData struct {
	TenantID string `json:"tenant_id"`
	KeyID    string `json:"key_id"`
	Prefix   string `json:"prefix"`
}

// Provider Events
type ProviderRegisteredData struct {
	ProviderID   string    `json:"provider_id"`
	Name         string    `json:"name"`
	Capabilities []string  `json:"capabilities"`
	Status       string    `json:"status"`
	RegisteredAt time.Time `json:"registered_at"`
}

type ProviderStatusChangedData struct {
	ProviderID     string    `json:"provider_id"`
	PreviousStatus string    `json:"previous_status"`
	NewStatus      string    `json:"new_status"`
	ChangedAt      time.Time `json:"changed_at"`
}

type SubscriptionCreatedData struct {
	SubscriptionID string    `json:"subscription_id"`
	ProviderID     string    `json:"provider_id"`
	Categories     []string  `json:"categories"`
	CreatedAt      time.Time `json:"created_at"`
}

// Event type constants
const (
	// Work events
	EventWorkSubmitted       = "work.submitted"
	EventWorkBidWindowClosed = "work.bid_window_closed"
	EventWorkCancelled       = "work.cancelled"

	// Bid events
	EventBidSubmitted  = "bid.submitted"
	EventBidsEvaluated = "bids.evaluated"

	// Contract events
	EventContractAwarded   = "contract.awarded"
	EventContractCompleted = "contract.completed"
	EventContractFailed    = "contract.failed"

	// Settlement events
	EventSettlementCompleted = "settlement.completed"

	// Trust events
	EventTrustScoreUpdated = "trust.score_updated"
	EventTrustTierChanged  = "trust.tier_changed"

	// Identity events
	EventTenantCreated   = "tenant.created"
	EventTenantSuspended = "tenant.suspended"
	EventAPIKeyRevoked   = "apikey.revoked"

	// Provider events
	EventProviderRegistered    = "provider.registered"
	EventProviderStatusChanged = "provider.status_changed"
	EventSubscriptionCreated   = "subscription.created"
)
