# aex-identity Service Specification

## Overview

**Purpose:** Identity and access management for the AEX platform. Manages tenants, API keys, users, roles, and authentication coordination with Firebase Auth.

**Language:** Go 1.22+
**Framework:** Chi router (net/http)
**Runtime:** Cloud Run
**Port:** 8080

## Architecture Position

```
                    External Auth
                   (Firebase Auth)
                         │
                         ▼
              ┌─────────────────┐
              │   aex-identity  │◄── THIS SERVICE
              │                 │
              │ • Tenant mgmt   │
              │ • API keys      │
              │ • User/roles    │
              │ • Auth coord    │
              └────────┬────────┘
                       │
         ┌─────────────┼─────────────┐
         ▼             ▼             ▼
    Firestore     Secret Mgr      Redis
    (tenants)     (API keys)     (sessions)
                       │
                       ▼
              ┌─────────────────┐
              │   aex-gateway   │
              │ (validates keys)│
              └─────────────────┘
```

## Core Responsibilities

1. **Tenant Management** - Create, update, suspend tenants
2. **API Key Management** - Generate, rotate, revoke API keys
3. **User Management** - Link Firebase users to tenants
4. **Role Management** - Define and assign roles/permissions
5. **Provider/Requestor Accounts** - Manage account types
6. **Quota Management** - Set and enforce tenant quotas

## API Endpoints

### Tenant Management

#### POST /v1/tenants

Create a new tenant.

**Request:**
```json
{
  "name": "Acme Corp",
  "type": "BOTH",
  "contact_email": "admin@acme.com",
  "billing_email": "billing@acme.com",
  "metadata": {
    "company_size": "enterprise",
    "industry": "technology"
  }
}
```

**Response (201 Created):**
```json
{
  "id": "tenant_550e8400-e29b-41d4-a716-446655440000",
  "external_id": "acme-corp",
  "name": "Acme Corp",
  "type": "BOTH",
  "status": "ACTIVE",
  "created_at": "2025-01-15T10:30:00Z",
  "api_key": {
    "id": "key_123",
    "key": "ak_live_xxxxxxxxxxxxxxxxxxxx",
    "prefix": "ak_live_xxxx"
  },
  "quotas": {
    "requests_per_minute": 1000,
    "requests_per_day": 100000,
    "max_agents": 100
  }
}
```

#### GET /v1/tenants/{tenant_id}

Get tenant details.

**Response:**
```json
{
  "id": "tenant_550e8400",
  "external_id": "acme-corp",
  "name": "Acme Corp",
  "type": "BOTH",
  "status": "ACTIVE",
  "contact_email": "admin@acme.com",
  "billing_email": "billing@acme.com",
  "quotas": {
    "requests_per_minute": 1000,
    "requests_per_day": 100000,
    "max_agents": 100
  },
  "usage": {
    "requests_today": 45000,
    "agents_registered": 12
  },
  "created_at": "2025-01-15T10:30:00Z",
  "updated_at": "2025-01-20T15:00:00Z"
}
```

#### PUT /v1/tenants/{tenant_id}

Update tenant configuration.

**Request:**
```json
{
  "name": "Acme Corporation",
  "billing_email": "new-billing@acme.com",
  "quotas": {
    "requests_per_minute": 2000
  }
}
```

#### POST /v1/tenants/{tenant_id}/suspend

Suspend a tenant.

**Response:**
```json
{
  "id": "tenant_550e8400",
  "status": "SUSPENDED",
  "suspended_at": "2025-01-25T10:30:00Z",
  "reason": "billing_overdue"
}
```

#### POST /v1/tenants/{tenant_id}/activate

Reactivate a suspended tenant.

### API Key Management

#### POST /v1/tenants/{tenant_id}/api-keys

Generate a new API key.

**Request:**
```json
{
  "name": "Production Key",
  "scopes": ["tasks:write", "tasks:read", "agents:read"],
  "expires_at": "2026-01-15T00:00:00Z"
}
```

