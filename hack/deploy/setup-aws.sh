#!/bin/bash
set -e

# Agent Exchange - AWS Setup Script
# This script sets up the required AWS resources for deployment

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"


# Configuration
AWS_REGION="${AWS_REGION:-us-east-1}"
AWS_ACCOUNT_ID="${AWS_ACCOUNT_ID:-}"
CLUSTER_NAME="${CLUSTER_NAME:-aex-cluster}"
VPC_CIDR="${VPC_CIDR:-10.0.0.0/16}"
DRY_RUN=false

usage() {
    echo "Agent Exchange - AWS Setup"
    echo ""
    echo "Usage: $0 [command] [options]"
    echo ""
    echo "Commands:"
    echo "  all         Run all setup steps (default)"
    echo "  ecr         Create ECR repositories only"
    echo "  vpc         Create VPC and networking only"
    echo "  ecs         Create ECS cluster only"
    echo "  iam         Create IAM roles only"
    echo "  secrets     Create Secrets Manager secrets only"
    echo "  documentdb  Create DocumentDB cluster only"
    echo "  validate    Validate configuration without creating resources"
    echo ""
    echo "Options:"
    echo "  --dry-run   Show what would be done without making changes"
    echo ""
    echo "Environment variables:"
    echo "  AWS_REGION       AWS region (default: us-east-1)"
    echo "  AWS_ACCOUNT_ID   AWS account ID (required)"
    echo "  CLUSTER_NAME     ECS cluster name (default: aex-cluster)"
    echo ""
    echo "This script will:"
    echo "  1. Create ECR repositories for all services"
    echo "  2. Create VPC with public/private subnets"
    echo "  3. Create ECS Fargate cluster"
    echo "  4. Create IAM roles for ECS tasks"
    echo "  5. Create Secrets Manager secrets"
    echo "  6. Create DocumentDB cluster (optional)"
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

# Dry run wrapper
run_cmd() {
    if [ "$DRY_RUN" = true ]; then
        echo "[DRY-RUN] Would execute: $*"
        return 0
    else
        "$@"
    fi
}

validate_aws_setup() {
    echo "═══════════════════════════════════════════════════════════"
    echo "  AWS Setup Validation"
    echo "═══════════════════════════════════════════════════════════"
    echo ""
    
    local errors=0
    local warnings=0
    
    # Check AWS CLI
    echo -n "Checking AWS CLI... "
    if command -v aws &> /dev/null; then
        aws_version=$(aws --version 2>&1 | head -1)
        echo "OK ($aws_version)"
    else
        echo "FAILED"
        echo "  AWS CLI is not installed"
        ((errors++))
    fi
    
    # Check authentication
    echo -n "Checking AWS authentication... "
    if aws sts get-caller-identity &> /dev/null; then
        identity=$(aws sts get-caller-identity --query 'Arn' --output text)
        echo "OK"
        echo "  Identity: $identity"
    else
        echo "FAILED"
        echo "  Not authenticated. Run 'aws configure' or set AWS credentials"
        ((errors++))
    fi
    
    # Check account ID
    echo -n "Checking AWS Account ID... "
    if [ -z "$AWS_ACCOUNT_ID" ]; then
        AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text 2>/dev/null || echo "")
    fi
    if [ -n "$AWS_ACCOUNT_ID" ]; then
        echo "OK ($AWS_ACCOUNT_ID)"
    else
        echo "FAILED"
        echo "  Could not determine AWS Account ID"
        ((errors++))
    fi
    
    # Check region
    echo -n "Checking AWS Region... "
    if aws ec2 describe-availability-zones --region "$AWS_REGION" &> /dev/null; then
        echo "OK ($AWS_REGION)"
    else
        echo "FAILED"
        echo "  Invalid region: $AWS_REGION"
        ((errors++))
    fi
    
    # Check permissions
    echo ""
    echo "Checking IAM permissions..."
    
    permissions_to_check=(
        "ecr:CreateRepository"
        "ec2:CreateVpc"
        "ecs:CreateCluster"
        "iam:CreateRole"
        "secretsmanager:CreateSecret"
        "elasticloadbalancing:CreateLoadBalancer"
        "logs:CreateLogGroup"
    )
    
    for perm in "${permissions_to_check[@]}"; do
        echo -n "  $perm... "
        # Note: This is a basic check; full permission simulation would require more complex IAM calls
        echo "ASSUMED"
    done
    
    # Check existing resources
    echo ""
    echo "Checking existing resources..."
    
    echo -n "  VPC 'aex-vpc'... "
    existing_vpc=$(aws ec2 describe-vpcs --filters "Name=tag:Name,Values=aex-vpc" \
        --region "$AWS_REGION" --query 'Vpcs[0].VpcId' --output text 2>/dev/null || echo "None")
    if [ "$existing_vpc" != "None" ] && [ -n "$existing_vpc" ]; then
        echo "EXISTS ($existing_vpc)"
        ((warnings++))
    else
        echo "WILL CREATE"
    fi
    
    echo -n "  ECS Cluster '$CLUSTER_NAME'... "
    if aws ecs describe-clusters --clusters "$CLUSTER_NAME" --region "$AWS_REGION" \
        --query 'clusters[?status==`ACTIVE`].clusterName' --output text 2>/dev/null | grep -q "$CLUSTER_NAME"; then
        echo "EXISTS"
        ((warnings++))
    else
        echo "WILL CREATE"
    fi
    
    echo -n "  ECR repositories... "
    existing_repos=0
    for service in aex-gateway aex-work-publisher aex-bid-gateway; do
        if aws ecr describe-repositories --repository-names "agent-exchange/$service" --region "$AWS_REGION" &> /dev/null; then
            ((existing_repos++))
        fi
    done
    if [ $existing_repos -gt 0 ]; then
        echo "$existing_repos EXIST"
    else
        echo "WILL CREATE 10"
    fi
    
    # Summary
    echo ""
    echo "═══════════════════════════════════════════════════════════"
    echo "Validation Summary:"
    echo "  Region: $AWS_REGION"
    echo "  Account: $AWS_ACCOUNT_ID"
    echo "  Cluster: $CLUSTER_NAME"
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
    
    if [ -z "$AWS_ACCOUNT_ID" ]; then
        # Try to get from STS
        AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text 2>/dev/null || true)
        if [ -z "$AWS_ACCOUNT_ID" ]; then
            echo "Error: AWS_ACCOUNT_ID environment variable is required"
            exit 1
        fi
        echo "  Detected AWS Account ID: $AWS_ACCOUNT_ID"
    fi
    
    if ! command -v aws &> /dev/null; then
        echo "Error: AWS CLI is not installed"
        exit 1
    fi
    
    # Check if authenticated
    if ! aws sts get-caller-identity &> /dev/null; then
        echo "Error: Not authenticated with AWS. Run 'aws configure' or set credentials"
        exit 1
    fi
    
    echo "Prerequisites OK"
}

