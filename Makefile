.PHONY: help build test clean docker-build docker-up docker-down run lint fmt tidy

# Default target
help:
	@echo "Agent Exchange - Build Automation"
	@echo ""
	@echo "Available targets:"
	@echo "  make build         - Build all services"
	@echo "  make test          - Run all tests"
	@echo "  make clean         - Clean build artifacts"
	@echo "  make docker-build  - Build all Docker images"
	@echo "  make docker-up     - Start all services with Docker Compose"
	@echo "  make docker-down   - Stop all Docker Compose services"
	@echo "  make run           - Run services locally (no Docker)"
	@echo "  make lint          - Run linters"
	@echo "  make fmt           - Format all Go code"
	@echo "  make tidy          - Run go mod tidy on all services"

# Service directories
SERVICES := aex-work-publisher aex-settlement aex-bid-gateway aex-bid-evaluator \
            aex-contract-engine aex-provider-registry aex-trust-broker aex-identity \
            aex-gateway aex-telemetry

# Build all services
build:
	@echo "Building all services..."
	@for service in $(SERVICES); do \
		if [ -f "src/$$service/src/main.go" ]; then \
			echo "Building $$service..."; \
			cd src/$$service && go build -o bin/$$service ./src && cd ../..; \
		fi \
	done
	@echo "Build complete!"

# Build specific service
build-%:
	@echo "Building $*..."
	@cd src/$* && go build -o bin/$* ./src

# Run tests
test:
	@echo "Running tests..."
	@for service in $(SERVICES); do \
		if [ -f "src/$$service/go.mod" ]; then \
			echo "Testing $$service..."; \
			cd src/$$service && go test ./... -v && cd ../..; \
		fi \
	done

# Test specific service
test-%:
	@echo "Testing $*..."
	@cd src/$* && go test ./... -v

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@for service in $(SERVICES); do \
		rm -rf src/$$service/bin; \
		rm -f src/$$service/build/app; \
		rm -f src/$$service/$$service; \
	done
	@echo "Clean complete!"

# Run go mod tidy on all services
tidy:
	@echo "Running go mod tidy..."
	@cd src/internal && go mod tidy
	@for service in $(SERVICES); do \
		if [ -f "src/$$service/go.mod" ]; then \
			echo "Tidying $$service..."; \
			cd src/$$service && go mod tidy && cd ../..; \
		fi \
	done
	@echo "Tidy complete!"

# Format all Go code
fmt:
	@echo "Formatting Go code..."
	@gofmt -s -w src/

# Run linters
lint:
	@echo "Running linters..."
	@if command -v golangci-lint > /dev/null; then \
		for service in $(SERVICES); do \
			if [ -f "src/$$service/go.mod" ]; then \
				echo "Linting $$service..."; \
				cd src/$$service && golangci-lint run ./... && cd ../..; \
			fi \
		done; \
	else \
		echo "golangci-lint not installed. Run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Build Docker images
docker-build:
	@echo "Building Docker images..."
	@docker build -f src/aex-work-publisher/Dockerfile -t agent-exchange/aex-work-publisher:local src/
	@docker build -f src/aex-settlement/Dockerfile -t agent-exchange/aex-settlement:local src/
	@docker build -f src/aex-bid-evaluator/Dockerfile -t agent-exchange/aex-bid-evaluator:local src/
	@docker build -f src/aex-bid-gateway/Dockerfile -t agent-exchange/aex-bid-gateway:local src/
	@docker build -f src/aex-contract-engine/Dockerfile -t agent-exchange/aex-contract-engine:local src/
	@docker build -f src/aex-provider-registry/Dockerfile -t agent-exchange/aex-provider-registry:local src/
	@docker build -f src/aex-trust-broker/Dockerfile -t agent-exchange/aex-trust-broker:local src/
	@docker build -f src/aex-identity/Dockerfile -t agent-exchange/aex-identity:local src/
	@docker build -f src/aex-gateway/Dockerfile -t agent-exchange/aex-gateway:local src/
	@docker build -f src/aex-telemetry/Dockerfile -t agent-exchange/aex-telemetry:local src/
	@echo "Docker build complete!"

# Build specific Docker image
docker-build-%:
	@echo "Building Docker image for $*..."
	@docker build -f src/$*/Dockerfile -t agent-exchange/$*:local src/

# Start Docker Compose
docker-up:
	@echo "Starting services with Docker Compose..."
	@docker-compose -f hack/docker-compose.yml up -d
	@echo "Services started! Check status with: docker-compose -f hack/docker-compose.yml ps"

# Stop Docker Compose
docker-down:
	@echo "Stopping Docker Compose services..."
	@docker-compose -f hack/docker-compose.yml down
	@echo "Services stopped!"

# Stop and remove volumes
docker-clean:
	@echo "Stopping services and removing volumes..."
	@docker-compose -f hack/docker-compose.yml down -v
	@echo "Services and volumes removed!"

# View Docker Compose logs
docker-logs:
	@docker-compose -f hack/docker-compose.yml logs -f

# View logs for specific service
docker-logs-%:
	@docker-compose -f hack/docker-compose.yml logs -f $*

# Run MongoDB locally (for development)
mongo-up:
	@docker run -d --name aex-mongo \
		-e MONGO_INITDB_ROOT_USERNAME=root \
		-e MONGO_INITDB_ROOT_PASSWORD=root \
		-p 27017:27017 \
		mongo:7

mongo-down:
	@docker stop aex-mongo && docker rm aex-mongo

# Database initialization
db-init:
	@echo "Initializing databases..."
	@echo "MongoDB will auto-create collections on first use"
	@echo "No manual initialization needed!"

# Quick start (build + docker up)
quickstart: build docker-build docker-up
	@echo ""
	@echo "🚀 Agent Exchange is running!"
	@echo ""
	@echo "Services:"
	@echo "  - Gateway:           http://localhost:8080"
	@echo "  - Work Publisher:    http://localhost:8081"
	@echo "  - Bid Gateway:       http://localhost:8082"
	@echo "  - Bid Evaluator:     http://localhost:8083"
	@echo "  - Contract Engine:   http://localhost:8084"
	@echo "  - Provider Registry: http://localhost:8085"
	@echo "  - Trust Broker:      http://localhost:8086"
	@echo "  - Identity:          http://localhost:8087"
	@echo "  - Settlement:        http://localhost:8088"
	@echo "  - Telemetry:         http://localhost:8089"
	@echo ""
	@echo "MongoDB: mongodb://root:root@localhost:27017"
	@echo ""

# Development helpers
dev-work-publisher:
	@cd src/aex-work-publisher && \
		ENVIRONMENT=development \
		STORE_TYPE=memory \
		PORT=8081 \
		PROVIDER_REGISTRY_URL=http://localhost:8085 \
		go run ./src

dev-settlement:
	@cd src/aex-settlement && \
		ENVIRONMENT=development \
		MONGO_URI=mongodb://root:root@localhost:27017/?authSource=admin \
		MONGO_DB=aex \
		PORT=8088 \
		go run ./src

# Check service health
health:
	@echo "Checking service health..."
	@for port in 8080 8081 8082 8083 8084 8085 8086 8087 8088 8089; do \
		if curl -s http://localhost:$$port/health > /dev/null 2>&1; then \
			echo "✓ Port $$port: Healthy"; \
		else \
			echo "✗ Port $$port: Not responding"; \
		fi \
	done
