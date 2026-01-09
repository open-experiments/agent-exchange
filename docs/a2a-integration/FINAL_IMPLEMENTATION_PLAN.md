# AEX + A2A Integration - Final Implementation Plan

**Version:** 1.0 | **Date:** January 2, 2026 | **Status:** APPROVED

---

## 1. Project Overview

### 1.1 Objective
Integrate Agent Exchange (AEX) with the A2A Protocol to create an intelligent agent marketplace with:
- **Smart Discovery**: A2A Agent Card-based provider registration
- **Competitive Bidding**: LLM-assisted bid evaluation
- **Trust & Reputation**: Performance-based scoring
- **Direct Execution**: A2A protocol for agent-to-agent communication
- **Settlement**: Automated payment with 15% platform fee

### 1.2 Decisions Summary

| Decision | Choice |
|----------|--------|
| **Demo Domains** | Legal + Travel |
| **LLMs** | Claude, GPT-4, Gemini (one per agent) |
| **Agent Framework** | LangGraph (consistent across all agents) |
| **Bidding** | LLM-assisted evaluation (simulated for demo) |
| **Primary Deployment** | Google Cloud Platform (GCP) |
| **Secondary Deployment** | AWS (future) |

---

## 2. Demo Architecture

### 2.1 Agent Landscape

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           DEMO ARCHITECTURE                                  │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                         USER INTERFACE                               │   │
│  │                      (Mesop Web Application)                         │   │
│  └───────────────────────────────┬─────────────────────────────────────┘   │
│                                  │                                          │
│                                  ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    ORCHESTRATOR AGENT (Consumer)                     │   │
│  │                                                                      │   │
│  │  Framework: LangGraph          LLM: Claude                          │   │
│  │  Role: Task decomposition, AEX integration, Result aggregation      │   │
│  │                                                                      │   │
│  │  Capabilities:                                                       │   │
│  │  • Parse user requests                                              │   │
│  │  • Identify required skills                                         │   │
│  │  • Submit work to AEX                                               │   │
│  │  • Execute A2A calls to providers                                   │   │
│  │  • Aggregate and present results                                    │   │
│  └───────────────────────────────┬─────────────────────────────────────┘   │
│                                  │                                          │
│              ┌───────────────────┼───────────────────┐                     │
│              │                   │                   │                      │
│              ▼                   ▼                   ▼                      │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐            │
│  │   AGENT EXCHANGE │  │                 │  │                 │            │
│  │   (Marketplace)  │  │                 │  │                 │            │
│  │                  │  │                 │  │                 │            │
│  │ • Agent Registry │  │                 │  │                 │            │
│  │ • Bid Evaluation │  │                 │  │                 │            │
│  │ • Contract Award │  │                 │  │                 │            │
│  │ • Settlement     │  │                 │  │                 │            │
│  └────────┬─────────┘  │                 │  │                 │            │
│           │            │                 │  │                 │            │
│           │ Discovery  │                 │  │                 │            │
│           │ & Bidding  │                 │  │                 │            │
│           ▼            │                 │  │                 │            │
│  ┌─────────────────────┴─────────────────┴──┴─────────────────┐            │
│  │                    PROVIDER AGENTS                          │            │
│  │                                                             │            │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────┐ │            │
│  │  │  LEGAL AGENT    │  │  TRAVEL AGENT   │  │ LEGAL AGENT │ │            │
│  │  │  (Provider A)   │  │  (Provider B)   │  │ (Provider C)│ │            │
│  │  │                 │  │                 │  │             │ │            │
│  │  │ LLM: Claude     │  │ LLM: GPT-4      │  │ LLM: Gemini │ │            │
│  │  │ Framework:      │  │ Framework:      │  │ Framework:  │ │            │
│  │  │ LangGraph       │  │ LangGraph       │  │ LangGraph   │ │            │
│  │  │                 │  │                 │  │             │ │            │
│  │  │ Skills:         │  │ Skills:         │  │ Skills:     │ │            │
│  │  │ • contract_     │  │ • flight_       │  │ • contract_ │ │            │
│  │  │   review        │  │   booking       │  │   review    │ │            │
│  │  │ • legal_        │  │ • hotel_        │  │ • compliance│ │            │
│  │  │   research      │  │   booking       │  │   _check    │ │            │
│  │  │ • compliance    │  │ • itinerary_    │  │             │ │            │
│  │  │                 │  │   planning      │  │             │ │            │
│  │  └─────────────────┘  └─────────────────┘  └─────────────┘ │            │
│  │                                                             │            │
│  │  All agents expose: /.well-known/agent-card.json           │            │
│  │  All agents implement: A2A JSON-RPC Server                 │            │
│  └─────────────────────────────────────────────────────────────┘            │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 Agent Specifications

