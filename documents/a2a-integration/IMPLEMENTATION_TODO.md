# A2A Integration - Implementation TODO

**Created:** January 2, 2026
**Status:** PLANNING

---

## Implementation Phases Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Phase 1: A2A Registry       │  Phase 2: Contract Token   │  Phase 3: Demo │
│  ─────────────────────────   │  ────────────────────────  │  ────────────  │
│  • Agent Card Resolver       │  • JWT Token Service       │  • Legal Agent │
│  • Skills Indexing           │  • JWKS Endpoint           │  • Travel Agent│
│  • Search API                │  • Provider SDK            │  • Compliance  │
│  • Award A2A Endpoint        │  • Token Validation        │  • Orchestrator│
│                              │                            │  • Demo UI     │
│  Duration: 2-3 weeks         │  Duration: 1-2 weeks       │  Duration: 2-3 │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Phase 1: A2A-Compatible Registry

### 1.1 Agent Card Resolver
- [ ] **Create `internal/agentcard` package**
  - [ ] Define AgentCard Go struct matching A2A spec
  - [ ] Define AgentSkill struct
  - [ ] Define AgentCapabilities struct
  - [ ] Define SupportedInterface struct

- [ ] **Implement Agent Card Fetcher**
  - [ ] `FetchAgentCard(baseURL string) (*AgentCard, error)`
  - [ ] Try `/.well-known/agent-card.json` first
  - [ ] Fallback to `/.well-known/agent.json` on 404
  - [ ] Validate JSON schema
  - [ ] Parse protocol_version
  - [ ] Handle HTTP errors gracefully

- [ ] **Add Agent Card Caching**
  - [ ] In-memory cache with TTL (configurable, default 1 hour)
  - [ ] Force refresh option
  - [ ] Cache invalidation on provider update

- [ ] **Add Agent Card Validation**
  - [ ] Required fields check (name, supported_interfaces)
  - [ ] URL format validation
  - [ ] Skills schema validation
  - [ ] Security schemes validation

### 1.2 Update Provider Registry Service
- [ ] **Update Provider Model**
  ```go
  type Provider struct {
    // Existing fields...
    AgentCardURL      string          `json:"agent_card_url"`
    AgentCardRaw      json.RawMessage `json:"agent_card_raw"`
    ProtocolVersion   string          `json:"protocol_version"`
    PreferredInterface Interface      `json:"preferred_interface"`
    SkillsIndex       []SkillIndex    `json:"skills_index"`
    Capabilities      Capabilities    `json:"capabilities"`
    SecuritySchemes   []string        `json:"security_schemes"`
    LastCardRefresh   time.Time       `json:"last_card_refresh"`
  }
  ```

- [ ] **Update Registration Endpoint**
  - [ ] Accept `agent_base_url` OR `agent_card_url`
  - [ ] Fetch and validate Agent Card
  - [ ] Extract and index skills/tags
  - [ ] Store raw card + projection
  - [ ] Return verification status

- [ ] **Add Skills Search API**
  - [ ] `GET /v1/providers/search`
  - [ ] Query by `skill_id`
  - [ ] Query by `skill_tag` (multiple)
  - [ ] Query by `requires_streaming`
  - [ ] Query by `requires_push_notifications`
  - [ ] Query by `min_trust_score`
  - [ ] Pagination support

- [ ] **Add Agent Card Refresh Job**
  - [ ] Background goroutine to refresh cards
  - [ ] Configurable refresh interval
  - [ ] Update skills index on change
  - [ ] Log/alert on fetch failures

### 1.3 Update Contract Engine
- [ ] **Enhance Award Response**
  ```go
  type AwardResponse struct {
    // Existing fields...
    ProviderA2AEndpoint string   `json:"provider_a2a_endpoint"`
    ProtocolBinding     string   `json:"protocol_binding"`
    SecuritySchemes     []string `json:"security_schemes"`
    ContractToken       string   `json:"contract_token"`
  }
  ```

- [ ] **Lookup Provider A2A Endpoint on Award**
  - [ ] Fetch provider from registry
  - [ ] Extract preferred_interface URL
  - [ ] Include protocol binding info

### 1.4 Testing
- [ ] Unit tests for Agent Card Fetcher
- [ ] Unit tests for Agent Card Validation
- [ ] Integration tests for provider registration with Agent Card
- [ ] Integration tests for skills search API
- [ ] Mock A2A agent for testing

---

## Phase 2: Contract Token & Auth

### 2.1 Contract Token Service
- [ ] **Create `internal/token` package**
  - [ ] Define ContractTokenClaims struct
  - [ ] Generate ES256 key pair (or load from config)
  - [ ] Implement `GenerateContractToken(claims) (string, error)`
  - [ ] Implement `ValidateContractToken(token) (*Claims, error)`

