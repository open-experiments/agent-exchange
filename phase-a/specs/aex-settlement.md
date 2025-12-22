# aex-settlement Service Specification

## Overview

**Purpose:** Usage metering, cost calculation, ledger management, and billing. This is the financial backbone of the exchange.

**Language:** Python 3.11+
**Framework:** FastAPI + SQLAlchemy
**Runtime:** Cloud Run
**Port:** 8080
**Database:** Cloud SQL (PostgreSQL)

## Architecture Position

```
              Pub/Sub
       (contract.completed)
                │
                ▼
       ┌────────────────┐
       │ aex-settlement │◄── THIS SERVICE
       │                │
       │ • Meter usage  │
       │ • Calculate $  │
       │ • Update ledger│
       └───────┬────────┘
               │
         ┌─────┴─────┐
         ▼           ▼
    Cloud SQL    BigQuery
    (ledger)    (analytics)
```

## Core Responsibilities

1. **Record executions** from contract.completed events
2. **Calculate costs** based on CPC pricing (Phase A)
3. **Manage ledger** with ACID transactions
4. **Track balances** per tenant
5. **Generate usage reports** for billing

## Data Model

### PostgreSQL Schema

```sql
-- Tenants
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    external_id VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(20) NOT NULL CHECK (type IN ('REQUESTOR', 'PROVIDER', 'BOTH')),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Tenant balances (denormalized for fast queries)
CREATE TABLE tenant_balances (
    tenant_id UUID PRIMARY KEY REFERENCES tenants(id),
    balance DECIMAL(15, 6) NOT NULL DEFAULT 0,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    last_updated TIMESTAMPTZ DEFAULT NOW()
);

-- Execution records (source of truth)
CREATE TABLE executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    work_id VARCHAR(255) NOT NULL,       -- Links to work spec from aex-work-publisher
    contract_id VARCHAR(255) NOT NULL,   -- Links to contract from aex-contract-engine
    agent_id VARCHAR(255) NOT NULL,
    consumer_id UUID NOT NULL REFERENCES tenants(id),
    provider_id UUID NOT NULL REFERENCES tenants(id),
    domain VARCHAR(255) NOT NULL,

    -- Timing
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL,
    duration_ms INT NOT NULL,

    -- Status
    status VARCHAR(20) NOT NULL CHECK (status IN ('COMPLETED', 'FAILED')),
    success BOOLEAN NOT NULL,

    -- Pricing (Phase A: CPC only)
    agreed_price DECIMAL(10, 6) NOT NULL,
    platform_fee DECIMAL(10, 6) NOT NULL,
    provider_payout DECIMAL(10, 6) NOT NULL,

    -- Metadata
    metadata JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),

    -- Indexes
    CONSTRAINT unique_contract_execution UNIQUE (contract_id)
);

CREATE INDEX idx_executions_consumer ON executions(consumer_id, created_at);
CREATE INDEX idx_executions_provider ON executions(provider_id, created_at);
CREATE INDEX idx_executions_domain ON executions(domain, created_at);

-- Ledger entries (immutable, append-only)
CREATE TABLE ledger_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    entry_type VARCHAR(20) NOT NULL CHECK (entry_type IN ('DEBIT', 'CREDIT', 'DEPOSIT', 'WITHDRAWAL')),
    amount DECIMAL(15, 6) NOT NULL,
    balance_after DECIMAL(15, 6) NOT NULL,
    reference_type VARCHAR(50) NOT NULL,  -- 'execution', 'deposit', 'withdrawal'
    reference_id UUID,  -- execution_id or deposit_id
    description VARCHAR(500),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_ledger_tenant ON ledger_entries(tenant_id, created_at);

-- Deposits/withdrawals
CREATE TABLE transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    type VARCHAR(20) NOT NULL CHECK (type IN ('DEPOSIT', 'WITHDRAWAL')),
    amount DECIMAL(15, 6) NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('PENDING', 'COMPLETED', 'FAILED')),
    payment_method VARCHAR(50),
    payment_reference VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);
```

### SQLAlchemy Models

