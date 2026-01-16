#!/bin/bash
# Switch between local and production database environments

if [ "$1" == "local" ]; then
    echo "Switching to LOCAL database..."
    cp .env.local .env
    echo "✓ Using local postgres (postgres:5432)"
elif [ "$1" == "production" ] || [ "$1" == "prod" ]; then
    echo "Switching to PRODUCTION database..."
    cp .env.production .env
    echo "✓ Using production Supabase"
else
    echo "Usage: ./switch-env.sh [local|production]"
    echo ""
    echo "Current config:"
    grep "DB_HOST=" .env
    exit 1
fi

echo ""
echo "Restarting tile-service..."
docker compose up -d tile-service
echo ""
echo "Done! Check connection:"
docker compose logs tile-service --tail 5
