#!/bin/bash
set -e

# Agent Exchange - GCP Teardown Script
# This script removes all GCP resources created for Agent Exchange
# WARNING: This is DESTRUCTIVE and will delete all data!

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"


# Configuration
PROJECT="${GCP_PROJECT_ID:-}"
REGION="${GCP_REGION:-us-central1}"

usage() {
    echo "Agent Exchange - GCP Teardown"
    echo ""
    echo "Usage: $0 [command]"
    echo ""
    echo "Commands:"
    echo "  all           Delete all GCP resources (default)"
    echo "  cloudrun      Delete Cloud Run services only"
    echo "  artifacts     Delete Artifact Registry only"
    echo "  firestore     Delete Firestore data only"
    echo "  secrets       Delete Secret Manager secrets only"
    echo "  iam           Delete IAM resources only"
    echo ""
    echo "Environment variables:"
    echo "  GCP_PROJECT_ID   Google Cloud project ID (required)"
    echo "  GCP_REGION       Google Cloud region (default: us-central1)"
    echo ""
    echo "WARNING: This will permanently delete all resources and data!"
}

check_prerequisites() {
    echo "Checking prerequisites..."
    
    if [ -z "$PROJECT" ]; then
        echo "Error: GCP_PROJECT_ID environment variable is required"
        exit 1
    fi
    
    if ! command -v gcloud &> /dev/null; then
        echo "Error: gcloud CLI is not installed"
        exit 1
    fi
    
    if ! gcloud auth print-identity-token &> /dev/null; then
        echo "Error: Not authenticated with gcloud. Run 'gcloud auth login'"
        exit 1
    fi
    
    echo "Prerequisites OK"
}

confirm_deletion() {
    echo ""
    echo "╔══════════════════════════════════════════════════════════════╗"
    echo "║                        WARNING                                ║"
    echo "║  This will PERMANENTLY DELETE all Agent Exchange resources   ║"
    echo "║  in GCP project: $PROJECT                                     "
    echo "║  Region: $REGION                                              "
    echo "║                                                                ║"
    echo "║  This action CANNOT be undone!                                ║"
    echo "╚══════════════════════════════════════════════════════════════╝"
    echo ""
    read -p "Type 'DELETE' to confirm: " confirmation
    
    if [ "$confirmation" != "DELETE" ]; then
        echo "Aborted."
        exit 1
    fi
}

delete_cloudrun_services() {
    echo "Deleting Cloud Run services..."
    
    services=(
        "aex-gateway"
        "aex-gateway-staging"
        "aex-work-publisher"
        "aex-work-publisher-staging"
        "aex-bid-gateway"
        "aex-bid-gateway-staging"
        "aex-bid-evaluator"
        "aex-bid-evaluator-staging"
        "aex-contract-engine"
        "aex-contract-engine-staging"
        "aex-provider-registry"
        "aex-provider-registry-staging"
        "aex-trust-broker"
        "aex-trust-broker-staging"
        "aex-identity"
        "aex-identity-staging"
        "aex-settlement"
        "aex-settlement-staging"
        "aex-telemetry"
        "aex-telemetry-staging"
    )
    
    for service in "${services[@]}"; do
        if gcloud run services describe "$service" --region="$REGION" --project="$PROJECT" &> /dev/null; then
            echo "  Deleting $service..."
            gcloud run services delete "$service" \
                --region="$REGION" \
                --project="$PROJECT" \
                --quiet 2>/dev/null || true
        fi
    done
    
    echo "Cloud Run services deleted"
}

delete_artifact_registry() {
    echo "Deleting Artifact Registry..."
    
    if gcloud artifacts repositories describe aex --location="$REGION" --project="$PROJECT" &> /dev/null; then
        echo "  Deleting repository 'aex'..."
        gcloud artifacts repositories delete aex \
            --location="$REGION" \
            --project="$PROJECT" \
            --quiet 2>/dev/null || true
    fi
    
    echo "Artifact Registry deleted"
}

