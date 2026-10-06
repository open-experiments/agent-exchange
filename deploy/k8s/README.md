# Agent Exchange (AEX) - Kubernetes Deployment

Production-ready Kubernetes manifests for deploying the Agent Exchange code review demo, including all AEX core microservices, code review agents, payment agents, and the NiceGUI dashboard.

## Architecture Overview

```
                         Ingress (nginx)
                        /              \
                       /                \
              /api/* -->              / -->
          aex-gateway:8080      demo-ui-nicegui:8502
               |                       |
    +----------+----------+            |
    |    AEX Core Services |           |
    |  (11 Go microservices)|          |
    +----------+----------+            |
               |                       |
         +-----+-----+         +------+------+
         |  MongoDB   |         | Code Review |
         | StatefulSet|         |   Agents    |
         +------------+         |  + Payment  |
                                |   Agents    |
                                +-------------+
```

### Services

| Category | Services | Count |
|----------|----------|-------|
| Database | MongoDB 7 (StatefulSet) | 1 |
| AEX Core | gateway, work-publisher, bid-gateway, bid-evaluator, contract-engine, provider-registry, trust-broker, identity, settlement, telemetry, credentials-provider | 11 |
| Code Review Agents | code-reviewer-a (QuickReview), code-reviewer-b (CodeGuard), code-reviewer-c (ArchitectAI), orchestrator | 4 |
| Payment Agents | payment-devpay, payment-codeauditpay, payment-securitypay | 3 |
| UI | demo-ui-nicegui (NiceGUI WebSocket dashboard) | 1 |
| **Total** | | **20** |

## Prerequisites

- Kubernetes cluster (v1.25+)
- `kubectl` (v1.25+)
- `kustomize` (v5.0+) or `kubectl` with kustomize support
- Nginx Ingress Controller (for ingress routing)
- Container images built and pushed to a registry

### For Local Development

