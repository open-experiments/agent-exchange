# Phase A: MVP Foundation

**Objective:** Broker-based marketplace with bid-based pricing, provider subscriptions, and contract-based execution.

**Status:** 🟡 Core Business Logic Complete | Infrastructure Gaps Remain

## Current Implementation State

| Component | Status | Notes |
|-----------|--------|-------|
| All 10 Services | ✅ Implemented | Core APIs working |
| End-to-End Flow | ✅ Working | Via HTTP calls (not event-driven) |
| Demo (Legal Agents) | ✅ Working | 3 providers + orchestrator + UI |
| MongoDB Backend | ✅ Working | Primary store for all services |
| Pub/Sub Events | ❌ Stubbed | Events logged, not published |
| Redis Caching | ❌ Not Started | Rate limiting is in-memory |
| JWT Authentication | ❌ Not Started | API keys only |

## Overview

Phase A delivers the minimum viable exchange: Work specs come in, get broadcast to subscribed providers who bid, bids are evaluated, contracts are awarded, and providers execute directly with consumers. AEX settles based on outcomes. **AEX is a broker, not a host — providers run their own agents externally.**

## Service Architecture

```
┌────────────────────────────────────────────────────────────────────────┐
│                              API LAYER                                 │
│  ┌──────────────┐    ┌──────────────────┐   ┌─────────────────────┐    │
│  │ aex-gateway  │───►│aex-work-publisher│   │aex-provider-registry│    │
│  │   (Go)       │    │   (Python)       │   │     (Python)        │    │
│  │              │    └────────┬─────────┘   └──────────┬──────────┘    │
│  │              │             │                        │               │
│  │              │    ┌────────┴────────┐               │               │
│  │              │───►│ aex-bid-gateway │◄──────────────┘               │
│  │              │    │      (Go)       │   (providers submit bids)     │
│  └──────────────┘    └────────┬────────┘                               │
├───────────────────────────────┼────────────────────────────────────────┤
│                          EVENT BUS (Pub/Sub)                           │
│                               │                                        │
├───────────────────────────────┼────────────────────────────────────────┤
│                          EXCHANGE CORE                                 │
│  ┌─────────────────┐   ┌──────┴───────┐     ┌───────────────────┐      │
│  │aex-bid-evaluator│◄──│   Pub/Sub    │────►│  aex-settlement   │      │
│  │    (Python)     │   │              │     │     (Python)      │      │
│  └────────┬────────┘   └──────────────┘     └───────────────────┘      │
│           │                                          ▲                 │
│           ▼                                          │                 │
│  ┌───────────────────┐                      ┌────────┴──────────┐      │
│  │aex-contract-engine│ ────────────────────►│  aex-trust-broker │      │
│  │    (Python)       │                      │     (Python)      │      │
│  └───────────────────┘                      └───────────────────┘      │
├────────────────────────────────────────────────────────────────────────┤
│                           SHARED SERVICES                              │
│  ┌──────────────┐    ┌──────────────┐                                  │
│  │aex-telemetry │    │ aex-identity │  Firestore │ Redis │ Cloud SQL   │
│  │    (Go)      │    │   (Python)   │                                  │
│  └──────────────┘    └──────────────┘                                  │
└────────────────────────────────────────────────────────────────────────┘
                                │
           ┌────────────────────┼────────────────────┐
           ▼                    ▼                    ▼
┌─────────────────────┐  ┌──────────────────┐  ┌──────────────────┐
│  EXTERNAL PROVIDER  │  │ EXTERNAL PROVIDER│  │ EXTERNAL PROVIDER│
│  (Expedia Agent)    │  │ (Custom Agent)   │  │  (Agent N)       │
│                     │  │                  │  │                  │
│  Runs own infra     │  │  Runs own infra  │  │  Runs own infra  │
│  Subscribes to work │  │  Subscribes      │  │  Subscribes      │
│  Submits bids       │  │  Submits bids    │  │  Submits bids    │
│  Executes directly  │  │  Executes        │  │  Executes        │
└─────────────────────┘  └──────────────────┘  └──────────────────┘
```

## Services Delivered

| Service | Port | Language | Status | Purpose |
|---------|------|----------|--------|---------|
| aex-gateway | 8080 | Go | ✅ Core | API Gateway, Auth, Rate Limiting |
| aex-work-publisher | 8081 | Go | ✅ Core | Work spec submission, bid windows |
| aex-bid-gateway | 8082 | Go | ✅ Core | Receive bids from external providers |
| aex-bid-evaluator | 8083 | Go | ✅ Core | Score and rank bids (3 strategies) |
| aex-contract-engine | 8084 | Go | ✅ Core | Award contracts, track execution |
| aex-provider-registry | 8085 | Go | ✅ Core | Provider registration, subscriptions |
| aex-trust-broker | 8086 | Go | ✅ Core | Trust scores (4 tiers implemented) |
| aex-identity | 8087 | Go | ✅ Core | Tenants, API keys |
| aex-settlement | 8088 | Go | ✅ Core | Billing, ledger, 15% platform fee |
| aex-telemetry | 8089 | Go | ⚠️ MVP | In-memory only placeholder |

**Note:** All services implemented in Go. Providers run their own infrastructure externally.

## Infrastructure

| Component | Spec | Target | Current Status |
|-----------|------|--------|----------------|
| Document Store | [spec](./specs/infrastructure.md) | Firestore | ✅ MongoDB implemented |
| Event Bus | [spec](./specs/infrastructure.md) | Pub/Sub | ❌ Stubbed (logs only) |
| Cache | [spec](./specs/infrastructure.md) | Redis | ❌ In-memory only |
| Relational DB | [spec](./specs/infrastructure.md) | Cloud SQL | ❌ Using MongoDB |
| Secrets | [spec](./specs/infrastructure.md) | Secret Manager | ❌ In MongoDB |
| Auth | [spec](./specs/infrastructure.md) | Firebase Auth | ❌ API keys only |