#### Legal Agent A (Claude)
```yaml
name: "Legal Counsel Agent"
description: "Expert contract review and legal document analysis"
endpoint: "https://legal-agent-a.aex-demo.example/a2a"
llm: "claude-3-5-sonnet"
framework: "LangGraph"
skills:
  - id: "contract_review"
    name: "Contract Review"
    description: "Analyze contracts for risks, obligations, and terms"
    tags: ["legal", "contracts", "review", "risk-analysis"]
    input_modes: ["text/plain", "application/pdf"]
    output_modes: ["text/plain", "application/json"]
  - id: "legal_research"
    name: "Legal Research"
    description: "Research case law, regulations, and legal precedents"
    tags: ["legal", "research", "regulations", "case-law"]
pricing:
  base_rate: 25.00  # USD per task
  complexity_multiplier: true
```

#### Travel Agent (GPT-4)
```yaml
name: "Travel Concierge Agent"
description: "Comprehensive travel planning and booking assistance"
endpoint: "https://travel-agent.aex-demo.example/a2a"
llm: "gpt-4-turbo"
framework: "LangGraph"
skills:
  - id: "flight_booking"
    name: "Flight Booking"
    description: "Search and book optimal flight itineraries"
    tags: ["travel", "flights", "booking", "airlines"]
    input_modes: ["text/plain", "application/json"]
    output_modes: ["text/plain", "application/json"]
  - id: "hotel_booking"
    name: "Hotel Booking"
    description: "Find and reserve accommodations"
    tags: ["travel", "hotels", "accommodation", "booking"]
  - id: "itinerary_planning"
    name: "Itinerary Planning"
    description: "Create comprehensive travel itineraries"
    tags: ["travel", "planning", "itinerary", "schedule"]
pricing:
  base_rate: 15.00
  per_booking_fee: 5.00
```

#### Legal Agent B (Gemini)
```yaml
name: "Compliance & Research Agent"
description: "Regulatory compliance and international law research"
endpoint: "https://legal-agent-b.aex-demo.example/a2a"
llm: "gemini-1.5-pro"
framework: "LangGraph"
skills:
  - id: "contract_review"
    name: "Contract Review"
    description: "Review contracts with focus on international compliance"
    tags: ["legal", "contracts", "review", "international"]
  - id: "compliance_check"
    name: "Compliance Check"
    description: "Verify regulatory compliance across jurisdictions"
    tags: ["compliance", "regulations", "gdpr", "international"]
pricing:
  base_rate: 20.00
```