```python
from sqlalchemy import Column, String, Numeric, DateTime, Boolean, ForeignKey, Enum, JSON
from sqlalchemy.dialects.postgresql import UUID
from sqlalchemy.orm import relationship
import uuid

class Tenant(Base):
    __tablename__ = "tenants"

    id = Column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    external_id = Column(String(255), unique=True, nullable=False)
    name = Column(String(255), nullable=False)
    type = Column(String(20), nullable=False)
    created_at = Column(DateTime(timezone=True), server_default=func.now())
    updated_at = Column(DateTime(timezone=True), onupdate=func.now())

    balance = relationship("TenantBalance", back_populates="tenant", uselist=False)

class TenantBalance(Base):
    __tablename__ = "tenant_balances"

    tenant_id = Column(UUID(as_uuid=True), ForeignKey("tenants.id"), primary_key=True)
    balance = Column(Numeric(15, 6), nullable=False, default=0)
    currency = Column(String(3), nullable=False, default="USD")
    last_updated = Column(DateTime(timezone=True), server_default=func.now())

    tenant = relationship("Tenant", back_populates="balance")

class Execution(Base):
    __tablename__ = "executions"

    id = Column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    work_id = Column(String(255), nullable=False)
    contract_id = Column(String(255), unique=True, nullable=False)
    agent_id = Column(String(255), nullable=False)
    consumer_id = Column(UUID(as_uuid=True), ForeignKey("tenants.id"), nullable=False)
    provider_id = Column(UUID(as_uuid=True), ForeignKey("tenants.id"), nullable=False)
    domain = Column(String(255), nullable=False)

    started_at = Column(DateTime(timezone=True), nullable=False)
    completed_at = Column(DateTime(timezone=True), nullable=False)
    duration_ms = Column(Integer, nullable=False)

    status = Column(String(20), nullable=False)
    success = Column(Boolean, nullable=False)

    agreed_price = Column(Numeric(10, 6), nullable=False)
    platform_fee = Column(Numeric(10, 6), nullable=False)
    provider_payout = Column(Numeric(10, 6), nullable=False)

    metadata = Column(JSON)
    created_at = Column(DateTime(timezone=True), server_default=func.now())

class LedgerEntry(Base):
    __tablename__ = "ledger_entries"

    id = Column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    tenant_id = Column(UUID(as_uuid=True), ForeignKey("tenants.id"), nullable=False)
    entry_type = Column(String(20), nullable=False)
    amount = Column(Numeric(15, 6), nullable=False)
    balance_after = Column(Numeric(15, 6), nullable=False)
    reference_type = Column(String(50), nullable=False)
    reference_id = Column(UUID(as_uuid=True))
    description = Column(String(500))
    created_at = Column(DateTime(timezone=True), server_default=func.now())
```

## Settlement Logic

### CPC Cost Calculation (Phase A)

```python
from decimal import Decimal

PLATFORM_FEE_RATE = Decimal("0.15")  # 15%

@dataclass
class CostBreakdown:
    agreed_price: Decimal      # What requestor pays
    platform_fee: Decimal      # AEX takes
    provider_payout: Decimal   # Provider receives

def calculate_cpc_cost(agreed_price: Decimal) -> CostBreakdown:
    """
    Calculate cost breakdown for CPC pricing.

    Example:
        agreed_price = $0.10
        platform_fee = $0.10 * 0.15 = $0.015
        provider_payout = $0.10 - $0.015 = $0.085
    """
    platform_fee = agreed_price * PLATFORM_FEE_RATE
    provider_payout = agreed_price - platform_fee

    return CostBreakdown(
        agreed_price=agreed_price,
        platform_fee=platform_fee.quantize(Decimal("0.000001")),
        provider_payout=provider_payout.quantize(Decimal("0.000001"))
    )
```

### Settlement Transaction

