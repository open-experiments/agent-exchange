# AEX + A2A Integration - PlantUML Diagrams

This directory contains PlantUML sequence and architecture diagrams for the Agent Exchange + A2A Protocol integration.

## Diagram Index

| # | Diagram | Description |
|---|---------|-------------|
| 01 | [Provider Onboarding](01-provider-onboarding.puml) | Agent Card fetching and provider registration |
| 02 | [Work Submission & Discovery](02-work-submission-discovery.puml) | Work creation and skill-based agent discovery |
| 03 | [Bidding & Evaluation](03-bidding-evaluation.puml) | Bid submission, scoring, and ranking |
| 04 | [Contract Award & A2A Handoff](04-contract-award-a2a-handoff.puml) | Contract creation and direct A2A execution |
| 05 | [Settlement & Trust Update](05-settlement-trust-update.puml) | Payment processing and trust score updates |
| 06 | [Complete Demo Flow](06-complete-demo-flow.puml) | End-to-end multi-agent scenario |
| 07 | [Architecture Overview](07-architecture-overview.puml) | Component diagram showing all services |
| 08 | [GCP Deployment](08-gcp-deployment.puml) | Cloud deployment architecture |
| 09 | [Demo Implementation Flow](09-demo-implementation-flow.puml) | Actual demo call flow with AEX discovery |

## How to View

### Option 1: PlantUML Online Server
Copy the content of any `.puml` file and paste it at:
- https://www.plantuml.com/plantuml/uml/

### Option 2: VS Code Extension
Install the "PlantUML" extension and preview directly in VS Code.

### Option 3: Command Line
```bash
# Install PlantUML
brew install plantuml

# Generate PNG for all diagrams
for f in *.puml; do plantuml -tpng "$f"; done

# Generate SVG for all diagrams
for f in *.puml; do plantuml -tsvg "$f"; done
```

### Option 4: Docker
```bash
docker run -v $(pwd):/data plantuml/plantuml -tpng /data/*.puml
```

## Key Flows

### 1. Provider Onboarding
```
Provider Agent → AEX Registry → Fetch Agent Card → Index Skills → Store
```

### 2. Work Execution Flow
```
User Request → Orchestrator → AEX (Discovery + Bidding + Award) → A2A Execution → Settlement
```

### 3. A2A Handoff Point
The critical handoff occurs at **Contract Award**:
- Before: All communication through AEX REST APIs
- After: Direct A2A JSON-RPC between Consumer and Provider
- AEX only re-enters for Settlement

## Color Legend

| Color | Meaning |
|-------|---------|
| 🟦 Light Blue | User Interface |
| 🟩 Light Green | Consumer/Orchestrator + New Components |
| 🟨 Light Yellow | Agent Exchange (Existing) |
| 🟪 Light Pink | Provider Agents (A2A) |
| 🟫 Light Blue (DB) | Data Storage |

## Existing vs New Components

### Existing (No Changes Needed)
- AEX Gateway
- AEX Work Publisher
- AEX Bid Gateway
- AEX Bid Evaluator
- AEX Contract Engine
- AEX Settlement
- AEX Trust Broker
- AEX Identity
- AEX Telemetry

### New Components to Add
- **Agent Card Resolver** - Fetch and validate A2A Agent Cards
- **Token Service** - Generate JWT contract tokens
- **Skills Index** - Query providers by A2A skills/tags

### New (Demo Only)
- Orchestrator Agent (LangGraph + Claude)
- Legal Agent A (LangGraph + Claude)
- Legal Agent B (LangGraph + Gemini)
- Travel Agent (LangGraph + GPT-4)
- Demo UI (Mesop)
