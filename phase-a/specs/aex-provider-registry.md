# aex-provider-registry Service Specification

## Overview

**Purpose:** Register external provider agents, manage their endpoints, and handle work category subscriptions. This is the entry point for providers joining the AEX marketplace.

**Language:** Go 1.22+
**Framework:** Chi router (net/http)
**Runtime:** Cloud Run
**Port:** 8080

## Architecture Position

```
                External Providers
                      │
                      ▼
         ┌────────────────────────┐
         │ aex-provider-registry  │◄── THIS SERVICE
         │                        │
         │ • Register providers   │
         │ • Manage subscriptions │
         │ • Validate endpoints   │
         └───────────┬────────────┘
                     │
          ┌──────────┼──────────┐
          ▼          ▼          ▼
     Firestore    Pub/Sub   Trust Broker
    (providers)  (events)   (init score)
```

## Core Responsibilities

1. **Provider Registration** - Onboard external agent providers
2. **Endpoint Validation** - Verify provider A2A endpoints are accessible
3. **Subscription Management** - Track which providers serve which work categories
4. **Credential Issuance** - Generate API keys for providers to submit bids
5. **Provider Lifecycle** - Handle status changes, suspension, reactivation

## API Endpoints

### Provider Registration

#### POST /v1/providers

Register a new provider agent.

```json
// Request
{
  "name": "Expedia Travel Agent",
  "description": "Full-service travel booking and search",
  "endpoint": "https://agent.expedia.com/a2a",
  "bid_webhook": "https://agent.expedia.com/aex/work",
  "capabilities": ["travel.booking", "travel.search", "hospitality.hotels"],
  "contact_email": "agents@expedia.com",
  "metadata": {
    "company": "Expedia Group",
    "region": "global"
  }
}

// Response
{
  "provider_id": "prov_abc123",
  "api_key": "aex_pk_live_...",  // For bid submission
  "api_secret": "aex_sk_live_...",
  "status": "PENDING_VERIFICATION",
  "trust_tier": "UNVERIFIED",
  "created_at": "2025-01-15T10:00:00Z"
}
```

#### GET /v1/providers/{provider_id}

Get provider details.

```json
{
  "provider_id": "prov_abc123",
  "name": "Expedia Travel Agent",
  "endpoint": "https://agent.expedia.com/a2a",
  "status": "ACTIVE",
  "trust_score": 0.87,
  "trust_tier": "TRUSTED",
  "capabilities": ["travel.booking", "travel.search"],
  "subscriptions": [
    {"category": "travel.*", "filters": {...}},
    {"category": "hospitality.hotels", "filters": {...}}
  ],
  "stats": {
    "total_contracts": 1250,
    "success_rate": 0.94,
    "avg_response_time_ms": 1200
  }
}
```

#### PUT /v1/providers/{provider_id}

Update provider details.

### Subscription Management

#### POST /v1/subscriptions

Subscribe to work categories.

```json
// Request
{
  "provider_id": "prov_abc123",
  "categories": ["travel.*", "hospitality.hotels"],
  "filters": {
    "min_budget": 0.05,
    "max_latency_ms": 5000,
    "regions": ["us", "eu"]
  },
  "delivery": {
    "method": "webhook",  // or "polling"
    "webhook_url": "https://agent.expedia.com/aex/work",
    "webhook_secret": "whsec_..."
  }
}

// Response
{
  "subscription_id": "sub_xyz789",
  "provider_id": "prov_abc123",
  "categories": ["travel.*", "hospitality.hotels"],
  "status": "ACTIVE",
  "created_at": "2025-01-15T10:05:00Z"
}
```

#### GET /v1/subscriptions?provider_id={id}

List subscriptions for a provider.

#### DELETE /v1/subscriptions/{subscription_id}

Remove a subscription.

### Internal APIs (Exchange Use Only)

#### GET /internal/v1/providers/subscribed

Get providers subscribed to a work category.

```json
// Request: GET /internal/v1/providers/subscribed?category=travel.booking

// Response
{
  "category": "travel.booking",
  "providers": [
    {
      "provider_id": "prov_abc123",
      "webhook_url": "https://agent.expedia.com/aex/work",
      "trust_score": 0.87
    },
    {
      "provider_id": "prov_def456",
      "webhook_url": "https://agent.booking.com/aex/work",
      "trust_score": 0.91
    }
  ]
}
```

## Data Models

### Provider

```go
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
	ID          string         `json:"provider_id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Endpoint    string         `json:"endpoint"`      // A2A endpoint for execution
	BidWebhook  string         `json:"bid_webhook"`   // delivery target (optional)
	Capabilities []string      `json:"capabilities"`
	ContactEmail string        `json:"contact_email"`
	Metadata    map[string]any `json:"metadata"`

	APIKeyHash    string `json:"-"`
	APISecretHash string `json:"-"`

	Status     ProviderStatus `json:"status"`
	TrustScore float64        `json:"trust_score"`
	TrustTier  TrustTier      `json:"trust_tier"`

	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
}
```

### Subscription

```go
type SubscriptionFilter struct {
	MinBudget     *float64  `json:"min_budget,omitempty"`
	MaxLatencyMs  *int64    `json:"max_latency_ms,omitempty"`
	Regions       []string  `json:"regions,omitempty"`
}

type DeliveryConfig struct {
	Method        string `json:"method"` // "webhook" or "polling"
	WebhookURL    string `json:"webhook_url,omitempty"`
	WebhookSecret string `json:"webhook_secret,omitempty"`
}

