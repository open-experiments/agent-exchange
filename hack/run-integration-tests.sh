#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Agent Exchange Integration Tests${NC}"
echo "=================================="

# Check if services are running
check_services() {
    echo -e "\n${YELLOW}Checking service health...${NC}"
    
    services=(
        "http://localhost:8081/health:work-publisher"
        "http://localhost:8082/health:bid-gateway"
        "http://localhost:8083/health:bid-evaluator"
        "http://localhost:8084/health:contract-engine"
        "http://localhost:8085/health:provider-registry"
        "http://localhost:8086/health:trust-broker"
        "http://localhost:8087/health:identity"
        "http://localhost:8088/health:settlement"
    )
    
    all_healthy=true
    for svc in "${services[@]}"; do
        url="${svc%%:*}"
        name="${svc##*:}"
        
        if curl -s -f "$url" > /dev/null 2>&1; then
            echo -e "  ${GREEN}OK${NC} $name"
        else
            echo -e "  ${RED}FAIL${NC} $name"
            all_healthy=false
        fi
    done
    
    if [ "$all_healthy" = false ]; then
        echo -e "\n${RED}Some services are not healthy.${NC}"
        echo "Start services with: make docker-up"
        return 1
    fi
    
    echo -e "\n${GREEN}All services healthy!${NC}"
    return 0
}

# Start services if not running
start_services() {
    echo -e "\n${YELLOW}Starting services...${NC}"
    cd "$PROJECT_ROOT"
    
    # Build and start
    make docker-build
    make docker-up
    
    # Wait for services to be ready
    echo "Waiting for services to start..."
    sleep 10
    
    # Check health with retries
    for i in {1..30}; do
        if check_services 2>/dev/null; then
            return 0
        fi
        echo "  Waiting... ($i/30)"
        sleep 2
    done
    
    echo -e "${RED}Services failed to start${NC}"
    return 1
}

# Run the tests
run_tests() {
    echo -e "\n${YELLOW}Running integration tests...${NC}"
    cd "$SCRIPT_DIR/integration"
    
    # Run with verbose output
    go test -v -timeout 5m ./...
    
    return $?
}

# Parse arguments
SKIP_START=false
VERBOSE=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --skip-start)
            SKIP_START=true
            shift
            ;;
        --verbose|-v)
            VERBOSE=true
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  --skip-start    Skip starting services (assume already running)"
            echo "  --verbose, -v   Verbose output"
            echo "  --help, -h      Show this help"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Main execution
if [ "$SKIP_START" = true ]; then
    if ! check_services; then
        echo -e "${RED}Services not running. Remove --skip-start to auto-start.${NC}"
        exit 1
    fi
else
    if ! check_services 2>/dev/null; then
        start_services
    fi
fi

# Run tests
if run_tests; then
    echo -e "\n${GREEN}All integration tests passed!${NC}"
    exit 0
else
    echo -e "\n${RED}Some integration tests failed.${NC}"
    exit 1
fi