### 2.3 Demo Scenario Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         DEMO SCENARIO                                        │
│                                                                             │
│  USER REQUEST:                                                              │
│  "I'm traveling to Berlin next week for a business meeting. I need to:     │
│   1. Review the partnership contract before I go                           │
│   2. Book flights and hotel                                                │
│   3. Understand German business regulations"                               │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  STEP 1: TASK DECOMPOSITION (Orchestrator)                                 │
│  ─────────────────────────────────────────                                  │
│  Orchestrator analyzes request and identifies 3 subtasks:                  │
│                                                                             │
│  Task A: Contract Review                                                   │
│    └─ Required skills: ["contract_review", "legal"]                        │
│                                                                             │
│  Task B: Travel Booking                                                    │
│    └─ Required skills: ["flight_booking", "hotel_booking"]                 │
│                                                                             │
│  Task C: Compliance Research                                               │
│    └─ Required skills: ["compliance_check", "regulations", "germany"]      │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  STEP 2: AGENT DISCOVERY (AEX)                                             │
│  ─────────────────────────────                                              │
│                                                                             │
│  Task A Query: GET /v1/providers/search?skill_tag=contract_review          │
│    └─ Matched: Legal Agent A (Claude), Legal Agent B (Gemini)              │
│                                                                             │
│  Task B Query: GET /v1/providers/search?skill_tag=flight_booking           │
│    └─ Matched: Travel Agent (GPT-4)                                        │
│                                                                             │
│  Task C Query: GET /v1/providers/search?skill_tag=compliance_check         │
│    └─ Matched: Legal Agent B (Gemini)                                      │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  STEP 3: BIDDING (AEX + LLM Evaluation)                                    │
│  ──────────────────────────────────────                                     │
│                                                                             │
│  Task A - Contract Review:                                                 │
│  ┌──────────────────┬─────────┬───────────┬───────────┬─────────┐         │
│  │ Provider         │ Price   │ Trust     │ Conf.     │ Score   │         │
│  ├──────────────────┼─────────┼───────────┼───────────┼─────────┤         │
│  │ Legal Agent A    │ $30     │ 0.92      │ 0.95      │ 0.87    │ ← WIN  │
│  │ Legal Agent B    │ $25     │ 0.78      │ 0.85      │ 0.76    │         │
│  └──────────────────┴─────────┴───────────┴───────────┴─────────┘         │
│                                                                             │
│  Task B - Travel Booking:                                                  │
│  ┌──────────────────┬─────────┬───────────┬───────────┬─────────┐         │
│  │ Provider         │ Price   │ Trust     │ Conf.     │ Score   │         │
│  ├──────────────────┼─────────┼───────────┼───────────┼─────────┤         │
│  │ Travel Agent     │ $20     │ 0.88      │ 0.90      │ 0.85    │ ← WIN  │
│  └──────────────────┴─────────┴───────────┴───────────┴─────────┘         │
│                                                                             │
│  Task C - Compliance Research:                                             │
│  ┌──────────────────┬─────────┬───────────┬───────────┬─────────┐         │
│  │ Provider         │ Price   │ Trust     │ Conf.     │ Score   │         │
│  ├──────────────────┼─────────┼───────────┼───────────┼─────────┤         │
│  │ Legal Agent B    │ $20     │ 0.78      │ 0.88      │ 0.80    │ ← WIN  │
│  └──────────────────┴─────────┴───────────┴───────────┴─────────┘         │
│                                                                             │
│  LLM Bid Evaluator considers:                                              │
│  • Price competitiveness                                                   │
│  • Provider trust score history                                            │
│  • Skill match quality                                                     │
│  • Response time SLA                                                       │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  STEP 4: CONTRACT AWARD (AEX)                                              │
│  ────────────────────────────                                               │
│                                                                             │
│  Three contracts awarded:                                                  │
│                                                                             │
│  Contract 1: Legal Agent A for Contract Review                             │
│    └─ A2A Endpoint: https://legal-agent-a.aex-demo.example/a2a            │
│    └─ Contract Token: eyJhbGciOiJFUzI1NiIs...                             │
│                                                                             │
│  Contract 2: Travel Agent for Travel Booking                               │
│    └─ A2A Endpoint: https://travel-agent.aex-demo.example/a2a             │
│    └─ Contract Token: eyJhbGciOiJFUzI1NiIs...                             │
│                                                                             │
│  Contract 3: Legal Agent B for Compliance Research                         │
│    └─ A2A Endpoint: https://legal-agent-b.aex-demo.example/a2a            │
│    └─ Contract Token: eyJhbGciOiJFUzI1NiIs...                             │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  STEP 5: A2A EXECUTION (Direct Agent-to-Agent)                             │
│  ─────────────────────────────────────────────                              │
│                                                                             │
│  Orchestrator executes in parallel:                                        │
│                                                                             │
│  POST https://legal-agent-a.aex-demo.example/a2a                           │
│  Authorization: Bearer {contract_token_1}                                  │
│  Body: {"jsonrpc":"2.0","method":"message/send","params":{                 │
│    "message":{"role":"user","parts":[{"text":"Review this contract..."}]}  │
│  }}                                                                         │
│                                                                             │
│  POST https://travel-agent.aex-demo.example/a2a                            │
│  Authorization: Bearer {contract_token_2}                                  │
│  Body: {"jsonrpc":"2.0","method":"message/send","params":{                 │
│    "message":{"role":"user","parts":[{"text":"Book flight to Berlin..."}]} │
│  }}                                                                         │
│                                                                             │
│  POST https://legal-agent-b.aex-demo.example/a2a                           │
│  Authorization: Bearer {contract_token_3}                                  │
│  Body: {"jsonrpc":"2.0","method":"message/send","params":{                 │
│    "message":{"role":"user","parts":[{"text":"Research German regs..."}]}  │
│  }}                                                                         │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  STEP 6: SETTLEMENT (AEX)                                                  │
│  ────────────────────────                                                   │
│                                                                             │
│  Total Cost: $70.00                                                        │
│  ┌─────────────────┬──────────┬──────────────┬────────────────┐           │
│  │ Provider        │ Amount   │ Platform Fee │ Provider Payout│           │
│  ├─────────────────┼──────────┼──────────────┼────────────────┤           │
│  │ Legal Agent A   │ $30.00   │ $4.50        │ $25.50         │           │
│  │ Travel Agent    │ $20.00   │ $3.00        │ $17.00         │           │
│  │ Legal Agent B   │ $20.00   │ $3.00        │ $17.00         │           │
│  ├─────────────────┼──────────┼──────────────┼────────────────┤           │
│  │ TOTAL           │ $70.00   │ $10.50 (15%) │ $59.50         │           │
│  └─────────────────┴──────────┴──────────────┴────────────────┘           │
│                                                                             │
│  Trust scores updated based on outcomes.                                   │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  FINAL OUTPUT TO USER:                                                     │
│  ─────────────────────                                                      │
│                                                                             │
│  "Here's your complete business trip preparation:                          │
│                                                                             │
│   📋 CONTRACT REVIEW:                                                      │
│   The partnership agreement has 3 key risk areas:                          │
│   1. Liability clause (Section 4.2) needs revision                         │
│   2. IP ownership unclear in Appendix B                                    │
│   3. Termination notice period too short (30 days)                         │
│                                                                             │
│   ✈️ TRAVEL BOOKED:                                                        │
│   Flight: LHR → BER, Jan 8, 09:15 (BA 984) - €245                         │
│   Return: BER → LHR, Jan 11, 18:30 (BA 987) - €198                         │
│   Hotel: Hotel Adlon Kempinski, 3 nights - €890                            │
│                                                                             │
│   📜 GERMAN REGULATIONS:                                                   │
│   Key compliance requirements for your industry:                           │
│   1. GDPR applies - ensure data processing agreement                       │
│   2. German Commercial Code (HGB) partnership rules                        │
│   3. No specific licensing required for your sector"                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 3. GCP Deployment Architecture

