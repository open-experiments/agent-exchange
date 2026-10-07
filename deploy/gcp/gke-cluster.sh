#!/bin/bash
set -euo pipefail

# Agent Exchange - GKE Cluster Setup Script
# Creates and configures a GKE cluster for AEX deployment
#
# Usage:
#   ./gke-cluster.sh --project-id my-project --region us-central1
#   ./gke-cluster.sh --project-id my-project --mode standard --region us-central1
#   ./gke-cluster.sh --project-id my-project --delete

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Defaults
CLUSTER_NAME="aex-cluster"
REGION="us-central1"
PROJECT_ID=""
MODE="autopilot"  # autopilot or standard
DELETE=false
NAMESPACE="aex"

# Standard mode defaults
MIN_NODES=2
MAX_NODES=5
MACHINE_TYPE="e2-standard-4"

usage() {
    cat <<EOF
Agent Exchange - GKE Cluster Setup

Usage: $0 [options]

Options:
  --project-id ID       GCP project ID (required)
  --cluster-name NAME   GKE cluster name (default: aex-cluster)
  --region REGION       GCP region (default: us-central1)
  --mode MODE           Cluster mode: autopilot or standard (default: autopilot)
  --min-nodes N         Min nodes for standard mode (default: 2)
  --max-nodes N         Max nodes for standard mode (default: 5)
  --machine-type TYPE   Machine type for standard mode (default: e2-standard-4)
  --delete              Delete the cluster and associated resources
  -h, --help            Show this help message

Examples:
  $0 --project-id my-project
  $0 --project-id my-project --mode standard --max-nodes 10
  $0 --project-id my-project --delete
EOF
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --project-id)    PROJECT_ID="$2"; shift 2 ;;
        --cluster-name)  CLUSTER_NAME="$2"; shift 2 ;;
        --region)        REGION="$2"; shift 2 ;;
        --mode)          MODE="$2"; shift 2 ;;
        --min-nodes)     MIN_NODES="$2"; shift 2 ;;
        --max-nodes)     MAX_NODES="$2"; shift 2 ;;
        --machine-type)  MACHINE_TYPE="$2"; shift 2 ;;
        --delete)        DELETE=true; shift ;;
        -h|--help)       usage; exit 0 ;;
        *)               echo "Unknown option: $1"; usage; exit 1 ;;
    esac
done

# Validate required parameters
if [[ -z "$PROJECT_ID" ]]; then
    PROJECT_ID="${GCP_PROJECT_ID:-}"
fi
if [[ -z "$PROJECT_ID" ]]; then
    echo "Error: --project-id is required (or set GCP_PROJECT_ID)"
    exit 1
fi

if [[ "$MODE" != "autopilot" && "$MODE" != "standard" ]]; then
    echo "Error: --mode must be 'autopilot' or 'standard'"
    exit 1
fi

# ============================================================
# Validation
# ============================================================

validate_prerequisites() {
    echo "Validating prerequisites..."

    # Check gcloud
    if ! command -v gcloud &> /dev/null; then
        echo "Error: gcloud CLI is not installed"
        echo "  Install: https://cloud.google.com/sdk/docs/install"
        exit 1
    fi

    # Check authentication
    if ! gcloud auth print-identity-token &> /dev/null; then
        echo "Error: Not authenticated with gcloud. Run 'gcloud auth login'"
        exit 1
    fi

    # Check project access
    if ! gcloud projects describe "$PROJECT_ID" &> /dev/null; then
        echo "Error: Cannot access project '$PROJECT_ID'"
        exit 1
    fi

    # Check kubectl
    if ! command -v kubectl &> /dev/null; then
        echo "Warning: kubectl not found. Installing via gcloud..."
        gcloud components install kubectl --quiet
    fi

    # Check helm
    if ! command -v helm &> /dev/null; then
        echo "Warning: helm not found."
        echo "  Install: https://helm.sh/docs/intro/install/"
        echo "  Or: curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash"
        exit 1
    fi

    echo "Prerequisites OK"
}

# ============================================================
# API Enablement
# ============================================================