type Subscription struct {
	ID         string `json:"subscription_id"`
	ProviderID string `json:"provider_id"`
	Categories []string `json:"categories"` // glob patterns, e.g. "travel.*"
	Filters    SubscriptionFilter `json:"filters"`
	Delivery   DeliveryConfig     `json:"delivery"`
	Status     string             `json:"status"`
	CreatedAt  time.Time          `json:"created_at"`
}
```

## Core Functions

### Provider Registration

```go
func (s *Service) RegisterProvider(ctx context.Context, req ProviderRegistration) (ProviderResponse, error) {
	// 1. Validate endpoint is accessible
	if err := s.validateEndpoint(ctx, req.Endpoint); err != nil {
		return ProviderResponse{}, ErrEndpointNotAccessible
	}

	// 2. Generate API credentials
	apiKey := generateAPIKey()
	apiSecret := generateAPISecret()

	// 3. Get initial trust score from Trust Broker
	trustScore, err := s.trustBroker.GetInitialScore(ctx, "external")
	if err != nil {
		return ProviderResponse{}, err
	}

	// 4. Create provider record
	now := time.Now().UTC()
	provider := Provider{
		ID:           generateProviderID(),
		Name:         req.Name,
		Description:  req.Description,
		Endpoint:     req.Endpoint,
		BidWebhook:   req.BidWebhook,
		Capabilities: req.Capabilities,
		ContactEmail: req.ContactEmail,
		Metadata:     req.Metadata,
		APIKeyHash:   hashKey(apiKey),
		APISecretHash: hashKey(apiSecret),
		Status:       ProviderStatusPendingVerification,
		TrustScore:   trustScore,
		TrustTier:    TrustTierUnverified,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// 5. Persist to Firestore
	if err := s.store.SaveProvider(ctx, provider); err != nil {
		return ProviderResponse{}, err
	}

	// 6. Publish event
	_ = s.events.Publish(ctx, "provider.registered", map[string]any{
		"provider_id": provider.ID,
		"name":        provider.Name,
		"timestamp":   now.Format(time.RFC3339Nano),
	})

	// 7. Return with credentials (only time they're shown)
	return ProviderResponse{
		ProviderID: provider.ID,
		APIKey:     apiKey,
		APISecret:  apiSecret,
		Status:     string(provider.Status),
	}, nil
}
```

### Endpoint Validation

```go
func (s *Service) validateEndpoint(ctx context.Context, endpoint string) error {
	// Send A2A discovery request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/.well-known/a2a", nil)
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}
	if _, ok := data["capabilities"]; !ok {
		return errors.New("missing capabilities")
	}
	if _, ok := data["version"]; !ok {
		return errors.New("missing version")
	}
	return nil
}
```

### Subscription Matching

```go
func (s *Service) GetSubscribedProviders(ctx context.Context, category string) ([]Provider, error) {
	subs, err := s.store.QuerySubscriptions(ctx, category)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(subs))
	for _, sub := range subs {
		ids = append(ids, sub.ProviderID)
	}
	providers, err := s.store.GetProviders(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make([]Provider, 0, len(providers))
	for _, p := range providers {
		if p.Status == ProviderStatusActive {
			out = append(out, p)
		}
	}
	return out, nil
}

func categoryMatches(subscriptionPattern, workCategory string) bool {
	// Examples:
	// - "travel.*" matches "travel.booking", "travel.search"
	// - "travel.booking" matches only "travel.booking"
	// - "*" matches everything
	ok, err := path.Match(subscriptionPattern, workCategory)
	return err == nil && ok
}
```

## Events

### Published Events

```json
{
  "event_type": "provider.registered",
  "provider_id": "prov_abc123",
  "name": "Expedia Travel Agent",
  "timestamp": "2025-01-15T10:00:00Z"
}
{
  "event_type": "provider.status_changed",
  "provider_id": "prov_abc123",
  "old_status": "PENDING_VERIFICATION",
  "new_status": "ACTIVE",
  "timestamp": "2025-01-15T10:30:00Z"
}
{
  "event_type": "subscription.created",
  "subscription_id": "sub_xyz789",
  "provider_id": "prov_abc123",
  "categories": ["travel.*"],
  "timestamp": "2025-01-15T10:05:00Z"
}
```

## Configuration

```bash
# Server
PORT=8080
ENV=production

# Firestore
FIRESTORE_PROJECT_ID=aex-prod
FIRESTORE_COLLECTION_PROVIDERS=providers
FIRESTORE_COLLECTION_SUBSCRIPTIONS=subscriptions

# Trust Broker
TRUST_BROKER_URL=https://aex-trust-broker-xxx.run.app

# Pub/Sub
PUBSUB_PROJECT_ID=aex-prod
PUBSUB_TOPIC_EVENTS=aex-provider-events

# Validation
ENDPOINT_VALIDATION_TIMEOUT_MS=10000

# Observability
LOG_LEVEL=info
```

## Directory Structure

```
aex-provider-registry/
├── cmd/
│   └── provider-registry/
│       └── main.go
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── api/
│   │   ├── providers.go
│   │   ├── subscriptions.go
│   │   └── internal.go
│   ├── model/
│   │   ├── provider.go
│   │   └── subscription.go
│   ├── service/
│   │   ├── registration.go
│   │   ├── subscription.go
│   │   └── validation.go
│   ├── clients/
│   │   └── trustbroker.go
│   └── store/
│       └── firestore.go
├── hack/
│   └── tests/
├── Dockerfile
├── go.mod
└── go.sum
```
