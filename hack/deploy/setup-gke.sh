#!/bin/bash
set -euo pipefail

# Agent Exchange - GKE Setup Helper Script
# End-to-end setup: prerequisites check, GKE cluster creation, Helm charts, and configuration
#
# Usage:
#   GCP_PROJECT_ID=my-project ./setup-gke.sh
#   GCP_PROJECT_ID=my-project GCP_REGION=us-east1 ./setup-gke.sh
#   GCP_PROJECT_ID=my-project ./setup-gke.sh --mode standard

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Configuration
PROJECT="${GCP_PROJECT_ID:-}"
REGION="${GCP_REGION:-us-central1}"
CLUSTER_NAME="${GKE_CLUSTER_NAME:-aex-cluster}"
MODE="autopilot"
NAMESPACE="aex"
DRY_RUN=false

usage() {
    cat <<EOF
Agent Exchange - GKE Setup Helper

Usage: $0 [options]

Options:
  --mode MODE     Cluster mode: autopilot or standard (default: autopilot)
  --dry-run       Show what would be done without making changes
  -h, --help      Show this help message

Environment variables:
  GCP_PROJECT_ID       Google Cloud project ID (required)
  GCP_REGION           Google Cloud region (default: us-central1)
  GKE_CLUSTER_NAME     GKE cluster name (default: aex-cluster)

This script will:
  1. Check prerequisites (gcloud, kubectl, helm)
  2. Enable required GCP APIs
  3. Create Artifact Registry (reuses existing if present)
  4. Create GKE cluster (Autopilot or Standard mode)
  5. Configure kubectl
  6. Install Helm charts:
     - Nginx Ingress Controller
     - cert-manager (for TLS)
     - External Secrets Operator
     - metrics-server (Standard mode only)
  7. Set up Workload Identity
  8. Create namespace and secrets
  9. Output cluster info and next steps
EOF
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --mode)      MODE="$2"; shift 2 ;;
        --dry-run)   DRY_RUN=true; shift ;;
        -h|--help)   usage; exit 0 ;;
        *)           echo "Unknown option: $1"; usage; exit 1 ;;
    esac
done

# Validate
if [[ -z "$PROJECT" ]]; then
    echo "Error: GCP_PROJECT_ID environment variable is required"
    echo "  export GCP_PROJECT_ID=your-project-id"
    exit 1
fi

# ============================================================
# Step 1: Check Prerequisites
# ============================================================

check_prerequisites() {
    echo "================================================================"
    echo "  Step 1: Checking Prerequisites"
    echo "================================================================"
    echo ""

    local errors=0

    # gcloud
    echo -n "  gcloud CLI... "
    if command -v gcloud &> /dev/null; then
        local gcloud_ver
        gcloud_ver=$(gcloud version --format='value(Google Cloud SDK)' 2>/dev/null | head -1)
        echo "OK (v$gcloud_ver)"
    else
        echo "NOT FOUND"
        echo "    Install: https://cloud.google.com/sdk/docs/install"
        ((errors++))
    fi

    # kubectl
    echo -n "  kubectl... "
    if command -v kubectl &> /dev/null; then
        local kubectl_ver
        kubectl_ver=$(kubectl version --client --short 2>/dev/null | head -1 || kubectl version --client -o json 2>/dev/null | grep -o '"gitVersion": "[^"]*"' | head -1 || echo "installed")
        echo "OK ($kubectl_ver)"
    else
        echo "NOT FOUND"
        echo "    Install: gcloud components install kubectl"
        echo "    Or: https://kubernetes.io/docs/tasks/tools/"
        ((errors++))
    fi

    # helm
    echo -n "  helm... "
    if command -v helm &> /dev/null; then
        local helm_ver
        helm_ver=$(helm version --short 2>/dev/null | head -1)
        echo "OK ($helm_ver)"
    else
        echo "NOT FOUND"
        echo "    Install: https://helm.sh/docs/intro/install/"
        echo "    Or: curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash"
        ((errors++))
    fi

    # docker (for building images)
    echo -n "  docker... "
    if command -v docker &> /dev/null; then
        local docker_ver
        docker_ver=$(docker --version 2>/dev/null | head -1)
        echo "OK ($docker_ver)"
    else
        echo "NOT FOUND (optional, needed for local builds)"
    fi

    # gcloud auth
    echo -n "  gcloud authentication... "
    if gcloud auth print-identity-token &> /dev/null; then
        local account
        account=$(gcloud config get-value account 2>/dev/null)
        echo "OK ($account)"
    else
        echo "NOT AUTHENTICATED"
        echo "    Run: gcloud auth login"
        ((errors++))
    fi

    # Project access
    echo -n "  GCP project ($PROJECT)... "
    if gcloud projects describe "$PROJECT" &> /dev/null; then
        echo "OK"
    else
        echo "NOT ACCESSIBLE"
        ((errors++))
    fi

    # Billing
    echo -n "  Billing enabled... "
    local billing
    billing=$(gcloud billing projects describe "$PROJECT" --format='value(billingEnabled)' 2>/dev/null || echo "false")
    if [[ "$billing" == "True" ]]; then
        echo "OK"
    else
        echo "WARNING (billing may not be enabled)"
    fi

    echo ""
    if [[ $errors -gt 0 ]]; then
        echo "FAILED: $errors prerequisite(s) missing. Please fix and retry."
        exit 1
    fi
    echo "All prerequisites satisfied."
}