- [minikube](https://minikube.sigs.k8s.io/) or [kind](https://kind.sigs.k8s.io/)
- Docker (for building images locally)

## Quick Start (Local Development)

### 1. Start a Local Cluster

**Using minikube:**

```bash
minikube start --memory 8192 --cpus 4
minikube addons enable ingress
```

**Using kind:**

```bash
kind create cluster --name aex
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
```

### 2. Set Up Secrets

No overlay ships Secret values. `kubectl apply -k` expects two Secrets to
already exist in the `aex` namespace and never creates or overwrites them:

- `aex-secrets`: credentials referenced by the services (keys below)
- `mongodb-keyfile`: the replica set keyfile mounted by the MongoDB StatefulSet

Create both with one command. It creates the namespace if needed, generates
random values for everything it can, keeps values that already exist in the
cluster (re-running it never rotates anything), and replaces known
placeholders such as `REPLACE_ME`, `root`/`root` or `dev-api-key`:

```bash
ANTHROPIC_API_KEY="sk-ant-your-key-here" ./deploy/k8s/create-secrets.sh
```

Any key can be supplied through an environment variable of the same name,
e.g. `MONGO_URI=...` to use an external MongoDB, or `AEX_API_KEY=...` once
aex-identity has issued an API key for the demo agents.

To create the Secret by hand instead, set every key the manifests reference:

```bash
kubectl apply -f deploy/k8s/namespace.yaml

MONGO_USERNAME=aex-admin
MONGO_PASSWORD="$(openssl rand -hex 24)"

kubectl create secret generic aex-secrets \
  --namespace aex \
  --from-literal=JWT_SECRET="$(openssl rand -base64 48)" \
  --from-literal=JWT_SIGNING_KEY="$(openssl rand -base64 48)" \
  --from-literal=WEBHOOK_SECRET="$(openssl rand -base64 48)" \
  --from-literal=MONGO_USERNAME="$MONGO_USERNAME" \
  --from-literal=MONGO_PASSWORD="$MONGO_PASSWORD" \
  --from-literal=MONGO_URI="mongodb://$MONGO_USERNAME:$MONGO_PASSWORD@mongodb.aex.svc.cluster.local:27017/?authSource=admin" \
  --from-literal=AEX_API_KEY="<api key issued by aex-identity>" \
  --from-literal=ANTHROPIC_API_KEY="sk-ant-your-key-here" \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl create secret generic mongodb-keyfile \
  --namespace aex \
  --from-literal=keyfile="$(openssl rand -base64 756 | tr -d '\n')"
```

| Key | Used by | Value |
|-----|---------|-------|
| `JWT_SECRET` | aex-gateway (verifies HS256 JWTs) | Random, **at least 32 bytes**. Anyone who knows it can mint tokens for any tenant and scope; the gateway refuses to start outside `ENVIRONMENT=development` with an empty, short or placeholder value. |
| `JWT_SIGNING_KEY` | aex-identity | Random, at least 32 bytes |
| `WEBHOOK_SECRET` | work-publisher, settlement, certauth | Random, at least 32 bytes |
| `MONGO_USERNAME` / `MONGO_PASSWORD` | MongoDB StatefulSet, backup CronJob | Generated password, never `root`/`root` |
| `MONGO_URI` | all AEX services | Must embed the same credentials (or point at an external MongoDB) |
| `AEX_API_KEY` | code review / orchestrator agents | API key issued by aex-identity |
| `ANTHROPIC_API_KEY` | code review / orchestrator agents | Your Anthropic API key |

`base/secrets.example.yaml` documents the same keys. It is not part of any
kustomization; do not apply it.

### 3. Deploy with Kustomize

**Development (local images, 1 replica, small resources):**

```bash
kubectl apply -k deploy/k8s/overlays/dev/
```

**Staging (2 replicas, moderate resources):**

```bash
kubectl apply -k deploy/k8s/overlays/staging/
```

**Production (HPA, PDB, NetworkPolicy, larger resources):**

```bash
kubectl apply -k deploy/k8s/overlays/production/
```

### 4. Verify Deployment

```bash
# Check all pods are running
kubectl get pods -n aex

# Check all services
kubectl get svc -n aex

# Watch pod status
kubectl get pods -n aex -w

# Check deployment rollout status
kubectl rollout status deployment/aex-gateway -n aex
```

### 5. Access the Application

**With minikube:**

```bash
# Get the UI URL
minikube service demo-ui-nicegui -n aex --url

# Or use port-forward
kubectl port-forward svc/demo-ui-nicegui 8502:8502 -n aex
```

**With Ingress:**

```bash
# Get the ingress address
kubectl get ingress -n aex

# Access:
# UI:  http://<INGRESS_IP>/
# API: http://<INGRESS_IP>/api/
```

## Directory Structure

```
deploy/k8s/
├── README.md                              # This file
├── create-secrets.sh                      # Creates/updates aex-secrets and mongodb-keyfile
├── namespace.yaml                         # aex namespace
├── base/                                  # Base Kustomize configuration
│   ├── kustomization.yaml                 # Assembles all resources
│   ├── namespace.yaml                     # Namespace definition
│   ├── configmap.yaml                     # Shared env vars and service URLs
│   └── secrets.example.yaml               # Documents aex-secrets keys (not applied)
├── services/                              # AEX Core Services (Go microservices)
│   ├── mongodb/
│   │   ├── statefulset.yaml               # MongoDB StatefulSet with PVC
│   │   └── service.yaml                   # ClusterIP service
│   ├── aex-gateway/
│   │   ├── deployment.yaml                # API gateway deployment
│   │   └── service.yaml
│   ├── aex-work-publisher/
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   ├── aex-bid-gateway/
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   ├── aex-bid-evaluator/
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   ├── aex-contract-engine/
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   ├── aex-provider-registry/
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   ├── aex-trust-broker/
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   ├── aex-identity/
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   ├── aex-settlement/
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   ├── aex-telemetry/
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   └── aex-credentials-provider/
│       ├── deployment.yaml
│       └── service.yaml
├── agents/                                # Code Review Demo Agents (Python)
│   ├── code-reviewer-a/
│   │   ├── deployment.yaml                # QuickReview - Budget reviews
│   │   └── service.yaml
│   ├── code-reviewer-b/
│   │   ├── deployment.yaml                # CodeGuard - Security-focused
│   │   └── service.yaml
│   ├── code-reviewer-c/
│   │   ├── deployment.yaml                # ArchitectAI - Architecture review
│   │   └── service.yaml
│   ├── orchestrator/
│   │   ├── deployment.yaml                # Workflow coordinator
│   │   └── service.yaml
│   ├── payment-devpay/
│   │   ├── deployment.yaml                # General dev payments
│   │   └── service.yaml
│   ├── payment-codeauditpay/
│   │   ├── deployment.yaml                # Code audit payments
│   │   └── service.yaml
│   └── payment-securitypay/
│       ├── deployment.yaml                # Security payments
│       └── service.yaml
├── ui/
│   ├── deployment.yaml                    # NiceGUI real-time dashboard
│   └── service.yaml                       # LoadBalancer service
├── ingress/
│   └── ingress.yaml                       # Nginx Ingress routing
└── overlays/
    ├── dev/
    │   └── kustomization.yaml             # Local dev (1 replica, NodePort, small resources)
    ├── staging/
    │   └── kustomization.yaml             # Staging (2 replicas, moderate resources)
    └── production/
        ├── kustomization.yaml             # Production config
        ├── hpa.yaml                       # HorizontalPodAutoscalers (2-10 replicas)
        ├── pdb.yaml                       # PodDisruptionBudgets
        └── networkpolicy.yaml             # Network policies (default deny + allow rules)
```

## Configuration

### Environment Variables

All shared configuration is managed through the `aex-config` ConfigMap. Service URLs use Kubernetes DNS:

```
http://<service-name>.aex.svc.cluster.local:<port>
```

### Secrets Management

Secrets are never part of the Kustomize output, so every overlay (dev,
staging, production) requires `aex-secrets` and `mongodb-keyfile` to be
created first (see [Set Up Secrets](#2-set-up-secrets)). Because they are not
Kustomize resources, `kubectl apply -k` never overwrites a Secret you created.
**Never commit real secret values.**

For shared or production clusters, manage the Secret with one of:
- **External Secrets Operator**: sync `aex-secrets` from AWS Secrets Manager, GCP Secret Manager, HashiCorp Vault, etc.
- **Sealed Secrets**: encrypt the Secret so it can live in the repository
- **SOPS**: Mozilla SOPS for encrypted secrets in git

Whatever produces it must create a Secret named `aex-secrets` with every key
in the table above.

### Rotate If You Deployed Before This Change

Earlier versions of these manifests included a Secret in the base
kustomization, so every `kubectl apply -k` (including the runs inside
`deploy/aws/deploy-eks.sh` and `deploy/gcp/deploy-gke.sh`) reset `aex-secrets` to public placeholder
values: `JWT_SECRET=REPLACE_ME`, MongoDB `root`/`root`,
`AEX_API_KEY=dev-api-key`. Anyone could forge gateway tokens for any tenant.
If you deployed one of those versions, treat these credentials as compromised:

1. Rotate `JWT_SECRET`, `JWT_SIGNING_KEY` and `WEBHOOK_SECRET` (running
   `./deploy/k8s/create-secrets.sh` replaces the placeholder values with random
   ones; to rotate a real value pass a new one, e.g.
   `JWT_SECRET="$(openssl rand -base64 48)" ./deploy/k8s/create-secrets.sh`).
   Webhook receivers must be given the new `WEBHOOK_SECRET`.
2. Change the MongoDB root password. MongoDB only reads
   `MONGO_INITDB_ROOT_*` when its volume is first initialised, so after
   updating the Secret also change the password inside MongoDB (the script
   prints the `mongosh` command), or recreate the `mongodb` PVC if the data is
   disposable.
3. Revoke and reissue API keys (`AEX_API_KEY`, any tenant keys) and rotate
   `ANTHROPIC_API_KEY` if it was ever stored in a shared cluster.
4. Restart the workloads so they pick up the new values:
   `kubectl rollout restart deployment -n aex && kubectl rollout restart statefulset -n aex`.
5. Review gateway and data access logs for requests made with forged tokens.

### Image Configuration

Base manifests use placeholder image names (`${REGISTRY}/image-name:${TAG}`). Each overlay sets the actual registry and tag via the Kustomize `images` transformer.

To update image tags for a deployment:

```bash
# Update a specific image tag
cd deploy/k8s/overlays/production/
kustomize edit set image YOUR_ACCOUNT.dkr.ecr.YOUR_REGION.amazonaws.com/aex-gateway=YOUR_ACCOUNT.dkr.ecr.YOUR_REGION.amazonaws.com/aex-gateway:v1.2.3
```

## Overlay Details

### Dev Overlay

- 1 replica for all services
- NodePort for UI (port 30502)
- Local image names (`agent-exchange/*:local`)
- 1Gi MongoDB PVC
- Small resource requests/limits

### Staging Overlay

- 2 replicas for core services
- Moderate resources (256Mi-512Mi request, 512Mi-1Gi limit)
- 5Gi MongoDB PVC
- ECR image placeholders

### Production Overlay

- 2-3 base replicas + HPA (scales to 6-10)
- HorizontalPodAutoscalers on key services
- PodDisruptionBudgets (minAvailable: 1)
- NetworkPolicies (default deny + allow rules)
- Node affinity for workload isolation
- Pod anti-affinity for spread across nodes
- TLS on Ingress (cert-manager integration)
- 10Gi MongoDB PVC
- Large resource limits (512Mi-2Gi)

## Monitoring and Troubleshooting

### Health Checks

All services expose `/health` endpoints. Kubernetes uses these for liveness and readiness probes.

```bash
# Check health of a specific service
kubectl exec -n aex deploy/aex-gateway -- wget -qO- http://localhost:8080/health

# Check all pod health
kubectl get pods -n aex -o wide
```

### Logs

```bash
# View logs for a specific service
kubectl logs -n aex deploy/aex-gateway -f

# View logs for all pods with a label
kubectl logs -n aex -l app.kubernetes.io/component=code-review-agent -f

# View previous container logs (if crashed)
kubectl logs -n aex deploy/code-reviewer-a --previous
```

### Common Issues

**Pods stuck in Pending:**
```bash
kubectl describe pod -n aex <pod-name>
# Check for resource constraints or PVC binding issues
```

**Pods in CrashLoopBackOff:**
```bash
kubectl logs -n aex <pod-name> --previous
# Check for missing env vars, connection issues, or startup failures
```

**MongoDB connection failures:**
```bash
# Verify MongoDB is running and ready
kubectl get pods -n aex -l app.kubernetes.io/name=mongodb
kubectl exec -n aex mongodb-0 -- mongosh --eval "db.adminCommand('ping')"
```

**Services not discovering each other:**
```bash
# Test DNS resolution
kubectl exec -n aex deploy/aex-gateway -- nslookup aex-bid-gateway.aex.svc.cluster.local

# Verify ConfigMap values
kubectl get configmap aex-config -n aex -o yaml
```

### Scaling

```bash
# Manual scaling
kubectl scale deployment aex-gateway -n aex --replicas=5

# Check HPA status (production)
kubectl get hpa -n aex

# View HPA details
kubectl describe hpa aex-gateway-hpa -n aex
```

### Resource Usage

```bash
# Pod resource usage (requires metrics-server)
kubectl top pods -n aex

# Node resource usage
kubectl top nodes
```

## Cleanup

```bash
# Delete all resources in the namespace
kubectl delete namespace aex

# Or delete specific overlay
kubectl delete -k deploy/k8s/overlays/dev/
```
