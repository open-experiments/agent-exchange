# AEX Development Roadmap

This document tracks the gaps between Phase A specifications and current implementation, broken down into actionable work items.

## Current State Summary

**Status: Core business logic implemented, infrastructure and completeness gaps remain**

### What's Working

The end-to-end marketplace flow works via HTTP calls:
```
Tenant creation → Provider registration → Work submission → Bidding →
Evaluation → Contract award → A2A execution → Settlement → Trust updates
```

### Service Implementation Status

| Service | Core API | Store | Events | Status |
|---------|----------|-------|--------|--------|
| aex-gateway | ✅ Proxy, rate limit | N/A | N/A | ✅ Working |
| aex-work-publisher | ✅ CRUD | MongoDB/Firestore | ⚠️ Logged | ✅ Working |
| aex-bid-gateway | ✅ Submit, list | MongoDB | ⚠️ Logged | ✅ Working |
| aex-bid-evaluator | ✅ Evaluate | Memory | ⚠️ Logged | ✅ Working |
| aex-contract-engine | ✅ Award, complete | MongoDB | ⚠️ Logged | ✅ Working |
| aex-provider-registry | ✅ Register, subscribe | MongoDB | ⚠️ Logged | ✅ Working |
| aex-trust-broker | ✅ Get/record | MongoDB | ⚠️ Logged | ✅ Working |
| aex-identity | ✅ Tenants, keys | MongoDB | ⚠️ Logged | ✅ Working |
| aex-settlement | ✅ Balance, settle | MongoDB | ⚠️ Logged | ✅ Working |
| aex-telemetry | ✅ Ingest, query | Memory only | N/A | ⚠️ MVP |

### Demo Status

| Component | Status |
|-----------|--------|
| 3 Legal Provider Agents (LangGraph) | ✅ Working |
| Orchestrator Consumer Agent | ✅ Working |
| Streamlit UI Dashboard | ✅ Working |
| A2A Protocol Integration | ✅ Working |

However, the following categories of work remain to achieve full Phase A spec compliance.

---

## Priority Levels

- **P0 - Critical**: Blocks production deployment or breaks core functionality
- **P1 - High**: Required for Phase A completion per spec
- **P2 - Medium**: Important for production readiness
- **P3 - Low**: Nice to have, can defer to Phase B

---

## 1. Infrastructure Gaps (P0/P1)

### 1.1 Pub/Sub Integration (P0)

Current state: In-memory `events.Publisher` that logs events but doesn't publish to GCP Pub/Sub.

| Task | Service(s) | Effort | Description |
|------|------------|--------|-------------|
| Create Pub/Sub topics | Infrastructure | S | Create topics: `aex-work-events`, `aex-bid-events`, `aex-contract-events`, `aex-settlement-events`, `aex-trust-events` |
| Implement Pub/Sub publisher | `internal/events` | M | Replace in-memory publisher with GCP Pub/Sub client |
| Add Pub/Sub subscriptions | All services | M | Create push subscriptions for each consuming service |
| Wire work-publisher events | `aex-work-publisher` | S | Publish: `work.submitted`, `work.cancelled`, `work.bid_window_closed` |
| Wire bid-gateway events | `aex-bid-gateway` | S | Publish: `bid.submitted` |
| Wire bid-evaluator consumer | `aex-bid-evaluator` | M | Subscribe to `work.bid_window_closed`, trigger evaluation |
| Wire contract-engine events | `aex-contract-engine` | S | Publish: `contract.awarded`, `contract.completed`, `contract.failed` |
| Wire settlement consumer | `aex-settlement` | M | Subscribe to `contract.completed`, trigger settlement |
| Wire trust-broker consumer | `aex-trust-broker` | M | Subscribe to `contract.completed`, record outcome |

### 1.2 Redis Integration (P0)