**Response (201 Created):**
```json
{
  "id": "key_789",
  "name": "Production Key",
  "key": "ak_live_xxxxxxxxxxxxxxxxxxxx",
  "prefix": "ak_live_xxxx",
  "scopes": ["tasks:write", "tasks:read", "agents:read"],
  "created_at": "2025-01-15T10:30:00Z",
  "expires_at": "2026-01-15T00:00:00Z",
  "last_used_at": null
}
```

**Note:** The full key is only returned once at creation. Store it securely.

#### GET /v1/tenants/{tenant_id}/api-keys

List all API keys for a tenant.

**Response:**
```json
{
  "api_keys": [
    {
      "id": "key_789",
      "name": "Production Key",
      "prefix": "ak_live_xxxx",
      "scopes": ["tasks:write", "tasks:read", "agents:read"],
      "status": "ACTIVE",
      "created_at": "2025-01-15T10:30:00Z",
      "expires_at": "2026-01-15T00:00:00Z",
      "last_used_at": "2025-01-20T14:30:00Z"
    }
  ]
}
```

#### DELETE /v1/tenants/{tenant_id}/api-keys/{key_id}

Revoke an API key.

**Response:**
```json
{
  "id": "key_789",
  "status": "REVOKED",
  "revoked_at": "2025-01-25T10:30:00Z"
}
```

#### POST /v1/tenants/{tenant_id}/api-keys/{key_id}/rotate

Rotate an API key (revoke old, create new).

**Response:**
```json
{
  "old_key": {
    "id": "key_789",
    "status": "REVOKED"
  },
  "new_key": {
    "id": "key_790",
    "key": "ak_live_yyyyyyyyyyyyyyyyyyyy",
    "prefix": "ak_live_yyyy"
  }
}
```

### User Management

#### POST /v1/tenants/{tenant_id}/users

Add a user to a tenant.

**Request:**
```json
{
  "firebase_uid": "firebase_user_123",
  "email": "user@acme.com",
  "role": "ADMIN"
}
```

**Response:**
```json
{
  "id": "user_456",
  "tenant_id": "tenant_550e8400",
  "firebase_uid": "firebase_user_123",
  "email": "user@acme.com",
  "role": "ADMIN",
  "status": "ACTIVE",
  "created_at": "2025-01-15T10:30:00Z"
}
```

#### GET /v1/tenants/{tenant_id}/users

List tenant users.

#### PUT /v1/tenants/{tenant_id}/users/{user_id}

Update user role.

#### DELETE /v1/tenants/{tenant_id}/users/{user_id}

Remove user from tenant.

### Internal API (Service-to-Service)

#### POST /internal/v1/api-keys/validate

Validate an API key (called by aex-gateway).

**Request:**
```json
{
  "api_key": "ak_live_xxxxxxxxxxxxxxxxxxxx"
}
```

**Response:**
```json
{
  "valid": true,
  "tenant_id": "tenant_550e8400",
  "tenant_external_id": "acme-corp",
  "tenant_type": "BOTH",
  "tenant_status": "ACTIVE",
  "scopes": ["tasks:write", "tasks:read", "agents:read"],
  "quotas": {
    "requests_per_minute": 1000,
    "requests_per_day": 100000
  }
}
```

#### GET /internal/v1/tenants/{tenant_id}/quotas

Get tenant quotas for enforcement.

#### POST /internal/v1/tenants/{tenant_id}/usage

Record usage for quota tracking.

## Data Model

### Firestore Collections