```python
from sqlalchemy.orm import Session
from decimal import Decimal

async def settle_execution(
    db: Session,
    event: ContractCompletedEvent
) -> Execution:
    """
    Settle a completed contract execution with ACID guarantees.
    """
    # Calculate costs
    agreed_price = Decimal(str(event.data.billing.cost))
    cost = calculate_cpc_cost(agreed_price)

    # Get tenant IDs
    requestor = await get_tenant_by_external_id(db, event.tenant_id)
    provider = await get_provider_by_agent_id(db, event.data.agent_id)

    # Start transaction
    try:
        # 1. Create execution record
        execution = Execution(
            work_id=event.work_id,
            contract_id=event.contract_id,
            agent_id=event.data.agent_id,
            consumer_id=consumer.id,
            provider_id=provider.id,
            domain=event.data.domain,
            started_at=event.data.started_at,
            completed_at=event.timestamp,
            duration_ms=event.data.duration_ms,
            status="COMPLETED",
            success=True,
            agreed_price=cost.agreed_price,
            platform_fee=cost.platform_fee,
            provider_payout=cost.provider_payout,
            metadata=event.data.metadata
        )
        db.add(execution)

        # 2. Debit requestor
        requestor_balance = await get_balance_for_update(db, requestor.id)
        new_requestor_balance = requestor_balance.balance - cost.agreed_price

        if new_requestor_balance < 0:
            raise InsufficientFundsError(
                f"Insufficient balance: {requestor_balance.balance} < {cost.agreed_price}"
            )

        requestor_balance.balance = new_requestor_balance
        requestor_balance.last_updated = datetime.utcnow()

        db.add(LedgerEntry(
            tenant_id=requestor.id,
            entry_type="DEBIT",
            amount=cost.agreed_price,
            balance_after=new_requestor_balance,
            reference_type="execution",
            reference_id=execution.id,
            description=f"Contract execution: {event.contract_id}"
        ))

        # 3. Credit provider
        provider_balance = await get_balance_for_update(db, provider.id)
        new_provider_balance = provider_balance.balance + cost.provider_payout

        provider_balance.balance = new_provider_balance
        provider_balance.last_updated = datetime.utcnow()

        db.add(LedgerEntry(
            tenant_id=provider.id,
            entry_type="CREDIT",
            amount=cost.provider_payout,
            balance_after=new_provider_balance,
            reference_type="execution",
            reference_id=execution.id,
            description=f"Agent payout: {event.data.agent_id}"
        ))

        # 4. Commit transaction
        db.commit()

        # 5. Export to BigQuery (async)
        await export_to_bigquery(execution)

        return execution

    except Exception as e:
        db.rollback()
        logger.error("settlement_failed", contract_id=event.contract_id, error=str(e))
        raise
```

### Balance Locking

```python
from sqlalchemy import select, text

async def get_balance_for_update(db: Session, tenant_id: UUID) -> TenantBalance:
    """Get balance with row-level lock for update."""
    result = db.execute(
        select(TenantBalance)
        .where(TenantBalance.tenant_id == tenant_id)
        .with_for_update()
    )
    return result.scalar_one()
```

## Event Handling

### Consumed Events

#### contract.completed

Published by `aex-contract-engine` when a provider completes work.

```python
@app.post("/events/contract.completed")
async def handle_contract_completed(request: Request, db: Session = Depends(get_db)):
    envelope = await request.json()
    event = ContractCompletedEvent.parse_obj(decode_pubsub_message(envelope))

    try:
        execution = await settle_execution(db, event)
        logger.info("settlement_completed",
            contract_id=event.contract_id,
            execution_id=str(execution.id),
            cost=float(execution.agreed_price)
        )
    except InsufficientFundsError as e:
        logger.error("insufficient_funds", contract_id=event.contract_id, error=str(e))
        # Could trigger alert or flag the contract
    except Exception as e:
        logger.error("settlement_error", contract_id=event.contract_id, error=str(e))
        raise

    return {"status": "ok"}
```

#### contract.failed

Published by `aex-contract-engine` when execution fails.

```python
@app.post("/events/contract.failed")
async def handle_contract_failed(request: Request, db: Session = Depends(get_db)):
    """Record failed executions (no billing, but track for analytics)."""
    envelope = await request.json()
    event = ContractFailedEvent.parse_obj(decode_pubsub_message(envelope))

    execution = Execution(
        work_id=event.work_id,
        contract_id=event.contract_id,
        agent_id=event.data.agent_id,
        status="FAILED",
        success=False,
        agreed_price=Decimal("0"),
        platform_fee=Decimal("0"),
        provider_payout=Decimal("0"),
        # ... other fields
    )
    db.add(execution)
    db.commit()

    return {"status": "ok"}
```

## API Endpoints

### GET /v1/usage

Get usage summary for authenticated tenant.

**Response:**
```json
{
  "period": {
    "from": "2025-01-01T00:00:00Z",
    "to": "2025-01-31T23:59:59Z"
  },
  "summary": {
    "total_executions": 15000,
    "successful_executions": 14700,
    "failed_executions": 300,
    "total_cost": 750.50,
    "currency": "USD"
  },
  "by_domain": [
    {
      "domain": "nlp.summarization",
      "executions": 10000,
      "cost": 500.00
    },
    {
      "domain": "nlp.translation",
      "executions": 5000,
      "cost": 250.50
    }
  ]
}
```

### GET /v1/usage/transactions

List transactions with pagination.

**Query Parameters:**
| Param | Type | Description |
|-------|------|-------------|
| `from` | datetime | Start date |
| `to` | datetime | End date |
| `type` | string | Filter by type (DEBIT, CREDIT) |
| `limit` | int | Max results |
| `cursor` | string | Pagination cursor |