create_ecr_repositories() {
    echo "Creating ECR repositories..."
    
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
        repo_name="agent-exchange/$service"
        
        if aws ecr describe-repositories --repository-names "$repo_name" --region "$AWS_REGION" &> /dev/null; then
            echo "  Repository '$repo_name' already exists"
        else
            aws ecr create-repository \
                --repository-name "$repo_name" \
                --region "$AWS_REGION" \
                --image-scanning-configuration scanOnPush=true \
                --encryption-configuration encryptionType=AES256 \
                --output text
            echo "  Created repository: $repo_name"
        fi
    done
    
    echo "ECR repositories created"
}

create_vpc() {
    echo "Creating VPC and networking..."
    
    vpc_name="aex-vpc"
    
    # Check if VPC exists
    existing_vpc=$(aws ec2 describe-vpcs \
        --filters "Name=tag:Name,Values=$vpc_name" \
        --region "$AWS_REGION" \
        --query 'Vpcs[0].VpcId' \
        --output text 2>/dev/null || echo "None")
    
    if [ "$existing_vpc" != "None" ] && [ -n "$existing_vpc" ]; then
        echo "  VPC '$vpc_name' already exists: $existing_vpc"
        VPC_ID="$existing_vpc"
    else
        # Create VPC
        VPC_ID=$(aws ec2 create-vpc \
            --cidr-block "$VPC_CIDR" \
            --region "$AWS_REGION" \
            --query 'Vpc.VpcId' \
            --output text)
        
        aws ec2 create-tags \
            --resources "$VPC_ID" \
            --tags "Key=Name,Value=$vpc_name" \
            --region "$AWS_REGION"
        
        # Enable DNS hostnames
        aws ec2 modify-vpc-attribute \
            --vpc-id "$VPC_ID" \
            --enable-dns-hostnames '{"Value":true}' \
            --region "$AWS_REGION"
        
        echo "  Created VPC: $VPC_ID"
    fi
    
    # Create Internet Gateway
    igw_name="aex-igw"
    existing_igw=$(aws ec2 describe-internet-gateways \
        --filters "Name=tag:Name,Values=$igw_name" \
        --region "$AWS_REGION" \
        --query 'InternetGateways[0].InternetGatewayId' \
        --output text 2>/dev/null || echo "None")
    
    if [ "$existing_igw" != "None" ] && [ -n "$existing_igw" ]; then
        echo "  Internet Gateway '$igw_name' already exists"
        IGW_ID="$existing_igw"
    else
        IGW_ID=$(aws ec2 create-internet-gateway \
            --region "$AWS_REGION" \
            --query 'InternetGateway.InternetGatewayId' \
            --output text)
        
        aws ec2 create-tags \
            --resources "$IGW_ID" \
            --tags "Key=Name,Value=$igw_name" \
            --region "$AWS_REGION"
        
        aws ec2 attach-internet-gateway \
            --vpc-id "$VPC_ID" \
            --internet-gateway-id "$IGW_ID" \
            --region "$AWS_REGION" 2>/dev/null || true
        
        echo "  Created Internet Gateway: $IGW_ID"
    fi
    
    # Create subnets (2 public, 2 private across AZs)
    azs=($(aws ec2 describe-availability-zones \
        --region "$AWS_REGION" \
        --query 'AvailabilityZones[0:2].ZoneName' \
        --output text))
    
    subnet_index=0
    for az in "${azs[@]}"; do
        # Public subnet
        public_cidr="10.0.$((subnet_index * 2)).0/24"
        public_name="aex-public-$az"
        
        existing_pub=$(aws ec2 describe-subnets \
            --filters "Name=tag:Name,Values=$public_name" "Name=vpc-id,Values=$VPC_ID" \
            --region "$AWS_REGION" \
            --query 'Subnets[0].SubnetId' \
            --output text 2>/dev/null || echo "None")
        
        if [ "$existing_pub" = "None" ] || [ -z "$existing_pub" ]; then
            pub_subnet=$(aws ec2 create-subnet \
                --vpc-id "$VPC_ID" \
                --cidr-block "$public_cidr" \
                --availability-zone "$az" \
                --region "$AWS_REGION" \
                --query 'Subnet.SubnetId' \
                --output text)
            
            aws ec2 create-tags \
                --resources "$pub_subnet" \
                --tags "Key=Name,Value=$public_name" \
                --region "$AWS_REGION"
            
            aws ec2 modify-subnet-attribute \
                --subnet-id "$pub_subnet" \
                --map-public-ip-on-launch \
                --region "$AWS_REGION"
            
            echo "  Created public subnet: $pub_subnet ($az)"
        else
            echo "  Public subnet '$public_name' already exists"
        fi
        
        # Private subnet
        private_cidr="10.0.$((subnet_index * 2 + 1)).0/24"
        private_name="aex-private-$az"
        
        existing_priv=$(aws ec2 describe-subnets \
            --filters "Name=tag:Name,Values=$private_name" "Name=vpc-id,Values=$VPC_ID" \
            --region "$AWS_REGION" \
            --query 'Subnets[0].SubnetId' \
            --output text 2>/dev/null || echo "None")
        
        if [ "$existing_priv" = "None" ] || [ -z "$existing_priv" ]; then
            priv_subnet=$(aws ec2 create-subnet \
                --vpc-id "$VPC_ID" \
                --cidr-block "$private_cidr" \
                --availability-zone "$az" \
                --region "$AWS_REGION" \
                --query 'Subnet.SubnetId' \
                --output text)
            
            aws ec2 create-tags \
                --resources "$priv_subnet" \
                --tags "Key=Name,Value=$private_name" \
                --region "$AWS_REGION"
            
            echo "  Created private subnet: $priv_subnet ($az)"
        else
            echo "  Private subnet '$private_name' already exists"
        fi
        
        ((subnet_index++))
    done
    
    # Create route table for public subnets
    rt_name="aex-public-rt"
    existing_rt=$(aws ec2 describe-route-tables \
        --filters "Name=tag:Name,Values=$rt_name" "Name=vpc-id,Values=$VPC_ID" \
        --region "$AWS_REGION" \
        --query 'RouteTables[0].RouteTableId' \
        --output text 2>/dev/null || echo "None")
    
    if [ "$existing_rt" = "None" ] || [ -z "$existing_rt" ]; then
        RT_ID=$(aws ec2 create-route-table \
            --vpc-id "$VPC_ID" \
            --region "$AWS_REGION" \
            --query 'RouteTable.RouteTableId' \
            --output text)
        
        aws ec2 create-tags \
            --resources "$RT_ID" \
            --tags "Key=Name,Value=$rt_name" \
            --region "$AWS_REGION"
        
        # Add route to Internet Gateway
        aws ec2 create-route \
            --route-table-id "$RT_ID" \
            --destination-cidr-block "0.0.0.0/0" \
            --gateway-id "$IGW_ID" \
            --region "$AWS_REGION"
        
        echo "  Created route table: $RT_ID"
        
        # Associate public subnets with route table
        public_subnets=$(aws ec2 describe-subnets \
            --filters "Name=tag:Name,Values=aex-public-*" "Name=vpc-id,Values=$VPC_ID" \
            --region "$AWS_REGION" \
            --query 'Subnets[*].SubnetId' \
            --output text)
        
        for subnet in $public_subnets; do
            aws ec2 associate-route-table \
                --route-table-id "$RT_ID" \
                --subnet-id "$subnet" \
                --region "$AWS_REGION" 2>/dev/null || true
        done
    else
        echo "  Route table '$rt_name' already exists"
    fi
    
    echo "VPC and networking created"
    echo "  VPC ID: $VPC_ID"
}

