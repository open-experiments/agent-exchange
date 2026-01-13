#!/bin/bash
# Generic deployment script for AEX (Agent Exchange)
# Usage: ./deploy.sh <target> [options]
#
# Targets:
#   local   - Run services locally with Docker Compose
#   gcp     - Deploy to Google Cloud Platform (Cloud Run)
#   aws     - Deploy to Amazon Web Services (ECS Fargate)
#
# Examples:
#   ./deploy.sh local
#   ./deploy.sh gcp my-project-id us-central1
#   ./deploy.sh aws us-east-1 aex

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print functions
print_header() {
    echo -e "${BLUE}"
    echo "========================================="
    echo "  AEX - Agent Exchange Deployment"
    echo "========================================="
    echo -e "${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_info() {
    echo -e "${BLUE}ℹ $1${NC}"
}

# Show usage
show_usage() {
    cat << EOF
Usage: $0 <target> [options]

Deployment Targets:
  local                         Run services locally with Docker Compose
  gcp <project-id> [region]     Deploy to Google Cloud Platform
  aws [region] [env-name]       Deploy to Amazon Web Services

Options:
  -h, --help                    Show this help message
  -b, --build-only              Only build images, don't deploy
  -s, --skip-build              Skip building images, deploy existing
  --clean                       Clean up / tear down deployment

Examples:
  $0 local                      # Start local development environment
  $0 local --clean              # Stop local environment
  $0 gcp my-project             # Deploy to GCP (us-central1)
  $0 gcp my-project asia-east1  # Deploy to GCP (asia-east1)
  $0 aws                        # Deploy to AWS (us-east-1, env: aex)
  $0 aws eu-west-1 prod         # Deploy to AWS (eu-west-1, env: prod)

Environment Variables:
  ANTHROPIC_API_KEY             Required for demo agents
  MONGO_URI                     MongoDB connection string (optional)
  JWT_SIGNING_KEY               JWT signing key (optional)

EOF
}

# Check prerequisites
check_prerequisites() {
    local target=$1
    local missing=()

    # Common requirements
    if ! command -v docker &> /dev/null; then
        missing+=("docker")
    fi

    case $target in
        local)
            if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
                missing+=("docker-compose")
            fi
            ;;
        gcp)
            if ! command -v gcloud &> /dev/null; then
                missing+=("gcloud CLI")
            fi
            ;;
        aws)
            if ! command -v aws &> /dev/null; then
                missing+=("aws CLI")
            fi
            ;;
    esac

    if [ ${#missing[@]} -gt 0 ]; then
        print_error "Missing required tools: ${missing[*]}"
        echo ""
        echo "Please install the missing tools and try again."
        exit 1
    fi

    print_success "All prerequisites met"
}

# Local deployment with Docker Compose
deploy_local() {
    local action=${1:-up}

    print_info "Target: Local (Docker Compose)"

    # Check if docker-compose.yaml exists
    if [ ! -f "$ROOT_DIR/docker-compose.yaml" ] && [ ! -f "$ROOT_DIR/docker-compose.yml" ]; then
        print_warning "docker-compose.yaml not found, creating one..."
        create_docker_compose
    fi

    case $action in
        up)
            print_info "Starting local services..."
            cd "$ROOT_DIR"

            # Check for .env file
            if [ ! -f ".env" ]; then
                print_warning "No .env file found. Creating template..."
                create_env_template
                print_warning "Please edit .env with your actual values and run again."
                exit 1
            fi

            if docker compose version &> /dev/null; then
                docker compose up -d --build
            else
                docker-compose up -d --build
            fi

            echo ""
            print_success "Local services started!"
            echo ""
            echo "Services available at:"
            echo "  Demo UI:     http://localhost:8501"
            echo "  AEX Gateway: http://localhost:8080"
            echo ""
            echo "View logs:     docker compose logs -f"
            echo "Stop services: $0 local --clean"
            ;;
        down)
            print_info "Stopping local services..."
            cd "$ROOT_DIR"
            if docker compose version &> /dev/null; then
                docker compose down -v
            else
                docker-compose down -v
            fi
            print_success "Local services stopped"
            ;;
    esac
}

