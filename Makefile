# TaskForge Makefile
.PHONY: help build test clean install lint format deps run-api run-worker run-scheduler docker-build docker-up docker-down

# Default target
.DEFAULT_GOAL := help

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOLINT=golangci-lint

# Build parameters
BINARY_DIR=bin
API_BINARY=$(BINARY_DIR)/taskforge-api
WORKER_BINARY=$(BINARY_DIR)/taskforge-worker
SCHEDULER_BINARY=$(BINARY_DIR)/taskforge-scheduler
CLI_BINARY=$(BINARY_DIR)/taskforge-cli

# Docker parameters
DOCKER_REGISTRY=2bxtech
IMAGE_TAG=latest

# Colors for output
RED=\033[0;31m
GREEN=\033[0;32m
YELLOW=\033[1;33m
BLUE=\033[0;34m
NC=\033[0m # No Color

help: ## Show this help message
	@echo "$(BLUE)TaskForge - Distributed Task Queue System$(NC)"
	@echo ""
	@echo "$(YELLOW)Available commands:$(NC)"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  $(GREEN)%-15s$(NC) %s\n", $$1, $$2}' $(MAKEFILE_LIST)

deps: ## Download and install dependencies
	@echo "$(BLUE)Installing dependencies...$(NC)"
	$(GOMOD) download
	$(GOMOD) tidy
	@echo "$(GREEN)Dependencies installed successfully!$(NC)"

build: deps ## Build all binaries
	@echo "$(BLUE)Building all binaries...$(NC)"
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) -o $(API_BINARY) ./cmd/api
	$(GOBUILD) -o $(WORKER_BINARY) ./cmd/worker
	$(GOBUILD) -o $(SCHEDULER_BINARY) ./cmd/scheduler
	$(GOBUILD) -o $(CLI_BINARY) ./cmd/cli
	@echo "$(GREEN)All binaries built successfully!$(NC)"

build-api: deps ## Build API server binary
	@echo "$(BLUE)Building API server...$(NC)"
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) -o $(API_BINARY) ./cmd/api
	@echo "$(GREEN)API server built successfully!$(NC)"

build-worker: deps ## Build worker binary
	@echo "$(BLUE)Building worker...$(NC)"
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) -o $(WORKER_BINARY) ./cmd/worker
	@echo "$(GREEN)Worker built successfully!$(NC)"

build-scheduler: deps ## Build scheduler binary
	@echo "$(BLUE)Building scheduler...$(NC)"
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) -o $(SCHEDULER_BINARY) ./cmd/scheduler
	@echo "$(GREEN)Scheduler built successfully!$(NC)"

build-cli: deps ## Build CLI binary
	@echo "$(BLUE)Building CLI...$(NC)"
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) -o $(CLI_BINARY) ./cmd/cli
	@echo "$(GREEN)CLI built successfully!$(NC)"

test: ## Run all tests
	@echo "$(BLUE)Running tests...$(NC)"
	$(GOTEST) -v ./...
	@echo "$(GREEN)All tests passed!$(NC)"

test-coverage: ## Run tests with coverage
	@echo "$(BLUE)Running tests with coverage...$(NC)"
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)Coverage report generated: coverage.html$(NC)"

test-integration: ## Run integration tests
	@echo "$(BLUE)Running integration tests...$(NC)"
	$(GOTEST) -v -tags=integration ./tests/...
	@echo "$(GREEN)Integration tests passed!$(NC)"

benchmark: ## Run benchmarks
	@echo "$(BLUE)Running benchmarks...$(NC)"
	$(GOTEST) -bench=. -benchmem ./...
	@echo "$(GREEN)Benchmarks completed!$(NC)"

lint: ## Run linter
	@echo "$(BLUE)Running linter...$(NC)"
	$(GOLINT) run ./...
	@echo "$(GREEN)Linting completed!$(NC)"