delete_firestore() {
    echo "Deleting Firestore data..."
    
    # Note: This deletes all data in Firestore, not the database itself
    # Firestore database cannot be deleted, only data can be cleared
    
    collections=(
        "work_specs"
        "bids"
        "contracts"
        "providers"
        "subscriptions"
        "tenants"
        "api_keys"
        "ledger_entries"
        "balances"
        "trust_scores"
        "outcomes"
    )
    
    echo "  Note: Firestore data deletion requires firebase-tools or manual deletion"
    echo "  Collections to delete: ${collections[*]}"
    
    if command -v firebase &> /dev/null; then
        for collection in "${collections[@]}"; do
            echo "  Deleting collection $collection..."
            firebase firestore:delete --project "$PROJECT" "$collection" --recursive --yes 2>/dev/null || true
        done
    else
        echo "  firebase-tools not installed. Please delete Firestore data manually from the console."
        echo "  Install with: npm install -g firebase-tools"
    fi
    
    echo "Firestore data deletion initiated"
}

delete_secrets() {
    echo "Deleting Secret Manager secrets..."
    
    secrets=(
        "aex-jwt-secret"
        "aex-api-key-salt"
        "aex-mongo-uri"
        "aex-db-password"
    )
    
    for secret in "${secrets[@]}"; do
        if gcloud secrets describe "$secret" --project="$PROJECT" &> /dev/null; then
            echo "  Deleting $secret..."
            gcloud secrets delete "$secret" \
                --project="$PROJECT" \
                --quiet 2>/dev/null || true
        fi
    done
    
    echo "Secrets deleted"
}

delete_iam_resources() {
    echo "Deleting IAM resources..."
    
    # Service accounts
    service_accounts=(
        "aex-cloudrun@$PROJECT.iam.gserviceaccount.com"
        "aex-github-actions@$PROJECT.iam.gserviceaccount.com"
    )
    
    for sa in "${service_accounts[@]}"; do
        if gcloud iam service-accounts describe "$sa" --project="$PROJECT" &> /dev/null; then
            echo "  Deleting service account $sa..."
            gcloud iam service-accounts delete "$sa" \
                --project="$PROJECT" \
                --quiet 2>/dev/null || true
        fi
    done
    
    # Workload Identity Pool
    pool_name="github-actions-pool"
    if gcloud iam workload-identity-pools describe "$pool_name" --location="global" --project="$PROJECT" &> /dev/null; then
        echo "  Deleting Workload Identity Pool..."
        
        # Delete provider first
        provider_name="github-actions-provider"
        gcloud iam workload-identity-pools providers delete "$provider_name" \
            --workload-identity-pool="$pool_name" \
            --location="global" \
            --project="$PROJECT" \
            --quiet 2>/dev/null || true
        
        # Delete pool
        gcloud iam workload-identity-pools delete "$pool_name" \
            --location="global" \
            --project="$PROJECT" \
            --quiet 2>/dev/null || true
    fi
    
    echo "IAM resources deleted"
}

delete_cloudsql() {
    echo "Checking for Cloud SQL instances..."
    
    instances=$(gcloud sql instances list \
        --project="$PROJECT" \
        --filter="name~aex" \
        --format="value(name)" 2>/dev/null || echo "")
    
    for instance in $instances; do
        echo "  Deleting Cloud SQL instance $instance..."
        gcloud sql instances delete "$instance" \
            --project="$PROJECT" \
            --quiet 2>/dev/null || true
    done
    
    echo "Cloud SQL instances deleted"
}