**Response:**
```json
{
  "transactions": [
    {
      "id": "txn_123",
      "type": "DEBIT",
      "amount": 0.05,
      "balance_after": 99.95,
      "reference": {
        "type": "execution",
        "work_id": "work_550e8400",
        "contract_id": "contract_789xyz"
      },
      "created_at": "2025-01-15T10:30:05Z"
    }
  ],
  "next_cursor": "..."
}
```

### GET /v1/balance

Get current balance.

**Response:**
```json
{
  "balance": 100.50,
  "currency": "USD",
  "last_updated": "2025-01-15T10:30:05Z"
}
```

### POST /v1/deposit (Phase B)

Add funds to account.

### POST /v1/withdraw (Phase B)

Withdraw funds.

## BigQuery Export

### Export Schema

```sql
-- BigQuery table: aex_analytics.executions
CREATE TABLE aex_analytics.executions (
    execution_id STRING,
    work_id STRING,
    contract_id STRING,
    agent_id STRING,
    consumer_id STRING,
    provider_id STRING,
    domain STRING,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    duration_ms INT64,
    status STRING,
    success BOOL,
    agreed_price FLOAT64,
    platform_fee FLOAT64,
    provider_payout FLOAT64,
    created_at TIMESTAMP
)
PARTITION BY DATE(created_at);
```

### Export Implementation

```python
from google.cloud import bigquery

bq_client = bigquery.Client()

async def export_to_bigquery(execution: Execution):
    """Async export to BigQuery for analytics."""
    row = {
        "execution_id": str(execution.id),
        "work_id": execution.work_id,
        "contract_id": execution.contract_id,
        "agent_id": execution.agent_id,
        "consumer_id": str(execution.consumer_id),
        "provider_id": str(execution.provider_id),
        "domain": execution.domain,
        "started_at": execution.started_at.isoformat(),
        "completed_at": execution.completed_at.isoformat(),
        "duration_ms": execution.duration_ms,
        "status": execution.status,
        "success": execution.success,
        "agreed_price": float(execution.agreed_price),
        "platform_fee": float(execution.platform_fee),
        "provider_payout": float(execution.provider_payout),
        "created_at": execution.created_at.isoformat()
    }

    errors = bq_client.insert_rows_json(
        "aex_analytics.executions",
        [row]
    )

    if errors:
        logger.error("bigquery_export_failed", errors=errors)
```

## Configuration

### Environment Variables

```bash
# Server
PORT=8080
ENV=production

# Database
DATABASE_URL=postgresql://user:pass@/aex_settlement?host=/cloudsql/project:region:instance
DATABASE_POOL_SIZE=10
DATABASE_MAX_OVERFLOW=20

# BigQuery
BIGQUERY_PROJECT=aex-prod
BIGQUERY_DATASET=aex_analytics

# Pub/Sub
PUBSUB_PROJECT_ID=aex-prod
PUBSUB_SUBSCRIPTION_CONTRACT_COMPLETED=aex-settlement-contract-completed-sub
PUBSUB_SUBSCRIPTION_CONTRACT_FAILED=aex-settlement-contract-failed-sub

# Business
PLATFORM_FEE_RATE=0.15

# Observability
LOG_LEVEL=info
```

## Observability

### Metrics

```
# Settlement metrics
aex_settlement_executions_total{domain, status}
aex_settlement_revenue_total{currency}
aex_settlement_payout_total{currency}
aex_settlement_fees_total{currency}

# Latency
aex_settlement_processing_duration_seconds

# Errors
aex_settlement_errors_total{error_type}
aex_settlement_insufficient_funds_total
```

## Directory Structure

```
aex-settlement/
├── app/
│   ├── __init__.py
│   ├── main.py
│   ├── config.py
│   ├── database.py
│   ├── models/
│   │   ├── __init__.py
│   │   ├── tenant.py
│   │   ├── execution.py
│   │   └── ledger.py
│   ├── services/
│   │   ├── settlement.py
│   │   ├── billing.py
│   │   └── export.py
│   ├── api/
│   │   ├── usage.py
│   │   └── balance.py
│   └── events/
│       └── handlers.py
├── migrations/
│   └── versions/
├── tests/
├── Dockerfile
└── requirements.txt
```

## Deployment

```bash
gcloud run deploy aex-settlement \
  --image gcr.io/PROJECT/aex-settlement:latest \
  --region us-central1 \
  --platform managed \
  --no-allow-unauthenticated \
  --add-cloudsql-instances PROJECT:REGION:INSTANCE \
  --min-instances 1 \
  --max-instances 20 \
  --memory 512Mi \
  --cpu 1
```
