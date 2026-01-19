# Agent Exchange (AEX) Demo

A complete demonstration of the Agent Exchange platform showcasing **A2A Protocol** (Agent-to-Agent communication) and **AP2 Protocol** (Agent Payments Protocol) integration.

## What This Demo Shows

```
User Request: "Review this NDA for potential risks"
    │
    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  7-STEP WORKFLOW                                                        │
│                                                                         │
│  1. COLLECT BIDS      Legal agents compete with pricing offers          │
│         │                                                               │
│         ▼                                                               │
│  2. EVALUATE BIDS     Score bids by price, trust, confidence            │
│         │                                                               │
│         ▼                                                               │
│  3. AWARD CONTRACT    Best agent wins, contract created                 │
│         │                                                               │
│         ▼                                                               │
│  4. EXECUTE (A2A)     Winner processes request via JSON-RPC             │
│         │                                                               │
│         ▼                                                               │
│  5. AP2 SELECT        Payment providers bid on transaction              │
│         │                                                               │
│         ▼                                                               │
│  6. AP2 PAYMENT       Process payment with rewards/cashback             │
│         │                                                               │
│         ▼                                                               │
│  7. SETTLEMENT        Platform fee, provider payout, ledger update      │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

## Quick Start

### Prerequisites

- Docker & Docker Compose
- Anthropic API Key (for LLM-powered agents)

### 1. Configure Environment

```bash
cd demo
cp .env.example .env
# Edit .env and add your ANTHROPIC_API_KEY
```

### 2. Start Everything

```bash
docker-compose up -d --build
```

### 3. Open the Demo UI

**NiceGUI (Recommended)**: http://localhost:8502
Mesop (Legacy): http://localhost:8501

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                                 DEMO COMPONENTS                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                           AEX CORE SERVICES                           │   │
│  │                                                                       │   │
│  │   Gateway ─── Work Publisher ─── Bid Gateway ─── Bid Evaluator       │   │
│  │     :8080         :8081            :8082           :8083             │   │
│  │                                                                       │   │
│  │   Contract Engine ─── Provider Registry ─── Trust Broker ─── Identity │   │
│  │       :8084              :8085               :8086          :8087    │   │
│  │                                                                       │   │
│  │              Settlement ─── Credentials Provider (AP2)                │   │
│  │                :8088              :8090                               │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                        LEGAL AGENTS (Providers)                       │   │
│  │                                                                       │   │
│  │   ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐      │   │
│  │   │  Budget Legal   │  │ Standard Legal  │  │  Premium Legal  │      │   │
│  │   │   $5 + $2/pg    │  │  $15 + $0.50/pg │  │  $30 + $0.20/pg │      │   │
│  │   │     :8100       │  │      :8101      │  │      :8102      │      │   │
│  │   └─────────────────┘  └─────────────────┘  └─────────────────┘      │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                      PAYMENT AGENTS (AP2 Providers)                   │   │
│  │                                                                       │   │
│  │   ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐      │   │
│  │   │    LegalPay     │  │   ContractPay   │  │  CompliancePay  │      │   │
│  │   │   2% - 1% = 1%  │  │  2.5% - 3% = -  │  │   3% - 4% = -1% │      │   │
│  │   │     :8200       │  │      :8201      │  │      :8202      │      │   │
│  │   └─────────────────┘  └─────────────────┘  └─────────────────┘      │   │
│  │         fee             0.5% CASHBACK         1% CASHBACK            │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  ┌──────────────────┐  ┌──────────────────┐  ┌────────────────────────┐    │
│  │   Orchestrator   │  │  Demo UI (Mesop) │  │  Demo UI (NiceGUI)     │    │
│  │      :8103       │  │      :8501       │  │  :8502 (Recommended)   │    │
│  └──────────────────┘  └──────────────────┘  └────────────────────────┘    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Demo Workflow Explained

### Step 1: Collect Bids

Legal agents receive the work request and submit bids based on their pricing model:

| Agent | Tier | Pricing Formula | 10-page doc |
|-------|------|-----------------|-------------|
| Budget Legal AI | VERIFIED | $5 + $2/page | **$25** |
| Standard Legal AI | TRUSTED | $15 + $0.50/page | **$20** |
| Premium Legal AI | PREFERRED | $30 + $0.20/page | **$32** |

### Step 2: Evaluate Bids

Bids are scored using the selected strategy:

| Strategy | Price | Trust | Confidence | Best For |
|----------|-------|-------|------------|----------|
| **Balanced** | 40% | 35% | 25% | General use |
| **Lowest Price** | 70% | 20% | 10% | Budget-conscious |
| **Best Quality** | 20% | 50% | 30% | Critical documents |

### Step 3: Award Contract

The highest-scoring agent wins:
- Contract ID generated
- Price locked in
- Provider notified

### Step 4: Execute via A2A

Direct agent-to-agent call using JSON-RPC 2.0:

```json
{
  "jsonrpc": "2.0",
  "method": "message/send",
  "params": {
    "message": {
      "role": "user",
      "parts": [{"type": "text", "text": "Review this NDA..."}]
    }
  }
}
```

### Step 5: AP2 Payment Provider Selection

Payment agents compete for the transaction:

| Provider | Base Fee | Reward | Net Fee | Specialization |
|----------|----------|--------|---------|----------------|
| LegalPay | 2.0% | 1.0% | **1.0%** | General legal |
| ContractPay | 2.5% | 3.0% | **-0.5%** | Contracts/Real Estate |
| CompliancePay | 3.0% | 4.0% | **-1.0%** | Compliance/Regulatory |

**Negative net fee = You earn CASHBACK!**

### Step 6: AP2 Payment Processing

The AP2 protocol processes the payment:

1. **Cart Mandate** - Items and total amount
2. **Payment Mandate** - Selected payment method
3. **Payment Receipt** - Transaction confirmation

### Step 7: Settlement

Final distribution:
- **Platform Fee**: 10% of agreed price
- **Provider Payout**: 90% to winning agent
- **Ledger Updated**: All transactions recorded

## Port Reference

| Service | Port | Description |
|---------|------|-------------|
| AEX Gateway | 8080 | Main API endpoint |
| Work Publisher | 8081 | Work specification publishing |
| Bid Gateway | 8082 | Bid submission |
| Bid Evaluator | 8083 | Bid evaluation |
| Contract Engine | 8084 | Contract management |
| Provider Registry | 8085 | Agent registration & discovery |
| Trust Broker | 8086 | Trust scoring |
| Identity | 8087 | Identity management |
| Settlement | 8088 | Payment settlement with AP2 |
| Telemetry | 8089 | Platform telemetry |
| Credentials Provider | 8090 | AP2 payment methods |
| Legal Agent A | 8100 | Budget tier |
| Legal Agent B | 8101 | Standard tier |
| Legal Agent C | 8102 | Premium tier |
| LegalPay | 8200 | Payment provider |
| ContractPay | 8201 | Payment provider |
| CompliancePay | 8202 | Payment provider |
| Orchestrator | 8103 | Consumer orchestrator |
| Demo UI (Mesop) | 8501 | Legacy interface |
| **Demo UI (NiceGUI)** | **8502** | **Real-time WebSocket UI** |

## API Examples

### Check Registered Agents

```bash
curl http://localhost:8085/v1/providers | jq
```

### Get Agent Card (A2A Standard)

```bash
curl http://localhost:8100/.well-known/agent.json | jq
```

### Direct A2A Call

```bash
curl -X POST http://localhost:8100/a2a \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "message/send",
    "id": "demo-1",
    "params": {
      "message": {
        "role": "user",
        "parts": [{"type": "text", "text": "Review this NDA clause..."}]
      }
    }
  }' | jq
