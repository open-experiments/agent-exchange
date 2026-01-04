#!/bin/bash
# Deploy AEX and demo agents to GCP Cloud Run
# Usage: ./deploy.sh <project-id> <region>

set -euo pipefail

PROJECT_ID="${1:-}"
REGION="${2:-us-central1}"

if [ -z "$PROJECT_ID" ]; then
    echo "Usage: $0 <project-id> [region]"
    echo "Example: $0 my-gcp-project us-central1"
    exit 1
fi

echo "Deploying to project: $PROJECT_ID, region: $REGION"

# Set project
gcloud config set project "$PROJECT_ID"

# Enable required APIs
echo "Enabling required APIs..."
gcloud services enable \
    cloudbuild.googleapis.com \
    run.googleapis.com \
    secretmanager.googleapis.com \
    firestore.googleapis.com \
    --quiet

# Create secrets if they don't exist
echo "Setting up secrets..."
for secret in ANTHROPIC_API_KEY MONGO_URI JWT_SIGNING_KEY; do
    if ! gcloud secrets describe "$secret" --quiet 2>/dev/null; then
        echo "Creating secret: $secret"
        echo "placeholder" | gcloud secrets create "$secret" --data-file=- --quiet
        echo "⚠️  Please update secret $secret with actual value:"
        echo "   gcloud secrets versions add $secret --data-file=-"
    fi
done

# Build images using Cloud Build
echo "Building images..."
gcloud builds submit --config=deploy/gcp/cloudbuild.yaml .

# Get the latest image tag
COMMIT_SHA=$(git rev-parse HEAD)

# Deploy AEX core services
echo "Deploying AEX services..."

# Provider Registry
gcloud run deploy aex-provider-registry \
    --image "gcr.io/$PROJECT_ID/aex-provider-registry:$COMMIT_SHA" \
    --region "$REGION" \
    --platform managed \
    --allow-unauthenticated \
    --set-env-vars "MONGO_URI=projects/$PROJECT_ID/secrets/MONGO_URI:latest" \
    --memory 512Mi \
    --cpu 1 \
    --min-instances 0 \
    --max-instances 10 \
    --quiet

PROVIDER_REGISTRY_URL=$(gcloud run services describe aex-provider-registry --region "$REGION" --format 'value(status.url)')

# Work Publisher
gcloud run deploy aex-work-publisher \
    --image "gcr.io/$PROJECT_ID/aex-work-publisher:$COMMIT_SHA" \
    --region "$REGION" \
    --platform managed \
    --allow-unauthenticated \
    --set-env-vars "PROVIDER_REGISTRY_URL=$PROVIDER_REGISTRY_URL" \
    --memory 512Mi \
    --cpu 1 \
    --min-instances 0 \
    --max-instances 10 \
    --quiet

WORK_PUBLISHER_URL=$(gcloud run services describe aex-work-publisher --region "$REGION" --format 'value(status.url)')

# Bid Gateway
gcloud run deploy aex-bid-gateway \
    --image "gcr.io/$PROJECT_ID/aex-bid-gateway:$COMMIT_SHA" \
    --region "$REGION" \
    --platform managed \
    --allow-unauthenticated \
    --set-env-vars "PROVIDER_REGISTRY_URL=$PROVIDER_REGISTRY_URL" \
    --memory 512Mi \
    --cpu 1 \
    --min-instances 0 \
    --max-instances 10 \
    --quiet

BID_GATEWAY_URL=$(gcloud run services describe aex-bid-gateway --region "$REGION" --format 'value(status.url)')

# Trust Broker
gcloud run deploy aex-trust-broker \
    --image "gcr.io/$PROJECT_ID/aex-trust-broker:$COMMIT_SHA" \
    --region "$REGION" \
    --platform managed \
    --allow-unauthenticated \
    --memory 512Mi \
    --cpu 1 \
    --min-instances 0 \
    --max-instances 5 \
    --quiet

TRUST_BROKER_URL=$(gcloud run services describe aex-trust-broker --region "$REGION" --format 'value(status.url)')

# Bid Evaluator
gcloud run deploy aex-bid-evaluator \
    --image "gcr.io/$PROJECT_ID/aex-bid-evaluator:$COMMIT_SHA" \
    --region "$REGION" \
    --platform managed \
    --allow-unauthenticated \
    --set-env-vars "BID_GATEWAY_URL=$BID_GATEWAY_URL,TRUST_BROKER_URL=$TRUST_BROKER_URL,WORK_PUBLISHER_URL=$WORK_PUBLISHER_URL" \
    --memory 512Mi \
    --cpu 1 \
    --min-instances 0 \
    --max-instances 10 \
    --quiet