```go
type TenantType string

const (
	TenantTypeRequestor TenantType = "REQUESTOR"
	TenantTypeProvider  TenantType = "PROVIDER"
	TenantTypeBoth      TenantType = "BOTH"
)

type TenantStatus string

const (
	TenantStatusPending    TenantStatus = "PENDING"
	TenantStatusActive     TenantStatus = "ACTIVE"
	TenantStatusSuspended  TenantStatus = "SUSPENDED"
	TenantStatusTerminated TenantStatus = "TERMINATED"
)

type Quotas struct {
	RequestsPerMinute   int   `json:"requests_per_minute"`
	RequestsPerDay      int   `json:"requests_per_day"`
	MaxAgents           int   `json:"max_agents"`
	MaxConcurrentTasks  int   `json:"max_concurrent_tasks"`
	MaxTaskPayloadBytes int64 `json:"max_task_payload_bytes"`
}

type Tenant struct {
	ID          string      `json:"id"`
	ExternalID  string      `json:"external_id"`
	Name        string      `json:"name"`
	Type        TenantType  `json:"type"`
	Status      TenantStatus `json:"status"`
	ContactEmail string     `json:"contact_email"`
	BillingEmail string     `json:"billing_email"`
	Quotas      Quotas      `json:"quotas"`
	Metadata    map[string]any `json:"metadata"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
	SuspendedAt *time.Time  `json:"suspended_at,omitempty"`
	SuspensionReason *string `json:"suspension_reason,omitempty"`
}

type APIKeyStatus string

const (
	APIKeyStatusActive  APIKeyStatus = "ACTIVE"
	APIKeyStatusRevoked APIKeyStatus = "REVOKED"
	APIKeyStatusExpired APIKeyStatus = "EXPIRED"
)

type APIKey struct {
	ID        string      `json:"id"`
	TenantID  string      `json:"tenant_id"`
	Name      string      `json:"name"`
	KeyHash   string      `json:"-"`
	Prefix    string      `json:"prefix"`
	Scopes    []string    `json:"scopes"`
	Status    APIKeyStatus `json:"status"`
	CreatedAt time.Time   `json:"created_at"`
	ExpiresAt *time.Time  `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt *time.Time  `json:"revoked_at,omitempty"`
}

type UserRole string

const (
	UserRoleOwner     UserRole = "OWNER"
	UserRoleAdmin     UserRole = "ADMIN"
	UserRoleDeveloper UserRole = "DEVELOPER"
	UserRoleViewer    UserRole = "VIEWER"
)

type TenantUser struct {
	ID         string   `json:"id"`
	TenantID   string   `json:"tenant_id"`
	FirebaseUID string  `json:"firebase_uid"`
	Email      string   `json:"email"`
	Role       UserRole `json:"role"`
	Status     string   `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
```

### Firestore Structure

```
tenants/
  {tenant_id}/
    - id: string
    - external_id: string
    - name: string
    - type: string
    - status: string
    - contact_email: string
    - billing_email: string
    - quotas: map
    - metadata: map
    - created_at: timestamp
    - updated_at: timestamp

    api_keys/
      {key_id}/
        - id: string
        - name: string
        - key_hash: string
        - prefix: string
        - scopes: array
        - status: string
        - created_at: timestamp
        - expires_at: timestamp
        - last_used_at: timestamp

    users/
      {user_id}/
        - id: string
        - firebase_uid: string
        - email: string
        - role: string
        - status: string
        - created_at: timestamp
```

### Secret Manager Structure

```
# API key secrets (for validation cache warming)
aex-api-keys/
  - JSON map of key_hash -> tenant_id + scopes

# Individual key storage (backup)
aex-api-key-{key_id}/
  - Encrypted key value
```

### Redis Cache Structure

```
# API key validation cache (TTL: 5 minutes)
apikey:{key_hash} -> {
  "tenant_id": "tenant_550e8400",
  "tenant_status": "ACTIVE",
  "scopes": ["tasks:write", "tasks:read"],
  "quotas": {...}
}

# Tenant cache (TTL: 5 minutes)
tenant:{tenant_id} -> {full tenant document}

# Rate limit counters (TTL: 1 minute)
ratelimit:{tenant_id}:{minute} -> count

# Daily usage (TTL: 25 hours)
usage:{tenant_id}:{date} -> count
```

## API Key Generation

```go
func generateAPIKey(environment string) (fullKey string, keyHash string, prefix string, err error) {
	// Generate 32 random bytes
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", "", err
	}

	// Encode as URL-safe base64 without padding
	keyBody := base64.RawURLEncoding.EncodeToString(b[:])
	fullKey = "ak_" + environment + "_" + keyBody

	// Create hash for storage (hex SHA-256)
	sum := sha256.Sum256([]byte(fullKey))
	keyHash = hex.EncodeToString(sum[:])

	// Extract prefix for display
	if len(fullKey) > 12 {
		prefix = fullKey[:12] + "..."
	} else {
		prefix = fullKey
	}
	return fullKey, keyHash, prefix, nil
}

