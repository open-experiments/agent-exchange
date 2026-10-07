#!/bin/bash
# Deploy AEX to AWS EKS using CloudFormation and Kubernetes manifests
# Usage: ./deploy-eks.sh [--region <region>] [--env <environment>] [--cluster <name>] [--clean]

set -euo pipefail

# ============================================================
# Configuration
# ============================================================

REGION="${AWS_REGION:-us-east-1}"
ENVIRONMENT_NAME="${ENVIRONMENT_NAME:-aex}"
ENVIRONMENT="${ENVIRONMENT:-dev}"
CLUSTER_NAME="${CLUSTER_NAME:-aex-eks}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
K8S_DIR="$PROJECT_ROOT/deploy/k8s"
CLEAN=false

# ============================================================
# Argument parsing
# ============================================================

while [[ $# -gt 0 ]]; do
    case $1 in
        --region)
            REGION="$2"
            shift 2
            ;;
        --env)
            ENVIRONMENT="$2"
            shift 2
            ;;
        --cluster)
            CLUSTER_NAME="$2"
            shift 2
            ;;
        --name)
            ENVIRONMENT_NAME="$2"
            shift 2
            ;;
        --clean)
            CLEAN=true
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  --region <region>    AWS region (default: us-east-1)"
            echo "  --env <environment>  Environment: dev, staging, production (default: dev)"
            echo "  --cluster <name>     EKS cluster name (default: aex-eks)"
            echo "  --name <name>        Environment name prefix (default: aex)"
            echo "  --clean              Teardown EKS resources"
            echo "  -h, --help           Show this help"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

echo "========================================="
echo "  AEX EKS Deployment"
echo "========================================="
echo "Region:      $REGION"
echo "Environment: $ENVIRONMENT"
echo "Cluster:     $CLUSTER_NAME"
echo "Env Name:    $ENVIRONMENT_NAME"
echo ""

# ============================================================
# Teardown mode
# ============================================================

if [ "$CLEAN" = true ]; then
    echo "Tearing down EKS resources..."
    echo ""

    # Check if cluster exists and kubeconfig can be configured
    if aws eks describe-cluster --name "$CLUSTER_NAME" --region "$REGION" &>/dev/null; then
        echo "Configuring kubectl..."
        aws eks update-kubeconfig --name "$CLUSTER_NAME" --region "$REGION" 2>/dev/null || true

        echo "Deleting Kubernetes namespace 'aex'..."
        kubectl delete namespace aex --ignore-not-found --timeout=120s 2>/dev/null || true

        echo "Uninstalling Helm charts..."
        helm uninstall aws-load-balancer-controller -n kube-system 2>/dev/null || true
        helm uninstall ingress-nginx -n ingress-nginx 2>/dev/null || true
        helm uninstall external-secrets -n external-secrets 2>/dev/null || true
        helm uninstall metrics-server -n kube-system 2>/dev/null || true
        kubectl delete namespace ingress-nginx --ignore-not-found 2>/dev/null || true
        kubectl delete namespace external-secrets --ignore-not-found 2>/dev/null || true
    fi

    echo "Deleting EKS CloudFormation stack..."
    aws cloudformation delete-stack \
        --stack-name "${ENVIRONMENT_NAME}-eks-cluster" \
        --region "$REGION" 2>/dev/null || true

    echo "Waiting for EKS stack deletion..."
    aws cloudformation wait stack-delete-complete \
        --stack-name "${ENVIRONMENT_NAME}-eks-cluster" \
        --region "$REGION" 2>/dev/null || true

    echo ""
    echo "EKS teardown complete."
    echo "Note: Infrastructure stack (VPC, ECR, secrets) was NOT deleted."
    echo "To delete everything: hack/deploy/teardown-eks.sh"
    exit 0
fi

# ============================================================
# Step 0: Validate prerequisites
# ============================================================

echo "Step 0: Validating prerequisites..."

if ! command -v aws &>/dev/null; then
    echo "Error: AWS CLI is not installed."
    exit 1
fi

if ! command -v kubectl &>/dev/null; then
    echo "Error: kubectl is not installed."
    echo "Install: https://kubernetes.io/docs/tasks/tools/"
    exit 1
fi

if ! command -v helm &>/dev/null; then
    echo "Error: Helm is not installed."
    echo "Install: https://helm.sh/docs/intro/install/"
    exit 1
fi

AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text 2>/dev/null)
if [ -z "$AWS_ACCOUNT_ID" ]; then
    echo "Error: Unable to get AWS account ID. Please configure AWS credentials."
    exit 1
fi
echo "AWS Account ID: $AWS_ACCOUNT_ID"

# ============================================================
# Step 1: Deploy infrastructure stack (VPC, ECR, Secrets)
# ============================================================

echo ""
echo "Step 1: Deploying infrastructure stack (VPC, ECR, Secrets)..."
aws cloudformation deploy \
    --template-file "$SCRIPT_DIR/infrastructure.yaml" \
    --stack-name "${ENVIRONMENT_NAME}-infrastructure" \
    --parameter-overrides EnvironmentName="$ENVIRONMENT_NAME" \
    --capabilities CAPABILITY_NAMED_IAM \
    --region "$REGION" \
    --no-fail-on-empty-changeset

echo "Infrastructure stack deployed."

# ============================================================
# Step 2: Deploy EKS cluster stack
# ============================================================

echo ""
echo "Step 2: Deploying EKS cluster stack..."

# Determine node sizing based on environment
case "$ENVIRONMENT" in
    production)
        NODE_INSTANCE_TYPE="m5.large"
        MIN_SIZE=3
        MAX_SIZE=10
        DESIRED_SIZE=3
        ;;
    staging)
        NODE_INSTANCE_TYPE="t3.large"
        MIN_SIZE=2
        MAX_SIZE=5
        DESIRED_SIZE=2
        ;;
    *)
        NODE_INSTANCE_TYPE="t3.medium"
        MIN_SIZE=2
        MAX_SIZE=5
        DESIRED_SIZE=2
        ;;
esac

aws cloudformation deploy \
    --template-file "$SCRIPT_DIR/eks-cluster.yaml" \
    --stack-name "${ENVIRONMENT_NAME}-eks-cluster" \
    --parameter-overrides \
        EnvironmentName="$ENVIRONMENT_NAME" \
        Environment="$ENVIRONMENT" \
        ClusterName="$CLUSTER_NAME" \
        NodeInstanceType="$NODE_INSTANCE_TYPE" \
        MinSize="$MIN_SIZE" \
        MaxSize="$MAX_SIZE" \
        DesiredSize="$DESIRED_SIZE" \
    --capabilities CAPABILITY_NAMED_IAM \
    --region "$REGION" \
    --no-fail-on-empty-changeset

echo "EKS cluster stack deployed."

# ============================================================
# Step 3: Configure kubectl
# ============================================================

echo ""
echo "Step 3: Configuring kubectl context..."
aws eks update-kubeconfig \
    --name "$CLUSTER_NAME" \
    --region "$REGION" \
    --alias "$CLUSTER_NAME"

echo "kubectl context configured: $CLUSTER_NAME"
kubectl cluster-info

# ============================================================
# Step 4: Install Helm add-ons
# ============================================================

echo ""
echo "Step 4: Installing Helm add-ons..."

# Get the LB controller role ARN from CloudFormation
LB_CONTROLLER_ROLE_ARN=$(aws cloudformation describe-stacks \
    --stack-name "${ENVIRONMENT_NAME}-eks-cluster" \
    --query "Stacks[0].Outputs[?OutputKey=='AWSLoadBalancerControllerRoleArn'].OutputValue" \
    --output text \
    --region "$REGION")

# AWS Load Balancer Controller
echo "Installing AWS Load Balancer Controller..."
helm repo add eks https://aws.github.io/eks-charts 2>/dev/null || true
helm repo update eks
helm upgrade --install aws-load-balancer-controller eks/aws-load-balancer-controller \
    --namespace kube-system \
    --set clusterName="$CLUSTER_NAME" \
    --set serviceAccount.create=true \
    --set serviceAccount.name=aws-load-balancer-controller \
    --set serviceAccount.annotations."eks\.amazonaws\.com/role-arn"="$LB_CONTROLLER_ROLE_ARN" \
    --set region="$REGION" \
    --set vpcId="$(aws cloudformation describe-stacks \
        --stack-name "${ENVIRONMENT_NAME}-infrastructure" \
        --query "Stacks[0].Outputs[?OutputKey=='VPCId'].OutputValue" \
        --output text --region "$REGION")" \
    --wait