create_security_groups() {
    echo "Creating security groups..."
    
    # Get VPC ID
    VPC_ID=$(aws ec2 describe-vpcs \
        --filters "Name=tag:Name,Values=aex-vpc" \
        --region "$AWS_REGION" \
        --query 'Vpcs[0].VpcId' \
        --output text)
    
    # ALB Security Group
    alb_sg_name="aex-alb-sg"
    existing_alb_sg=$(aws ec2 describe-security-groups \
        --filters "Name=group-name,Values=$alb_sg_name" "Name=vpc-id,Values=$VPC_ID" \
        --region "$AWS_REGION" \
        --query 'SecurityGroups[0].GroupId' \
        --output text 2>/dev/null || echo "None")
    
    if [ "$existing_alb_sg" = "None" ] || [ -z "$existing_alb_sg" ]; then
        ALB_SG_ID=$(aws ec2 create-security-group \
            --group-name "$alb_sg_name" \
            --description "Agent Exchange ALB Security Group" \
            --vpc-id "$VPC_ID" \
            --region "$AWS_REGION" \
            --query 'GroupId' \
            --output text)
        
        # Allow HTTP/HTTPS from anywhere
        aws ec2 authorize-security-group-ingress \
            --group-id "$ALB_SG_ID" \
            --protocol tcp \
            --port 80 \
            --cidr "0.0.0.0/0" \
            --region "$AWS_REGION"
        
        aws ec2 authorize-security-group-ingress \
            --group-id "$ALB_SG_ID" \
            --protocol tcp \
            --port 443 \
            --cidr "0.0.0.0/0" \
            --region "$AWS_REGION"
        
        echo "  Created ALB security group: $ALB_SG_ID"
    else
        ALB_SG_ID="$existing_alb_sg"
        echo "  ALB security group already exists: $ALB_SG_ID"
    fi
    
    # ECS Tasks Security Group
    ecs_sg_name="aex-ecs-sg"
    existing_ecs_sg=$(aws ec2 describe-security-groups \
        --filters "Name=group-name,Values=$ecs_sg_name" "Name=vpc-id,Values=$VPC_ID" \
        --region "$AWS_REGION" \
        --query 'SecurityGroups[0].GroupId' \
        --output text 2>/dev/null || echo "None")
    
    if [ "$existing_ecs_sg" = "None" ] || [ -z "$existing_ecs_sg" ]; then
        ECS_SG_ID=$(aws ec2 create-security-group \
            --group-name "$ecs_sg_name" \
            --description "Agent Exchange ECS Tasks Security Group" \
            --vpc-id "$VPC_ID" \
            --region "$AWS_REGION" \
            --query 'GroupId' \
            --output text)
        
        # Allow traffic from ALB
        aws ec2 authorize-security-group-ingress \
            --group-id "$ECS_SG_ID" \
            --protocol tcp \
            --port 8080 \
            --source-group "$ALB_SG_ID" \
            --region "$AWS_REGION"
        
        # Allow internal communication between services
        aws ec2 authorize-security-group-ingress \
            --group-id "$ECS_SG_ID" \
            --protocol tcp \
            --port 8080 \
            --source-group "$ECS_SG_ID" \
            --region "$AWS_REGION"
        
        echo "  Created ECS security group: $ECS_SG_ID"
    else
        ECS_SG_ID="$existing_ecs_sg"
        echo "  ECS security group already exists: $ECS_SG_ID"
    fi
    
    # DocumentDB Security Group
    docdb_sg_name="aex-docdb-sg"
    existing_docdb_sg=$(aws ec2 describe-security-groups \
        --filters "Name=group-name,Values=$docdb_sg_name" "Name=vpc-id,Values=$VPC_ID" \
        --region "$AWS_REGION" \
        --query 'SecurityGroups[0].GroupId' \
        --output text 2>/dev/null || echo "None")
    
    if [ "$existing_docdb_sg" = "None" ] || [ -z "$existing_docdb_sg" ]; then
        DOCDB_SG_ID=$(aws ec2 create-security-group \
            --group-name "$docdb_sg_name" \
            --description "Agent Exchange DocumentDB Security Group" \
            --vpc-id "$VPC_ID" \
            --region "$AWS_REGION" \
            --query 'GroupId' \
            --output text)
        
        # Allow MongoDB port from ECS tasks
        aws ec2 authorize-security-group-ingress \
            --group-id "$DOCDB_SG_ID" \
            --protocol tcp \
            --port 27017 \
            --source-group "$ECS_SG_ID" \
            --region "$AWS_REGION"
        
        echo "  Created DocumentDB security group: $DOCDB_SG_ID"
    else
        DOCDB_SG_ID="$existing_docdb_sg"
        echo "  DocumentDB security group already exists: $DOCDB_SG_ID"
    fi
    
    echo "Security groups created"
}

