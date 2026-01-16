#!/bin/bash
# Quick Docker testing - fast iteration without full rebuild
# Usage: ./scripts/test/test-docker.sh [region]

set -e

REGION="${1:-test-region}"
CONTAINER_NAME="tile-service-container"

echo "========================================="
echo "  Quick Docker Test"
echo "========================================="
echo "Region: $REGION"
echo ""

# Step 1: Build binary for Linux
echo "Step 1: Building Linux binary..."
echo "----------------------------------------"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o tile-service-linux
echo "✓ Built tile-service-linux"
echo "  Size: $(du -h tile-service-linux | cut -f1)"
echo ""

# Step 2: Check if container is running
echo "Step 2: Checking container status..."
echo "----------------------------------------"

if docker ps -a | grep -q "$CONTAINER_NAME"; then
    CONTAINER_STATUS=$(docker inspect -f '{{.State.Status}}' "$CONTAINER_NAME")
    echo "Container exists (status: $CONTAINER_STATUS)"

    if [ "$CONTAINER_STATUS" != "running" ]; then
        echo "Starting container..."
        docker start "$CONTAINER_NAME"
        sleep 2
    fi
else
    echo "Container not found. Creating new container..."
    docker run -d \
        --name "$CONTAINER_NAME" \
        --env-file .env \
        -v "$(pwd)/curvature-data:/app/curvature-data" \
        -v "$(pwd)/public/tiles:/app/tiles" \
        drivefinder/tile-service:latest \
        tail -f /dev/null

    sleep 2
fi

echo "✓ Container is running"
echo ""

# Step 3: Copy new binary to container
echo "Step 3: Updating binary in container..."
echo "----------------------------------------"
docker cp tile-service-linux "$CONTAINER_NAME:/app/tile-service"
docker exec "$CONTAINER_NAME" chmod +x /app/tile-service
echo "✓ Binary updated in container"
echo ""

# Step 4: Run test in container
echo "Step 4: Running tile generation in container..."
echo "----------------------------------------"
echo "Command: /app/tile-service -skip-upload -max-zoom 10 $REGION"
echo ""

docker exec "$CONTAINER_NAME" /app/tile-service \
    -skip-upload \
    -max-zoom 10 \
    -no-cleanup \
    "$REGION"

echo ""
echo "✓ Test complete"
echo ""

# Step 5: Verify output
echo "Step 5: Verifying output in container..."
echo "----------------------------------------"

# List tiles
TILE_COUNT=$(docker exec "$CONTAINER_NAME" sh -c "find /app/tiles/$REGION -name '*.pbf' 2>/dev/null | wc -l" | tr -d ' ')
echo "Generated tiles: $TILE_COUNT files"

if [ "$TILE_COUNT" -gt 0 ]; then
    echo ""
    echo "Tile structure in container:"
    docker exec "$CONTAINER_NAME" sh -c "find /app/tiles/$REGION -type d | head -10"

    echo ""
    echo "Sample tiles:"
    docker exec "$CONTAINER_NAME" sh -c "find /app/tiles/$REGION -name '*.pbf' | head -5"
fi

echo ""
echo "========================================="
echo "  Docker Test Complete ✓"
echo "========================================="
echo ""
echo "Container: $CONTAINER_NAME (still running)"
echo "To stop: docker stop $CONTAINER_NAME"
echo "To remove: docker rm $CONTAINER_NAME"
echo ""
echo "To view logs: docker logs $CONTAINER_NAME"
echo "To exec into container: docker exec -it $CONTAINER_NAME sh"
