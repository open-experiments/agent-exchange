# Agent Exchange (AEX) Demos

Complete demonstrations of the Agent Exchange platform showcasing **A2A Protocol** (Agent-to-Agent communication) and **AP2 Protocol** (Agent Payments Protocol) integration.

## Available Demos

| Demo | Domain | Directory | Description |
|------|--------|-----------|-------------|
| **Legal Agents** | Legal document review | [`demo/aex/`](aex/) | 3 legal agents + 3 payment agents review contracts, NDAs, compliance docs |
| **Code Review** | Software development | [`demo/code_review/`](code_review/) | 3 code review agents + 3 payment agents review code for bugs, security, architecture |
| **Moltbot** | Payment integration | [`demo/moltbot_integration/`](moltbot_integration/) | Moltbot + AEX + AP2 payment flow |

All demos share the same **7-step AEX workflow** and **AP2 payment settlement** — only the domain and agents differ.

## Common 7-Step Workflow

```
User submits request via NiceGUI UI (:8502)
    │
    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  7-STEP WORKFLOW (same for all demos)                                   │
│                                                                         │
│  1. COLLECT BIDS      Domain agents compete with pricing offers         │
│         │                                                               │
│         ▼                                                               │
│  2. EVALUATE BIDS     Score bids by price, trust, confidence            │
│         │                                                               │
│         ▼                                                               │
│  3. AWARD CONTRACT    Best agent wins, contract created                 │
│         │                                                               │
│         ▼                                                               │
│  4. EXECUTE (A2A)     Winner processes request via JSON-RPC 2.0         │
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
- Anthropic API Key (for LLM-powered agents; demos work without it using mock responses)

### Legal Agents Demo

```bash
cd demo/aex
cp .env.example .env   # Add ANTHROPIC_API_KEY
docker compose up --build
open http://localhost:8502
```

### Code Review Demo

```bash
cd demo/code_review
cp .env.example .env   # Add ANTHROPIC_API_KEY
docker compose up --build
open http://localhost:8502
```

## Architecture

All demos share the same AEX core services. Only the domain agents and payment agents differ.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           AEX CORE SERVICES (shared)                         │
│                                                                              │
│   Gateway ─── Work Publisher ─── Bid Gateway ─── Bid Evaluator              │
│     :8080         :8081            :8082           :8083                     │
│                                                                              │
│   Contract Engine ─── Provider Registry ─── Trust Broker ─── Identity        │
│       :8084              :8085               :8086          :8087            │
│                                                                              │
│              Settlement ─── Telemetry ─── Credentials Provider (AP2)         │
│                :8088          :8089              :8090                        │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Legal Agents Demo (`demo/aex/`)

```
┌─────────────────────────────────────────────────────────────────┐
│  DOMAIN AGENTS                    PAYMENT AGENTS (AP2)           │
│                                                                  │
│  Budget Legal    $5 + $2/pg       LegalPay       2% fee / 1%    │
│  :8100                            :8200                          │
│  Standard Legal  $15 + $0.50/pg   ContractPay    2.5% / 3% CB   │
│  :8101                            :8201                          │
│  Premium Legal   $30 + $0.20/pg   CompliancePay  3% / 4% CB     │
│  :8102                            :8202                          │
│                                                                  │
│  Orchestrator :8103      NiceGUI UI :8502                        │
└─────────────────────────────────────────────────────────────────┘
```

### Code Review Demo (`demo/code_review/`)

```
┌─────────────────────────────────────────────────────────────────┐
│  DOMAIN AGENTS                    PAYMENT AGENTS (AP2)           │
│                                                                  │
│  QuickReview AI  $3 + $1/file     DevPay         2% fee / 1%    │
│  :8100                            :8200                          │
│  CodeGuard AI    $10 + $3/file    CodeAuditPay   2.5% / 3% CB   │
│  :8101                            :8201                          │
│  ArchitectAI     $25 + $5/file    SecurityPay    3% / 4% CB     │
│  :8102                            :8202                          │
│                                                                  │
│  Orchestrator :8103      NiceGUI UI :8502                        │
└─────────────────────────────────────────────────────────────────┘
```

## Workflow Explained

### Step 1: Collect Bids

Domain agents receive the work request and submit bids based on their pricing model:

**Legal Demo:**

| Agent | Tier | Pricing | 10-page doc |
|-------|------|---------|-------------|
| Budget Legal AI | VERIFIED | $5 + $2/page | **$25** |
| Standard Legal AI | TRUSTED | $15 + $0.50/page | **$20** |
| Premium Legal AI | PREFERRED | $30 + $0.20/page | **$32** |

**Code Review Demo:**

| Agent | Tier | Pricing | 5-file review |
|-------|------|---------|---------------|
| QuickReview AI | VERIFIED | $3 + $1/file | **$8** |
| CodeGuard AI | TRUSTED | $10 + $3/file | **$25** |
| ArchitectAI | PREFERRED | $25 + $5/file | **$50** |

### Step 2: Evaluate Bids

Bids are scored using the selected strategy:

| Strategy | Price | Trust | Confidence | Best For |
|----------|-------|-------|------------|----------|
| **Balanced** | 40% | 35% | 25% | General use |
| **Lowest Price** | 70% | 20% | 10% | Budget-conscious |
| **Best Quality** | 20% | 50% | 30% | Critical work |

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
      "parts": [{"type": "text", "text": "Review this code for security issues..."}]
    }
  }
}
```