# Create docker-compose.yaml if it doesn't exist
create_docker_compose() {
    cat > "$ROOT_DIR/docker-compose.yaml" << 'COMPOSE_EOF'
version: '3.8'

services:
  # AEX Core Services
  aex-provider-registry:
    build:
      context: ./src
      dockerfile: aex-provider-registry/Dockerfile
    ports:
      - "8085:8085"
    environment:
      - PORT=8085
      - MONGO_URI=${MONGO_URI:-}
    networks:
      - aex-network

  aex-work-publisher:
    build:
      context: ./src
      dockerfile: aex-work-publisher/Dockerfile
    ports:
      - "8081:8081"
    environment:
      - PORT=8081
      - PROVIDER_REGISTRY_URL=http://aex-provider-registry:8085
    depends_on:
      - aex-provider-registry
    networks:
      - aex-network

  aex-bid-gateway:
    build:
      context: ./src
      dockerfile: aex-bid-gateway/Dockerfile
    ports:
      - "8082:8082"
    environment:
      - PORT=8082
      - PROVIDER_REGISTRY_URL=http://aex-provider-registry:8085
    depends_on:
      - aex-provider-registry
    networks:
      - aex-network

  aex-trust-broker:
    build:
      context: ./src
      dockerfile: aex-trust-broker/Dockerfile
    ports:
      - "8088:8088"
    environment:
      - PORT=8088
    networks:
      - aex-network

  aex-bid-evaluator:
    build:
      context: ./src
      dockerfile: aex-bid-evaluator/Dockerfile
    ports:
      - "8083:8083"
    environment:
      - PORT=8083
      - BID_GATEWAY_URL=http://aex-bid-gateway:8082
      - TRUST_BROKER_URL=http://aex-trust-broker:8088
      - WORK_PUBLISHER_URL=http://aex-work-publisher:8081
    depends_on:
      - aex-bid-gateway
      - aex-trust-broker
      - aex-work-publisher
    networks:
      - aex-network

  aex-contract-engine:
    build:
      context: ./src
      dockerfile: aex-contract-engine/Dockerfile
    ports:
      - "8084:8084"
    environment:
      - PORT=8084
      - BID_GATEWAY_URL=http://aex-bid-gateway:8082
      - WORK_PUBLISHER_URL=http://aex-work-publisher:8081
    depends_on:
      - aex-bid-gateway
      - aex-work-publisher
    networks:
      - aex-network

  aex-settlement:
    build:
      context: ./src
      dockerfile: aex-settlement/Dockerfile
    ports:
      - "8086:8086"
    environment:
      - PORT=8086
      - CONTRACT_ENGINE_URL=http://aex-contract-engine:8084
      - TRUST_BROKER_URL=http://aex-trust-broker:8088
    depends_on:
      - aex-contract-engine
      - aex-trust-broker
    networks:
      - aex-network

  aex-identity:
    build:
      context: ./src
      dockerfile: aex-identity/Dockerfile
    ports:
      - "8089:8089"
    environment:
      - PORT=8089
    networks:
      - aex-network

  aex-telemetry:
    build:
      context: ./src
      dockerfile: aex-telemetry/Dockerfile
    ports:
      - "8090:8090"
    environment:
      - PORT=8090
    networks:
      - aex-network

  aex-gateway:
    build:
      context: ./src
      dockerfile: aex-gateway/Dockerfile
    ports:
      - "8080:8080"
    environment:
      - PORT=8080
      - WORK_PUBLISHER_URL=http://aex-work-publisher:8081
      - BID_GATEWAY_URL=http://aex-bid-gateway:8082
      - BID_EVALUATOR_URL=http://aex-bid-evaluator:8083
      - CONTRACT_ENGINE_URL=http://aex-contract-engine:8084
      - SETTLEMENT_URL=http://aex-settlement:8086
      - PROVIDER_REGISTRY_URL=http://aex-provider-registry:8085
      - TRUST_BROKER_URL=http://aex-trust-broker:8088
      - IDENTITY_URL=http://aex-identity:8089
      - TELEMETRY_URL=http://aex-telemetry:8090
    depends_on:
      - aex-work-publisher
      - aex-bid-gateway
      - aex-bid-evaluator
      - aex-contract-engine
      - aex-settlement
      - aex-provider-registry
      - aex-trust-broker
      - aex-identity
      - aex-telemetry
    networks:
      - aex-network

  # Demo Agents
  legal-agent-a:
    build:
      context: ./demo/agents
      dockerfile: Dockerfile
      args:
        - AGENT_DIR=legal-agent-a
    ports:
      - "8100:8100"
    environment:
      - PORT=8100
      - AEX_GATEWAY_URL=http://aex-gateway:8080
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
    depends_on:
      - aex-gateway
    networks:
      - aex-network

  legal-agent-b:
    build:
      context: ./demo/agents
      dockerfile: Dockerfile
      args:
        - AGENT_DIR=legal-agent-b
    ports:
      - "8101:8101"
    environment:
      - PORT=8101
      - AEX_GATEWAY_URL=http://aex-gateway:8080
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
    depends_on:
      - aex-gateway
    networks:
      - aex-network

  legal-agent-c:
    build:
      context: ./demo/agents
      dockerfile: Dockerfile
      args:
        - AGENT_DIR=legal-agent-c
    ports:
      - "8102:8102"
    environment:
      - PORT=8102
      - AEX_GATEWAY_URL=http://aex-gateway:8080
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
    depends_on:
      - aex-gateway
    networks:
      - aex-network

  orchestrator:
    build:
      context: ./demo/agents
      dockerfile: Dockerfile
      args:
        - AGENT_DIR=orchestrator
    ports:
      - "8103:8103"
    environment:
      - PORT=8103
      - AEX_GATEWAY_URL=http://aex-gateway:8080
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
    depends_on:
      - aex-gateway
    networks:
      - aex-network

  demo-ui:
    build:
      context: ./demo/ui
      dockerfile: Dockerfile
    ports:
      - "8501:8501"
    environment:
      - ORCHESTRATOR_URL=http://orchestrator:8103
      - LEGAL_AGENT_A_URL=http://legal-agent-a:8100
      - LEGAL_AGENT_B_URL=http://legal-agent-b:8101
      - LEGAL_AGENT_C_URL=http://legal-agent-c:8102
    depends_on:
      - orchestrator
      - legal-agent-a
      - legal-agent-b
      - legal-agent-c
    networks:
      - aex-network

networks:
  aex-network:
    driver: bridge
COMPOSE_EOF

    print_success "Created docker-compose.yaml"
}

