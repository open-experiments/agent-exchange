# Infrastructure Specification

## Overview

This document specifies the GCP infrastructure required for Phase A of the Agent Exchange (AEX) platform.

## GCP Project Structure

```
aex-prod (Production)
├── aex-dev (Development)
└── aex-staging (Staging)

Each environment is a separate GCP project for isolation.
```

## Infrastructure Components

### 1. Networking (VPC)

```hcl
# terraform/networking.tf

resource "google_compute_network" "aex_vpc" {
  name                    = "aex-vpc"
  auto_create_subnetworks = false
  project                 = var.project_id
}

resource "google_compute_subnetwork" "aex_subnet" {
  name          = "aex-subnet"
  ip_cidr_range = "10.0.0.0/20"
  region        = var.region
  network       = google_compute_network.aex_vpc.id

  secondary_ip_range {
    range_name    = "gke-pods"
    ip_cidr_range = "10.1.0.0/16"
  }

  secondary_ip_range {
    range_name    = "gke-services"
    ip_cidr_range = "10.2.0.0/20"
  }

  private_ip_google_access = true
}

# Cloud NAT for outbound internet access from private resources
resource "google_compute_router" "aex_router" {
  name    = "aex-router"
  region  = var.region
  network = google_compute_network.aex_vpc.id
}

resource "google_compute_router_nat" "aex_nat" {
  name                               = "aex-nat"
  router                             = google_compute_router.aex_router.name
  region                             = var.region
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "ALL_SUBNETWORKS_ALL_IP_RANGES"
}

# VPC Connector for Cloud Run to access VPC resources
resource "google_vpc_access_connector" "aex_connector" {
  name          = "aex-connector"
  region        = var.region
  network       = google_compute_network.aex_vpc.name
  ip_cidr_range = "10.8.0.0/28"
  min_instances = 2
  max_instances = 10
}
```

### 2. GKE Cluster

```hcl
# terraform/gke.tf

resource "google_container_cluster" "aex_cluster" {
  name     = "aex-cluster"
  location = var.region
  project  = var.project_id

  # Use Autopilot for simplified management in Phase A
  enable_autopilot = true

  network    = google_compute_network.aex_vpc.name
  subnetwork = google_compute_subnetwork.aex_subnet.name

  ip_allocation_policy {
    cluster_secondary_range_name  = "gke-pods"
    services_secondary_range_name = "gke-services"
  }

  private_cluster_config {
    enable_private_nodes    = true
    enable_private_endpoint = false
    master_ipv4_cidr_block  = "172.16.0.0/28"
  }

  master_authorized_networks_config {
    cidr_blocks {
      cidr_block   = "0.0.0.0/0"
      display_name = "All"
    }
  }

  release_channel {
    channel = "REGULAR"
  }

  workload_identity_config {
    workload_pool = "${var.project_id}.svc.id.goog"
  }

  # Enable required addons
  addons_config {
    http_load_balancing {
      disabled = false
    }
    horizontal_pod_autoscaling {
      disabled = false
    }
  }
}

# Namespaces
resource "kubernetes_namespace" "aex_system" {
  metadata {
    name = "aex-system"
    labels = {
      "app.kubernetes.io/managed-by" = "terraform"
    }
  }
}

resource "kubernetes_namespace" "aex_agents" {
  metadata {
    name = "aex-agents"
    labels = {
      "app.kubernetes.io/managed-by" = "terraform"
    }
  }
}
```

### 3. Cloud SQL (PostgreSQL)

```hcl
# terraform/cloudsql.tf

resource "google_sql_database_instance" "aex_postgres" {
  name             = "aex-postgres"
  database_version = "POSTGRES_15"
  region           = var.region
  project          = var.project_id

  settings {
    tier = "db-custom-2-4096"  # 2 vCPU, 4GB RAM

    ip_configuration {
      ipv4_enabled    = false
      private_network = google_compute_network.aex_vpc.id
    }

    backup_configuration {
      enabled                        = true
      start_time                     = "03:00"
      point_in_time_recovery_enabled = true
      transaction_log_retention_days = 7
      backup_retention_settings {
        retained_backups = 7
      }
    }

    maintenance_window {
      day  = 7  # Sunday
      hour = 3
    }

    database_flags {
      name  = "max_connections"
      value = "200"
    }

    insights_config {
      query_insights_enabled  = true
      record_application_tags = true
    }
  }

  deletion_protection = true
}

resource "google_sql_database" "aex_settlement" {
  name     = "aex_settlement"
  instance = google_sql_database_instance.aex_postgres.name
}

resource "google_sql_user" "aex_user" {
  name     = "aex_app"
  instance = google_sql_database_instance.aex_postgres.name
  password = random_password.db_password.result
}
```

