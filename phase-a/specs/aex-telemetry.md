# aex-telemetry Service Specification

## Overview

**Purpose:** Centralized telemetry collection, aggregation, and export. Provides metrics, logging, and tracing infrastructure for all AEX services.

**Language:** Go 1.22+
**Framework:** OpenTelemetry Collector + custom exporters
**Runtime:** Cloud Run
**Ports:** 4317 (OTLP gRPC), 4318 (OTLP HTTP), 8080 (health)

## Architecture Position

```
┌──────────────────────────────────────────────────────────────────────────┐
│                         AEX Services                                      │
│  ┌────────────┐ ┌──────────────┐ ┌───────────────┐ ┌────────────┐        │
│  │aex-gateway │ │aex-bid-eval- │ │aex-contract-  │ │aex-settle- │        │
│  │            │ │uator         │ │engine         │ │ment        │        │
│  └─────┬──────┘ └─────┬────────┘ └──────┬────────┘ └─────┬──────┘        │
│        │              │                 │                │               │
│        └──────────────┴─────────────────┴────────────────┘               │
│                              │                                            │
│                              │ OTLP (metrics, traces, logs)               │
│                              ▼                                            │
│                    ┌─────────────────┐                                    │
│                    │  aex-telemetry  │◄── THIS SERVICE                    │
│                    │                 │                                    │
│                    │  • Collect      │                                    │
│                    │  • Process      │                                    │
│                    │  • Export       │                                    │
│                    └────────┬────────┘                                    │
│                             │                                             │
│           ┌─────────────────┼─────────────────┐                           │
│           ▼                 ▼                 ▼                           │
│    ┌────────────┐    ┌────────────┐    ┌────────────┐                     │
│    │Cloud       │    │ BigQuery   │    │Cloud       │                     │
│    │Monitoring  │    │ (metrics)  │    │Logging     │                     │
│    └────────────┘    └────────────┘    └────────────┘                     │
└──────────────────────────────────────────────────────────────────────────┘
```

## Core Responsibilities

1. **Metrics Collection** - Receive and aggregate metrics from all services
2. **Trace Collection** - Collect distributed traces across service boundaries
3. **Log Aggregation** - Centralize structured logs
4. **Data Processing** - Enrich, sample, and filter telemetry data
5. **Export** - Send to GCP Cloud Monitoring, BigQuery, and Cloud Logging

## OpenTelemetry Collector Configuration

### Main Configuration

```yaml
# otel-collector-config.yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

  # Prometheus scraping for services that expose /metrics
  prometheus:
    config:
      scrape_configs:
        - job_name: 'aex-services'
          scrape_interval: 15s
          kubernetes_sd_configs:
            - role: pod
          relabel_configs:
            - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
              action: keep
              regex: true
            - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_port]
              action: replace
              target_label: __address__
              regex: (.+)
              replacement: $1

processors:
  # Add resource attributes
  resource:
    attributes:
      - key: service.namespace
        value: aex
        action: upsert
      - key: deployment.environment
        from_attribute: ENV
        action: upsert

  # Batch for efficiency
  batch:
    timeout: 10s
    send_batch_size: 1024
    send_batch_max_size: 2048

  # Memory limiter to prevent OOM
  memory_limiter:
    check_interval: 1s
    limit_mib: 1500
    spike_limit_mib: 500

  # Filter out high-cardinality/noisy data
  filter:
    metrics:
      exclude:
        match_type: regexp
        metric_names:
          - "go_.*"
          - "process_.*"

  # Sampling for traces (keep 10% of traces, all errors)
  probabilistic_sampler:
    sampling_percentage: 10

  # Tail-based sampling for important traces
  tail_sampling:
    decision_wait: 10s
    num_traces: 100
    policies:
      - name: errors-policy
        type: status_code
        status_code: {status_codes: [ERROR]}
      - name: slow-traces-policy
        type: latency
        latency: {threshold_ms: 5000}
      - name: probabilistic-policy
        type: probabilistic
        probabilistic: {sampling_percentage: 10}

exporters:
  # Google Cloud Monitoring (metrics)
  googlecloud:
    project: ${GOOGLE_CLOUD_PROJECT}
    metric:
      prefix: custom.googleapis.com/aex

  # BigQuery (metrics for analytics)
  bigquery:
    project: ${GOOGLE_CLOUD_PROJECT}
    dataset: aex_telemetry
    table: metrics
    batch_size: 1000

  # Cloud Logging (logs)
  googlecloud/logging:
    project: ${GOOGLE_CLOUD_PROJECT}
    log:
      default_log_name: aex-services

  # Cloud Trace (traces)
  googlecloud/trace:
    project: ${GOOGLE_CLOUD_PROJECT}

  # Debug logging (development only)
  logging:
    loglevel: debug

service:
  pipelines:
    metrics:
      receivers: [otlp, prometheus]
      processors: [memory_limiter, resource, filter, batch]
      exporters: [googlecloud, bigquery]

    traces:
      receivers: [otlp]
      processors: [memory_limiter, resource, tail_sampling, batch]
      exporters: [googlecloud/trace]

    logs:
      receivers: [otlp]
      processors: [memory_limiter, resource, batch]
      exporters: [googlecloud/logging]

  extensions: [health_check, zpages]

extensions:
  health_check:
    endpoint: 0.0.0.0:13133
  zpages:
    endpoint: 0.0.0.0:55679
```

