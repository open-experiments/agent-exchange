# AEX + A2A Demo Call Flow

This document describes the complete call flow when running the demo orchestrator with AEX discovery.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              Docker Network (aex-network)                        │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  ┌─────────┐    ┌─────────────┐    ┌──────────────────┐    ┌──────────────────┐ │
│  │  User   │───▶│ Orchestrator│───▶│   AEX Gateway    │───▶│Provider Registry │ │
│  │         │    │  :8103      │    │     :8080        │    │     :8085        │ │
│  └─────────┘    └──────┬──────┘    └──────────────────┘    └────────┬─────────┘ │
│                        │                                            │           │
│                        │ A2A Protocol                    Registered │           │
│                        ▼                                            ▼           │
│       ┌────────────────────────────────────────────────────────────────┐        │
│       │                     Provider Agents                            │        │
│       │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐         │        │
│       │  │Legal Agent A │  │Legal Agent B │  │Legal Agent C │         │        │
│       │  │   :8100      │  │   :8101      │  │   :8102      │         │        │
│       │  │ Budget Tier  │  │Standard Tier │  │Premium Tier  │         │        │
│       │  │ $5 + $2/pg   │  │$15 + $0.5/pg │  │$30 + $0.2/pg │         │        │
│       │  └──────────────┘  └──────────────┘  └──────────────┘         │        │
│       └────────────────────────────────────────────────────────────────┘        │
│                                                                                  │
│  ┌─────────────────────────────────────────────────────────────────────────┐    │
│  │                         Supporting Services                              │    │
│  │  MongoDB:27017  │  Trust Broker:8086  │  Identity:8087  │  ...          │    │
│  └─────────────────────────────────────────────────────────────────────────┘    │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

## Complete Call Flow Sequence

### Phase 1: Agent Startup & Registration

When agents start, they automatically register with AEX:

```
┌──────────────┐          ┌─────────────┐          ┌──────────────────┐
│ Legal Agent  │          │ AEX Gateway │          │Provider Registry │
│   (A/B/C)    │          │   :8080     │          │     :8085        │
└──────┬───────┘          └──────┬──────┘          └────────┬─────────┘
       │                         │                          │
       │  POST /v1/providers     │                          │
       │  {                      │                          │
       │    name: "Legal Agent A"│                          │
       │    endpoint: "http://   │                          │
       │      legal-agent-a:8100"│                          │
       │    capabilities: [      │                          │
       │      "contract_review", │                          │
       │      "compliance_check",│                          │
       │      "risk_assessment"  │                          │
       │    ]                    │                          │
       │  }                      │                          │
       │────────────────────────▶│                          │
       │                         │  Store Provider          │
       │                         │─────────────────────────▶│
       │                         │                          │
       │                         │  provider_id             │
       │                         │◀─────────────────────────│
       │  201 Created            │                          │
       │  {provider_id: "prov_*"}│                          │
       │◀────────────────────────│                          │
       │                         │                          │
```

**Log Output:**
```
legal-agent-a  | 2025-01-03 02:58:30 - INFO - Registered with AEX: prov_83b1d3f3236dc93d
legal-agent-b  | 2025-01-03 02:58:31 - INFO - Registered with AEX: prov_a1c2d3e4f5g6h7i8
legal-agent-c  | 2025-01-03 02:58:32 - INFO - Registered with AEX: prov_b2c3d4e5f6g7h8i9
```

### Phase 2: User Request to Orchestrator

User sends a request via A2A protocol:

```bash
curl -X POST http://localhost:8103/a2a \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "message/send",
    "id": "test-1",
    "params": {
      "message": {
        "role": "user",
        "parts": [{"type": "text", "text": "Review this contract and check compliance"}]
      }
    }
  }'
```

### Phase 3: Task Decomposition (Claude API)

```
┌──────────────┐          ┌─────────────────┐
│ Orchestrator │          │  Claude API     │
│   :8103      │          │ (Anthropic)     │
└──────┬───────┘          └────────┬────────┘
       │                           │
       │  POST /messages           │
       │  {                        │
       │    model: "claude-sonnet-4│
       │      -20250514",          │
       │    messages: [user_req],  │
       │    tools: [decompose_tool]│
       │  }                        │
       │──────────────────────────▶│
       │                           │
       │  Tool Call Response:      │
       │  {subtasks: [             │
       │    {name: "contract_rev", │
       │     skill_tags: [         │
       │       "contract_review"   │
       │     ]},                   │
       │    {name: "compliance",   │
       │     skill_tags: [         │
       │       "compliance_check"  │
       │     ]}                    │
       │  ]}                       │
       │◀──────────────────────────│
       │                           │
```