Current state: No Redis anywhere. Rate limiting is in-memory (won't work with multiple instances).

| Task | Service(s) | Effort | Description |
|------|------------|--------|-------------|
| Add Redis client library | `internal/` | S | Add `github.com/redis/go-redis/v9` to shared deps |
| Implement Redis rate limiter | `aex-gateway` | M | Replace in-memory rate limiter with Redis-backed token bucket |
| Add API key cache | `aex-gateway` | M | Cache validated API keys in Redis (TTL 5 min) |
| Add tenant cache | `aex-identity` | S | Cache tenant lookups in Redis |
| Add trust score cache | `aex-trust-broker` | S | Cache trust scores in Redis (TTL 1 min) |
| Add config for Redis | All services | S | `REDIS_HOST`, `REDIS_PORT` environment variables |

### 1.3 Firebase Auth / JWT Validation (P1)

Current state: Only API key authentication works. JWT Bearer token auth not implemented.

| Task | Service(s) | Effort | Description |
|------|------------|--------|-------------|
| Add Firebase Admin SDK | `aex-gateway` | S | Add `firebase.google.com/go/v4` dependency |
| Implement JWT validator | `aex-gateway` | M | Validate `Authorization: Bearer <token>` against Firebase |
| Extract claims | `aex-gateway` | S | Extract `tenant_id`, `roles` from JWT claims |
| Add Firebase project config | `aex-gateway` | S | `FIREBASE_PROJECT_ID` environment variable |
| Link Firebase users to tenants | `aex-identity` | L | Full user management API (see Identity gaps below) |

### 1.4 Secret Manager Integration (P1)

Current state: API keys stored in MongoDB with hash. Spec calls for Secret Manager.

| Task | Service(s) | Effort | Description |
|------|------------|--------|-------------|
| Add Secret Manager client | `aex-identity` | S | Add `cloud.google.com/go/secretmanager` |
| Store API key hashes in Secret Manager | `aex-identity` | M | Migrate from MongoDB to Secret Manager for key storage |
| Warm cache on startup | `aex-gateway` | S | Load API keys from Secret Manager to Redis on startup |
| Add config | `aex-identity`, `aex-gateway` | S | `SECRET_PROJECT` environment variable |

---

## 2. Service-Specific Gaps

### 2.1 AEX-GATEWAY

| Task | Priority | Effort | Description |
|------|----------|--------|-------------|
| JWT authentication | P1 | M | See Firebase Auth section above |
| Redis rate limiting | P0 | M | See Redis section above |
| Rate limit headers | P2 | S | Return `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset` headers |
| Request ID propagation | P2 | S | Generate UUID if not provided, propagate `X-Request-ID` to downstream |
| Upstream error handling | P2 | S | Return proper 502/503 errors when downstream services fail |
| CORS configuration | P2 | S | Make CORS configurable (not allow-all in production) |

### 2.2 AEX-WORK-PUBLISHER

| Task | Priority | Effort | Description |
|------|----------|--------|-------------|
| GET /v1/work list endpoint | P1 | M | Add paginated list endpoint with filters (consumer_id, status, category) |
| WebSocket streaming | P2 | L | Add `WS /v1/work/{work_id}/stream` for real-time updates |
| Provider webhook delivery | P1 | M | Actually POST work opportunities to provider webhook URLs |
| Work spec validation | P2 | M | Validate full work spec schema (category format, budget constraints, etc.) |
| Bid window background closer | P1 | M | Background goroutine to close bid windows after timeout |
| Pub/Sub integration | P0 | M | See Pub/Sub section above |

### 2.3 AEX-BID-GATEWAY

| Task | Priority | Effort | Description |
|------|----------|--------|-------------|
| Pub/Sub integration | P0 | S | Publish `bid.submitted` events |
| Bid expiration validation | P2 | S | Reject bids where `expires_at` is in the past |
| Work status validation | P1 | M | Check work is in OPEN status before accepting bids |

### 2.4 AEX-BID-EVALUATOR

| Task | Priority | Effort | Description |
|------|----------|--------|-------------|
| Pub/Sub consumer | P0 | M | Subscribe to `work.bid_window_closed`, auto-trigger evaluation |
| LLM MVP evaluation | P2 | L | Integrate Vertex AI / Gemini for MVP sample quality scoring |
| Firestore store option | P3 | M | Add Firestore implementation alongside MongoDB |
| Store evaluation results | P1 | S | Persist evaluation results (currently in-memory only in some paths) |
| Pub/Sub publisher | P0 | S | Publish `bids.evaluated` event with winner |

### 2.5 AEX-CONTRACT-ENGINE

| Task | Priority | Effort | Description |
|------|----------|--------|-------------|
| Fix auto-award logic | P1 | M | Use top-ranked bid from evaluator instead of lowest price |
| Fetch evaluation results | P1 | M | Add client to call bid-evaluator for ranked bids |
| Provider webhook notification | P1 | M | POST contract award to provider's A2A endpoint |
| Update work status | P1 | S | Call work-publisher to update work status (AWARDED, COMPLETED, etc.) |
| Contract expiry background job | P1 | M | Background goroutine to expire/fail contracts past `expires_at` |
| Execution token validation | P2 | S | Validate execution token on progress/complete/fail endpoints |
| Pub/Sub integration | P0 | M | Publish contract events, consume `bids.evaluated` for auto-award |

### 2.6 AEX-PROVIDER-REGISTRY

| Task | Priority | Effort | Description |
|------|----------|--------|-------------|
| PUT /v1/providers/{id} | P1 | M | Update provider details (name, endpoint, capabilities) |
| DELETE /v1/providers/{id} | P1 | M | Deactivate/remove provider |
| DELETE /v1/subscriptions/{id} | P1 | S | Remove subscription |
| A2A endpoint validation | P1 | M | Probe `{endpoint}/.well-known/a2a` on registration |
| Trust Broker integration | P1 | S | Call trust broker for initial trust score instead of hardcoded 0.3 |
| Provider status management | P2 | M | Implement PENDING_VERIFICATION → ACTIVE flow |
| Pub/Sub integration | P0 | S | Publish `provider.registered`, `provider.status_changed` events |

### 2.7 AEX-TRUST-BROKER

| Task | Priority | Effort | Description |
|------|----------|--------|-------------|
| Add PREFERRED tier | P1 | S | Score >= 0.9 AND contracts >= 100 |
| Add INTERNAL tier | P1 | S | For enterprise-managed agents (trust = 1.0) |
| Identity verification modifier | P1 | M | +0.05 for verified identity |
| Endpoint verification modifier | P1 | M | +0.05 for verified A2A endpoint |
| Tenure bonus modifier | P1 | M | +0.02/month good standing (max +0.1) |
| Dispute penalty modifier | P1 | M | -0.1 per open dispute |
| Compliance penalty modifier | P1 | S | -0.2 per compliance violation |
| GET /v1/providers/{id}/history | P1 | M | Return paginated contract outcome history |
| POST /v1/disputes | P1 | L | Create dispute against provider/consumer |
| GET /v1/disputes | P2 | M | List disputes with filters |
| PUT /v1/disputes/{id} | P2 | M | Update dispute status (resolve) |
| POST /internal/v1/providers/{id}/verify | P1 | M | Admin endpoint to verify provider identity/endpoint |
| BigQuery export | P2 | M | Export outcomes to BigQuery for analytics |
| Pub/Sub integration | P0 | M | Consume contract events, publish trust events |

### 2.8 AEX-IDENTITY

| Task | Priority | Effort | Description |
|------|----------|--------|-------------|
| PUT /v1/tenants/{id} | P1 | M | Update tenant details |
| POST /v1/tenants/{id}/api-keys/{key_id}/rotate | P1 | M | Revoke old key, create new key atomically |
| POST /v1/tenants/{id}/users | P1 | M | Add user to tenant (link Firebase UID) |
| GET /v1/tenants/{id}/users | P1 | S | List tenant users |
| PUT /v1/tenants/{id}/users/{user_id} | P1 | S | Update user role |
| DELETE /v1/tenants/{id}/users/{user_id} | P1 | S | Remove user from tenant |
| POST /internal/v1/tenants/{id}/usage | P1 | M | Record usage for quota tracking |
| Quota enforcement | P1 | M | Check quotas before allowing requests (integrate with gateway) |
| Redis cache | P1 | M | Cache tenant and API key lookups |
| Secret Manager | P1 | M | Store API key hashes in Secret Manager |
| Pub/Sub integration | P2 | S | Publish tenant/apikey events |

### 2.9 AEX-SETTLEMENT

| Task | Priority | Effort | Description |
|------|----------|--------|-------------|
| POST /v1/withdraw | P1 | M | Process withdrawal requests |
| Pub/Sub consumer | P0 | M | Subscribe to `contract.completed`, auto-trigger settlement |
| Trust Broker integration | P1 | M | Record outcome to trust broker after settlement |
| PostgreSQL migration | P2 | L | Migrate from MongoDB to Cloud SQL for true ACID |
| BigQuery export | P2 | M | Export executions to BigQuery for analytics |
| Insufficient funds handling | P1 | M | Proper error handling and alerts when consumer balance insufficient |
| Pub/Sub publisher | P0 | S | Publish `settlement.completed` event |

### 2.10 AEX-TELEMETRY

Current implementation is an MVP placeholder. Full implementation requires significant work.

| Task | Priority | Effort | Description |
|------|----------|--------|-------------|
| OpenTelemetry Collector | P1 | L | Replace custom REST API with OTel Collector |
| OTLP gRPC receiver | P1 | M | Add port 4317 for gRPC telemetry ingestion |
| OTLP HTTP receiver | P1 | M | Add port 4318 for HTTP telemetry ingestion |
| Cloud Monitoring exporter | P1 | M | Export metrics to GCP Cloud Monitoring |
| Cloud Logging exporter | P1 | M | Export logs to GCP Cloud Logging |
| Cloud Trace exporter | P1 | M | Export traces to GCP Cloud Trace |
| BigQuery exporter | P2 | M | Export metrics/traces to BigQuery for analytics |
| Prometheus scraping | P2 | M | Scrape /metrics from all services |
| Tail-based sampling | P2 | M | Keep all error traces, sample normal traces |
| Service instrumentation | P1 | L | Add OTel SDK to all 9 other services |

---

## 3. Shared Library Gaps

### 3.1 internal/events

| Task | Priority | Effort | Description |
|------|----------|--------|-------------|
| Pub/Sub publisher implementation | P0 | M | Real GCP Pub/Sub client |
| Pub/Sub subscriber helper | P0 | M | Helper for creating push subscription handlers |
| Event schema validation | P2 | S | Validate event payloads against schemas |
| Idempotency handling | P2 | M | Deduplicate events using idempotency key |

### 3.2 internal/httpclient

| Task | Priority | Effort | Description |
|------|----------|--------|-------------|
| Circuit breaker | P2 | M | Add circuit breaker pattern for downstream calls |
| Request tracing | P2 | S | Propagate trace context in headers |

### 3.3 New: internal/cache

| Task | Priority | Effort | Description |
|------|----------|--------|-------------|
| Redis client wrapper | P1 | M | Shared Redis client with connection pooling |
| Cache helpers | P1 | S | Get/Set/Delete with TTL helpers |
| Cache-aside pattern | P2 | M | Helper for cache-aside pattern |

---

## 4. Testing Gaps

| Task | Priority | Effort | Description |
|------|----------|--------|-------------|
| Pub/Sub integration tests | P1 | M | Test event publishing and consumption |
| Redis integration tests | P1 | S | Test rate limiting and caching |
| End-to-end with real infra | P1 | L | Docker Compose with Pub/Sub emulator, Redis |
| Load tests | P2 | M | k6 scripts for gateway throughput testing |
| Contract tests | P2 | M | Pact or similar for service contract testing |

---

## 5. Documentation Gaps

| Task | Priority | Effort | Description |
|------|----------|--------|-------------|
| API documentation | P2 | M | OpenAPI/Swagger specs for all services |
| Deployment runbook | P2 | M | Step-by-step production deployment guide |
| Monitoring runbook | P2 | M | Alert response procedures |
| Architecture diagrams | P3 | S | Update diagrams to reflect actual implementation |

---

## Implementation Order

### Sprint 1: Infrastructure Foundation (P0)
1. Pub/Sub topics and publisher implementation
2. Redis integration for gateway rate limiting
3. Wire Pub/Sub to work-publisher, bid-evaluator, settlement

### Sprint 2: Event-Driven Flow (P0/P1)
1. Bid-evaluator Pub/Sub consumer (auto-trigger on bid window close)
2. Settlement Pub/Sub consumer (auto-trigger on contract complete)
3. Trust-broker Pub/Sub consumer (record outcomes)
4. Contract-engine event publishing

### Sprint 3: Service Completeness (P1)
1. Fix contract-engine auto-award logic
2. Provider webhook notifications
3. Trust-broker full tier system and modifiers
4. Missing CRUD endpoints across services

### Sprint 4: Auth & Security (P1)
1. Firebase Auth / JWT validation in gateway
2. Secret Manager for API keys
3. Quota enforcement

### Sprint 5: Telemetry Overhaul (P1)
1. OpenTelemetry Collector deployment
2. Service instrumentation
3. GCP exporters (Monitoring, Logging, Trace)

### Sprint 6: Production Hardening (P2)
1. PostgreSQL migration for settlement
2. BigQuery exports
3. Load testing
4. Documentation

---

## Effort Estimates

- **S (Small)**: < 1 day
- **M (Medium)**: 1-3 days
- **L (Large)**: 3-5 days

---

## Dependencies

```
Pub/Sub topics → Pub/Sub publisher → Service event wiring
Redis setup → Gateway rate limiting → API key caching
Firebase project → JWT validator → User management
Secret Manager → API key storage → Gateway cache warming
OTel Collector → Service instrumentation → GCP exporters
```

---

## Success Criteria for Phase A Completion

- [ ] All 10 services deployed and healthy
- [ ] Pub/Sub event-driven flow working end-to-end
- [ ] Redis-backed rate limiting in gateway
- [ ] Both API key and JWT authentication working
- [ ] All spec'd API endpoints implemented
- [ ] Trust broker with full tier system and modifiers
- [ ] Settlement with trust broker integration
- [ ] Telemetry collecting metrics from all services
- [ ] Integration tests passing with real infrastructure
- [ ] Load test: Gateway handles 5000 RPS with P95 < 100ms