### 4. Firestore

```hcl
# terraform/firestore.tf

resource "google_firestore_database" "aex_firestore" {
  project     = var.project_id
  name        = "(default)"
  location_id = var.region
  type        = "FIRESTORE_NATIVE"

  concurrency_mode            = "OPTIMISTIC"
  app_engine_integration_mode = "DISABLED"
}

# Indexes are defined in firestore.indexes.json and deployed separately
```

**Firestore Indexes (firestore.indexes.json):**

```json
{
  "indexes": [
    {
      "collectionGroup": "work",
      "queryScope": "COLLECTION",
      "fields": [
        {"fieldPath": "tenant_id", "order": "ASCENDING"},
        {"fieldPath": "created_at", "order": "DESCENDING"}
      ]
    },
    {
      "collectionGroup": "work",
      "queryScope": "COLLECTION",
      "fields": [
        {"fieldPath": "tenant_id", "order": "ASCENDING"},
        {"fieldPath": "state", "order": "ASCENDING"},
        {"fieldPath": "created_at", "order": "DESCENDING"}
      ]
    },
    {
      "collectionGroup": "providers",
      "queryScope": "COLLECTION",
      "fields": [
        {"fieldPath": "status", "order": "ASCENDING"},
        {"fieldPath": "trust_score", "order": "DESCENDING"}
      ]
    },
    {
      "collectionGroup": "providers",
      "queryScope": "COLLECTION",
      "fields": [
        {"fieldPath": "capabilities.domain", "arrayConfig": "CONTAINS"},
        {"fieldPath": "status", "order": "ASCENDING"},
        {"fieldPath": "trust_score", "order": "DESCENDING"}
      ]
    }
  ]
}
```

### 5. Redis (Memorystore)

```hcl
# terraform/redis.tf

resource "google_redis_instance" "aex_redis" {
  name           = "aex-redis"
  tier           = "BASIC"
  memory_size_gb = 1
  region         = var.region
  project        = var.project_id

  authorized_network = google_compute_network.aex_vpc.id

  redis_version = "REDIS_7_0"

  display_name = "AEX Cache"

  labels = {
    environment = var.environment
  }
}
```

### 6. Pub/Sub

```hcl
# terraform/pubsub.tf

# Topics
resource "google_pubsub_topic" "aex_work_events" {
  name    = "aex-work-events"
  project = var.project_id

  message_retention_duration = "86400s"  # 24 hours
}

resource "google_pubsub_topic" "aex_bid_events" {
  name    = "aex-bid-events"
  project = var.project_id
}

resource "google_pubsub_topic" "aex_contract_events" {
  name    = "aex-contract-events"
  project = var.project_id
}

resource "google_pubsub_topic" "aex_provider_events" {
  name    = "aex-provider-events"
  project = var.project_id
}

# Subscriptions
resource "google_pubsub_subscription" "bid_evaluator_work" {
  name    = "aex-bid-evaluator-work-sub"
  topic   = google_pubsub_topic.aex_work_events.name
  project = var.project_id

  filter = "attributes.event_type = \"work.submitted\""

  push_config {
    push_endpoint = "https://aex-bid-evaluator-${var.project_number}.${var.region}.run.app/events/work.submitted"

    oidc_token {
      service_account_email = google_service_account.aex_bid_evaluator.email
    }
  }

  ack_deadline_seconds = 30

  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }

  dead_letter_policy {
    dead_letter_topic     = google_pubsub_topic.aex_dlq.id
    max_delivery_attempts = 5
  }
}

resource "google_pubsub_subscription" "contract_engine_awarded" {
  name    = "aex-contract-engine-awarded-sub"
  topic   = google_pubsub_topic.aex_bid_events.name
  project = var.project_id

  filter = "attributes.event_type = \"bids.evaluated\""

  push_config {
    push_endpoint = "https://aex-contract-engine-${var.project_number}.${var.region}.run.app/events/bids.evaluated"

    oidc_token {
      service_account_email = google_service_account.aex_contract_engine.email
    }
  }

  ack_deadline_seconds = 60
}

resource "google_pubsub_subscription" "settlement_completed" {
  name    = "aex-settlement-completed-sub"
  topic   = google_pubsub_topic.aex_contract_events.name
  project = var.project_id

  filter = "attributes.event_type = \"contract.completed\" OR attributes.event_type = \"contract.failed\""

  push_config {
    push_endpoint = "https://aex-settlement-${var.project_number}.${var.region}.run.app/events"

    oidc_token {
      service_account_email = google_service_account.aex_settlement.email
    }
  }

  ack_deadline_seconds = 30
}

# Dead letter queue
resource "google_pubsub_topic" "aex_dlq" {
  name    = "aex-dlq"
  project = var.project_id
}
```

