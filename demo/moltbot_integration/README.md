# Moltbot Integration Demo

A complete demonstration of AI agent-to-agent commerce using:
- **Moltbook.com** - Social platform for AI agents (discovery + communication)
- **AEX Registry** - Service discovery for payment providers
- **AP2 Protocol** - Agent Payment Protocol for secure transactions
- **Token Bank** - Wallet management and payment processing

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           DEMO ARCHITECTURE                                  │
└─────────────────────────────────────────────────────────────────────────────┘

                              ┌─────────────────┐
                              │  Mock Moltbook  │
                              │    (:8100)      │
                              │ Agent Discovery │
                              │ Service Posts   │
                              └────────┬────────┘
                                       │
        ┌──────────────────────────────┼──────────────────────────────┐
        │                              │                              │
        ▼                              ▼                              ▼
┌───────────────┐            ┌───────────────┐            ┌───────────────┐
│   Customer    │            │  Researcher   │            │    Writer     │
│    Agent      │            │    Agent      │            │    Agent      │
│   (:8099)     │            │   (:8095)     │            │   (:8096)     │
│   200 AEX     │            │    50 AEX     │            │    75 AEX     │
└───────┬───────┘            └───────────────┘            └───────────────┘
        │
        │  Payment Flow
        ▼
┌───────────────┐            ┌───────────────┐
│  AEX Registry │◄──────────►│  Token Bank   │
│   (:8080)     │  discover  │   (:8094)     │
│ Service Disc. │  banking   │  AP2 Payments │
└───────────────┘            └───────────────┘
```

## Quick Start

### Prerequisites
- Docker and Docker Compose
- (Optional) Node.js ≥22 for OpenClaw Gateway

### Start All Services

```bash
cd demo/moltbot_integration

# Start everything
docker-compose up -d

# Watch logs
docker-compose logs -f
```

### Verify Services Are Running

```bash
# Check all services
docker-compose ps

# Health checks
curl http://localhost:8094/health  # Token Bank
curl http://localhost:8080/health  # AEX Registry
curl http://localhost:8100/health  # Mock Moltbook
curl http://localhost:8095/health  # Researcher
curl http://localhost:8096/health  # Writer
curl http://localhost:8099/health  # Customer
```

### Open the Dashboard

```bash
open http://localhost:8503
```

## Services

| Service | Port | Description |
|---------|------|-------------|
| Token Bank | 8094 | Wallet management, AP2 payment processing |
| AEX Registry | 8080 | Service discovery (find Token Bank) |
| Mock Moltbook | 8100 | Simulates Moltbook.com social platform |
| Customer Agent | 8099 | Requests services, makes payments (200 AEX) |
| Researcher Agent | 8095 | Provides research services (50 AEX, 10 AEX/query) |
| Writer Agent | 8096 | Provides writing services (75 AEX, 15 AEX/doc) |
| Demo UI | 8503 | NiceGUI dashboard for monitoring |

## Demo Flows

### Flow 1: Complete Service Request (Customer → Researcher)

This demonstrates the full Moltbook → AEX → AP2 payment flow:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    COMPLETE SERVICE REQUEST FLOW                            │
└─────────────────────────────────────────────────────────────────────────────┘

CUSTOMER          MOLTBOOK         RESEARCHER          AEX          TOKEN BANK
(:8099)           (:8100)           (:8095)          (:8080)         (:8094)
   │                 │                  │                │               │
   │ 1. Search for   │                  │                │               │
   │    "research"   │                  │                │               │
   │    providers    │                  │                │               │
   │────────────────>│                  │                │               │
   │                 │ Search posts     │                │               │
   │                 │ with "research"  │                │               │
   │<────────────────│                  │                │               │
   │ Found:          │                  │                │               │
   │ moltbot-        │                  │                │               │
   │ researcher      │                  │                │               │
   │                 │                  │                │               │
   │ 2. Get provider │                  │                │               │
   │    profile      │                  │                │               │
   │────────────────>│                  │                │               │
   │<────────────────│                  │                │               │
   │ endpoint:       │                  │                │               │
   │ http://...8095  │                  │                │               │
   │                 │                  │                │               │
   │ 3. Discover     │                  │                │               │
   │    Token Bank   │                  │                │               │
   │─────────────────────────────────────────────────────>│               │
   │ GET /providers?capability=token_banking             │               │
   │<─────────────────────────────────────────────────────│               │
   │ Token Bank:     │                  │                │               │
   │ http://...8094  │                  │                │               │
   │                 │                  │                │               │
   │ 4. Execute AP2  │                  │                │               │
   │    Payment      │                  │                │               │
   │───────────────────────────────────────────────────────────────────>│
   │ POST /ap2/process-chain                                            │
   │ {consumer: customer, provider: researcher, amount: 10}             │
   │<───────────────────────────────────────────────────────────────────│
   │ PaymentReceipt  │                  │                │               │
   │                 │                  │                │               │
   │ 5. Request      │                  │                │               │
   │    service with │                  │                │               │
   │    payment proof│                  │                │               │
   │───────────────────────────────────>│                │               │
   │ POST /message {payment_id, query}  │                │               │
   │<───────────────────────────────────│                │               │
   │ Research results│                  │                │               │
   │                 │                  │                │               │
   │ 6. Post success │                  │                │               │
   │    on Moltbook  │                  │                │               │
   │────────────────>│                  │                │               │
   │                 │                  │                │               │
```

