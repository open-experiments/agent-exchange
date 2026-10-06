#!/bin/bash
set -euo pipefail

# Agent Exchange - GKE Deployment Script
# Builds images, pushes to Artifact Registry, and deploys to GKE
#
# Usage:
#   ./deploy-gke.sh --project-id my-project
#   ./deploy-gke.sh --project-id my-project --environment production
#   ./deploy-gke.sh --project-id my-project --skip-build
#   ./deploy-gke.sh --project-id my-project --build-only
#   ./deploy-gke.sh --project-id my-project --clean

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Defaults
PROJECT_ID=""
REGION="us-central1"
CLUSTER_NAME="aex-cluster"
ENVIRONMENT="dev"
NAMESPACE="aex"
BUILD_ONLY=false
SKIP_BUILD=false
CLEAN=false
VERSION=""

usage() {
    cat <<EOF
Agent Exchange - GKE Deployment

Usage: $0 [options]

Options:
  --project-id ID        GCP project ID (required)
  --region REGION        GCP region (default: us-central1)
  --cluster-name NAME    GKE cluster name (default: aex-cluster)
  --environment ENV      Environment: dev, staging, production (default: dev)
  --version VERSION      Image version tag (default: git SHA)
  --build-only           Build and push images only (no deploy)
  --skip-build           Skip image build (deploy existing images)
  --clean                Delete all K8s resources from the cluster
  -h, --help             Show this help message

Examples:
  $0 --project-id my-project
  $0 --project-id my-project --environment staging
  $0 --project-id my-project --skip-build --environment production
  $0 --project-id my-project --clean
EOF
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --project-id)    PROJECT_ID="$2"; shift 2 ;;
        --region)        REGION="$2"; shift 2 ;;
        --cluster-name)  CLUSTER_NAME="$2"; shift 2 ;;
        --environment)   ENVIRONMENT="$2"; shift 2 ;;
        --version)       VERSION="$2"; shift 2 ;;
        --build-only)    BUILD_ONLY=true; shift ;;
        --skip-build)    SKIP_BUILD=true; shift ;;
        --clean)         CLEAN=true; shift ;;
        -h|--help)       usage; exit 0 ;;
        *)               echo "Unknown option: $1"; usage; exit 1 ;;
    esac
done

# Validate
if [[ -z "$PROJECT_ID" ]]; then
    PROJECT_ID="${GCP_PROJECT_ID:-}"
fi
if [[ -z "$PROJECT_ID" ]]; then
    echo "Error: --project-id is required (or set GCP_PROJECT_ID)"
    exit 1
fi

if [[ ! "$ENVIRONMENT" =~ ^(dev|staging|production)$ ]]; then
    echo "Error: --environment must be dev, staging, or production"
    exit 1
fi

REGISTRY="$REGION-docker.pkg.dev/$PROJECT_ID/aex"

if [[ -z "$VERSION" ]]; then
    VERSION=$(git -C "$PROJECT_ROOT" rev-parse --short HEAD 2>/dev/null || echo "latest")
fi

# ============================================================
# Validation
# ============================================================

validate_prerequisites() {
    echo "Validating prerequisites..."

    if ! command -v gcloud &> /dev/null; then
        echo "Error: gcloud CLI is not installed"
        exit 1
    fi

    if ! command -v kubectl &> /dev/null; then
        echo "Error: kubectl is not installed"
        exit 1
    fi

    echo "Prerequisites OK"
}

get_cluster_credentials() {
    echo "Getting GKE cluster credentials..."

    gcloud container clusters get-credentials "$CLUSTER_NAME" \
        --region="$REGION" \
        --project="$PROJECT_ID"

    echo "Connected to cluster: $(kubectl config current-context)"

    # Verify namespace exists
    if ! kubectl get namespace "$NAMESPACE" &> /dev/null; then
        echo "Creating namespace '$NAMESPACE'..."
        kubectl create namespace "$NAMESPACE"
    fi
}

# ============================================================
# Image Building
# ============================================================