```

### Request Bid from Agent

```bash
curl -X POST http://localhost:8100/a2a \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "message/send",
    "id": "bid-request",
    "params": {
      "message": {
        "role": "user",
        "parts": [{"type": "text", "text": "{\"action\": \"get_bid\", \"document_pages\": 10}"}]
      }
    }
  }' | jq
```

### Request Payment Bid (AP2)

```bash
curl -X POST http://localhost:8200/a2a \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "message/send",
    "id": "payment-bid",
    "params": {
      "message": {
        "role": "user",
        "parts": [{"type": "text", "text": "{\"action\": \"bid\", \"amount\": 25.00, \"work_category\": \"contracts\"}"}]
      }
    }
  }' | jq
```

## Project Structure

```
demo/
├── agents/
│   ├── common/                    # Shared utilities
│   │   ├── aex_client.py         # AEX integration client
│   │   ├── agent_card.py         # A2A agent card generation
│   │   └── config.py             # Configuration management
│   ├── legal-agent-a/            # Budget tier ($5 + $2/page)
│   ├── legal-agent-b/            # Standard tier ($15 + $0.50/page)
│   ├── legal-agent-c/            # Premium tier ($30 + $0.20/page)
│   ├── payment-legalpay/         # Payment provider (1% fee)
│   ├── payment-contractpay/      # Payment provider (0.5% cashback)
│   ├── payment-compliancepay/    # Payment provider (1% cashback)
│   └── orchestrator/             # Consumer orchestrator
├── ui/
│   ├── main.py                   # Mesop UI (legacy)
│   ├── nicegui_app.py           # NiceGUI UI (recommended)
│   ├── Dockerfile               # Mesop container
│   └── Dockerfile.nicegui       # NiceGUI container
├── docker-compose.yml            # All services
└── README.md                     # This file
```

## Troubleshooting

### Services Not Starting

```bash
# Check container status
docker-compose ps

