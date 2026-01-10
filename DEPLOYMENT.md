# Agent Exchange - Production Deployment Guide

This guide covers deploying Agent Exchange to cloud platforms. Choose your preferred provider:

- **Google Cloud Platform (GCP)** - Cloud Run, Firestore
- **Amazon Web Services (AWS)** - ECS Fargate, DocumentDB

## Table of Contents

1. [Overview](#overview)
2. [GCP Deployment](#gcp-deployment)
   - [Prerequisites (GCP)](#prerequisites-gcp)
   - [Setup (GCP)](#setup-gcp)
   - [Database (GCP)](#database-gcp)
   - [Secrets (GCP)](#secrets-gcp)
   - [Deploy (GCP)](#deploy-gcp)
   - [Monitoring (GCP)](#monitoring-gcp)
3. [AWS Deployment](#aws-deployment)
   - [Prerequisites (AWS)](#prerequisites-aws)
   - [Setup (AWS)](#setup-aws)
   - [Database (AWS)](#database-aws)
   - [Deploy (AWS)](#deploy-aws)
   - [Monitoring (AWS)](#monitoring-aws)
4. [CI/CD Pipelines](#cicd-pipelines)
5. [Service Configuration](#service-configuration)
6. [Scaling](#scaling)
7. [Security Best Practices](#security-best-practices)
8. [Troubleshooting](#troubleshooting)
9. [Teardown](#teardown)

---

## Overview

### Architecture

Agent Exchange consists of 10 microservices:

| Service | Port | Description |
|---------|------|-------------|
| `aex-gateway` | 8080 | API gateway, routing |
| `aex-work-publisher` | 8081 | Work specification management |
| `aex-bid-gateway` | 8082 | Bid submission and storage |
| `aex-bid-evaluator` | 8083 | Bid evaluation and ranking |
| `aex-contract-engine` | 8084 | Contract lifecycle management |
| `aex-provider-registry` | 8085 | Provider registration |
| `aex-trust-broker` | 8086 | Trust score management |
| `aex-identity` | 8087 | Tenant and API key management |
| `aex-settlement` | 8088 | Financial transactions |
| `aex-telemetry` | 8089 | Metrics and events |

### Deployment Order

Deploy services in this order due to dependencies:

1. **Infrastructure services**: aex-identity, aex-telemetry
2. **Core services**: aex-provider-registry, aex-trust-broker, aex-settlement
3. **Business services**: aex-bid-gateway, aex-bid-evaluator, aex-work-publisher, aex-contract-engine
4. **Gateway**: aex-gateway

---

## GCP Deployment

### Prerequisites (GCP)

#### Required Tools

```bash
# Google Cloud SDK
curl https://sdk.cloud.google.com | bash
gcloud init

# Docker
# Install from https://docs.docker.com/get-docker/

# Go 1.22+ (for local builds)
# Install from https://go.dev/dl/
```

#### Required Permissions

You need the following IAM roles on your GCP project:

- `roles/owner` or these specific roles:
  - `roles/run.admin`
  - `roles/artifactregistry.admin`
  - `roles/iam.serviceAccountAdmin`
  - `roles/secretmanager.admin`
  - `roles/datastore.owner`
  - `roles/logging.admin`

#### Environment Variables

```bash
export GCP_PROJECT_ID="your-project-id"
export GCP_REGION="us-central1"
```

### Setup (GCP)

#### 1. Create or Select Project

```bash
# Create new project
gcloud projects create $GCP_PROJECT_ID --name="Agent Exchange"

# Or select existing project
gcloud config set project $GCP_PROJECT_ID
```

#### 2. Enable Billing

Ensure billing is enabled in the [GCP Console](https://console.cloud.google.com/billing).

#### 3. Run Setup Script

```bash
./hack/deploy/setup-gcp.sh
```

This script will:
- Enable required APIs
- Create Artifact Registry repository
- Create service accounts with proper roles
- Set up Workload Identity for GitHub Actions
- Create Firestore database
- Create initial secrets

#### 4. Manual API Enablement (if needed)

```bash
gcloud services enable \
  run.googleapis.com \
  artifactregistry.googleapis.com \
  firestore.googleapis.com \
  secretmanager.googleapis.com \
  cloudresourcemanager.googleapis.com \
  iam.googleapis.com \
  iamcredentials.googleapis.com \
  logging.googleapis.com \
  monitoring.googleapis.com \
  cloudtrace.googleapis.com
```

### Database (GCP)

Agent Exchange uses Firestore in Native mode for production.

```bash
# Create database (done by setup script)
gcloud firestore databases create \
  --location=$GCP_REGION \
  --type=firestore-native
```

#### Collections Structure

| Collection | Description |
|------------|-------------|
| `work_specs` | Work specifications |
| `bids` | Provider bids |
| `contracts` | Awarded contracts |
| `providers` | Registered providers |
| `subscriptions` | Category subscriptions |
| `tenants` | Tenant accounts |
| `api_keys` | API keys |
| `trust_records` | Provider trust data |
| `balances` | Account balances |
| `transactions` | Financial transactions |
| `ledger` | Ledger entries |

#### Firestore Indexes

```bash
gcloud firestore indexes composite create \
  --collection-group=work_specs \
  --field-config field-path=consumer_id,order=ASCENDING \
  --field-config field-path=created_at,order=DESCENDING

gcloud firestore indexes composite create \
  --collection-group=bids \
  --field-config field-path=work_id,order=ASCENDING \
  --field-config field-path=created_at,order=ASCENDING
```

### Secrets (GCP)

```bash
# JWT signing secret
echo -n "$(openssl rand -base64 32)" | \
  gcloud secrets create aex-jwt-secret --data-file=-

# API key salt
echo -n "$(openssl rand -base64 32)" | \
  gcloud secrets create aex-api-key-salt --data-file=-
```

### Deploy (GCP)

#### Build and Push Images

```bash
# Authenticate Docker with Artifact Registry
gcloud auth configure-docker ${GCP_REGION}-docker.pkg.dev

# Build all images
make docker-build

# Tag and push
VERSION="v1.0.0"
REGISTRY="${GCP_REGION}-docker.pkg.dev/${GCP_PROJECT_ID}/aex"

for service in aex-gateway aex-work-publisher aex-bid-gateway aex-bid-evaluator \
               aex-contract-engine aex-provider-registry aex-trust-broker \
               aex-identity aex-settlement aex-telemetry; do
  docker tag agent-exchange/${service}:local ${REGISTRY}/${service}:${VERSION}
  docker push ${REGISTRY}/${service}:${VERSION}
done
```

#### Deploy Services

```bash
# Deploy to staging
./hack/deploy/deploy-cloudrun.sh staging all

# Deploy to production
./hack/deploy/deploy-cloudrun.sh production all

# Deploy specific service
./hack/deploy/deploy-cloudrun.sh production aex-gateway
```

#### Get Service URLs

```bash
for service in aex-gateway aex-work-publisher aex-bid-gateway aex-bid-evaluator \
               aex-contract-engine aex-provider-registry aex-trust-broker \
               aex-identity aex-settlement aex-telemetry; do
  URL=$(gcloud run services describe $service --region=$GCP_REGION --format='value(status.url)')
  echo "$service: $URL"
done
```

### Monitoring (GCP)

#### Cloud Monitoring

View metrics in [Cloud Monitoring](https://console.cloud.google.com/monitoring):

```bash
# Key metrics
- cloud.run/request_count
- cloud.run/request_latencies
- cloud.run/container/instance_count
- cloud.run/container/cpu/utilizations
- cloud.run/container/memory/utilizations
```

#### Cloud Logging

```bash
# All AEX logs
gcloud logging read 'resource.type="cloud_run_revision" AND resource.labels.service_name=~"aex-.*"' \
  --limit=100 \
  --format="table(timestamp,resource.labels.service_name,textPayload)"

# Error logs only
gcloud logging read 'resource.type="cloud_run_revision" AND severity>=ERROR' \
  --limit=50
```

#### Rollback (GCP)

```bash
# List revisions
gcloud run revisions list --service=SERVICE_NAME --region=$GCP_REGION

# Route traffic to previous revision
gcloud run services update-traffic SERVICE_NAME \
  --to-revisions=REVISION_NAME=100 \
  --region=$GCP_REGION

# Gradual rollout (90/10 split)
gcloud run services update-traffic SERVICE_NAME \
  --to-revisions=NEW_REVISION=10,OLD_REVISION=90 \
  --region=$GCP_REGION
```

---

## AWS Deployment

### Prerequisites (AWS)

#### Required Tools

```bash
# AWS CLI v2
curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip"
unzip awscliv2.zip
sudo ./aws/install

# Docker
# https://docs.docker.com/engine/install/

# jq (for JSON processing)
sudo apt-get install jq  # Ubuntu/Debian
brew install jq          # macOS
```

#### AWS Account Setup

```bash
aws configure
# Enter your AWS Access Key ID
# Enter your AWS Secret Access Key
# Enter your preferred region (e.g., us-east-1)
# Enter output format (json)
```

#### Verify Access

```bash
aws sts get-caller-identity
```

### Setup (AWS)

#### Environment Variables

```bash
export AWS_REGION="us-east-1"
export AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
```

#### Run Setup Script

```bash
chmod +x hack/deploy/setup-aws.sh
./hack/deploy/setup-aws.sh all
```

This creates:
- ECR repositories for all 10 services
- VPC with public/private subnets across 2 AZs
- Security groups for ALB, ECS, and DocumentDB
- ECS Fargate cluster
- IAM roles for ECS tasks and GitHub Actions
- Secrets Manager secrets
- CloudWatch log groups
- Application Load Balancer

#### Individual Setup Commands

```bash
./hack/deploy/setup-aws.sh ecr      # Create ECR repositories
./hack/deploy/setup-aws.sh vpc      # Create VPC and networking
./hack/deploy/setup-aws.sh ecs      # Create ECS cluster
./hack/deploy/setup-aws.sh iam      # Create IAM roles
./hack/deploy/setup-aws.sh secrets  # Create secrets
./hack/deploy/setup-aws.sh logs     # Create CloudWatch log groups
./hack/deploy/setup-aws.sh alb      # Create ALB
```

### Database (AWS)

#### Option A: Amazon DocumentDB (Managed)

```bash
VPC_ID=$(aws ec2 describe-vpcs --filters "Name=tag:Name,Values=aex-vpc" \
  --query 'Vpcs[0].VpcId' --output text)

PRIVATE_SUBNETS=$(aws ec2 describe-subnets \
  --filters "Name=tag:Name,Values=aex-private-*" "Name=vpc-id,Values=$VPC_ID" \
  --query 'Subnets[*].SubnetId' --output text | tr '\t' ',')

DOCDB_SG=$(aws ec2 describe-security-groups \
  --filters "Name=group-name,Values=aex-docdb-sg" "Name=vpc-id,Values=$VPC_ID" \
  --query 'SecurityGroups[0].GroupId' --output text)

# Create subnet group
aws docdb create-db-subnet-group \
  --db-subnet-group-name aex-docdb-subnet-group \
  --db-subnet-group-description "Agent Exchange DocumentDB Subnet Group" \
  --subnet-ids ${PRIVATE_SUBNETS//,/ }

# Get password from Secrets Manager
DOCDB_PASSWORD=$(aws secretsmanager get-secret-value \
  --secret-id aex-docdb-password \
  --query SecretString --output text)

# Create DocumentDB cluster
aws docdb create-db-cluster \
  --db-cluster-identifier aex-docdb \
  --engine docdb \
  --master-username aexadmin \
  --master-user-password "$DOCDB_PASSWORD" \
  --vpc-security-group-ids "$DOCDB_SG" \
  --db-subnet-group-name aex-docdb-subnet-group

# Create instance
aws docdb create-db-instance \
  --db-instance-identifier aex-docdb-1 \
  --db-cluster-identifier aex-docdb \
  --db-instance-class db.r5.large \
  --engine docdb
```

#### Option B: MongoDB Atlas (External)

1. Create a MongoDB Atlas cluster
2. Configure VPC peering with your AWS VPC
3. Store connection string in Secrets Manager:

```bash
aws secretsmanager create-secret \
  --name aex-mongo-uri \
  --secret-string "mongodb+srv://<USERNAME>:<PASSWORD>@<CLUSTER>.mongodb.net/aex"
```

### Deploy (AWS)

#### Build and Push Images

```bash
# Login to ECR
aws ecr get-login-password --region $AWS_REGION | \
  docker login --username AWS --password-stdin $AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com

# Build and push all services
VERSION=v1.0.0 ./hack/deploy/deploy-ecs.sh build
```

#### Deploy Services

```bash
# Deploy all services to staging
./hack/deploy/deploy-ecs.sh staging all

# Deploy all services to production
./hack/deploy/deploy-ecs.sh production all

# Deploy a single service
./hack/deploy/deploy-ecs.sh staging aex-gateway
```

#### Verify Deployment

```bash
# Check service status
aws ecs describe-services \
  --cluster aex-cluster \
  --services aex-gateway aex-work-publisher \
  --query 'services[*].{name:serviceName,status:status,running:runningCount,desired:desiredCount}'

# Get ALB URL
ALB_DNS=$(aws elbv2 describe-load-balancers --names "aex-alb" \
  --query 'LoadBalancers[0].DNSName' --output text)
echo "Access at: http://$ALB_DNS"

# Test health endpoint
curl http://$ALB_DNS/health
```

### Monitoring (AWS)

#### CloudWatch Logs

```bash
# View recent logs
aws logs tail /ecs/agent-exchange/aex-gateway --follow

# Search logs
aws logs filter-log-events \
  --log-group-name /ecs/agent-exchange/aex-gateway \
  --filter-pattern "ERROR"
```

#### CloudWatch Metrics

| Metric | Description | Threshold |
|--------|-------------|-----------|
| `CPUUtilization` | CPU usage percentage | Alert > 80% |
| `MemoryUtilization` | Memory usage percentage | Alert > 85% |
| `RunningTaskCount` | Number of running tasks | Alert if 0 |
| `HTTPCode_Target_5XX_Count` | 5xx errors | Alert > 10/min |
| `TargetResponseTime` | Response latency | Alert > 2s |

#### Rollback (AWS)

```bash
# Rollback to previous task definition
aws ecs update-service \
  --cluster aex-cluster \
  --service aex-gateway \
  --task-definition aex-gateway:PREVIOUS_REVISION

# Force new deployment
aws ecs update-service --cluster aex-cluster --service aex-gateway \
  --force-new-deployment
```

---

## CI/CD Pipelines

### GCP (Cloud Run)

1. **Add GitHub Secrets:**
   - `GCP_PROJECT_ID`
   - `GCP_REGION`
   - `GCP_WORKLOAD_IDENTITY_PROVIDER`
   - `GCP_SERVICE_ACCOUNT`

2. **Deploy via tag:**
```bash
git tag v1.0.0
git push origin v1.0.0
```

### AWS (ECS Fargate)

1. **Add GitHub Secrets:**
   - `AWS_ROLE_ARN` - `arn:aws:iam::<account-id>:role/aex-github-actions-role`

2. **Add GitHub Variables:**
   - `AWS_REGION` - `us-east-1`

3. **Configure Environments:**
   - `aws-staging` - For staging deployments
   - `aws-production` - For production (add required reviewers)

4. **Deploy via tag:**
```bash
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

---

## Service Configuration

### Resource Allocation

| Service | GCP Memory | GCP CPU | AWS Memory | AWS CPU |
|---------|------------|---------|------------|---------|
| `aex-gateway` | 1Gi | 2 | 1024 MB | 512 |
| `aex-work-publisher` | 512Mi | 1 | 512 MB | 256 |
| `aex-bid-gateway` | 512Mi | 1 | 512 MB | 256 |
| `aex-bid-evaluator` | 512Mi | 1 | 512 MB | 256 |
| `aex-contract-engine` | 512Mi | 1 | 512 MB | 256 |
| `aex-provider-registry` | 512Mi | 1 | 512 MB | 256 |
| `aex-trust-broker` | 512Mi | 1 | 512 MB | 256 |
| `aex-identity` | 512Mi | 1 | 512 MB | 256 |
| `aex-settlement` | 512Mi | 1 | 512 MB | 256 |
| `aex-telemetry` | 256Mi | 1 | 512 MB | 256 |

### Environment Variables

#### All Services

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | HTTP port | `8080` |
| `ENVIRONMENT` | Environment name | `production` |
| `LOG_LEVEL` | Logging level | `info` |

#### Service-Specific

| Service | Variables |
|---------|-----------|
| `aex-gateway` | `WORK_PUBLISHER_URL`, `BID_GATEWAY_URL`, `PROVIDER_REGISTRY_URL`, `SETTLEMENT_URL`, `IDENTITY_URL` |
| `aex-work-publisher` | `STORE_TYPE`, `PROVIDER_REGISTRY_URL` |
| `aex-bid-gateway` | `PROVIDER_REGISTRY_URL` |
| `aex-bid-evaluator` | `BID_GATEWAY_URL`, `TRUST_BROKER_URL` |
| `aex-contract-engine` | `BID_GATEWAY_URL`, `SETTLEMENT_URL` |

---

## Scaling

### GCP Auto-scaling

```bash
# High-throughput services (gateway)
--concurrency=250

# CPU-intensive services (bid-evaluator)
--concurrency=50

# Cold start optimization
--min-instances=1 --cpu-boost
```

### AWS Auto-scaling

```bash
# Register scalable target
aws application-autoscaling register-scalable-target \
  --service-namespace ecs \
  --resource-id service/aex-cluster/aex-gateway \
  --scalable-dimension ecs:service:DesiredCount \
  --min-capacity 1 \
  --max-capacity 10

# Create scaling policy (target tracking)
aws application-autoscaling put-scaling-policy \
  --service-namespace ecs \
  --resource-id service/aex-cluster/aex-gateway \
  --policy-name cpu-tracking \
  --policy-type TargetTrackingScaling \
  --target-tracking-scaling-policy-configuration '{
    "TargetValue": 70.0,
    "PredefinedMetricSpecification": {
      "PredefinedMetricType": "ECSServiceAverageCPUUtilization"
    },
    "ScaleOutCooldown": 60,
    "ScaleInCooldown": 300
  }'

# Manual scaling
aws ecs update-service \
  --cluster aex-cluster \
  --service aex-gateway \
  --desired-count 5
```

---

## Security Best Practices

### Authentication

- Disable unauthenticated access in production
- Use service-to-service authentication
- Validate API keys against Identity service
- Implement rate limiting

### Network Security

#### GCP
```bash
# VPC Connector for private networking
--vpc-connector=aex-connector
--vpc-egress=private-ranges-only
```

#### AWS
- Services run in private subnets
- ALB is the only public-facing component
- Security groups restrict traffic between tiers

### Secret Management

- Never commit secrets to code
- Use Secret Manager (GCP) or Secrets Manager (AWS)
- Rotate secrets regularly
- Use separate secrets per environment

---

## Troubleshooting

### GCP

#### Service Won't Start

```bash
gcloud run services logs read SERVICE_NAME --region=$GCP_REGION
gcloud run revisions describe REVISION_NAME --region=$GCP_REGION
```

#### Connection Refused

```bash
curl -H "Authorization: Bearer $(gcloud auth print-identity-token)" \
  https://target-service-xxx.run.app/health
```

### AWS

#### Task Fails to Start

```bash
aws ecs describe-tasks \
  --cluster aex-cluster \
  --tasks $(aws ecs list-tasks --cluster aex-cluster --service-name aex-gateway \
    --desired-status STOPPED --query 'taskArns[0]' --output text) \
  --query 'tasks[0].stoppedReason'
```

#### Service Unhealthy

```bash
aws elbv2 describe-target-health \
  --target-group-arn $(aws elbv2 describe-target-groups \
    --names aex-gateway-tg --query 'TargetGroups[0].TargetGroupArn' --output text)
```

#### Database Connection Issues

```bash
aws ecs execute-command \
  --cluster aex-cluster \
  --task <task-id> \
  --container aex-gateway \
  --interactive \
  --command "/bin/sh"
```

---

## Teardown

### Validate Before Teardown

```bash
# GCP - validate what will be deleted
./hack/deploy/teardown-gcp.sh validate

# AWS - validate what will be deleted
./hack/deploy/teardown-aws.sh validate
```

### Execute Teardown

```bash
# GCP - delete all resources
./hack/deploy/teardown-gcp.sh

# AWS - delete all resources
./hack/deploy/teardown-aws.sh
```

### Make Targets

```bash
# GCP
make gcp-teardown

# AWS
make aws-teardown
```

---

## Support Resources

### GCP
- [Cloud Run Documentation](https://cloud.google.com/run/docs)
- [Firestore Documentation](https://cloud.google.com/firestore/docs)
- [Secret Manager Documentation](https://cloud.google.com/secret-manager/docs)

### AWS
- [ECS Documentation](https://docs.aws.amazon.com/ecs/)
- [Fargate Documentation](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/AWS_Fargate.html)
- [DocumentDB Documentation](https://docs.aws.amazon.com/documentdb/)

---

*Last Updated: 2025-12-30*
