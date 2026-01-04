# Agent Exchange - A2A Integration Demo

This directory contains demo agents and a UI showcasing the integration between Agent Exchange (AEX) and the A2A Protocol. The demo features **three legal agents with tiered pricing** to demonstrate AEX's intelligent bid evaluation and selection.

## Overview

```
demo/
├── agents/
│   ├── common/              # Shared utilities for all agents
│   ├── legal-agent-a/       # Budget tier - Claude (fast, basic analysis)
│   ├── legal-agent-b/       # Standard tier - Claude (thorough analysis)
│   ├── legal-agent-c/       # Premium tier - Claude (exhaustive expert analysis)
│   └── orchestrator/        # Consumer agent that coordinates work
├── ui/                      # Mesop web interface
├── docker-compose.yml       # Run all demo components
└── README.md
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              DEMO FLOW                                      │
│                                                                             │
│  User ──► Mesop UI ──► Orchestrator ──► Agent Exchange ──► Provider Agents  │
│                              │                                   │          │
│                              │         (Discovery, Bidding)      │          │
│                              │                                   │          │
│                              └───────────── A2A ─────────────────┘          │
│                                      (Direct Execution)                     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Agents

### Legal Agents with Tiered Pricing

| Agent | Tier | LLM | Pricing | Best For | Port |
|-------|------|-----|---------|----------|------|
| **Legal Agent A** | Budget | Claude | $5 + $2/page | Short docs (1-5 pages) | 8100 |
| **Legal Agent B** | Standard | Claude | $15 + $0.50/page | Medium docs (5-30 pages) | 8101 |
| **Legal Agent C** | Premium | Claude | $30 + $0.20/page | Large docs (30+ pages) | 8102 |

### Pricing Examples

| Document Size | Agent A (Budget) | Agent B (Standard) | Agent C (Premium) | Best Choice |
|---------------|------------------|--------------------|--------------------|-------------|
| 3 pages | $11 | $16.50 | $30.60 | **Agent A** |
| 10 pages | $25 | $20 | $32 | **Agent B** |
| 20 pages | $45 | $25 | $34 | **Agent B** |
| 50 pages | $105 | $40 | $40 | **Agent B/C** |
| 100 pages | $205 | $65 | $50 | **Agent C** |

### Other Services

| Agent | LLM | Framework | Purpose | Port |
|-------|-----|-----------|---------|------|
| **Orchestrator** | Claude | LangGraph | Task decomposition, AEX integration | 8103 |
| **Demo UI** | - | Mesop | Web interface | 8501 |

## Quick Start

### Prerequisites

- Docker & Docker Compose
- Python 3.11+
- API Key: `ANTHROPIC_API_KEY`

> **Note**: No GPU required! All agents use Anthropic Claude API. Your machine only runs lightweight Python servers that make API calls.

### Run the Demo

```bash
# From project root
cd demo

# Set your API key
cp .env.example .env
# Edit .env with your Anthropic API key

# Start everything (AEX system + Demo agents + UI)
docker-compose up --build

# Access the demo
open http://localhost:8501
```

This single command builds and starts:
- All AEX core services (Gateway, Provider Registry, Bid Gateway, etc.)
- MongoDB database
- 3 Legal Agents (Budget, Standard, Premium)
- Orchestrator
- Demo UI

### Run Locally (Development)

```bash
# Terminal 1: Start AEX services
cd .. && make docker-up

# Terminal 2: Start Legal Agent A (Budget)
cd agents/legal-agent-a
pip install -r requirements.txt
python main.py

# Terminal 3: Start Legal Agent B (Standard)
cd agents/legal-agent-b
pip install -r requirements.txt
python main.py

# Terminal 4: Start Legal Agent C (Premium)
cd agents/legal-agent-c
pip install -r requirements.txt
python main.py

