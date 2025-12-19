# aex-contract-engine Service Specification

## Overview

**Purpose:** Award contracts to winning bidders, track execution status, and handle completion verification. This is the contract lifecycle manager.

**Language:** Python 3.11+
**Framework:** FastAPI
**Runtime:** Cloud Run
**Port:** 8080

## Architecture Position

```
       Bid Evaluator        Consumer/Provider
            │                     │
            ▼                     ▼
   ┌─────────────────────────────────────┐
   │       aex-contract-engine           │◄── THIS SERVICE
   │                                     │
   │ • Award contracts                   │
   │ • Track execution                   │
   │ • Verify completion                 │
   └──────────────┬──────────────────────┘
                  │
        ┌─────────┼─────────┐
        ▼         ▼         ▼
   Firestore  Settlement  Trust Broker
  (contracts)  (payment)   (reputation)
```

## Core Responsibilities

1. **Contract Award** - Create contracts from winning bids
2. **Endpoint Delivery** - Return provider A2A endpoint to consumer
3. **Execution Tracking** - Monitor progress (via provider callbacks)
4. **Completion Handling** - Process outcome reports
5. **Dispute Management** - Handle contract disputes

## Key Concept

After contract award, **AEX exits the execution path**. The consumer communicates directly with the provider via A2A protocol. AEX only re-enters when:
- Provider reports completion
- Consumer reports an issue
- Contract times out

## API Endpoints

### Award Contract

#### POST /v1/work/{work_id}/award

Award contract to a bid (can be auto or consumer-selected).

```json
// Request
{
  "bid_id": "bid_def456",
  "auto_award": false  // true = award to top-ranked bid
}

// Response
{
  "contract_id": "contract_789xyz",
  "work_id": "work_550e8400",
  "provider_id": "prov_booking",
  "agreed_price": 0.08,
  "status": "AWARDED",
  "provider_endpoint": "https://agent.booking.com/a2a/v1",
  "execution_token": "exec_token_abc...",  // Provider uses this to report
  "expires_at": "2025-01-15T11:30:00Z",
  "awarded_at": "2025-01-15T10:31:00Z"
}
```

### Get Contract

#### GET /v1/contracts/{contract_id}

Get contract details and status.

```json
{
  "contract_id": "contract_789xyz",
  "work_id": "work_550e8400",
  "consumer_id": "tenant_123",
  "provider_id": "prov_booking",
  "agreed_price": 0.08,
  "status": "EXECUTING",
  "provider_endpoint": "https://agent.booking.com/a2a/v1",
  "awarded_at": "2025-01-15T10:31:00Z",
  "execution_updates": [
    {"status": "started", "timestamp": "2025-01-15T10:31:05Z"},
    {"status": "progress", "percent": 50, "timestamp": "2025-01-15T10:31:30Z"}
  ],
  "outcome": null
}
```

### Report Execution Progress (Provider)

#### POST /v1/contracts/{contract_id}/progress

Provider reports execution progress.

```json
// Headers
Authorization: Bearer {execution_token}

// Request
{
  "status": "progress",
  "percent": 75,
  "message": "Processing booking confirmation"
}

// Response
{
  "acknowledged": true,
  "contract_id": "contract_789xyz"
}
```

### Report Completion (Provider)

#### POST /v1/contracts/{contract_id}/complete

Provider reports task completion with outcome.

```json
// Headers
Authorization: Bearer {execution_token}

// Request
{
  "success": true,
  "result_summary": "Flight booked successfully",
  "metrics": {
    "booking_confirmed": true,
    "response_time_ms": 2300,
    "total_price": 598.00,
    "options_found": 23
  },
  "result_location": "https://agent.booking.com/results/abc123"  // Optional
}

// Response
{
  "contract_id": "contract_789xyz",
  "status": "COMPLETED",
  "settlement_initiated": true,
  "completed_at": "2025-01-15T10:32:00Z"
}
```

