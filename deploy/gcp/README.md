# GCP Deployment Guide

This guide explains how to deploy Agent Exchange (AEX) and demo agents to Google Cloud Platform using **Cloud Run** or **GKE (Google Kubernetes Engine)**.

## Deployment Options

| Feature | Cloud Run | GKE |
|---------|-----------|-----|
| Infrastructure management | Fully managed (serverless) | Managed K8s (Autopilot) or self-managed (Standard) |
| Scale to zero | Yes | No (minimum nodes required in Standard) |
| Startup latency | Cold start possible | Always warm |
| Networking | Per-service URLs | Single Ingress IP, K8s DNS |
| Service discovery | HTTP URLs | K8s DNS (`svc.cluster.local`) |
| Persistent storage | No (use external DB) | PersistentVolumeClaims |
| Cost model | Pay per request | Pay per node-hour |
| Best for | Low/variable traffic, demos | Production, high traffic, complex networking |
| MongoDB | External (Atlas/Cloud SQL) | In-cluster StatefulSet |

## Prerequisites

1. **GCP Project** with billing enabled
2. **gcloud CLI** installed and authenticated
3. **API Keys** for LLM providers:
   - Anthropic API Key (for Claude - used by all agents)

### Additional prerequisites for GKE

4. **kubectl** (`gcloud components install kubectl`)
5. **helm** (https://helm.sh/docs/intro/install/)

## Architecture

### Cloud Run Architecture

```
+-----------------------------------------------------------------------------+
|                              GCP Cloud Run                                   |
|                                                                             |
|  +-----------+    +--------------------------------------------+            |
|  |  Demo UI  |--->|              AEX Gateway                   |            |
|  | (NiceGUI) |    |                (8080)                      |            |
|  +-----------+    +--------------------------------------------+            |
|       |                          |                                          |
|       |          +---------------+----------------+                         |
|       |          |               |                |                         |
|       v          v               v                v                         |
|  +-----------+ +---------+ +-----------+ +-----------------+               |
|  |Orchestrator| |Work Pub | |Bid Gateway| |Provider Registry|               |
|  |  (8103)   | | (8081)  | |  (8082)   | |     (8085)      |               |
|  +-----------+ +---------+ +-----------+ +-----------------+               |
|       |                                          |                          |
|       | A2A                          Skill Search|                          |
|       v                                          |                          |
|  +----------------------------------------------+-+                        |
|  |            Provider Agents (A2A)                |                        |
|  |  +----------+ +----------+ +----------+         |                        |
|  |  |Reviewer A| |Reviewer B| |Reviewer C|         |                        |
|  |  |Budget    | |Standard  | |Premium   |         |                        |
|  |  |(Claude)  | |(Claude)  | |(Claude)  |         |                        |
|  |  +----------+ +----------+ +----------+         |                        |
|  +-------------------------------------------------+                        |
|                                                                             |
|  +-----------------------------------------------------------------------+  |
|  |                         Secret Manager                                |  |
|  |                        ANTHROPIC_API_KEY                              |  |
|  +-----------------------------------------------------------------------+  |
+-----------------------------------------------------------------------------+
```

### GKE Architecture

```
+-----------------------------------------------------------------------------+
|                        GKE Cluster (Autopilot/Standard)                      |
|                                                                             |
|  +-- Namespace: aex -------------------------------------------------------+|
|  |                                                                         ||
|  |  +--- Ingress (nginx) -----------------------------------------------+  ||
|  |  |  External IP -> /api     -> aex-gateway:8080                      |  ||
|  |  |               -> /demo    -> demo-ui-nicegui:8502                 |  ||
|  |  |               -> /agents  -> code-reviewer-{a,b,c}, orchestrator  |  ||
|  |  +-------------------------------------------------------------------+  ||
|  |                                                                         ||
|  |  +--- AEX Core (Deployments) ----------------------------------------+  ||
|  |  |  gateway | work-publisher | bid-gateway | bid-evaluator            |  ||
|  |  |  contract-engine | provider-registry | trust-broker                |  ||
|  |  |  identity | settlement | telemetry | credentials-provider          |  ||
|  |  +-------------------------------------------------------------------+  ||
|  |                                                                         ||
|  |  +--- Code Review Agents (Deployments) ------------------------------+  ||
|  |  |  code-reviewer-a:8100 | code-reviewer-b:8101 | code-reviewer-c:8102||
|  |  |  orchestrator:8103                                                 |  ||
|  |  +-------------------------------------------------------------------+  ||
|  |                                                                         ||
|  |  +--- Payment Agents (Deployments) ----------------------------------+  ||
|  |  |  payment-devpay:8200 | payment-codeauditpay:8201                   |  ||
|  |  |  payment-securitypay:8202                                          |  ||
|  |  +-------------------------------------------------------------------+  ||
|  |                                                                         ||
|  |  +--- MongoDB (StatefulSet) -----------------------------------------+  ||
|  |  |  mongodb:27017 with PersistentVolumeClaim                          |  ||
|  |  +-------------------------------------------------------------------+  ||
|  |                                                                         ||
|  +-------------------------------------------------------------------------+|
|                                                                             |
|  +--- Cluster-level -------------------------------------------------------+|
|  |  ingress-nginx (LoadBalancer) | cert-manager | external-secrets         ||
|  |  Workload Identity -> GCP Secret Manager                                ||
|  +-------------------------------------------------------------------------+|
+-----------------------------------------------------------------------------+
```

## Cloud Run Deployment

### Quick Start

```bash
# Set your project ID
export PROJECT_ID="your-project-id"
export REGION="us-central1"

# Authenticate
gcloud auth login
gcloud config set project $PROJECT_ID

# Deploy everything
cd deploy/gcp
./deploy.sh $PROJECT_ID $REGION
```

### Configure API keys

After deployment, update the secrets with your actual API keys:

```bash
# Anthropic (Claude) - used by all demo agents
echo "sk-ant-..." | gcloud secrets versions add ANTHROPIC_API_KEY --data-file=-
```

### Access the demo

The deploy script will output the Demo UI URL. Open it in your browser to try the demo.

### Manual Deployment

If you prefer to deploy services individually:

```bash
# Build with Cloud Build
gcloud builds submit --config=deploy/gcp/cloudbuild.yaml .

# Deploy a single service
gcloud run deploy aex-gateway \
    --image gcr.io/$PROJECT_ID/aex-gateway:latest \
    --region $REGION \
    --platform managed \
    --allow-unauthenticated
```

## GKE Deployment

### Quick Start

```bash
# Set your project ID
export GCP_PROJECT_ID="your-project-id"
export GCP_REGION="us-central1"

# Option A: Full setup (cluster + deploy)
./hack/deploy/setup-gke.sh
./deploy/gcp/deploy-gke.sh --project-id $GCP_PROJECT_ID

# Option B: Step by step
./deploy/gcp/gke-cluster.sh --project-id $GCP_PROJECT_ID --mode autopilot
./deploy/gcp/deploy-gke.sh --project-id $GCP_PROJECT_ID --environment staging
```

### GKE Autopilot vs Standard

| Feature | Autopilot | Standard |
|---------|-----------|----------|
| Node management | Google-managed | User-managed |
| Node pools | Automatic | Configurable |
| Pricing | Per-pod resource requests | Per-node (VM) |
| HPA/VPA | Automatic | Manual setup |
| GPU/TPU | Supported | Supported |
| Min cost | ~$70/month (base) | ~$150/month (3 e2-standard-4 nodes) |
| Best for | Most workloads | Custom requirements, cost control |

**Recommendation:** Use Autopilot for most cases. Use Standard mode if you need specific node configurations, DaemonSets, or tighter cost control.

### GKE Cluster Setup

```bash
# Autopilot (recommended)
./deploy/gcp/gke-cluster.sh \
    --project-id your-project \
    --region us-central1

# Standard mode (more control)
./deploy/gcp/gke-cluster.sh \
    --project-id your-project \
    --mode standard \
    --min-nodes 2 \
    --max-nodes 5

# Delete cluster
./deploy/gcp/gke-cluster.sh \
    --project-id your-project \
    --delete
```

### GKE Application Deployment

```bash
# Full deploy (build + push + apply manifests)
./deploy/gcp/deploy-gke.sh --project-id your-project

# Deploy to staging
./deploy/gcp/deploy-gke.sh --project-id your-project --environment staging

# Skip build (use existing images)
./deploy/gcp/deploy-gke.sh --project-id your-project --skip-build

# Build images only (no deploy)
./deploy/gcp/deploy-gke.sh --project-id your-project --build-only

# Clean up deployed resources
./deploy/gcp/deploy-gke.sh --project-id your-project --clean
```

### GKE with Cloud Build

Use the GKE-specific Cloud Build configuration:

```bash
# Build and deploy to GKE via Cloud Build
gcloud builds submit --config=deploy/gcp/cloudbuild-gke.yaml \
    --substitutions=_GKE_CLUSTER=aex-cluster,_GKE_REGION=us-central1 .
```

### GKE with Kustomize (Direct)

If you prefer using `kubectl` directly:

```bash
# Create aex-secrets and mongodb-keyfile first; the manifests do not include
# them (values already in the cluster are kept)
ANTHROPIC_API_KEY="sk-ant-..." ./deploy/k8s/create-secrets.sh

# Apply base manifests
kubectl apply -k deploy/k8s/base/

# Apply with environment overlay (if available)
kubectl apply -k deploy/k8s/overlays/staging/
```

## Environment Variables

### AEX Services

| Service | Required Variables |
|---------|-------------------|
| aex-gateway | WORK_PUBLISHER_URL, BID_GATEWAY_URL, etc. |
| aex-provider-registry | MONGO_URI (optional) |
| aex-work-publisher | PROVIDER_REGISTRY_URL |
| aex-bid-gateway | PROVIDER_REGISTRY_URL |
| aex-bid-evaluator | BID_GATEWAY_URL, TRUST_BROKER_URL, WORK_PUBLISHER_URL |
| aex-contract-engine | BID_GATEWAY_URL, WORK_PUBLISHER_URL, SETTLEMENT_URL |
| aex-settlement | CONTRACT_ENGINE_URL, TRUST_BROKER_URL |

### Demo Agents

| Agent | Tier | LLM | Port | Required Secrets |
|-------|------|-----|------|-----------------|
| code-reviewer-a | Budget | Claude | 8100 | ANTHROPIC_API_KEY |
| code-reviewer-b | Standard | Claude | 8101 | ANTHROPIC_API_KEY |
| code-reviewer-c | Premium | Claude | 8102 | ANTHROPIC_API_KEY |
| orchestrator | - | Claude | 8103 | ANTHROPIC_API_KEY |
| payment-devpay | - | - | 8200 | - |
| payment-codeauditpay | - | - | 8201 | - |
| payment-securitypay | - | - | 8202 | - |

## Cost Optimization

### Cloud Run

- All services use **min-instances: 0** to scale to zero when idle
- Services auto-scale based on traffic
- Memory is set conservatively (512Mi-1Gi)

```bash
# Set all services to min-instances 0
for service in aex-gateway aex-provider-registry code-reviewer-a code-reviewer-b code-reviewer-c; do
    gcloud run services update $service --min-instances 0 --region $REGION
done
```

### GKE

- **Autopilot:** Pay only for pod resource requests. Set resource requests/limits carefully.
- **Standard:** Use cluster autoscaler with min-nodes=1 for dev/staging.
- Use `kubectl top pods -n aex` to right-size resource requests.
- Consider Spot/Preemptible nodes for non-production workloads.

```bash
# Scale down non-critical services in dev
kubectl scale deployment code-reviewer-b code-reviewer-c --replicas=0 -n aex

# Check resource usage
kubectl top pods -n aex
kubectl top nodes
```

## Monitoring

### Cloud Run

```bash
gcloud logging read "resource.type=cloud_run_revision" --limit 50
```

Or use the Cloud Console: https://console.cloud.google.com/run

### GKE

```bash
# Pod logs
kubectl logs -n aex deployment/aex-gateway -f

# All pod status
kubectl get pods -n aex -o wide

# Resource usage
kubectl top pods -n aex
kubectl top nodes

# Events (troubleshooting)
kubectl get events -n aex --sort-by='.lastTimestamp'

# Port-forward for local access
kubectl port-forward -n aex svc/aex-gateway 8080:8080
kubectl port-forward -n aex svc/demo-ui-nicegui 8502:8502
```

GKE Dashboard: https://console.cloud.google.com/kubernetes
Cloud Monitoring: https://console.cloud.google.com/monitoring

## Cleanup

### Cloud Run

```bash
# Delete Cloud Run services
for service in demo-ui orchestrator code-reviewer-c code-reviewer-b code-reviewer-a \
    aex-gateway aex-telemetry aex-identity aex-settlement aex-contract-engine \
    aex-bid-evaluator aex-trust-broker aex-bid-gateway aex-work-publisher \
    aex-provider-registry; do
    gcloud run services delete $service --region $REGION --quiet
done

# Delete container images
gcloud container images list --repository gcr.io/$PROJECT_ID | \
    xargs -I {} gcloud container images delete {} --force-delete-tags --quiet
```

### GKE

```bash
# Delete namespace only (keep cluster)
./hack/deploy/teardown-gke.sh namespace

# Delete everything (namespace + Helm charts + cluster + IAM)
./hack/deploy/teardown-gke.sh all

# Or use the cluster script directly
./deploy/gcp/gke-cluster.sh --project-id $PROJECT_ID --delete
```

## Troubleshooting

### Cloud Run

#### Service not starting

```bash
gcloud run services logs read aex-gateway --region $REGION
```

#### Secret not found

```bash
gcloud secrets list
```

#### Connection refused between services

```bash
gcloud run services add-iam-policy-binding SERVICE_NAME \
    --member="allUsers" \
    --role="roles/run.invoker" \
    --region $REGION
```

### GKE

#### Pods stuck in Pending

```bash
# Check events
kubectl describe pod <pod-name> -n aex

# Check node capacity (Standard mode)
kubectl describe nodes | grep -A 5 "Allocated resources"

# Autopilot: pods may take 1-2 minutes to schedule (node provisioning)
```

#### Pods in CrashLoopBackOff

```bash
# Check logs
kubectl logs <pod-name> -n aex --previous

# Check environment variables
kubectl describe pod <pod-name> -n aex | grep -A 20 "Environment"
```

#### Ingress not getting external IP

```bash
# Check ingress-nginx controller
kubectl get svc -n ingress-nginx
kubectl logs -n ingress-nginx deployment/ingress-nginx-controller

# Check ingress resource
kubectl describe ingress -n aex
```

#### Workload Identity issues

```bash
# Verify K8s SA annotation
kubectl describe sa aex-workload -n aex

# Test from a pod
kubectl run test --rm -it --image=google/cloud-sdk:slim \
    --serviceaccount=aex-workload -n aex -- \
    gcloud secrets list --project=$PROJECT_ID
```

## CI/CD

### Cloud Run CI/CD

The existing `.github/workflows/cd.yml` handles Cloud Run deployments on tag pushes and manual dispatch.

### GKE CI/CD

The `.github/workflows/cd-gcp-gke.yml` workflow provides:
- Build and push images to Artifact Registry
- Deploy to GKE staging (automatic on tags)
- Run smoke tests against staging
- Deploy to GKE production (manual approval)
- Uses Workload Identity Federation for authentication

Required GitHub secrets for GKE:
- `GCP_PROJECT_ID` - GCP project ID
- `GCP_WORKLOAD_IDENTITY_PROVIDER` - Workload Identity provider
- `GCP_SERVICE_ACCOUNT` - GitHub Actions service account
- `GKE_CLUSTER_NAME` - GKE cluster name (default: aex-cluster)
- `GKE_CLUSTER_REGION` - GKE cluster region (default: us-central1)