### 3.1 Infrastructure Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        GCP DEPLOYMENT ARCHITECTURE                          │
│                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐ │
│  │                        Google Cloud Platform                          │ │
│  │                                                                       │ │
│  │  ┌─────────────────────────────────────────────────────────────────┐ │ │
│  │  │                     Cloud Load Balancer                         │ │ │
│  │  │                    (Global HTTP/HTTPS)                          │ │ │
│  │  └─────────────────────────────┬───────────────────────────────────┘ │ │
│  │                                │                                     │ │
│  │              ┌─────────────────┼─────────────────┐                   │ │
│  │              │                 │                 │                   │ │
│  │              ▼                 ▼                 ▼                   │ │
│  │  ┌───────────────┐ ┌───────────────┐ ┌───────────────────────────┐  │ │
│  │  │  Cloud Run    │ │  Cloud Run    │ │      Cloud Run            │  │ │
│  │  │  (AEX APIs)   │ │  (Demo UI)    │ │   (Provider Agents)       │  │ │
│  │  │               │ │               │ │                           │  │ │
│  │  │ • gateway     │ │ • Mesop App   │ │ • legal-agent-a (Claude)  │  │ │
│  │  │ • work-pub    │ │               │ │ • legal-agent-b (Gemini)  │  │ │
│  │  │ • bid-gateway │ │               │ │ • travel-agent (GPT-4)    │  │ │
│  │  │ • bid-eval    │ │               │ │ • orchestrator (Claude)   │  │ │
│  │  │ • contract    │ │               │ │                           │  │ │
│  │  │ • settlement  │ │               │ │                           │  │ │
│  │  │ • registry    │ │               │ │                           │  │ │
│  │  │ • trust       │ │               │ │                           │  │ │
│  │  │ • identity    │ │               │ │                           │  │ │
│  │  │ • telemetry   │ │               │ │                           │  │ │
│  │  └───────┬───────┘ └───────────────┘ └───────────────────────────┘  │ │
│  │          │                                                           │ │
│  │          │                                                           │ │
│  │          ▼                                                           │ │
│  │  ┌───────────────────────────────────────────────────────────────┐  │ │
│  │  │                         Data Layer                            │  │ │
│  │  │                                                               │  │ │
│  │  │  ┌─────────────────┐  ┌─────────────────┐  ┌───────────────┐ │  │ │
│  │  │  │    Firestore    │  │  Cloud Storage  │  │ Secret Manager│ │  │ │
│  │  │  │   (Documents)   │  │   (Artifacts)   │  │  (API Keys)   │ │  │ │
│  │  │  │                 │  │                 │  │               │ │  │ │
│  │  │  │ • work_specs    │  │ • contracts     │  │ • LLM keys    │ │  │ │
│  │  │  │ • providers     │  │ • documents     │  │ • JWT keys    │ │  │ │
│  │  │  │ • bids          │  │ • results       │  │ • service creds│ │  │ │
│  │  │  │ • contracts     │  │                 │  │               │ │  │ │
│  │  │  │ • ledger        │  │                 │  │               │ │  │ │
│  │  │  │ • trust_scores  │  │                 │  │               │ │  │ │
│  │  │  └─────────────────┘  └─────────────────┘  └───────────────┘ │  │ │
│  │  │                                                               │  │ │
│  │  └───────────────────────────────────────────────────────────────┘  │ │
│  │                                                                       │ │
│  │  ┌───────────────────────────────────────────────────────────────┐  │ │
│  │  │                      Observability                            │  │ │
│  │  │                                                               │  │ │
│  │  │  ┌─────────────────┐  ┌─────────────────┐  ┌───────────────┐ │  │ │
│  │  │  │ Cloud Logging   │  │ Cloud Monitoring│  │ Cloud Trace   │ │  │ │
│  │  │  └─────────────────┘  └─────────────────┘  └───────────────┘ │  │ │
│  │  │                                                               │  │ │
│  │  └───────────────────────────────────────────────────────────────┘  │ │
│  │                                                                       │ │
│  └───────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.2 Cloud Run Services