# ============================================================
# Step 2: Enable GCP APIs
# ============================================================

enable_apis() {
    echo ""
    echo "================================================================"
    echo "  Step 2: Enabling GCP APIs"
    echo "================================================================"
    echo ""

    local apis=(
        "container.googleapis.com:Kubernetes Engine"
        "compute.googleapis.com:Compute Engine"
        "artifactregistry.googleapis.com:Artifact Registry"
        "secretmanager.googleapis.com:Secret Manager"
        "iam.googleapis.com:IAM"
        "iamcredentials.googleapis.com:IAM Credentials"
        "cloudresourcemanager.googleapis.com:Resource Manager"
        "certificatemanager.googleapis.com:Certificate Manager"
    )

    for api_info in "${apis[@]}"; do
        local api="${api_info%%:*}"
        local name="${api_info##*:}"
        echo -n "  $name ($api)... "
        if [[ "$DRY_RUN" == "true" ]]; then
            echo "WOULD ENABLE"
        else
            gcloud services enable "$api" --project="$PROJECT" --quiet
            echo "ENABLED"
        fi
    done
}

# ============================================================
# Step 3: Create Artifact Registry (reuse existing)
# ============================================================

ensure_artifact_registry() {
    echo ""
    echo "================================================================"
    echo "  Step 3: Artifact Registry"
    echo "================================================================"
    echo ""

    echo -n "  Repository 'aex' in $REGION... "
    if gcloud artifacts repositories describe aex --location="$REGION" --project="$PROJECT" &> /dev/null; then
        echo "EXISTS (reusing)"
    elif [[ "$DRY_RUN" == "true" ]]; then
        echo "WOULD CREATE"
    else
        gcloud artifacts repositories create aex \
            --repository-format=docker \
            --location="$REGION" \
            --project="$PROJECT" \
            --description="Agent Exchange Docker images"
        echo "CREATED"
    fi
}

# ============================================================
# Step 4: Create GKE Cluster
# ============================================================

