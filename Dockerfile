# Multi-stage build for Go tile service
# Stage 1: Build the binary
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY *.go ./

# Build the binary
RUN CGO_ENABLED=1 GOOS=linux go build -o tile-service .

# Stage 2: Runtime environment
FROM alpine:latest

# Install runtime dependencies
# - PostgreSQL client libraries for psycopg2-like connectivity
# - tippecanoe for tile generation
RUN apk add --no-cache postgresql-client tippecanoe ca-certificates

# Create app directory
WORKDIR /app

# Create necessary directories
RUN mkdir -p /app/curvature-data /app/public/tiles

# Copy binary from builder
COPY --from=builder /build/tile-service /app/tile-service

# Make binary executable
RUN chmod +x /app/tile-service

# Set entrypoint
ENTRYPOINT ["/app/tile-service"]

# Default command - generate tiles for region specified via REGION env var
# Can be overridden: docker run ... tile-service washington -max-zoom 14
CMD ["-config", ".env", "-debug", "${REGION:-maryland}"]