func validateAPIKey(providedKey string, storedHash string) bool {
	sum := sha256.Sum256([]byte(providedKey))
	providedHash := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(providedHash), []byte(storedHash)) == 1
}
```

## Quota Enforcement

```go
type QuotaService struct {
	redis     RedisClient
	firestore FirestoreClient
}

func (q *QuotaService) CheckQuota(ctx context.Context, tenantID string, quotaType string) (allowed bool, current int, limit int, err error) {
	tenant, err := q.getTenant(ctx, tenantID)
	if err != nil {
		return false, 0, 0, err
	}

	switch quotaType {
	case "requests_per_minute":
		bucket := time.Now().UTC().Format("200601021504")
		key := "ratelimit:" + tenantID + ":" + bucket
		current, err := q.redis.GetInt(ctx, key)
		if err != nil {
			return false, 0, 0, err
		}
		limit = tenant.Quotas.RequestsPerMinute
		return current < limit, current, limit, nil

	case "requests_per_day":
		day := time.Now().UTC().Format("2006-01-02")
		key := "usage:" + tenantID + ":" + day
		current, err := q.redis.GetInt(ctx, key)
		if err != nil {
			return false, 0, 0, err
		}
		limit = tenant.Quotas.RequestsPerDay
		return current < limit, current, limit, nil

	default:
		return true, 0, 0, nil
	}
}

func (q *QuotaService) RecordUsage(ctx context.Context, tenantID string) error {
	bucket := time.Now().UTC().Format("200601021504")
	day := time.Now().UTC().Format("2006-01-02")

	pipe := q.redis.Pipeline()
	pipe.Incr(ctx, "ratelimit:"+tenantID+":"+bucket)
	pipe.Expire(ctx, "ratelimit:"+tenantID+":"+bucket, 2*time.Minute)
	pipe.Incr(ctx, "usage:"+tenantID+":"+day)
	pipe.Expire(ctx, "usage:"+tenantID+":"+day, 25*time.Hour)
	return pipe.Exec(ctx)
}
```

## Scopes & Permissions

### Available Scopes

```go
var Scopes = map[string]string{
	"tasks:write":  "Submit and cancel tasks",
	"tasks:read":   "View task status and results",
	"agents:write": "Register and manage agents",
	"agents:read":  "View agent details",
	"usage:read":   "View usage statistics",
	"billing:read": "View billing information",
	"admin:users":  "Manage tenant users",
	"admin:keys":   "Manage API keys",
	"*":            "Full access",
}

func CheckScope(required string, granted []string) bool {
	for _, g := range granted {
		if g == "*" || g == required {
			return true
		}
	}
	// Parent scope: "tasks:*" covers "tasks:read"
	if i := strings.LastIndexByte(required, ':'); i >= 0 {
		parent := required[:i] + ":*"
		for _, g := range granted {
			if g == parent {
				return true
			}
		}
	}
	return false
}
```

## Event Publishing

### tenant.created

```json
{
  "event_type": "tenant.created",
  "tenant_id": "tenant_550e8400",
  "data": {
    "external_id": "acme-corp",
    "name": "Acme Corp",
    "type": "BOTH"
  },
  "timestamp": "2025-01-15T10:30:00Z"
}
```

### tenant.suspended

```json
{
  "event_type": "tenant.suspended",
  "tenant_id": "tenant_550e8400",
  "data": {
    "reason": "billing_overdue"
  },
  "timestamp": "2025-01-25T10:30:00Z"
}
```

### apikey.created / apikey.revoked

```json
{
  "event_type": "apikey.revoked",
  "tenant_id": "tenant_550e8400",
  "data": {
    "key_id": "key_789",
    "prefix": "ak_live_xxxx"
  },
  "timestamp": "2025-01-25T10:30:00Z"
}
```

## Configuration

### Environment Variables

```bash
# Server
PORT=8080
ENV=production

