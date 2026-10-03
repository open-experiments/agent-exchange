# AWS Deployment Guide

This guide explains how to deploy Agent Exchange (AEX) and demo agents to AWS. Two deployment options are supported:

- **ECS Fargate** -- Serverless containers, simpler operational model
- **EKS (Kubernetes)** -- Full Kubernetes, richer ecosystem, advanced scheduling

Both options share the same infrastructure foundation (VPC, ECR, Secrets Manager).

## Prerequisites

### Common (both ECS and EKS)

1. **AWS Account** with appropriate permissions
2. **AWS CLI** installed and configured (`aws configure`)
3. **Docker** installed locally for building images
4. **API Keys** for LLM providers:
   - Anthropic API Key (for Claude - used by all agents)

### Additional for EKS

5. **kubectl** -- Kubernetes CLI ([install](https://kubernetes.io/docs/tasks/tools/))
6. **helm** -- Kubernetes package manager ([install](https://helm.sh/docs/intro/install/))
7. **eksctl** (optional) -- EKS management CLI ([install](https://eksctl.io/installation/))

## ECS Fargate Architecture

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
| aex-bid-evaluator | 8083 | BID_GATEWAY_URL, TRUST_BROKER_URL, WORK_PUBLISHER_URL |
| aex-contract-engine | 8084 | BID_GATEWAY_URL, WORK_PUBLISHER_URL, SETTLEMENT_URL |
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

---

## EKS Deployment (Kubernetes)

### EKS Architecture

```
+-----------------------------------------------------------------------------+
|                               AWS Cloud                                      |
|                                                                              |
|  +-----------------------------------------------------------------------+  |
|  |                    Nginx Ingress / AWS ALB                             |  |
|  |                        (Public LB)                                    |  |
|  +-----------------------------------------------------------------------+  |
|           |                                      |                           |
|           | /api/*                              | /*                         |
|           v                                      v                           |
|  +-------------+                        +---------------+                    |
|  | AEX Gateway |                        | Demo UI       |                    |
|  | (8080)      |                        | NiceGUI(8502) |                    |
|  +-------------+                        +---------------+                    |
|           |                                      |                           |
|  +--------+----------------------------------------------+                   |
|  |                    EKS Cluster (K8s)                   |                   |
|  |  Namespace: aex                                        |                   |
|  |                                                        |                   |
|  |  +--- AEX Core (Deployments) -----------------------+ |                   |
|  |  | provider-registry  work-publisher  bid-gateway    | |                   |
|  |  | bid-evaluator  contract-engine  settlement        | |                   |
|  |  | trust-broker  identity  telemetry                 | |                   |
|  |  | credentials-provider                              | |                   |
|  |  +---------------------------------------------------+ |                   |
|  |                                                        |                   |
|  |  +--- Code Review Demo (Deployments) ---------------+ |                   |
|  |  | code-reviewer-a  code-reviewer-b  code-reviewer-c | |                   |
|  |  | orchestrator                                      | |                   |
|  |  +---------------------------------------------------+ |                   |
|  |                                                        |                   |
|  |  +--- Payment Agents (Deployments) -----------------+ |                   |
|  |  | payment-devpay  payment-codeauditpay              | |                   |
|  |  | payment-securitypay                               | |                   |
|  |  +---------------------------------------------------+ |                   |
|  |                                                        |                   |
|  |  +--- Data (StatefulSet) ---------------------------+ |                   |
|  |  | MongoDB:27017                                     | |                   |
|  |  +---------------------------------------------------+ |                   |
|  |                                                        |                   |
|  |  Add-ons: CoreDNS, kube-proxy, vpc-cni, ebs-csi     |                   |
|  |  Helm: AWS LB Controller, Nginx Ingress,             |                   |
|  |        External Secrets, Metrics Server              |                   |
|  +--------------------------------------------------------+                   |
|                                                                              |
|  +--- Shared Infrastructure (CloudFormation) ----------------------------+  |
|  | VPC (public+private subnets) | ECR | Secrets Manager | IAM | ALB     |  |
|  +-----------------------------------------------------------------------+  |
+------------------------------------------------------------------------------+
```

### EKS Quick Start

```bash
# 1. Set up prerequisites and create cluster
hack/deploy/setup-eks.sh

# 2. Deploy all services to EKS
deploy/aws/deploy-eks.sh --region us-east-1 --env dev

# 3. Verify
kubectl get pods -n aex

# 4. Access services locally
kubectl port-forward svc/aex-gateway 8080:8080 -n aex
kubectl port-forward svc/demo-ui-nicegui 8502:8502 -n aex
```

### EKS Quick Start (Step by Step)

```bash
# 1. Install prerequisites
hack/deploy/setup-eks.sh prerequisites

# 2. Validate configuration
hack/deploy/setup-eks.sh validate

# 3. Create the cluster
hack/deploy/setup-eks.sh cluster

# 4. Install add-ons (LB controller, ingress, metrics)
hack/deploy/setup-eks.sh addons

# 5. Deploy services
deploy/aws/deploy-eks.sh --region us-east-1 --env dev
```

### EKS CloudFormation Stack

The EKS deployment adds one CloudFormation stack on top of the shared infrastructure:

#### eks-cluster.yaml

Creates EKS-specific resources:
- EKS cluster (Kubernetes 1.29)
- Managed node group (auto-scaling)
- IAM roles: cluster role, node group role
- OIDC provider for IRSA (IAM Roles for Service Accounts)
- IRSA roles: pod role, AWS LB controller role, EBS CSI driver role
- Security groups: cluster SG, node SG
- EKS add-ons: CoreDNS, kube-proxy, vpc-cni, ebs-csi-driver
- CloudWatch log groups

Parameters:

| Parameter | Default | Description |
|-----------|---------|-------------|
| EnvironmentName | aex | Prefix for resource names |
| Environment | dev | dev, staging, or production |
| ClusterName | aex-eks | EKS cluster name |
| KubernetesVersion | 1.29 | K8s version |
| NodeInstanceType | t3.medium | EC2 instance type |
| MinSize | 2 | Min nodes (3 for prod) |
| MaxSize | 5 | Max nodes (10 for prod) |
| DesiredSize | 2 | Desired nodes (3 for prod) |

### ECS vs EKS Comparison

| Feature | ECS Fargate | EKS |
|---------|-------------|-----|
| **Compute** | Serverless (no nodes) | Managed EC2 node groups |
| **Complexity** | Lower | Higher (full K8s) |
| **Scaling** | Per-task auto-scaling | HPA + Cluster Autoscaler |
| **Service Discovery** | AWS Cloud Map | K8s DNS (CoreDNS) |
| **Secrets** | ECS Secrets integration | External Secrets Operator / IRSA |
| **Load Balancing** | ALB target groups | Ingress controllers |
| **Cost (dev)** | ~$150/month | ~$183/month (+EKS fee) |
| **Cost (prod)** | Higher per-task cost | Better at scale with Spot |
| **Ecosystem** | AWS-native | K8s ecosystem (Helm, Kustomize) |
| **Portability** | AWS only | Multi-cloud capable |
| **CI/CD** | cd-aws.yml | cd-aws-eks.yml |
| **Deploy script** | deploy.sh | deploy-eks.sh |
| **Setup script** | hack/deploy/setup-aws.sh | hack/deploy/setup-eks.sh |
| **Teardown** | hack/deploy/teardown-aws.sh | hack/deploy/teardown-eks.sh |

### EKS Monitoring

#### CloudWatch Container Insights

EKS cluster logging is enabled for all control plane log types (api, audit, authenticator, controllerManager, scheduler). Logs are stored in:

```
/aws/eks/aex-eks/cluster          -- Control plane logs
/aws/containerinsights/aex-eks/   -- Container Insights
```

#### kubectl Monitoring

```bash
# View pod status
kubectl get pods -n aex -o wide

# View pod resource usage (requires metrics-server)
kubectl top pods -n aex

# View node resource usage
kubectl top nodes

# View pod logs
kubectl logs -f deployment/aex-gateway -n aex

# Describe a problematic pod
kubectl describe pod <pod-name> -n aex

# View events
kubectl get events -n aex --sort-by='.lastTimestamp'
```

### EKS Scaling

#### Horizontal Pod Autoscaler (HPA)

```bash
# Create HPA for gateway (2-10 replicas, target 70% CPU)
kubectl autoscale deployment aex-gateway \
  --min=2 --max=10 --cpu-percent=70 -n aex

# View HPA status
kubectl get hpa -n aex
```

#### Manual Scaling

```bash
# Scale a specific deployment
kubectl scale deployment/aex-gateway --replicas=3 -n aex

# Scale all deployments to 0 (cost saving)
kubectl get deployments -n aex -o name | \
  xargs -I {} kubectl scale {} --replicas=0 -n aex

# Scale all back to 1
kubectl get deployments -n aex -o name | \
  xargs -I {} kubectl scale {} --replicas=1 -n aex
```

#### Cluster Autoscaler

The cluster autoscaler is installed by `setup-eks.sh` and automatically adjusts the number of nodes based on pod scheduling demands. Configuration:

- Scale-down delay after add: 5 minutes
- Scale-down unneeded time: 5 minutes
- Expander strategy: least-waste

### EKS Cleanup

```bash
# Delete EKS resources only (keep shared infrastructure)
hack/deploy/teardown-eks.sh

# Delete everything including shared VPC, ECR, secrets
hack/deploy/teardown-eks.sh --include-infra

# Quick cleanup via deploy script
deploy/aws/deploy-eks.sh --clean
```

### EKS Troubleshooting

#### Pods Not Starting

```bash
# Check pod status and events
kubectl describe pod <pod-name> -n aex

# Check if image pull is failing
kubectl get events -n aex --field-selector reason=Failed

# Verify ECR images exist
aws ecr describe-images --repository-name aex/aex-gateway --region us-east-1
```

#### Node Issues

```bash
# Check node status
kubectl get nodes -o wide

# Check node conditions
kubectl describe node <node-name>

# View cluster autoscaler logs
kubectl logs -f deployment/cluster-autoscaler -n kube-system
```

#### IRSA / Secret Issues

```bash
# Verify service account annotation
kubectl get sa aex-service-account -n aex -o yaml

# Test secret access from a pod
kubectl exec -it deployment/aex-gateway -n aex -- \
  env | grep -i secret

# Check External Secrets Operator
kubectl get externalsecrets -n aex
```

#### Ingress Not Working

```bash
# Check ingress status
kubectl get ingress -n aex

# Check nginx-ingress controller logs
kubectl logs -f deployment/ingress-nginx-controller -n ingress-nginx

# Check AWS LB controller logs
kubectl logs -f deployment/aws-load-balancer-controller -n kube-system
```
