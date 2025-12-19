# aex-identity Service Specification

## Overview

**Purpose:** Identity and access management for the AEX platform. Manages tenants, API keys, users, roles, and authentication coordination with Firebase Auth.

**Language:** Python 3.11+
**Framework:** FastAPI + Pydantic
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

```python
from pydantic import BaseModel, Field, EmailStr
from datetime import datetime
from enum import Enum
from typing import Optional, List, Dict, Any

class TenantType(str, Enum):
    REQUESTOR = "REQUESTOR"   # Can only submit tasks
    PROVIDER = "PROVIDER"     # Can only register agents
    BOTH = "BOTH"             # Can do both

class TenantStatus(str, Enum):
    PENDING = "PENDING"       # Awaiting verification
    ACTIVE = "ACTIVE"
    SUSPENDED = "SUSPENDED"
    TERMINATED = "TERMINATED"

class Quotas(BaseModel):
    requests_per_minute: int = 1000
    requests_per_day: int = 100000
    max_agents: int = 100
    max_concurrent_tasks: int = 50
    max_task_payload_bytes: int = 1048576  # 1MB

class Tenant(BaseModel):
    id: str
    external_id: str          # URL-safe identifier
    name: str
    type: TenantType
    status: TenantStatus = TenantStatus.ACTIVE
    contact_email: EmailStr
    billing_email: EmailStr
    quotas: Quotas = Field(default_factory=Quotas)
    metadata: Dict[str, Any] = Field(default_factory=dict)
    created_at: datetime
    updated_at: datetime
    suspended_at: Optional[datetime] = None
    suspension_reason: Optional[str] = None

class APIKeyStatus(str, Enum):
    ACTIVE = "ACTIVE"
    REVOKED = "REVOKED"
    EXPIRED = "EXPIRED"

class APIKey(BaseModel):
    id: str
    tenant_id: str
    name: str
    key_hash: str             # SHA-256 hash of key
    prefix: str               # First 12 chars for identification
    scopes: List[str]
    status: APIKeyStatus = APIKeyStatus.ACTIVE
    created_at: datetime
    expires_at: Optional[datetime] = None
    last_used_at: Optional[datetime] = None
    revoked_at: Optional[datetime] = None

class UserRole(str, Enum):
    OWNER = "OWNER"           # Full access
    ADMIN = "ADMIN"           # Manage users, keys
    DEVELOPER = "DEVELOPER"   # Use API, view stats
    VIEWER = "VIEWER"         # Read-only

class TenantUser(BaseModel):
    id: str
    tenant_id: str
    firebase_uid: str
    email: EmailStr
    role: UserRole
    status: str = "ACTIVE"
    created_at: datetime
    updated_at: datetime
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

```python
import secrets
import hashlib
from base64 import urlsafe_b64encode

def generate_api_key(environment: str = "live") -> tuple[str, str, str]:
    """
    Generate a new API key.
    Returns: (full_key, key_hash, prefix)
    """
    # Generate 32 random bytes
    random_bytes = secrets.token_bytes(32)

    # Encode as URL-safe base64
    key_body = urlsafe_b64encode(random_bytes).decode('utf-8').rstrip('=')

    # Prefix with environment indicator
    full_key = f"ak_{environment}_{key_body}"

    # Create hash for storage
    key_hash = hashlib.sha256(full_key.encode()).hexdigest()

    # Extract prefix for display
    prefix = full_key[:12] + "..."

    return full_key, key_hash, prefix


def validate_api_key(provided_key: str, stored_hash: str) -> bool:
    """Validate an API key against stored hash."""
    provided_hash = hashlib.sha256(provided_key.encode()).hexdigest()
    return secrets.compare_digest(provided_hash, stored_hash)
```

## Quota Enforcement

```python
from datetime import datetime, date

