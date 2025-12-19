# aex-gateway Service Specification

## Overview

**Purpose:** Unified API entry point for all AEX external traffic. Handles authentication, authorization, rate limiting, and request routing to internal services.

**Language:** Go 1.22+
**Framework:** Chi router + custom middleware
**Runtime:** Cloud Run (min 1 instance for cold start avoidance)
**Port:** 8080

## Architecture Position

```
                    Internet
                        │
                        ▼
              ┌─────────────────┐
              │   Cloud Load    │
              │    Balancer     │
              └────────┬────────┘
                       │
                       ▼
              ┌─────────────────┐
              │   aex-gateway   │◄── THIS SERVICE
              │                 │
              │  • Auth         │
              │  • Rate Limit   │
              │  • Route        │
              └────────┬────────┘
                       │
         ┌─────────────┼─────────────┐
         ▼             ▼             ▼
   ┌───────────┐ ┌───────────┐ ┌───────────┐
   │aex-task-  │ │aex-agent- │ │aex-       │
   │intake     │ │registry   │ │settlement │
   └───────────┘ └───────────┘ └───────────┘
```

## API Endpoints

### Task Management (→ aex-task-intake)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/tasks` | Submit new task |
| `GET` | `/v1/tasks/{task_id}` | Get task status/result |
| `GET` | `/v1/tasks` | List tasks (paginated, filtered) |
| `DELETE` | `/v1/tasks/{task_id}` | Cancel pending task |
| `WS` | `/v1/tasks/{task_id}/stream` | Stream task updates |

### Agent Management (→ aex-agent-registry)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/agents` | Register new agent |
| `GET` | `/v1/agents/{agent_id}` | Get agent details |
| `GET` | `/v1/agents` | List agents (paginated, filtered) |
| `PUT` | `/v1/agents/{agent_id}` | Update agent config |
| `DELETE` | `/v1/agents/{agent_id}` | Deregister agent |
| `POST` | `/v1/agents/{agent_id}/heartbeat` | Agent health check |

### Capability Discovery (→ aex-agent-registry)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/capabilities` | List all capability domains |
| `GET` | `/v1/capabilities/{domain}` | Get agents for domain |

### Usage & Billing (→ aex-settlement)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/usage` | Get usage summary |
| `GET` | `/v1/usage/transactions` | List transactions |
| `GET` | `/v1/balance` | Get current balance |

### Health & Meta

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check (for LB) |
| `GET` | `/ready` | Readiness check |
| `GET` | `/v1/info` | API version info |

## Authentication

### Supported Methods

1. **API Key** (Primary for Phase A)
   - Header: `X-API-Key: <key>`
   - Keys stored in Secret Manager, cached in Redis
   - Keys scoped to tenant

2. **JWT Bearer Token** (Firebase Auth)
   - Header: `Authorization: Bearer <token>`
   - Validated against Firebase Auth public keys
   - Claims extracted: `tenant_id`, `roles`

### Authentication Flow

```go
func AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // 1. Check API Key header
        if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
            tenant, err := validateAPIKey(apiKey)
            if err != nil {
                respondError(w, 401, "invalid_api_key")
                return
            }
            ctx := context.WithValue(r.Context(), "tenant", tenant)
            next.ServeHTTP(w, r.WithContext(ctx))
            return
        }

        // 2. Check Bearer token
        if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
            token := strings.TrimPrefix(auth, "Bearer ")
            claims, err := validateJWT(token)
            if err != nil {
                respondError(w, 401, "invalid_token")
                return
            }
            ctx := context.WithValue(r.Context(), "tenant", claims.TenantID)
            ctx = context.WithValue(ctx, "roles", claims.Roles)
            next.ServeHTTP(w, r.WithContext(ctx))
            return
        }

        respondError(w, 401, "authentication_required")
    })
}
```

## Rate Limiting

### Strategy: Token Bucket per Tenant

```go
type RateLimitConfig struct {
    // Per-tenant limits
    RequestsPerMinute int     // Default: 1000
    RequestsPerDay    int     // Default: 100000
    BurstSize         int     // Default: 50

    // Per-endpoint overrides
    EndpointLimits map[string]int // e.g., "/v1/tasks": 500/min
}
```

### Implementation

```go
func RateLimitMiddleware(redis *redis.Client) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            tenant := r.Context().Value("tenant").(string)

            // Key: ratelimit:{tenant}:{minute_bucket}
            key := fmt.Sprintf("ratelimit:%s:%d", tenant, time.Now().Unix()/60)

            count, err := redis.Incr(r.Context(), key).Result()
            if err != nil {
                // Fail open on Redis error
                log.Warn("redis error in rate limit", "error", err)
                next.ServeHTTP(w, r)
                return
            }

            if count == 1 {
                redis.Expire(r.Context(), key, 2*time.Minute)
            }

            limit := getRateLimit(tenant, r.URL.Path)
            if count > int64(limit) {
                w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
                w.Header().Set("X-RateLimit-Remaining", "0")
                w.Header().Set("Retry-After", "60")
                respondError(w, 429, "rate_limit_exceeded")
                return
            }

            w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
            w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(limit-int(count)))
            next.ServeHTTP(w, r)
        })
    }
}
```

