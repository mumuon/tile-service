.PHONY: build build-linux build-macos build-windows clean run help

# Default target
help:
	@echo "Tile Service Build Targets"
	@echo "=========================="
	@echo ""
	@echo "make build          - Build for current platform (default: macOS arm64)"
	@echo "make build-linux    - Build for Linux (amd64)"
	@echo "make build-windows  - Build for Windows (amd64)"
	@echo "make build-macos    - Build for macOS (arm64)"
	@echo "make build-all      - Build for all platforms"
	@echo "make clean          - Remove built binaries"
	@echo "make run            - Build and run locally (requires .env)"
	@echo "make docker-build   - Build Docker image"
	@echo "make docker-run     - Run service in Docker container"
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