BID_EVALUATOR_URL=$(gcloud run services describe aex-bid-evaluator --region "$REGION" --format 'value(status.url)')

# Contract Engine
gcloud run deploy aex-contract-engine \
    --image "gcr.io/$PROJECT_ID/aex-contract-engine:$COMMIT_SHA" \
    --region "$REGION" \
    --platform managed \
    --allow-unauthenticated \
    --set-env-vars "BID_GATEWAY_URL=$BID_GATEWAY_URL,WORK_PUBLISHER_URL=$WORK_PUBLISHER_URL" \
    --memory 512Mi \
    --cpu 1 \
    --min-instances 0 \
    --max-instances 10 \
    --quiet

CONTRACT_ENGINE_URL=$(gcloud run services describe aex-contract-engine --region "$REGION" --format 'value(status.url)')

# Settlement
gcloud run deploy aex-settlement \
    --image "gcr.io/$PROJECT_ID/aex-settlement:$COMMIT_SHA" \
    --region "$REGION" \
    --platform managed \
    --allow-unauthenticated \
    --set-env-vars "CONTRACT_ENGINE_URL=$CONTRACT_ENGINE_URL,TRUST_BROKER_URL=$TRUST_BROKER_URL" \
    --memory 512Mi \
    --cpu 1 \
    --min-instances 0 \
    --max-instances 5 \
    --quiet

SETTLEMENT_URL=$(gcloud run services describe aex-settlement --region "$REGION" --format 'value(status.url)')

# Identity
gcloud run deploy aex-identity \
    --image "gcr.io/$PROJECT_ID/aex-identity:$COMMIT_SHA" \
    --region "$REGION" \
    --platform managed \
    --allow-unauthenticated \
    --memory 512Mi \
    --cpu 1 \
    --min-instances 0 \
    --max-instances 5 \
    --quiet

IDENTITY_URL=$(gcloud run services describe aex-identity --region "$REGION" --format 'value(status.url)')

# Telemetry
gcloud run deploy aex-telemetry \
    --image "gcr.io/$PROJECT_ID/aex-telemetry:$COMMIT_SHA" \
    --region "$REGION" \
    --platform managed \
    --allow-unauthenticated \
    --memory 512Mi \
    --cpu 1 \
    --min-instances 0 \
    --max-instances 5 \
    --quiet

TELEMETRY_URL=$(gcloud run services describe aex-telemetry --region "$REGION" --format 'value(status.url)')

# Gateway (main entry point)
gcloud run deploy aex-gateway \
    --image "gcr.io/$PROJECT_ID/aex-gateway:$COMMIT_SHA" \
    --region "$REGION" \
    --platform managed \
    --allow-unauthenticated \
    --set-env-vars "\
WORK_PUBLISHER_URL=$WORK_PUBLISHER_URL,\
BID_GATEWAY_URL=$BID_GATEWAY_URL,\
BID_EVALUATOR_URL=$BID_EVALUATOR_URL,\
CONTRACT_ENGINE_URL=$CONTRACT_ENGINE_URL,\
SETTLEMENT_URL=$SETTLEMENT_URL,\
PROVIDER_REGISTRY_URL=$PROVIDER_REGISTRY_URL,\
TRUST_BROKER_URL=$TRUST_BROKER_URL,\
IDENTITY_URL=$IDENTITY_URL,\
TELEMETRY_URL=$TELEMETRY_URL" \
    --memory 512Mi \
    --cpu 1 \
    --min-instances 1 \
    --max-instances 20 \
    --quiet

AEX_GATEWAY_URL=$(gcloud run services describe aex-gateway --region "$REGION" --format 'value(status.url)')

echo ""
echo "=== AEX Services Deployed ==="
echo "Gateway URL: $AEX_GATEWAY_URL"
echo ""

# Deploy demo agents
echo "Deploying demo agents..."

# Legal Agent A (Budget - uses Claude)
gcloud run deploy legal-agent-a \
    --image "gcr.io/$PROJECT_ID/legal-agent-a:$COMMIT_SHA" \
    --region "$REGION" \
    --platform managed \
    --allow-unauthenticated \
    --set-secrets "ANTHROPIC_API_KEY=ANTHROPIC_API_KEY:latest" \
    --set-env-vars "AEX_GATEWAY_URL=$AEX_GATEWAY_URL" \
    --memory 1Gi \
    --cpu 1 \
    --min-instances 0 \
    --max-instances 5 \
    --quiet

