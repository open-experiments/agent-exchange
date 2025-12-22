# Phase A: MVP Foundation

**Objective:** Broker-based marketplace with bid-based pricing, provider subscriptions, and contract-based execution.

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

| Service | Spec | Language | Runtime | Purpose |
|---------|------|----------|---------|---------|
| aex-gateway | [spec](./specs/aex-gateway.md) | Go | Cloud Run | API Gateway, Auth, Rate Limiting |
| aex-work-publisher | [spec](./specs/aex-work-publisher.md) | Python | Cloud Run | Work spec submission, broadcast |
| aex-provider-registry | [spec](./specs/aex-provider-registry.md) | Python | Cloud Run | Provider registration, subscriptions |
| aex-bid-gateway | [spec](./specs/aex-bid-gateway.md) | Go | Cloud Run | Receive bids from external providers |
| aex-bid-evaluator | [spec](./specs/aex-bid-evaluator.md) | Python | Cloud Run | Score and rank bids |
| aex-contract-engine | [spec](./specs/aex-contract-engine.md) | Python | Cloud Run | Award contracts, track execution |
| aex-settlement | [spec](./specs/aex-settlement.md) | Python | Cloud Run | Outcome verification, billing |
| aex-trust-broker | [spec](./specs/aex-trust-broker.md) | Python | Cloud Run | Provider reputation, compliance |
| aex-telemetry | [spec](./specs/aex-telemetry.md) | Go | Cloud Run | Metrics, logging |
| aex-identity | [spec](./specs/aex-identity.md) | Python | Cloud Run | IAM, tenant management |

**Note:** No GKE/agent hosting services. Providers run their own infrastructure.

## Infrastructure

| Component | Spec | GCP Service | Purpose |
|-----------|------|-------------|---------|
| GCP Project | [spec](./specs/infrastructure.md) | Resource Manager | Project setup, IAM |
| Networking | [spec](./specs/infrastructure.md) | VPC, Cloud NAT | Network isolation |
| Event Bus | [spec](./specs/infrastructure.md) | Pub/Sub | Work broadcast, bid collection |
| Document Store | [spec](./specs/infrastructure.md) | Firestore | Providers, contracts, work specs |
| Cache | [spec](./specs/infrastructure.md) | Memorystore (Redis) | Bids, trust scores |
| Relational DB | [spec](./specs/infrastructure.md) | Cloud SQL (PostgreSQL) | Billing, ledger |
| Secrets | [spec](./specs/infrastructure.md) | Secret Manager | API keys, creds |

## Data Flow

### Work Execution Flow (Happy Path)

```
1.  Consumer Agent → aex-gateway: POST /v1/work {category, description, budget}
2.  aex-gateway → aex-work-publisher: Validate, persist work spec (OPEN)
3.  aex-work-publisher → aex-provider-registry: Get subscribed providers
4.  aex-work-publisher → Pub/Sub: Broadcast "work.submitted" to subscribed providers
5.  External Providers: Receive work opportunity, decide to bid
6.  External Providers → aex-bid-gateway: POST /v1/bids {price, confidence, mvp_sample}
7.  aex-bid-gateway: Store bids, notify consumer of incoming bids
8.  [Bid window closes]
9.  aex-bid-evaluator: Score bids (price, trust, MVP sample quality)
10. aex-bid-evaluator → aex-contract-engine: Send ranked bids
11. Consumer Agent or Auto: Select winner, award contract
12. aex-contract-engine: Create contract, return provider A2A endpoint
13. Consumer Agent ←──── Direct A2A ────→ Provider Agent (AEX NOT IN PATH)
14. Provider Agent → aex-contract-engine: Report completion + outcome metrics
15. aex-contract-engine → aex-settlement: Trigger settlement
16. aex-settlement: Verify outcome, calculate cost (base + CPA), update ledger
17. aex-settlement → aex-trust-broker: Update provider reputation
```

**Key Insight:** After step 12, AEX exits the execution path. Consumer and provider communicate directly via A2A protocol.

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

- [ ] All 10 services deployed and healthy
- [ ] End-to-end flow working (publish → bid → award → execute → settle)
- [ ] <2s P95 latency for work submission
- [ ] <500ms P95 latency for bid submission
- [ ] At least 3 external test providers registered
- [ ] Direct A2A execution working (AEX not in execution path)
- [ ] Trust scores updating based on outcomes
- [ ] Basic dashboard showing work volume, bid rates, success rates

## Build Order

Infrastructure first, then services in dependency order:

```
Week 1:     [Infrastructure] GCP Project, VPC, Pub/Sub, Firestore, Redis
Week 1:     [Infrastructure] Cloud SQL, Secret Manager
Week 1:     [Shared] aex-identity (needed by gateway for auth)
Week 1:     [API] aex-gateway
Week 2:     [API] aex-provider-registry (providers must register first)
Week 2:     [API] aex-work-publisher, aex-bid-gateway (parallel)
Week 2:     [Core] aex-trust-broker (needed by evaluator)
Week 3:     [Core] aex-bid-evaluator
Week 3:     [Core] aex-contract-engine
Week 3:     [Core] aex-settlement
Week 4:     [Shared] aex-telemetry
Week 4:     [Integration] Provider SDK, test providers, E2E testing
```
