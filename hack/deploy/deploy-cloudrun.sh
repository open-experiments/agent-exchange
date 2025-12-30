#!/bin/bash
set -e

# Agent Exchange - Cloud Run Deployment Script
# Usage: ./deploy-cloudrun.sh [environment] [service]
# Example: ./deploy-cloudrun.sh staging aex-gateway
# Example: ./deploy-cloudrun.sh production all

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"


# Default values
ENVIRONMENT="${1:-staging}"
SERVICE="${2:-all}"
REGION="${GCP_REGION:-us-central1}"
PROJECT="${GCP_PROJECT_ID:-}"

# Service configuration
declare -A SERVICE_CONFIG=(
    ["aex-gateway"]="8080:1Gi:2:1:100"
    ["aex-work-publisher"]="8080:512Mi:1:0:10"
    ["aex-bid-gateway"]="8080:512Mi:1:0:10"
    ["aex-bid-evaluator"]="8080:512Mi:1:0:10"
    ["aex-contract-engine"]="8080:512Mi:1:0:10"
    ["aex-provider-registry"]="8080:512Mi:1:0:10"
    ["aex-trust-broker"]="8080:512Mi:1:0:10"
    ["aex-identity"]="8080:512Mi:1:0:10"
    ["aex-settlement"]="8080:512Mi:1:0:10"
    ["aex-telemetry"]="8080:256Mi:1:0:5"
)

SERVICES=(
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
)

usage() {
    echo "Agent Exchange - Cloud Run Deployment"
    echo ""
    echo "Usage: $0 [environment] [service]"
    echo ""
    echo "Arguments:"
    echo "  environment   staging or production (default: staging)"
    echo "  service       Service name or 'all' (default: all)"
    echo ""
    echo "Environment variables:"
    echo "  GCP_PROJECT_ID   Google Cloud project ID (required)"
    echo "  GCP_REGION       Google Cloud region (default: us-central1)"
    echo "  VERSION          Image version tag (default: latest)"
    echo ""
    echo "Examples:"
    echo "  $0 staging all"
    echo "  $0 production aex-gateway"
    echo "  VERSION=v1.0.0 $0 production all"
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
    
    # Check if authenticated
    if ! gcloud auth print-identity-token &> /dev/null; then
        echo "Error: Not authenticated with gcloud. Run 'gcloud auth login'"
        exit 1
    fi
    
    echo "Prerequisites OK"
}

get_service_config() {
    local service=$1
    local config="${SERVICE_CONFIG[$service]}"
    
    if [ -z "$config" ]; then
        echo "8080:512Mi:1:0:10"
    else
        echo "$config"
    fi
}

deploy_service() {
    local service=$1
    local env=$2
    local version="${VERSION:-latest}"
    
    local config=$(get_service_config "$service")
    IFS=':' read -r port memory cpu min_instances max_instances <<< "$config"
    
    local service_name="$service"
    if [ "$env" = "staging" ]; then
        service_name="$service-staging"
    fi
    
    local image="$REGION-docker.pkg.dev/$PROJECT/aex/$service:$version"
    
    echo "Deploying $service to $env..."
    echo "  Image: $image"
    echo "  Memory: $memory, CPU: $cpu"
    echo "  Instances: $min_instances - $max_instances"
    
    # Build environment variables
    local env_vars="ENVIRONMENT=$env"
    env_vars+=",PORT=$port"
    
    # Service-specific environment variables
    case $service in
        aex-gateway)
            env_vars+=",WORK_PUBLISHER_URL=https://aex-work-publisher-$PROJECT.run.app"
            env_vars+=",BID_GATEWAY_URL=https://aex-bid-gateway-$PROJECT.run.app"
            env_vars+=",PROVIDER_REGISTRY_URL=https://aex-provider-registry-$PROJECT.run.app"
            env_vars+=",SETTLEMENT_URL=https://aex-settlement-$PROJECT.run.app"
            env_vars+=",IDENTITY_URL=https://aex-identity-$PROJECT.run.app"
            ;;
        aex-work-publisher)
            env_vars+=",STORE_TYPE=firestore"
            env_vars+=",PROVIDER_REGISTRY_URL=https://aex-provider-registry-$PROJECT.run.app"
            ;;
        aex-bid-evaluator)
            env_vars+=",BID_GATEWAY_URL=https://aex-bid-gateway-$PROJECT.run.app"
            env_vars+=",TRUST_BROKER_URL=https://aex-trust-broker-$PROJECT.run.app"
            ;;
        aex-contract-engine)
            env_vars+=",BID_GATEWAY_URL=https://aex-bid-gateway-$PROJECT.run.app"
            env_vars+=",SETTLEMENT_URL=https://aex-settlement-$PROJECT.run.app"
            ;;
    esac
    
    # Determine authentication
    local auth_flag="--no-allow-unauthenticated"
    if [ "$env" = "staging" ]; then
        auth_flag="--allow-unauthenticated"
    fi
    
    # Deploy
    gcloud run deploy "$service_name" \
        --image "$image" \
        --region "$REGION" \
        --project "$PROJECT" \
        --platform managed \
        $auth_flag \
        --set-env-vars "$env_vars" \
        --min-instances "$min_instances" \
        --max-instances "$max_instances" \
        --memory "$memory" \
        --cpu "$cpu" \
        --port "$port" \
        --quiet
    
    local url=$(gcloud run services describe "$service_name" \
        --region "$REGION" \
        --project "$PROJECT" \
        --format 'value(status.url)')
    
    echo "Deployed: $url"
}

deploy_all() {
    local env=$1
    
    echo "Deploying all services to $env..."
    
    for service in "${SERVICES[@]}"; do
        deploy_service "$service" "$env"
        echo ""
    done
    
    echo "All services deployed!"
}

# Main
case "$1" in
    -h|--help|help)
        usage
        exit 0
        ;;
esac

if [[ ! "$ENVIRONMENT" =~ ^(staging|production)$ ]]; then
    echo "Error: Invalid environment '$ENVIRONMENT'. Use 'staging' or 'production'."
    exit 1
fi

check_prerequisites

echo ""
echo "Environment: $ENVIRONMENT"
echo "Project: $PROJECT"
echo "Region: $REGION"
echo ""

if [ "$SERVICE" = "all" ]; then
    deploy_all "$ENVIRONMENT"
else
    if [[ ! " ${SERVICES[*]} " =~ " ${SERVICE} " ]]; then
        echo "Error: Unknown service '$SERVICE'"
        echo "Available services: ${SERVICES[*]}"
        exit 1
    fi
    deploy_service "$SERVICE" "$ENVIRONMENT"
fi

echo ""
echo "Deployment complete!"