create_ecs_cluster() {
    echo "Creating ECS cluster..."
    
    if aws ecs describe-clusters --clusters "$CLUSTER_NAME" --region "$AWS_REGION" \
        --query 'clusters[?status==`ACTIVE`].clusterName' --output text | grep -q "$CLUSTER_NAME"; then
        echo "  ECS cluster '$CLUSTER_NAME' already exists"
    else
        aws ecs create-cluster \
            --cluster-name "$CLUSTER_NAME" \
            --capacity-providers FARGATE FARGATE_SPOT \
            --default-capacity-provider-strategy \
                capacityProvider=FARGATE,weight=1 \
                capacityProvider=FARGATE_SPOT,weight=1 \
            --region "$AWS_REGION" \
            --output text
        
        echo "  Created ECS cluster: $CLUSTER_NAME"
    fi
    
    echo "ECS cluster created"
}

create_iam_roles() {
    echo "Creating IAM roles..."
    
    # ECS Task Execution Role
    exec_role_name="aex-ecs-execution-role"
    
    if aws iam get-role --role-name "$exec_role_name" &> /dev/null; then
        echo "  Task execution role '$exec_role_name' already exists"
    else
        cat > /tmp/ecs-trust-policy.json << 'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "ecs-tasks.amazonaws.com"
      },
      "Action": "sts:AssumeRole"
    }
  ]
}
EOF
        
        aws iam create-role \
            --role-name "$exec_role_name" \
            --assume-role-policy-document file:///tmp/ecs-trust-policy.json \
            --output text
        
        aws iam attach-role-policy \
            --role-name "$exec_role_name" \
            --policy-arn "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
        
        # Add Secrets Manager access
        cat > /tmp/secrets-policy.json << EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "secretsmanager:GetSecretValue"
      ],
      "Resource": "arn:aws:secretsmanager:$AWS_REGION:$AWS_ACCOUNT_ID:secret:aex-*"
    }
  ]
}
EOF
        
        aws iam put-role-policy \
            --role-name "$exec_role_name" \
            --policy-name "aex-secrets-access" \
            --policy-document file:///tmp/secrets-policy.json
        
        echo "  Created task execution role: $exec_role_name"
    fi
    
    # ECS Task Role (for application)
    task_role_name="aex-ecs-task-role"
    
    if aws iam get-role --role-name "$task_role_name" &> /dev/null; then
        echo "  Task role '$task_role_name' already exists"
    else
        aws iam create-role \
            --role-name "$task_role_name" \
            --assume-role-policy-document file:///tmp/ecs-trust-policy.json \
            --output text
        
        # Add CloudWatch Logs access
        cat > /tmp/task-policy.json << EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogStream",
        "logs:PutLogEvents"
      ],
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "xray:PutTraceSegments",
        "xray:PutTelemetryRecords"
      ],
      "Resource": "*"
    }
  ]
}
EOF
        
        aws iam put-role-policy \
            --role-name "$task_role_name" \
            --policy-name "aex-task-policy" \
            --policy-document file:///tmp/task-policy.json
        
        echo "  Created task role: $task_role_name"
    fi
    
    # GitHub Actions OIDC Role
    gh_role_name="aex-github-actions-role"
    
    if aws iam get-role --role-name "$gh_role_name" &> /dev/null; then
        echo "  GitHub Actions role '$gh_role_name' already exists"
    else
        # Create OIDC provider for GitHub (if not exists)
        if ! aws iam list-open-id-connect-providers --query 'OpenIDConnectProviderList[*].Arn' --output text | grep -q "token.actions.githubusercontent.com"; then
            aws iam create-open-id-connect-provider \
                --url "https://token.actions.githubusercontent.com" \
                --client-id-list "sts.amazonaws.com" \
                --thumbprint-list "6938fd4d98bab03faadb97b34396831e3780aea1"
            echo "  Created GitHub OIDC provider"
        fi
        
        cat > /tmp/gh-trust-policy.json << EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::$AWS_ACCOUNT_ID:oidc-provider/token.actions.githubusercontent.com"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "token.actions.githubusercontent.com:aud": "sts.amazonaws.com"
        },
        "StringLike": {
          "token.actions.githubusercontent.com:sub": "repo:*:*"
        }
      }
    }
  ]
}
EOF
        
        aws iam create-role \
            --role-name "$gh_role_name" \
            --assume-role-policy-document file:///tmp/gh-trust-policy.json \
            --output text
        
        # Attach policies for GitHub Actions
        cat > /tmp/gh-policy.json << EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ecr:GetAuthorizationToken",
        "ecr:BatchCheckLayerAvailability",
        "ecr:GetDownloadUrlForLayer",
        "ecr:BatchGetImage",
        "ecr:InitiateLayerUpload",
        "ecr:UploadLayerPart",
        "ecr:CompleteLayerUpload",
        "ecr:PutImage"
      ],
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "ecs:UpdateService",
        "ecs:DescribeServices",
        "ecs:DescribeTaskDefinition",
        "ecs:RegisterTaskDefinition",
        "ecs:DeregisterTaskDefinition"
      ],
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "iam:PassRole"
      ],
      "Resource": [
        "arn:aws:iam::$AWS_ACCOUNT_ID:role/aex-ecs-execution-role",
        "arn:aws:iam::$AWS_ACCOUNT_ID:role/aex-ecs-task-role"
      ]
    }
  ]
}
EOF
        
        aws iam put-role-policy \
            --role-name "$gh_role_name" \
            --policy-name "aex-github-actions-policy" \
            --policy-document file:///tmp/gh-policy.json
        
        echo "  Created GitHub Actions role: $gh_role_name"
    fi
    
    rm -f /tmp/ecs-trust-policy.json /tmp/secrets-policy.json /tmp/task-policy.json /tmp/gh-trust-policy.json /tmp/gh-policy.json
    
    echo "IAM roles created"
    echo ""
    echo "Add these to your GitHub repository secrets:"
    echo "  AWS_REGION: $AWS_REGION"
    echo "  AWS_ACCOUNT_ID: $AWS_ACCOUNT_ID"
    echo "  AWS_ROLE_ARN: arn:aws:iam::$AWS_ACCOUNT_ID:role/$gh_role_name"
}