# Nginx Ingress Controller
echo "Installing Nginx Ingress Controller..."
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx 2>/dev/null || true
helm repo update ingress-nginx
kubectl create namespace ingress-nginx 2>/dev/null || true
helm upgrade --install ingress-nginx ingress-nginx/ingress-nginx \
    --namespace ingress-nginx \
    --set controller.service.type=LoadBalancer \
    --set controller.service.annotations."service\.beta\.kubernetes\.io/aws-load-balancer-type"=nlb \
    --set controller.service.annotations."service\.beta\.kubernetes\.io/aws-load-balancer-scheme"=internet-facing \
    --wait

# External Secrets Operator
echo "Installing External Secrets Operator..."
helm repo add external-secrets https://charts.external-secrets.io 2>/dev/null || true
helm repo update external-secrets
kubectl create namespace external-secrets 2>/dev/null || true
helm upgrade --install external-secrets external-secrets/external-secrets \
    --namespace external-secrets \
    --set installCRDs=true \
    --wait

# Metrics Server (for HPA)
echo "Installing Metrics Server..."
helm repo add metrics-server https://kubernetes-sigs.github.io/metrics-server/ 2>/dev/null || true
helm repo update metrics-server
helm upgrade --install metrics-server metrics-server/metrics-server \
    --namespace kube-system \
    --set args[0]="--kubelet-preferred-address-types=InternalIP" \
    --wait

echo "Helm add-ons installed."

# ============================================================
# Step 5: Create Kubernetes namespace and secrets
# ============================================================

echo ""
echo "Step 5: Creating Kubernetes namespace and secrets..."

kubectl create namespace aex 2>/dev/null || true

# Get Pod role ARN for service account annotation
POD_ROLE_ARN=$(aws cloudformation describe-stacks \
    --stack-name "${ENVIRONMENT_NAME}-eks-cluster" \
    --query "Stacks[0].Outputs[?OutputKey=='EKSPodRoleArn'].OutputValue" \
    --output text \
    --region "$REGION")

# Create annotated service account for IRSA
kubectl apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: aex-service-account
  namespace: aex
  annotations:
    eks.amazonaws.com/role-arn: "$POD_ROLE_ARN"
  labels:
    app.kubernetes.io/part-of: agent-exchange
EOF

# Create secrets from AWS Secrets Manager
echo "Syncing secrets from AWS Secrets Manager..."

# read_aws_secret <secret-id> <json-field>
# Prints the field of a JSON secret, or nothing when the secret is missing
# or still holds the "placeholder-update-me" value infrastructure.yaml seeds.
read_aws_secret() {
    local raw value
    raw=$(aws secretsmanager get-secret-value \
        --secret-id "$1" \
        --query 'SecretString' --output text \
        --region "$REGION" 2>/dev/null) || return 0
    value=$(printf '%s' "$raw" | python3 -c "import sys,json; print(json.load(sys.stdin).get('$2',''))" 2>/dev/null) || return 0
    case "$value" in
        placeholder*) return 0 ;;
    esac
    printf '%s' "$value"
}

# Values found in Secrets Manager (or already exported by the caller) are
# passed through; create-secrets.sh keeps values already stored in the
# cluster and generates random JWT/webhook/MongoDB secrets for anything
# missing. It never falls back to a fixed default.
ANTHROPIC_API_KEY="${ANTHROPIC_API_KEY:-$(read_aws_secret "${ENVIRONMENT_NAME}/anthropic-api-key" api_key)}"
MONGO_URI="${MONGO_URI:-$(read_aws_secret "${ENVIRONMENT_NAME}/mongo-uri" uri)}"
JWT_SECRET="${JWT_SECRET:-$(read_aws_secret "${ENVIRONMENT_NAME}/jwt-signing-key" key)}"
JWT_SIGNING_KEY="${JWT_SIGNING_KEY:-$JWT_SECRET}"
export ANTHROPIC_API_KEY MONGO_URI JWT_SECRET JWT_SIGNING_KEY

bash "$K8S_DIR/create-secrets.sh" --namespace aex

echo "Namespace and secrets configured."

# ============================================================
# Step 6: Build and push Docker images to ECR
# ============================================================

echo ""
echo "Step 6: Building and pushing Docker images to ECR..."

ECR_REGISTRY="$AWS_ACCOUNT_ID.dkr.ecr.$REGION.amazonaws.com"
aws ecr get-login-password --region "$REGION" | docker login --username AWS --password-stdin "$ECR_REGISTRY"

