.PHONY: build build-linux build-macos build-windows clean run help
.PHONY: test test-unit test-unit-coverage test-converter test-extractor test-api test-integration test-docker test-watch test-all

# Default target
help:
	@echo "Tile Service Build & Test Targets"
	@echo "=================================="
	@echo ""
	@echo "Build Targets:"
	@echo "  make build          - Build for current platform (default: macOS arm64)"
	@echo "  make build-linux    - Build for Linux (amd64)"
	@echo "  make build-windows  - Build for Windows (amd64)"
	@echo "  make build-macos    - Build for macOS (arm64)"
	@echo "  make build-all      - Build for all platforms"
	@echo "  make clean          - Remove built binaries"
	@echo "  make run            - Build and run locally (requires .env)"
	@echo ""
	@echo "Test Targets:"
	@echo "  make test           - Run all unit tests"
	@echo "  make test-unit      - Run unit tests with verbose output"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make test-converter - Test KML to GeoJSON converter"
	@echo "  make test-extractor - Test geometry extractor"
	@echo "  make test-api       - Test API server endpoints"
	@echo "  make test-integration - Full pipeline integration test"
	@echo "  make test-docker    - Quick Docker test"
	@echo "  make test-watch     - Watch mode for continuous testing"
	@echo "  make test-all       - Run all tests (unit + components + integration)"
	@echo ""
	@echo "Docker Targets:"
	@echo "  make docker-build   - Build Docker image"
	@echo "  make docker-run     - Run service in Docker container"
	@echo ""

# Build variables
BINARY_NAME=tile-service
VERSION?=0.1.0
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# Platform-specific variables
LINUX_BINARY=$(BINARY_NAME)-linux-amd64
MACOS_BINARY=$(BINARY_NAME)-darwin-arm64
WINDOWS_BINARY=$(BINARY_NAME)-windows-amd64.exe

# Default build target (current platform)
build: build-macos

# Build for Linux
build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build $(LDFLAGS) -o $(LINUX_BINARY) .
	@echo "✓ Built $(LINUX_BINARY)"

# Build for macOS (ARM64)
build-macos:
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(MACOS_BINARY) .
	@echo "✓ Built $(MACOS_BINARY)"

# Build for Windows
build-windows:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o $(WINDOWS_BINARY) .
	@echo "✓ Built $(WINDOWS_BINARY)"

# Build for all platforms
build-all: build-linux build-macos build-windows
	@echo "✓ Built all platform binaries"

# Clean build artifacts
clean:
	rm -f $(LINUX_BINARY) $(MACOS_BINARY) $(WINDOWS_BINARY) $(BINARY_NAME)
	@echo "✓ Cleaned build artifacts"

# Run locally (requires .env file)
run: build
	./$(MACOS_BINARY)

# Docker build
docker-build:
	docker build -t drivefinder/tile-service:$(VERSION) .
	docker tag drivefinder/tile-service:$(VERSION) drivefinder/tile-service:latest
	@echo "✓ Built Docker image drivefinder/tile-service:$(VERSION)"

# Docker run
docker-run: docker-build
	docker run --env-file .env \
		-v ./curvature-data:/app/curvature-data \
		-v ./public/tiles:/app/public/tiles \
		drivefinder/tile-service:latest
	@echo "✓ Running Docker container"

# Test compilation
test-build:
	go build -o /dev/null .
	@echo "✓ Compilation successful"

# Test targets
test:
	@echo "Running unit tests..."
	go test ./...

test-unit:
	@echo "Running unit tests (verbose)..."
	./scripts/test/test-unit.sh -v

test-coverage:
	@echo "Running tests with coverage..."
	./scripts/test/test-unit.sh -v -c

test-coverage-html:
	@echo "Running tests with coverage (HTML report)..."
	./scripts/test/test-unit.sh -h

test-converter:
	@echo "Testing converter component..."
	./scripts/test/test-converter.sh

test-extractor:
	@echo "Testing geometry extractor..."
	./scripts/test/test-extractor.sh

test-api:
	@echo "Testing API server..."
	./scripts/test/test-api.sh

test-integration:
	@echo "Running integration test..."
	./scripts/test/test-integration.sh

test-integration-cleanup:
	@echo "Running integration test with cleanup..."
	./scripts/test/test-integration.sh test-region --cleanup

test-docker:
	@echo "Running Docker test..."
	./scripts/test/test-docker.sh

test-watch:
	@echo "Starting watch mode..."
	./scripts/test/watch-tests.sh

test-all: test-unit test-converter test-extractor test-integration
	@echo ""
	@echo "========================================="
	@echo "  All Tests Complete ✓"
	@echo "========================================="
