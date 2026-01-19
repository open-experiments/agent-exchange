package model

import "time"

type TrustTier string

const (
	TrustTierUnverified TrustTier = "UNVERIFIED"
	TrustTierVerified   TrustTier = "VERIFIED"
	TrustTierTrusted    TrustTier = "TRUSTED"
	TrustTierPreferred  TrustTier = "PREFERRED"
	TrustTierInternal   TrustTier = "INTERNAL"
)

type OutcomeType string

const (
	OutcomeSuccess         OutcomeType = "SUCCESS"
	OutcomeSuccessPartial  OutcomeType = "SUCCESS_PARTIAL"
	OutcomeFailureProvider OutcomeType = "FAILURE_PROVIDER"
	OutcomeFailureExternal OutcomeType = "FAILURE_EXTERNAL"
	OutcomeFailureConsumer OutcomeType = "FAILURE_CONSUMER"
	OutcomeDisputeWon      OutcomeType = "DISPUTE_WON"
	OutcomeDisputeLost     OutcomeType = "DISPUTE_LOST"
	OutcomeExpired         OutcomeType = "EXPIRED"
)

type TrustRecord struct {
	ProviderID string `json:"provider_id" bson:"provider_id"`

	TrustScore float64   `json:"trust_score" bson:"trust_score"`
	TrustTier  TrustTier `json:"trust_tier" bson:"trust_tier"`
	BaseScore  float64   `json:"base_score" bson:"base_score"`

	IdentityVerified   bool `json:"identity_verified" bson:"identity_verified"`
	EndpointVerified   bool `json:"endpoint_verified" bson:"endpoint_verified"`
	ComplianceVerified bool `json:"compliance_verified" bson:"compliance_verified"`

	TotalContracts      int `json:"total_contracts" bson:"total_contracts"`
	SuccessfulContracts int `json:"successful_contracts" bson:"successful_contracts"`
	FailedContracts     int `json:"failed_contracts" bson:"failed_contracts"`
	DisputedContracts   int `json:"disputed_contracts" bson:"disputed_contracts"`
	DisputesWon         int `json:"disputes_won" bson:"disputes_won"`
	DisputesLost        int `json:"disputes_lost" bson:"disputes_lost"`

	RegisteredAt   time.Time  `json:"registered_at" bson:"registered_at"`
	LastContractAt *time.Time `json:"last_contract_at,omitempty" bson:"last_contract_at,omitempty"`
	LastUpdated    time.Time  `json:"last_updated" bson:"last_updated"`
}

type ContractOutcome struct {
	ID         string `json:"id" bson:"id"`
	ContractID string `json:"contract_id" bson:"contract_id"`
	ProviderID string `json:"provider_id" bson:"provider_id"`
	ConsumerID string `json:"consumer_id" bson:"consumer_id"`

	Outcome OutcomeType    `json:"outcome" bson:"outcome"`
	Metrics map[string]any `json:"metrics" bson:"metrics"`

	AgreedPrice float64 `json:"agreed_price" bson:"agreed_price"`
	FinalPrice  float64 `json:"final_price" bson:"final_price"`

	CompletedAt time.Time `json:"completed_at" bson:"completed_at"`
	RecordedAt  time.Time `json:"recorded_at" bson:"recorded_at"`
}

type BatchTrustRequest struct {
	ProviderIDs []string `json:"provider_ids"`
}

type BatchTrustResponse struct {
	Scores map[string]float64 `json:"scores"`
}
