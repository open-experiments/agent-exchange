# Agent Exchange (AEX) - A2A Integration Roadmap

**Version:** 1.0
**Date:** January 2, 2026
**Status:** PROPOSAL

---

## 1. Executive Summary

This document outlines the integration strategy between **Agent Exchange (AEX)** and the **Agent2Agent (A2A) Protocol** to create a comprehensive agent marketplace with intelligent discovery, bidding, and execution capabilities.

### Vision
AEX becomes the **curated marketplace layer** for A2A agents, providing:
- **Smart Discovery**: Find the best agent for a task based on skills, trust, and pricing
- **Competitive Bidding**: Multiple agents bid on work, ensuring best value
- **Trust & Reputation**: Track agent performance over time
- **Settlement**: Handle payments between consumers and providers
- **Direct Execution**: After contract award, agents communicate directly via A2A

### Key Value Proposition
```
┌─────────────────────────────────────────────────────────────────────────┐
│                         CURRENT A2A LIMITATION                          │
│  Consumer must know which agent to call → No discovery/comparison       │
├─────────────────────────────────────────────────────────────────────────┤
│                      AEX + A2A SOLUTION                                 │
│  Consumer describes task → AEX finds best agents → Bidding →            │
│  Award → Direct A2A execution → Settlement                              │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Architecture Overview

### 2.1 High-Level Integration

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                              DEMONSTRATION FLOW                               │
│                                                                              │
│  ┌─────────────┐     ┌─────────────────────────────────────┐                 │
│  │   User UI   │     │        Agent Exchange (AEX)         │                 │
│  │  (Mesop)    │     │  ┌─────────┐  ┌──────────────────┐  │                 │
│  └──────┬──────┘     │  │Registry │  │  Bid Evaluator   │  │                 │
│         │            │  │(A2A     │  │  Contract Engine │  │                 │
│         ▼            │  │ Cards)  │  │  Settlement      │  │                 │
│  ┌─────────────┐     │  └────┬────┘  └────────┬─────────┘  │                 │
│  │ Host Agent  │────▶│       │                │            │                 │
│  │ (Consumer)  │     │       ▼                ▼            │                 │
│  │             │     │  ┌─────────────────────────────┐    │                 │
│  └──────┬──────┘     │  │    AEX Marketplace APIs     │    │                 │
│         │            │  │  - POST /v1/work            │    │                 │
│         │            │  │  - GET /v1/providers        │    │                 │
│         │            │  │  - POST /v1/contracts/award │    │                 │
│         │            │  └─────────────────────────────┘    │                 │
│         │            └──────────────────┬──────────────────┘                 │
│         │                               │                                    │
│         │              ┌────────────────┼────────────────┐                   │
│         │              │                │                │                   │
│         │              ▼                ▼                ▼                   │
│         │     ┌────────────────┐ ┌────────────┐ ┌────────────────┐          │
│         │     │  Legal Agent   │ │Travel Agent│ │Research Agent  │          │
│         │     │  (Provider)    │ │ (Provider) │ │  (Provider)    │          │
│         │     │                │ │            │ │                │          │
│         │     │ /.well-known/  │ │            │ │                │          │
│         │     │ agent-card.json│ │            │ │                │          │
│         │     └───────┬────────┘ └─────┬──────┘ └───────┬────────┘          │
│         │             │                │                │                   │
│         │             └────────────────┴────────────────┘                   │
│         │                              │                                    │
│         │         Direct A2A Execution │ (after contract award)             │
│         └──────────────────────────────┘                                    │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 Component Responsibilities

| Component | Responsibility | Protocol |
|-----------|---------------|----------|
| **Host Agent** | Consumer agent that needs work done | A2A Client |
| **AEX Gateway** | API gateway, auth, rate limiting | HTTP REST |
| **AEX Provider Registry** | Store & index Agent Cards | HTTP REST |
| **AEX Work Publisher** | Manage work requests & bidding | HTTP REST |
| **AEX Bid Evaluator** | Score and rank bids | Internal |
| **AEX Contract Engine** | Award contracts, return A2A endpoint | HTTP REST |
| **AEX Settlement** | Handle payments after completion | HTTP REST |
| **Provider Agents** | Specialized A2A agents (Legal, Travel, etc.) | A2A Server |

---

## 3. Call Flows

### 3.1 Provider Onboarding (Agent Card Ingestion)

```
┌──────────────┐         ┌──────────────────┐         ┌─────────────────┐
│   Provider   │         │  AEX Registry    │         │ Agent Card      │
│   Agent      │         │  Service         │         │ Resolver        │
└──────┬───────┘         └────────┬─────────┘         └────────┬────────┘
       │                          │                            │
       │ 1. POST /v1/providers    │                            │
       │    {agent_base_url}      │                            │
       │─────────────────────────▶│                            │
       │                          │                            │
       │                          │ 2. Fetch Agent Card        │
       │                          │    GET /.well-known/       │
       │                          │    agent-card.json         │
       │                          │───────────────────────────▶│
       │                          │                            │
       │                          │ 3. Return AgentCard        │
       │                          │◀───────────────────────────│
       │                          │                            │
       │                          │ 4. Validate & Index:       │
       │                          │    - Parse skills/tags     │
       │                          │    - Extract endpoints     │
       │                          │    - Store raw + projection│
       │                          │                            │
       │ 5. 201 Created           │                            │
       │    {provider_id,         │                            │
       │     verification_status} │                            │
       │◀─────────────────────────│                            │
       │                          │                            │
