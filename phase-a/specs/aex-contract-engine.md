# aex-contract-engine Service Specification

## Overview

**Purpose:** Award contracts to winning bidders, track execution status, and handle completion verification. This is the contract lifecycle manager.

**Language:** Go 1.22+
**Framework:** Chi router (net/http)
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

```go
type ContractStatus string

const (
	ContractStatusAwarded   ContractStatus = "AWARDED"
	ContractStatusExecuting ContractStatus = "EXECUTING"
	ContractStatusCompleted ContractStatus = "COMPLETED"
	ContractStatusFailed    ContractStatus = "FAILED"
	ContractStatusExpired   ContractStatus = "EXPIRED"
	ContractStatusDisputed  ContractStatus = "DISPUTED"
)

type ExecutionUpdate struct {
	Status    string    `json:"status"`
	Percent   *int      `json:"percent,omitempty"`
	Message   *string   `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type OutcomeReport struct {
	Success           bool           `json:"success"`
	ResultSummary     string         `json:"result_summary"`
	Metrics           map[string]any `json:"metrics"`
	ResultLocation    *string        `json:"result_location,omitempty"`
	ReportedAt        time.Time      `json:"reported_at"`
	ProviderSignature *string        `json:"provider_signature,omitempty"`
}

type Contract struct {
	ID         string `json:"contract_id"`
	WorkID     string `json:"work_id"`
	ConsumerID string `json:"consumer_id"`
	ProviderID string `json:"provider_id"`
	BidID      string `json:"bid_id"`

	AgreedPrice      float64       `json:"agreed_price"`
	SLA              SLACommitment `json:"sla"`
	ProviderEndpoint string        `json:"provider_endpoint"`

	ExecutionToken string `json:"execution_token"`
	ConsumerToken  string `json:"consumer_token"`

	Status    ContractStatus `json:"status"`
	ExpiresAt time.Time      `json:"expires_at"`

	AwardedAt   time.Time  `json:"awarded_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	FailedAt    *time.Time `json:"failed_at,omitempty"`

	ExecutionUpdates []ExecutionUpdate `json:"execution_updates,omitempty"`
	Outcome          *OutcomeReport    `json:"outcome,omitempty"`
}
```

## Core Functions

### Award Contract

```go
func (s *Service) AwardContract(ctx context.Context, workID string, bidID string, auto bool) (Contract, error) {
	// 1. Get work and evaluation
	work, err := s.workPublisher.GetWork(ctx, workID)
	if err != nil {
		return Contract{}, err
	}
	eval, err := s.bidEvaluator.GetEvaluation(ctx, workID)
	if err != nil {
		return Contract{}, err
	}

	// 2. Determine winning bid
	if auto {
		if len(eval.RankedBids) == 0 {
			return Contract{}, ErrNoValidBids
		}
		bidID = eval.RankedBids[0].BidID
	} else {
		if !eval.HasBid(bidID) {
			return Contract{}, ErrInvalidBidID
		}
	}

	// 3. Get full bid details
	bid, err := s.bidGateway.GetBid(ctx, bidID)
	if err != nil {
		return Contract{}, err
	}

	// 4. Create contract
	now := time.Now().UTC()
	expires := now.Add(time.Hour)
	contract := Contract{
		ID:              generateContractID(),
		WorkID:          workID,
		ConsumerID:      work.ConsumerID,
		ProviderID:      bid.ProviderID,
		BidID:           bidID,
		AgreedPrice:     bid.Price,
		SLA:             bid.SLA,
		ProviderEndpoint: bid.A2AEndpoint,
		ExecutionToken:  generateExecutionToken(),
		ConsumerToken:   generateConsumerToken(),
		Status:          ContractStatusAwarded,
		ExpiresAt:       expires,
		AwardedAt:       now,
	}

	// 5. Persist contract
	if err := s.store.SaveContract(ctx, contract); err != nil {
		return Contract{}, err
	}

	// 6. Update work status
	_ = s.workPublisher.UpdateWorkStatus(ctx, workID, "AWARDED", &contract.ID)

	// 7. Notify provider
	_ = s.webhooks.Send(ctx, bid.A2AEndpoint+"/contract-awarded", ContractAwardNotification{
		ContractID:     contract.ID,
		WorkID:         contract.WorkID,
		ExecutionToken: contract.ExecutionToken,
		ExpiresAt:      contract.ExpiresAt,
	}, "")

	// 8. Publish event
	_ = s.events.Publish(ctx, "contract.awarded", map[string]any{
		"contract_id":   contract.ID,
		"work_id":       contract.WorkID,
		"provider_id":   contract.ProviderID,
		"consumer_id":   contract.ConsumerID,
		"agreed_price":  contract.AgreedPrice,
		"provider_endpoint": contract.ProviderEndpoint,
	})

	return contract, nil
}
```

### Handle Completion

```go
func (s *Service) CompleteContract(ctx context.Context, contractID string, executionToken string, outcome OutcomeReport) (Contract, error) {
	// 1. Validate execution token
	contract, err := s.store.GetContract(ctx, contractID)
	if err != nil {
		return Contract{}, err
	}
	if contract.ExecutionToken != executionToken {
		return Contract{}, ErrInvalidExecutionToken
	}
	if contract.Status != ContractStatusExecuting {
		return Contract{}, ErrInvalidContractState
	}

	// 2. Update contract
	now := time.Now().UTC()
	contract.Status = ContractStatusCompleted
	contract.CompletedAt = &now
	outcome.ReportedAt = now
	contract.Outcome = &outcome
	if err := s.store.UpdateContract(ctx, contract); err != nil {
		return Contract{}, err
	}

	// 3. Trigger settlement
	_ = s.events.Publish(ctx, "contract.completed", map[string]any{
		"contract_id":  contract.ID,
		"work_id":      contract.WorkID,
		"consumer_id":  contract.ConsumerID,
		"provider_id":  contract.ProviderID,
		"agreed_price": contract.AgreedPrice,
		"outcome":      contract.Outcome,
	})

	// 4. Update work status
	_ = s.workPublisher.UpdateWorkStatus(ctx, contract.WorkID, "COMPLETED", nil)

	return contract, nil
}
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
├── cmd/
│   └── contract-engine/
│       └── main.go
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── api/
│   │   ├── http.go
│   │   └── callbacks.go
│   ├── model/
│   │   ├── contract.go
│   │   └── outcome.go
│   ├── service/
│   │   ├── award.go
│   │   └── completion.go
│   ├── clients/
│   │   ├── workpublisher.go
│   │   └── bidgateway.go
│   ├── store/
│   │   └── firestore.go
│   └── events/
│       └── publisher.go
├── hack/
│   └── tests/
├── Dockerfile
├── go.mod
└── go.sum
```