| Service | Image | Memory | CPU | Min/Max Instances |
|---------|-------|--------|-----|-------------------|
| aex-gateway | gcr.io/PROJECT/aex-gateway | 256Mi | 1 | 1/10 |
| aex-work-publisher | gcr.io/PROJECT/aex-work-publisher | 512Mi | 1 | 1/5 |
| aex-bid-gateway | gcr.io/PROJECT/aex-bid-gateway | 256Mi | 1 | 1/5 |
| aex-bid-evaluator | gcr.io/PROJECT/aex-bid-evaluator | 512Mi | 1 | 1/3 |
| aex-contract-engine | gcr.io/PROJECT/aex-contract-engine | 256Mi | 1 | 1/5 |
| aex-settlement | gcr.io/PROJECT/aex-settlement | 256Mi | 1 | 1/3 |
| aex-provider-registry | gcr.io/PROJECT/aex-provider-registry | 256Mi | 1 | 1/5 |
| aex-trust-broker | gcr.io/PROJECT/aex-trust-broker | 256Mi | 1 | 1/3 |
| aex-identity | gcr.io/PROJECT/aex-identity | 256Mi | 1 | 1/3 |
| aex-telemetry | gcr.io/PROJECT/aex-telemetry | 256Mi | 1 | 1/3 |
| demo-ui | gcr.io/PROJECT/demo-ui | 512Mi | 1 | 1/3 |
| legal-agent-a | gcr.io/PROJECT/legal-agent-a | 1Gi | 2 | 0/3 |
| legal-agent-b | gcr.io/PROJECT/legal-agent-b | 1Gi | 2 | 0/3 |
| travel-agent | gcr.io/PROJECT/travel-agent | 1Gi | 2 | 0/3 |
| orchestrator | gcr.io/PROJECT/orchestrator | 1Gi | 2 | 1/3 |

### 3.3 Estimated Monthly Cost (Demo/Staging)

| Service | Estimated Cost |
|---------|---------------|
| Cloud Run (14 services) | $50-100 |
| Firestore | $10-20 |
| Cloud Storage | $5 |
| Secret Manager | $2 |
| Load Balancer | $20 |
| Cloud Logging/Monitoring | $10 |
| **LLM API Costs (variable)** | $50-200 |
| **Total** | **~$150-350/month** |