COMMIT_HASH=$(git -C "$PROJECT_ROOT" rev-parse --short HEAD 2>/dev/null || echo "latest")
IMAGE_TAG="$COMMIT_HASH"
echo "Image tag: $IMAGE_TAG"

# Setup buildx
docker buildx create --name aex-eks-builder --use 2>/dev/null || docker buildx use aex-eks-builder 2>/dev/null || true

# AEX core services
SERVICES=(
    "aex-gateway"
    "aex-provider-registry"
    "aex-work-publisher"
    "aex-bid-gateway"
    "aex-bid-evaluator"
    "aex-contract-engine"
    "aex-settlement"
    "aex-trust-broker"
    "aex-identity"
    "aex-telemetry"
    "aex-credentials-provider"
)

cd "$PROJECT_ROOT"

for service in "${SERVICES[@]}"; do
    echo "Building $service..."
    docker buildx build --platform linux/amd64 \
        -t "$ECR_REGISTRY/$ENVIRONMENT_NAME/$service:$IMAGE_TAG" \
        -t "$ECR_REGISTRY/$ENVIRONMENT_NAME/$service:latest" \
        -f "src/$service/Dockerfile" src/ \
        --push
done

# Code review demo agents
DEMO_AGENTS=("code-reviewer-a" "code-reviewer-b" "code-reviewer-c" "orchestrator")

for agent in "${DEMO_AGENTS[@]}"; do
    AGENT_DIR="demo/code_review/agents/$agent"
    if [ -d "$AGENT_DIR" ]; then
        echo "Building $agent..."
        docker buildx build --platform linux/amd64 \
            -t "$ECR_REGISTRY/$ENVIRONMENT_NAME/$agent:$IMAGE_TAG" \
            -t "$ECR_REGISTRY/$ENVIRONMENT_NAME/$agent:latest" \
            --build-arg AGENT_DIR="$agent" \
            -f demo/code_review/agents/Dockerfile demo/code_review/agents/ \
            --push
    fi
done

# Payment agents
PAYMENT_AGENTS=("payment-devpay" "payment-codeauditpay" "payment-securitypay")

for agent in "${PAYMENT_AGENTS[@]}"; do
    AGENT_DIR="demo/code_review/agents/$agent"
    if [ -d "$AGENT_DIR" ]; then
        echo "Building $agent..."
        docker buildx build --platform linux/amd64 \
            -t "$ECR_REGISTRY/$ENVIRONMENT_NAME/$agent:$IMAGE_TAG" \
            -t "$ECR_REGISTRY/$ENVIRONMENT_NAME/$agent:latest" \
            --build-arg AGENT_DIR="$agent" \
            -f demo/code_review/agents/Dockerfile demo/code_review/agents/ \
            --push
    fi
done

# Demo UI (NiceGUI)
if [ -d "demo/code_review/ui" ]; then
    echo "Building demo-ui-nicegui..."
    docker buildx build --platform linux/amd64 \
        -t "$ECR_REGISTRY/$ENVIRONMENT_NAME/demo-ui-nicegui:$IMAGE_TAG" \
        -t "$ECR_REGISTRY/$ENVIRONMENT_NAME/demo-ui-nicegui:latest" \
        -f demo/code_review/ui/Dockerfile demo/code_review/ui/ \
        --push
fi

echo "All images built and pushed."

# ============================================================
# Step 7: Apply Kubernetes manifests with Kustomize
# ============================================================

echo ""
echo "Step 7: Applying Kubernetes manifests..."

# Check if kustomize overlay exists for this environment
OVERLAY_DIR="$K8S_DIR/overlays/$ENVIRONMENT"
if [ -d "$OVERLAY_DIR" ]; then
    echo "Using kustomize overlay: $ENVIRONMENT"
    kubectl apply -k "$OVERLAY_DIR"
elif [ -d "$K8S_DIR/base" ]; then
    echo "Using kustomize base (no overlay for $ENVIRONMENT)"
    kubectl apply -k "$K8S_DIR/base"
