#!/bin/bash
set -e

# Agent Exchange - ECS/Fargate Deployment Script
# Usage: ./deploy-ecs.sh [environment] [service]
# Example: ./deploy-ecs.sh staging aex-gateway
# Example: ./deploy-ecs.sh production all

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"


# Default values
ENVIRONMENT="${1:-staging}"
SERVICE="${2:-all}"
AWS_REGION="${AWS_REGION:-us-east-1}"
AWS_ACCOUNT_ID="${AWS_ACCOUNT_ID:-}"
CLUSTER_NAME="${CLUSTER_NAME:-aex-cluster}"

# Service configuration: port:cpu:memory:min:max
declare -A SERVICE_CONFIG=(
    ["aex-gateway"]="8080:512:1024:1:10"
    ["aex-work-publisher"]="8080:256:512:0:5"
    ["aex-bid-gateway"]="8080:256:512:0:5"
    ["aex-bid-evaluator"]="8080:256:512:0:5"
    ["aex-contract-engine"]="8080:256:512:0:5"
    ["aex-provider-registry"]="8080:256:512:0:5"
    ["aex-trust-broker"]="8080:256:512:0:5"
    ["aex-identity"]="8080:256:512:0:5"
    ["aex-settlement"]="8080:256:512:0:5"
    ["aex-telemetry"]="8080:256:512:0:3"
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
    echo "Agent Exchange - ECS/Fargate Deployment"
    echo ""
    echo "Usage: $0 [environment] [service]"
    echo ""
    echo "Arguments:"
    echo "  environment   staging or production (default: staging)"
    echo "  service       Service name or 'all' (default: all)"
    echo ""
    echo "Environment variables:"
    echo "  AWS_REGION      AWS region (default: us-east-1)"
    echo "  AWS_ACCOUNT_ID  AWS account ID (required)"
    echo "  CLUSTER_NAME    ECS cluster name (default: aex-cluster)"
    echo "  VERSION         Image version tag (default: latest)"
    echo ""
    echo "Examples:"
    echo "  $0 staging all"
    echo "  $0 production aex-gateway"
    echo "  VERSION=v1.0.0 $0 production all"
}

check_prerequisites() {
    echo "Checking prerequisites..."
    
    if [ -z "$AWS_ACCOUNT_ID" ]; then
        AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text 2>/dev/null || true)
        if [ -z "$AWS_ACCOUNT_ID" ]; then
            echo "Error: AWS_ACCOUNT_ID environment variable is required"
            exit 1
        fi
    fi
    
    if ! command -v aws &> /dev/null; then
        echo "Error: AWS CLI is not installed"
        exit 1
    fi
    
    if ! aws sts get-caller-identity &> /dev/null; then
        echo "Error: Not authenticated with AWS"
        exit 1
    fi
    
    echo "Prerequisites OK"
}

get_service_config() {
    local service=$1
    local config="${SERVICE_CONFIG[$service]}"
    
    if [ -z "$config" ]; then
        echo "8080:256:512:0:5"
    else
        echo "$config"
    fi
}

get_vpc_config() {
    # Get VPC ID
    VPC_ID=$(aws ec2 describe-vpcs \
        --filters "Name=tag:Name,Values=aex-vpc" \
        --region "$AWS_REGION" \
        --query 'Vpcs[0].VpcId' \
        --output text)
    
    # Get private subnets
    PRIVATE_SUBNETS=$(aws ec2 describe-subnets \
        --filters "Name=tag:Name,Values=aex-private-*" "Name=vpc-id,Values=$VPC_ID" \
        --region "$AWS_REGION" \
        --query 'Subnets[*].SubnetId' \
        --output text | tr '\t' ',')
    
    # Get ECS security group
    ECS_SG_ID=$(aws ec2 describe-security-groups \
        --filters "Name=group-name,Values=aex-ecs-sg" "Name=vpc-id,Values=$VPC_ID" \
        --region "$AWS_REGION" \
        --query 'SecurityGroups[0].GroupId' \
        --output text)
}

get_service_urls() {
    local env=$1
    local suffix=""
    if [ "$env" = "staging" ]; then
        suffix="-staging"
    fi
    
    # Get service discovery namespace or ALB URLs
    ALB_DNS=$(aws elbv2 describe-load-balancers --names "aex-alb" --region "$AWS_REGION" \
        --query 'LoadBalancers[0].DNSName' --output text 2>/dev/null || echo "localhost")
    
    echo "http://$ALB_DNS"
}