- [ ] **Token Claims Structure**
  ```go
  type ContractTokenClaims struct {
    Issuer        string   `json:"iss"` // "aex"
    Audience      string   `json:"aud"` // provider domain
    Subject       string   `json:"sub"` // consumer agent ID
    WorkID        string   `json:"work_id"`
    ContractID    string   `json:"contract_id"`
    ProviderID    string   `json:"provider_id"`
    PriceMicrounits int64  `json:"price_microunits"`
    Scope         []string `json:"scope"` // ["a2a:message:send"]
    ExpiresAt     int64    `json:"exp"`
    IssuedAt      int64    `json:"iat"`
    TokenID       string   `json:"jti"`
  }
  ```

- [ ] **Integrate Token Generation in Award Flow**
  - [ ] Generate token after bid selection
  - [ ] Include in award response
  - [ ] Store token hash in contract record

### 2.2 JWKS Endpoint
- [ ] **Create `/.well-known/jwks.json` endpoint**
  - [ ] Expose public key for token verification
  - [ ] Support key rotation (kid)
  - [ ] Add to gateway routes

### 2.3 Provider SDK for Token Validation
- [ ] **Go SDK** (`pkg/aex-token`)
  - [ ] `ValidateContractToken(token, jwksURL) (*Claims, error)`
  - [ ] Fetch and cache JWKS
  - [ ] Verify signature
  - [ ] Validate claims (expiry, audience)

- [ ] **Python SDK** (`aex-token-python`)
  - [ ] Same functionality as Go SDK
  - [ ] Publish to PyPI (optional)

### 2.4 Testing
- [ ] Unit tests for token generation
- [ ] Unit tests for token validation
- [ ] Integration test: award → token → validation
- [ ] Test token expiry handling

---

## Phase 3: Demo Agents

### 3.1 Demo Infrastructure
- [ ] **Create `demo/` directory structure**
  ```
  demo/
  ├── docker-compose.yml
  ├── agents/
  │   ├── legal/
  │   ├── travel/
  │   ├── compliance/
  │   └── orchestrator/
  ├── ui/
  │   └── mesop/
  └── scripts/
      ├── setup.sh
      └── seed-data.sh
  ```

- [ ] **Docker Compose for Demo**
  - [ ] All AEX services
  - [ ] MongoDB
  - [ ] 3 provider agents
  - [ ] 1 orchestrator agent
  - [ ] Mesop UI
  - [ ] Proper networking

### 3.2 Legal Agent (Provider)
- [ ] **Setup Python A2A Agent**
  - [ ] Use a2a-python SDK
  - [ ] LangGraph + Claude integration
  - [ ] Implement A2A server

- [ ] **Define Agent Card**
  ```json
  {
    "name": "Legal Assistant Agent",
    "description": "Contract review and legal research",
    "supported_interfaces": [{
      "url": "http://legal-agent:8000/a2a",
      "protocol_binding": "JSONRPC"
    }],
    "skills": [
      {"id": "contract_review", "name": "Contract Review", "tags": ["legal", "contracts"]},
      {"id": "legal_research", "name": "Legal Research", "tags": ["legal", "research"]}
    ],
    "capabilities": {"streaming": true}
  }
  ```

- [ ] **Implement Skills**
  - [ ] Contract review (analyze document, identify risks)
  - [ ] Legal research (search regulations, summarize)

- [ ] **AEX Integration**
  - [ ] Listen for opportunities webhook
  - [ ] Auto-submit bids
  - [ ] Accept contract token auth
  - [ ] Report completion to AEX

### 3.3 Travel Agent (Provider)
- [ ] **Setup Python A2A Agent**
  - [ ] Use a2a-python SDK
  - [ ] CrewAI or AutoGen + GPT-4 integration

- [ ] **Define Agent Card**
  ```json
  {
    "name": "Travel Booking Agent",
    "description": "Flight, hotel, and itinerary planning",
    "skills": [
      {"id": "flight_booking", "tags": ["travel", "flights"]},
      {"id": "hotel_booking", "tags": ["travel", "hotels"]},
      {"id": "itinerary_planning", "tags": ["travel", "planning"]}
    ]
  }
  ```

- [ ] **Implement Skills**
  - [ ] Flight search (mock or real API)
  - [ ] Hotel search (mock or real API)
  - [ ] Itinerary creation

### 3.4 Compliance Agent (Provider)
- [ ] **Setup Python A2A Agent**
  - [ ] Use a2a-python SDK
  - [ ] AutoGen + Gemini integration

