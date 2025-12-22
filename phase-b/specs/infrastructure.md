# Phase B Infrastructure Additions Specification

## Overview

Phase B introduces new infrastructure components to support CPA pricing, ML-based trust scoring, and enhanced monitoring. This document specifies the additional GCP resources and configurations required.

## Infrastructure Changes Summary

| Component | Phase A | Phase B Addition |
|-----------|---------|------------------|
| BigQuery | Analytics only | + BigQuery ML models |
| Vertex AI | - | Online prediction endpoints |
| Cloud Monitoring | Basic metrics | + Custom dashboards |
| Pub/Sub | Event bus | + New topics/subscriptions |
| OPA | - | Policy engine sidecar |

## BigQuery ML Setup

### Dataset Structure

```sql
-- Create ML dataset
CREATE SCHEMA IF NOT EXISTS `aex-prod.ml`
OPTIONS(
  location = 'US',
  description = 'Machine learning models and training data'
);

-- Create feature store dataset
CREATE SCHEMA IF NOT EXISTS `aex-prod.features`
OPTIONS(
  location = 'US',
  description = 'Feature engineering tables'
);
```

### Training Data Tables

```sql
-- Agent features snapshot (for point-in-time training)
CREATE TABLE `aex-prod.features.agent_features_snapshot` (
  snapshot_id STRING NOT NULL,
  agent_id STRING NOT NULL,
  snapshot_start TIMESTAMP NOT NULL,
  snapshot_end TIMESTAMP NOT NULL,

  -- Historical features
  total_executions INT64,
  success_rate_7d FLOAT64,
  success_rate_30d FLOAT64,
  avg_latency_7d FLOAT64,
  avg_latency_30d FLOAT64,
  std_latency_30d FLOAT64,

  -- Domain-specific
  domains ARRAY<STRING>,
  domain_success_rates ARRAY<STRUCT<domain STRING, rate FLOAT64>>,

  -- Trust components
  trust_score FLOAT64,
  consistency_score FLOAT64,

  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP()
)
PARTITION BY DATE(snapshot_start)
CLUSTER BY agent_id;

-- Training dataset for success prediction
CREATE TABLE `aex-prod.ml.training_success_prediction` (
  -- Features
  agent_id STRING,
  domain STRING,
  payload_size_bytes INT64,
  num_success_criteria INT64,
  max_latency_requirement_ms INT64,
  budget_max_cpc FLOAT64,

  -- Agent features at task time
  agent_total_executions INT64,
  agent_success_rate_30d FLOAT64,
  agent_avg_latency_30d FLOAT64,
  agent_domain_success_rate FLOAT64,
  agent_trust_score FLOAT64,

  -- Cross features
  sla_headroom_ratio FLOAT64,
  budget_headroom_ratio FLOAT64,

  -- Target
  success BOOL,
  outcome_accuracy FLOAT64,
  outcome_latency_ms INT64,

  -- Metadata
  work_id STRING,
  created_at TIMESTAMP
)
PARTITION BY DATE(created_at)
CLUSTER BY domain, agent_id;
```

### ML Models

