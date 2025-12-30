package model

import "time"

type ProviderStatus string

const (
	ProviderStatusPendingVerification ProviderStatus = "PENDING_VERIFICATION"
	ProviderStatusActive              ProviderStatus = "ACTIVE"
	ProviderStatusSuspended           ProviderStatus = "SUSPENDED"
	ProviderStatusInactive            ProviderStatus = "INACTIVE"
)

type TrustTier string

const (
	TrustTierUnverified TrustTier = "UNVERIFIED"
	TrustTierVerified   TrustTier = "VERIFIED"
	TrustTierTrusted    TrustTier = "TRUSTED"
	TrustTierPreferred  TrustTier = "PREFERRED"
)

type Provider struct {
	ProviderID   string         `json:"provider_id" bson:"provider_id"`
	Name         string         `json:"name" bson:"name"`
	Description  string         `json:"description" bson:"description"`
	Endpoint     string         `json:"endpoint" bson:"endpoint"`
	BidWebhook   string         `json:"bid_webhook" bson:"bid_webhook"`
	Capabilities []string       `json:"capabilities" bson:"capabilities"`
	ContactEmail string         `json:"contact_email" bson:"contact_email"`
	Metadata     map[string]any `json:"metadata" bson:"metadata"`

	APIKeyHash    string `json:"-" bson:"api_key_hash"`
	APISecretHash string `json:"-" bson:"api_secret_hash"`

	Status     ProviderStatus `json:"status" bson:"status"`
	TrustScore float64        `json:"trust_score" bson:"trust_score"`
	TrustTier  TrustTier      `json:"trust_tier" bson:"trust_tier"`

	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}

type ProviderRegistrationRequest struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	Endpoint     string         `json:"endpoint"`
	BidWebhook   string         `json:"bid_webhook"`
	Capabilities []string       `json:"capabilities"`
	ContactEmail string         `json:"contact_email"`
	Metadata     map[string]any `json:"metadata"`
}

type ProviderRegistrationResponse struct {
	ProviderID string         `json:"provider_id"`
	APIKey     string         `json:"api_key"`
	APISecret  string         `json:"api_secret"`
	Status     ProviderStatus `json:"status"`
	TrustTier  TrustTier      `json:"trust_tier"`
	CreatedAt  time.Time      `json:"created_at"`
}

type SubscriptionFilter struct {
	MinBudget    *float64 `json:"min_budget,omitempty"`
	MaxLatencyMs *int64   `json:"max_latency_ms,omitempty"`
	Regions      []string `json:"regions,omitempty"`
}

type DeliveryConfig struct {
	Method        string `json:"method"` // webhook|polling
	WebhookURL    string `json:"webhook_url,omitempty"`
	WebhookSecret string `json:"webhook_secret,omitempty"`
}

type Subscription struct {
	SubscriptionID string             `json:"subscription_id" bson:"subscription_id"`
	ProviderID     string             `json:"provider_id" bson:"provider_id"`
	Categories     []string           `json:"categories" bson:"categories"`
	Filters        SubscriptionFilter `json:"filters" bson:"filters"`
	Delivery       DeliveryConfig     `json:"delivery" bson:"delivery"`
	Status         string             `json:"status" bson:"status"`
	CreatedAt      time.Time          `json:"created_at" bson:"created_at"`
}

type SubscriptionRequest struct {
	ProviderID string             `json:"provider_id"`
	Categories []string           `json:"categories"`
	Filters    SubscriptionFilter `json:"filters"`
	Delivery   DeliveryConfig     `json:"delivery"`
}

type SubscriptionResponse struct {
	SubscriptionID string    `json:"subscription_id"`
	ProviderID     string    `json:"provider_id"`
	Categories     []string  `json:"categories"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}



