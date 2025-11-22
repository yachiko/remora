.PHONY: help build test test-integration test-coverage run clean docker-build docker-run lint fmt vet

# Variables
BINARY_NAME=remora
DOCKER_IMAGE=remora
DOCKER_TAG=latest
GO_FILES=$(shell find . -name '*.go' -not -path "./vendor/*")

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the application binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p bin
	@go build -o bin/$(BINARY_NAME) ./cmd/remora

run: ## Run the application locally
	@echo "Running $(BINARY_NAME)..."
	@go run ./cmd/remora

test: ## Run unit tests
	@echo "Running unit tests..."
	@go test -v -short ./...

test-integration: ## Run integration tests
	@echo "Running integration tests..."
	@go test -v -run Integration ./...

test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	@go test -v -coverprofile=coverage.txt -covermode=atomic ./...
	@go tool cover -html=coverage.txt -o coverage.html
	@echo "Coverage report generated: coverage.html"

lint: ## Run golangci-lint
	@echo "Running linter..."
	@golangci-lint run ./...

fmt: ## Format Go code
	@echo "Formatting code..."
	@gofmt -s -w $(GO_FILES)

vet: ## Run go vet
	@echo "Running go vet..."
	@go vet ./...

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -rf dist/
	@rm -f coverage.txt coverage.html
	@rm -f *.db *.sqlite *.sqlite3

docker-build: ## Build Docker image
	@echo "Building Docker image..."
	@docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) -f deployments/docker/Dockerfile .

docker-run: ## Run Docker container
	@echo "Running Docker container..."
	@docker run --rm --env-file .env -p 8080:8080 $(DOCKER_IMAGE):$(DOCKER_TAG)

install-tools: ## Install development tools
	@echo "Installing development tools..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

.DEFAULT_GOAL := help