```sql
-- Success prediction model (logistic regression)
CREATE OR REPLACE MODEL `aex-prod.ml.success_predictor_v1`
OPTIONS(
  model_type = 'LOGISTIC_REG',
  input_label_cols = ['task_success'],
  auto_class_weights = TRUE,
  enable_global_explain = TRUE,
  l2_reg = 0.01,
  max_iterations = 50,
  learn_rate_strategy = 'LINE_SEARCH',
  model_registry = 'vertex_ai',
  vertex_ai_model_id = 'success_predictor'
) AS
SELECT
  domain,
  payload_size_bytes,
  num_success_criteria,
  max_latency_requirement_ms,
  budget_max_cpc,
  agent_total_executions,
  agent_success_rate_30d,
  agent_avg_latency_30d,
  agent_domain_success_rate,
  agent_trust_score,
  sla_headroom_ratio,
  budget_headroom_ratio,
  task_success
FROM `aex-prod.ml.training_success_prediction`
WHERE created_at > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 90 DAY);

-- Accuracy prediction model (linear regression)
CREATE OR REPLACE MODEL `aex-prod.ml.accuracy_predictor_v1`
OPTIONS(
  model_type = 'LINEAR_REG',
  input_label_cols = ['outcome_accuracy'],
  enable_global_explain = TRUE,
  l2_reg = 0.01,
  max_iterations = 50,
  model_registry = 'vertex_ai',
  vertex_ai_model_id = 'accuracy_predictor'
) AS
SELECT
  domain,
  payload_size_bytes,
  agent_success_rate_30d,
  agent_domain_success_rate,
  agent_trust_score,
  outcome_accuracy
FROM `aex-prod.ml.training_success_prediction`
WHERE outcome_accuracy IS NOT NULL
AND created_at > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 90 DAY);
```

### Scheduled Training

```sql
-- Daily feature snapshot job
CREATE OR REPLACE PROCEDURE `aex-prod.features.generate_daily_snapshot`()
BEGIN
  INSERT INTO `aex-prod.features.agent_features_snapshot`
  SELECT
    GENERATE_UUID() as snapshot_id,
    agent_id,
    TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 1 DAY) as snapshot_start,
    CURRENT_TIMESTAMP() as snapshot_end,
    COUNT(*) as total_executions,
    AVG(CASE WHEN created_at > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
             AND success THEN 1.0 ELSE 0.0 END) as success_rate_7d,
    AVG(CASE WHEN success THEN 1.0 ELSE 0.0 END) as success_rate_30d,
    AVG(CASE WHEN created_at > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
             THEN latency_ms END) as avg_latency_7d,
    AVG(latency_ms) as avg_latency_30d,
    STDDEV(latency_ms) as std_latency_30d,
    ARRAY_AGG(DISTINCT domain) as domains,
    ARRAY(
      SELECT AS STRUCT domain, AVG(CASE WHEN success THEN 1.0 ELSE 0.0 END) as rate
      FROM UNNEST(domains) as domain
      GROUP BY domain
    ) as domain_success_rates,
    -- Trust score calculated separately
    NULL as trust_score,
    NULL as consistency_score,
    CURRENT_TIMESTAMP() as created_at
  FROM `aex-prod.executions`
  WHERE created_at > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
  GROUP BY agent_id;
END;

-- Schedule: Run daily at 2am UTC
-- Configure via Cloud Scheduler or BigQuery scheduled queries
```

## Vertex AI Setup (Optional)

### Terraform Configuration

```hcl
# vertex_ai.tf

# Enable Vertex AI API
resource "google_project_service" "vertex_ai" {
  service            = "aiplatform.googleapis.com"
  disable_on_destroy = false
}

# Model registry
resource "google_vertex_ai_model" "success_predictor" {
  display_name = "aex-success-predictor"
  description  = "Predicts task success probability"

  container_spec {
    image_uri = "us-docker.pkg.dev/vertex-ai/prediction/sklearn-cpu.1-0:latest"
  }

  labels = {
    environment = var.environment
    phase       = "b"
  }
}

# Online prediction endpoint
resource "google_vertex_ai_endpoint" "success_predictor" {
  display_name = "aex-success-predictor-endpoint"
  location     = var.region
  description  = "Online prediction endpoint for success probability"

  labels = {
    environment = var.environment
  }
}

# Deploy model to endpoint
resource "google_vertex_ai_endpoint_deployment" "success_predictor" {
  endpoint = google_vertex_ai_endpoint.success_predictor.id
  model    = google_vertex_ai_model.success_predictor.id

  dedicated_resources {
    machine_spec {
      machine_type = "n1-standard-2"
    }
    min_replica_count = 1
    max_replica_count = 5

    autoscaling_metric_specs {
      metric_name = "aiplatform.googleapis.com/prediction/online/cpu/utilization"
      target      = 0.7
    }
  }

  traffic_split = {
    "0" = 100
  }
}
```