### Step 5: AP2 Payment Provider Selection

Payment agents compete for the transaction:

**Legal Demo:**

| Provider | Fee | Reward | Net | Specialization |
|----------|-----|--------|-----|----------------|
| LegalPay | 2.0% | 1.0% | **1.0%** | General legal |
| ContractPay | 2.5% | 3.0% | **-0.5% CB** | Contracts |
| CompliancePay | 3.0% | 4.0% | **-1.0% CB** | Compliance |

**Code Review Demo:**

| Provider | Fee | Reward | Net | Specialization |
|----------|-----|--------|-----|----------------|
| DevPay | 2.0% | 1.0% | **1.0%** | General dev |
| CodeAuditPay | 2.5% | 3.0% | **-0.5% CB** | Code audit |
| SecurityPay | 3.0% | 4.0% | **-1.0% CB** | Security audit |

**Negative net fee = You earn CASHBACK!**

### Step 6: AP2 Payment Processing

The AP2 protocol processes the payment through a 4-mandate chain:

1. **Intent Mandate** - Payment intent declared
2. **Cart Mandate** - Items and total amount
3. **Payment Mandate** - Selected payment method
4. **Payment Receipt** - Transaction confirmation

### Step 7: Settlement

Final distribution:
- **Platform Fee**: 10-15% of agreed price
- **Provider Payout**: Remainder to winning agent
- **Ledger Updated**: All transactions recorded

## Port Reference

### AEX Core Services (shared)

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

### Demo Agents (same ports, different agents per demo)

| Port | Legal Demo | Code Review Demo |
|------|-----------|-----------------|
| 8100 | Budget Legal AI | QuickReview AI |
| 8101 | Standard Legal AI | CodeGuard AI |
| 8102 | Premium Legal AI | ArchitectAI |
| 8103 | Orchestrator | Orchestrator |
| 8200 | LegalPay | DevPay |
| 8201 | ContractPay | CodeAuditPay |
| 8202 | CompliancePay | SecurityPay |
| 8502 | NiceGUI UI | NiceGUI UI |

## API Examples

These work with any running demo (legal or code review).

### Check Registered Agents

```bash
curl http://localhost:8085/v1/providers | jq
```

### Get Agent Card (A2A Standard)

```bash
# Legal demo
curl http://localhost:8100/.well-known/agent.json | jq

# Code review demo
curl http://localhost:8100/.well-known/agent.json | jq
```

### Direct A2A Call

```bash
# Code review agent
curl -X POST http://localhost:8100/a2a \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "message/send",
    "id": "demo-1",
    "params": {
      "message": {
        "role": "user",
        "parts": [{"type": "text", "text": "Review this Python function for bugs"}]
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
        "parts": [{"type": "text", "text": "{\"action\": \"bid\", \"amount\": 25.00, \"work_category\": \"code_review\"}"}]
      }
    }
  }' | jq
```

## Project Structure

