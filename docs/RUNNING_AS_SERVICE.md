# Running Tile Service as a Long-Running Service

The tile-service is designed to run as a persistent background service that serves tiles over HTTP. This document explains how to run it as a service using Docker.

## Overview

When running as a service, the tile-service:
- Serves tiles over HTTP at `http://localhost:8080/tiles/`
- Provides a REST API for tile generation jobs
- Automatically restarts if it crashes
- Connects to PostgreSQL for road geometry data
- Serves tiles from the mounted `./tiles/` directory

## Quick Start

```bash
# Start as a background service
docker-compose up -d

# Check status
docker-compose ps

# View logs
docker-compose logs -f tile-service
```

The service is now running and will:
- ✅ Serve tiles from `./tiles/` directory
- ✅ Accept REST API requests for tile generation
- ✅ Automatically restart if it crashes
- ✅ Start automatically when Docker starts (unless manually stopped)

## Service Endpoints

Once running, the tile-service exposes:

### Tile Server
- **URL**: `http://localhost:8080/tiles/{region}/{z}/{x}/{y}.pbf`
- **Example**: `http://localhost:8080/tiles/oregon/10/163/357.pbf`
- **Purpose**: Serves vector tile files to the Next.js app

### REST API
- **POST** `http://localhost:8080/api/generate` - Submit tile generation job
- **GET** `http://localhost:8080/api/jobs` - List all jobs
- **GET** `http://localhost:8080/api/jobs/{jobId}` - Get job status
- **GET** `http://localhost:8080/api/stream/{jobId}` - Stream job updates (SSE)
- **GET** `http://localhost:8080/health` - Health check

### Health Check

```bash
curl http://localhost:8080/health
```

Expected response:
```json
{"status": "ok"}
```

## Service Management

### Start Service
```bash
# Start in background (detached mode)
docker-compose up -d

# Start in foreground (see logs)
docker-compose up
```

### Stop Service
```bash
# Stop but keep containers
docker-compose stop

# Stop and remove containers
docker-compose down
```

### Restart Service
```bash
# Restart both tile-service and postgres
docker-compose restart

# Restart only tile-service
docker-compose restart tile-service
```

### View Status
```bash
# Check if services are running
docker-compose ps

# Should show:
# NAME                          STATUS
# drivefinder-postgres          Up (healthy)
# drivefinder-tile-service      Up (healthy)
```

### View Logs
```bash
# Follow logs (tail -f style)
docker-compose logs -f tile-service

# View last 100 lines
docker-compose logs --tail=100 tile-service

# View logs from both services
docker-compose logs -f
```

## Generating Tiles While Service Runs

You can generate tiles while the service is running by executing commands in the container:

```bash
# Generate tiles for a region
docker-compose exec tile-service /app/tile-service generate oregon --skip-upload

# Extract geometries from existing tiles
docker-compose exec tile-service /app/tile-service extract /app/tiles/oregon

# Or use the convenience script
./docker-generate.sh oregon
```

## Auto-Restart Behavior

The service is configured with `restart: unless-stopped`, which means:
- ✅ Restarts automatically if it crashes
- ✅ Starts automatically when Docker daemon starts
- ✅ Does NOT restart if you manually stop it (`docker-compose stop`)

To change this behavior, edit `docker-compose.yml`:
```yaml
restart: always        # Always restart, even if manually stopped
restart: on-failure    # Only restart on crash
restart: "no"          # Never restart
```

## Monitoring

### Check Health
```bash
# Via health check endpoint
curl http://localhost:8080/health

# Via Docker health status
docker-compose ps

# View health check logs
docker inspect drivefinder-tile-service --format='{{.State.Health.Status}}'
```

### Resource Usage
```bash
# View resource usage
docker stats drivefinder-tile-service

# Shows:
# - CPU usage
# - Memory usage
# - Network I/O
# - Block I/O
```

## Integration with Next.js App

Configure your Next.js app's `.env.local` to point to the service:

```env
NEXT_PUBLIC_TILE_URL=http://localhost:8080/tiles
DATABASE_URL="postgres://postgres:localdev@localhost:5432/postgres"
```

The Next.js app will:
1. Fetch tiles from the tile-service at `http://localhost:8080/tiles/`
2. Query road geometries from the same PostgreSQL database

## Running as System Service (macOS/Linux)

For production deployments, you can set Docker to start on boot:

### macOS
```bash
# Docker Desktop starts automatically by default
# Check: Docker Desktop → Preferences → General → "Start Docker Desktop when you log in"
```

### Linux (systemd)
```bash
# Enable Docker to start on boot
sudo systemctl enable docker

# Create a systemd service for docker-compose
sudo nano /etc/systemd/system/drivefinder.service
```

Example systemd service file:
```ini
[Unit]
Description=Drive Finder Tile Service
Requires=docker.service
After=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/path/to/tile-service
ExecStart=/usr/local/bin/docker-compose up -d
ExecStop=/usr/local/bin/docker-compose down
TimeoutStartSec=0

[Install]
WantedBy=multi-user.target
```

Then:
```bash
sudo systemctl enable drivefinder
sudo systemctl start drivefinder
```

## Troubleshooting

### Service won't start
```bash
# Check logs for errors
docker-compose logs tile-service

# Common issues:
# 1. Port 8080 already in use
lsof -i :8080

# 2. Database not ready
docker-compose logs postgres
```

### Service keeps restarting
```bash
# Check logs
docker-compose logs --tail=50 tile-service

# Check health
docker inspect drivefinder-tile-service --format='{{json .State.Health}}'
```

### Can't reach service
```bash
# Test locally
curl http://localhost:8080/health

# Check if port is exposed
docker-compose ps

# Check firewall rules (macOS)
sudo pfctl -sr | grep 8080
```

## Performance Tuning

### Resource Limits
Uncomment resource limits in `docker-compose.yml`:
```yaml
deploy:
  resources:
    limits:
      cpus: '2'
      memory: 2G
    reservations:
      cpus: '1'
      memory: 1G
```

### Worker Configuration
Adjust environment variables in `docker-compose.yml`:
```yaml
environment:
  - WORKERS=4               # Number of parallel workers
  - POLL_INTERVAL_SECONDS=10
```

## Security Considerations

For production:
1. Change default passwords
2. Use environment variables for secrets
3. Enable PostgreSQL SSL
4. Use a reverse proxy (nginx) for HTTPS
5. Implement rate limiting
6. Use proper firewall rules

This Docker setup is designed for local development. For production, use managed services and proper security practices.
