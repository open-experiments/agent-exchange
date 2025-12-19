# aex-provider-registry Service Specification

## Overview

**Purpose:** Register external provider agents, manage their endpoints, and handle work category subscriptions. This is the entry point for providers joining the AEX marketplace.

**Language:** Python 3.11+
**Framework:** FastAPI
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

#### GET /internal/providers/subscribed

Get providers subscribed to a work category.

```json
// Request: GET /internal/providers/subscribed?category=travel.booking

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

```python
class Provider(BaseModel):
    id: str
    name: str
    description: str
    endpoint: str                    # A2A endpoint for execution
    bid_webhook: str | None          # Where to send work opportunities
    capabilities: list[str]          # Claimed capability categories
    contact_email: str
    metadata: dict

    # Credentials (hashed)
    api_key_hash: str
    api_secret_hash: str

    # Status
    status: ProviderStatus           # PENDING|ACTIVE|SUSPENDED|INACTIVE
    trust_score: float               # From Trust Broker
    trust_tier: TrustTier            # UNVERIFIED|VERIFIED|TRUSTED|PREFERRED

    # Timestamps
    created_at: datetime
    updated_at: datetime
    verified_at: datetime | None

class ProviderStatus(str, Enum):
    PENDING_VERIFICATION = "PENDING_VERIFICATION"
    ACTIVE = "ACTIVE"
    SUSPENDED = "SUSPENDED"
    INACTIVE = "INACTIVE"

class TrustTier(str, Enum):
    UNVERIFIED = "UNVERIFIED"
    VERIFIED = "VERIFIED"
    TRUSTED = "TRUSTED"
    PREFERRED = "PREFERRED"
```

### Subscription

```python
class Subscription(BaseModel):
    id: str
    provider_id: str
    categories: list[str]            # Glob patterns like "travel.*"
    filters: SubscriptionFilter
    delivery: DeliveryConfig
    status: SubscriptionStatus
    created_at: datetime

class SubscriptionFilter(BaseModel):
    min_budget: float | None
    max_latency_ms: int | None
    regions: list[str] | None

class DeliveryConfig(BaseModel):
    method: str                      # "webhook" or "polling"
    webhook_url: str | None
    webhook_secret: str | None
```

## Core Functions

### Provider Registration

```python
async def register_provider(req: ProviderRegistration) -> Provider:
    # 1. Validate endpoint is accessible
    endpoint_valid = await validate_endpoint(req.endpoint)
    if not endpoint_valid:
        raise HTTPException(400, "Endpoint not accessible")

    # 2. Generate API credentials
    api_key = generate_api_key()
    api_secret = generate_api_secret()

    # 3. Get initial trust score from Trust Broker
    trust_score = await trust_broker.get_initial_score()

    # 4. Create provider record
    provider = Provider(
        id=generate_provider_id(),
        name=req.name,
        endpoint=req.endpoint,
        api_key_hash=hash_key(api_key),
        api_secret_hash=hash_key(api_secret),
        status=ProviderStatus.PENDING_VERIFICATION,
        trust_score=trust_score,
        trust_tier=TrustTier.UNVERIFIED,
        created_at=datetime.utcnow()
    )

    # 5. Persist to Firestore
    await firestore.save_provider(provider)

    # 6. Publish event
    await pubsub.publish("provider.registered", provider)

    # 7. Return with credentials (only time they're shown)
    return ProviderResponse(
        provider_id=provider.id,
        api_key=api_key,
        api_secret=api_secret,
        status=provider.status
    )
```

### Endpoint Validation

```python
async def validate_endpoint(endpoint: str) -> bool:
    """Verify provider endpoint is accessible and responds correctly."""
    try:
        # Send A2A discovery request
        async with httpx.AsyncClient(timeout=10) as client:
            resp = await client.get(f"{endpoint}/.well-known/a2a")
            if resp.status_code == 200:
                data = resp.json()
                # Verify required A2A fields
                return "capabilities" in data and "version" in data
    except Exception:
        pass
    return False
```

### Subscription Matching

```python
async def get_subscribed_providers(category: str) -> list[Provider]:
    """Get all providers subscribed to a work category."""
    # Query subscriptions that match this category
    subscriptions = await firestore.query_subscriptions(category)

    # Get provider details
    provider_ids = [s.provider_id for s in subscriptions]
    providers = await firestore.get_providers(provider_ids)

    # Filter to active only
    active = [p for p in providers if p.status == ProviderStatus.ACTIVE]

    return active

def category_matches(subscription_pattern: str, work_category: str) -> bool:
    """Check if subscription pattern matches work category.

    Examples:
    - "travel.*" matches "travel.booking", "travel.search"
    - "travel.booking" matches only "travel.booking"
    - "*" matches everything
    """
    import fnmatch
    return fnmatch.fnmatch(work_category, subscription_pattern)
```

## Events

### Published Events

```python
# Provider registered
{
    "event_type": "provider.registered",
    "provider_id": "prov_abc123",
    "name": "Expedia Travel Agent",
    "timestamp": "2025-01-15T10:00:00Z"
}

# Provider status changed
{
    "event_type": "provider.status_changed",
    "provider_id": "prov_abc123",
    "old_status": "PENDING_VERIFICATION",
    "new_status": "ACTIVE",
    "timestamp": "2025-01-15T10:30:00Z"
}

# Subscription created
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
├── app/
│   ├── __init__.py
│   ├── main.py                 # FastAPI app
│   ├── config.py
│   ├── models/
│   │   ├── provider.py
│   │   └── subscription.py
│   ├── services/
│   │   ├── registration.py
│   │   ├── subscription.py
│   │   └── validation.py
│   ├── clients/
│   │   └── trust_broker.py
│   ├── store/
│   │   └── firestore.py
│   └── api/
│       ├── providers.py
│       ├── subscriptions.py
│       └── internal.py
├── tests/
│   ├── test_registration.py
│   └── test_subscription.py
├── Dockerfile
└── requirements.txt
```