create_gke_cluster() {
    echo ""
    echo "================================================================"
    echo "  Step 4: Creating GKE Cluster ($MODE mode)"
    echo "================================================================"
    echo ""

    echo "  Name:   $CLUSTER_NAME"
    echo "  Region: $REGION"
    echo "  Mode:   $MODE"
    echo ""

    if gcloud container clusters describe "$CLUSTER_NAME" \
        --region="$REGION" --project="$PROJECT" &> /dev/null; then
        echo "  Cluster already exists. Skipping creation."
        return 0
    fi

    if [[ "$DRY_RUN" == "true" ]]; then
        echo "  WOULD CREATE cluster"
        return 0
    fi

    if [[ "$MODE" == "autopilot" ]]; then
        echo "  Creating Autopilot cluster..."
        gcloud container clusters create-auto "$CLUSTER_NAME" \
            --region="$REGION" \
            --project="$PROJECT" \
            --release-channel=regular \
            --network=default \
            --subnetwork=default \
            --quiet
    else
        echo "  Creating Standard cluster with node autoscaling..."
        gcloud container clusters create "$CLUSTER_NAME" \
            --region="$REGION" \
            --project="$PROJECT" \
            --machine-type=e2-standard-4 \
            --num-nodes=1 \
            --min-nodes=2 \
            --max-nodes=5 \
            --enable-autoscaling \
            --enable-autorepair \
            --enable-autoupgrade \
            --release-channel=regular \
            --workload-pool="$PROJECT.svc.id.goog" \
            --network=default \
            --subnetwork=default \
            --quiet
    fi

    echo "  Cluster created"
}

# ============================================================
# Step 5: Configure kubectl
# ============================================================

configure_kubectl() {
    echo ""
    echo "================================================================"
    echo "  Step 5: Configuring kubectl"
    echo "================================================================"
    echo ""

    if [[ "$DRY_RUN" == "true" ]]; then
        echo "  WOULD configure kubectl context"
        return 0
    fi

    gcloud container clusters get-credentials "$CLUSTER_NAME" \
        --region="$REGION" \
        --project="$PROJECT"

    echo "  Context: $(kubectl config current-context)"

    # Create namespace
    if kubectl get namespace "$NAMESPACE" &> /dev/null; then
        echo "  Namespace '$NAMESPACE' already exists"
    else
        kubectl create namespace "$NAMESPACE"
        echo "  Namespace '$NAMESPACE' created"
    fi
}

# ============================================================
# Step 6: Install Helm Charts
# ============================================================

install_helm_charts() {
    echo ""
    echo "================================================================"
    echo "  Step 6: Installing Helm Charts"
    echo "================================================================"
    echo ""

    if [[ "$DRY_RUN" == "true" ]]; then
        echo "  WOULD install: ingress-nginx, cert-manager, external-secrets"
        if [[ "$MODE" == "standard" ]]; then
            echo "  WOULD install: metrics-server (standard mode)"
        fi
        return 0
    fi

    # Add Helm repos
    echo "  Adding Helm repositories..."
    helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx 2>/dev/null || true
    helm repo add jetstack https://charts.jetstack.io 2>/dev/null || true
    helm repo add external-secrets https://charts.external-secrets.io 2>/dev/null || true
    if [[ "$MODE" == "standard" ]]; then
        helm repo add metrics-server https://kubernetes-sigs.github.io/metrics-server/ 2>/dev/null || true
    fi
    helm repo update

    # Nginx Ingress Controller
    echo ""
    echo "  Installing Nginx Ingress Controller..."
    helm upgrade --install ingress-nginx ingress-nginx/ingress-nginx \
        --namespace ingress-nginx \
        --create-namespace \
        --set controller.service.type=LoadBalancer \
        --set controller.metrics.enabled=true \
        --set controller.podAnnotations."prometheus\.io/scrape"=true \
        --set controller.podAnnotations."prometheus\.io/port"=10254 \
        --wait \
        --timeout 300s
    echo "  Nginx Ingress Controller installed"

    # cert-manager
    echo ""
    echo "  Installing cert-manager..."
    helm upgrade --install cert-manager jetstack/cert-manager \
        --namespace cert-manager \
        --create-namespace \
        --set crds.enabled=true \
        --wait \
        --timeout 300s
    echo "  cert-manager installed"

    # External Secrets Operator
    echo ""
    echo "  Installing External Secrets Operator..."
    helm upgrade --install external-secrets external-secrets/external-secrets \
        --namespace external-secrets \
        --create-namespace \
        --set installCRDs=true \
        --wait \
        --timeout 300s
    echo "  External Secrets Operator installed"

    # metrics-server (Standard mode only, Autopilot has it built in)
    if [[ "$MODE" == "standard" ]]; then
        echo ""
        echo "  Installing metrics-server (Standard mode)..."
        helm upgrade --install metrics-server metrics-server/metrics-server \
            --namespace kube-system \
            --set args[0]="--kubelet-insecure-tls" \
            --wait \
            --timeout 120s
        echo "  metrics-server installed"
    fi
}