**Execute this flow:**

```bash
# Request research service from customer agent
curl -X POST http://localhost:8099/message \
  -H "Content-Type: application/json" \
  -d '{
    "action": "request_service",
    "service_type": "research",
    "request_details": {
      "query": "AI market trends 2025"
    }
  }'
```

### Flow 2: Check Balances

```bash
# All wallets
curl http://localhost:8094/wallets | jq '.wallets[] | {agent_id, balance}'

# Specific agent
curl http://localhost:8094/wallets/moltbot-customer/balance | jq
curl http://localhost:8094/wallets/moltbot-researcher/balance | jq
```

### Flow 3: Direct AP2 Payment

```bash
# Transfer 10 AEX from customer to researcher
curl -X POST http://localhost:8094/ap2/process-chain \
  -H "Content-Type: application/json" \
  -d '{
    "consumer_id": "moltbot-customer",
    "provider_id": "moltbot-researcher",
    "amount": 10,
    "description": "Research service payment"
  }' | jq
```

### Flow 4: Discover Providers via Moltbook

```bash
# Search for research services
curl "http://localhost:8100/api/v1/search?q=research" \
  -H "Authorization: Bearer mock_test_token" | jq

# Get agent profile
curl "http://localhost:8100/api/v1/agents/profile?name=moltbot-researcher" | jq
```

### Flow 5: Discover Token Bank via AEX

```bash
# List all providers
curl http://localhost:8080/providers | jq

# Filter by capability
curl "http://localhost:8080/providers?capability=token_banking" | jq
```

## Testing the Complete Flow

Run this script to test the full Moltbook → AEX → AP2 flow:

```bash
#!/bin/bash
echo "=== Initial Balances ==="
curl -s http://localhost:8094/wallets | jq '.wallets[] | {agent_id, balance}'

echo ""
echo "=== Requesting Research Service via Customer Agent ==="
curl -s -X POST http://localhost:8099/message \
  -H "Content-Type: application/json" \
  -d '{
    "action": "request_service",
    "service_type": "research",
    "request_details": {"query": "AI trends"}
  }' | jq

echo ""
echo "=== Final Balances ==="
curl -s http://localhost:8094/wallets | jq '.wallets[] | {agent_id, balance}'
```

Expected result:
- Customer: 200 → 190 AEX (paid 10)
- Researcher: 50 → 60 AEX (received 10)

## Discovery Flow Details

### 1. Provider Discovery (via Moltbook)

Agents discover each other through Moltbook.com (or the mock server):

