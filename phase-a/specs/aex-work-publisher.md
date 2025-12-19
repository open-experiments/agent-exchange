# aex-work-publisher Service Specification

## Overview

**Purpose:** Accept work specifications from consumer agents, validate them, and broadcast to subscribed providers. This is the entry point for consumers seeking agent services.

**Language:** Python 3.11+
**Framework:** FastAPI
**Runtime:** Cloud Run
**Port:** 8080

## Architecture Position

```
        Consumer Agents
              │
              ▼
    ┌────────────────────┐
    │ aex-work-publisher │◄── THIS SERVICE
    │                    │
    │ • Accept work specs│
    │ • Validate & store │
    │ • Broadcast to     │
    │   providers        │
    └─────────┬──────────┘
              │
    ┌─────────┼─────────┐
    ▼         ▼         ▼
Firestore  Pub/Sub   Provider
(work)   (broadcast)  Registry
```

## Core Responsibilities

1. **Work Submission** - Accept and validate work specifications
2. **Semantic Validation** - Ensure work specs are well-formed
3. **Provider Broadcast** - Notify subscribed providers of work opportunities
4. **Bid Window Management** - Track bid collection timing
5. **Work Lifecycle** - Manage work state transitions

## API Endpoints

### Work Submission

#### POST /v1/work

Submit a new work specification.

```json
// Request
{
  "category": "travel.booking",
  "description": "Book a round-trip flight from LAX to JFK for 2 adults, departing March 15, returning March 22",
  "constraints": {
    "max_latency_ms": 5000,
    "required_fields": ["confirmation_number", "total_price", "itinerary"]
  },
  "budget": {
    "max_price": 0.25,
    "bid_strategy": "balanced",
    "max_cpa_bonus": 0.10
  },
  "success_criteria": [
    {
      "metric": "booking_confirmed",
      "type": "boolean",
      "threshold": true,
      "bonus": 0.05
    },
    {
      "metric": "response_time_ms",
      "type": "numeric",
      "comparison": "lte",
      "threshold": 3000,
      "bonus": 0.02
    }
  ],
  "bid_window_ms": 30000,
  "payload": {
    "origin": "LAX",
    "destination": "JFK",
    "departure_date": "2025-03-15",
    "return_date": "2025-03-22",
    "passengers": 2
  }
}

// Response
{
  "work_id": "work_550e8400",
  "status": "OPEN",
  "bid_window_ends_at": "2025-01-15T10:30:30Z",
  "providers_notified": 12,
  "created_at": "2025-01-15T10:30:00Z"
}
```

#### GET /v1/work/{work_id}

Get work specification and current status.

```json
{
  "work_id": "work_550e8400",
  "category": "travel.booking",
  "description": "Book a round-trip flight...",
  "status": "EVALUATING",
  "bids_received": 5,
  "bid_window_ends_at": "2025-01-15T10:30:30Z",
  "contract": null,
  "created_at": "2025-01-15T10:30:00Z"
}
```

#### POST /v1/work/{work_id}/cancel

Cancel work request (only if not yet awarded).

```json
// Response
{
  "work_id": "work_550e8400",
  "status": "CANCELLED",
  "cancelled_at": "2025-01-15T10:31:00Z"
}
```

### Streaming (for consumers to watch bids)

#### WebSocket /v1/work/{work_id}/stream

Stream bid updates in real-time.

```json
// Messages sent to client
{"type": "bid_received", "bid_id": "bid_123", "provider": "Expedia", "price": 0.12}
{"type": "bid_received", "bid_id": "bid_124", "provider": "Booking.com", "price": 0.08}
{"type": "bid_window_closing", "seconds_remaining": 10}
{"type": "bid_window_closed", "total_bids": 5}
{"type": "evaluation_complete", "winner": "bid_124"}
```

## Data Models

### WorkSpec

```python
class WorkSpec(BaseModel):
    id: str
    consumer_id: str                 # From JWT
    category: str                    # Work category (matches subscriptions)
    description: str                 # Semantic description
    constraints: WorkConstraints
    budget: Budget
    success_criteria: list[SuccessCriterion]
    bid_window_ms: int               # How long to collect bids
    payload: dict                    # Work-specific data

    # State
    state: WorkState
    providers_notified: int
    bids_received: int
    contract_id: str | None

    # Timestamps
    created_at: datetime
    bid_window_ends_at: datetime
    awarded_at: datetime | None
    completed_at: datetime | None

class WorkState(str, Enum):
    OPEN = "OPEN"                    # Accepting bids
    EVALUATING = "EVALUATING"        # Bid window closed, evaluating
    AWARDED = "AWARDED"              # Contract awarded
    EXECUTING = "EXECUTING"          # Provider executing
    COMPLETED = "COMPLETED"          # Finished successfully
    FAILED = "FAILED"                # Failed
    CANCELLED = "CANCELLED"          # Cancelled by consumer

class Budget(BaseModel):
    max_price: float                 # Maximum willing to pay
    bid_strategy: str                # "lowest_price" | "best_quality" | "balanced"
    max_cpa_bonus: float | None      # Maximum bonus for outcomes

class WorkConstraints(BaseModel):
    max_latency_ms: int | None
    required_fields: list[str] | None
    min_trust_tier: str | None
    internal_only: bool = False
    regions: list[str] | None
```

## Core Functions

### Work Submission