build_and_push_images() {
    echo ""
    echo "Building and pushing images to Artifact Registry..."
    echo "  Registry: $REGISTRY"
    echo "  Version:  $VERSION"
    echo ""

    # Configure Docker for Artifact Registry
    gcloud auth configure-docker "$REGION-docker.pkg.dev" --quiet

    # Ensure Artifact Registry exists
    if ! gcloud artifacts repositories describe aex --location="$REGION" --project="$PROJECT_ID" &> /dev/null; then
        echo "Creating Artifact Registry repository 'aex'..."
        gcloud artifacts repositories create aex \
            --repository-format=docker \
            --location="$REGION" \
            --project="$PROJECT_ID" \
            --description="Agent Exchange Docker images"
    fi

    # AEX Core services
    local core_services=(
        "aex-gateway"
        "aex-work-publisher"
        "aex-bid-gateway"
        "aex-bid-evaluator"
        "aex-contract-engine"
        "aex-provider-registry"
        "aex-trust-broker"
        "aex-identity"
        "aex-settlement"
        "aex-telemetry"
        "aex-credentials-provider"
    )

    for service in "${core_services[@]}"; do
        echo "Building $service..."
        docker build \
            -f "$PROJECT_ROOT/src/$service/Dockerfile" \
            -t "$REGISTRY/$service:$VERSION" \
            -t "$REGISTRY/$service:latest" \
            "$PROJECT_ROOT/src/"

        echo "Pushing $service..."
        docker push "$REGISTRY/$service:$VERSION"
        docker push "$REGISTRY/$service:latest"
    done

    # Code Review demo agents
    local demo_agents=(
        "code-reviewer-a"
        "code-reviewer-b"
        "code-reviewer-c"
        "orchestrator"
        "payment-devpay"
        "payment-codeauditpay"
        "payment-securitypay"
    )

    for agent in "${demo_agents[@]}"; do
        echo "Building $agent..."
        docker build \
            -f "$PROJECT_ROOT/demo/code_review/agents/Dockerfile" \
            --build-arg "AGENT_DIR=$agent" \
            -t "$REGISTRY/$agent:$VERSION" \
            -t "$REGISTRY/$agent:latest" \
            "$PROJECT_ROOT/demo/code_review/agents/"

        echo "Pushing $agent..."
        docker push "$REGISTRY/$agent:$VERSION"
        docker push "$REGISTRY/$agent:latest"
    done

    # Demo UI
    echo "Building demo-ui-nicegui..."
    docker build \
        -f "$PROJECT_ROOT/demo/code_review/ui/Dockerfile" \
        -t "$REGISTRY/demo-ui-nicegui:$VERSION" \
        -t "$REGISTRY/demo-ui-nicegui:latest" \
        "$PROJECT_ROOT/demo/code_review/ui/"

    echo "Pushing demo-ui-nicegui..."
    docker push "$REGISTRY/demo-ui-nicegui:$VERSION"
    docker push "$REGISTRY/demo-ui-nicegui:latest"

    echo ""
    echo "All images built and pushed successfully"
}

# ============================================================
# K8s Deployment
# ============================================================

deploy_manifests() {
    echo ""
    echo "Deploying K8s manifests..."
    echo "  Environment: $ENVIRONMENT"
    echo "  Namespace:   $NAMESPACE"
    echo "  Version:     $VERSION"
    echo ""

    local kustomize_dir="$PROJECT_ROOT/deploy/k8s/base"

    # Check for environment-specific overlay
    local overlay_dir="$PROJECT_ROOT/deploy/k8s/overlays/$ENVIRONMENT"
    if [[ -d "$overlay_dir" ]] && [[ -f "$overlay_dir/kustomization.yaml" ]]; then
        echo "Using overlay: $overlay_dir"
        kustomize_dir="$overlay_dir"
    else
        echo "No overlay found for '$ENVIRONMENT', using base manifests"
    fi

    # The manifests do not render aex-secrets; make sure it exists with every
    # key they reference. Values already in the cluster are kept.
    bash "$PROJECT_ROOT/deploy/k8s/create-secrets.sh" --namespace "$NAMESPACE"

    # Apply manifests with Kustomize, setting the image registry
    echo "Applying manifests with image overrides..."

    # Build the kustomize command with all image overrides
    local kustomize_cmd="kubectl kustomize $kustomize_dir"

    # Apply with image substitution using sed for the registry/tag replacement
    $kustomize_cmd | \
        sed "s|\${REGISTRY}|$REGISTRY|g" | \
        sed "s|\${TAG}|$VERSION|g" | \
        kubectl apply -n "$NAMESPACE" -f -

    echo "Manifests applied"
}

wait_for_pods() {
    echo ""
    echo "Waiting for pods to be ready..."

    local timeout=300
    local start_time=$(date +%s)

    while true; do
        local not_ready
        not_ready=$(kubectl get pods -n "$NAMESPACE" --no-headers 2>/dev/null | \
            grep -v "Running\|Completed" | wc -l | tr -d ' ')

        if [[ "$not_ready" -eq 0 ]]; then
            local total
            total=$(kubectl get pods -n "$NAMESPACE" --no-headers 2>/dev/null | wc -l | tr -d ' ')
            if [[ "$total" -gt 0 ]]; then
                echo "All $total pods are ready!"
                break
            fi
        fi

        local elapsed=$(( $(date +%s) - start_time ))
        if [[ "$elapsed" -ge "$timeout" ]]; then
            echo "Warning: Timeout waiting for pods after ${timeout}s"
            echo "Current pod status:"
            kubectl get pods -n "$NAMESPACE"
            break
        fi

        echo "  Waiting... ($not_ready pods not ready, ${elapsed}s elapsed)"
        sleep 10
    done
}

