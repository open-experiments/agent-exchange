# Agent Exchange - Quick Start Guide

Get up and running with Agent Exchange in minutes!

## Prerequisites

- **Go 1.22+** - [Install Go](https://golang.org/doc/install)
- **Docker & Docker Compose** - [Install Docker](https://docs.docker.com/get-docker/)
- **Make** (optional but recommended)

## Quick Start (5 minutes)

### Option 1: Using Make (Recommended)

```bash
# Clone the repository
git clone <repo-url>
cd agent-exchange

# Build and start everything
make quickstart

# Check service health
make health
```

That's it! All services are now running.

### Option 2: Manual Setup

```bash
# 1. Build all services
make build

# 2. Build Docker images
make docker-build

# 3. Start services
make docker-up

# 4. View logs
make docker-logs
```

### Option 3: Local Development (without Docker)

```bash
# 1. Start MongoDB
make mongo-up

# 2. Initialize database
mongosh mongodb://root:root@localhost:27017 < hack/mongo-init.js

# 3. Run a service locally
make dev-work-publisher
# or
make dev-settlement
```

## Service URLs

Once running, services are available at:

| Service | URL | Status |
|---------|-----|--------|
| **Gateway** | http://localhost:8080 | ✅ Ready |
| **Work Publisher** | http://localhost:8081 | ✅ Ready |
| **Bid Gateway** | http://localhost:8082 | ✅ Ready |
| **Bid Evaluator** | http://localhost:8083 | ✅ Ready |
| **Contract Engine** | http://localhost:8084 | ✅ Ready |
| **Provider Registry** | http://localhost:8085 | ✅ Ready |
| **Trust Broker** | http://localhost:8086 | ✅ Ready |
| **Identity** | http://localhost:8087 | ✅ Ready |
| **Settlement** | http://localhost:8088 | ✅ Ready |
| **Telemetry** | http://localhost:8089 | ⚠️ MVP |

**MongoDB:** `mongodb://root:root@localhost:27017`

## Test the System

### 1. Submit Work

```bash
curl -X POST http://localhost:8081/v1/work \
  -H "Content-Type: application/json" \
  -d '{
    "category": "general",
    "description": "Test work request",
    "budget": {
      "max_price": 100.0,
      "bid_strategy": "balanced"
    },
    "success_criteria": [
      {
        "metric": "accuracy",
        "threshold": 0.95,
        "comparison": ">=",
        "bonus": 10.0
      }
    ]
  }'
```

Response:
```json
{
  "work_id": "work_abc123",
  "status": "open",
  "bid_window_ends_at": "2025-12-23T12:30:00Z",
  "providers_notified": 5,
  "created_at": "2025-12-23T12:00:00Z"
}
```

### 2. Check Balance

```bash
curl http://localhost:8088/v1/balance?tenant_id=tenant_test001
```

Response:
```json
{
  "tenant_id": "tenant_test001",
  "balance": "1000.00",
  "currency": "USD"
}
```

### 3. Process Deposit

```bash
curl -X POST http://localhost:8088/v1/deposits \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "tenant_test001",
    "amount": "500.00"
  }'
```

### 4. Get Usage Data

```bash
curl "http://localhost:8088/v1/usage?tenant_id=tenant_test001&limit=10"
```

### 5. Health Checks

```bash
# Check all services
make health

# Or individually
curl http://localhost:8081/health  # work-publisher
curl http://localhost:8088/health  # settlement
```

## Common Commands

```bash
# Build specific service
make build-aex-work-publisher

# Test specific service
make test-aex-settlement

# View logs for specific service
make docker-logs-aex-work-publisher

# Rebuild Docker image
make docker-build-aex-settlement

# Stop all services
make docker-down

# Clean and restart
make docker-clean && make quickstart

# Format code
make fmt

# Run linters
make lint

# Tidy dependencies
make tidy
```

## Development Workflow

### Working on a Service

```bash
# 1. Start dependencies (MongoDB)
make mongo-up

# 2. Run service locally
cd src/aex-work-publisher
ENVIRONMENT=development \
STORE_TYPE=memory \
PORT=8081 \
go run ./src

# 3. Make changes and test
# 4. Build and test
go build -o bin/aex-work-publisher ./src
./bin/aex-work-publisher

# 5. Run tests
go test ./... -v
```

### Adding a New HTTP Client

```go
// Example: Create client in internal/clients/
package clients

import (
    "context"
    "time"
    "github.com/parlakisik/agent-exchange/internal/httpclient"
)

type MyServiceClient struct {
    baseURL string
    client  *httpclient.Client
}

func NewMyServiceClient(baseURL string) *MyServiceClient {
    return &MyServiceClient{
        baseURL: baseURL,
        client:  httpclient.NewClient("my-service", 10*time.Second),
    }
}

func (c *MyServiceClient) GetData(ctx context.Context, id string) (*Data, error) {
    var result Data
    err := httpclient.NewRequest("GET", c.baseURL).
        Path("/v1/data/" + id).
        Context(ctx).
        ExecuteJSON(c.client, &result)
    return &result, err
}
```

## Troubleshooting

### Services won't start

```bash
# Check if ports are already in use
lsof -i :8080-8089

# Check Docker logs
make docker-logs

# Restart with fresh state
make docker-clean && make quickstart
```

### MongoDB connection issues

```bash
# Check MongoDB is running
docker ps | grep mongo

# Test connection
mongosh mongodb://root:root@localhost:27017

# Restart MongoDB
docker restart aex-mongo
```

### Build failures

```bash
# Clean and rebuild
make clean
make tidy
make build
```

### Service-specific issues

```bash
# Check service logs
make docker-logs-aex-work-publisher

# Restart specific service
docker-compose -f hack/docker-compose.yml restart aex-work-publisher

# Check service health
curl http://localhost:8081/health -v
```

## Environment Variables

Copy `.env.example` to `.env` and customize:

```bash
cp .env.example .env
# Edit .env with your values
```

Key variables:
- `MONGO_URI` - MongoDB connection string
- `ENVIRONMENT` - development | production
- `WORK_PUBLISHER_STORE_TYPE` - mongo | firestore | memory
- `PLATFORM_FEE_RATE` - Settlement platform fee (default: 0.15)

## Next Steps

1. **Run the Demo** - See `demo/README.md` for the full demo with UI
2. **Run Integration Tests** - `make test`
3. **Explore the Code** - Start with `src/aex-work-publisher`
4. **Check the Roadmap** - See `src/development-roadmap.md` for gaps and next steps
5. **Read Phase A Specs** - See `phase-a/readme.md` for architecture details

## Architecture

```
┌─────────────┐
│ aex-gateway │ (8080) - API Gateway
└──────┬──────┘
       │
   ┌───┴────────────────────┐
   │                        │
┌──▼───────────┐   ┌────────▼───────┐
│work-publisher│   │   settlement   │
│   (8081)     │   │     (8088)     │
└──┬───────────┘   └────────┬───────┘
   │                        │
   │  ┌─────────────────┐   │
   └─▶│  bid-gateway    │◀──┘
      │    (8082)       │
      └────────┬────────┘
               │
      ┌────────▼────────┐
      │ bid-evaluator   │
      │    (8083)       │
      └─────────────────┘
```

## Getting Help

- **Issues:** Report bugs in GitHub Issues
- **Documentation:** See `docs/` folder and `phase-a/specs/`
- **Roadmap:** Check `src/development-roadmap.md` for current status and gaps