# ============================================================
# Step 7: Set up Workload Identity
# ============================================================

setup_workload_identity() {
    echo ""
    echo "================================================================"
    echo "  Step 7: Setting Up Workload Identity"
    echo "================================================================"
    echo ""

    local sa_name="aex-gke"
    local sa_email="$sa_name@$PROJECT.iam.gserviceaccount.com"
    local k8s_sa="aex-workload"

    if [[ "$DRY_RUN" == "true" ]]; then
        echo "  WOULD create GCP SA: $sa_email"
        echo "  WOULD create K8s SA: $k8s_sa"
        echo "  WOULD bind Workload Identity"
        return 0
    fi

    # Create GCP service account
    if gcloud iam service-accounts describe "$sa_email" --project="$PROJECT" &> /dev/null; then
        echo "  GCP SA '$sa_name' already exists"
    else
        gcloud iam service-accounts create "$sa_name" \
            --display-name="Agent Exchange GKE Workload" \
            --project="$PROJECT"
        echo "  Created GCP SA: $sa_email"
    fi

    # Grant roles
    local roles=(
        "roles/secretmanager.secretAccessor"
        "roles/datastore.user"
        "roles/logging.logWriter"
        "roles/cloudtrace.agent"
        "roles/monitoring.metricWriter"
    )

    for role in "${roles[@]}"; do
        gcloud projects add-iam-policy-binding "$PROJECT" \
            --member="serviceAccount:$sa_email" \
            --role="$role" \
            --quiet 2>/dev/null || true
    done
    echo "  IAM roles granted"

    # Create K8s service account
    if kubectl get serviceaccount "$k8s_sa" -n "$NAMESPACE" &> /dev/null; then
        echo "  K8s SA '$k8s_sa' already exists"
    else
        kubectl create serviceaccount "$k8s_sa" -n "$NAMESPACE"
        echo "  Created K8s SA: $k8s_sa"
    fi

    # Annotate K8s SA
    kubectl annotate serviceaccount "$k8s_sa" \
        --namespace="$NAMESPACE" \
        "iam.gke.io/gcp-service-account=$sa_email" \
        --overwrite

    # Bind Workload Identity
    gcloud iam service-accounts add-iam-policy-binding "$sa_email" \
        --project="$PROJECT" \
        --role="roles/iam.workloadIdentityUser" \
        --member="serviceAccount:$PROJECT.svc.id.goog[$NAMESPACE/$k8s_sa]" \
        --quiet 2>/dev/null || true

    echo "  Workload Identity configured"

    # Also set up GitHub Actions service account for GKE access
    local gh_sa_name="aex-github-actions"
    local gh_sa_email="$gh_sa_name@$PROJECT.iam.gserviceaccount.com"

    if gcloud iam service-accounts describe "$gh_sa_email" --project="$PROJECT" &> /dev/null; then
        echo "  GitHub Actions SA already exists"

        # Grant additional GKE-specific roles
        local gke_roles=(
            "roles/container.developer"
            "roles/container.clusterViewer"
        )
        for role in "${gke_roles[@]}"; do
            gcloud projects add-iam-policy-binding "$PROJECT" \
                --member="serviceAccount:$gh_sa_email" \
                --role="$role" \
                --quiet 2>/dev/null || true
        done
        echo "  GKE roles granted to GitHub Actions SA"
    else
        echo "  Warning: GitHub Actions SA not found. Run setup-gcp.sh first."
    fi
}

# ============================================================
# Step 8: Create Namespace and Secrets
# ============================================================

