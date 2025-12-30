#!/bin/bash
set -e

# Agent Exchange - GCP Setup Script
# This script sets up the required GCP resources for deployment

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"


# Configuration
PROJECT="${GCP_PROJECT_ID:-}"
REGION="${GCP_REGION:-us-central1}"
DRY_RUN=false

usage() {
    echo "Agent Exchange - GCP Setup"
    echo ""
    echo "Usage: $0 [command] [options]"
    echo ""
    echo "Commands:"
    echo "  all         Run all setup steps (default)"
    echo "  validate    Validate configuration without creating resources"
    echo ""
    echo "Options:"
    echo "  --dry-run   Show what would be done without making changes"
    echo ""
    echo "Environment variables:"
    echo "  GCP_PROJECT_ID   Google Cloud project ID (required)"
    echo "  GCP_REGION       Google Cloud region (default: us-central1)"
    echo ""
    echo "This script will:"
    echo "  1. Enable required GCP APIs"
    echo "  2. Create Artifact Registry repository"
    echo "  3. Create service accounts"
    echo "  4. Set up Workload Identity for GitHub Actions"
    echo "  5. Create Firestore database"
    echo "  6. Create Secret Manager secrets"
}

# Parse options
parse_options() {
    for arg in "$@"; do
        case $arg in
            --dry-run)
                DRY_RUN=true
                shift
                ;;
        esac
    done
}

validate_gcp_setup() {
    echo "═══════════════════════════════════════════════════════════"
    echo "  GCP Setup Validation"
    echo "═══════════════════════════════════════════════════════════"
    echo ""
    
    local errors=0
    local warnings=0
    
    # Check gcloud CLI
    echo -n "Checking gcloud CLI... "
    if command -v gcloud &> /dev/null; then
        gcloud_version=$(gcloud version --format='value(Google Cloud SDK)' 2>/dev/null | head -1)
        echo "OK (version $gcloud_version)"
    else
        echo "FAILED"
        echo "  gcloud CLI is not installed"
        ((errors++))
    fi
    
    # Check authentication
    echo -n "Checking gcloud authentication... "
    if gcloud auth print-identity-token &> /dev/null; then
        account=$(gcloud config get-value account 2>/dev/null)
        echo "OK"
        echo "  Account: $account"
    else
        echo "FAILED"
        echo "  Not authenticated. Run 'gcloud auth login'"
        ((errors++))
    fi
    
    # Check project ID
    echo -n "Checking GCP Project ID... "
    if [ -z "$PROJECT" ]; then
        PROJECT=$(gcloud config get-value project 2>/dev/null)
    fi
    if [ -n "$PROJECT" ]; then
        if gcloud projects describe "$PROJECT" &> /dev/null; then
            echo "OK ($PROJECT)"
        else
            echo "FAILED"
            echo "  Project '$PROJECT' not found or inaccessible"
            ((errors++))
        fi
    else
        echo "FAILED"
        echo "  GCP_PROJECT_ID not set"
        ((errors++))
    fi
    
    # Check region
    echo -n "Checking GCP Region... "
    if gcloud compute regions describe "$REGION" --project="$PROJECT" &> /dev/null; then
        echo "OK ($REGION)"
    else
        echo "WARNING"
        echo "  Region '$REGION' may not be valid for all services"
        ((warnings++))
    fi
    
    # Check billing
    echo -n "Checking billing account... "
    billing=$(gcloud billing projects describe "$PROJECT" --format='value(billingEnabled)' 2>/dev/null || echo "false")
    if [ "$billing" = "True" ]; then
        echo "OK (billing enabled)"
    else
        echo "WARNING"
        echo "  Billing may not be enabled. Some resources require billing."
        ((warnings++))
    fi
    
    # Check required APIs
    echo ""
    echo "Checking required APIs..."
    apis=(
        "run.googleapis.com:Cloud Run"
        "artifactregistry.googleapis.com:Artifact Registry"
        "firestore.googleapis.com:Firestore"
        "secretmanager.googleapis.com:Secret Manager"
        "iam.googleapis.com:IAM"
    )
    
    for api_info in "${apis[@]}"; do
        api="${api_info%%:*}"
        name="${api_info##*:}"
        echo -n "  $name ($api)... "
        if gcloud services list --enabled --project="$PROJECT" --filter="name:$api" --format='value(name)' 2>/dev/null | grep -q "$api"; then
            echo "ENABLED"
        else
            echo "NOT ENABLED (will be enabled)"
        fi
    done
    
    # Check existing resources
    echo ""
    echo "Checking existing resources..."
    
    echo -n "  Artifact Registry 'aex'... "
    if gcloud artifacts repositories describe aex --location="$REGION" --project="$PROJECT" &> /dev/null; then
        echo "EXISTS"
        ((warnings++))
    else
        echo "WILL CREATE"
    fi
    
    echo -n "  Firestore database... "
    if gcloud firestore databases describe --project="$PROJECT" &> /dev/null; then
        echo "EXISTS"
        ((warnings++))
    else
        echo "WILL CREATE"
    fi
    
    echo -n "  Service account 'aex-cloudrun'... "
    if gcloud iam service-accounts describe "aex-cloudrun@$PROJECT.iam.gserviceaccount.com" --project="$PROJECT" &> /dev/null; then
        echo "EXISTS"
        ((warnings++))
    else
        echo "WILL CREATE"
    fi
    
    # Summary
    echo ""
    echo "═══════════════════════════════════════════════════════════"
    echo "Validation Summary:"
    echo "  Project: $PROJECT"
    echo "  Region: $REGION"
    echo ""
    
    if [ $errors -gt 0 ]; then
        echo "VALIDATION FAILED: $errors error(s)"
        return 1
    elif [ $warnings -gt 0 ]; then
        echo "VALIDATION PASSED with $warnings warning(s)"
        echo "Some resources already exist and will be skipped."
        return 0
    else
        echo "VALIDATION PASSED"
        echo "Ready to create resources."
        return 0
    fi
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
    
    echo "Prerequisites OK"
}