delete_pubsub() {
    echo "Checking for Pub/Sub resources..."
    
    # Topics
    topics=$(gcloud pubsub topics list \
        --project="$PROJECT" \
        --filter="name~aex" \
        --format="value(name)" 2>/dev/null || echo "")
    
    for topic in $topics; do
        echo "  Deleting topic $topic..."
        gcloud pubsub topics delete "$topic" \
            --project="$PROJECT" \
            --quiet 2>/dev/null || true
    done
    
    # Subscriptions
    subs=$(gcloud pubsub subscriptions list \
        --project="$PROJECT" \
        --filter="name~aex" \
        --format="value(name)" 2>/dev/null || echo "")
    
    for sub in $subs; do
        echo "  Deleting subscription $sub..."
        gcloud pubsub subscriptions delete "$sub" \
            --project="$PROJECT" \
            --quiet 2>/dev/null || true
    done
    
    echo "Pub/Sub resources deleted"
}

delete_cloud_tasks() {
    echo "Checking for Cloud Tasks queues..."
    
    queues=$(gcloud tasks queues list \
        --location="$REGION" \
        --project="$PROJECT" \
        --filter="name~aex" \
        --format="value(name)" 2>/dev/null || echo "")
    
    for queue in $queues; do
        echo "  Deleting queue $queue..."
        gcloud tasks queues delete "$queue" \
            --location="$REGION" \
            --project="$PROJECT" \
            --quiet 2>/dev/null || true
    done
    
    echo "Cloud Tasks queues deleted"
}

delete_logging() {
    echo "Deleting log-based metrics and sinks..."
    
    # Log-based metrics
    metrics=$(gcloud logging metrics list \
        --project="$PROJECT" \
        --filter="name~aex" \
        --format="value(name)" 2>/dev/null || echo "")
    
    for metric in $metrics; do
        echo "  Deleting metric $metric..."
        gcloud logging metrics delete "$metric" \
            --project="$PROJECT" \
            --quiet 2>/dev/null || true
    done
    
    # Log sinks
    sinks=$(gcloud logging sinks list \
        --project="$PROJECT" \
        --filter="name~aex" \
        --format="value(name)" 2>/dev/null || echo "")
    
    for sink in $sinks; do
        echo "  Deleting sink $sink..."
        gcloud logging sinks delete "$sink" \
            --project="$PROJECT" \
            --quiet 2>/dev/null || true
    done
    
    echo "Logging resources deleted"
}

print_summary() {
    echo ""
    echo "========================================"
    echo "GCP Teardown Complete!"
    echo "========================================"
    echo ""
    echo "Deleted resources:"
    echo "  - Cloud Run services"
    echo "  - Artifact Registry repository"
    echo "  - Secret Manager secrets"
    echo "  - IAM service accounts and Workload Identity"
    echo "  - Cloud SQL instances (if any)"
    echo "  - Pub/Sub topics and subscriptions (if any)"
    echo "  - Cloud Tasks queues (if any)"
    echo ""
    echo "Note: Firestore data may need manual deletion from the console."
    echo "      APIs are not disabled (may be used by other services)."
}

# Main
case "${1:-all}" in
    -h|--help|help)
        usage
        exit 0
        ;;
    cloudrun)
        check_prerequisites
        confirm_deletion
        delete_cloudrun_services
        ;;
    artifacts)
        check_prerequisites
        confirm_deletion
        delete_artifact_registry
        ;;
    firestore)
        check_prerequisites
        confirm_deletion
        delete_firestore
        ;;
    secrets)
        check_prerequisites
        confirm_deletion
        delete_secrets
        ;;
    iam)
        check_prerequisites
        confirm_deletion
        delete_iam_resources
        ;;
    all)
        check_prerequisites
        echo ""
        echo "Project: $PROJECT"
        echo "Region: $REGION"
        echo ""
        confirm_deletion
        echo ""
        
        delete_cloudrun_services
        echo ""
        delete_artifact_registry
        echo ""
        delete_secrets
        echo ""
        delete_iam_resources
        echo ""
        delete_cloudsql
        echo ""
        delete_pubsub
        echo ""
        delete_cloud_tasks
        echo ""
        delete_logging
        echo ""
        delete_firestore
        echo ""
        print_summary
        ;;
    *)
        echo "Unknown command: $1"
        usage
        exit 1
        ;;
esac