# Create .env template
create_env_template() {
    cat > "$ROOT_DIR/.env" << 'ENV_EOF'
# AEX Environment Configuration
# Copy this file and fill in your actual values

# Required for demo agents (Claude)
ANTHROPIC_API_KEY=sk-ant-your-key-here

# Optional: MongoDB connection string
# MONGO_URI=mongodb+srv://<username>:<password>@<cluster>.mongodb.net/aex

# Optional: JWT signing key
# JWT_SIGNING_KEY=your-secret-key-here
ENV_EOF

    print_success "Created .env template"
}

# GCP deployment
deploy_gcp() {
    local project_id=${1:-}
    local region=${2:-us-central1}
    local action=${3:-deploy}

    if [ -z "$project_id" ] && [ "$action" != "clean" ]; then
        print_error "GCP project ID is required"
        echo "Usage: $0 gcp <project-id> [region]"
        exit 1
    fi

    print_info "Target: Google Cloud Platform"
    print_info "Project: $project_id"
    print_info "Region: $region"

    # Check GCP authentication
    if ! gcloud auth list --filter=status:ACTIVE --format="value(account)" | head -n1 &> /dev/null; then
        print_error "Not authenticated with GCP. Run: gcloud auth login"
        exit 1
    fi

    case $action in
        deploy)
            if [ -f "$SCRIPT_DIR/gcp/deploy.sh" ]; then
                chmod +x "$SCRIPT_DIR/gcp/deploy.sh"
                "$SCRIPT_DIR/gcp/deploy.sh" "$project_id" "$region"
            else
                print_error "GCP deploy script not found at $SCRIPT_DIR/gcp/deploy.sh"
                exit 1
            fi
            ;;
        clean)
            print_info "Cleaning up GCP resources..."
            gcloud config set project "$project_id"

            # Delete Cloud Run services
            for service in demo-ui orchestrator legal-agent-c legal-agent-b legal-agent-a \
                aex-gateway aex-telemetry aex-identity aex-settlement aex-contract-engine \
                aex-bid-evaluator aex-trust-broker aex-bid-gateway aex-work-publisher \
                aex-provider-registry; do
                print_info "Deleting $service..."
                gcloud run services delete $service --region "$region" --quiet 2>/dev/null || true
            done

            print_success "GCP cleanup complete"
            ;;
    esac
}