enable_apis() {
    echo "Enabling required APIs..."
    
    apis=(
        "run.googleapis.com"
        "artifactregistry.googleapis.com"
        "firestore.googleapis.com"
        "secretmanager.googleapis.com"
        "cloudresourcemanager.googleapis.com"
        "iam.googleapis.com"
        "iamcredentials.googleapis.com"
    )
    
    for api in "${apis[@]}"; do
        echo "  Enabling $api..."
        gcloud services enable "$api" --project="$PROJECT" --quiet
    done
    
    echo "APIs enabled"
}

create_artifact_registry() {
    echo "Creating Artifact Registry repository..."
    
    if gcloud artifacts repositories describe aex --location="$REGION" --project="$PROJECT" &> /dev/null; then
        echo "  Repository 'aex' already exists"
    else
        gcloud artifacts repositories create aex \
            --repository-format=docker \
            --location="$REGION" \
            --project="$PROJECT" \
            --description="Agent Exchange Docker images"
        echo "Repository created"
    fi
}

create_service_accounts() {
    echo "Creating service accounts..."
    
    # Cloud Run service account
    sa_name="aex-cloudrun"
    sa_email="$sa_name@$PROJECT.iam.gserviceaccount.com"
    
    if gcloud iam service-accounts describe "$sa_email" --project="$PROJECT" &> /dev/null; then
        echo "  Service account '$sa_name' already exists"
    else
        gcloud iam service-accounts create "$sa_name" \
            --display-name="Agent Exchange Cloud Run" \
            --project="$PROJECT"
        echo "  Created service account: $sa_email"
    fi
    
    # Grant roles
    roles=(
        "roles/datastore.user"
        "roles/secretmanager.secretAccessor"
        "roles/logging.logWriter"
        "roles/cloudtrace.agent"
    )
    
    for role in "${roles[@]}"; do
        gcloud projects add-iam-policy-binding "$PROJECT" \
            --member="serviceAccount:$sa_email" \
            --role="$role" \
            --quiet
    done
    
    # GitHub Actions service account
    gh_sa_name="aex-github-actions"
    gh_sa_email="$gh_sa_name@$PROJECT.iam.gserviceaccount.com"
    
    if gcloud iam service-accounts describe "$gh_sa_email" --project="$PROJECT" &> /dev/null; then
        echo "  Service account '$gh_sa_name' already exists"
    else
        gcloud iam service-accounts create "$gh_sa_name" \
            --display-name="Agent Exchange GitHub Actions" \
            --project="$PROJECT"
        echo "  Created service account: $gh_sa_email"
    fi
    
    # Grant roles for GitHub Actions
    gh_roles=(
        "roles/run.admin"
        "roles/artifactregistry.writer"
        "roles/iam.serviceAccountUser"
    )
    
    for role in "${gh_roles[@]}"; do
        gcloud projects add-iam-policy-binding "$PROJECT" \
            --member="serviceAccount:$gh_sa_email" \
            --role="$role" \
            --quiet
    done
    
    echo "Service accounts configured"
}

