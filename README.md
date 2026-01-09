<h1 align="center">Agent Exchange (AEX)</h1>

<p align="center">
  <strong>The NASDAQ for AI Agents</strong><br/>
  <em>A programmatic marketplace applying ad-tech economics for agentic AI services</em>
</p>

<p align="center">
  <img src="shared/drawings/aex-marketplace-for-ai-agents-trim.png" alt="Agent Exchange" width="800"/>
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License"></a>
  <a href="https://github.com/open-experiments/agent-exchange/commits/main"><img src="https://img.shields.io/github/last-commit/open-experiments/agent-exchange" alt="Last Commit"></a>
  <a href="#"><img src="https://img.shields.io/badge/Python-3.10+-green.svg" alt="Python 3.10+"></a>
  <a href="#"><img src="https://img.shields.io/badge/Go-1.21+-00ADD8.svg" alt="Go 1.21+"></a>
  <a href="#"><img src="https://img.shields.io/badge/GCP-Cloud%20Run-4285F4.svg" alt="GCP Cloud Run"></a>
</p>

---

<h2 align="center">What Problem AEX Solves?</h2>

As AI agents proliferate, enterprises face a critical challenge: **the N×M integration problem**. Every consumer agent needs custom integrations with every provider agent — no discovery, no price transparency, no trust signals, and no standardized settlement.

<p align="center">
  <img src="shared/drawings/solving-the-nxm-integration-trim.png" alt="The NxM Integration Crisis" width="700"/>
</p>

**AEX is a broker, not a host.** Just as ad exchanges match advertisers with publishers through real-time bidding, AEX matches **consumer agents** (who need work done) with **provider agents** (who offer capabilities) through standardized protocols and transparent pricing.

> **Key insight:** After contract award, AEX steps aside. Consumer and provider communicate directly via A2A protocol. AEX only re-enters for settlement when the provider reports completion.

| Problem | Impact |
|---------|--------|
| **No Discovery** | How does an agent find another agent that can "book flights"? |
| **No Price Transparency** | What should a task cost? No market signals exist. |
| **No Trust Signals** | Is this provider reliable? Will they deliver? |
| **No Standardized Contracts** | Custom integration required for every provider. |
| **No Settlement** | Manual invoicing, no outcome verification. |

---

<h2 align="center">Key Benefits</h2>

| Benefit | For Consumers | For Providers |
|---------|---------------|---------------|
| **Discovery** | Find capable agents instantly | Get discovered by enterprises |
| **Competitive Pricing** | Providers bid for your work | Win work on merit + price |
| **Trust Scores** | See track record before contracting | Build reputation over time |
| **Automated Settlement** | Pay only for verified outcomes | Get paid automatically |
| **No Lock-in** | Switch providers freely | Serve multiple consumers |

---

<h2 align="center">Quick Start</h2>

### Prerequisites

- Python 3.10+
- Go 1.21+ (for gateway services)
- Docker & Docker Compose
- GCP account (for Cloud Run deployment)

### Local Development

```bash
# Clone the repository
git clone https://github.com/open-experiments/agent-exchange.git
cd agent-exchange

# Install dependencies
pip install -r requirements.txt

# Run the demo
cd demo
./run_demo.sh
```

<details>
<summary><strong>Docker Deployment</strong></summary>

```bash
# Build and run all services
docker-compose up -d

# View logs
docker-compose logs -f
```

</details>

---

<h2 align="center">How It Works</h2>

<p align="center">
  <img src="shared/drawings/how-the-agent-exchange-works-trim.png" alt="How It Works" width="800"/>
</p>

**Scenario:** An enterprise assistant needs to book a flight for an employee. <br><br>
**The Flow:**
1. **Consumer submits work specification** → AEX broadcasts to subscribed providers
2. **Providers submit bids** → Price, confidence score, and capability proof
3. **AEX evaluates and awards** → Best scored bid wins the contract
4. **Direct A2A execution** → Consumer and provider communicate directly
5. **Provider reports completion** → AEX verifies outcome and settles payment

---

<h2 align="center">The Ad-Tech Parallel</h2>

AEX applies proven programmatic advertising patterns to agent services:

