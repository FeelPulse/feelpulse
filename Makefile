.PHONY: build install clean test run setup start stop restart logs status reset fmt vet lint deps dev check help docker-build docker-run docker-stop docker-push bench test-integration

# Binary name
BINARY=fp

# Build directory
BUILD_DIR=./build

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOINSTALL=$(GOCMD) install
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet

# Version info
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.gitCommit=$(GIT_COMMIT)"

# Main package
MAIN_PKG=./cmd/feelpulse

all: build

## Build & Install

build: ## Build binary to ./build/fp
	@echo "🔨 Building $(BINARY) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) $(MAIN_PKG)
	@echo "✅ Built: $(BUILD_DIR)/$(BINARY)"

install: build ## Install binary to GOPATH/bin
	@echo "📦 Installing $(BINARY)..."
	$(GOINSTALL) $(LDFLAGS) $(MAIN_PKG)
	@echo "✅ Installed to $(shell go env GOPATH)/bin/$(BINARY)"

## Gateway Management (uses built binary)

setup: build ## Run initial setup (creates config, starts daemon)
	@echo "⚙️  Running setup..."
	$(BUILD_DIR)/$(BINARY) setup

start: build ## Start gateway daemon
	@echo "🫀 Starting gateway..."
	$(BUILD_DIR)/$(BINARY) start

stop: build ## Stop gateway daemon
	@echo "🛑 Stopping gateway..."
	$(BUILD_DIR)/$(BINARY) stop

restart: build ## Restart gateway daemon
	@echo "🔄 Restarting gateway..."
	$(BUILD_DIR)/$(BINARY) restart

status: build ## Show gateway status
	$(BUILD_DIR)/$(BINARY) status

logs: build ## View gateway logs (live, Ctrl+C to exit)
	$(BUILD_DIR)/$(BINARY) logs

reset: build ## Clear all memory and sessions (requires confirmation)
	@echo "⚠️  This will clear all data!"
	$(BUILD_DIR)/$(BINARY) reset

## Development

run: build start ## Build and start gateway (alias for: make build start)

dev: fmt vet build start ## Format, vet, build, and start

test: ## Run all tests
	@echo "🧪 Running tests..."
	$(GOTEST) -v -race ./...

test-short: ## Run tests (no race detector, faster)
	$(GOTEST) ./...

test-integration: build ## Run integration tests
	@echo "🧪 Running integration tests..."
	$(GOTEST) -v -tags=integration ./cmd/feelpulse/...

bench: ## Run benchmarks
	@echo "⚡ Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./internal/session/...

bench-all: ## Run all benchmarks with full output
	@echo "⚡ Running all benchmarks..."
	$(GOTEST) -bench=. -benchmem -benchtime=3s ./...

fmt: ## Format code
	@echo "📝 Formatting..."
	$(GOFMT) ./...

vet: ## Vet code
	@echo "🔍 Vetting..."
	$(GOVET) ./...

lint: ## Run golangci-lint (requires golangci-lint installed)
	@which golangci-lint > /dev/null || (echo "❌ Install: brew install golangci-lint" && exit 1)
	golangci-lint run ./...

deps: ## Download and tidy dependencies
	@echo "📥 Tidying dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

clean: ## Remove build artifacts
	@echo "🧹 Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	@echo "✅ Clean"

check: fmt vet test ## Format, vet, and run all tests

release: check build ## Full check + build

## Docker

DOCKER_IMAGE=feelpulse
DOCKER_TAG=latest
DOCKER_REGISTRY=

docker-build: ## Build Docker image
	@echo "🐳 Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	@echo "✅ Built: $(DOCKER_IMAGE):$(DOCKER_TAG)"

docker-run: ## Run Docker container
	@echo "🐳 Starting Docker container..."
	docker run -d --name feelpulse \
		-p 18789:18789 \
		-v ~/.feelpulse:/home/feelpulse/.feelpulse \
		$(DOCKER_IMAGE):$(DOCKER_TAG)
	@echo "✅ Container started: feelpulse"

docker-stop: ## Stop Docker container
	@echo "🛑 Stopping Docker container..."
	docker stop feelpulse && docker rm feelpulse || true
	@echo "✅ Container stopped"

docker-push: docker-build ## Build and push Docker image to registry
	@if [ -z "$(DOCKER_REGISTRY)" ]; then \
		echo "⚠️  DOCKER_REGISTRY not set. Usage: make docker-push DOCKER_REGISTRY=ghcr.io/username"; \
		exit 1; \
	fi
	@echo "🐳 Pushing to $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_TAG)..."
	docker tag $(DOCKER_IMAGE):$(DOCKER_TAG) $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_TAG)
	docker push $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_TAG)
	@echo "✅ Pushed"

docker-compose-up: ## Start with docker-compose
	docker-compose up -d

docker-compose-down: ## Stop docker-compose
	docker-compose down

## Help

help: ## Show this help
	@echo "FeelPulse Makefile Commands:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Quick Start:"
	@echo "  make setup         # First-time setup"
	@echo "  make start         # Start gateway"
	@echo "  make logs          # View logs"
	@echo "  make stop          # Stop gateway"
	@echo ""
	@echo "Development:"
	@echo "  make dev           # Format + vet + build + start"
	@echo "  make check         # Format + vet + test"
	@echo "  make test          # Run all tests"