get_ingress_ip() {
    echo ""
    echo "Getting Ingress external IP..."

    local ingress_ip=""
    local attempts=0
    local max_attempts=30

    while [[ -z "$ingress_ip" || "$ingress_ip" == "<pending>" ]] && [[ $attempts -lt $max_attempts ]]; do
        ingress_ip=$(kubectl get svc ingress-nginx-controller \
            -n ingress-nginx \
            -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo "")

        if [[ -z "$ingress_ip" ]]; then
            ((attempts++))
            echo "  Waiting for external IP... (attempt $attempts/$max_attempts)"
            sleep 10
        fi
    done

    if [[ -n "$ingress_ip" ]]; then
        echo "Ingress IP: $ingress_ip"
    else
        echo "Warning: Could not get Ingress IP. Check ingress-nginx status:"
        echo "  kubectl get svc -n ingress-nginx"
    fi

    echo "$ingress_ip"
}

print_service_urls() {
    local ingress_ip="$1"

    echo ""
    echo "================================================================"
    echo "  Deployment Complete"
    echo "================================================================"
    echo ""
    echo "Environment: $ENVIRONMENT"
    echo "Version:     $VERSION"
    echo "Namespace:   $NAMESPACE"
    echo ""

    if [[ -n "$ingress_ip" ]]; then
        echo "Service URLs (via Ingress at $ingress_ip):"
        echo ""
        echo "  AEX Core:"
        echo "    Gateway:             http://$ingress_ip/api"
        echo ""
        echo "  Code Review Demo:"
        echo "    Demo UI:             http://$ingress_ip/demo"
        echo "    Code Reviewer A:     http://$ingress_ip/agents/code-reviewer-a"
        echo "    Code Reviewer B:     http://$ingress_ip/agents/code-reviewer-b"
        echo "    Code Reviewer C:     http://$ingress_ip/agents/code-reviewer-c"
        echo "    Orchestrator:        http://$ingress_ip/agents/orchestrator"
        echo ""
        echo "  Payment Agents:"
        echo "    DevPay:              http://$ingress_ip/payments/devpay"
        echo "    CodeAuditPay:        http://$ingress_ip/payments/codeauditpay"
        echo "    SecurityPay:         http://$ingress_ip/payments/securitypay"
    else
        echo "Internal Service URLs (ClusterIP):"
        kubectl get svc -n "$NAMESPACE" -o wide
    fi

    echo ""
    echo "Pod status:"
    kubectl get pods -n "$NAMESPACE" -o wide
    echo ""
    echo "Useful commands:"
    echo "  kubectl get pods -n $NAMESPACE"
    echo "  kubectl logs -n $NAMESPACE deployment/aex-gateway"
    echo "  kubectl port-forward -n $NAMESPACE svc/aex-gateway 8080:8080"
    echo "  kubectl port-forward -n $NAMESPACE svc/demo-ui-nicegui 8502:8502"
    echo ""
}

# ============================================================
# Cleanup
# ============================================================

clean_resources() {
    echo ""
    echo "Cleaning K8s resources in namespace '$NAMESPACE'..."

    read -p "This will delete all AEX resources. Continue? (y/N): " confirm
    if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
        echo "Aborted."
        exit 0
    fi

    # Delete namespace (cascades all resources)
    kubectl delete namespace "$NAMESPACE" --wait=true 2>/dev/null || true

    echo "K8s resources deleted"
    echo ""
    echo "Note: Ingress controller and other cluster-level resources are preserved."
    echo "To fully remove the cluster, use: ./gke-cluster.sh --project-id $PROJECT_ID --delete"
}

# ============================================================
# Main
# ============================================================

echo ""
echo "================================================================"
echo "  Agent Exchange - GKE Deployment"
echo "================================================================"
echo ""
echo "  Project:     $PROJECT_ID"
echo "  Cluster:     $CLUSTER_NAME"
echo "  Region:      $REGION"
echo "  Environment: $ENVIRONMENT"
echo "  Version:     $VERSION"
echo ""

validate_prerequisites

if [[ "$CLEAN" == "true" ]]; then
    get_cluster_credentials
    clean_resources
    exit 0
fi

get_cluster_credentials

if [[ "$SKIP_BUILD" != "true" ]]; then
    build_and_push_images
fi

if [[ "$BUILD_ONLY" == "true" ]]; then
    echo ""
    echo "Build complete. Skipping deployment (--build-only)."
    exit 0
fi

deploy_manifests
wait_for_pods

INGRESS_IP=$(get_ingress_ip)
print_service_urls "$INGRESS_IP"
