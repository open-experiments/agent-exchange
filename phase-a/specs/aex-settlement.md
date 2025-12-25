# aex-settlement Service Specification

## Overview

**Purpose:** Usage metering, cost calculation, ledger management, and billing. This is the financial backbone of the exchange.

**Language:** Go 1.22+
**Framework:** Chi router + pgxpool (PostgreSQL)
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

### Go Models

```go
type Tenant struct {
	ID         uuid.UUID `json:"id"`
	ExternalID string    `json:"external_id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"` // REQUESTOR|PROVIDER|BOTH
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type TenantBalance struct {
	TenantID    uuid.UUID `json:"tenant_id"`
	Balance     string    `json:"balance"` // store/load as DECIMAL string
	Currency    string    `json:"currency"`
	LastUpdated time.Time `json:"last_updated"`
}

type Execution struct {
	ID               uuid.UUID `json:"execution_id"`
	TaskID           string    `json:"task_id"`
	AgentID          string    `json:"agent_id"`
	RequestorTenantID uuid.UUID `json:"requestor_tenant_id"`
	ProviderTenantID  uuid.UUID `json:"provider_tenant_id"`
	Domain           string    `json:"domain"`
	StartedAt        time.Time `json:"started_at"`
	CompletedAt      time.Time `json:"completed_at"`
	DurationMs       int64     `json:"duration_ms"`
	Status           string    `json:"status"`
	Success          bool      `json:"success"`
	AgreedPrice      string    `json:"agreed_price"`
	PlatformFee      string    `json:"platform_fee"`
	ProviderPayout   string    `json:"provider_payout"`
	Metadata         map[string]any `json:"metadata"`
	CreatedAt        time.Time `json:"created_at"`
}

type LedgerEntry struct {
	ID           uuid.UUID `json:"id"`
	TenantID     uuid.UUID `json:"tenant_id"`
	EntryType    string    `json:"entry_type"` // DEBIT|CREDIT|DEPOSIT|WITHDRAWAL
	Amount       string    `json:"amount"`
	BalanceAfter string    `json:"balance_after"`
	ReferenceType string   `json:"reference_type"`
	ReferenceID  *uuid.UUID `json:"reference_id,omitempty"`
	Description  string    `json:"description"`
	CreatedAt    time.Time `json:"created_at"`
}
```

## Settlement Logic

### CPC Cost Calculation (Phase A)

```go
var platformFeeRate = decimal.RequireFromString("0.15") // 15%

type CostBreakdown struct {
	AgreedPrice    decimal.Decimal
	PlatformFee    decimal.Decimal
	ProviderPayout decimal.Decimal
}

func calculateCPCCost(agreedPrice decimal.Decimal) CostBreakdown {
	platformFee := agreedPrice.Mul(platformFeeRate).Round(6)
	providerPayout := agreedPrice.Sub(platformFee).Round(6)
	return CostBreakdown{
		AgreedPrice:    agreedPrice.Round(6),
		PlatformFee:    platformFee,
		ProviderPayout: providerPayout,
	}
}
```

### Settlement Transaction

```go
func (s *Service) SettleExecution(ctx context.Context, event TaskCompletedEvent) (Execution, error) {
	agreedPrice := decimal.RequireFromString(fmt.Sprint(event.Data.Billing.Cost))
	cost := calculateCPCCost(agreedPrice)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Execution{}, err
	}
	defer tx.Rollback(ctx)

	// Lookup requestor/provider (external IDs -> UUIDs) as needed
	requestorID, err := s.lookupTenantID(ctx, tx, event.TenantID)
	if err != nil {
		return Execution{}, err
	}
	providerID, err := s.lookupProviderTenantID(ctx, tx, event.Data.AgentID)
	if err != nil {
		return Execution{}, err
	}

	// Lock balances
	reqBal, err := s.getBalanceForUpdate(ctx, tx, requestorID)
	if err != nil {
		return Execution{}, err
	}
	provBal, err := s.getBalanceForUpdate(ctx, tx, providerID)
	if err != nil {
		return Execution{}, err
	}

	newReqBal := reqBal.Sub(cost.AgreedPrice)
	if newReqBal.IsNegative() {
		return Execution{}, ErrInsufficientFunds
	}
	newProvBal := provBal.Add(cost.ProviderPayout)

	execID := uuid.New()
	now := time.Now().UTC()

	// 1. Create execution record
	if _, err := tx.Exec(ctx, `
