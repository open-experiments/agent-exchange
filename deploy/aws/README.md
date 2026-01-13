# AWS Deployment Guide

This guide explains how to deploy Agent Exchange (AEX) and demo agents to AWS using ECS Fargate and CloudFormation.

## Prerequisites

1. **AWS Account** with appropriate permissions
2. **AWS CLI** installed and configured (`aws configure`)
3. **Docker** installed locally for building images
4. **API Keys** for LLM providers:
   - Anthropic API Key (for Claude - used by all agents)

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              AWS Cloud                                       │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                    Application Load Balancer                         │    │
│  │                         (Public)                                     │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│           │                                      │                          │
│           │ /api/*                              │ /*                        │
│           ▼                                      ▼                          │
│  ┌─────────────┐                        ┌─────────────┐                    │
│  │ AEX Gateway │                        │   Demo UI   │                    │
│  │   (8080)    │                        │  (Streamlit)│                    │
│  └─────────────┘                        └─────────────┘                    │
│           │                                      │                          │
│  ┌────────┴──────────────────────────────────────┴───────┐                 │
│  │                    ECS Fargate Cluster                 │                 │
│  │   ┌──────────────┐ ┌──────────────┐ ┌──────────────┐  │                 │
│  │   │Provider      │ │Work          │ │Bid           │  │                 │
│  │   │Registry:8085 │ │Publisher:8081│ │Gateway:8082  │  │                 │
│  │   └──────────────┘ └──────────────┘ └──────────────┘  │                 │
│  │   ┌──────────────┐ ┌──────────────┐ ┌──────────────┐  │                 │
│  │   │Bid           │ │Contract      │ │Settlement    │  │                 │
│  │   │Evaluator:8083│ │Engine:8084   │ │:8086         │  │                 │
│  │   └──────────────┘ └──────────────┘ └──────────────┘  │                 │
│  │   ┌──────────────┐ ┌──────────────┐ ┌──────────────┐  │                 │
│  │   │Trust         │ │Identity      │ │Telemetry     │  │                 │
│  │   │Broker:8088   │ │:8089         │ │:8090         │  │                 │
│  │   └──────────────┘ └──────────────┘ └──────────────┘  │                 │
│  │                                                        │                 │
│  │   ┌──────────────────────────────────────────────┐    │                 │
│  │   │           Demo Agents (A2A)                   │    │                 │
│  │   │  ┌─────────┐ ┌─────────┐ ┌─────────┐        │    │                 │
│  │   │  │Legal A  │ │Legal B  │ │Legal C  │        │    │                 │
│  │   │  │:8100    │ │:8101    │ │:8102    │        │    │                 │
│  │   │  └─────────┘ └─────────┘ └─────────┘        │    │                 │
│  │   │  ┌─────────────────────────────────┐        │    │                 │
│  │   │  │      Orchestrator:8103          │        │    │                 │
│  │   │  └─────────────────────────────────┘        │    │                 │
│  │   └──────────────────────────────────────────────┘    │                 │
│  └────────────────────────────────────────────────────────┘                 │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                       Secrets Manager                                │    │
│  │  ANTHROPIC_API_KEY │ MONGO_URI │ JWT_SIGNING_KEY                    │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                              ECR                                     │    │
│  │            (Container Registry for all images)                       │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Quick Start

### 1. Configure AWS CLI

```bash
aws configure
# Enter your AWS Access Key ID, Secret Access Key, and region
```

### 2. Deploy Everything

```bash
cd deploy/aws
./deploy.sh us-east-1 aex
```

This will:
- Create VPC, subnets, and networking
- Create ECS cluster and ECR repositories
- Create Secrets Manager secrets (with placeholders)
- Build and push all Docker images
- Deploy all ECS services
- Configure Application Load Balancer

### 3. Update Secrets

After deployment, update the secrets with your actual values:

```bash
# Anthropic API Key
aws secretsmanager update-secret \
  --secret-id aex/anthropic-api-key \
  --secret-string '{"api_key":"sk-ant-your-key-here"}' \
  --region us-east-1

# MongoDB URI (if using MongoDB)
aws secretsmanager update-secret \
  --secret-id aex/mongo-uri \
  --secret-string '{"uri":"mongodb+srv://<USERNAME>:<PASSWORD>@<CLUSTER>.mongodb.net/aex"}' \
  --region us-east-1
```

### 4. Force Service Update

After updating secrets, force a new deployment:

```bash
aws ecs update-service \
  --cluster aex-cluster \
  --service gateway \
  --force-new-deployment \
  --region us-east-1
```

## CloudFormation Stacks

The deployment creates two CloudFormation stacks:

### infrastructure.yaml

Creates foundational resources:
- VPC with public and private subnets
- Internet Gateway and NAT Gateway
- Security Groups
- ECS Cluster
- ECR Repositories (15 total)
- Application Load Balancer
- Secrets Manager secrets
- IAM roles for ECS
- CloudWatch Log Group
- Service Discovery namespace

### services.yaml

Creates ECS services:
- Task Definitions for all 15 services
- ECS Services with Fargate launch type
- Service Discovery registrations
- ALB Target Groups and Listener Rules

## Manual Deployment

If you prefer to deploy step by step:

### Deploy Infrastructure

```bash
aws cloudformation deploy \
  --template-file infrastructure.yaml \
  --stack-name aex-infrastructure \
  --parameter-overrides EnvironmentName=aex \
  --capabilities CAPABILITY_NAMED_IAM \
  --region us-east-1
```

### Build and Push Images

```bash
# Login to ECR
aws ecr get-login-password --region us-east-1 | \
  docker login --username AWS --password-stdin \
  123456789012.dkr.ecr.us-east-1.amazonaws.com

# Build and push (example for gateway)
docker build -t 123456789012.dkr.ecr.us-east-1.amazonaws.com/aex/aex-gateway:latest \
  -f src/aex-gateway/Dockerfile src/
docker push 123456789012.dkr.ecr.us-east-1.amazonaws.com/aex/aex-gateway:latest
```

### Deploy Services

```bash
aws cloudformation deploy \
  --template-file services.yaml \
  --stack-name aex-services \
  --parameter-overrides EnvironmentName=aex ImageTag=latest \
  --region us-east-1
```

## CI/CD with CodeBuild

The `buildspec.yaml` file configures AWS CodeBuild to automatically build and push images.

### Setup CodeBuild Project

1. Create a CodeBuild project in AWS Console
2. Connect to your GitHub repository
3. Use `deploy/aws/buildspec.yaml` as the buildspec file
4. Set environment variables:
   - `AWS_ACCOUNT_ID`: Your AWS account ID
   - `AWS_DEFAULT_REGION`: Target region

### Trigger Builds

Builds can be triggered:
- On push to main branch
- Manually from AWS Console
- Via AWS CLI: `aws codebuild start-build --project-name aex-build`

## Environment Variables

### AEX Services

| Service | Port | Environment Variables |
|---------|------|----------------------|
| aex-gateway | 8080 | All service URLs |
| aex-provider-registry | 8085 | MONGO_URI |
| aex-work-publisher | 8081 | PROVIDER_REGISTRY_URL |
| aex-bid-gateway | 8082 | PROVIDER_REGISTRY_URL |
| aex-bid-evaluator | 8083 | BID_GATEWAY_URL, TRUST_BROKER_URL |
| aex-contract-engine | 8084 | BID_GATEWAY_URL, WORK_PUBLISHER_URL |
| aex-settlement | 8086 | CONTRACT_ENGINE_URL, TRUST_BROKER_URL |
| aex-trust-broker | 8088 | - |
| aex-identity | 8089 | - |
| aex-telemetry | 8090 | - |

### Demo Agents

| Agent | Port | Secrets |
|-------|------|---------|
| legal-agent-a | 8100 | ANTHROPIC_API_KEY |
| legal-agent-b | 8101 | ANTHROPIC_API_KEY |
| legal-agent-c | 8102 | ANTHROPIC_API_KEY |
| orchestrator | 8103 | ANTHROPIC_API_KEY |
| demo-ui | 8501 | - |

## Cost Optimization

### Use Fargate Spot

The infrastructure enables Fargate Spot capacity provider. To use spot instances:

```bash
aws ecs update-service \
  --cluster aex-cluster \
  --service gateway \
  --capacity-provider-strategy capacityProvider=FARGATE_SPOT,weight=1 \
  --region us-east-1
```

### Scale Down When Not in Use

```bash
# Scale all services to 0
for service in gateway provider-registry work-publisher bid-gateway \
  bid-evaluator contract-engine settlement trust-broker identity \
  telemetry legal-agent-a legal-agent-b legal-agent-c orchestrator demo-ui; do
  aws ecs update-service \
    --cluster aex-cluster \
    --service $service \
    --desired-count 0 \
    --region us-east-1
done
```

### Scale Up

```bash
# Scale all services to 1
for service in gateway provider-registry work-publisher bid-gateway \
  bid-evaluator contract-engine settlement trust-broker identity \
  telemetry legal-agent-a legal-agent-b legal-agent-c orchestrator demo-ui; do
  aws ecs update-service \
    --cluster aex-cluster \
    --service $service \
    --desired-count 1 \
    --region us-east-1
done
```

## Monitoring

### View Service Status

```bash
aws ecs list-services --cluster aex-cluster --region us-east-1
```

### View Running Tasks

```bash
aws ecs list-tasks --cluster aex-cluster --region us-east-1
```

### View Logs

```bash
# Tail logs for all services
aws logs tail /ecs/aex --follow --region us-east-1

# Filter by service
aws logs tail /ecs/aex --follow --filter-pattern "gateway" --region us-east-1
```

### CloudWatch Dashboard

Create a dashboard in CloudWatch to monitor:
- ECS CPU/Memory utilization
- ALB request counts and latency
- Error rates

## Cleanup

To delete all resources:

```bash
# Delete services stack first
aws cloudformation delete-stack \
  --stack-name aex-services \
  --region us-east-1

# Wait for deletion
aws cloudformation wait stack-delete-complete \
  --stack-name aex-services \
  --region us-east-1

# Delete infrastructure stack
aws cloudformation delete-stack \
  --stack-name aex-infrastructure \
  --region us-east-1

# Note: ECR repositories with images won't be deleted automatically
# Delete them manually if needed:
for repo in aex-gateway aex-provider-registry aex-work-publisher \
  aex-bid-gateway aex-bid-evaluator aex-contract-engine aex-settlement \
  aex-trust-broker aex-identity aex-telemetry legal-agent-a legal-agent-b \
  legal-agent-c orchestrator demo-ui; do
  aws ecr delete-repository \
    --repository-name aex/$repo \
    --force \
    --region us-east-1
done
```

## Troubleshooting

### Service Not Starting

```bash
# Check service events
aws ecs describe-services \
  --cluster aex-cluster \
  --services gateway \
  --region us-east-1

# Check task failures
aws ecs describe-tasks \
  --cluster aex-cluster \
  --tasks $(aws ecs list-tasks --cluster aex-cluster --service-name gateway --query 'taskArns[0]' --output text) \
  --region us-east-1
```

### Image Pull Errors

Ensure the ECR repository exists and has the image:

```bash
aws ecr describe-images \
  --repository-name aex/aex-gateway \
  --region us-east-1
```

### Secret Access Issues

Verify the task role has permission to access secrets:

```bash
aws secretsmanager get-secret-value \
  --secret-id aex/anthropic-api-key \
  --region us-east-1
```

### Network Connectivity

Check that services are registered in Service Discovery:

```bash
aws servicediscovery list-instances \
  --service-id <service-id> \
  --region us-east-1
```