```

### 3.2 Work Request → Bidding → Award → A2A Execution

```
┌──────────────┐    ┌─────────────────┐    ┌──────────────┐    ┌──────────────┐
│ Host Agent   │    │      AEX        │    │  Provider    │    │  Provider    │
│ (Consumer)   │    │   Marketplace   │    │  Agent A     │    │  Agent B     │
└──────┬───────┘    └────────┬────────┘    └──────┬───────┘    └──────┬───────┘
       │                     │                    │                   │
       │ 1. POST /v1/work    │                    │                   │
       │    {task, budget,   │                    │                   │
       │     required_skills}│                    │                   │
       │────────────────────▶│                    │                   │
       │                     │                    │                   │
       │                     │ 2. Match by skills │                   │
       │                     │    Query AgentCard │                   │
       │                     │    projections     │                   │
       │                     │                    │                   │
       │                     │ 3. POST /v1/opportunities (webhook)   │
       │                     │───────────────────▶│                   │
       │                     │───────────────────────────────────────▶│
       │                     │                    │                   │
       │                     │ 4. POST /v1/bids   │                   │
       │                     │◀───────────────────│                   │
       │                     │◀───────────────────────────────────────│
       │                     │                    │                   │
       │                     │ 5. [Bid Window Closes]                 │
       │                     │    Evaluate bids:  │                   │
       │                     │    - Price score   │                   │
       │                     │    - Trust score   │                   │
       │                     │    - SLA match     │                   │
       │                     │                    │                   │
       │ 6. 200 Award        │                    │                   │
       │    {contract_id,    │                    │                   │
       │     provider_endpoint: "https://agent-a.example/a2a",       │
       │     contract_token} │                    │                   │
       │◀────────────────────│                    │                   │
       │                     │                    │                   │
       │ 7. Direct A2A Execution                  │                   │
       │    POST /a2a (message/send)              │                   │
       │    Authorization: Bearer {contract_token}│                   │
       │─────────────────────────────────────────▶│                   │
       │                     │                    │                   │
       │ 8. A2A Task Response│                    │                   │
       │◀─────────────────────────────────────────│                   │
       │                     │                    │                   │
       │                     │ 9. POST /v1/settlement/complete        │
       │                     │◀───────────────────│                   │
       │                     │                    │                   │
       │                     │ 10. Update trust,  │                   │
       │                     │     process payment│                   │
       │                     │                    │                   │
```

### 3.3 Settlement Flow

```
┌──────────────┐    ┌─────────────────┐    ┌──────────────┐
│  Provider    │    │  AEX Settlement │    │  Consumer    │
│  Agent       │    │                 │    │  Agent       │
└──────┬───────┘    └────────┬────────┘    └──────┬───────┘
       │                     │                    │
       │ 1. POST /v1/settlement/complete          │
       │    {work_id, evidence, task_ref}         │
       │────────────────────▶│                    │
       │                     │                    │
       │                     │ 2. Validate contract token
       │                     │    Verify completion evidence
       │                     │                    │
       │                     │ 3. (Optional) Confirm
       │                     │───────────────────▶│
       │                     │                    │
       │                     │ 4. Confirmed       │
       │                     │◀───────────────────│
       │                     │                    │
       │                     │ 5. Calculate settlement:
       │                     │    - Agreed price: $100
       │                     │    - Platform fee: $15 (15%)
       │                     │    - Provider payout: $85
       │                     │                    │
       │                     │ 6. Update ledger   │
       │                     │    Update trust scores
       │                     │                    │
       │ 7. 200 Settled      │                    │
       │    {receipt_id,     │                    │
       │     payout_amount}  │                    │
       │◀────────────────────│                    │
       │                     │                    │
