#!/bin/bash
# Start tile-service and PostgreSQL in Docker

set -e

echo "🚀 Starting Drive Finder Tile Service in Docker"
echo ""

# Check if Docker is running
if ! docker info > /dev/null 2>&1; then
    echo "❌ Docker is not running. Please start Docker and try again."
    exit 1
fi

# Check if docker-compose is available
if ! command -v docker-compose &> /dev/null; then
    echo "❌ docker-compose not found. Please install docker-compose."
    exit 1
fi

# Remove old standalone postgres container if it exists
if docker ps -a --format '{{.Names}}' | grep -q '^drivefinder-postgres$'; then
    echo "🧹 Removing old standalone postgres container..."
    docker rm -f drivefinder-postgres > /dev/null 2>&1 || true
fi

# Build and start services
echo "📦 Building and starting services..."
docker-compose up -d --build

echo ""
echo "✓ Services started successfully!"
echo ""
echo "Services running:"
echo "  - PostgreSQL: localhost:5432"
echo "  - Tile Service API: http://localhost:8080"
echo ""
echo "📋 Useful commands:"
echo ""
echo "View logs:"
echo "  docker-compose logs -f tile-service"
echo "  docker-compose logs -f postgres"
echo ""
echo "Stop services:"
echo "  docker-compose down"
echo ""
echo "Generate tiles (from within container):"
echo "  docker-compose exec tile-service /app/tile-service generate <region> --skip-upload"
echo ""
echo "Access tile service shell:"
echo "  docker-compose exec tile-service sh"
echo ""
echo "To initialize the database schema from the Next.js app:"
echo "  cd ../df && npm run db:push:local"
echo ""