setup_workload_identity() {
    echo "Setting up Workload Identity for GitHub Actions..."
    
    pool_name="github-actions-pool"
    provider_name="github-actions-provider"
    
    # Create Workload Identity Pool
    if gcloud iam workload-identity-pools describe "$pool_name" --location="global" --project="$PROJECT" &> /dev/null; then
        echo "  Workload Identity Pool already exists"
    else
        gcloud iam workload-identity-pools create "$pool_name" \
            --location="global" \
            --project="$PROJECT" \
            --display-name="GitHub Actions Pool"
    fi
    
    # Create Workload Identity Provider
    if gcloud iam workload-identity-pools providers describe "$provider_name" \
        --workload-identity-pool="$pool_name" \
        --location="global" \
        --project="$PROJECT" &> /dev/null; then
        echo "  Workload Identity Provider already exists"
    else
        gcloud iam workload-identity-pools providers create-oidc "$provider_name" \
            --location="global" \
            --workload-identity-pool="$pool_name" \
            --project="$PROJECT" \
            --display-name="GitHub Actions Provider" \
            --attribute-mapping="google.subject=assertion.sub,attribute.actor=assertion.actor,attribute.repository=assertion.repository" \
            --issuer-uri="https://token.actions.githubusercontent.com"
    fi
    
    # Get the full provider name
    provider_full="projects/$PROJECT/locations/global/workloadIdentityPools/$pool_name/providers/$provider_name"
    
    echo ""
    echo "Workload Identity configured"
    echo ""
    echo "Add these secrets to your GitHub repository:"
    echo "  GCP_PROJECT_ID: $PROJECT"
    echo "  GCP_REGION: $REGION"
    echo "  GCP_WORKLOAD_IDENTITY_PROVIDER: $provider_full"
    echo "  GCP_SERVICE_ACCOUNT: aex-github-actions@$PROJECT.iam.gserviceaccount.com"
}

create_firestore() {
    echo "Creating Firestore database..."
    
    if gcloud firestore databases describe --project="$PROJECT" &> /dev/null; then
        echo "  Firestore database already exists"
    else
        gcloud firestore databases create \
            --project="$PROJECT" \
            --location="$REGION" \
            --type=firestore-native
        echo "Firestore database created"
    fi
}

create_secrets() {
    echo "Creating Secret Manager secrets..."
    
    secrets=(
        "aex-jwt-secret"
        "aex-api-key-salt"
    )
    
    for secret in "${secrets[@]}"; do
        if gcloud secrets describe "$secret" --project="$PROJECT" &> /dev/null; then
            echo "  Secret '$secret' already exists"
        else
            # Generate random value
            value=$(openssl rand -base64 32)
            echo -n "$value" | gcloud secrets create "$secret" \
                --project="$PROJECT" \
                --data-file=-
            echo "  Created secret: $secret"
        fi
    done
    
    echo "Secrets configured"
}

# Main
parse_options "$@"

# Remove --dry-run from args for case matching
CMD="${1:-all}"
if [ "$CMD" = "--dry-run" ]; then
    CMD="${2:-all}"
fi

case "$CMD" in
    -h|--help|help)
        usage
        exit 0
        ;;
    validate|--validate)
        check_prerequisites
        validate_gcp_setup
        exit $?
        ;;
    all)
        check_prerequisites
        
        if [ "$DRY_RUN" = true ]; then
            echo ""
            echo "═══════════════════════════════════════════════════════════"
            echo "  DRY-RUN MODE - No changes will be made"
            echo "═══════════════════════════════════════════════════════════"
            echo ""
            echo "Project: $PROJECT"
            echo "Region: $REGION"
            echo ""
            echo "The following resources would be created:"
            echo "  • Enable 7 GCP APIs (run, artifactregistry, firestore, etc.)"
            echo "  • Artifact Registry repository: aex"
            echo "  • Service accounts: aex-cloudrun, aex-github-actions"
            echo "  • Workload Identity Pool and Provider for GitHub Actions"
            echo "  • Firestore database (native mode)"
            echo "  • 2 Secret Manager secrets"
            echo ""
            echo "Run without --dry-run to create these resources"
        else
            echo ""
            echo "Project: $PROJECT"
            echo "Region: $REGION"
            echo ""

            enable_apis
            echo ""

            create_artifact_registry
            echo ""

            create_service_accounts
            echo ""

            setup_workload_identity
            echo ""

            create_firestore
            echo ""

            create_secrets
            echo ""

            echo "GCP setup complete!"
            echo ""
            echo "Next steps:"
            echo "1. Add the GitHub secrets shown above to your repository"
            echo "2. Grant Workload Identity binding to the GitHub repository"
            echo "3. Push code to trigger the CI/CD pipeline"
        fi
        ;;
    *)
        echo "Unknown command: $CMD"
        usage
        exit 1
        ;;
esac

