#!/bin/bash
set -e

# Agent Exchange - AWS Teardown Script
# This script removes all AWS resources created for Agent Exchange
# WARNING: This is DESTRUCTIVE and will delete all data!

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"


# Configuration
AWS_REGION="${AWS_REGION:-us-east-1}"
AWS_ACCOUNT_ID="${AWS_ACCOUNT_ID:-}"
CLUSTER_NAME="${CLUSTER_NAME:-aex-cluster}"

usage() {
    echo "Agent Exchange - AWS Teardown"
    echo ""
    echo "Usage: $0 [command]"
    echo ""
    echo "Commands:"
    echo "  all         Delete all AWS resources (default)"
    echo "  ecs         Delete ECS services and cluster only"
    echo "  ecr         Delete ECR repositories only"
    echo "  vpc         Delete VPC and networking only"
    echo "  iam         Delete IAM roles only"
    echo "  secrets     Delete Secrets Manager secrets only"
    echo "  alb         Delete ALB and target groups only"
    echo "  logs        Delete CloudWatch log groups only"
    echo "  documentdb  Delete DocumentDB cluster only"
    echo ""
    echo "Environment variables:"
    echo "  AWS_REGION       AWS region (default: us-east-1)"
    echo "  AWS_ACCOUNT_ID   AWS account ID (auto-detected if not set)"
    echo "  CLUSTER_NAME     ECS cluster name (default: aex-cluster)"
    echo ""
    echo "WARNING: This will permanently delete all resources and data!"
}

check_prerequisites() {
    echo "Checking prerequisites..."
    
    if [ -z "$AWS_ACCOUNT_ID" ]; then
        AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text 2>/dev/null || true)
        if [ -z "$AWS_ACCOUNT_ID" ]; then
            echo "Error: Could not determine AWS Account ID"
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

