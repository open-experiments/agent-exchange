# GCP Deployment Guide

This guide explains how to deploy Agent Exchange (AEX) and demo agents to Google Cloud Platform using Cloud Run.

## Prerequisites

1. **GCP Project** with billing enabled
2. **gcloud CLI** installed and authenticated
3. **API Keys** for LLM providers:
   - Anthropic API Key (for Claude - used by Legal Agent A and Orchestrator)
   - Google AI API Key (for Gemini - used by Legal Agent B and Travel Agent)

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              GCP Cloud Run                                  │
│                                                                             │
│  ┌─────────────┐    ┌────────────────────────────────────────────────┐      │
│  │   Demo UI   │───▶│                  AEX Gateway                   │      │
│  └─────────────┘    └────────────────────────────────────────────────┘      │
│         │                              │                                    │
│         │           ┌──────────────────┼──────────────────┐                 │
│         │           │                  │                  │                 │
│         ▼           ▼                  ▼                  ▼                 │
│  ┌─────────────┐ ┌─────────┐ ┌─────────────┐ ┌─────────────────┐            │
│  │ Orchestrator│ │Work Pub │ │Bid Gateway  │ │Provider Registry│            │ 
│  └─────────────┘ └─────────┘ └─────────────┘ └─────────────────┘            │
│         │                                              │                    │
│         │ A2A                            Skill Search  │                    │
│         ▼                                              │                    │
│  ┌──────────────────────────────────────────────────┐  │                    │
│  │              Provider Agents (A2A)               │◀┘                     │
│  │  ┌────────────┐ ┌────────────┐ ┌────────────┐    │                       │
│  │  │Legal A     │ │Legal B     │ │Travel      │    │                       │
│  │  │(Claude)    │ │(Gemini)    │ │(Gemini)    │    │                       │
│  │  └────────────┘ └────────────┘ └────────────┘    │                       │
│  └──────────────────────────────────────────────────┘                       │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                         Secret Manager                              │    │
│  │         ANTHROPIC_API_KEY  |  GOOGLE_AI_API_KEY                     │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Quick Start

### 1. Set up your project

```bash
# Set your project ID
export PROJECT_ID="your-project-id"
export REGION="us-central1"

# Authenticate
gcloud auth login
gcloud config set project $PROJECT_ID
```

### 2. Deploy everything

```bash
cd deploy/gcp
./deploy.sh $PROJECT_ID $REGION
```

### 3. Configure API keys

After deployment, update the secrets with your actual API keys:

```bash
# Anthropic (Claude) - for Legal Agent A and Orchestrator
echo "sk-ant-..." | gcloud secrets versions add ANTHROPIC_API_KEY --data-file=-

# Google AI (Gemini) - for Legal Agent B and Travel Agent
echo "AI..." | gcloud secrets versions add GOOGLE_AI_API_KEY --data-file=-
```

### 4. Access the demo

The deploy script will output the Demo UI URL. Open it in your browser to try the demo.

## Manual Deployment

If you prefer to deploy services individually:

### Build images

```bash
# Build with Cloud Build
gcloud builds submit --config=deploy/gcp/cloudbuild.yaml .
```

### Deploy a single service

```bash
gcloud run deploy aex-gateway \
    --image gcr.io/$PROJECT_ID/aex-gateway:latest \
    --region $REGION \
    --platform managed \
    --allow-unauthenticated
```

## Environment Variables

### AEX Services

| Service | Required Variables |
|---------|-------------------|
| aex-gateway | WORK_PUBLISHER_URL, BID_GATEWAY_URL, etc. |
| aex-provider-registry | MONGO_URI (optional) |
| aex-work-publisher | PROVIDER_REGISTRY_URL |
| aex-bid-gateway | PROVIDER_REGISTRY_URL |
| aex-bid-evaluator | BID_GATEWAY_URL, TRUST_BROKER_URL |
| aex-contract-engine | BID_GATEWAY_URL, WORK_PUBLISHER_URL |
| aex-settlement | CONTRACT_ENGINE_URL, TRUST_BROKER_URL |

### Demo Agents

| Agent | LLM | Required Secrets |
|-------|-----|-----------------|
| legal-agent-a | Claude | ANTHROPIC_API_KEY |
| legal-agent-b | Gemini | GOOGLE_AI_API_KEY |
| travel-agent | Gemini | GOOGLE_AI_API_KEY |
| orchestrator | Claude | ANTHROPIC_API_KEY |

## Cost Optimization

- All services use **min-instances: 0** to scale to zero when idle
- Services auto-scale based on traffic
- Memory is set conservatively (512Mi-1Gi)

To reduce costs further:
```bash
# Set all services to min-instances 0
for service in aex-gateway aex-provider-registry legal-agent-a; do
    gcloud run services update $service --min-instances 0 --region $REGION
done
```

## Monitoring

View logs in Cloud Console:
```bash
gcloud logging read "resource.type=cloud_run_revision" --limit 50
```

Or use the Cloud Console: https://console.cloud.google.com/run

## Cleanup

To delete all deployed services:

```bash
# Delete Cloud Run services
for service in demo-ui orchestrator travel-agent legal-agent-b legal-agent-a \
    aex-gateway aex-telemetry aex-identity aex-settlement aex-contract-engine \
    aex-bid-evaluator aex-trust-broker aex-bid-gateway aex-work-publisher \
    aex-provider-registry; do
    gcloud run services delete $service --region $REGION --quiet
done

# Delete container images
gcloud container images list --repository gcr.io/$PROJECT_ID | \
    xargs -I {} gcloud container images delete {} --force-delete-tags --quiet
```

## Troubleshooting

### Service not starting

Check logs:
```bash
gcloud run services logs read aex-gateway --region $REGION
```

### Secret not found

Ensure secrets are created:
```bash
gcloud secrets list
```

### Connection refused between services

Ensure services allow unauthenticated access:
```bash
gcloud run services add-iam-policy-binding SERVICE_NAME \
    --member="allUsers" \
    --role="roles/run.invoker" \
    --region $REGION
```