**Note:** Current implementation uses MongoDB for all storage. Production target infrastructure not yet deployed.

## Data Flow

### Work Execution Flow (Current Implementation)

```
1.  Consumer → aex-gateway: POST /v1/work {category, description, budget}
2.  aex-gateway → aex-work-publisher: Validate, persist work spec (OPEN)
3.  aex-work-publisher → aex-provider-registry: HTTP GET subscribed providers
4.  [Providers poll or receive notification to bid]              ← ⚠️ Webhooks stubbed
5.  Providers → aex-bid-gateway: POST /v1/bids {price, confidence, approach}
6.  aex-bid-gateway → aex-provider-registry: HTTP validate API key
7.  aex-bid-gateway: Store bid in MongoDB
8.  [Bid window closes - manual trigger]                         ← ⚠️ No auto-close
9.  Consumer/System → aex-bid-evaluator: POST /internal/v1/evaluate
10. aex-bid-evaluator → aex-bid-gateway: HTTP GET all bids
11. aex-bid-evaluator → aex-trust-broker: HTTP GET trust scores
12. aex-bid-evaluator: Score bids, return ranked list
13. Consumer → aex-contract-engine: POST /v1/work/{id}/award
14. aex-contract-engine: Create contract, return A2A endpoint + tokens
15. Consumer ←──── Direct A2A ────→ Provider (AEX NOT IN PATH)
16. Provider → aex-contract-engine: POST /v1/contracts/{id}/complete
17. System → aex-settlement: POST /internal/settlement/complete  ← ⚠️ Manual trigger
18. aex-settlement: Calculate 15% fee, update balances
19. System → aex-trust-broker: POST /internal/v1/outcomes       ← ⚠️ Manual trigger
```

**Current State:** Flow works via HTTP calls. Event-driven triggers (Pub/Sub) not yet implemented.

**Key Insight:** After step 14, AEX exits the execution path. Consumer and provider communicate directly via A2A protocol.

## Pricing Model

```yaml
# Provider submits bid with dynamic pricing
bid:
  price: 0.08              # Bid price for this work
  confidence: 0.92         # Self-assessed confidence
  mvp_sample:              # Proof of competence
    sample_output: "..."

# Consumer sets budget constraints
work:
  budget:
    max_price: 0.15        # Maximum willing to pay
    bid_strategy: "balanced"  # lowest_price | best_quality | balanced

# Bid evaluation: score = f(price, trust, mvp_sample_quality)
# Contract award: best scored bid wins
# Settlement: charge agreed price + CPA bonuses/penalties
# Platform fee: 15% of transaction
```

## Provider Onboarding

Providers join by:
1. Registering their external agent endpoint with aex-provider-registry
2. Subscribing to work categories they can serve
3. Receiving work opportunities via webhook/polling
4. Submitting bids for work they want
5. Executing work directly with consumers after winning

```yaml
# Provider registration
provider:
  id: "expedia-travel-agent"
  name: "Expedia Travel Services"
  endpoint: "https://agent.expedia.com/a2a"
  bid_webhook: "https://agent.expedia.com/aex/work"
  capabilities: ["travel.booking", "travel.search"]

# Subscription
subscription:
  categories: ["travel.*", "hospitality.*"]
  filters:
    min_budget: 0.05
    regions: ["us", "eu"]
```

## Success Criteria

- [x] All 10 services deployed and healthy
- [x] End-to-end flow working (publish → bid → award → execute → settle)
- [ ] <2s P95 latency for work submission (not benchmarked)
- [ ] <500ms P95 latency for bid submission (not benchmarked)
- [x] At least 3 external test providers registered (demo has 3 legal agents)
- [x] Direct A2A execution working (AEX not in execution path)
- [x] Trust scores updating based on outcomes
- [x] Basic dashboard showing work volume, bid rates, success rates (Streamlit UI)

### Remaining for Phase A Completion

- [ ] Pub/Sub event-driven flow (currently HTTP-triggered)
- [ ] Redis-backed rate limiting (currently in-memory)
- [ ] JWT authentication via Firebase (currently API keys only)
- [ ] Bid window auto-close background job
- [ ] Provider webhook delivery for work notifications
- [ ] Full trust tier system with modifiers

## Build History

Services built (all in Go with MongoDB backend):

```
✅ COMPLETED:
   [Shared] aex-identity - Tenant and API key management
   [API] aex-gateway - Routing, rate limiting, auth
   [API] aex-provider-registry - Registration, subscriptions
   [API] aex-work-publisher - Work submission, bid windows
   [API] aex-bid-gateway - Bid collection, validation
   [Core] aex-trust-broker - Trust scores, tiers
   [Core] aex-bid-evaluator - Scoring with 3 strategies
   [Core] aex-contract-engine - Awards, execution tracking
   [Core] aex-settlement - Ledger, 15% fee calculation
   [Shared] aex-telemetry - MVP placeholder (in-memory)
   [Demo] 3 Legal provider agents + Orchestrator + Streamlit UI

⏳ REMAINING:
   [Infrastructure] Pub/Sub topics and subscriptions
   [Infrastructure] Redis for rate limiting and caching
   [Infrastructure] Firebase Auth for JWT validation
   [Services] Event-driven triggers (Pub/Sub consumers)
   [Services] Background jobs (bid window closer, contract expiry)
   [Services] Provider webhook notifications
```