confirm_deletion() {
    echo ""
    echo "╔══════════════════════════════════════════════════════════════╗"
    echo "║                        WARNING                                ║"
    echo "║  This will PERMANENTLY DELETE all Agent Exchange resources   ║"
    echo "║  in AWS region: $AWS_REGION                                   "
    echo "║  Account: $AWS_ACCOUNT_ID                                     "
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

delete_ecs_services() {
    echo "Deleting ECS services..."
    
    for cluster in "$CLUSTER_NAME" "$CLUSTER_NAME-staging"; do
        if aws ecs describe-clusters --clusters "$cluster" --region "$AWS_REGION" \
            --query 'clusters[?status==`ACTIVE`].clusterName' --output text 2>/dev/null | grep -q "$cluster"; then
            
            # List and delete services
            services=$(aws ecs list-services --cluster "$cluster" --region "$AWS_REGION" \
                --query 'serviceArns[*]' --output text 2>/dev/null || echo "")
            
            for service_arn in $services; do
                service_name=$(basename "$service_arn")
                echo "  Scaling down $service_name..."
                aws ecs update-service \
                    --cluster "$cluster" \
                    --service "$service_name" \
                    --desired-count 0 \
                    --region "$AWS_REGION" \
                    --output text > /dev/null 2>&1 || true
            done
            
            # Wait a bit for tasks to drain
            sleep 10
            
            for service_arn in $services; do
                service_name=$(basename "$service_arn")
                echo "  Deleting service $service_name..."
                aws ecs delete-service \
                    --cluster "$cluster" \
                    --service "$service_name" \
                    --force \
                    --region "$AWS_REGION" \
                    --output text > /dev/null 2>&1 || true
            done
            
            echo "  Deleting cluster $cluster..."
            aws ecs delete-cluster \
                --cluster "$cluster" \
                --region "$AWS_REGION" \
                --output text > /dev/null 2>&1 || true
        fi
    done
    
    # Delete task definitions
    echo "  Deleting task definitions..."
    task_defs=$(aws ecs list-task-definitions \
        --family-prefix "aex-" \
        --region "$AWS_REGION" \
        --query 'taskDefinitionArns[*]' \
        --output text 2>/dev/null || echo "")
    
    for td in $task_defs; do
        echo "    Deregistering $td..."
        aws ecs deregister-task-definition \
            --task-definition "$td" \
            --region "$AWS_REGION" \
            --output text > /dev/null 2>&1 || true
    done
    
    echo "ECS resources deleted"
}

delete_alb() {
    echo "Deleting Application Load Balancer..."
    
    # Delete listeners first
    alb_arn=$(aws elbv2 describe-load-balancers --names "aex-alb" --region "$AWS_REGION" \
        --query 'LoadBalancers[0].LoadBalancerArn' --output text 2>/dev/null || echo "")
    
    if [ -n "$alb_arn" ] && [ "$alb_arn" != "None" ]; then
        listeners=$(aws elbv2 describe-listeners \
            --load-balancer-arn "$alb_arn" \
            --region "$AWS_REGION" \
            --query 'Listeners[*].ListenerArn' \
            --output text 2>/dev/null || echo "")
        
        for listener in $listeners; do
            echo "  Deleting listener..."
            aws elbv2 delete-listener \
                --listener-arn "$listener" \
                --region "$AWS_REGION" 2>/dev/null || true
        done
        
        echo "  Deleting ALB..."
        aws elbv2 delete-load-balancer \
            --load-balancer-arn "$alb_arn" \
            --region "$AWS_REGION" 2>/dev/null || true
        
        # Wait for ALB deletion
        echo "  Waiting for ALB deletion..."
        sleep 30
    fi
    
    # Delete target groups
    echo "  Deleting target groups..."
    tgs=$(aws elbv2 describe-target-groups \
        --region "$AWS_REGION" \
        --query 'TargetGroups[?starts_with(TargetGroupName, `aex-`)].TargetGroupArn' \
        --output text 2>/dev/null || echo "")
    
    for tg in $tgs; do
        echo "    Deleting $(basename $tg)..."
        aws elbv2 delete-target-group \
            --target-group-arn "$tg" \
            --region "$AWS_REGION" 2>/dev/null || true
    done
    
    echo "ALB deleted"
}

delete_ecr_repositories() {
    echo "Deleting ECR repositories..."
    
    services=(
        "agent-exchange/aex-gateway"
        "agent-exchange/aex-work-publisher"
        "agent-exchange/aex-bid-gateway"
        "agent-exchange/aex-bid-evaluator"
        "agent-exchange/aex-contract-engine"
        "agent-exchange/aex-provider-registry"
        "agent-exchange/aex-trust-broker"
        "agent-exchange/aex-identity"
        "agent-exchange/aex-settlement"
        "agent-exchange/aex-telemetry"
    )
    
    for repo in "${services[@]}"; do
        if aws ecr describe-repositories --repository-names "$repo" --region "$AWS_REGION" &> /dev/null; then
            echo "  Deleting $repo..."
            aws ecr delete-repository \
                --repository-name "$repo" \
                --force \
                --region "$AWS_REGION" \
                --output text > /dev/null 2>&1 || true
        fi
    done
    
    echo "ECR repositories deleted"
}

delete_vpc() {
    echo "Deleting VPC and networking..."
    
    VPC_ID=$(aws ec2 describe-vpcs \
        --filters "Name=tag:Name,Values=aex-vpc" \
        --region "$AWS_REGION" \
        --query 'Vpcs[0].VpcId' \
        --output text 2>/dev/null || echo "None")
    
    if [ "$VPC_ID" = "None" ] || [ -z "$VPC_ID" ]; then
        echo "  VPC not found, skipping..."
        return
    fi
    
    # Delete NAT Gateways
    echo "  Deleting NAT Gateways..."
    nat_gws=$(aws ec2 describe-nat-gateways \
        --filter "Name=vpc-id,Values=$VPC_ID" \
        --region "$AWS_REGION" \
        --query 'NatGateways[*].NatGatewayId' \
        --output text 2>/dev/null || echo "")
    
    for nat in $nat_gws; do
        aws ec2 delete-nat-gateway --nat-gateway-id "$nat" --region "$AWS_REGION" 2>/dev/null || true
    done
    
    # Wait for NAT Gateway deletion
    if [ -n "$nat_gws" ]; then
        echo "  Waiting for NAT Gateway deletion..."
        sleep 60
    fi
    
    # Delete security groups (except default)
    echo "  Deleting security groups..."
    sgs=$(aws ec2 describe-security-groups \
        --filters "Name=vpc-id,Values=$VPC_ID" \
        --region "$AWS_REGION" \
        --query 'SecurityGroups[?GroupName!=`default`].GroupId' \
        --output text 2>/dev/null || echo "")
    
    for sg in $sgs; do
        echo "    Deleting $sg..."
        aws ec2 delete-security-group --group-id "$sg" --region "$AWS_REGION" 2>/dev/null || true
    done
    
    # Delete subnets
    echo "  Deleting subnets..."
    subnets=$(aws ec2 describe-subnets \
        --filters "Name=vpc-id,Values=$VPC_ID" \
        --region "$AWS_REGION" \
        --query 'Subnets[*].SubnetId' \
        --output text 2>/dev/null || echo "")
    
    for subnet in $subnets; do
        echo "    Deleting $subnet..."
        aws ec2 delete-subnet --subnet-id "$subnet" --region "$AWS_REGION" 2>/dev/null || true
    done
    
    # Delete route tables (except main)
    echo "  Deleting route tables..."
    rts=$(aws ec2 describe-route-tables \
        --filters "Name=vpc-id,Values=$VPC_ID" \
        --region "$AWS_REGION" \
        --query 'RouteTables[?Associations[0].Main!=`true`].RouteTableId' \
        --output text 2>/dev/null || echo "")
    
    for rt in $rts; do
        # Delete route table associations first
        associations=$(aws ec2 describe-route-tables \
            --route-table-ids "$rt" \
            --region "$AWS_REGION" \
            --query 'RouteTables[0].Associations[?!Main].RouteTableAssociationId' \
            --output text 2>/dev/null || echo "")
        
        for assoc in $associations; do
            aws ec2 disassociate-route-table --association-id "$assoc" --region "$AWS_REGION" 2>/dev/null || true
        done
        
        echo "    Deleting $rt..."
        aws ec2 delete-route-table --route-table-id "$rt" --region "$AWS_REGION" 2>/dev/null || true
    done
    
    # Detach and delete internet gateway
    echo "  Deleting internet gateway..."
    igw=$(aws ec2 describe-internet-gateways \
        --filters "Name=attachment.vpc-id,Values=$VPC_ID" \
        --region "$AWS_REGION" \
        --query 'InternetGateways[0].InternetGatewayId' \
        --output text 2>/dev/null || echo "None")
    
    if [ "$igw" != "None" ] && [ -n "$igw" ]; then
        aws ec2 detach-internet-gateway --internet-gateway-id "$igw" --vpc-id "$VPC_ID" --region "$AWS_REGION" 2>/dev/null || true
        aws ec2 delete-internet-gateway --internet-gateway-id "$igw" --region "$AWS_REGION" 2>/dev/null || true
    fi
    
    # Delete VPC
    echo "  Deleting VPC..."
    aws ec2 delete-vpc --vpc-id "$VPC_ID" --region "$AWS_REGION" 2>/dev/null || true
    
    echo "VPC deleted"
}

delete_iam_roles() {
    echo "Deleting IAM roles..."
    
    roles=(
        "aex-ecs-execution-role"
        "aex-ecs-task-role"
        "aex-github-actions-role"
    )
    
    for role in "${roles[@]}"; do
        if aws iam get-role --role-name "$role" &> /dev/null; then
            # Delete inline policies
            policies=$(aws iam list-role-policies --role-name "$role" \
                --query 'PolicyNames[*]' --output text 2>/dev/null || echo "")
            
            for policy in $policies; do
                echo "  Deleting inline policy $policy from $role..."
                aws iam delete-role-policy --role-name "$role" --policy-name "$policy" 2>/dev/null || true
            done
            
            # Detach managed policies
            attached=$(aws iam list-attached-role-policies --role-name "$role" \
                --query 'AttachedPolicies[*].PolicyArn' --output text 2>/dev/null || echo "")
            
            for policy_arn in $attached; do
                echo "  Detaching $policy_arn from $role..."
                aws iam detach-role-policy --role-name "$role" --policy-arn "$policy_arn" 2>/dev/null || true
            done
            
            echo "  Deleting role $role..."
            aws iam delete-role --role-name "$role" 2>/dev/null || true
        fi
    done
    
    # Delete OIDC provider for GitHub
    echo "  Checking for GitHub OIDC provider..."
    oidc_arn=$(aws iam list-open-id-connect-providers \
        --query 'OpenIDConnectProviderList[?contains(Arn, `token.actions.githubusercontent.com`)].Arn' \
        --output text 2>/dev/null || echo "")
    
    if [ -n "$oidc_arn" ]; then
        echo "  Deleting GitHub OIDC provider..."
        aws iam delete-open-id-connect-provider --open-id-connect-provider-arn "$oidc_arn" 2>/dev/null || true
    fi
    
    echo "IAM roles deleted"
}

delete_secrets() {
    echo "Deleting Secrets Manager secrets..."
    
    secrets=(
        "aex-jwt-secret"
        "aex-api-key-salt"
        "aex-docdb-password"
        "aex-mongo-uri"
    )
    
    for secret in "${secrets[@]}"; do
        if aws secretsmanager describe-secret --secret-id "$secret" --region "$AWS_REGION" &> /dev/null; then
            echo "  Deleting $secret..."
            aws secretsmanager delete-secret \
                --secret-id "$secret" \
                --force-delete-without-recovery \
                --region "$AWS_REGION" 2>/dev/null || true
        fi
    done
    
    echo "Secrets deleted"
}

delete_cloudwatch_logs() {
    echo "Deleting CloudWatch log groups..."
    
    services=(
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
    
    for service in "${services[@]}"; do
        log_group="/ecs/agent-exchange/$service"
        
        if aws logs describe-log-groups --log-group-name-prefix "$log_group" --region "$AWS_REGION" \
            --query 'logGroups[?logGroupName==`'"$log_group"'`].logGroupName' --output text 2>/dev/null | grep -q "$log_group"; then
            echo "  Deleting $log_group..."
            aws logs delete-log-group --log-group-name "$log_group" --region "$AWS_REGION" 2>/dev/null || true
        fi
    done
    
    echo "CloudWatch log groups deleted"
}

delete_documentdb() {
    echo "Deleting DocumentDB..."
    
    # Delete instances first
    instances=$(aws docdb describe-db-instances \
        --filters "Name=db-cluster-id,Values=aex-docdb" \
        --region "$AWS_REGION" \
        --query 'DBInstances[*].DBInstanceIdentifier' \
        --output text 2>/dev/null || echo "")
    
    for instance in $instances; do
        echo "  Deleting instance $instance..."
        aws docdb delete-db-instance \
            --db-instance-identifier "$instance" \
            --region "$AWS_REGION" 2>/dev/null || true
    done
    
    # Wait for instances to be deleted
    if [ -n "$instances" ]; then
        echo "  Waiting for instances to be deleted..."
        sleep 120
    fi
    
    # Delete cluster
    if aws docdb describe-db-clusters --db-cluster-identifier "aex-docdb" --region "$AWS_REGION" &> /dev/null; then
        echo "  Deleting cluster aex-docdb..."
        aws docdb delete-db-cluster \
            --db-cluster-identifier "aex-docdb" \
            --skip-final-snapshot \
            --region "$AWS_REGION" 2>/dev/null || true
    fi
    
    # Delete subnet group
    if aws docdb describe-db-subnet-groups --db-subnet-group-name "aex-docdb-subnet-group" --region "$AWS_REGION" &> /dev/null; then
        echo "  Deleting subnet group..."
        aws docdb delete-db-subnet-group \
            --db-subnet-group-name "aex-docdb-subnet-group" \
            --region "$AWS_REGION" 2>/dev/null || true
    fi
    
    echo "DocumentDB deleted"
}

print_summary() {
    echo ""
    echo "========================================"
    echo "AWS Teardown Complete!"
    echo "========================================"
    echo ""
    echo "Deleted resources:"
    echo "  - ECS services and clusters"
    echo "  - Application Load Balancer"
    echo "  - ECR repositories"
    echo "  - VPC and networking"
    echo "  - IAM roles"
    echo "  - Secrets Manager secrets"
    echo "  - CloudWatch log groups"
    echo ""
    echo "Note: Some resources may take a few minutes to fully delete."
}

# Main
case "${1:-all}" in
    -h|--help|help)
        usage
        exit 0
        ;;
    ecs)
        check_prerequisites
        confirm_deletion
        delete_ecs_services
        ;;
    alb)
        check_prerequisites
        confirm_deletion
        delete_alb
        ;;
    ecr)
        check_prerequisites
        confirm_deletion
        delete_ecr_repositories
        ;;
    vpc)
        check_prerequisites
        confirm_deletion
        delete_vpc
        ;;
    iam)
        check_prerequisites
        confirm_deletion
        delete_iam_roles
        ;;
    secrets)
        check_prerequisites
        confirm_deletion
        delete_secrets
        ;;
    logs)
        check_prerequisites
        confirm_deletion
        delete_cloudwatch_logs
        ;;
    documentdb)
        check_prerequisites
        confirm_deletion
        delete_documentdb
        ;;
    all)
        check_prerequisites
        echo ""
        echo "Region: $AWS_REGION"
        echo "Account: $AWS_ACCOUNT_ID"
        echo ""
        confirm_deletion
        echo ""
        
        delete_ecs_services
        echo ""
        delete_alb
        echo ""
        delete_documentdb
        echo ""
        delete_ecr_repositories
        echo ""
        delete_secrets
        echo ""
        delete_cloudwatch_logs
        echo ""
        delete_iam_roles
        echo ""
        delete_vpc
        echo ""
        print_summary
        ;;
    *)
        echo "Unknown command: $1"
        usage
        exit 1
        ;;
esac