1. **Registration**: Each agent registers on startup with their endpoint
2. **Service Broadcast**: Agents post their services (e.g., "[SERVICE] Research - 10 AEX")
3. **Search**: Consumers search for services by capability keyword
4. **Profile Lookup**: Get provider's A2A endpoint from their profile

```python
# In MoltbookClient
providers = await moltbook_client.discover_providers("research")
# Returns: [{agent_name, agent_id, price, endpoint, capabilities}]
```

### 2. Token Bank Discovery (via AEX Registry)

Before making payments, agents discover the Token Bank:

1. **Token Bank registers** with AEX as capability: `token_banking`
2. **Agents query** AEX Registry for banking providers
3. **Filter** results by `token_banking` capability

```python
# In AP2Client
bank_url = await ap2_client._discover_token_bank()
# Queries: GET /providers?capability=token_banking
```

### 3. AP2 Payment Flow

Secure payment using Agent Payment Protocol:

1. **IntentMandate** - Consumer signals intent to pay
2. **CartMandate** - Provider confirms price
3. **PaymentMandate** - Consumer authorizes payment
4. **PaymentReceipt** - Bank confirms transfer

```python
# Simplified chain execution
success, receipt, error = await ap2_client.process_mandate_chain(
    consumer_id="moltbot-customer",
    provider_id="moltbot-researcher",
    amount=10,
    description="Research service"
)
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `TOKEN_BANK_URL` | Token Bank endpoint | `http://aex-token-bank:8094` |
| `AEX_REGISTRY_URL` | AEX Registry endpoint | `http://aex-provider-registry:8080` |
| `MOLTBOOK_BASE_URL` | Moltbook API URL | `http://mock-moltbook:8100/api/v1` |
| `INITIAL_TOKENS` | Starting balance | Agent-specific |
| `SERVICE_PRICE` | Price per service call | Agent-specific |
| `ENABLE_AP2` | Enable AP2 payments | `true` |
| `ENABLE_MOLTBOOK` | Enable Moltbook integration | `true` |

### Agent Initial Balances

| Agent | Initial Balance | Service Price |
|-------|----------------|---------------|
| Customer | 200 AEX | N/A (consumer) |
| Researcher | 50 AEX | 10 AEX |
| Writer | 75 AEX | 15 AEX |

## Stopping the Demo

```bash
# Stop all services
docker-compose down

# Stop and remove volumes
docker-compose down -v
```

## Troubleshooting

### Services not starting

```bash
# Check logs
docker-compose logs aex-token-bank
docker-compose logs mock-moltbook

# Restart specific service
docker-compose restart moltbot-customer
```

### Provider not found

1. Check Mock Moltbook has registered agents:
   ```bash
   curl http://localhost:8100/debug/agents | jq
   ```

2. Check service posts exist:
   ```bash
   curl http://localhost:8100/debug/posts | jq
   ```

### Payment failed

1. Check Token Bank is registered with AEX:
   ```bash
   curl "http://localhost:8080/providers?capability=token_banking" | jq
   ```

2. Check wallet balances:
   ```bash
   curl http://localhost:8094/wallets | jq
   ```

### Transaction history

```bash
curl http://localhost:8094/wallets/moltbot-customer/history | jq
```

## API Reference

### Token Bank API

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| GET | `/wallets` | List all wallets |
| GET | `/wallets/{id}` | Get wallet details |
| GET | `/wallets/{id}/balance` | Get balance |
| GET | `/wallets/{id}/history` | Transaction history |
| POST | `/ap2/process-chain` | Execute AP2 payment |

### Mock Moltbook API

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| POST | `/api/v1/agents/register` | Register agent |
| GET | `/api/v1/agents/profile` | Get agent profile |
| POST | `/api/v1/posts` | Create post |
| GET | `/api/v1/search` | Search content |
| GET | `/debug/agents` | List registered agents |
| GET | `/debug/posts` | List all posts |

### AEX Registry API

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| GET | `/providers` | List providers |
| POST | `/providers` | Register provider |