create_or_update_service() {
    local service=$1
    local env=$2
    local version="${VERSION:-latest}"
    
    local config=$(get_service_config "$service")
    IFS=':' read -r port cpu memory min_count max_count <<< "$config"
    
    local service_name="$service"
    if [ "$env" = "staging" ]; then
        service_name="$service-staging"
    fi
    
    local image="$AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com/agent-exchange/$service:$version"
    local cluster="$CLUSTER_NAME"
    if [ "$env" = "staging" ]; then
        cluster="$CLUSTER_NAME-staging"
    fi
    
    echo "Deploying $service to $env..."
    echo "  Image: $image"
    echo "  CPU: $cpu, Memory: $memory"
    echo "  Desired count: $min_count"
    
    # Build environment variables JSON
    local base_url=$(get_service_urls "$env")
    local env_vars="["
    env_vars+="{\"name\":\"ENVIRONMENT\",\"value\":\"$env\"},"
    env_vars+="{\"name\":\"PORT\",\"value\":\"$port\"},"
    env_vars+="{\"name\":\"AWS_REGION\",\"value\":\"$AWS_REGION\"}"
    
    # Service-specific environment variables
    case $service in
        aex-gateway)
            env_vars+=",{\"name\":\"WORK_PUBLISHER_URL\",\"value\":\"http://aex-work-publisher.$service_name.local:8080\"}"
            env_vars+=",{\"name\":\"BID_GATEWAY_URL\",\"value\":\"http://aex-bid-gateway.$service_name.local:8080\"}"
            env_vars+=",{\"name\":\"PROVIDER_REGISTRY_URL\",\"value\":\"http://aex-provider-registry.$service_name.local:8080\"}"
            env_vars+=",{\"name\":\"SETTLEMENT_URL\",\"value\":\"http://aex-settlement.$service_name.local:8080\"}"
            env_vars+=",{\"name\":\"IDENTITY_URL\",\"value\":\"http://aex-identity.$service_name.local:8080\"}"
            ;;
        aex-work-publisher)
            env_vars+=",{\"name\":\"STORE_TYPE\",\"value\":\"mongo\"}"
            env_vars+=",{\"name\":\"PROVIDER_REGISTRY_URL\",\"value\":\"http://aex-provider-registry.$service_name.local:8080\"}"
            ;;
        aex-bid-gateway)
            env_vars+=",{\"name\":\"PROVIDER_REGISTRY_URL\",\"value\":\"http://aex-provider-registry.$service_name.local:8080\"}"
            ;;
        aex-bid-evaluator)
            env_vars+=",{\"name\":\"BID_GATEWAY_URL\",\"value\":\"http://aex-bid-gateway.$service_name.local:8080\"}"
            env_vars+=",{\"name\":\"TRUST_BROKER_URL\",\"value\":\"http://aex-trust-broker.$service_name.local:8080\"}"
            ;;
        aex-contract-engine)
            env_vars+=",{\"name\":\"BID_GATEWAY_URL\",\"value\":\"http://aex-bid-gateway.$service_name.local:8080\"}"
            env_vars+=",{\"name\":\"SETTLEMENT_URL\",\"value\":\"http://aex-settlement.$service_name.local:8080\"}"
            ;;
    esac
    
    env_vars+="]"
    
    # Create task definition
    local task_def=$(cat << EOF
{
    "family": "$service_name",
    "networkMode": "awsvpc",
    "requiresCompatibilities": ["FARGATE"],
    "cpu": "$cpu",
    "memory": "$memory",
    "executionRoleArn": "arn:aws:iam::$AWS_ACCOUNT_ID:role/aex-ecs-execution-role",
    "taskRoleArn": "arn:aws:iam::$AWS_ACCOUNT_ID:role/aex-ecs-task-role",
    "containerDefinitions": [
        {
            "name": "$service",
            "image": "$image",
            "portMappings": [
                {
                    "containerPort": $port,
                    "protocol": "tcp"
                }
            ],
            "environment": $env_vars,
            "logConfiguration": {
                "logDriver": "awslogs",
                "options": {
                    "awslogs-group": "/ecs/agent-exchange/$service",
                    "awslogs-region": "$AWS_REGION",
                    "awslogs-stream-prefix": "ecs"
                }
            },
            "healthCheck": {
                "command": ["CMD-SHELL", "curl -f http://localhost:$port/health || exit 1"],
                "interval": 30,
                "timeout": 5,
                "retries": 3,
                "startPeriod": 60
            },
            "essential": true
        }
    ]
}
EOF
)
    
    # Register task definition
    echo "$task_def" > /tmp/task-def-$service.json
    
    local task_def_arn=$(aws ecs register-task-definition \
        --cli-input-json file:///tmp/task-def-$service.json \
        --region "$AWS_REGION" \
        --query 'taskDefinition.taskDefinitionArn' \
        --output text)
    
    echo "  Task definition: $task_def_arn"
    
    # Check if service exists
    local service_exists=$(aws ecs describe-services \
        --cluster "$cluster" \
        --services "$service_name" \
        --region "$AWS_REGION" \
        --query 'services[?status==`ACTIVE`].serviceName' \
        --output text 2>/dev/null || echo "")
    
    get_vpc_config
    
    if [ -n "$service_exists" ]; then
        # Update existing service
        echo "  Updating existing service..."
        aws ecs update-service \
            --cluster "$cluster" \
            --service "$service_name" \
            --task-definition "$task_def_arn" \
            --desired-count "$min_count" \
            --region "$AWS_REGION" \
            --output text > /dev/null
    else
        # Create new service
        echo "  Creating new service..."
        
        # Create target group for this service
        local tg_name="$service_name-tg"
        local tg_arn=""
        
        if aws elbv2 describe-target-groups --names "$tg_name" --region "$AWS_REGION" &> /dev/null; then
            tg_arn=$(aws elbv2 describe-target-groups --names "$tg_name" --region "$AWS_REGION" \
                --query 'TargetGroups[0].TargetGroupArn' --output text)
        else
            tg_arn=$(aws elbv2 create-target-group \
                --name "$tg_name" \
                --protocol HTTP \
                --port "$port" \
                --vpc-id "$VPC_ID" \
                --target-type ip \
                --health-check-path "/health" \
                --health-check-interval-seconds 30 \
                --region "$AWS_REGION" \
                --query 'TargetGroups[0].TargetGroupArn' \
                --output text)
        fi
        
        aws ecs create-service \
            --cluster "$cluster" \
            --service-name "$service_name" \
            --task-definition "$task_def_arn" \
            --desired-count "$min_count" \
            --launch-type FARGATE \
            --network-configuration "awsvpcConfiguration={subnets=[${PRIVATE_SUBNETS//,/,}],securityGroups=[$ECS_SG_ID],assignPublicIp=DISABLED}" \
            --load-balancers "targetGroupArn=$tg_arn,containerName=$service,containerPort=$port" \
            --region "$AWS_REGION" \
            --output text > /dev/null
    fi
    
    rm -f /tmp/task-def-$service.json
    
    echo "Deployed: $service_name"
}