## Request Routing

### Internal Service URLs (Cloud Run)

```yaml
services:
  task-intake: "https://aex-task-intake-xxxxx.run.app"
  agent-registry: "https://aex-agent-registry-xxxxx.run.app"
  settlement: "https://aex-settlement-xxxxx.run.app"
```

### Routing Table

```go
var routes = map[string]string{
    "/v1/tasks":        "task-intake",
    "/v1/agents":       "agent-registry",
    "/v1/capabilities": "agent-registry",
    "/v1/usage":        "settlement",
    "/v1/balance":      "settlement",
}

func getUpstream(path string) string {
    for prefix, service := range routes {
        if strings.HasPrefix(path, prefix) {
            return services[service]
        }
    }
    return ""
}
```

### Proxy Implementation

```go
func ProxyHandler(w http.ResponseWriter, r *http.Request) {
    upstream := getUpstream(r.URL.Path)
    if upstream == "" {
        respondError(w, 404, "endpoint_not_found")
        return
    }

    // Create proxy request
    proxyURL, _ := url.Parse(upstream)
    proxy := httputil.NewSingleHostReverseProxy(proxyURL)

    // Add internal headers
    r.Header.Set("X-Tenant-ID", r.Context().Value("tenant").(string))
    r.Header.Set("X-Request-ID", r.Context().Value("request_id").(string))

    // Remove external auth headers (already validated)
    r.Header.Del("X-API-Key")
    r.Header.Del("Authorization")

    proxy.ServeHTTP(w, r)
}
```

## Request/Response Handling

### Standard Request Headers

```
X-API-Key: <api_key>              # API key auth
Authorization: Bearer <jwt>        # JWT auth
X-Request-ID: <uuid>              # Client correlation ID (optional)
X-Idempotency-Key: <key>          # For POST requests (optional)
Content-Type: application/json
```

### Standard Response Headers

```
X-Request-ID: <uuid>              # Echo or generate
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 950
X-RateLimit-Reset: 1234567890
Content-Type: application/json
```

### Error Response Format

```json
{
  "error": {
    "code": "rate_limit_exceeded",
    "message": "Rate limit exceeded. Retry after 60 seconds.",
    "request_id": "550e8400-e29b-41d4-a716-446655440000"
  }
}
```

### Error Codes

| HTTP Status | Code | Description |
|-------------|------|-------------|
| 400 | `bad_request` | Invalid request format |
| 401 | `authentication_required` | No auth credentials |
| 401 | `invalid_api_key` | API key not valid |
| 401 | `invalid_token` | JWT not valid |
| 403 | `forbidden` | Not authorized for resource |
| 404 | `not_found` | Resource not found |
| 429 | `rate_limit_exceeded` | Too many requests |
| 500 | `internal_error` | Server error |
| 502 | `upstream_error` | Backend service error |
| 503 | `service_unavailable` | Service temporarily down |

## Configuration

### Environment Variables

```bash
# Server
PORT=8080
ENV=production                    # development|staging|production

# Service Discovery
TASK_INTAKE_URL=https://aex-task-intake-xxx.run.app
AGENT_REGISTRY_URL=https://aex-agent-registry-xxx.run.app
SETTLEMENT_URL=https://aex-settlement-xxx.run.app

# Redis (for rate limiting)
REDIS_HOST=10.0.0.5
REDIS_PORT=6379

# Auth
FIREBASE_PROJECT_ID=aex-prod
API_KEY_SECRET=projects/aex-prod/secrets/api-keys/versions/latest

# Observability
LOG_LEVEL=info                    # debug|info|warn|error
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4317
```

### Cloud Run Configuration

```yaml
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: aex-gateway
spec:
  template:
    metadata:
      annotations:
        autoscaling.knative.dev/minScale: "1"    # Avoid cold starts
        autoscaling.knative.dev/maxScale: "100"
        run.googleapis.com/cpu-throttling: "false"
    spec:
      containerConcurrency: 250
      timeoutSeconds: 30
      containers:
      - image: gcr.io/PROJECT/aex-gateway:latest
        ports:
        - containerPort: 8080
        resources:
          limits:
            cpu: "2"
            memory: "1Gi"
        env:
        - name: PORT
          value: "8080"
        # ... other env vars from Secret Manager
```

## Middleware Stack

Order matters — executed top to bottom on request, bottom to top on response:

```go
func NewRouter() *chi.Mux {
    r := chi.NewRouter()

    // 1. Request ID (first, for tracing)
    r.Use(RequestIDMiddleware)

    // 2. Logging (captures all requests)
    r.Use(LoggingMiddleware)

    // 3. Recovery (catches panics)
    r.Use(RecoveryMiddleware)

    // 4. CORS
    r.Use(CORSMiddleware)

    // 5. Timeout
    r.Use(TimeoutMiddleware(25 * time.Second))

    // Health endpoints (no auth)
    r.Get("/health", HealthHandler)
    r.Get("/ready", ReadyHandler)

    // API routes (with auth)
    r.Route("/v1", func(r chi.Router) {
        r.Use(AuthMiddleware)
        r.Use(RateLimitMiddleware)

        r.HandleFunc("/*", ProxyHandler)
    })

    return r
}
```

## Observability

### Metrics (Prometheus format)

```
# Request metrics
aex_gateway_requests_total{method, path, status}
aex_gateway_request_duration_seconds{method, path}
aex_gateway_request_size_bytes{method, path}
aex_gateway_response_size_bytes{method, path}

# Rate limiting
aex_gateway_rate_limit_hits_total{tenant}

# Upstream
aex_gateway_upstream_requests_total{upstream, status}
aex_gateway_upstream_duration_seconds{upstream}

# Auth
aex_gateway_auth_success_total{method}  # api_key, jwt
aex_gateway_auth_failure_total{method, reason}
```

### Logging

```json
{
  "timestamp": "2025-01-15T10:30:00Z",
  "level": "info",
  "message": "request completed",
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "tenant_id": "tenant-123",
  "method": "POST",
  "path": "/v1/tasks",
  "status": 201,
  "duration_ms": 45,
  "upstream": "task-intake",
  "client_ip": "203.0.113.50"
}
```

### Tracing

- OpenTelemetry instrumentation
- Trace context propagated to upstream services
- Spans: `gateway.auth`, `gateway.ratelimit`, `gateway.proxy`

## Dependencies

### External

| Dependency | Purpose | Failure Mode |
|------------|---------|--------------|
| Redis (Memorystore) | Rate limiting, API key cache | Fail open (allow requests) |
| Secret Manager | API keys | Fail closed (reject requests) |
| Firebase Auth | JWT validation | Fail closed |
| Upstream services | Business logic | Return 502 |

### Go Modules

```go
require (
    github.com/go-chi/chi/v5 v5.0.12
    github.com/redis/go-redis/v9 v9.4.0
    github.com/golang-jwt/jwt/v5 v5.2.0
    go.opentelemetry.io/otel v1.24.0
    go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.49.0
    cloud.google.com/go/secretmanager v1.11.5
    firebase.google.com/go/v4 v4.13.0
)
```

## Testing

### Unit Tests

```bash
go test ./... -cover -coverprofile=coverage.out
# Target: >80% coverage
```

### Integration Tests

```bash
# Start local dependencies
docker-compose -f docker-compose.test.yml up -d

# Run integration tests
go test ./... -tags=integration
```

### Load Tests

```bash
# Using k6
k6 run --vus 100 --duration 60s load-test.js

# Targets:
# - P95 latency < 100ms (excluding upstream)
# - 5000 RPS sustained
# - Error rate < 0.1%
```

## Directory Structure

```
aex-gateway/
├── cmd/
│   └── gateway/
│       └── main.go           # Entry point
├── internal/
│   ├── config/
│   │   └── config.go         # Configuration loading
│   ├── middleware/
│   │   ├── auth.go           # Authentication
│   │   ├── ratelimit.go      # Rate limiting
│   │   ├── logging.go        # Request logging
│   │   ├── recovery.go       # Panic recovery
│   │   ├── cors.go           # CORS handling
│   │   ├── timeout.go        # Request timeout
│   │   └── requestid.go      # Request ID generation
│   ├── proxy/
│   │   └── proxy.go          # Reverse proxy logic
│   ├── auth/
│   │   ├── apikey.go         # API key validation
│   │   └── jwt.go            # JWT validation
│   └── health/
│       └── health.go         # Health checks
├── api/
│   └── errors.go             # Error response types
├── Dockerfile
├── go.mod
├── go.sum
└── README.md
```

## Deployment

### Build

```bash
# Build binary
CGO_ENABLED=0 GOOS=linux go build -o gateway ./cmd/gateway

# Build container
docker build -t gcr.io/PROJECT/aex-gateway:latest .
docker push gcr.io/PROJECT/aex-gateway:latest
```

### Deploy

```bash
gcloud run deploy aex-gateway \
  --image gcr.io/PROJECT/aex-gateway:latest \
  --region us-central1 \
  --platform managed \
  --allow-unauthenticated \
  --min-instances 1 \
  --max-instances 100 \
  --memory 1Gi \
  --cpu 2 \
  --set-env-vars "ENV=production"
```

## Security Considerations

1. **No sensitive data in logs** — Mask API keys, tokens
2. **TLS everywhere** — Cloud Run provides this automatically
3. **Input validation** — Validate Content-Type, size limits
4. **Header sanitization** — Remove internal headers from external requests
5. **Rate limiting** — Prevent DoS, abuse
6. **CORS** — Restrict to known origins in production