LEGAL_A_URL=$(gcloud run services describe legal-agent-a --region "$REGION" --format 'value(status.url)')

# Legal Agent B (Standard - uses Claude)
gcloud run deploy legal-agent-b \
    --image "gcr.io/$PROJECT_ID/legal-agent-b:$COMMIT_SHA" \
    --region "$REGION" \
    --platform managed \
    --allow-unauthenticated \
    --set-secrets "ANTHROPIC_API_KEY=ANTHROPIC_API_KEY:latest" \
    --set-env-vars "AEX_GATEWAY_URL=$AEX_GATEWAY_URL" \
    --memory 1Gi \
    --cpu 1 \
    --min-instances 0 \
    --max-instances 5 \
    --quiet

LEGAL_B_URL=$(gcloud run services describe legal-agent-b --region "$REGION" --format 'value(status.url)')

# Legal Agent C (Premium - uses Claude)
gcloud run deploy legal-agent-c \
    --image "gcr.io/$PROJECT_ID/legal-agent-c:$COMMIT_SHA" \
    --region "$REGION" \
    --platform managed \
    --allow-unauthenticated \
    --set-secrets "ANTHROPIC_API_KEY=ANTHROPIC_API_KEY:latest" \
    --set-env-vars "AEX_GATEWAY_URL=$AEX_GATEWAY_URL" \
    --memory 1Gi \
    --cpu 1 \
    --min-instances 0 \
    --max-instances 5 \
    --quiet

LEGAL_C_URL=$(gcloud run services describe legal-agent-c --region "$REGION" --format 'value(status.url)')

# Orchestrator
gcloud run deploy orchestrator \
    --image "gcr.io/$PROJECT_ID/orchestrator:$COMMIT_SHA" \
    --region "$REGION" \
    --platform managed \
    --allow-unauthenticated \
    --set-secrets "ANTHROPIC_API_KEY=ANTHROPIC_API_KEY:latest" \
    --set-env-vars "AEX_GATEWAY_URL=$AEX_GATEWAY_URL,LEGAL_AGENT_A_URL=$LEGAL_A_URL,LEGAL_AGENT_B_URL=$LEGAL_B_URL,LEGAL_AGENT_C_URL=$LEGAL_C_URL" \
    --memory 1Gi \
    --cpu 1 \
    --min-instances 0 \
    --max-instances 5 \
    --quiet

ORCHESTRATOR_URL=$(gcloud run services describe orchestrator --region "$REGION" --format 'value(status.url)')

# Demo UI
gcloud run deploy demo-ui \
    --image "gcr.io/$PROJECT_ID/demo-ui:$COMMIT_SHA" \
    --region "$REGION" \
    --platform managed \
    --allow-unauthenticated \
    --set-env-vars "ORCHESTRATOR_URL=$ORCHESTRATOR_URL,LEGAL_AGENT_A_URL=$LEGAL_A_URL,LEGAL_AGENT_B_URL=$LEGAL_B_URL,LEGAL_AGENT_C_URL=$LEGAL_C_URL" \
    --memory 512Mi \
    --cpu 1 \
    --min-instances 0 \
    --max-instances 5 \
    --quiet

DEMO_UI_URL=$(gcloud run services describe demo-ui --region "$REGION" --format 'value(status.url)')

echo ""
echo "========================================="
echo "         DEPLOYMENT COMPLETE"
echo "========================================="
echo ""
echo "AEX Gateway:     $AEX_GATEWAY_URL"
echo ""
echo "Demo Legal Agents (with tiered pricing):"
echo "  Legal Agent A (Budget):   $LEGAL_A_URL"
echo "    - Base: \$5 + \$2/page (best for 1-5 pages)"
echo "  Legal Agent B (Standard): $LEGAL_B_URL"
echo "    - Base: \$15 + \$0.50/page (best for 5-30 pages)"
echo "  Legal Agent C (Premium):  $LEGAL_C_URL"
echo "    - Base: \$30 + \$0.20/page (best for 30+ pages)"
echo "  Orchestrator:             $ORCHESTRATOR_URL"
echo ""
echo "Demo UI:         $DEMO_UI_URL"
echo ""
echo "========================================="
echo ""
echo "Next steps:"
echo "1. Update API key secret:"
echo "   echo 'your-key' | gcloud secrets versions add ANTHROPIC_API_KEY --data-file=-"
echo ""
echo "2. Open the demo UI: $DEMO_UI_URL"
echo ""