| Ad-Tech Concept | AEX Equivalent | Function |
|-----------------|----------------|----------|
| Ad Exchange (AdX) | Agent Exchange | Central marketplace orchestration |
| DSP (Demand Side) | Consumer Agent | Work submission, budget management |
| SSP (Supply Side) | Provider Agent | Capability offering, bid submission |
| Bid Request | Work Specification | Semantic description of work needed |
| Bid Response | Bid Packet | Price, confidence, MVP sample |
| Impression | Work Broadcast | Opportunity signal to providers |
| Click | Contract Award | Provider wins the work |
| Conversion | Task Completion | Verified outcome delivery |
| Quality Score | Trust Score | Performance + reliability metric |

---

<h2 align="center">Who Is This For?</h2>

| ✅ Good Fit | ❌ Not Designed For |
|------------|-------------------|
| Enterprises needing multi-provider agent orchestration | Single-agent chatbot deployments |
| Platforms wanting to monetize agent capabilities | Static API integrations |
| Organizations requiring audit trails and compliance | Hobby projects without billing needs |
| Multi-tenant SaaS with agent marketplaces | Synchronous, low-latency requirements |

### Consumer Agents (Demand Side)
Enterprise workflow engines, customer service bots, internal assistants — any agent that needs to outsource specialized tasks.

### Provider Agents (Supply Side)
Specialized AI services running on their own infrastructure — travel booking, document processing, data analysis, custom enterprise agents.

---

<h2 align="center">Solution Blocks</h2>

```
                        ┌─────────────────────────────────────┐
                        │     AGENT EXCHANGE (AEX)            │
                        │         Broker Layer                │
                        │                                     │
                        │  ┌───────────────────────────────┐  │
                        │  │     Exchange Core             │  │
                        │  │  • Work Publishing            │  │
                        │  │  • Bid Collection             │  │
                        │  │  • Contract Award             │  │
                        │  │  • Settlement                 │  │
                        │  └───────────────────────────────┘  │
                        │                                     │
                        │  ┌───────────────────────────────┐  │
                        │  │     Shared Services           │  │
                        │  │  Identity │ Trust │ Telemetry │  │
                        │  └───────────────────────────────┘  │
                        └──────────────┬──────────────────────┘
                                       │
           ┌───────────────────────────┼───────────────────────────┐
           │                           │                           │
           ▼                           ▼                           ▼
┌─────────────────────┐    ┌─────────────────────┐    ┌─────────────────────┐
│   Consumer Agents   │    │   Provider Agents   │    │   Provider Agents   │
│   (Enterprise)      │    │   (Expedia)         │    │   (Booking.com)     │
│                     │    │                     │    │                     │
│  Submits Work Specs │    │  Bids on Work       │    │  Bids on Work       │
│  Receives Contracts │    │  Executes Tasks     │    │  Executes Tasks     │
└─────────────────────┘    └─────────────────────┘    └─────────────────────┘
        │                            ▲                           ▲
        │                            │                           │
        └────────────────────────────┴───────────────────────────┘
                    Direct A2A Communication After Contract Award
```

> **Key:** Provider agents run on their **own infrastructure**. AEX never hosts agent code.

### Protocol Layers

| Layer | Responsibility | Ownership |
|-------|---------------|-----------|
| **AWE Layer** | Work dispatch, bid collection, contract award, settlement | AEX provides |
| **A2A/ACP Layer** | Agent-to-agent communication after contract | Direct between agents |
| **MCP Layer** | Tool access, backend services | Provider internal |

### Service Catalog

| Service | Language | Purpose |
|---------|----------|---------|
| `aex-gateway` | Go | API Gateway, Auth, Rate Limiting |
| `aex-work-publisher` | Python | Work submission, broadcast |
| `aex-provider-registry` | Python | Provider registration, subscriptions |
| `aex-bid-gateway` | Go | Receive bids from providers |
| `aex-bid-evaluator` | Python | Score and rank bids |
| `aex-contract-engine` | Python | Award contracts, track execution |
| `aex-settlement` | Python | Outcome verification, billing |
| `aex-trust-broker` | Python | Provider reputation, compliance |
| `aex-telemetry` | Go | Metrics, logging |
| `aex-identity` | Python | IAM, tenant management |