- [ ] **Define Agent Card**
  ```json
  {
    "name": "Compliance Research Agent",
    "description": "Regulatory compliance and research",
    "skills": [
      {"id": "regulatory_research", "tags": ["compliance", "regulations"]},
      {"id": "gdpr_compliance", "tags": ["compliance", "gdpr", "europe"]}
    ]
  }
  ```

- [ ] **Implement Skills**
  - [ ] Regulatory research
  - [ ] GDPR compliance check

### 3.5 Orchestrator Agent (Consumer/Host)
- [ ] **Setup Python A2A Agent**
  - [ ] Use a2a-python SDK
  - [ ] Google ADK or custom orchestration

- [ ] **Implement AEX Client**
  - [ ] Submit work to AEX
  - [ ] Handle bid notifications
  - [ ] Accept award response
  - [ ] Execute via A2A using contract token

- [ ] **Implement Task Decomposition**
  - [ ] Parse user request
  - [ ] Identify required skills
  - [ ] Create sub-tasks
  - [ ] Route to AEX marketplace

- [ ] **Implement Result Aggregation**
  - [ ] Collect responses from all providers
  - [ ] Combine into unified response
  - [ ] Handle partial failures

### 3.6 Demo UI (Mesop)
- [ ] **User Input Panel**
  - [ ] Text area for user request
  - [ ] Submit button

- [ ] **Agent Discovery View**
  - [ ] Show matched agents from AEX
  - [ ] Display skills and trust scores

- [ ] **Bidding Visualization**
  - [ ] Show incoming bids
  - [ ] Display bid evaluation scores
  - [ ] Highlight winner

- [ ] **Execution Trace View**
  - [ ] Show A2A messages to each provider
  - [ ] Display task status updates
  - [ ] Show streaming responses

- [ ] **Settlement View**
  - [ ] Show cost breakdown
  - [ ] Display platform fee
  - [ ] Show settlement status

### 3.7 Testing
- [ ] End-to-end demo flow test
- [ ] Test each agent individually
- [ ] Test orchestrator with mock providers
- [ ] Load test with multiple concurrent requests

---

## Phase 4: Trust & Governance (Future)

### 4.1 Agent Card Signatures
- [ ] Implement JWS verification for Agent Cards
- [ ] Add signature status to provider model
- [ ] Create verification tiers

### 4.2 Extended Agent Cards
- [ ] Support authenticated Agent Card fetch
- [ ] Private skills indexing
- [ ] Access control for sensitive endpoints

---

## Documentation

- [ ] Update QUICKSTART.md with A2A integration guide
- [ ] Add demo setup instructions
- [ ] Document Agent Card registration process
- [ ] Document contract token flow
- [ ] API documentation for new endpoints
- [ ] Provider SDK documentation

---

## Quick Reference: File Changes

### Phase 1 Files
```
src/internal/agentcard/           # NEW
  ├── types.go
  ├── fetcher.go
  ├── validator.go
  ├── cache.go
  └── fetcher_test.go

src/aex-provider-registry/
  ├── internal/model/model.go     # UPDATE
  ├── internal/service/service.go # UPDATE
  └── internal/httpapi/handlers.go # UPDATE

src/aex-contract-engine/
  └── internal/model/model.go     # UPDATE
```

### Phase 2 Files
```
src/internal/token/               # NEW
  ├── token.go
  ├── claims.go
  ├── jwks.go
  └── token_test.go

src/aex-gateway/
  └── internal/httpapi/router.go  # UPDATE (add JWKS route)

pkg/aex-token/                    # NEW (Go SDK)
aex-token-python/                 # NEW (Python SDK)
```

### Phase 3 Files
```
demo/                             # NEW
  ├── docker-compose.yml
  ├── agents/
  │   ├── legal/
  │   │   ├── Dockerfile
  │   │   ├── agent.py
  │   │   ├── agent_card.json
  │   │   └── requirements.txt
  │   ├── travel/
  │   ├── compliance/
  │   └── orchestrator/
  ├── ui/
  │   └── mesop/
  └── scripts/
```

---

## Success Checklist

### Phase 1 Complete When:
- [ ] Can register provider with Agent Card URL
- [ ] Skills are indexed and searchable
- [ ] Award response includes A2A endpoint

### Phase 2 Complete When:
- [ ] Contract token generated on award
- [ ] JWKS endpoint returns public key
- [ ] SDK can validate tokens

### Phase 3 Complete When:
- [ ] All 3 provider agents running
- [ ] Orchestrator can execute multi-agent workflow
- [ ] Demo UI shows complete journey
- [ ] Settlement completes correctly

---

**Next Action:** Get user approval, then start Phase 1.1 (Agent Card Resolver)