format: ## Format Go code
	@echo "$(BLUE)Formatting code...$(NC)"
	$(GOFMT) -s -w .
	@echo "$(GREEN)Code formatted!$(NC)"

clean: ## Clean build artifacts
	@echo "$(BLUE)Cleaning build artifacts...$(NC)"
	$(GOCLEAN)
	rm -rf $(BINARY_DIR)
	rm -f coverage.out coverage.html
	@echo "$(GREEN)Clean completed!$(NC)"

install: build ## Install binaries to $GOPATH/bin
	@echo "$(BLUE)Installing binaries...$(NC)"
	cp $(API_BINARY) $(GOPATH)/bin/
	cp $(WORKER_BINARY) $(GOPATH)/bin/
	cp $(SCHEDULER_BINARY) $(GOPATH)/bin/
	cp $(CLI_BINARY) $(GOPATH)/bin/
	@echo "$(GREEN)Binaries installed successfully!$(NC)"

run-api: build-api ## Run the API server
	@echo "$(BLUE)Starting API server...$(NC)"
	./$(API_BINARY)

run-worker: build-worker ## Run a worker
	@echo "$(BLUE)Starting worker...$(NC)"
	./$(WORKER_BINARY)

run-scheduler: build-scheduler ## Run the scheduler
	@echo "$(BLUE)Starting scheduler...$(NC)"
	./$(SCHEDULER_BINARY)

dev-setup: ## Set up development environment
	@echo "$(BLUE)Setting up development environment...$(NC)"
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "Installing golangci-lint..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin v1.54.2; \
	}
	@command -v docker >/dev/null 2>&1 || { \
		echo "$(YELLOW)Warning: Docker not found. Please install Docker for full development experience.$(NC)"; \
	}
	@command -v docker-compose >/dev/null 2>&1 || { \
		echo "$(YELLOW)Warning: Docker Compose not found. Please install Docker Compose for full development experience.$(NC)"; \
	}
	@echo "$(GREEN)Development environment setup completed!$(NC)"

docker-build: ## Build Docker images
	@echo "$(BLUE)Building Docker images...$(NC)"
	docker build -t $(DOCKER_REGISTRY)/taskforge-api:$(IMAGE_TAG) -f deployments/docker/Dockerfile.api .
	docker build -t $(DOCKER_REGISTRY)/taskforge-worker:$(IMAGE_TAG) -f deployments/docker/Dockerfile.worker .
	docker build -t $(DOCKER_REGISTRY)/taskforge-scheduler:$(IMAGE_TAG) -f deployments/docker/Dockerfile.scheduler .
	@echo "$(GREEN)Docker images built successfully!$(NC)"

docker-push: docker-build ## Build and push Docker images
	@echo "$(BLUE)Pushing Docker images...$(NC)"
	docker push $(DOCKER_REGISTRY)/taskforge-api:$(IMAGE_TAG)
	docker push $(DOCKER_REGISTRY)/taskforge-worker:$(IMAGE_TAG)
	docker push $(DOCKER_REGISTRY)/taskforge-scheduler:$(IMAGE_TAG)
	@echo "$(GREEN)Docker images pushed successfully!$(NC)"

docker-up: ## Start services with Docker Compose
	@echo "$(BLUE)Starting services with Docker Compose...$(NC)"
	docker-compose up -d
	@echo "$(GREEN)Services started successfully!$(NC)"

docker-down: ## Stop services with Docker Compose
	@echo "$(BLUE)Stopping services with Docker Compose...$(NC)"
	docker-compose down
	@echo "$(GREEN)Services stopped successfully!$(NC)"

docker-logs: ## Show Docker Compose logs
	@echo "$(BLUE)Showing Docker Compose logs...$(NC)"
	docker-compose logs -f

security-scan: ## Run security scan
	@echo "$(BLUE)Running security scan...$(NC)"
	@command -v gosec >/dev/null 2>&1 || { \
		echo "Installing gosec..."; \
		go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest; \
	}
	gosec ./...
	@echo "$(GREEN)Security scan completed!$(NC)"