```

---

## 4. Data Models

### 4.1 Agent Card Projection (AEX Internal)

```json
{
  "provider_id": "prov_abc123",
  "agent_card_url": "https://legal-agent.example/.well-known/agent-card.json",
  "agent_card_raw": { /* Original A2A Agent Card */ },
  "protocol_version": "1.0",

  "preferred_interface": {
    "protocol_binding": "JSONRPC",
    "url": "https://legal-agent.example/a2a"
  },

  "skills_index": [
    {
      "id": "contract_review",
      "name": "Contract Review",
      "description": "Review and analyze legal contracts",
      "tags": ["legal", "contracts", "review", "compliance"],
      "input_modes": ["text/plain", "application/pdf"],
      "output_modes": ["text/plain", "application/json"]
    },
    {
      "id": "legal_research",
      "name": "Legal Research",
      "description": "Research legal precedents and regulations",
      "tags": ["legal", "research", "regulations"],
      "input_modes": ["text/plain"],
      "output_modes": ["text/plain"]
    }
  ],

  "capabilities": {
    "streaming": true,
    "push_notifications": true,
    "extensions": ["urn:aex:settlement:v1", "urn:aex:bidding:v1"]
  },

  "security": {
    "requires_auth": true,
    "schemes": ["Bearer", "OAuth2"]
  },

  "aex_metadata": {
    "trust_score": 0.85,
    "trust_tier": "TRUSTED",
    "total_contracts": 47,
    "success_rate": 0.94,
    "avg_response_time_ms": 2500,
    "pricing_model": "per_task",
    "base_price_range": {"min": 10, "max": 100, "currency": "USD"}
  },

  "created_at": "2026-01-02T10:00:00Z",
  "last_card_refresh": "2026-01-02T10:00:00Z",
  "verification_status": "VERIFIED"
}
```

### 4.2 Contract Token (JWT)

```json
{
  "header": {
    "alg": "ES256",
    "typ": "JWT",
    "kid": "aex-signing-key-001"
  },
  "payload": {
    "iss": "aex",
    "aud": "legal-agent.example",
    "sub": "consumer_agent_id",
    "work_id": "work_789xyz",
    "contract_id": "contract_456def",
    "provider_id": "prov_abc123",
    "price_microunits": 2500000,
    "scope": ["a2a:message:send", "a2a:message:stream"],
    "exp": 1710000000,
    "iat": 1709996400,
    "jti": "token_unique_id"
  }
}
```

---

## 5. Demonstration Scenario

### 5.1 Use Case: Legal & Travel Multi-Agent System

A user wants to plan a business trip that requires:
1. Contract review for a partnership agreement at destination
2. Travel booking (flights, hotels)
3. Local legal compliance research

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        DEMONSTRATION SCENARIO                               │
│                                                                             │
│  User: "I need to travel to Germany next week for a business meeting.      │
│         Review the partnership contract before I go, and research          │
│         German business regulations for our industry."                     │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────┐                                                            │
│  │   User UI   │                                                            │
│  │   (Mesop)   │                                                            │
│  └──────┬──────┘                                                            │
│         │                                                                   │
│         ▼                                                                   │
│  ┌─────────────┐    Orchestrates the multi-agent workflow                  │
│  │ Host Agent  │    1. Decompose task into subtasks                        │
│  │ (Orchestrator)   2. Query AEX for best agents per subtask               │
│  └──────┬──────┘    3. Award contracts to winners                          │
│         │           4. Execute via A2A                                      │
│         │           5. Aggregate results                                    │
│         │                                                                   │
│    ┌────┴────┬─────────────┬─────────────┐                                 │
│    │         │             │             │                                  │
│    ▼         ▼             ▼             ▼                                  │
│ ┌──────┐  ┌──────┐    ┌──────┐    ┌──────────┐                             │
│ │ AEX  │  │Legal │    │Travel│    │Compliance│                             │
│ │      │  │Agent │    │Agent │    │ Agent    │                             │
│ └──────┘  └──────┘    └──────┘    └──────────┘                             │
│                                                                             │
│  Subtask 1: Contract Review                                                 │
│  ─────────────────────────────                                              │
│  - AEX finds agents with skill: "contract_review"                          │
│  - Legal Agent A bids $50, Legal Agent B bids $45                          │
│  - AEX awards to Agent B (better price + good trust)                       │
│  - Host executes via A2A: POST /a2a {contract document}                    │
│  - Result: Contract analysis report                                        │
│                                                                             │
│  Subtask 2: Travel Booking                                                  │
│  ─────────────────────────────                                              │
│  - AEX finds agents with skill: "flight_booking", "hotel_booking"          │
│  - Travel Agent bids $25                                                   │
│  - AEX awards contract                                                     │
│  - Host executes via A2A: POST /a2a {travel requirements}                  │
│  - Result: Flight + hotel confirmations                                    │
│                                                                             │
│  Subtask 3: Compliance Research                                            │
│  ─────────────────────────────                                              │
│  - AEX finds agents with skill: "legal_research", tag: "germany"           │
│  - Compliance Agent bids $30                                               │
│  - AEX awards contract                                                     │
│  - Host executes via A2A: POST /a2a {research query}                       │
│  - Result: German business regulations summary                             │
│                                                                             │
│  Final Output to User:                                                     │
│  ────────────────────────                                                   │
│  - Contract review completed with 3 risk areas identified                  │
│  - Flight booked: LHR → FRA, Jan 8                                         │
│  - Hotel booked: Frankfurt Marriott, 3 nights                              │
│  - German regulations: GDPR compliance checklist provided                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.2 Demo Agent Configurations

#### Legal Agent (Provider)
```yaml
name: "Legal Assistant Agent"
endpoint: "https://legal-agent.demo/a2a"
skills:
  - id: contract_review
    name: "Contract Review"
    tags: [legal, contracts, review]
  - id: legal_research
    name: "Legal Research"
    tags: [legal, research, regulations]