else
    echo "Applying individual manifests from $K8S_DIR..."
    kubectl apply -f "$K8S_DIR/namespace.yaml"
    for f in "$K8S_DIR"/*.yaml; do
        kubectl apply -f "$f" 2>/dev/null || true
    done
fi

# Update image references to ECR
echo "Patching deployments with ECR images..."

ALL_DEPLOYMENTS=(
    "aex-gateway"
    "aex-provider-registry"
    "aex-work-publisher"
    "aex-bid-gateway"
    "aex-bid-evaluator"
    "aex-contract-engine"
    "aex-settlement"
    "aex-trust-broker"
    "aex-identity"
    "aex-telemetry"
    "code-reviewer-a"
    "code-reviewer-b"
    "code-reviewer-c"
    "orchestrator"
    "payment-devpay"
    "payment-codeauditpay"
    "payment-securitypay"
    "demo-ui-nicegui"
)

for deploy in "${ALL_DEPLOYMENTS[@]}"; do
    if kubectl get deployment "$deploy" -n aex &>/dev/null; then
        echo "  Patching $deploy with ECR image..."
        kubectl set image deployment/"$deploy" \
            "$deploy=$ECR_REGISTRY/$ENVIRONMENT_NAME/$deploy:$IMAGE_TAG" \
            -n aex 2>/dev/null || true
    fi
done

# Patch service account on all deployments
for deploy in "${ALL_DEPLOYMENTS[@]}"; do
    if kubectl get deployment "$deploy" -n aex &>/dev/null; then
        kubectl patch deployment "$deploy" -n aex \
            --type=json \
            -p='[{"op": "add", "path": "/spec/template/spec/serviceAccountName", "value": "aex-service-account"}]' \
            2>/dev/null || true
    fi
done

echo "Kubernetes manifests applied."

# ============================================================
# Step 8: Wait for deployments
# ============================================================

echo ""
echo "Step 8: Waiting for deployments to become ready..."

TIMEOUT=300
for deploy in "${ALL_DEPLOYMENTS[@]}"; do
    if kubectl get deployment "$deploy" -n aex &>/dev/null; then
        echo "  Waiting for $deploy..."
        kubectl rollout status deployment/"$deploy" -n aex --timeout="${TIMEOUT}s" 2>/dev/null || {
            echo "  Warning: $deploy did not become ready within ${TIMEOUT}s"
        }
    fi
done

echo "Deployment rollout complete."

# ============================================================
# Step 9: Output endpoints
# ============================================================

echo ""
echo "========================================="
echo "  EKS Deployment Complete!"
echo "========================================="
echo ""

# Get ingress/LB endpoints
echo "Service Endpoints:"
echo ""

# Check for ingress
INGRESS_HOST=$(kubectl get ingress -n aex -o jsonpath='{.items[0].status.loadBalancer.ingress[0].hostname}' 2>/dev/null || echo "")
if [ -n "$INGRESS_HOST" ]; then
    echo "  Ingress URL: http://$INGRESS_HOST"
fi

# Check for nginx-ingress LB
NGINX_LB=$(kubectl get svc -n ingress-nginx ingress-nginx-controller -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null || echo "")
if [ -n "$NGINX_LB" ]; then
    echo "  Nginx Ingress LB: http://$NGINX_LB"
fi

echo ""
echo "Cluster:     $CLUSTER_NAME"
echo "Region:      $REGION"
echo "Namespace:   aex"
echo "Image Tag:   $IMAGE_TAG"
echo ""

echo "Useful commands:"
echo ""
echo "  # View all pods"
echo "  kubectl get pods -n aex"
echo ""
echo "  # View services"
echo "  kubectl get svc -n aex"
echo ""
echo "  # View logs for a service"
echo "  kubectl logs -f deployment/aex-gateway -n aex"
echo ""
echo "  # Port forward gateway locally"
echo "  kubectl port-forward svc/aex-gateway 8080:8080 -n aex"
echo ""
echo "  # Port forward demo UI locally"
echo "  kubectl port-forward svc/demo-ui-nicegui 8502:8502 -n aex"
echo ""
echo "  # Scale a deployment"
echo "  kubectl scale deployment/aex-gateway --replicas=3 -n aex"
echo ""

echo "Next Steps:"
echo ""
echo "1. Update secrets with actual values:"
echo "   aws secretsmanager update-secret \\"
echo "     --secret-id ${ENVIRONMENT_NAME}/anthropic-api-key \\"
echo "     --secret-string '{\"api_key\":\"sk-ant-...\"}' \\"
echo "     --region $REGION"
echo ""
echo "2. Re-run secret sync:"
echo "   $0 --region $REGION --env $ENVIRONMENT --cluster $CLUSTER_NAME"
echo ""
echo "3. Clean up:"
echo "   $0 --clean --region $REGION --cluster $CLUSTER_NAME"
echo ""