### Alternative: BigQuery ML Remote Model

```sql
-- Use BigQuery ML for simpler deployment (no Vertex AI required)
-- Predictions are made via SQL queries

-- Example prediction query
SELECT
  agent_id,
  predicted_task_success,
  predicted_task_success_probs[OFFSET(1)].prob as success_probability
FROM ML.PREDICT(
  MODEL `aex-prod.ml.success_predictor_v1`,
  (
    SELECT
      'nlp.summarization' as domain,
      50000 as payload_size_bytes,
      2 as num_success_criteria,
      2000 as max_latency_requirement_ms,
      0.10 as budget_max_cpc,
      a.total_executions as agent_total_executions,
      a.success_rate_30d as agent_success_rate_30d,
      a.avg_latency_30d as agent_avg_latency_30d,
      a.domain_success_rate as agent_domain_success_rate,
      a.trust_score as agent_trust_score,
      (2000 - a.avg_latency_30d) / 2000 as sla_headroom_ratio,
      (0.10 - 0.05) / 0.10 as budget_headroom_ratio
    FROM `aex-prod.features.agent_features_snapshot` a
    WHERE agent_id IN ('agent-1', 'agent-2', 'agent-3')
    AND snapshot_end > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 1 DAY)
  )
);
```

## Open Policy Agent (OPA) Setup

### Sidecar Deployment

```yaml
# opa-sidecar.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: aex-governance
spec:
  template:
    spec:
      containers:
        # Main governance container
        - name: governance
          image: gcr.io/PROJECT/aex-governance:latest
          ports:
            - containerPort: 8080
          env:
            - name: OPA_URL
              value: "http://localhost:8181"

        # OPA sidecar
        - name: opa
          image: openpolicyagent/opa:latest
          ports:
            - containerPort: 8181
          args:
            - "run"
            - "--server"
            - "--addr=0.0.0.0:8181"
            - "--bundle=/policies"
            - "--log-level=info"
          volumeMounts:
            - name: policies
              mountPath: /policies
              readOnly: true

      volumes:
        - name: policies
          configMap:
            name: aex-opa-policies
```

### Policy ConfigMap

```yaml
# opa-policies-configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: aex-opa-policies
data:
  submission.rego: |
    package aex.submission

    default allow = false

    allow {
        valid_domain
        valid_budget
        content_size_ok
    }

    valid_domain {
        input.domain in data.allowed_domains
    }

    valid_budget {
        input.budget.max_cpc <= 1.0
        input.budget.max_cpa_total <= 5.0
    }

    content_size_ok {
        input.payload_size_bytes <= 10485760
    }

  outcome.rego: |
    package aex.outcome

    default valid = true

    valid = false {
        gaming_detected
    }

    gaming_detected {
        input.claimed_metrics.accuracy == 1.0
        count(input.history.perfect_scores) > 5
    }

  data.json: |
    {
      "allowed_domains": [
        "nlp.summarization",
        "nlp.classification",
        "nlp.extraction",
        "vision.classification",
        "vision.detection",
        "code.generation",
        "code.review"
      ]
    }
```

## Pub/Sub Topics and Subscriptions

### New Topics