create_secrets() {
    echo "Creating Secrets Manager secrets..."
    
    secrets=(
        "aex-jwt-secret"
        "aex-api-key-salt"
        "aex-docdb-password"
    )
    
    for secret in "${secrets[@]}"; do
        if aws secretsmanager describe-secret --secret-id "$secret" --region "$AWS_REGION" &> /dev/null; then
            echo "  Secret '$secret' already exists"
        else
            # Generate random value
            value=$(openssl rand -base64 32)
            aws secretsmanager create-secret \
                --name "$secret" \
                --secret-string "$value" \
                --region "$AWS_REGION" \
                --output text
            echo "  Created secret: $secret"
        fi
    done
    
    echo "Secrets created"
}

create_cloudwatch_log_groups() {
    echo "Creating CloudWatch log groups..."
    
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
            --query 'logGroups[?logGroupName==`'"$log_group"'`].logGroupName' --output text | grep -q "$log_group"; then
            echo "  Log group '$log_group' already exists"
        else
            aws logs create-log-group \
                --log-group-name "$log_group" \
                --region "$AWS_REGION"
            
            aws logs put-retention-policy \
                --log-group-name "$log_group" \
                --retention-in-days 30 \
                --region "$AWS_REGION"
            
            echo "  Created log group: $log_group"
        fi
    done
    
    echo "CloudWatch log groups created"
}