enable_apis() {
    echo "Enabling required GCP APIs..."

    local apis=(
        "container.googleapis.com"
        "compute.googleapis.com"
        "artifactregistry.googleapis.com"
        "secretmanager.googleapis.com"
        "iam.googleapis.com"
        "iamcredentials.googleapis.com"
        "cloudresourcemanager.googleapis.com"
    )

    for api in "${apis[@]}"; do
        echo "  Enabling $api..."
        gcloud services enable "$api" --project="$PROJECT_ID" --quiet
    done

    echo "APIs enabled"
}

# ============================================================
# Cluster Creation
# ============================================================

create_cluster() {
    echo ""
    echo "Creating GKE cluster..."
    echo "  Name:    $CLUSTER_NAME"
    echo "  Region:  $REGION"
    echo "  Mode:    $MODE"
    echo ""

    # Check if cluster already exists
    if gcloud container clusters describe "$CLUSTER_NAME" \
        --region="$REGION" \
        --project="$PROJECT_ID" &> /dev/null; then
        echo "Cluster '$CLUSTER_NAME' already exists. Skipping creation."
        return 0
    fi

    if [[ "$MODE" == "autopilot" ]]; then
        echo "Creating Autopilot cluster (Google manages node infrastructure)..."
        gcloud container clusters create-auto "$CLUSTER_NAME" \
            --region="$REGION" \
            --project="$PROJECT_ID" \
            --release-channel=regular \
            --network=default \
            --subnetwork=default \
            --quiet
    else
        echo "Creating Standard cluster with node autoscaling..."
        gcloud container clusters create "$CLUSTER_NAME" \
            --region="$REGION" \
            --project="$PROJECT_ID" \
            --machine-type="$MACHINE_TYPE" \
            --num-nodes=1 \
            --min-nodes="$MIN_NODES" \
            --max-nodes="$MAX_NODES" \
            --enable-autoscaling \
            --enable-autorepair \
            --enable-autoupgrade \
            --release-channel=regular \
            --workload-pool="$PROJECT_ID.svc.id.goog" \
            --network=default \
            --subnetwork=default \
            --quiet
    fi

    echo "Cluster created successfully"
}

# ============================================================
# kubectl Configuration
# ============================================================

configure_kubectl() {
    echo "Configuring kubectl context..."

    gcloud container clusters get-credentials "$CLUSTER_NAME" \
        --region="$REGION" \
        --project="$PROJECT_ID"

    echo "kubectl context set to: $(kubectl config current-context)"

    # Create namespace
    if kubectl get namespace "$NAMESPACE" &> /dev/null; then
        echo "Namespace '$NAMESPACE' already exists"
    else
        echo "Creating namespace '$NAMESPACE'..."
        kubectl create namespace "$NAMESPACE"
    fi

    echo "kubectl configured"
}

# ============================================================
# Helm Chart Installations
# ============================================================

install_nginx_ingress() {
    echo "Installing Nginx Ingress Controller..."

    helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx 2>/dev/null || true
    helm repo update

    if helm status ingress-nginx -n ingress-nginx &> /dev/null; then
        echo "  Nginx Ingress already installed. Upgrading..."
    fi

    helm upgrade --install ingress-nginx ingress-nginx/ingress-nginx \
        --namespace ingress-nginx \
        --create-namespace \
        --set controller.service.type=LoadBalancer \
        --set controller.metrics.enabled=true \
        --set controller.podAnnotations."prometheus\.io/scrape"=true \
        --set controller.podAnnotations."prometheus\.io/port"=10254 \
        --wait \
        --timeout 300s

    echo "Nginx Ingress Controller installed"
}

install_external_secrets() {
    echo "Installing External Secrets Operator..."

    helm repo add external-secrets https://charts.external-secrets.io 2>/dev/null || true
    helm repo update

    if helm status external-secrets -n external-secrets &> /dev/null; then
        echo "  External Secrets already installed. Upgrading..."
    fi

    helm upgrade --install external-secrets external-secrets/external-secrets \
        --namespace external-secrets \
        --create-namespace \
        --set installCRDs=true \
        --wait \
        --timeout 300s

    echo "External Secrets Operator installed"
}