## Custom Metrics

### Business Metrics

```go
// metrics/business.go
package metrics

import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/metric"
)

var (
    meter = otel.Meter("aex-telemetry")

    // Task metrics
    TasksSubmitted metric.Int64Counter
    TasksCompleted metric.Int64Counter
    TasksFailed    metric.Int64Counter
    TaskDuration   metric.Float64Histogram

    // Agent metrics
    AgentsRegistered   metric.Int64UpDownCounter
    AgentInvocations   metric.Int64Counter
    AgentTrustScore    metric.Float64Gauge

    // Financial metrics
    TransactionValue   metric.Float64Counter
    PlatformRevenue    metric.Float64Counter
    ProviderPayouts    metric.Float64Counter
)

func InitMetrics() error {
    var err error

    TasksSubmitted, err = meter.Int64Counter(
        "aex.tasks.submitted",
        metric.WithDescription("Total tasks submitted"),
        metric.WithUnit("1"),
    )
    if err != nil {
        return err
    }

    TasksCompleted, err = meter.Int64Counter(
        "aex.tasks.completed",
        metric.WithDescription("Total tasks completed successfully"),
        metric.WithUnit("1"),
    )
    if err != nil {
        return err
    }

    TasksFailed, err = meter.Int64Counter(
        "aex.tasks.failed",
        metric.WithDescription("Total tasks failed"),
        metric.WithUnit("1"),
    )
    if err != nil {
        return err
    }

    TaskDuration, err = meter.Float64Histogram(
        "aex.tasks.duration",
        metric.WithDescription("Task execution duration"),
        metric.WithUnit("ms"),
        metric.WithExplicitBucketBoundaries(10, 50, 100, 250, 500, 1000, 2500, 5000, 10000),
    )
    if err != nil {
        return err
    }

    TransactionValue, err = meter.Float64Counter(
        "aex.transactions.value",
        metric.WithDescription("Total transaction value"),
        metric.WithUnit("USD"),
    )
    if err != nil {
        return err
    }

    return nil
}
```

### Service Health Metrics

```go
// Emitted by each service
var (
    RequestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aex_requests_total",
            Help: "Total requests processed",
        },
        []string{"service", "method", "status"},
    )

    RequestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "aex_request_duration_seconds",
            Help:    "Request duration in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"service", "method"},
    )

    ActiveConnections = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "aex_active_connections",
            Help: "Number of active connections",
        },
        []string{"service"},
    )
)
```

## BigQuery Metrics Schema