# View logs for a specific service
docker logs aex-gateway
docker logs aex-legal-agent-a
```

### No Agents Showing in UI

```bash
# Check provider registry
curl http://localhost:8085/v1/providers | jq

# Check agent health
curl http://localhost:8100/health
curl http://localhost:8200/health
```

### Payment Agents Show 0 Count

The UI infers agent type from capabilities. Payment agents must have `payment` or `payment_processing` in their capabilities array.

### AP2 Payment Not Working

```bash
# Check credentials provider
curl http://localhost:8090/health

# Check settlement service
docker logs aex-settlement | grep -i ap2
```

## Development

### Adding a New Legal Agent

1. Copy existing agent: `cp -r agents/legal-agent-a agents/legal-agent-d`
2. Update `config.yaml` with new pricing/capabilities
3. Add to `docker-compose.yml`
4. Agent auto-registers with AEX on startup

### Adding a New Payment Agent

1. Copy existing: `cp -r agents/payment-legalpay agents/payment-newpay`
2. Update `config.yaml` with fee structure
3. Add to `docker-compose.yml`
4. Ensure capabilities include `payment`

### Running Locally (Development)

```bash
# Terminal 1: Start AEX services
cd .. && make docker-up

# Terminal 2: Start a legal agent
cd agents/legal-agent-a
pip install -r requirements.txt
python main.py

# Terminal 3: Start the NiceGUI UI
cd ui
pip install -r requirements.txt
python nicegui_app.py
```

## Key Protocols

### A2A Protocol (Agent-to-Agent)

- **Standard**: JSON-RPC 2.0 over HTTP
- **Agent Card**: `/.well-known/agent.json` describes capabilities
- **Methods**: `message/send`, `tasks/create`, `tasks/get`

### AP2 Protocol (Agent Payments)

- **Standard**: Google's Agent Payments Protocol
- **Mandates**: Intent -> Cart -> Payment -> Receipt
- **Extension URI**: `https://github.com/google-agentic-commerce/ap2/v1`

## Related Documentation

- [AEX A2A Integration](../docs/a2a-integration/)
- [AP2 Integration](../docs/AP2_INTEGRATION.md)
- [AWS Deployment](../deploy/aws/README.md)