# ============================================================
# Workload Identity
# ============================================================

setup_workload_identity() {
    echo "Configuring Workload Identity..."

    local sa_name="aex-gke"
    local sa_email="$sa_name@$PROJECT_ID.iam.gserviceaccount.com"
    local k8s_sa="aex-workload"

    # Create GCP service account if it does not exist
    if gcloud iam service-accounts describe "$sa_email" --project="$PROJECT_ID" &> /dev/null; then
        echo "  GCP service account '$sa_name' already exists"
    else
        gcloud iam service-accounts create "$sa_name" \
            --display-name="Agent Exchange GKE Workload" \
            --project="$PROJECT_ID"
        echo "  Created GCP service account: $sa_email"
    fi

    # Grant roles to GCP service account
    local roles=(
        "roles/secretmanager.secretAccessor"
        "roles/datastore.user"
        "roles/logging.logWriter"
        "roles/cloudtrace.agent"
        "roles/monitoring.metricWriter"
    )

    for role in "${roles[@]}"; do
        gcloud projects add-iam-policy-binding "$PROJECT_ID" \
            --member="serviceAccount:$sa_email" \
            --role="$role" \
            --quiet 2>/dev/null || true
    done

    # Create K8s service account
    if kubectl get serviceaccount "$k8s_sa" -n "$NAMESPACE" &> /dev/null; then
        echo "  K8s service account '$k8s_sa' already exists"
    else
        kubectl create serviceaccount "$k8s_sa" -n "$NAMESPACE"
    fi

    # Annotate K8s SA with GCP SA
    kubectl annotate serviceaccount "$k8s_sa" \
        --namespace="$NAMESPACE" \
        "iam.gke.io/gcp-service-account=$sa_email" \
        --overwrite

    # Allow K8s SA to impersonate GCP SA
    gcloud iam service-accounts add-iam-policy-binding "$sa_email" \
        --project="$PROJECT_ID" \
        --role="roles/iam.workloadIdentityUser" \
        --member="serviceAccount:$PROJECT_ID.svc.id.goog[$NAMESPACE/$k8s_sa]" \
        --quiet 2>/dev/null || true

    echo "Workload Identity configured"
    echo "  GCP SA: $sa_email"
    echo "  K8s SA: $k8s_sa (namespace: $NAMESPACE)"
}

# ============================================================
# Secrets from GCP Secret Manager
# ============================================================

create_k8s_secrets() {
    echo "Creating K8s secrets from GCP Secret Manager..."

    # GCP secret -> aex-secrets key. aex-jwt-secret feeds both the gateway's
    # JWT_SECRET and aex-identity's JWT_SIGNING_KEY.
    local secrets_to_sync=(
        "aex-jwt-secret:JWT_SECRET"
        "aex-jwt-secret:JWT_SIGNING_KEY"
        "aex-api-key-salt:API_KEY_SALT"
        "ANTHROPIC_API_KEY:ANTHROPIC_API_KEY"
    )

    for secret_mapping in "${secrets_to_sync[@]}"; do
        local gcp_secret="${secret_mapping%%:*}"
        local k8s_key="${secret_mapping##*:}"

        # A value already exported by the caller wins over Secret Manager.
        if [[ -n "${!k8s_key:-}" ]]; then
            echo "  Using $k8s_key from the environment"
            continue
        fi

        local value=""
        value=$(gcloud secrets versions access latest --secret="$gcp_secret" --project="$PROJECT_ID" 2>/dev/null) || true

        if [[ -n "$value" ]] && [[ "$value" != placeholder* ]]; then
            export "$k8s_key=$value"
            echo "  Synced: $gcp_secret -> $k8s_key"
        else
            echo "  Not in Secret Manager: $gcp_secret (keeping any value already in the cluster)"
        fi
    done

    if [[ -z "${ANTHROPIC_API_KEY:-}" ]]; then
        echo "  Create the Anthropic key with: echo 'your-key' | gcloud secrets create ANTHROPIC_API_KEY --data-file=- --project=$PROJECT_ID"
    fi

    # Creates or updates aex-secrets with every key the manifests reference,
    # generating random JWT/webhook/MongoDB secrets that are still missing.
    bash "$PROJECT_ROOT/deploy/k8s/create-secrets.sh" --namespace "$NAMESPACE"
}