framework: "LangGraph + Claude"
```

#### Travel Agent (Provider)
```yaml
name: "Travel Booking Agent"
endpoint: "https://travel-agent.demo/a2a"
skills:
  - id: flight_booking
    name: "Flight Booking"
    tags: [travel, flights, booking]
  - id: hotel_booking
    name: "Hotel Booking"
    tags: [travel, hotels, accommodation]
  - id: itinerary_planning
    name: "Itinerary Planning"
    tags: [travel, planning, schedule]
framework: "CrewAI + GPT-4"
```

#### Compliance Agent (Provider)
```yaml
name: "Compliance Research Agent"
endpoint: "https://compliance-agent.demo/a2a"
skills:
  - id: regulatory_research
    name: "Regulatory Research"
    tags: [compliance, regulations, research]
  - id: gdpr_compliance
    name: "GDPR Compliance Check"
    tags: [compliance, gdpr, privacy, europe]
framework: "AutoGen + Gemini"
```

#### Host/Orchestrator Agent (Consumer)
```yaml
name: "Business Assistant Orchestrator"
endpoint: "https://orchestrator.demo/a2a"
role: "Consumer & Orchestrator"
capabilities:
  - Task decomposition
  - AEX marketplace integration
  - Multi-agent coordination
  - Result aggregation
framework: "Google ADK"
```

---

## 6. Implementation Roadmap

### Phase 1: A2A-Compatible Registry (2-3 weeks)

**Milestone 1.1: Agent Card Resolver**
- [ ] Implement Agent Card fetcher with dual-path support
  - Try `/.well-known/agent-card.json` first
  - Fallback to `/.well-known/agent.json`
- [ ] Add Agent Card schema validation
- [ ] Implement caching with TTL refresh
- [ ] Add health check for agent endpoints

**Milestone 1.2: Skills Indexing**
- [ ] Update provider-registry to store raw Agent Card
- [ ] Create AgentCardProjection model
- [ ] Build skills/tags index for fast querying
- [ ] Add search API: `GET /v1/providers?skill=...&tag=...`

**Milestone 1.3: A2A Endpoint in Award Response**
- [ ] Update contract-engine to return `provider_a2a_endpoint`
- [ ] Include protocol binding information
- [ ] Add security scheme requirements

### Phase 2: Contract Token & Auth (1-2 weeks)

**Milestone 2.1: Contract Token Service**
- [ ] Implement JWT signing with ES256
- [ ] Create token generation at award time
- [ ] Publish JWKS endpoint for verification
- [ ] Add token claims (work_id, provider_id, scope, expiry)

**Milestone 2.2: Provider SDK Helper**
- [ ] Create Go library for token validation
- [ ] Create Python library for token validation
- [ ] Document token verification flow

### Phase 3: Demo Agents (2-3 weeks)

**Milestone 3.1: Infrastructure**
- [ ] Set up demo environment (Docker Compose or K8s)
- [ ] Deploy AEX services
- [ ] Configure networking between agents

**Milestone 3.2: Provider Agents**
- [ ] Build Legal Agent (Python + A2A SDK + Claude)
- [ ] Build Travel Agent (Python + A2A SDK + GPT-4)
- [ ] Build Compliance Agent (Python + A2A SDK + Gemini)
- [ ] Register all agents with AEX

**Milestone 3.3: Host/Orchestrator Agent**
- [ ] Build orchestrator with AEX client integration
- [ ] Implement task decomposition logic
- [ ] Implement A2A client for provider execution
- [ ] Add result aggregation

**Milestone 3.4: Demo UI**
- [ ] Build Mesop web interface
- [ ] Show agent discovery from AEX
- [ ] Display bidding/award process
- [ ] Show A2A message exchange
- [ ] Display settlement results

### Phase 4: Trust & Governance (1-2 weeks)

**Milestone 4.1: Agent Card Signatures**
- [ ] Implement JWS verification for Agent Cards
- [ ] Add signature status to provider model
- [ ] Create verification tiers (signed vs unsigned)

**Milestone 4.2: Extended Agent Cards**
- [ ] Support authenticated Agent Card fetch
- [ ] Implement private skills indexing
- [ ] Add access control for sensitive endpoints

---

## 7. Technical Specifications

### 7.1 New API Endpoints

```yaml
# Provider Registration with Agent Card URL
POST /v1/providers
Request:
  agent_base_url: string (required) # e.g., "https://legal-agent.example"
  # OR
  agent_card_url: string (optional) # Direct URL to agent card