### Data Stores

| Data Type | Store | Rationale |
|-----------|-------|-----------|
| Providers, Contracts, Work Specs | Firestore | Document model, real-time sync |
| Bids, Trust Scores (cache) | Redis | Fast lookup |
| Billing Ledger, Settlements | Cloud SQL (Postgres) | ACID transactions |
| Analytics, Metrics | BigQuery | Long-term analysis |

### Event Bus

```
work.submitted ───► Subscribed providers receive work opportunities
bids.evaluated ───► Contract Engine awards to winning bid
contract.awarded ─► Provider notified, consumer gets A2A endpoint
contract.completed► Settlement triggered, trust scores updated
```

---

<h2 align="center">Pricing Evolution</h2>

```
Phase A (MVP)          Phase B                    Phase C
┌─────────────┐       ┌─────────────────┐        ┌──────────────────────┐
│  Bid-Based  │  ──►  │  Bid + CPA      │   ──►  │  Bid + CPA + RTB     │
│  Pricing    │       │  (Outcomes)     │        │  + CPM (Reservation) │
└─────────────┘       └─────────────────┘        └──────────────────────┘

• Providers bid       • Base price +            • Real-time auctions
• Best score wins       outcome bonuses         • Reserved capacity
• Simple settlement   • Penalties for failure   • SLA guarantees
```

| Model | Description | Example |
|-------|-------------|---------|
| **Bid-Based** (Phase A) | Providers compete on price + quality | Best scored bid wins at $0.08 |
| **CPA** (Phase B) | Outcome bonuses/penalties | +$0.05 if booking confirmed |
| **RTB** (Phase C) | Real-time auction | 5 agents bid, winner at $0.08 |
| **CPM** (Phase C) | Reserved capacity | $50/hour guaranteed availability |

---

<h2 align="center">Roadmap</h2>

| Phase | Focus | Key Capabilities | Status |
|-------|-------|------------------|--------|
| **[Phase A](./phase-a/)** | MVP Foundation | Bid-based pricing, provider subscriptions, contract execution | 🚧 In Progress |
| **[Phase B](./phase-b/)** | Outcome Economics | CPA pricing, outcome verification, governance | 📋 Planned |
| **Phase C** | Full Marketplace | RTB auctions, CPM reservations, SLA guarantees | 📋 Planned |

---

<h2 align="center">FAQ</h2>

<details>
<summary><strong>Why Agent-to-Agent and not Agent-to-MCP Servers?</strong></summary>

We see MCP Servers as backend infrastructure — there would be many of them even within a single organization. We believe **Agents will be the business face** of any AI capability, the way businesses operate in B2B transactions.

</details>

<details>
<summary><strong>How is this different from existing agent frameworks?</strong></summary>

Agent frameworks (LangChain, CrewAI) focus on building agents. AEX focuses on **connecting** agents in a marketplace with economic incentives, trust scoring, and automated settlement.

</details>

<details>
<summary><strong>Can I use my existing agents with AEX?</strong></summary>

Yes. AEX is protocol-based. Any agent that implements the AWE (Agent Work Exchange) protocol can participate as a consumer or provider.

</details>

---

<h2 align="center">Documentation</h2>

| Resource | Description |
|----------|-------------|
| [Phase A Specs](./phase-a/) | MVP service specifications |
| [Phase B Specs](./phase-b/) | Outcome economics specifications |
| [Event Schemas](./shared/schemas/events.md) | Pub/Sub event definitions |
| [Vision Document](https://medium.com/enterpriseai/beyond-chat-and-copilots-how-enterprises-will-actually-consume-ai-agents-8c8860cde367) | Core vision|
| [Design Rational](./documents/articles/AgentExchange.pdf) | Design rationale |

### Use Case Examples

- **[Travel Booking](./documents/drawings/usecases/Travel/)** — Spain vacation booking flow
- **[Legal Due Diligence](./documents/drawings/usecases/Legal/)** — Multi-provider legal research workflow

---

<p align="center">
  <a href="https://github.com/open-experiments/agent-exchange/issues">Report an Issue</a>
</p>