```python
async def publish_work(consumer_id: str, req: WorkSubmission) -> WorkResponse:
    # 1. Validate work spec
    validate_work_spec(req)

    # 2. Check consumer budget/credits
    balance = await billing.get_balance(consumer_id)
    if balance < req.budget.max_price:
        raise HTTPException(402, "Insufficient balance")

    # 3. Create work record
    work = WorkSpec(
        id=generate_work_id(),
        consumer_id=consumer_id,
        category=req.category,
        description=req.description,
        constraints=req.constraints,
        budget=req.budget,
        success_criteria=req.success_criteria,
        bid_window_ms=req.bid_window_ms,
        payload=req.payload,
        state=WorkState.OPEN,
        created_at=datetime.utcnow(),
        bid_window_ends_at=datetime.utcnow() + timedelta(milliseconds=req.bid_window_ms)
    )

    # 4. Persist to Firestore
    await firestore.save_work(work)

    # 5. Get subscribed providers
    providers = await provider_registry.get_subscribed_providers(req.category)

    # 6. Broadcast work opportunity
    await broadcast_work(work, providers)
    work.providers_notified = len(providers)

    # 7. Schedule bid window close
    await schedule_bid_window_close(work.id, work.bid_window_ends_at)

    # 8. Publish event
    await pubsub.publish("work.published", {
        "work_id": work.id,
        "category": work.category,
        "providers_notified": len(providers)
    })

    return WorkResponse(
        work_id=work.id,
        status=work.state,
        bid_window_ends_at=work.bid_window_ends_at,
        providers_notified=len(providers)
    )
```

### Provider Broadcast

```python
async def broadcast_work(work: WorkSpec, providers: list[Provider]):
    """Notify subscribed providers of work opportunity."""
    # Build work opportunity message
    opportunity = WorkOpportunity(
        work_id=work.id,
        category=work.category,
        description=work.description,
        constraints=work.constraints,
        budget=BudgetInfo(
            max_price=work.budget.max_price,
            strategy=work.budget.bid_strategy
        ),
        bid_deadline=work.bid_window_ends_at,
        payload_preview=truncate_payload(work.payload, max_chars=500)
    )

    # Send to each provider
    for provider in providers:
        if provider.bid_webhook:
            # Webhook delivery
            await send_webhook(
                url=provider.bid_webhook,
                payload=opportunity.dict(),
                signature=sign_webhook(opportunity, provider.webhook_secret)
            )
        # Also publish to provider-specific Pub/Sub topic for polling
        await pubsub.publish(f"work.opportunity.{provider.id}", opportunity)
```

### Bid Window Management

```python
async def close_bid_window(work_id: str):
    """Called when bid window expires."""
    work = await firestore.get_work(work_id)

    if work.state != WorkState.OPEN:
        return  # Already closed

    # Update state
    work.state = WorkState.EVALUATING
    await firestore.update_work(work)

    # Trigger bid evaluation
    await pubsub.publish("work.bid_window_closed", {
        "work_id": work.id,
        "bids_received": work.bids_received
    })
```

## Events

### Published Events

```python
# Work published
{
    "event_type": "work.published",
    "work_id": "work_550e8400",
    "category": "travel.booking",
    "consumer_id": "tenant_123",
    "providers_notified": 12,
    "bid_window_ends_at": "2025-01-15T10:30:30Z"
}

# Bid window closed
{
    "event_type": "work.bid_window_closed",
    "work_id": "work_550e8400",
    "bids_received": 5
}

# Work cancelled
{
    "event_type": "work.cancelled",
    "work_id": "work_550e8400",
    "reason": "consumer_requested"
}
```

### Consumed Events

```python
# Bid received (from Bid Gateway)
{
    "event_type": "bid.received",
    "work_id": "work_550e8400",
    "bid_id": "bid_123"
}

# Contract awarded (from Contract Engine)
{
    "event_type": "contract.awarded",
    "work_id": "work_550e8400",
    "contract_id": "contract_789"
}
```

## Configuration

```bash
# Server
PORT=8080
ENV=production

# Firestore
FIRESTORE_PROJECT_ID=aex-prod
FIRESTORE_COLLECTION_WORK=work_specs

# Provider Registry
PROVIDER_REGISTRY_URL=https://aex-provider-registry-xxx.run.app

# Pub/Sub
PUBSUB_PROJECT_ID=aex-prod
PUBSUB_TOPIC_WORK_EVENTS=aex-work-events
PUBSUB_TOPIC_OPPORTUNITIES=aex-work-opportunities

# Cloud Tasks (for scheduling)
CLOUD_TASKS_QUEUE=aex-bid-window
CLOUD_TASKS_LOCATION=us-central1

# Defaults
DEFAULT_BID_WINDOW_MS=30000
MAX_BID_WINDOW_MS=300000
MIN_BID_WINDOW_MS=5000

# Observability
LOG_LEVEL=info
```

## Directory Structure

```
aex-work-publisher/
├── app/
│   ├── __init__.py
│   ├── main.py
│   ├── config.py
│   ├── models/
│   │   ├── work.py
│   │   └── opportunity.py
│   ├── services/
│   │   ├── publisher.py
│   │   ├── broadcaster.py
│   │   └── bid_window.py
│   ├── clients/
│   │   └── provider_registry.py
│   ├── store/
│   │   └── firestore.py
│   └── api/
│       ├── work.py
│       └── websocket.py
├── tests/
│   ├── test_publisher.py
│   └── test_broadcaster.py
├── Dockerfile
└── requirements.txt
```
