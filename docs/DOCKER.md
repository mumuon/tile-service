# Docker Setup for Tile Service

This guide explains how to run the tile-service in Docker containers as a persistent background service.

## Overview

The Docker setup runs the tile-service as a **long-running background service** that:
- ✅ Serves tiles over HTTP at `http://localhost:8080/tiles/`
- ✅ Provides REST API for tile generation
- ✅ Automatically restarts if it crashes (`restart: unless-stopped`)
- ✅ Starts with Docker daemon (persistent service)
- ✅ Includes health checks for monitoring

The Docker setup includes:
- **PostgreSQL 15** - Database for storing road geometries
- **Tile Service** - Go service running in serve mode (HTTP server)

## Quick Start

```bash
# Start all services (PostgreSQL + Tile Service)
# This runs as a background service
./docker-start.sh

# Check service status
./docker-status.sh

# Initialize database schema from the Next.js app
cd ../df && npm run db:push:local

# Generate tiles for a region (while service is running)
./docker-generate.sh oregon
```

Once started, the tile-service runs continuously in the background, serving tiles and accepting API requests.

## What's Included

### Services

1. **tile-service** (port 8080)
   - Serves tiles at http://localhost:8080/tiles/
   - REST API for tile generation
   - Connects to PostgreSQL for road geometry storage

2. **postgres** (port 5432)
   - PostgreSQL 15 database
   - Credentials: postgres/localdev
   - Database: postgres
   - Data persisted in Docker volume

### Network

Both services run on a shared Docker network called `drivefinder`, allowing them to communicate.

### Volumes

- `./tiles` → `/app/tiles` - Generated tiles (persisted on host)
- `./curvature-data` → `/app/curvature-data` - KMZ source files
- `postgres-data` - PostgreSQL data (Docker volume)

## Configuration

The tile-service is configured via environment variables in `docker-compose.yml`:

### Database Configuration
```yaml
DB_HOST=postgres           # Connect to postgres container
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=localdev
DB_NAME=postgres
DB_SSLMODE=disable
```

### S3/R2 Configuration (Optional)
Only needed if you want to upload tiles to cloud storage:
```yaml
S3_ENDPOINT=
S3_ACCESS_KEY_ID=
S3_SECRET_ACCESS_KEY=
S3_REGION=us-west-1
S3_BUCKET=drivefinder-tiles
```

## Usage

### Starting Services

```bash
# Start in detached mode
docker-compose up -d

# Start and view logs
docker-compose up

# Using the convenience script
./docker-start.sh
```

### Generating Tiles

```bash
# Using the convenience script
./docker-generate.sh oregon

# With custom options
./docker-generate.sh oregon --max-zoom 12

# Manually via docker-compose exec
docker-compose exec tile-service /app/tile-service generate oregon --skip-upload
```

### Viewing Logs

```bash
# All services
docker-compose logs -f

# Just tile-service
docker-compose logs -f tile-service

# Just postgres
docker-compose logs -f postgres
```

### Stopping Services

```bash
# Stop services (keeps containers)
docker-compose stop

# Stop and remove containers (keeps volumes)
docker-compose down

# Stop and remove everything including volumes
docker-compose down -v
```

### Rebuilding

If you change the Go code, rebuild the tile-service image:

```bash
# Rebuild and restart
docker-compose up -d --build

# Or rebuild without starting
docker-compose build
```

## Running Commands in Containers

### Tile Service Commands

```bash
# Generate tiles
docker-compose exec tile-service /app/tile-service generate oregon --skip-upload

# Extract geometries from existing tiles
docker-compose exec tile-service /app/tile-service extract /app/tiles/oregon

# Start interactive shell
docker-compose exec tile-service sh
```

### PostgreSQL Commands

```bash
# Connect to PostgreSQL
docker-compose exec postgres psql -U postgres

# Check database
docker-compose exec postgres psql -U postgres -c '\dt'

# Interactive shell
docker-compose exec postgres bash
```

## Health Checks

Both services include health checks:

- **tile-service**: Checks http://localhost:8080/health
- **postgres**: Runs pg_isready

Check health status:
```bash
docker-compose ps
```

## Troubleshooting

### Port Conflicts

If port 5432 or 8080 is already in use:

```bash
# Find what's using the port
lsof -i :5432
lsof -i :8080

# Stop local PostgreSQL
brew services stop postgresql@14

# Or change ports in docker-compose.yml
ports:
  - "5433:5432"  # Use different host port
```

### Database Connection Issues

```bash
# Check if postgres is running
docker-compose ps postgres

# View postgres logs
docker-compose logs postgres

# Test connection
docker-compose exec postgres psql -U postgres -c 'SELECT 1'
```

### Tile Service Issues

```bash
# View logs
docker-compose logs tile-service

# Restart service
docker-compose restart tile-service

# Check if it's listening
curl http://localhost:8080/health
```

### Rebuild from Scratch

```bash
# Stop and remove everything
docker-compose down -v

# Remove images
docker-compose down --rmi all -v

# Rebuild
docker-compose up -d --build
```

## File Structure

```
tile-service/
├── Dockerfile              # Multi-stage build for tile-service
├── docker-compose.yml      # Service definitions
├── docker-start.sh         # Quick start script
├── docker-generate.sh      # Generate tiles script
├── .dockerignore          # Files to exclude from image
├── tiles/                 # Generated tiles (mounted volume)
└── curvature-data/        # KMZ source files (mounted volume)
```

## Integration with Next.js App

The Next.js app in `../df` connects to:
- PostgreSQL at `localhost:5432` (via `.env.local`)
- Tile service at `http://localhost:8080/tiles`

Make sure the Next.js app's `.env.local` has:
```env
DATABASE_URL="postgres://postgres:localdev@localhost:5432/postgres"
NEXT_PUBLIC_TILE_URL=http://localhost:8080/tiles
```

## Production Deployment

For production, you would:
1. Use stronger passwords
2. Enable SSL for PostgreSQL
3. Use a managed database (not Docker)
4. Configure S3/R2 credentials for tile uploads
5. Set up proper networking and firewall rules

This Docker setup is designed for local development only.