```hcl
# pubsub_phase_b.tf

# Governance events topic
resource "google_pubsub_topic" "governance_events" {
  name = "aex-governance-events"

  message_retention_duration = "604800s"  # 7 days

  labels = {
    phase = "b"
  }
}

# Trust scoring events topic
resource "google_pubsub_topic" "trust_scoring_events" {
  name = "aex-trust-scoring-events"

  message_retention_duration = "604800s"

  labels = {
    phase = "b"
  }
}

# Subscriptions for trust scoring
resource "google_pubsub_subscription" "trust_scoring_executions" {
  name  = "aex-trust-scoring-executions-sub"
  topic = google_pubsub_topic.events.name  # Main events topic

  filter = "attributes.event_type = \"task.execution_completed\""

  ack_deadline_seconds = 60

  push_config {
    push_endpoint = "${google_cloud_run_service.trust_scoring.status[0].url}/events"
    oidc_token {
      service_account_email = google_service_account.trust_scoring.email
    }
  }
}

resource "google_pubsub_subscription" "governance_validation" {
  name  = "aex-governance-validation-sub"
  topic = google_pubsub_topic.events.name

  filter = "attributes.event_type = \"outcome.validation_requested\""

  ack_deadline_seconds = 30

  push_config {
    push_endpoint = "${google_cloud_run_service.governance.status[0].url}/events"
    oidc_token {
      service_account_email = google_service_account.governance.email
    }
  }
}
```

## Cloud Monitoring Dashboards

### CPA Metrics Dashboard

```json
{
  "displayName": "AEX Phase B - CPA Metrics",
  "gridLayout": {
    "columns": 2,
    "widgets": [
      {
        "title": "CPA Tasks Success Rate",
        "xyChart": {
          "dataSets": [{
            "timeSeriesQuery": {
              "timeSeriesFilter": {
                "filter": "metric.type=\"custom.googleapis.com/aex/settlement/cpa_success_rate\""
              }
            }
          }]
        }
      },
      {
        "title": "Average CPA Bonus Amount",
        "xyChart": {
          "dataSets": [{
            "timeSeriesQuery": {
              "timeSeriesFilter": {
                "filter": "metric.type=\"custom.googleapis.com/aex/settlement/cpa_bonus_amount\""
              },
              "aggregation": {
                "alignmentPeriod": "60s",
                "perSeriesAligner": "ALIGN_MEAN"
              }
            }
          }]
        }
      },
      {
        "title": "Trust Score Distribution",
        "xyChart": {
          "dataSets": [{
            "timeSeriesQuery": {
              "timeSeriesFilter": {
                "filter": "metric.type=\"custom.googleapis.com/aex/trust_scoring/score_distribution\""
              }
            }
          }]
        }
      },
      {
        "title": "Prediction Accuracy",
        "scorecard": {
          "timeSeriesQuery": {
            "timeSeriesFilter": {
              "filter": "metric.type=\"custom.googleapis.com/aex/trust_scoring/prediction_accuracy\""
            }
          }
        }
      },
      {
        "title": "Governance Policy Evaluations",
        "xyChart": {
          "dataSets": [{
            "timeSeriesQuery": {
              "timeSeriesFilter": {
                "filter": "metric.type=\"custom.googleapis.com/aex/governance/evaluations_total\""
              }
            }
          }]
        }
      },
      {
        "title": "Outcome Verification Latency",
        "xyChart": {
          "dataSets": [{
            "timeSeriesQuery": {
              "timeSeriesFilter": {
                "filter": "metric.type=\"custom.googleapis.com/aex/settlement/verification_latency\""
              },
              "aggregation": {
                "alignmentPeriod": "60s",
                "perSeriesAligner": "ALIGN_PERCENTILE_95"
              }
            }
          }]
        }
      }
    ]
  }
}
```

### Provider Earnings Dashboard