### Report Failure (Provider or Consumer)

#### POST /v1/contracts/{contract_id}/fail

Report contract failure.

```json
// Request
{
  "reason": "external_api_error",
  "message": "Booking API returned 503",
  "reported_by": "provider"  // or "consumer"
}

// Response
{
  "contract_id": "contract_789xyz",
  "status": "FAILED",
  "failure_reason": "external_api_error",
  "failed_at": "2025-01-15T10:32:00Z"
}
```

## Data Models

### Contract

```python
class Contract(BaseModel):
    id: str
    work_id: str
    consumer_id: str
    provider_id: str
    bid_id: str

    # Agreement
    agreed_price: float
    sla: SLACommitment
    provider_endpoint: str

    # Security
    execution_token: str       # Provider uses to authenticate callbacks
    consumer_token: str        # Consumer can use to report issues

    # Status
    status: ContractStatus
    expires_at: datetime

    # Timestamps
    awarded_at: datetime
    started_at: datetime | None
    completed_at: datetime | None
    failed_at: datetime | None

    # Tracking
    execution_updates: list[ExecutionUpdate]
    outcome: OutcomeReport | None

class ContractStatus(str, Enum):
    AWARDED = "AWARDED"          # Contract created, pending start
    EXECUTING = "EXECUTING"      # Provider is working
    COMPLETED = "COMPLETED"      # Successfully finished
    FAILED = "FAILED"            # Failed
    EXPIRED = "EXPIRED"          # Timed out
    DISPUTED = "DISPUTED"        # Under dispute

class ExecutionUpdate(BaseModel):
    status: str
    percent: int | None
    message: str | None
    timestamp: datetime

class OutcomeReport(BaseModel):
    success: bool
    result_summary: str
    metrics: dict[str, Any]
    result_location: str | None
    reported_at: datetime
    provider_signature: str | None
```

## Core Functions

### Award Contract

```python
async def award_contract(work_id: str, bid_id: str, auto: bool = False) -> Contract:
    # 1. Get work and evaluation
    work = await work_publisher.get_work(work_id)
    evaluation = await bid_evaluator.get_evaluation(work_id)

    # 2. Determine winning bid
    if auto:
        if not evaluation.ranked_bids:
            raise HTTPException(400, "No valid bids to award")
        winning_bid = evaluation.ranked_bids[0]
        bid_id = winning_bid.bid_id
    else:
        winning_bid = next(
            (b for b in evaluation.ranked_bids if b.bid_id == bid_id),
            None
        )
        if not winning_bid:
            raise HTTPException(400, "Invalid bid ID")

    # 3. Get full bid details
    bid = await bid_gateway.get_bid(bid_id)

    # 4. Create contract
    contract = Contract(
        id=generate_contract_id(),
        work_id=work_id,
        consumer_id=work.consumer_id,
        provider_id=bid.provider_id,
        bid_id=bid_id,
        agreed_price=bid.price,
        sla=bid.sla,
        provider_endpoint=bid.a2a_endpoint,
        execution_token=generate_execution_token(),
        consumer_token=generate_consumer_token(),
        status=ContractStatus.AWARDED,
        expires_at=datetime.utcnow() + timedelta(hours=1),
        awarded_at=datetime.utcnow()
    )

    # 5. Persist contract
    await firestore.save_contract(contract)

    # 6. Update work status
    await work_publisher.update_work_status(work_id, "AWARDED", contract.id)

    # 7. Notify provider
    await notify_provider_awarded(contract, bid)

    # 8. Publish event
    await pubsub.publish("contract.awarded", {
        "contract_id": contract.id,
        "work_id": work_id,
        "provider_id": contract.provider_id,
        "consumer_id": contract.consumer_id,
        "agreed_price": contract.agreed_price
    })

    return contract

async def notify_provider_awarded(contract: Contract, bid: BidPacket):
    """Notify provider that their bid won."""
    notification = ContractAwardNotification(
        contract_id=contract.id,
        work_id=contract.work_id,
        execution_token=contract.execution_token,
        consumer_endpoint=None,  # Consumer initiates contact
        expires_at=contract.expires_at
    )

    # Send to provider webhook
    await send_webhook(
        url=f"{bid.a2a_endpoint}/contract-awarded",
        payload=notification.dict()
    )
```