# AWS deployment
deploy_aws() {
    local region=${1:-us-east-1}
    local env_name=${2:-aex}
    local action=${3:-deploy}

    print_info "Target: Amazon Web Services"
    print_info "Region: $region"
    print_info "Environment: $env_name"

    # Check AWS authentication
    if ! aws sts get-caller-identity &> /dev/null; then
        print_error "Not authenticated with AWS. Run: aws configure"
        exit 1
    fi

    case $action in
        deploy)
            if [ -f "$SCRIPT_DIR/aws/deploy.sh" ]; then
                chmod +x "$SCRIPT_DIR/aws/deploy.sh"
                "$SCRIPT_DIR/aws/deploy.sh" "$region" "$env_name"
            else
                print_error "AWS deploy script not found at $SCRIPT_DIR/aws/deploy.sh"
                exit 1
            fi
            ;;
        clean)
            print_info "Cleaning up AWS resources..."

            # Delete services stack
            print_info "Deleting services stack..."
            aws cloudformation delete-stack \
                --stack-name "${env_name}-services" \
                --region "$region" 2>/dev/null || true

            aws cloudformation wait stack-delete-complete \
                --stack-name "${env_name}-services" \
                --region "$region" 2>/dev/null || true

            # Delete infrastructure stack
            print_info "Deleting infrastructure stack..."
            aws cloudformation delete-stack \
                --stack-name "${env_name}-infrastructure" \
                --region "$region" 2>/dev/null || true

            aws cloudformation wait stack-delete-complete \
                --stack-name "${env_name}-infrastructure" \
                --region "$region" 2>/dev/null || true

            print_success "AWS cleanup complete"
            print_warning "ECR repositories with images were not deleted. Delete manually if needed."
            ;;
    esac
}

# Main entry point
main() {
    print_header

    # Parse arguments
    local target=""
    local clean=false
    local build_only=false
    local skip_build=false
    local extra_args=()

    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_usage
                exit 0
                ;;
            --clean)
                clean=true
                shift
                ;;
            -b|--build-only)
                build_only=true
                shift
                ;;
            -s|--skip-build)
                skip_build=true
                shift
                ;;
            local|gcp|aws)
                target=$1
                shift
                # Collect remaining args
                while [[ $# -gt 0 ]] && [[ ! $1 =~ ^- ]]; do
                    extra_args+=("$1")
                    shift
                done
                ;;
            *)
                print_error "Unknown option: $1"
                show_usage
                exit 1
                ;;
        esac
    done

    # Validate target
    if [ -z "$target" ]; then
        print_error "Deployment target is required"
        echo ""
        show_usage
        exit 1
    fi

    # Check prerequisites
    check_prerequisites "$target"

    # Determine action
    local action="deploy"
    if [ "$clean" = true ]; then
        action="clean"
    fi

    # Execute deployment
    case $target in
        local)
            if [ "$clean" = true ]; then
                deploy_local "down"
            else
                deploy_local "up"
            fi
            ;;
        gcp)
            deploy_gcp "${extra_args[0]:-}" "${extra_args[1]:-us-central1}" "$action"
            ;;
        aws)
            deploy_aws "${extra_args[0]:-us-east-1}" "${extra_args[1]:-aex}" "$action"
            ;;
    esac

    echo ""
    print_success "Done!"
}

# Run main
main "$@"
