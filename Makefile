.PHONY: build install clean test run fmt vet

# Binary name
BINARY=feelpulse

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
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

# Main package
MAIN_PKG=./cmd/feelpulse

all: build

build:
	@echo "🔨 Building $(BINARY)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) $(MAIN_PKG)
	@echo "✅ Built: $(BUILD_DIR)/$(BINARY)"

install:
	@echo "📦 Installing $(BINARY)..."
	$(GOINSTALL) $(LDFLAGS) $(MAIN_PKG)
	@echo "✅ Installed to $(shell go env GOPATH)/bin/$(BINARY)"

clean:
	@echo "🧹 Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	@echo "✅ Clean"

test:
	@echo "🧪 Running tests..."
	$(GOTEST) -v ./...

run: build
	@echo "🚀 Running $(BINARY)..."
	$(BUILD_DIR)/$(BINARY) start

fmt:
	@echo "📝 Formatting code..."
	$(GOFMT) ./...

vet:
	@echo "🔍 Vetting code..."
	$(GOVET) ./...

deps:
	@echo "📥 Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Quick dev commands
dev: fmt vet build run

check: fmt vet test