```sql
-- Table: aex_telemetry.metrics
CREATE TABLE aex_telemetry.metrics (
    timestamp TIMESTAMP NOT NULL,
    metric_name STRING NOT NULL,
    metric_type STRING NOT NULL,  -- counter, gauge, histogram
    value FLOAT64,
    labels JSON,
    service STRING,
    environment STRING
)
PARTITION BY DATE(timestamp)
CLUSTER BY metric_name, service;

-- Table: aex_telemetry.traces
CREATE TABLE aex_telemetry.traces (
    trace_id STRING NOT NULL,
    span_id STRING NOT NULL,
    parent_span_id STRING,
    operation_name STRING NOT NULL,
    service_name STRING NOT NULL,
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP NOT NULL,
    duration_ms INT64,
    status_code STRING,
    attributes JSON,
    events JSON
)
PARTITION BY DATE(start_time)
CLUSTER BY service_name, operation_name;
```

## Dashboard Queries

### Task Volume & Success Rate

```sql
-- Last 24 hours task metrics
SELECT
    TIMESTAMP_TRUNC(timestamp, HOUR) as hour,
    SUM(CASE WHEN metric_name = 'aex.tasks.submitted' THEN value ELSE 0 END) as submitted,
    SUM(CASE WHEN metric_name = 'aex.tasks.completed' THEN value ELSE 0 END) as completed,
    SUM(CASE WHEN metric_name = 'aex.tasks.failed' THEN value ELSE 0 END) as failed,
    SAFE_DIVIDE(
        SUM(CASE WHEN metric_name = 'aex.tasks.completed' THEN value ELSE 0 END),
        SUM(CASE WHEN metric_name = 'aex.tasks.submitted' THEN value ELSE 0 END)
    ) as success_rate
FROM aex_telemetry.metrics
WHERE timestamp > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 24 HOUR)
GROUP BY hour
ORDER BY hour;
```

### Revenue Dashboard

```sql
-- Daily revenue breakdown
SELECT
    DATE(timestamp) as date,
    JSON_VALUE(labels, '$.domain') as domain,
    SUM(CASE WHEN metric_name = 'aex.transactions.value' THEN value ELSE 0 END) as gmv,
    SUM(CASE WHEN metric_name = 'aex.platform.revenue' THEN value ELSE 0 END) as revenue
FROM aex_telemetry.metrics
WHERE timestamp > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
    AND metric_name IN ('aex.transactions.value', 'aex.platform.revenue')
GROUP BY date, domain
ORDER BY date DESC, gmv DESC;
```

### Latency Analysis

```sql
-- P50, P95, P99 latency by service
SELECT
    service,
    APPROX_QUANTILES(duration_ms, 100)[OFFSET(50)] as p50_ms,
    APPROX_QUANTILES(duration_ms, 100)[OFFSET(95)] as p95_ms,
    APPROX_QUANTILES(duration_ms, 100)[OFFSET(99)] as p99_ms
FROM aex_telemetry.traces
WHERE start_time > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 1 HOUR)
GROUP BY service;
```

## Alerting Rules

### Cloud Monitoring Alert Policies

```yaml
# High error rate alert
- displayName: "AEX High Error Rate"
  conditions:
    - displayName: "Error rate > 5%"
      conditionThreshold:
        filter: 'resource.type="cloud_run_revision" AND metric.type="custom.googleapis.com/aex/aex.tasks.failed"'
        comparison: COMPARISON_GT
        thresholdValue: 0.05
        duration: "300s"
        aggregations:
          - alignmentPeriod: "60s"
            perSeriesAligner: ALIGN_RATE

# High latency alert
- displayName: "AEX High Latency"
  conditions:
    - displayName: "P95 latency > 5s"
      conditionThreshold:
        filter: 'metric.type="custom.googleapis.com/aex/aex.tasks.duration"'
        comparison: COMPARISON_GT
        thresholdValue: 5000
        duration: "300s"
        aggregations:
          - alignmentPeriod: "60s"
            perSeriesAligner: ALIGN_PERCENTILE_95

# Agent unhealthy alert
- displayName: "AEX Agent Unhealthy"
  conditions:
    - displayName: "Agent unhealthy for 5 min"
      conditionThreshold:
        filter: 'metric.type="custom.googleapis.com/aex/aex.agent.health" AND metric.labels.status="unhealthy"'
        comparison: COMPARISON_GT
        thresholdValue: 0
        duration: "300s"
```