**Log Output:**
```
orchestrator | 2025-01-03 02:58:45 - INFO - Decomposed into 2 subtasks
orchestrator | 2025-01-03 02:58:45 - INFO - Subtask: contract_review [contract_review]
orchestrator | 2025-01-03 02:58:45 - INFO - Subtask: compliance_check [compliance_check]
```

### Phase 4: AEX Provider Discovery (Key Step!)

For each subtask, orchestrator queries AEX to find providers:

```
┌──────────────┐          ┌─────────────┐          ┌──────────────────┐
│ Orchestrator │          │ AEX Gateway │          │Provider Registry │
│   :8103      │          │   :8080     │          │     :8085        │
└──────┬───────┘          └──────┬──────┘          └────────┬─────────┘
       │                         │                          │
       │  GET /v1/providers/     │                          │
       │    search?skills=       │                          │
       │    contract_review      │                          │
       │────────────────────────▶│                          │
       │                         │  Search by skill tags    │
       │                         │─────────────────────────▶│
       │                         │                          │
       │                         │  Query: capabilities     │
       │                         │    contains              │
       │                         │    "contract_review"     │
       │                         │                          │
       │                         │  [{                      │
       │                         │    provider_id: "prov_*",│
       │                         │    name: "Legal Agent A",│
       │                         │    endpoint: "http://    │
       │                         │      legal-agent-a:8100",│
       │                         │    trust_score: 0.85     │
       │                         │  }]                      │
       │                         │◀─────────────────────────│
       │  200 OK                 │                          │
       │  {providers: [...]}     │                          │
       │◀────────────────────────│                          │
       │                         │                          │
```

**Log Output:**
```
aex-gateway  | 2025-01-03 02:58:47 - GET /v1/providers/search?skills=contract_review - 200
orchestrator | 2025-01-03 02:58:47 - INFO - [AEX] Found provider prov_83b1d3f3236dc93d for task_1
orchestrator | 2025-01-03 02:58:47 - INFO - Selected: Legal Agent A (http://legal-agent-a:8100)
```

### Phase 5: Direct A2A Execution

Orchestrator sends task directly to selected agent via A2A:

```
┌──────────────┐                              ┌──────────────┐
│ Orchestrator │                              │ Legal Agent A│
│   :8103      │                              │   :8100      │
└──────┬───────┘                              └──────┬───────┘
       │                                             │
       │  POST /a2a                                  │
       │  {                                          │
       │    "jsonrpc": "2.0",                        │
       │    "method": "message/send",                │
       │    "params": {                              │
       │      "message": {                           │
       │        "role": "user",                      │
       │        "parts": [{                          │
       │          "type": "text",                    │
       │          "text": "Review contract..."       │
       │        }]                                   │
       │      }                                      │
       │    }                                        │
       │  }                                          │
       │─────────────────────────────────────────────▶
       │                                             │
       │                                    ┌────────┴────────┐
       │                                    │ Claude API Call │
       │                                    │ (Task Execution)│
       │                                    └────────┬────────┘
       │                                             │
       │  {                                          │
       │    "result": {                              │
       │      "status": "completed",                 │
       │      "result": {                            │
       │        "content": "Analysis: ..."           │
       │      }                                      │
       │    }                                        │
       │  }                                          │
       │◀─────────────────────────────────────────────
       │                                             │
```

**Log Output:**
```
legal-agent-a | 2025-01-03 02:58:48 - INFO - Received A2A request: message/send
legal-agent-a | 2025-01-03 02:58:50 - INFO - Processing task with Claude
legal-agent-a | 2025-01-03 02:58:55 - INFO - Task completed successfully
orchestrator  | 2025-01-03 02:58:55 - INFO - Received result from Legal Agent A
```

### Phase 6: Response Aggregation

```
┌──────────────┐          ┌─────────────────┐          ┌──────────┐
│ Orchestrator │          │  Claude API     │          │   User   │
│   :8103      │          │ (Aggregation)   │          │          │
└──────┬───────┘          └────────┬────────┘          └────┬─────┘
       │                           │                        │
       │  POST /messages           │                        │
       │  {combine subtask results}│                        │
       │──────────────────────────▶│                        │
       │                           │                        │
       │  Aggregated Response      │                        │
       │◀──────────────────────────│                        │
       │                           │                        │
       │  A2A Response with        │                        │
       │  combined results +       │                        │
       │  Agent Selection Summary  │                        │
       │───────────────────────────────────────────────────▶│
       │                           │                        │
```

## Complete End-to-End Flow Diagram