### 7. Secret Manager

```hcl
# terraform/secrets.tf

resource "google_secret_manager_secret" "db_password" {
  secret_id = "aex-db-password"
  project   = var.project_id

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "db_password" {
  secret      = google_secret_manager_secret.db_password.id
  secret_data = random_password.db_password.result
}

resource "google_secret_manager_secret" "api_keys" {
  secret_id = "aex-api-keys"
  project   = var.project_id

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret" "jwt_signing_key" {
  secret_id = "aex-jwt-signing-key"
  project   = var.project_id

  replication {
    auto {}
  }
}
```

### 8. IAM & Service Accounts

```hcl
# terraform/iam.tf

# Service Accounts
resource "google_service_account" "aex_gateway" {
  account_id   = "aex-gateway"
  display_name = "AEX Gateway Service"
  project      = var.project_id
}

resource "google_service_account" "aex_work_publisher" {
  account_id   = "aex-work-publisher"
  display_name = "AEX Work Publisher Service"
  project      = var.project_id
}

resource "google_service_account" "aex_provider_registry" {
  account_id   = "aex-provider-registry"
  display_name = "AEX Provider Registry Service"
  project      = var.project_id
}

resource "google_service_account" "aex_contract_engine" {
  account_id   = "aex-contract-engine"
  display_name = "AEX Contract Engine Service"
  project      = var.project_id
}

resource "google_service_account" "aex_bid_evaluator" {
  account_id   = "aex-bid-evaluator"
  display_name = "AEX Bid Evaluator Service"
  project      = var.project_id
}

resource "google_service_account" "aex_bid_gateway" {
  account_id   = "aex-bid-gateway"
  display_name = "AEX Bid Gateway Service"
  project      = var.project_id
}

resource "google_service_account" "aex_trust_broker" {
  account_id   = "aex-trust-broker"
  display_name = "AEX Trust Broker Service"
  project      = var.project_id
}

resource "google_service_account" "aex_settlement" {
  account_id   = "aex-settlement"
  display_name = "AEX Settlement Service"
  project      = var.project_id
}

# IAM Bindings

# Firestore access
resource "google_project_iam_member" "work_publisher_firestore" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.aex_work_publisher.email}"
}

resource "google_project_iam_member" "provider_registry_firestore" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.aex_provider_registry.email}"
}

resource "google_project_iam_member" "contract_engine_firestore" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.aex_contract_engine.email}"
}

# Pub/Sub access
resource "google_project_iam_member" "work_publisher_pubsub" {
  project = var.project_id
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.aex_work_publisher.email}"
}

resource "google_project_iam_member" "bid_evaluator_pubsub" {
  project = var.project_id
  role    = "roles/pubsub.subscriber"
  member  = "serviceAccount:${google_service_account.aex_bid_evaluator.email}"
}

resource "google_project_iam_member" "contract_engine_pubsub" {
  project = var.project_id
  role    = "roles/pubsub.subscriber"
  member  = "serviceAccount:${google_service_account.aex_contract_engine.email}"
}

# Cloud SQL access
resource "google_project_iam_member" "settlement_cloudsql" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.aex_settlement.email}"
}

# Secret Manager access
resource "google_secret_manager_secret_iam_member" "gateway_secrets" {
  secret_id = google_secret_manager_secret.api_keys.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.aex_gateway.email}"
}
```

### 9. BigQuery