class QuotaService:
    def __init__(self, redis_client, firestore_client):
        self.redis = redis_client
        self.firestore = firestore_client

    async def check_quota(self, tenant_id: str, quota_type: str) -> tuple[bool, int, int]:
        """
        Check if tenant is within quota.
        Returns: (allowed, current_usage, limit)
        """
        tenant = await self.get_tenant(tenant_id)
        quotas = tenant.quotas

        if quota_type == "requests_per_minute":
            current_minute = datetime.utcnow().strftime("%Y%m%d%H%M")
            key = f"ratelimit:{tenant_id}:{current_minute}"
            current = await self.redis.get(key) or 0
            limit = quotas.requests_per_minute
            return int(current) < limit, int(current), limit

        elif quota_type == "requests_per_day":
            today = date.today().isoformat()
            key = f"usage:{tenant_id}:{today}"
            current = await self.redis.get(key) or 0
            limit = quotas.requests_per_day
            return int(current) < limit, int(current), limit

        return True, 0, 0

    async def record_usage(self, tenant_id: str):
        """Record a request for quota tracking."""
        current_minute = datetime.utcnow().strftime("%Y%m%d%H%M")
        today = date.today().isoformat()

        pipe = self.redis.pipeline()

        # Minute counter
        minute_key = f"ratelimit:{tenant_id}:{current_minute}"
        pipe.incr(minute_key)
        pipe.expire(minute_key, 120)  # 2 minute TTL

        # Daily counter
        day_key = f"usage:{tenant_id}:{today}"
        pipe.incr(day_key)
        pipe.expire(day_key, 90000)  # 25 hour TTL

        await pipe.execute()
```

## Scopes & Permissions

### Available Scopes

```python
SCOPES = {
    # Task operations
    "tasks:write": "Submit and cancel tasks",
    "tasks:read": "View task status and results",

    # Agent operations
    "agents:write": "Register and manage agents",
    "agents:read": "View agent details",

    # Usage/billing
    "usage:read": "View usage statistics",
    "billing:read": "View billing information",

    # Admin
    "admin:users": "Manage tenant users",
    "admin:keys": "Manage API keys",

    # Wildcard
    "*": "Full access"
}

def check_scope(required: str, granted: List[str]) -> bool:
    """Check if required scope is in granted scopes."""
    if "*" in granted:
        return True
    if required in granted:
        return True
    # Check parent scope (e.g., "tasks:*" covers "tasks:read")
    parent = required.rsplit(":", 1)[0] + ":*"
    return parent in granted
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

```python
logger.info("tenant_created",
    tenant_id=tenant.id,
    external_id=tenant.external_id,
    type=tenant.type
)

logger.info("apikey_validated",
    tenant_id=tenant_id,
    key_prefix=prefix,
    scopes=scopes
)

logger.warn("quota_exceeded",
    tenant_id=tenant_id,
    quota_type="requests_per_minute",
    current=1001,
    limit=1000
)
```

## Directory Structure

```
aex-identity/
├── app/
│   ├── __init__.py
│   ├── main.py
│   ├── config.py
│   ├── models/
│   │   ├── tenant.py
│   │   ├── apikey.py
│   │   └── user.py
│   ├── services/
│   │   ├── tenant_service.py
│   │   ├── apikey_service.py
│   │   ├── user_service.py
│   │   └── quota_service.py
│   ├── repositories/
│   │   ├── tenant_repository.py
│   │   └── apikey_repository.py
│   ├── api/
│   │   ├── tenants.py
│   │   ├── apikeys.py
│   │   ├── users.py
│   │   └── internal.py
│   └── events/
│       └── publisher.py
├── tests/
├── Dockerfile
└── requirements.txt
```

## Dependencies

```
fastapi>=0.109.0
uvicorn[standard]>=0.27.0
pydantic>=2.5.0
google-cloud-firestore>=2.14.0
google-cloud-secret-manager>=2.18.0
redis>=5.0.0
firebase-admin>=6.4.0
structlog>=24.1.0
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