create_namespace_and_secrets() {
    echo ""
    echo "================================================================"
    echo "  Step 8: Creating Namespace and Secrets"
    echo "================================================================"
    echo ""

    if [[ "$DRY_RUN" == "true" ]]; then
        echo "  WOULD create namespace '$NAMESPACE'"
        echo "  WOULD sync secrets from Secret Manager"
        return 0
    fi

    # Namespace should already exist from step 5, but ensure it
    kubectl get namespace "$NAMESPACE" &> /dev/null || kubectl create namespace "$NAMESPACE"

    # Sync secrets from GCP Secret Manager. aex-jwt-secret feeds both the
    # gateway's JWT_SECRET and aex-identity's JWT_SIGNING_KEY.
    local secrets_to_sync=(
        "aex-jwt-secret:JWT_SECRET"
        "aex-jwt-secret:JWT_SIGNING_KEY"
        "aex-api-key-salt:API_KEY_SALT"
        "ANTHROPIC_API_KEY:ANTHROPIC_API_KEY"
    )

    for mapping in "${secrets_to_sync[@]}"; do
        local gcp_secret="${mapping%%:*}"
        local k8s_key="${mapping##*:}"

        # A value already exported by the caller wins over Secret Manager.
        if [[ -n "${!k8s_key:-}" ]]; then
            echo "  Using $k8s_key from the environment"
            continue
        fi

        local value=""
        value=$(gcloud secrets versions access latest --secret="$gcp_secret" --project="$PROJECT" 2>/dev/null) || true

        if [[ -n "$value" ]] && [[ "$value" != placeholder* ]]; then
            export "$k8s_key=$value"
            echo "  Synced: $gcp_secret -> $k8s_key"
        else
            echo "  Skipped: $gcp_secret (not found in Secret Manager)"
        fi
    done

    # Creates or updates aex-secrets with every key the manifests reference,
    # keeping values already in the cluster and generating random
    # JWT/webhook/MongoDB secrets that are still missing.
    bash "$PROJECT_ROOT/deploy/k8s/create-secrets.sh" --namespace "$NAMESPACE"
    echo "  K8s secrets configured"
}

# ============================================================
# Step 9: Output Summary
# ============================================================

print_summary() {
    echo ""
    echo "================================================================"
    echo "  GKE Setup Complete"
    echo "================================================================"
    echo ""
    echo "  Project:     $PROJECT"
    echo "  Cluster:     $CLUSTER_NAME ($MODE mode)"
    echo "  Region:      $REGION"
    echo "  Namespace:   $NAMESPACE"

    if [[ "$DRY_RUN" != "true" ]]; then
        echo "  Context:     $(kubectl config current-context)"
        echo ""
        echo "  Cluster nodes:"
        kubectl get nodes -o wide 2>/dev/null || echo "    (Autopilot scales on demand)"
        echo ""
        echo "  Installed Helm charts:"
        helm list -A 2>/dev/null | head -20
    fi

    echo ""
    echo "Next steps:"
    echo ""
    echo "  1. Deploy AEX to GKE:"
    echo "     ./deploy/gcp/deploy-gke.sh --project-id $PROJECT --region $REGION"
    echo ""
    echo "  2. Or apply K8s manifests directly:"
    echo "     kubectl apply -k deploy/k8s/base/"
    echo ""
    echo "  3. Check cluster status:"
    echo "     kubectl get pods -n $NAMESPACE"
    echo "     kubectl get svc -n $NAMESPACE"
    echo ""
    echo "  4. Tear down when done:"
    echo "     ./hack/deploy/teardown-gke.sh"
    echo ""
    echo "  5. Add these GitHub secrets for CI/CD:"
    echo "     GKE_CLUSTER_NAME: $CLUSTER_NAME"
    echo "     GKE_CLUSTER_REGION: $REGION"
    echo ""
}

# ============================================================
# Main
# ============================================================

echo ""
echo "================================================================"
echo "  Agent Exchange - GKE Setup"
echo "================================================================"
echo ""
echo "  Project:  $PROJECT"
echo "  Region:   $REGION"
echo "  Cluster:  $CLUSTER_NAME"
echo "  Mode:     $MODE"
echo ""

if [[ "$DRY_RUN" == "true" ]]; then
    echo "  *** DRY-RUN MODE - No changes will be made ***"
    echo ""
fi

check_prerequisites
enable_apis
ensure_artifact_registry
create_gke_cluster
configure_kubectl
install_helm_charts
setup_workload_identity
create_namespace_and_secrets
print_summary
