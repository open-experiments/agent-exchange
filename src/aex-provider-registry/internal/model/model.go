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

// A2A Agent Card models

type AgentCard struct {
	Name             string            `json:"name" bson:"name"`
	Description      string            `json:"description" bson:"description"`
	URL              string            `json:"url" bson:"url"`
	Version          string            `json:"version" bson:"version"`
	Provider         *AgentProvider    `json:"provider,omitempty" bson:"provider,omitempty"`
	DocumentationURL string            `json:"documentationUrl,omitempty" bson:"documentation_url,omitempty"`
	Capabilities     AgentCapabilities `json:"capabilities" bson:"capabilities"`
	Skills           []AgentSkill      `json:"skills" bson:"skills"`
}

type AgentProvider struct {
	Organization string `json:"organization" bson:"organization"`
	URL          string `json:"url,omitempty" bson:"url,omitempty"`
}

type AgentCapabilities struct {
	Streaming              bool             `json:"streaming,omitempty" bson:"streaming,omitempty"`
	PushNotifications      bool             `json:"pushNotifications,omitempty" bson:"push_notifications,omitempty"`
	StateTransitionHistory bool             `json:"stateTransitionHistory,omitempty" bson:"state_transition_history,omitempty"`
	Extensions             []AgentExtension `json:"extensions,omitempty" bson:"extensions,omitempty"`
}

// AgentExtension represents an A2A extension capability (e.g., AP2)
type AgentExtension struct {
	URI         string `json:"uri" bson:"uri"`
	Description string `json:"description,omitempty" bson:"description,omitempty"`
	Required    bool   `json:"required,omitempty" bson:"required,omitempty"`
}

// AP2 Extension URI constant
const AP2ExtensionURI = "https://github.com/google-agentic-commerce/ap2/v1"

// HasAP2Support checks if the capabilities include AP2 extension
func (c *AgentCapabilities) HasAP2Support() bool {
	for _, ext := range c.Extensions {
		if ext.URI == AP2ExtensionURI {
			return true
		}
	}
	return false
}

type AgentSkill struct {
	ID          string   `json:"id" bson:"id"`
	Name        string   `json:"name" bson:"name"`
	Description string   `json:"description,omitempty" bson:"description,omitempty"`
	Tags        []string `json:"tags,omitempty" bson:"tags,omitempty"`
	Examples    []string `json:"examples,omitempty" bson:"examples,omitempty"`
	InputModes  []string `json:"inputModes,omitempty" bson:"input_modes,omitempty"`
	OutputModes []string `json:"outputModes,omitempty" bson:"output_modes,omitempty"`
}

// SkillIndex for fast skill-based lookups
type SkillIndex struct {
	SkillID     string    `json:"skill_id" bson:"skill_id"`
	SkillName   string    `json:"skill_name" bson:"skill_name"`
	Description string    `json:"description" bson:"description"`
	Tags        []string  `json:"tags" bson:"tags"`
	ProviderID  string    `json:"provider_id" bson:"provider_id"`
	AgentName   string    `json:"agent_name" bson:"agent_name"`
	AgentURL    string    `json:"agent_url" bson:"agent_url"`
	A2AEndpoint string    `json:"a2a_endpoint" bson:"a2a_endpoint"`
	CreatedAt   time.Time `json:"created_at" bson:"created_at"`
}

// ProviderWithA2A extends Provider with A2A information
type ProviderWithA2A struct {
	Provider
	AgentCard   *AgentCard `json:"agent_card,omitempty" bson:"agent_card,omitempty"`
	A2AEndpoint string     `json:"a2a_endpoint,omitempty" bson:"a2a_endpoint,omitempty"`
}

// SearchProvidersRequest for skill-based search
type SearchProvidersRequest struct {
	SkillTags []string `json:"skill_tags,omitempty"`
	Domain    string   `json:"domain,omitempty"`
	MinTrust  float64  `json:"min_trust,omitempty"`
	Limit     int      `json:"limit,omitempty"`
}

// SearchProvidersResponse contains matching providers
type SearchProvidersResponse struct {
	Providers []ProviderSearchResult `json:"providers"`
	Total     int                    `json:"total"`
}

type ProviderSearchResult struct {
	ProviderID  string   `json:"provider_id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Endpoint    string   `json:"endpoint"`
	A2AEndpoint string   `json:"a2a_endpoint"`
	TrustScore  float64  `json:"trust_score"`
	TrustTier   string   `json:"trust_tier"`
	Skills      []string `json:"skills"`
	MatchedTags []string `json:"matched_tags"`
	AP2Enabled  bool     `json:"ap2_enabled"`
}

// SearchProvidersRequestV2 includes AP2 filter
type SearchProvidersRequestV2 struct {
	SkillTags  []string `json:"skill_tags,omitempty"`
	Domain     string   `json:"domain,omitempty"`
	MinTrust   float64  `json:"min_trust,omitempty"`
	Limit      int      `json:"limit,omitempty"`
	RequireAP2 bool     `json:"require_ap2,omitempty"`
}