INSERT INTO executions (id, task_id, agent_id, requestor_tenant_id, provider_tenant_id, domain, started_at, completed_at, duration_ms, status, success, agreed_price, platform_fee, provider_payout, metadata)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'COMPLETED',true,$10,$11,$12,$13)
`, execID, event.TaskID, event.Data.AgentID, requestorID, providerID, event.Data.Domain, event.Data.StartedAt, event.Timestamp, event.Data.DurationMs, cost.AgreedPrice.StringFixed(6), cost.PlatformFee.StringFixed(6), cost.ProviderPayout.StringFixed(6), event.Data.Metadata); err != nil {
		return Execution{}, err
	}

	// 2. Update balances + ledger entries
	if err := s.updateBalanceAndLedger(ctx, tx, requestorID, newReqBal, "DEBIT", cost.AgreedPrice, execID, "execution", "Task execution: "+event.TaskID); err != nil {
		return Execution{}, err
	}
	if err := s.updateBalanceAndLedger(ctx, tx, providerID, newProvBal, "CREDIT", cost.ProviderPayout, execID, "execution", "Agent payout: "+event.Data.AgentID); err != nil {
		return Execution{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Execution{}, err
	}

	execution := Execution{
		ID: execID,
		TaskID: event.TaskID,
		AgentID: event.Data.AgentID,
		RequestorTenantID: requestorID,
		ProviderTenantID: providerID,
		Domain: event.Data.Domain,
		StartedAt: event.Data.StartedAt,
		CompletedAt: event.Timestamp,
		DurationMs: event.Data.DurationMs,
		Status: "COMPLETED",
		Success: true,
		AgreedPrice: cost.AgreedPrice.StringFixed(6),
		PlatformFee: cost.PlatformFee.StringFixed(6),
		ProviderPayout: cost.ProviderPayout.StringFixed(6),
		Metadata: event.Data.Metadata,
		CreatedAt: now,
	}

	// Export to BigQuery (best-effort async)
	go func() { _ = s.exportToBigQuery(context.Background(), execution) }()

	return execution, nil
}
```

### Balance Locking

```go
func (s *Service) getBalanceForUpdate(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID) (decimal.Decimal, error) {
	var balanceStr string
	if err := tx.QueryRow(ctx, `
SELECT balance FROM tenant_balances WHERE tenant_id = $1 FOR UPDATE
`, tenantID).Scan(&balanceStr); err != nil {
		return decimal.Zero, err
	}
	return decimal.RequireFromString(balanceStr), nil
}
```

## Event Handling

### Consumed Events

#### contract.completed

Published by `aex-contract-engine` when a provider completes work.

```go
func (h *Handlers) HandleTaskCompleted(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	envelope, err := decodePubSubPush(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var event TaskCompletedEvent
	if err := json.Unmarshal(envelope.Data, &event); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	exec, err := h.svc.SettleExecution(ctx, event)
	if err != nil {
		if errors.Is(err, ErrInsufficientFunds) {
			// Optionally flag tenant, alert, etc.
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	slog.Info("settlement_completed", "task_id", event.TaskID, "execution_id", exec.ID.String(), "cost", exec.AgreedPrice)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
```

#### contract.failed

Published by `aex-contract-engine` when execution fails.

```go
func (h *Handlers) HandleTaskFailed(w http.ResponseWriter, r *http.Request) {
	// Record failed executions (no billing, but track for analytics)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
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

```go
func (s *Service) exportToBigQuery(ctx context.Context, execution Execution) error {
	// Best-effort: do not block settlement commit on BQ.
	ins := s.bigquery.Dataset(s.cfg.BigQueryDataset).Table("executions").Inserter()
	row := map[string]any{
		"execution_id":         execution.ID.String(),
		"task_id":              execution.TaskID,
		"agent_id":             execution.AgentID,
		"requestor_tenant_id":  execution.RequestorTenantID.String(),
		"provider_tenant_id":   execution.ProviderTenantID.String(),
		"domain":               execution.Domain,
		"started_at":           execution.StartedAt,
		"completed_at":         execution.CompletedAt,
		"duration_ms":          execution.DurationMs,
		"status":               execution.Status,
		"success":              execution.Success,
		"agreed_price":         execution.AgreedPrice,
		"platform_fee":         execution.PlatformFee,
		"provider_payout":      execution.ProviderPayout,
		"created_at":           execution.CreatedAt,
	}
	if err := ins.Put(ctx, []*bigquery.ValuesSaver{{Schema: nil, Row: row}}); err != nil {
		slog.Warn("bigquery_export_failed", "error", err)
		return err
	}
	return nil
}
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
├── cmd/
│   └── settlement/
│       └── main.go
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── db/
│   │   └── postgres.go       # pgxpool init
│   ├── model/
│   │   ├── tenant.go
│   │   ├── execution.go
│   │   └── ledger.go
│   ├── service/
│   │   ├── settlement.go
│   │   └── export.go
│   ├── api/
│   │   ├── usage.go
│   │   └── balance.go
│   └── events/
│       └── handlers.go
├── hack/
│   └── tests/
├── Dockerfile
├── go.mod
└── go.sum
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
