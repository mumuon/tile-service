# Multi-stage build for Go tile service
# Stage 1: Build the binary
FROM golang:1.24.2-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source code
COPY *.go ./

# Build the binary
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 GOOS=linux go build -o tile-service .

# Stage 2: Build tippecanoe
FROM alpine:latest AS tippecanoe-builder

# Install build dependencies for tippecanoe
RUN apk add --no-cache \
    bash \
    git \
    build-base \
    sqlite-dev \
    zlib-dev

# Clone and build tippecanoe (pinned to specific version for caching)
RUN git clone --depth 1 --branch 2.72.0 https://github.com/felt/tippecanoe.git /tmp/tippecanoe && \
    cd /tmp/tippecanoe && \
    make -j$(nproc) && \
    make install

# Stage 3: Runtime environment
FROM alpine:latest

# Install runtime dependencies
# - PostgreSQL client libraries for database connectivity
# - ca-certificates for HTTPS
# - wget for health checks
# - sqlite-libs and zlib for tippecanoe runtime
# - libstdc++ and libgcc for C++ runtime (needed by tippecanoe)
RUN apk add --no-cache postgresql-client ca-certificates wget sqlite-libs zlib libstdc++ libgcc

# Create app directory
WORKDIR /app

# Create necessary directories
RUN mkdir -p /app/tiles /app/curvature-data

# Copy binary from builder
COPY --from=builder /build/tile-service /app/tile-service

# Copy tippecanoe from tippecanoe-builder
COPY --from=tippecanoe-builder /usr/local/bin/tippecanoe /usr/local/bin/tippecanoe
COPY --from=tippecanoe-builder /usr/local/bin/tippecanoe-enumerate /usr/local/bin/tippecanoe-enumerate
COPY --from=tippecanoe-builder /usr/local/bin/tippecanoe-decode /usr/local/bin/tippecanoe-decode
COPY --from=tippecanoe-builder /usr/local/bin/tile-join /usr/local/bin/tile-join

# Make binaries executable
RUN chmod +x /app/tile-service /usr/local/bin/tippecanoe*

# Expose default port for serve mode
EXPOSE 8080

# Default command is to serve tiles
CMD ["/app/tile-service", "serve", "-port", "8080"]