# Firestore
GOOGLE_CLOUD_PROJECT=aex-prod

# Redis
REDIS_HOST=10.0.0.5
REDIS_PORT=6379

# Secret Manager
SECRET_PROJECT=aex-prod

# Firebase
FIREBASE_PROJECT_ID=aex-prod

# Pub/Sub
PUBSUB_PROJECT_ID=aex-prod
PUBSUB_TOPIC_IDENTITY=aex-identity

# Defaults
DEFAULT_REQUESTS_PER_MINUTE=1000
DEFAULT_REQUESTS_PER_DAY=100000
DEFAULT_MAX_AGENTS=100

# Observability
LOG_LEVEL=info
```

## Observability

### Metrics

```
# Tenant metrics
aex_identity_tenants_total{type, status}
aex_identity_tenant_operations_total{operation}

# API key metrics
aex_identity_apikeys_total{status}
aex_identity_apikey_validations_total{result}
aex_identity_apikey_validation_duration_seconds

# Quota metrics
aex_identity_quota_checks_total{quota_type, result}
aex_identity_quota_exceeded_total{tenant_id, quota_type}
```

### Logging

```go
slog.Info("tenant_created",
	"tenant_id", tenant.ID,
	"external_id", tenant.ExternalID,
	"type", tenant.Type,
)

slog.Info("apikey_validated",
	"tenant_id", tenantID,
	"key_prefix", prefix,
	"scopes", scopes,
)

slog.Warn("quota_exceeded",
	"tenant_id", tenantID,
	"quota_type", "requests_per_minute",
	"current", 1001,
	"limit", 1000,
)
```

## Directory Structure

```
aex-identity/
├── cmd/
│   └── identity/
│       └── main.go
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── api/
│   │   ├── tenants.go
│   │   ├── apikeys.go
│   │   ├── users.go
│   │   └── internal.go
│   ├── model/
│   │   ├── tenant.go
│   │   ├── apikey.go
│   │   └── user.go
│   ├── service/
│   │   ├── tenant.go
│   │   ├── apikey.go
│   │   ├── user.go
│   │   └── quota.go
│   ├── store/
│   │   ├── firestore.go
│   │   └── secretmanager.go
│   └── events/
│       └── publisher.go
├── hack/
│   └── tests/
├── Dockerfile
├── go.mod
└── go.sum
```

## Dependencies

```
github.com/go-chi/chi/v5
cloud.google.com/go/firestore
cloud.google.com/go/secretmanager
cloud.google.com/go/pubsub
github.com/redis/go-redis/v9
firebase.google.com/go/v4
```

## Deployment

```bash
gcloud run deploy aex-identity \
  --image gcr.io/PROJECT/aex-identity:latest \
  --region us-central1 \
  --platform managed \
  --no-allow-unauthenticated \
  --min-instances 1 \
  --max-instances 10 \
  --memory 512Mi \
  --cpu 1
```

## Integration with aex-gateway

The gateway calls aex-identity for API key validation:

```go
// In aex-gateway
func (g *Gateway) validateAPIKey(ctx context.Context, apiKey string) (*TenantInfo, error) {
    // Check Redis cache first
    cached, err := g.redis.Get(ctx, "apikey:"+hash(apiKey)).Result()
    if err == nil {
        return parseTenantInfo(cached), nil
    }

    // Call aex-identity
    resp, err := g.identityClient.ValidateAPIKey(ctx, &ValidateRequest{
        ApiKey: apiKey,
    })
    if err != nil {
        return nil, err
    }

    // Cache result
    g.redis.SetEx(ctx, "apikey:"+hash(apiKey), resp, 5*time.Minute)

    return resp, nil
}
```