---

## 4. Implementation Phases (Updated)

### Phase 1: A2A Registry Integration (Week 1-2)

```
Week 1:
├── Day 1-2: Agent Card Package
│   ├── Create src/internal/agentcard/types.go
│   ├── Create src/internal/agentcard/fetcher.go
│   └── Create src/internal/agentcard/validator.go
│
├── Day 3-4: Provider Registry Updates
│   ├── Update provider model with Agent Card fields
│   ├── Implement Agent Card fetch on registration
│   └── Create skills indexing logic
│
└── Day 5: Search API
    ├── Implement GET /v1/providers/search
    └── Add filtering by skills/tags

Week 2:
├── Day 1-2: Contract Engine Updates
│   ├── Add A2A endpoint to award response
│   └── Include protocol binding info
│
├── Day 3-4: Testing
│   ├── Unit tests for Agent Card components
│   └── Integration tests for registration flow
│
└── Day 5: Documentation
    └── Update API docs with new endpoints
```

### Phase 2: Contract Token System (Week 3)

```
Week 3:
├── Day 1-2: Token Service
│   ├── Create src/internal/token/token.go
│   ├── Implement JWT generation (ES256)
│   └── Create JWKS endpoint
│
├── Day 3: Contract Engine Integration
│   ├── Generate token on award
│   └── Include in response
│
├── Day 4: Validation SDK
│   ├── Go SDK for token validation
│   └── Python SDK for providers
│
└── Day 5: Testing
    └── End-to-end token flow tests
```

### Phase 3: Demo Agents (Week 4-5)

```
Week 4:
├── Day 1-2: Agent Framework Setup
│   ├── Create demo/agents/ structure
│   ├── Setup LangGraph base agent
│   └── Create A2A server wrapper
│
├── Day 3-4: Legal Agent A (Claude)
│   ├── Implement contract_review skill
│   ├── Implement legal_research skill
│   └── Create agent-card.json
│
└── Day 5: Legal Agent B (Gemini)
    ├── Implement contract_review skill
    └── Implement compliance_check skill

Week 5:
├── Day 1-2: Travel Agent (GPT-4)
│   ├── Implement flight_booking skill
│   ├── Implement hotel_booking skill
│   └── Implement itinerary_planning skill
│
├── Day 3: Orchestrator Agent
│   ├── Implement task decomposition
│   ├── Implement AEX client
│   └── Implement result aggregation
│
├── Day 4-5: Demo UI (Mesop)
    ├── User input panel
    ├── Agent discovery view
    ├── Bidding visualization
    ├── Execution trace
    └── Settlement display
```

### Phase 4: GCP Deployment (Week 6)

```
Week 6:
├── Day 1-2: Infrastructure Setup
│   ├── Create GCP project
│   ├── Setup Firestore
│   ├── Configure Secret Manager
│   └── Setup Cloud Build
│
├── Day 3-4: Deploy Services
│   ├── Deploy AEX services
│   ├── Deploy demo agents
│   ├── Deploy UI
│   └── Configure load balancer
│
└── Day 5: Testing & Demo
    ├── End-to-end demo flow
    ├── Performance testing
    └── Documentation
```

---

## 5. Success Criteria

### Technical Validation
- [ ] Agent Card fetched from `/.well-known/agent-card.json`
- [ ] Skills indexed and searchable
- [ ] Contract token generated and validated
- [ ] A2A execution completes successfully
- [ ] Settlement calculates correct fees

### Demo Validation
- [ ] User can input multi-task request
- [ ] UI shows discovered agents
- [ ] Bidding process visible
- [ ] All 3 provider agents respond via A2A
- [ ] Results aggregated and displayed
- [ ] Settlement breakdown shown

### Performance Targets
- [ ] Agent discovery: < 500ms
- [ ] Bid evaluation: < 2s
- [ ] A2A task execution: < 30s per agent
- [ ] End-to-end demo: < 2 minutes

---

## 6. Next Steps

1. **Get Final Approval** on this plan
2. **Start Phase 1.1**: Create Agent Card package
3. **Setup Development Environment**: Ensure all LLM API keys available

---

**Document Status:** FINAL
**Approval Required:** Yes
**Next Action:** Begin Phase 1 implementation upon approval