```hcl
# terraform/bigquery.tf

resource "google_bigquery_dataset" "aex_analytics" {
  dataset_id  = "aex_analytics"
  project     = var.project_id
  location    = var.region
  description = "AEX Analytics Dataset"

  default_table_expiration_ms = null  # No expiration

  labels = {
    environment = var.environment
  }
}

resource "google_bigquery_dataset" "aex_telemetry" {
  dataset_id  = "aex_telemetry"
  project     = var.project_id
  location    = var.region
  description = "AEX Telemetry Dataset"

  labels = {
    environment = var.environment
  }
}

resource "google_bigquery_table" "executions" {
  dataset_id = google_bigquery_dataset.aex_analytics.dataset_id
  table_id   = "executions"
  project    = var.project_id

  time_partitioning {
    type  = "DAY"
    field = "created_at"
  }

  clustering = ["domain", "requestor_tenant_id"]

  schema = file("${path.module}/schemas/executions.json")
}
```

### 10. Cloud Run Services

```hcl
# terraform/cloudrun.tf

resource "google_cloud_run_service" "aex_gateway" {
  name     = "aex-gateway"
  location = var.region
  project  = var.project_id

  template {
    spec {
      service_account_name = google_service_account.aex_gateway.email
      containers {
        image = "gcr.io/${var.project_id}/aex-gateway:latest"

        ports {
          container_port = 8080
        }

        resources {
          limits = {
            cpu    = "2"
            memory = "1Gi"
          }
        }

        env {
          name  = "ENV"
          value = var.environment
        }

        env {
          name  = "REDIS_HOST"
          value = google_redis_instance.aex_redis.host
        }
      }
    }

    metadata {
      annotations = {
        "autoscaling.knative.dev/minScale"      = "1"
        "autoscaling.knative.dev/maxScale"      = "100"
        "run.googleapis.com/vpc-access-connector" = google_vpc_access_connector.aex_connector.name
        "run.googleapis.com/vpc-access-egress"    = "private-ranges-only"
      }
    }
  }

  traffic {
    percent         = 100
    latest_revision = true
  }

  autogenerate_revision_name = true
}

# Allow public access to gateway
resource "google_cloud_run_service_iam_member" "gateway_public" {
  service  = google_cloud_run_service.aex_gateway.name
  location = var.region
  project  = var.project_id
  role     = "roles/run.invoker"
  member   = "allUsers"
}

# Similar definitions for other services...
```

## Deployment Order

```bash
# 1. Initialize Terraform
terraform init

# 2. Create base infrastructure
terraform apply -target=google_compute_network.aex_vpc
terraform apply -target=google_compute_subnetwork.aex_subnet
terraform apply -target=google_vpc_access_connector.aex_connector

# 3. Create data stores
terraform apply -target=google_sql_database_instance.aex_postgres
terraform apply -target=google_firestore_database.aex_firestore
terraform apply -target=google_redis_instance.aex_redis

# 4. Create GKE cluster
terraform apply -target=google_container_cluster.aex_cluster

# 5. Create Pub/Sub infrastructure
terraform apply -target=module.pubsub

# 6. Create IAM resources
terraform apply -target=module.iam

# 7. Deploy remaining infrastructure
terraform apply
```

## Cost Estimation (Phase A)

| Resource | SKU | Monthly Est. |
|----------|-----|-------------|
| Cloud Run (6 services) | ~100K requests/day | $50-100 |
| GKE Autopilot | 3 nodes avg | $150-200 |
| Cloud SQL | db-custom-2-4096 | $100-150 |
| Memorystore Redis | 1GB Basic | $35 |
| Firestore | 1M reads, 500K writes | $20-50 |
| Pub/Sub | 10M messages | $10-20 |
| BigQuery | 100GB storage, queries | $20-50 |
| Networking | NAT, LB | $50-100 |
| **Total** | | **~$450-700/month** |

## Directory Structure

```
infrastructure/
├── terraform/
│   ├── main.tf
│   ├── variables.tf
│   ├── outputs.tf
│   ├── networking.tf
│   ├── gke.tf
│   ├── cloudsql.tf
│   ├── firestore.tf
│   ├── redis.tf
│   ├── pubsub.tf
│   ├── bigquery.tf
│   ├── cloudrun.tf
│   ├── iam.tf
│   ├── secrets.tf
│   └── schemas/
│       └── executions.json
├── kubernetes/
│   ├── namespaces.yaml
│   ├── service-accounts.yaml
│   └── network-policies.yaml
├── firestore/
│   └── firestore.indexes.json
└── scripts/
    ├── deploy.sh
    └── destroy.sh
```