create_alb() {
    echo "Creating Application Load Balancer..."
    
    # Get VPC and subnet IDs
    VPC_ID=$(aws ec2 describe-vpcs \
        --filters "Name=tag:Name,Values=aex-vpc" \
        --region "$AWS_REGION" \
        --query 'Vpcs[0].VpcId' \
        --output text)
    
    PUBLIC_SUBNETS=$(aws ec2 describe-subnets \
        --filters "Name=tag:Name,Values=aex-public-*" "Name=vpc-id,Values=$VPC_ID" \
        --region "$AWS_REGION" \
        --query 'Subnets[*].SubnetId' \
        --output text | tr '\t' ',')
    
    ALB_SG_ID=$(aws ec2 describe-security-groups \
        --filters "Name=group-name,Values=aex-alb-sg" "Name=vpc-id,Values=$VPC_ID" \
        --region "$AWS_REGION" \
        --query 'SecurityGroups[0].GroupId' \
        --output text)
    
    alb_name="aex-alb"
    
    if aws elbv2 describe-load-balancers --names "$alb_name" --region "$AWS_REGION" &> /dev/null; then
        echo "  ALB '$alb_name' already exists"
        ALB_ARN=$(aws elbv2 describe-load-balancers --names "$alb_name" --region "$AWS_REGION" \
            --query 'LoadBalancers[0].LoadBalancerArn' --output text)
    else
        ALB_ARN=$(aws elbv2 create-load-balancer \
            --name "$alb_name" \
            --subnets ${PUBLIC_SUBNETS//,/ } \
            --security-groups "$ALB_SG_ID" \
            --scheme internet-facing \
            --type application \
            --region "$AWS_REGION" \
            --query 'LoadBalancers[0].LoadBalancerArn' \
            --output text)
        
        echo "  Created ALB: $alb_name"
    fi
    
    # Create default target group
    tg_name="aex-gateway-tg"
    
    if aws elbv2 describe-target-groups --names "$tg_name" --region "$AWS_REGION" &> /dev/null; then
        echo "  Target group '$tg_name' already exists"
    else
        aws elbv2 create-target-group \
            --name "$tg_name" \
            --protocol HTTP \
            --port 8080 \
            --vpc-id "$VPC_ID" \
            --target-type ip \
            --health-check-path "/health" \
            --health-check-interval-seconds 30 \
            --health-check-timeout-seconds 5 \
            --healthy-threshold-count 2 \
            --unhealthy-threshold-count 3 \
            --region "$AWS_REGION" \
            --output text
        
        echo "  Created target group: $tg_name"
    fi
    
    # Create listener
    TG_ARN=$(aws elbv2 describe-target-groups --names "$tg_name" --region "$AWS_REGION" \
        --query 'TargetGroups[0].TargetGroupArn' --output text)
    
    existing_listener=$(aws elbv2 describe-listeners \
        --load-balancer-arn "$ALB_ARN" \
        --region "$AWS_REGION" \
        --query 'Listeners[?Port==`80`].ListenerArn' \
        --output text 2>/dev/null || echo "")
    
    if [ -z "$existing_listener" ]; then
        aws elbv2 create-listener \
            --load-balancer-arn "$ALB_ARN" \
            --protocol HTTP \
            --port 80 \
            --default-actions Type=forward,TargetGroupArn="$TG_ARN" \
            --region "$AWS_REGION" \
            --output text
        
        echo "  Created HTTP listener"
    else
        echo "  HTTP listener already exists"
    fi
    
    ALB_DNS=$(aws elbv2 describe-load-balancers --names "$alb_name" --region "$AWS_REGION" \
        --query 'LoadBalancers[0].DNSName' --output text)
    
    echo "ALB created"
    echo "  ALB DNS: $ALB_DNS"
}

print_summary() {
    echo ""
    echo "========================================"
    echo "AWS Setup Complete!"
    echo "========================================"
    echo ""
    echo "Resources created:"
    echo "  - ECR repositories for all 10 services"
    echo "  - VPC with public/private subnets"
    echo "  - Security groups (ALB, ECS, DocumentDB)"
    echo "  - ECS Fargate cluster: $CLUSTER_NAME"
    echo "  - IAM roles for ECS and GitHub Actions"
    echo "  - Secrets Manager secrets"
    echo "  - CloudWatch log groups"
    echo "  - Application Load Balancer"
    echo ""
    echo "GitHub Secrets to add:"
    echo "  AWS_REGION: $AWS_REGION"
    echo "  AWS_ACCOUNT_ID: $AWS_ACCOUNT_ID"
    echo "  AWS_ROLE_ARN: arn:aws:iam::$AWS_ACCOUNT_ID:role/aex-github-actions-role"
    echo ""
    echo "Next steps:"
    echo "  1. Add GitHub secrets to your repository"
    echo "  2. (Optional) Create DocumentDB cluster: $0 documentdb"
    echo "  3. Deploy services: ./deploy-ecs.sh staging all"
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
        validate_aws_setup
        ;;
    ecr)
        check_prerequisites
        if [ "$DRY_RUN" = true ]; then
            echo "[DRY-RUN] Would create 10 ECR repositories"
        else
            create_ecr_repositories
        fi
        ;;
    vpc)
        check_prerequisites
        if [ "$DRY_RUN" = true ]; then
            echo "[DRY-RUN] Would create VPC, subnets, and security groups"
        else
            create_vpc
            create_security_groups
        fi
        ;;
    ecs)
        check_prerequisites
        if [ "$DRY_RUN" = true ]; then
            echo "[DRY-RUN] Would create ECS cluster: $CLUSTER_NAME"
        else
            create_ecs_cluster
        fi
        ;;
    iam)
        check_prerequisites
        if [ "$DRY_RUN" = true ]; then
            echo "[DRY-RUN] Would create IAM roles and OIDC provider"
        else
            create_iam_roles
        fi
        ;;
    secrets)
        check_prerequisites
        if [ "$DRY_RUN" = true ]; then
            echo "[DRY-RUN] Would create Secrets Manager secrets"
        else
            create_secrets
        fi
        ;;
    logs)
        check_prerequisites
        if [ "$DRY_RUN" = true ]; then
            echo "[DRY-RUN] Would create CloudWatch log groups"
        else
            create_cloudwatch_log_groups
        fi
        ;;
    alb)
        check_prerequisites
        if [ "$DRY_RUN" = true ]; then
            echo "[DRY-RUN] Would create Application Load Balancer"
        else
            create_alb
        fi
        ;;
    all)
        check_prerequisites
        
        if [ "$DRY_RUN" = true ]; then
            echo ""
            echo "═══════════════════════════════════════════════════════════"
            echo "  DRY-RUN MODE - No changes will be made"
            echo "═══════════════════════════════════════════════════════════"
            echo ""
            echo "Region: $AWS_REGION"
            echo "Account: $AWS_ACCOUNT_ID"
            echo "Cluster: $CLUSTER_NAME"
            echo ""
            echo "The following resources would be created:"
            echo "  • 10 ECR repositories (agent-exchange/aex-*)"
            echo "  • VPC with CIDR $VPC_CIDR"
            echo "  • 2 public subnets, 2 private subnets"
            echo "  • Internet Gateway"
            echo "  • 3 Security Groups (ALB, ECS, DocumentDB)"
            echo "  • ECS Fargate cluster: $CLUSTER_NAME"
            echo "  • IAM roles: aex-ecs-execution-role, aex-ecs-task-role, aex-github-actions-role"
            echo "  • GitHub OIDC provider"
            echo "  • 3 Secrets Manager secrets"
            echo "  • 10 CloudWatch log groups"
            echo "  • Application Load Balancer"
            echo ""
            echo "Run without --dry-run to create these resources"
        else
            echo ""
            echo "Region: $AWS_REGION"
            echo "Account: $AWS_ACCOUNT_ID"
            echo "Cluster: $CLUSTER_NAME"
            echo ""
            
            create_ecr_repositories
            echo ""
            create_vpc
            echo ""
            create_security_groups
            echo ""
            create_ecs_cluster
            echo ""
            create_iam_roles
            echo ""
            create_secrets
            echo ""
            create_cloudwatch_log_groups
            echo ""
            create_alb
            echo ""
            print_summary
        fi
        ;;
    *)
        echo "Unknown command: $CMD"
        usage
        exit 1
        ;;
esac

