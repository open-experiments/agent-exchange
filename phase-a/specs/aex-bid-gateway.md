# aex-bid-gateway Service Specification

## Overview

**Purpose:** Receive and validate bids from external provider agents. This is the high-throughput ingestion point for the bidding process.

**Language:** Go 1.22+
**Framework:** Chi router + gRPC
**Runtime:** Cloud Run
**Ports:** 8080 (HTTP), 50051 (gRPC)

## Architecture Position

```
    External Providers
           │
           ▼
  ┌─────────────────┐
  │ aex-bid-gateway │◄── THIS SERVICE
  │                 │
  │ • Receive bids  │
  │ • Validate      │
  │ • Store & notify│
  └────────┬────────┘
           │
  ┌────────┼────────┐
  ▼        ▼        ▼
Redis  Firestore  Pub/Sub
(cache) (durable)  (notify)
```

## Core Responsibilities

1. **Bid Ingestion** - High-throughput bid reception
2. **Credential Validation** - Verify provider API keys
3. **Bid Validation** - Check bid structure and constraints
4. **Storage** - Fast cache + durable persistence
5. **Notification** - Alert consumers of new bids
6. **Streaming** - Real-time bid feeds to consumers

## API Endpoints

### Submit Bid

#### POST /v1/bids

Submit a bid for work.

```json
// Headers
Authorization: Bearer {provider_api_key}

// Request
{
  "work_id": "work_550e8400",
  "price": 0.08,
  "confidence": 0.92,
  "approach": "Will use premium API for faster results with real-time availability",
  "estimated_latency_ms": 1500,
  "mvp_sample": {
    "sample_input": "Book flight LAX→JFK, March 15-22, 2 adults",
    "sample_output": "Found 23 options. Best: Delta DL123, $299/person, 5h 30m nonstop",
    "sample_latency_ms": 450
  },
  "sla": {
    "max_latency_ms": 3000,
    "availability": 0.99
  },
  "a2a_endpoint": "https://agent.expedia.com/a2a/v1",
  "expires_at": "2025-01-15T10:35:00Z"
}

// Response
{
  "bid_id": "bid_abc123",
  "work_id": "work_550e8400",
  "status": "RECEIVED",
  "received_at": "2025-01-15T10:30:15Z"
}
```

### Get Bids (Internal)

#### GET /internal/v1/bids?work_id={id}

Get all bids for a work spec (used by Bid Evaluator).

```json
{
  "work_id": "work_550e8400",
  "bids": [
    {
      "bid_id": "bid_abc123",
      "provider_id": "prov_expedia",
      "price": 0.08,
      "confidence": 0.92,
      "mvp_sample": {...},
      "received_at": "2025-01-15T10:30:15Z"
    },
    {
      "bid_id": "bid_def456",
      "provider_id": "prov_booking",
      "price": 0.12,
      "confidence": 0.88,
      "mvp_sample": {...},
      "received_at": "2025-01-15T10:30:18Z"
    }
  ],
  "total_bids": 2
}
```

### Consumer Bid Stream

#### WebSocket /v1/work/{work_id}/bids/stream

Real-time bid updates for consumers.

```json
// Connection: ws://aex-bid-gateway/v1/work/{work_id}/bids/stream
// Authorization via query param: ?token={consumer_jwt}

// Messages
{"type": "connected", "work_id": "work_550e8400"}
{"type": "bid", "bid_id": "bid_abc123", "provider": "Expedia", "price": 0.08, "confidence": 0.92}
{"type": "bid", "bid_id": "bid_def456", "provider": "Booking.com", "price": 0.12, "confidence": 0.88}
{"type": "window_closing", "seconds": 10}
{"type": "window_closed"}
```

## Data Models

### BidPacket

```go
type BidPacket struct {
    BidID            string            `json:"bid_id"`
    WorkID           string            `json:"work_id"`
    ProviderID       string            `json:"provider_id"`

    // Pricing
    Price            float64           `json:"price"`
    PriceBreakdown   map[string]float64 `json:"price_breakdown,omitempty"`

    // Quality signals
    Confidence       float64           `json:"confidence"`
    Approach         string            `json:"approach"`
    EstimatedLatency int64             `json:"estimated_latency_ms"`

    // Proof of competence
    MVPSample        *MVPSample        `json:"mvp_sample,omitempty"`

    // SLA commitment
    SLA              SLACommitment     `json:"sla"`

    // Execution endpoint
    A2AEndpoint      string            `json:"a2a_endpoint"`

    // Timestamps
    ExpiresAt        time.Time         `json:"expires_at"`
    ReceivedAt       time.Time         `json:"received_at"`
}

type MVPSample struct {
    SampleInput    string `json:"sample_input"`
    SampleOutput   string `json:"sample_output"`
    SampleLatency  int64  `json:"sample_latency_ms"`
}

type SLACommitment struct {
    MaxLatencyMs   int64   `json:"max_latency_ms"`
    Availability   float64 `json:"availability"`
}
```

## Core Implementation

### Bid Submission Handler