mod-update: ## Update all dependencies
	@echo "$(BLUE)Updating dependencies...$(NC)"
	$(GOGET) -u ./...
	$(GOMOD) tidy
	@echo "$(GREEN)Dependencies updated!$(NC)"

proto-gen: ## Generate protobuf files (if using gRPC)
	@echo "$(BLUE)Generating protobuf files...$(NC)"
	# Add protobuf generation commands here when we implement gRPC
	@echo "$(GREEN)Protobuf files generated!$(NC)"

release: clean test lint ## Prepare a release build
	@echo "$(BLUE)Preparing release build...$(NC)"
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -ldflags="-w -s" -o $(API_BINARY)-linux-amd64 ./cmd/api
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -ldflags="-w -s" -o $(WORKER_BINARY)-linux-amd64 ./cmd/worker
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -ldflags="-w -s" -o $(SCHEDULER_BINARY)-linux-amd64 ./cmd/scheduler
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -ldflags="-w -s" -o $(CLI_BINARY)-linux-amd64 ./cmd/cli
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) -ldflags="-w -s" -o $(API_BINARY)-windows-amd64.exe ./cmd/api
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) -ldflags="-w -s" -o $(WORKER_BINARY)-windows-amd64.exe ./cmd/worker
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) -ldflags="-w -s" -o $(SCHEDULER_BINARY)-windows-amd64.exe ./cmd/scheduler
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) -ldflags="-w -s" -o $(CLI_BINARY)-windows-amd64.exe ./cmd/cli
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) -ldflags="-w -s" -o $(API_BINARY)-darwin-amd64 ./cmd/api
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) -ldflags="-w -s" -o $(WORKER_BINARY)-darwin-amd64 ./cmd/worker
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) -ldflags="-w -s" -o $(SCHEDULER_BINARY)-darwin-amd64 ./cmd/scheduler
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) -ldflags="-w -s" -o $(CLI_BINARY)-darwin-amd64 ./cmd/cli
	@echo "$(GREEN)Release build completed!$(NC)"

watch: ## Watch for changes and rebuild
	@echo "$(BLUE)Watching for changes...$(NC)"
	@command -v air >/dev/null 2>&1 || { \
		echo "Installing air for hot reloading..."; \
		go install github.com/cosmtrek/air@latest; \
	}
	air

init-project: ## Initialize a new development environment
	@echo "$(BLUE)Initializing TaskForge development environment...$(NC)"
	@echo "Creating necessary directories..."
	@mkdir -p {cmd/{api,worker,scheduler,cli},internal/{queue,worker,scheduler,task,api},pkg/{client,types},deployments/{k8s,docker,helm},examples,tests,docs}
	@echo "Setting up git hooks..."
	@git config core.hooksPath .githooks 2>/dev/null || true
	@echo "$(GREEN)Project initialization completed!$(NC)"
	@echo "$(YELLOW)Next steps:$(NC)"
	@echo "  1. Run 'make dev-setup' to install development tools"
	@echo "  2. Run 'make deps' to install Go dependencies"
	@echo "  3. Run 'make docker-up' to start development services"
	@echo "  4. Run 'make build' to build all components"

# Generate version info
version:
	@echo "TaskForge Development Build"
	@echo "Go version: $(shell go version)"
	@echo "Git commit: $(shell git rev-parse --short HEAD 2>/dev/null || echo 'unknown')"
	@echo "Build time: $(shell date -u '+%Y-%m-%d %H:%M:%S UTC')"

# Show project statistics
stats:
	@echo "$(BLUE)Project Statistics:$(NC)"
	@echo "Go files: $(shell find . -name '*.go' | wc -l)"
	@echo "Lines of code: $(shell find . -name '*.go' -exec wc -l {} + | tail -1 | awk '{print $$1}')"
	@echo "Test files: $(shell find . -name '*_test.go' | wc -l)"
	@echo "Packages: $(shell go list ./... | wc -l)"