### Handle Completion

```python
async def complete_contract(
    contract_id: str,
    outcome: OutcomeReport,
    execution_token: str
) -> Contract:
    # 1. Validate execution token
    contract = await firestore.get_contract(contract_id)
    if contract.execution_token != execution_token:
        raise HTTPException(401, "Invalid execution token")

    if contract.status != ContractStatus.EXECUTING:
        raise HTTPException(400, f"Cannot complete contract in {contract.status} status")

    # 2. Update contract
    contract.status = ContractStatus.COMPLETED
    contract.completed_at = datetime.utcnow()
    contract.outcome = outcome
    await firestore.update_contract(contract)

    # 3. Trigger settlement
    await pubsub.publish("contract.completed", {
        "contract_id": contract.id,
        "work_id": contract.work_id,
        "consumer_id": contract.consumer_id,
        "provider_id": contract.provider_id,
        "agreed_price": contract.agreed_price,
        "outcome": outcome.dict()
    })

    # 4. Update work status
    await work_publisher.update_work_status(contract.work_id, "COMPLETED")

    return contract
```

## Events

### Published Events

```json
// Contract awarded
{
  "event_type": "contract.awarded",
  "contract_id": "contract_789xyz",
  "work_id": "work_550e8400",
  "provider_id": "prov_booking",
  "consumer_id": "tenant_123",
  "agreed_price": 0.08,
  "provider_endpoint": "https://agent.booking.com/a2a/v1"
}

// Contract completed
{
  "event_type": "contract.completed",
  "contract_id": "contract_789xyz",
  "work_id": "work_550e8400",
  "outcome": {
    "success": true,
    "metrics": {...}
  }
}

// Contract failed
{
  "event_type": "contract.failed",
  "contract_id": "contract_789xyz",
  "work_id": "work_550e8400",
  "reason": "provider_timeout"
}
```

### Consumed Events

```json
// Bids evaluated (for auto-award)
{
  "event_type": "bids.evaluated",
  "work_id": "work_550e8400",
  "winner_bid_id": "bid_def456"
}
```

## Configuration

```bash
# Server
PORT=8080
ENV=production

# Firestore
FIRESTORE_PROJECT_ID=aex-prod
FIRESTORE_COLLECTION_CONTRACTS=contracts

# Service URLs
WORK_PUBLISHER_URL=https://aex-work-publisher-xxx.run.app
BID_GATEWAY_URL=https://aex-bid-gateway-xxx.run.app

# Pub/Sub
PUBSUB_PROJECT_ID=aex-prod
PUBSUB_TOPIC_EVENTS=aex-contract-events

# Contract settings
DEFAULT_EXPIRY_HOURS=1
MAX_EXPIRY_HOURS=24

# Observability
LOG_LEVEL=info
```

## Directory Structure

```
aex-contract-engine/
├── app/
│   ├── __init__.py
│   ├── main.py
│   ├── config.py
│   ├── models/
│   │   ├── contract.py
│   │   └── outcome.py
│   ├── services/
│   │   ├── award.py
│   │   ├── tracking.py
│   │   └── completion.py
│   ├── clients/
│   │   ├── work_publisher.py
│   │   └── bid_gateway.py
│   ├── store/
│   │   └── firestore.py
│   └── api/
│       ├── contracts.py
│       └── callbacks.py
├── tests/
│   ├── test_award.py
│   └── test_completion.py
├── Dockerfile
└── requirements.txt
```