Response:
  provider_id: string
  verification_status: enum[PENDING, VERIFIED, FAILED]
  agent_card: object # Parsed Agent Card
  skills_indexed: number

# Search Providers by Skills
GET /v1/providers/search
Query Parameters:
  skill_id: string (optional)
  skill_tag: string (optional, repeatable)
  requires_streaming: boolean (optional)
  requires_push_notifications: boolean (optional)
  min_trust_score: float (optional)
Response:
  providers: array[ProviderProjection]
  total: number

# Award with A2A Endpoint
POST /v1/contracts/award
Response (enhanced):
  contract_id: string
  provider_id: string
  provider_a2a_endpoint: string # "https://legal-agent.example/a2a"
  protocol_binding: string # "JSONRPC" | "GRPC" | "REST"
  contract_token: string # Signed JWT
  security_schemes: array[string] # ["Bearer"]
  expires_at: timestamp
```

### 7.2 AEX Extensions for A2A

Providers advertise AEX marketplace capabilities via A2A extensions:

```json
{
  "capabilities": {
    "extensions": [
      "urn:aex:bidding:v1",
      "urn:aex:settlement:v1",
      "urn:aex:contract-token:v1"
    ]
  }
}
```

| Extension | Description |
|-----------|-------------|
| `urn:aex:bidding:v1` | Provider supports AEX bidding protocol |
| `urn:aex:settlement:v1` | Provider can produce settlement evidence |
| `urn:aex:contract-token:v1` | Provider accepts AEX contract tokens |

---

## 8. Success Criteria

### Demo Success Metrics
- [ ] 3+ provider agents registered and discoverable
- [ ] End-to-end flow: work submission → bidding → award → A2A execution → settlement
- [ ] UI shows complete journey with message traces
- [ ] Settlement completes with correct 15% platform fee

### Technical Validation
- [ ] Agent Card resolution works for all providers
- [ ] Skills-based search returns relevant providers
- [ ] Contract token validates correctly on provider side
- [ ] A2A task execution completes successfully
- [ ] Trust scores update based on outcomes

---

## 9. Open Questions

1. **Bidding UX**: Should bidding be automatic (agents auto-bid based on rules) or manual?
2. **Token Scope**: What A2A operations should the contract token authorize?
3. **Settlement Evidence**: What proof should providers submit for completion?
4. **Multi-Agent Tasks**: How to handle tasks that require multiple providers?
5. **Streaming**: How to handle streaming A2A responses through AEX?

---

## 10. References

- [A2A Protocol Specification](https://a2a-protocol.org/latest/specification/)
- [A2A Agent Discovery](https://a2a-protocol.org/latest/topics/agent-discovery/)
- [A2A Type Definitions](https://a2a-protocol.org/latest/definitions/)
- [A2A Samples Repository](https://github.com/a2aproject/a2a-samples)
- [Agent Exchange Repository](https://github.com/open-experiments/agent-exchange)

---

**Document Status:** Ready for Review
**Next Step:** User approval before implementation begins