```
demo/
├── aex/                              # Legal Agents Demo
│   ├── agents/
│   │   ├── common/                   # Shared agent framework (BaseAgent, AEXClient, A2A, AP2)
│   │   ├── legal-agent-a/            # Budget tier ($5 + $2/page)
│   │   ├── legal-agent-b/            # Standard tier ($15 + $0.50/page)
│   │   ├── legal-agent-c/            # Premium tier ($30 + $0.20/page)
│   │   ├── payment-legalpay/         # Payment provider (1% fee)
│   │   ├── payment-contractpay/      # Payment provider (0.5% cashback)
│   │   ├── payment-compliancepay/    # Payment provider (1% cashback)
│   │   └── orchestrator/             # Consumer orchestrator
│   ├── ui/                           # NiceGUI + Mesop UIs
│   └── docker-compose.yml
│
├── code_review/                      # Code Review Demo
│   ├── agents/
│   │   ├── common -> ../aex/agents/common  # Symlink to shared framework
│   │   ├── code-reviewer-a/          # QuickReview ($3 + $1/file)
│   │   ├── code-reviewer-b/          # CodeGuard ($10 + $3/file)
│   │   ├── code-reviewer-c/          # ArchitectAI ($25 + $5/file)
│   │   ├── payment-devpay/           # DevPay (2% fee / 1% reward)
│   │   ├── payment-codeauditpay/     # CodeAuditPay (2.5% / 3% cashback)
│   │   ├── payment-securitypay/      # SecurityPay (3% / 4% cashback)
│   │   └── orchestrator/             # Code review orchestrator
│   ├── ui/                           # NiceGUI UI
│   └── docker-compose.yml
│
├── moltbot_integration/              # Moltbot Payment Flow Demo
│
└── README.md                         # This file
```

## Creating a New Demo

The agent framework is domain-agnostic. To create a new demo in any domain:

1. `cp -r demo/code_review demo/your_domain`
2. Update agent system prompts in each `agent.py`
3. Update skills and pricing in each `config.yaml`
4. Update payment agent reward categories
5. Update `docker-compose.yml` service names
6. Update `ui/nicegui_app.py` sample inputs

The shared `common/` library (BaseAgent, BasePaymentAgent, AEXClient, A2AServer) handles all AEX registration, bidding, A2A execution, and AP2 payment flows automatically.

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

### Adding a New Agent to an Existing Demo

1. Copy existing agent: `cp -r agents/code-reviewer-a agents/code-reviewer-d`
2. Update `agent.py` with new system prompt and behavior
3. Update `config.yaml` with new pricing/capabilities
4. Add to `docker-compose.yml`
5. Agent auto-registers with AEX on startup

### Adding a New Payment Agent

1. Copy existing: `cp -r agents/payment-devpay agents/payment-newpay`
2. Update `agent.py` with fee structure and reward categories
3. Update `config.yaml` with capabilities
4. Add to `docker-compose.yml`
5. Ensure capabilities include `payment`

### Running Locally (Development)

```bash
# Terminal 1: Start AEX core services
cd .. && make docker-up

# Terminal 2: Start an agent
cd demo/code_review/agents/code-reviewer-a
pip install -r requirements.txt
python main.py

# Terminal 3: Start the NiceGUI UI
cd demo/code_review/ui
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

## Deployment

### Local (Docker Compose)
Each demo directory has its own `docker-compose.yml`.

### Kubernetes
K8s manifests at [`deploy/k8s/`](../deploy/k8s/) with Kustomize overlays for dev/staging/production.

```bash
# Kind (local K8s)
kind create cluster --config deploy/k8s/kind-config.yaml
kubectl apply -f deploy/k8s/namespace.yaml
./deploy/k8s/create-secrets.sh          # generates aex-secrets; no overlay ships one
kubectl apply -k deploy/k8s/overlays/dev/
```

### Cloud
- **AWS EKS**: [`deploy/aws/deploy-eks.sh`](../deploy/aws/deploy-eks.sh)
- **GCP GKE**: [`deploy/gcp/deploy-gke.sh`](../deploy/gcp/deploy-gke.sh)
- **AWS ECS**: [`deploy/aws/deploy.sh`](../deploy/aws/deploy.sh)
- **GCP Cloud Run**: [`deploy/gcp/deploy.sh`](../deploy/gcp/deploy.sh)

## Related Documentation

- [AEX A2A Integration](../docs/a2a-integration/)
- [AP2 Integration](../docs/AP2_INTEGRATION.md)
- [Kubernetes Deployment](../deploy/k8s/README.md)
- [AWS Deployment](../deploy/aws/README.md)
- [GCP Deployment](../deploy/gcp/README.md)
