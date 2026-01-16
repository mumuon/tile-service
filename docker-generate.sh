#!/bin/bash
# Generate tiles using the Docker container

set -e

if [ -z "$1" ]; then
    echo "Usage: ./docker-generate.sh <region> [additional-flags]"
    echo ""
    echo "Examples:"
    echo "  ./docker-generate.sh oregon"
    echo "  ./docker-generate.sh oregon --skip-upload"
    echo "  ./docker-generate.sh oregon --max-zoom 12 --skip-upload"
    exit 1
fi

REGION=$1
shift  # Remove first argument, remaining args are passed through

echo "🚀 Generating tiles for region: $REGION"
echo ""

# Check if services are running
if ! docker-compose ps | grep -q "drivefinder-tile-service.*Up"; then
    echo "⚠️  Tile service is not running. Starting services..."
    docker-compose up -d
    echo "Waiting for services to be ready..."
    sleep 3
fi

# Run tile generation inside the container
echo "Running tile generation..."
docker-compose exec tile-service /app/tile-service generate "$REGION" --skip-upload "$@"

echo ""
echo "✓ Tile generation complete!"
echo ""
echo "Tiles are stored in: ./tiles/$REGION/"
echo ""
echo "To view in the app:"
echo "  1. Make sure the tile service is running: docker-compose up -d"
echo "  2. Start the Next.js app: cd ../df && npm run dev"
echo "  3. Open http://localhost:3000"
echo ""