## SDK Integration

### Go SDK

```go
// pkg/telemetry/telemetry.go
package telemetry

import (
    "context"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    "go.opentelemetry.io/otel/sdk/metric"
    "go.opentelemetry.io/otel/sdk/trace"
)

func Init(ctx context.Context, serviceName string, otlpEndpoint string) (func(), error) {
    // Trace exporter
    traceExporter, err := otlptracegrpc.New(ctx,
        otlptracegrpc.WithEndpoint(otlpEndpoint),
        otlptracegrpc.WithInsecure(),
    )
    if err != nil {
        return nil, err
    }

    tp := trace.NewTracerProvider(
        trace.WithBatcher(traceExporter),
        trace.WithResource(newResource(serviceName)),
    )
    otel.SetTracerProvider(tp)

    // Metric exporter
    metricExporter, err := otlpmetricgrpc.New(ctx,
        otlpmetricgrpc.WithEndpoint(otlpEndpoint),
        otlpmetricgrpc.WithInsecure(),
    )
    if err != nil {
        return nil, err
    }

    mp := metric.NewMeterProvider(
        metric.WithReader(metric.NewPeriodicReader(metricExporter)),
        metric.WithResource(newResource(serviceName)),
    )
    otel.SetMeterProvider(mp)

    // Cleanup function
    return func() {
        tp.Shutdown(ctx)
        mp.Shutdown(ctx)
    }, nil
}
```

### Python SDK

```python
# aex_telemetry/sdk.py
from opentelemetry import trace, metrics
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.exporter.otlp.proto.grpc.metric_exporter import OTLPMetricExporter
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.metrics import MeterProvider
from opentelemetry.sdk.resources import Resource

def init_telemetry(service_name: str, otlp_endpoint: str):
    resource = Resource.create({"service.name": service_name})

    # Traces
    trace_provider = TracerProvider(resource=resource)
    trace_exporter = OTLPSpanExporter(endpoint=otlp_endpoint, insecure=True)
    trace_provider.add_span_processor(BatchSpanProcessor(trace_exporter))
    trace.set_tracer_provider(trace_provider)

    # Metrics
    metric_exporter = OTLPMetricExporter(endpoint=otlp_endpoint, insecure=True)
    meter_provider = MeterProvider(
        resource=resource,
        metric_readers=[PeriodicExportingMetricReader(metric_exporter)]
    )
    metrics.set_meter_provider(meter_provider)

    return trace.get_tracer(service_name), metrics.get_meter(service_name)
```

## Configuration

### Environment Variables

```bash
# Server
PORT=4317
HTTP_PORT=4318
HEALTH_PORT=8080
ENV=production

# GCP
GOOGLE_CLOUD_PROJECT=aex-prod

# BigQuery
BIGQUERY_DATASET=aex_telemetry

# Sampling
TRACE_SAMPLING_RATE=0.1  # 10%

# Processing
BATCH_TIMEOUT_SECONDS=10
BATCH_SIZE=1024
MEMORY_LIMIT_MIB=1500

# Logging
LOG_LEVEL=info
```

## Deployment

```bash
gcloud run deploy aex-telemetry \
  --image gcr.io/PROJECT/aex-telemetry:latest \
  --region us-central1 \
  --platform managed \
  --no-allow-unauthenticated \
  --min-instances 1 \
  --max-instances 10 \
  --memory 2Gi \
  --cpu 2 \
  --set-env-vars "ENV=production"
```

## Directory Structure

```
aex-telemetry/
├── cmd/
│   └── collector/
│       └── main.go
├── config/
│   ├── otel-collector-config.yaml
│   └── alerting-rules.yaml
├── internal/
│   ├── exporters/
│   │   └── bigquery.go
│   └── processors/
│       └── aex_processor.go
├── pkg/
│   └── sdk/
│       ├── go/
│       │   └── telemetry.go
│       └── python/
│           └── aex_telemetry/
├── dashboards/
│   ├── operations.json
│   └── business.json
├── Dockerfile
└── README.md
```