```json
{
  "displayName": "AEX Phase B - Provider Earnings",
  "gridLayout": {
    "columns": 2,
    "widgets": [
      {
        "title": "Total Provider Payouts (Daily)",
        "xyChart": {
          "dataSets": [{
            "timeSeriesQuery": {
              "timeSeriesFilter": {
                "filter": "metric.type=\"custom.googleapis.com/aex/settlement/provider_payout_total\""
              },
              "aggregation": {
                "alignmentPeriod": "86400s",
                "perSeriesAligner": "ALIGN_SUM"
              }
            }
          }]
        }
      },
      {
        "title": "CPC vs CPA Earnings Split",
        "pieChart": {
          "dataSets": [{
            "timeSeriesQuery": {
              "timeSeriesFilter": {
                "filter": "metric.type=\"custom.googleapis.com/aex/settlement/earnings_by_type\""
              }
            }
          }]
        }
      },
      {
        "title": "Top Earning Agents",
        "table": {
          "dataSets": [{
            "timeSeriesQuery": {
              "timeSeriesFilter": {
                "filter": "metric.type=\"custom.googleapis.com/aex/settlement/agent_earnings\""
              }
            }
          }]
        }
      }
    ]
  }
}
```

## Terraform Module

### Main Configuration

```hcl
# phase_b_infrastructure/main.tf

terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

variable "project_id" {
  description = "GCP Project ID"
  type        = string
}

variable "region" {
  description = "GCP Region"
  type        = string
  default     = "us-central1"
}

variable "environment" {
  description = "Environment (dev, staging, prod)"
  type        = string
}

# BigQuery ML datasets
module "bigquery_ml" {
  source = "./modules/bigquery_ml"

  project_id  = var.project_id
  location    = "US"
  environment = var.environment
}

# Vertex AI (optional)
module "vertex_ai" {
  source = "./modules/vertex_ai"
  count  = var.enable_vertex_ai ? 1 : 0

  project_id  = var.project_id
  region      = var.region
  environment = var.environment
}

# Pub/Sub additions
module "pubsub_phase_b" {
  source = "./modules/pubsub"

  project_id  = var.project_id
  environment = var.environment
}

# Monitoring dashboards
module "monitoring" {
  source = "./modules/monitoring"

  project_id  = var.project_id
  environment = var.environment
}

# Service accounts for new services
resource "google_service_account" "governance" {
  account_id   = "aex-governance"
  display_name = "AEX Governance Service"
}

resource "google_service_account" "trust_scoring" {
  account_id   = "aex-trust-scoring"
  display_name = "AEX Trust Scoring Service"
}

resource "google_service_account" "adapter_mcp" {
  account_id   = "aex-adapter-mcp"
  display_name = "AEX MCP Adapter Service"
}

# IAM bindings
resource "google_project_iam_member" "trust_scoring_bigquery" {
  project = var.project_id
  role    = "roles/bigquery.dataViewer"
  member  = "serviceAccount:${google_service_account.trust_scoring.email}"
}

resource "google_project_iam_member" "trust_scoring_bqml" {
  project = var.project_id
  role    = "roles/bigquery.jobUser"
  member  = "serviceAccount:${google_service_account.trust_scoring.email}"
}
```

## Deployment Order

1. **BigQuery ML Setup**
   - Create datasets
   - Create training data tables
   - Create initial models (empty, will be trained after data collection)

2. **Pub/Sub Topics**
   - Create new topics
   - Create subscriptions

3. **Service Accounts & IAM**
   - Create service accounts
   - Assign roles

4. **OPA Policies**
   - Deploy policy ConfigMaps
   - Configure sidecars

5. **Monitoring**
   - Create custom metrics
   - Deploy dashboards

6. **Vertex AI (if enabled)**
   - Create endpoints
   - Deploy models

## Resource Estimates

| Resource | Type | Estimated Monthly Cost |
|----------|------|----------------------|
| BigQuery ML | Training (daily) | $50-100 |
| BigQuery ML | Predictions | $20-50 |
| Vertex AI Endpoint | n1-standard-2 | $200-400 |
| Pub/Sub | Additional topics | $10-20 |
| Cloud Monitoring | Custom metrics | $20-50 |
| OPA Sidecars | CPU/Memory | Included in Cloud Run |

**Total Phase B Infrastructure Addition: ~$300-620/month** (varies with usage)