```go
func (h *BidHandler) SubmitBid(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // 1. Extract and validate provider credentials
    providerID, err := h.validateProviderAuth(r)
    if err != nil {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }

    // 2. Parse bid
    var bid BidPacket
    if err := json.NewDecoder(r.Body).Decode(&bid); err != nil {
        http.Error(w, "Invalid bid format", http.StatusBadRequest)
        return
    }
    bid.ProviderID = providerID
    bid.ReceivedAt = time.Now()
    bid.BidID = generateBidID()

    // 3. Validate bid
    if err := h.validateBid(ctx, &bid); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // 4. Check work is still accepting bids
    work, err := h.workStore.Get(ctx, bid.WorkID)
    if err != nil || work.State != "OPEN" {
        http.Error(w, "Work not accepting bids", http.StatusConflict)
        return
    }

    // 5. Store bid (Redis for speed, Firestore for durability)
    if err := h.storeBid(ctx, &bid); err != nil {
        http.Error(w, "Failed to store bid", http.StatusInternalServerError)
        return
    }

    // 6. Notify consumer of new bid
    h.notifyConsumer(ctx, &bid)

    // 7. Publish event
    h.publishBidReceived(ctx, &bid)

    // 8. Return response
    json.NewEncoder(w).Encode(BidResponse{
        BidID:      bid.BidID,
        WorkID:     bid.WorkID,
        Status:     "RECEIVED",
        ReceivedAt: bid.ReceivedAt,
    })
}

func (h *BidHandler) validateBid(ctx context.Context, bid *BidPacket) error {
    // Check required fields
    if bid.WorkID == "" || bid.Price <= 0 || bid.A2AEndpoint == "" {
        return errors.New("missing required fields")
    }

    // Check confidence in valid range
    if bid.Confidence < 0 || bid.Confidence > 1 {
        return errors.New("confidence must be between 0 and 1")
    }

    // Check bid not expired
    if bid.ExpiresAt.Before(time.Now()) {
        return errors.New("bid already expired")
    }

    return nil
}
```

### Bid Storage

```go
func (h *BidHandler) storeBid(ctx context.Context, bid *BidPacket) error {
    // Store in Redis for fast access during evaluation
    key := fmt.Sprintf("bids:%s:%s", bid.WorkID, bid.BidID)
    data, _ := json.Marshal(bid)

    if err := h.redis.Set(ctx, key, data, 1*time.Hour).Err(); err != nil {
        return err
    }

    // Add to work's bid list
    listKey := fmt.Sprintf("bids:%s", bid.WorkID)
    h.redis.SAdd(ctx, listKey, bid.BidID)

    // Store in Firestore for durability
    go func() {
        _ = h.firestore.SaveBid(context.Background(), bid)
    }()

    return nil
}

func (h *BidHandler) getBidsForWork(ctx context.Context, workID string) ([]*BidPacket, error) {
    // Get bid IDs from Redis set
    listKey := fmt.Sprintf("bids:%s", workID)
    bidIDs, err := h.redis.SMembers(ctx, listKey).Result()
    if err != nil {
        return nil, err
    }

    // Fetch each bid
    bids := make([]*BidPacket, 0, len(bidIDs))
    for _, bidID := range bidIDs {
        key := fmt.Sprintf("bids:%s:%s", workID, bidID)
        data, err := h.redis.Get(ctx, key).Bytes()
        if err != nil {
            continue
        }
        var bid BidPacket
        json.Unmarshal(data, &bid)
        bids = append(bids, &bid)
    }

    return bids, nil
}
```

### Consumer Notification

```go
func (h *BidHandler) notifyConsumer(ctx context.Context, bid *BidPacket) {
    // Get work to find consumer
    work, _ := h.workStore.Get(ctx, bid.WorkID)
    if work == nil {
        return
    }

    // Publish to consumer's WebSocket topic
    notification := BidNotification{
        Type:       "bid",
        BidID:      bid.BidID,
        Provider:   h.getProviderName(bid.ProviderID),
        Price:      bid.Price,
        Confidence: bid.Confidence,
    }

    h.wsHub.Broadcast(bid.WorkID, notification)
}
```

## Events

### Published Events

```json
// Bid received
{
  "event_type": "bid.submitted",
  "bid_id": "bid_abc123",
  "work_id": "work_550e8400",
  "provider_id": "prov_expedia",
  "price": 0.08,
  "received_at": "2025-01-15T10:30:15Z"
}
```

## Configuration

```bash
# Server
HTTP_PORT=8080
GRPC_PORT=50051
ENV=production

# Redis
REDIS_HOST=10.0.0.5
REDIS_PORT=6379
BID_CACHE_TTL_SECONDS=3600

# Firestore
FIRESTORE_PROJECT_ID=aex-prod
FIRESTORE_COLLECTION_BIDS=bids

# Pub/Sub
PUBSUB_PROJECT_ID=aex-prod
PUBSUB_TOPIC_BID_EVENTS=aex-bid-events

# WebSocket
WS_MAX_CONNECTIONS=10000
WS_PING_INTERVAL_SECONDS=30

# Observability
LOG_LEVEL=info
```

## Directory Structure

```
aex-bid-gateway/
├── cmd/
│   └── gateway/
│       └── main.go
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── handler/
│   │   ├── bid.go
│   │   └── stream.go
│   ├── store/
│   │   ├── redis.go
│   │   └── firestore.go
│   ├── websocket/
│   │   └── hub.go
│   └── auth/
│       └── provider.go
├── api/
│   └── proto/
│       └── bid.proto
├── Dockerfile
├── go.mod
└── go.sum
```