wait_for_deployment() {
    local service=$1
    local env=$2
    local cluster="$CLUSTER_NAME"
    
    if [ "$env" = "staging" ]; then
        cluster="$CLUSTER_NAME-staging"
    fi
    
    local service_name="$service"
    if [ "$env" = "staging" ]; then
        service_name="$service-staging"
    fi
    
    echo "Waiting for $service_name deployment to stabilize..."
    
    aws ecs wait services-stable \
        --cluster "$cluster" \
        --services "$service_name" \
        --region "$AWS_REGION" 2>/dev/null || true
    
    echo "$service_name is stable"
}

deploy_service() {
    local service=$1
    local env=$2
    
    create_or_update_service "$service" "$env"
}

deploy_all() {
    local env=$1
    
    echo "Deploying all services to $env..."
    
    for service in "${SERVICES[@]}"; do
        deploy_service "$service" "$env"
        echo ""
    done
    
    echo "All services deployed!"
    echo ""
    echo "Waiting for services to stabilize..."
    
    for service in "${SERVICES[@]}"; do
        wait_for_deployment "$service" "$env" &
    done
    wait
    
    echo "All services stable!"
}

build_and_push() {
    local version="${VERSION:-latest}"
    
    echo "Building and pushing Docker images..."
    
    # Login to ECR
    aws ecr get-login-password --region "$AWS_REGION" | \
        docker login --username AWS --password-stdin "$AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com"
    
    for service in "${SERVICES[@]}"; do
        echo "  Building $service..."
        local image="$AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com/agent-exchange/$service"
        
        docker build \
            -f "$PROJECT_ROOT/src/$service/Dockerfile" \
            -t "$image:$version" \
            -t "$image:latest" \
            "$PROJECT_ROOT/src/"
        
        echo "  Pushing $service..."
        docker push "$image:$version"
        docker push "$image:latest"
    done
    
    echo "All images built and pushed!"
}

# Main
case "$1" in
    -h|--help|help)
        usage
        exit 0
        ;;
    build)
        check_prerequisites
        build_and_push
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
echo "Region: $AWS_REGION"
echo "Account: $AWS_ACCOUNT_ID"
echo "Cluster: $CLUSTER_NAME"
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

# Get ALB URL
ALB_DNS=$(aws elbv2 describe-load-balancers --names "aex-alb" --region "$AWS_REGION" \
    --query 'LoadBalancers[0].DNSName' --output text 2>/dev/null || echo "")

if [ -n "$ALB_DNS" ]; then
    echo ""
    echo "Access your services at:"
    echo "  http://$ALB_DNS"
fi