# Terminal 5: Start Demo UI
cd ui
pip install -r requirements.txt
mesop main.py
```

## Demo Scenario

**User Request**:
> "Review this 15-page partnership agreement and identify all risks and obligations."

**What Happens**:

1. **Orchestrator** analyzes request, identifies task:
   - Contract Review → requires `contract_review` skill
   - Document: 15 pages

2. **AEX Discovery**:
   ```
   GET /v1/providers/search?skill_tag=contract_review
   → Found: Legal Agent A (Budget), Legal Agent B (Standard), Legal Agent C (Premium)
   ```

3. **Bidding** (document-aware pricing):
   ```
   Legal Agent A (Budget):   $5 + (15 × $2)    = $35, confidence 75%
   Legal Agent B (Standard): $15 + (15 × $0.50) = $22.50, confidence 88%
   Legal Agent C (Premium):  $30 + (15 × $0.20) = $33, confidence 95%
   ```

4. **Evaluation** (balanced strategy):
   ```
   Score = 0.3×price + 0.3×trust + 0.15×confidence + 0.15×quality + 0.1×SLA

   Agent A: Low price but lower confidence → Score: 0.72
   Agent B: Best price/quality balance    → Score: 0.85 ← Winner
   Agent C: High confidence but pricier   → Score: 0.81
   ```

5. **A2A Execution**:
   ```
   POST https://legal-agent-b:8101/a2a
   Authorization: Bearer {contract_token}
   {"jsonrpc":"2.0","method":"message/send",...}
   ```

6. **Settlement**:
   ```
   Total: $22.50
   Platform Fee (15%): $3.38
   Provider Payout: $19.12
   ```

### Document Size Impact

For a **100-page contract**, selection would differ:
```
Legal Agent A: $5 + (100 × $2)    = $205 ← Too expensive
Legal Agent B: $15 + (100 × $0.50) = $65
Legal Agent C: $30 + (100 × $0.20) = $50  ← Best value
```

## Configuration

### Agent Configuration

Each agent has a `config.yaml` with tiered pricing:

```yaml
# agents/legal-agent-a/config.yaml (Budget Tier)
agent:
  name: "Budget Legal AI"
  description: "Fast, affordable contract review - basic analysis at low cost"
  version: "1.0.0"

server:
  host: "0.0.0.0"
  port: 8100

llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
  temperature: 0.5

characteristics:
  tier: "budget"
  response_style: "concise"
  detail_level: "basic"
  turnaround: "fast"

skills:
  - id: "contract_review"
    name: "Contract Review"
    description: "Quick contract review highlighting major issues"
    tags: ["legal", "contracts", "review"]

aex:
  enabled: true
  gateway_url: "http://localhost:8080"
  auto_register: true
  auto_bid: true
  pricing:
    base_rate: 5.00
    per_page_rate: 2.00
    max_pages_optimal: 5
    currency: "USD"
    description: "Best for quick reviews of short documents (1-5 pages)"
  bidding:
    confidence: 0.75
    estimated_time_minutes: 3
    max_document_pages: 10
```

### Environment Variables

```bash
# .env
# Claude - used by all agents
ANTHROPIC_API_KEY=sk-ant-...

# AEX Configuration
AEX_GATEWAY_URL=http://localhost:8080

# Agent Ports
LEGAL_AGENT_A_PORT=8100  # Budget: $5 + $2/page
LEGAL_AGENT_B_PORT=8101  # Standard: $15 + $0.50/page
LEGAL_AGENT_C_PORT=8102  # Premium: $30 + $0.20/page
ORCHESTRATOR_PORT=8103
```

## API Endpoints

### Agent Card (A2A Standard)

```bash
# Get agent capabilities
curl http://localhost:8100/.well-known/agent-card.json
```

### A2A JSON-RPC

```bash
# Send message to agent
curl -X POST http://localhost:8100/a2a \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer {token}" \
  -d '{
    "jsonrpc": "2.0",
    "method": "message/send",
    "id": "1",
    "params": {
      "message": {
        "role": "user",
        "parts": [{"type": "text", "text": "Review this contract..."}]
      }
    }
  }'
```

## Development

### Adding a New Agent

1. Copy an existing agent folder:
   ```bash
   cp -r agents/legal-agent-a agents/my-new-agent
   ```

2. Update `config.yaml` with new skills

3. Implement skill handlers in `skills/`

4. Update `docker-compose.yml`

5. Register with AEX

### Testing

```bash
# Test individual agent
cd agents/legal-agent-a
pytest tests/ -v

# Test full demo flow
cd demo
python scripts/test_e2e.py
```

## Troubleshooting

### Agent not discovered by AEX

1. Check agent is registered:
   ```bash
   curl http://localhost:8080/v1/providers
   ```

2. Verify agent card is accessible:
   ```bash
   curl http://localhost:8100/.well-known/agent-card.json
   ```

### Bidding not working

1. Check agent has `auto_bid: true` in config
2. Verify AEX gateway URL is correct
3. Check agent logs for errors

### A2A execution fails

1. Verify contract token is valid
2. Check agent is accepting the token
3. Review agent logs for authentication errors

## Related Documentation

- [A2A Integration Roadmap](../documents/a2a-integration/AEX_A2A_INTEGRATION_ROADMAP.md)
- [Value Proposition](../documents/a2a-integration/AEX_A2A_VALUE_PROPOSITION.md)
- [Call Flow Diagrams](../documents/a2a-integration/diagrams/)
