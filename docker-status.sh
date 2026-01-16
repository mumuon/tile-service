#!/bin/bash
# Check status of tile-service running in Docker

set -e

echo "🔍 Drive Finder Service Status"
echo "================================"
echo ""

# Check if Docker is running
if ! docker info > /dev/null 2>&1; then
    echo "❌ Docker is not running"
    exit 1
fi

echo "✓ Docker is running"
echo ""

# Check container status
echo "📦 Container Status:"
echo "-------------------"
docker-compose ps
echo ""

# Check health status
echo "🏥 Health Check:"
echo "----------------"
if curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo "✓ Tile service is healthy at http://localhost:8080"
    response=$(curl -s http://localhost:8080/health)
    echo "  Response: $response"
else
    echo "❌ Tile service is not responding at http://localhost:8080"
fi
echo ""

# Check PostgreSQL
echo "🗄️  Database Status:"
echo "-------------------"
if docker-compose exec -T postgres pg_isready -U postgres > /dev/null 2>&1; then
    echo "✓ PostgreSQL is ready"
else
    echo "❌ PostgreSQL is not ready"
fi
echo ""

# Show resource usage
echo "📊 Resource Usage:"
echo "------------------"
docker stats --no-stream drivefinder-tile-service drivefinder-postgres 2>/dev/null || echo "Containers not running"
echo ""

# Show recent logs
echo "📝 Recent Logs (last 10 lines):"
echo "--------------------------------"
docker-compose logs --tail=10 tile-service
echo ""

# Show available tiles
echo "📁 Available Tiles:"
echo "-------------------"
if [ -d "./tiles" ]; then
    for region in ./tiles/*/; do
        if [ -d "$region" ]; then
            region_name=$(basename "$region")
            tile_count=$(find "$region" -name "*.pbf" 2>/dev/null | wc -l | tr -d ' ')
            echo "  - $region_name: $tile_count tiles"
        fi
    done

    total_tiles=$(find ./tiles -name "*.pbf" 2>/dev/null | wc -l | tr -d ' ')
    echo ""
    echo "  Total: $total_tiles tiles"
else
    echo "  No tiles directory found"
fi
echo ""

# Show helpful commands
echo "💡 Useful Commands:"
echo "-------------------"
echo "  View live logs:       docker-compose logs -f tile-service"
echo "  Restart service:      docker-compose restart"
echo "  Stop service:         docker-compose down"
echo "  Generate tiles:       ./docker-generate.sh <region>"
echo "  Connect to postgres:  docker-compose exec postgres psql -U postgres"
echo ""