```
    ┌─────┐
    │User │
    └──┬──┘
       │ 1. A2A Request
       ▼
┌──────────────┐
│ Orchestrator │
│   :8103      │
└──────┬───────┘
       │
       │ 2. Task Decomposition
       ▼
┌──────────────┐
│  Claude API  │ ──▶ Returns: [subtask_1, subtask_2]
└──────────────┘
       │
       │ 3. For each subtask:
       ▼
┌──────────────┐     ┌──────────────────┐
│ AEX Gateway  │────▶│Provider Registry │
│   :8080      │     │     :8085        │
└──────────────┘     └──────────────────┘
       │
       │ 4. Returns matching providers by skill
       ▼
┌──────────────────────────────────────────────────────────────┐
│                    Provider Selection                        │
│  ┌─────────────────────────────────────────────────────────┐│
│  │ Skill: contract_review                                  ││
│  │ Found: Legal Agent A (trust: 0.85), Legal Agent B (0.80)││
│  │ Selected: Legal Agent A (highest trust + budget tier)   ││
│  └─────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────────────────────────────────────┐│
│  │ Skill: compliance_check                                 ││
│  │ Found: Legal Agent B (trust: 0.80), Legal Agent C (0.90)││
│  │ Selected: Legal Agent B (standard tier for compliance)  ││
│  └─────────────────────────────────────────────────────────┘│
└──────────────────────────────────────────────────────────────┘
       │
       │ 5. Direct A2A calls to selected agents
       ▼
┌──────────────────────────────────────────────────────────────┐
│                    A2A Execution                             │
│                                                              │
│   Orchestrator ──────────────────▶ Legal Agent A :8100       │
│       │         POST /a2a            │                       │
│       │         message/send         │ Claude processes      │
│       │                              ▼                       │
│       │                          Result returned             │
│       │                                                      │
│   Orchestrator ──────────────────▶ Legal Agent B :8101       │
│                 POST /a2a            │                       │
│                 message/send         │ Claude processes      │
│                                      ▼                       │
│                                  Result returned             │
└──────────────────────────────────────────────────────────────┘
       │
       │ 6. Aggregate results
       ▼
┌──────────────┐
│  Claude API  │ ──▶ Combined summary
└──────────────┘
       │
       │ 7. Return to user
       ▼
    ┌─────┐
    │User │  ◀── Final response with Agent Selection Summary
    └─────┘
```

## API Endpoints Used

| Step | Service | Endpoint | Method | Purpose |
|------|---------|----------|--------|---------|
| 1 | Provider Registry | `/v1/providers` | POST | Agent registration |
| 2 | Orchestrator | `/a2a` | POST | User request (A2A) |
| 3 | AEX Gateway | `/v1/providers/search` | GET | Provider discovery |
| 4 | Legal Agents | `/a2a` | POST | Task execution (A2A) |

## Environment Configuration

### Required for AEX Discovery

```yaml
# docker-compose.yml

aex-provider-registry:
  environment:
    ENVIRONMENT: "development"  # Enables HTTP URLs

legal-agent-a:
  environment:
    AGENT_HOSTNAME: legal-agent-a  # Docker network hostname
    AEX_GATEWAY_URL: http://aex-gateway:8080

orchestrator:
  environment:
    AEX_GATEWAY_URL: http://aex-gateway:8080
```

## Testing the Flow

### 1. Start all services
```bash
cd demo
docker-compose up -d
```

### 2. Verify agent registration
```bash
# Check registered providers
curl http://localhost:8085/v1/providers | jq

# Expected: 3 providers (legal-agent-a, b, c)
```

### 3. Test provider search
```bash
# Search by skill
curl "http://localhost:8085/v1/providers/search?skills=contract_review" | jq

# Expected: Providers with contract_review capability
```

### 4. Send test request to orchestrator
```bash
curl -X POST http://localhost:8103/a2a \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "message/send",
    "id": "test-1",
    "params": {
      "message": {
        "role": "user",
        "parts": [{"type": "text", "text": "Review this contract and check compliance"}]
      }
    }
  }'
```

### 5. Check logs for AEX discovery
```bash
# View orchestrator logs
docker logs aex-orchestrator 2>&1 | grep -E "(AEX|Found provider|Selected)"

# View gateway logs
docker logs aex-gateway 2>&1 | grep "providers/search"
```

## Key Points

1. **AEX is the Discovery Layer**: Orchestrator uses AEX `/v1/providers/search` to find agents by skill tags
2. **A2A is the Execution Layer**: Once discovered, agents communicate directly via A2A protocol
3. **HTTP in Development**: `ENVIRONMENT=development` allows HTTP URLs for Docker networking
4. **Docker Hostnames**: Agents register with Docker service names (not localhost)
5. **Skill-based Matching**: Provider capabilities are matched against subtask skill tags