# ============================================================
# Cluster Deletion
# ============================================================

delete_cluster() {
    echo ""
    echo "================================================================"
    echo "  WARNING: Deleting GKE cluster and associated resources"
    echo "  Cluster: $CLUSTER_NAME"
    echo "  Project: $PROJECT_ID"
    echo "  Region:  $REGION"
    echo "================================================================"
    echo ""

    read -p "Type 'DELETE' to confirm: " confirmation
    if [[ "$confirmation" != "DELETE" ]]; then
        echo "Aborted."
        exit 1
    fi

    # Get credentials first (may fail if cluster is already gone)
    gcloud container clusters get-credentials "$CLUSTER_NAME" \
        --region="$REGION" \
        --project="$PROJECT_ID" 2>/dev/null || true

    # Delete namespace (cascades to all resources within)
    echo "Deleting namespace '$NAMESPACE'..."
    kubectl delete namespace "$NAMESPACE" --wait=false 2>/dev/null || true

    # Uninstall Helm charts
    echo "Uninstalling Helm charts..."
    helm uninstall ingress-nginx -n ingress-nginx 2>/dev/null || true
    helm uninstall external-secrets -n external-secrets 2>/dev/null || true
    kubectl delete namespace ingress-nginx --wait=false 2>/dev/null || true
    kubectl delete namespace external-secrets --wait=false 2>/dev/null || true

    # Delete the cluster
    echo "Deleting GKE cluster '$CLUSTER_NAME'..."
    gcloud container clusters delete "$CLUSTER_NAME" \
        --region="$REGION" \
        --project="$PROJECT_ID" \
        --quiet

    # Clean up IAM bindings
    echo "Cleaning up IAM bindings..."
    local sa_email="aex-gke@$PROJECT_ID.iam.gserviceaccount.com"
    if gcloud iam service-accounts describe "$sa_email" --project="$PROJECT_ID" &> /dev/null; then
        gcloud iam service-accounts delete "$sa_email" \
            --project="$PROJECT_ID" \
            --quiet 2>/dev/null || true
    fi

    echo ""
    echo "GKE cluster and resources deleted successfully"
}

# ============================================================
# Main
# ============================================================

echo ""
echo "================================================================"
echo "  Agent Exchange - GKE Cluster Setup"
echo "================================================================"
echo ""
echo "  Project:  $PROJECT_ID"
echo "  Cluster:  $CLUSTER_NAME"
echo "  Region:   $REGION"
echo "  Mode:     $MODE"
echo ""

if [[ "$DELETE" == "true" ]]; then
    validate_prerequisites
    delete_cluster
    exit 0
fi

validate_prerequisites
echo ""

enable_apis
echo ""

create_cluster
echo ""

configure_kubectl
echo ""

install_nginx_ingress
echo ""

install_external_secrets
echo ""

setup_workload_identity
echo ""

create_k8s_secrets
echo ""

echo ""
echo "================================================================"
echo "  GKE Cluster Setup Complete"
echo "================================================================"
echo ""
echo "Cluster: $CLUSTER_NAME ($MODE mode)"
echo "Region:  $REGION"
echo "Context: $(kubectl config current-context)"
echo ""
echo "Next steps:"
echo "  1. Deploy AEX services:"
echo "     ./deploy/gcp/deploy-gke.sh --project-id $PROJECT_ID --region $REGION"
echo ""
echo "  2. Or use Kustomize directly:"
echo "     kubectl apply -k deploy/k8s/base/"
echo ""
echo "  3. Check cluster status:"
echo "     kubectl get nodes"
echo "     kubectl get pods -n $NAMESPACE"
echo ""
