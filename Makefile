.PHONY: build install clean test run start stop restart logs status tui fmt vet lint deps dev check help install-service uninstall-service

# Binary name
BINARY=feelpulse

# Build directory
BUILD_DIR=./build

# PID file for background process
PID_FILE=/tmp/feelpulse.pid
LOG_FILE=/tmp/feelpulse.log

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

build: ## Build binary to ./build/feelpulse
	@echo "🔨 Building $(BINARY) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) $(MAIN_PKG)
	@echo "✅ Built: $(BUILD_DIR)/$(BINARY)"

install: build ## Install binary to GOPATH/bin
	@echo "📦 Installing $(BINARY)..."
	$(GOINSTALL) $(LDFLAGS) $(MAIN_PKG)
	@echo "✅ Installed to $(shell go env GOPATH)/bin/$(BINARY)"

## Run

start: build ## Build and start gateway in foreground
	@echo "🫀 Starting FeelPulse..."
	$(BUILD_DIR)/$(BINARY) start

start-bg: build ## Build and start gateway in background (logs → /tmp/feelpulse.log)
	@echo "🫀 Starting FeelPulse in background..."
	@$(BUILD_DIR)/$(BINARY) start > $(LOG_FILE) 2>&1 & echo $$! > $(PID_FILE)
	@echo "✅ Started (PID $$(cat $(PID_FILE)))"
	@echo "📋 Logs: tail -f $(LOG_FILE)"

stop: ## Stop background gateway
	@if [ -f $(PID_FILE) ]; then \
		PID=$$(cat $(PID_FILE)); \
		echo "🛑 Stopping FeelPulse (PID $$PID)..."; \
		kill $$PID 2>/dev/null && rm -f $(PID_FILE) && echo "✅ Stopped"; \
	else \
		echo "⚠️  No PID file found (not running in background?)"; \
	fi

restart: stop start-bg ## Restart background gateway

## systemd service

install-service: build ## Install and enable systemd service (user mode)
	@echo "📦 Installing systemd service..."
	$(BUILD_DIR)/$(BINARY) service install
	$(BUILD_DIR)/$(BINARY) service enable
	@echo "✅ Service installed and enabled"
	@echo "💡 Start with: systemctl --user start feelpulse"

uninstall-service: build ## Uninstall systemd service
	@echo "🗑️  Uninstalling systemd service..."
	$(BUILD_DIR)/$(BINARY) service uninstall
	@echo "✅ Service uninstalled"

logs: ## Tail gateway logs
	@tail -f $(LOG_FILE)

status: build ## Show gateway status
	$(BUILD_DIR)/$(BINARY) status

tui: build ## Launch interactive terminal chat
	$(BUILD_DIR)/$(BINARY) tui

auth: build ## Configure authentication (API key or setup-token)
	$(BUILD_DIR)/$(BINARY) auth

init: build ## Initialize config
	$(BUILD_DIR)/$(BINARY) init

## Development

test: ## Run all tests
	@echo "🧪 Running tests..."
	$(GOTEST) -v -race ./...

test-short: ## Run tests (no race detector, faster)
	$(GOTEST) ./...

fmt: ## Format code
	@echo "📝 Formatting..."
	$(GOFMT) ./...

vet: ## Vet code
	@echo "🔍 Vetting..."
	$(GOVET) ./...

lint: ## Run golangci-lint (requires golangci-lint installed)
	@which golangci-lint > /dev/null || (echo "Install: brew install golangci-lint" && exit 1)
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

## Combos

dev: fmt vet build start ## Format, vet, build, and start (foreground)

check: fmt vet test ## Format, vet, and run all tests

release: check build ## Full check + build

## Help

help: ## Show this help
	@echo "FeelPulse Makefile"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
