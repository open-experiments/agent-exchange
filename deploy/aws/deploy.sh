#!/bin/bash
# Deploy AEX to AWS using CloudFormation and ECS Fargate
# Usage: ./deploy.sh <aws-region>

set -euo pipefail

REGION="${1:-us-east-1}"
ENVIRONMENT_NAME="${2:-aex}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "========================================="
echo "  AEX AWS Deployment"
echo "========================================="
echo "Region: $REGION"
echo "Environment: $ENVIRONMENT_NAME"
echo ""

# Check AWS CLI is installed
if ! command -v aws &> /dev/null; then
    echo "Error: AWS CLI is not installed. Please install it first."
    exit 1
fi

# Check AWS credentials
echo "Checking AWS credentials..."
AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
if [ -z "$AWS_ACCOUNT_ID" ]; then
    echo "Error: Unable to get AWS account ID. Please configure AWS credentials."
    exit 1
fi
echo "AWS Account ID: $AWS_ACCOUNT_ID"

# Deploy infrastructure stack
echo ""
echo "Step 1: Deploying infrastructure stack..."
aws cloudformation deploy \
    --template-file "$SCRIPT_DIR/infrastructure.yaml" \
    --stack-name "${ENVIRONMENT_NAME}-infrastructure" \
    --parameter-overrides EnvironmentName=$ENVIRONMENT_NAME \
    --capabilities CAPABILITY_NAMED_IAM \
    --region $REGION \
    --no-fail-on-empty-changeset

echo "Infrastructure stack deployed successfully."

# Get ECR registry URL
ECR_REGISTRY="$AWS_ACCOUNT_ID.dkr.ecr.$REGION.amazonaws.com"

# Build and push images
echo ""
echo "Step 2: Building and pushing Docker images..."
echo "Logging into ECR..."
aws ecr get-login-password --region $REGION | docker login --username AWS --password-stdin $ECR_REGISTRY

# Get commit hash for tagging
COMMIT_HASH=$(git rev-parse --short HEAD 2>/dev/null || echo "latest")
IMAGE_TAG=$COMMIT_HASH

echo "Building images with tag: $IMAGE_TAG"

# Setup docker buildx for multi-platform builds (required for ARM Mac -> AMD64 ECS)
echo "Setting up Docker buildx for linux/amd64 builds..."
docker buildx create --name aex-builder --use 2>/dev/null || docker buildx use aex-builder

# Build AEX core services
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
)

cd "$SCRIPT_DIR/../.."

for service in "${SERVICES[@]}"; do
    echo "Building $service..."
    docker buildx build --platform linux/amd64 \
        -t "$ECR_REGISTRY/$ENVIRONMENT_NAME/$service:$IMAGE_TAG" \
        -t "$ECR_REGISTRY/$ENVIRONMENT_NAME/$service:latest" \
        -f "src/$service/Dockerfile" src/ \
        --push
done

# Build demo agents
DEMO_AGENTS=("legal-agent-a" "legal-agent-b" "legal-agent-c" "orchestrator")

for agent in "${DEMO_AGENTS[@]}"; do
    echo "Building $agent..."
    docker buildx build --platform linux/amd64 \
        -t "$ECR_REGISTRY/$ENVIRONMENT_NAME/$agent:$IMAGE_TAG" \
        -t "$ECR_REGISTRY/$ENVIRONMENT_NAME/$agent:latest" \
        --build-arg AGENT_DIR=$agent \
        -f demo/agents/Dockerfile demo/agents/ \
        --push
done

# Build demo UI
echo "Building demo-ui..."
docker buildx build --platform linux/amd64 \
    -t "$ECR_REGISTRY/$ENVIRONMENT_NAME/demo-ui:$IMAGE_TAG" \
    -t "$ECR_REGISTRY/$ENVIRONMENT_NAME/demo-ui:latest" \
    -f demo/ui/Dockerfile demo/ui/ \
    --push

echo "All images built and pushed successfully."

# Deploy services stack
echo ""
echo "Step 3: Deploying services stack..."
aws cloudformation deploy \
    --template-file "$SCRIPT_DIR/services.yaml" \
    --stack-name "${ENVIRONMENT_NAME}-services" \
    --parameter-overrides \
        EnvironmentName=$ENVIRONMENT_NAME \
        ImageTag=$IMAGE_TAG \
    --region $REGION \
    --no-fail-on-empty-changeset

echo "Services stack deployed successfully."

# Get outputs
echo ""
echo "========================================="
echo "  Deployment Complete!"
echo "========================================="
echo ""

ALB_DNS=$(aws cloudformation describe-stacks \
    --stack-name "${ENVIRONMENT_NAME}-infrastructure" \
    --query "Stacks[0].Outputs[?OutputKey=='ALBDNSName'].OutputValue" \
    --output text \
    --region $REGION)

echo "Application URL: http://$ALB_DNS"
echo ""
echo "Demo UI: http://$ALB_DNS/"
echo "API Gateway: http://$ALB_DNS/api/"
echo ""
echo "========================================="
echo ""
echo "Next Steps:"
echo ""
echo "1. Update secrets with actual values:"
echo "   aws secretsmanager update-secret \\"
echo "     --secret-id ${ENVIRONMENT_NAME}/anthropic-api-key \\"
echo "     --secret-string '{\"api_key\":\"sk-ant-...\"}' \\"
echo "     --region $REGION"
echo ""
echo "   aws secretsmanager update-secret \\"
echo "     --secret-id ${ENVIRONMENT_NAME}/mongo-uri \\"
echo "     --secret-string '{\"uri\":\"mongodb+srv://<USERNAME>:<PASSWORD>@<CLUSTER>.mongodb.net/aex\"}' \\"
echo "     --region $REGION"
echo ""
echo "2. Force new deployment to pick up secrets:"
echo "   aws ecs update-service --cluster ${ENVIRONMENT_NAME}-cluster \\"
echo "     --service gateway --force-new-deployment --region $REGION"
echo ""
echo "3. Monitor services:"
echo "   aws ecs list-services --cluster ${ENVIRONMENT_NAME}-cluster --region $REGION"
echo ""
echo "4. View logs:"
echo "   aws logs tail /ecs/${ENVIRONMENT_NAME} --follow --region $REGION"
echo ""
