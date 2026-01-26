# Tile Service

A high-performance Go service for generating vector tiles from road curvature data, extracting road geometries, and serving tiles over HTTP.

---

## Table of Contents

1. [Overview](#overview)
2. [Quick Start](#quick-start)
3. [Features](#features)
4. [Installation](#installation)
5. [CLI Usage](#cli-usage)
6. [HTTP Server](#http-server)
7. [Docker](#docker)
8. [Tile Generation Pipeline](#tile-generation-pipeline)
9. [Road Geometry Extraction](#road-geometry-extraction)
10. [Two-Phase Workflow](#two-phase-workflow)
11. [Testing](#testing)
12. [Configuration](#configuration)
13. [Database Schema](#database-schema)
14. [Troubleshooting](#troubleshooting)

---

## Overview

The tile-service processes road curvature KMZ data from [roadcurvature.com](http://roadcurvature.com) into:
- **Vector tiles** (MVT format) for map visualization
- **Road geometry records** for spatial queries ("Find Nearby Roads")

It integrates with the Drive Finder web application, providing the backend infrastructure for scenic road discovery.

### Key Capabilities

- **Native Go KML parsing** - No external Python dependencies
- **Tippecanoe integration** - High-quality vector tile generation
- **Cloudflare R2 uploads** - CDN-hosted tiles via AWS SDK v2
- **PostgreSQL tracking** - Optional job and geometry storage
- **Docker support** - Containerized deployment
- **HTTP server mode** - REST API for tile operations

---

## Quick Start

### Docker (Recommended)

```bash
# Start PostgreSQL + Tile Service
./docker-start.sh

# Check status
./docker-status.sh

# Generate tiles for a region
./docker-generate.sh oregon
```

### Native

```bash
# Build
go build -o tile-service .

# Generate tiles
./tile-service generate oregon

# Start HTTP server
./tile-service serve -port 8080
```

---

## Features

### Tile Generation
- Extract KMZ archives (zip handling)
- Parse KML with nested folder support
- Convert to GeoJSON with road properties
- Generate tiles with Tippecanoe (zoom 5-16)
- Upload to Cloudflare R2

### Road Geometry Extraction
- Parse MVT tiles with paulmach/orb library
- Calculate geographic bounding boxes
- Batch insert to PostgreSQL
- Resumable extraction with checkpointing

### HTTP Server
- Serve tiles at `/tiles/{region}/{z}/{x}/{y}.pbf`
- REST API for job management
- Server-Sent Events for progress streaming
- Health checks for monitoring

### Docker Support
- Multi-stage Dockerfile
- Docker Compose with PostgreSQL
- Volume mounts for data persistence
- Auto-restart and health checks

---

## Installation

### Prerequisites

- Go 1.21+
- Tippecanoe
- PostgreSQL 12+ (optional)
- Docker (optional)

### Build from Source

```bash
# Clone repository
git clone <repository>
cd tile-service

# Build for current platform
go build -o tile-service .

# Build all platforms
make build-all
```

### Install Tippecanoe

```bash
# macOS
brew install tippecanoe

# Ubuntu/Debian
apt-get install tippecanoe

# From source
git clone https://github.com/mapbox/tippecanoe.git
cd tippecanoe && make install
```

### Configure Environment

```bash
cp .env.example .env
# Edit .env with your credentials
```

---

## CLI Usage

### Generate Command

Generate tiles from KMZ curvature data.

```bash
./tile-service generate [options] <region>

Options:
  -config string     Path to .env configuration file (default ".env")
  -max-zoom int      Maximum zoom level (default 16)
  -min-zoom int      Minimum zoom level (default 5)
  -skip-upload       Skip R2 upload, save tiles locally only
  -no-cleanup        Don't cleanup temporary files
  -extract-geometry  Extract road geometry (default true)
  -skip-geometry-insertion  Skip database insertion
  -debug             Enable debug logging
```

**Examples:**

```bash
# Basic generation with upload
./tile-service generate washington

# Local only (no upload)
./tile-service generate -skip-upload oregon

# Custom zoom levels
./tile-service generate -max-zoom 12 -min-zoom 6 california

# Debug mode, keep temp files
./tile-service -debug generate -no-cleanup -skip-upload maryland
```

### Extract Command

Extract road geometries from existing tiles.

```bash
./tile-service extract [options] <tiles_directory>

Examples:
  ./tile-service extract public/tiles/oregon
  ./tile-service -debug extract ~/data/df/tiles/washington
```

### Upload Command

Upload tiles to Cloudflare R2.

```bash
./tile-service upload [options] <tiles_directory>

Options:
  -min-zoom int     Minimum zoom level to upload (-1 = all)
  -max-zoom int     Maximum zoom level to upload (-1 = all)

Examples:
  ./tile-service upload public/tiles/oregon
  ./tile-service upload -min-zoom 5 -max-zoom 10 public/tiles/california
```

### Insert-Geometries Command

Insert road geometries from JSON file to database.

```bash
./tile-service insert-geometries <region_or_file>

Examples:
  ./tile-service insert-geometries oregon
  ./tile-service insert-geometries .extracted-roads-oregon.json
```

### Serve Command

Start HTTP server for tile serving and job management.

```bash
./tile-service serve [options]

Options:
  -port int    HTTP server port (default 8080)
  -host string Server host (default "0.0.0.0")

Examples:
  ./tile-service serve
  ./tile-service serve -port 3001
```

---

## HTTP Server

### Endpoints

#### Tile Serving
```
GET /tiles/{region}/{z}/{x}/{y}.pbf
GET /health
```

#### Job Management
```
POST /api/generate         - Submit tile generation job
GET  /api/jobs             - List all jobs
GET  /api/jobs/{id}        - Get job status
GET  /api/stream/{id}      - Stream job updates (SSE)
POST /api/cancel/{id}      - Cancel running job
GET  /api/regions          - List available regions
```

#### Environment
```
GET  /api/environment      - Get current environment
POST /api/environment      - Switch environment
GET  /api/environment/list - List environments
```

### Example Requests

```bash
# Health check
curl http://localhost:8080/health

# Submit generation job
curl -X POST http://localhost:8080/api/generate \
  -H "Content-Type: application/json" \
  -d '{"region": "oregon", "maxZoom": 14, "skipUpload": true}'

# Get job status
curl http://localhost:8080/api/jobs/abc123

# List regions
curl http://localhost:8080/api/regions
```

---

## Docker

### Quick Start

```bash
# Start all services
./docker-start.sh

# Check status
./docker-status.sh

# Generate tiles
./docker-generate.sh oregon --skip-upload

# View logs
docker-compose logs -f tile-service
```

### Services

| Service | Port | Description |
|---------|------|-------------|
| tile-service | 8080 | Go tile server |
| postgres | 5432 | PostgreSQL 15 |

### Docker Compose Commands

```bash
# Start services
docker-compose up -d

# Stop services
docker-compose stop

# Stop and remove
docker-compose down

# Rebuild after code changes
docker-compose up -d --build

# View logs
docker-compose logs -f tile-service
```

### Volume Mounts

| Host Path | Container Path | Purpose |
|-----------|----------------|---------|
| `./tiles` | `/app/tiles` | Generated tiles |
| `./curvature-data` | `/app/curvature-data` | KMZ source files |
| `postgres-data` | `/var/lib/postgresql/data` | Database |

### Running Commands in Container

```bash
# Generate tiles
docker-compose exec tile-service /app/tile-service generate oregon --skip-upload

# Extract geometries
docker-compose exec tile-service /app/tile-service extract /app/tiles/oregon

# PostgreSQL shell
docker-compose exec postgres psql -U postgres
```

---

## Tile Generation Pipeline

### Pipeline Stages

```
1. Extract KMZ    → Unzip archive, locate doc.kml
2. Convert KML    → Parse XML, generate GeoJSON
3. Generate Tiles → Run Tippecanoe (zoom 5-16)
4. Extract Geometry → Parse MVT, calculate bounds
5. Insert to DB   → Batch upsert RoadGeometry
6. Upload to R2   → AWS SDK v2 parallel uploads
7. Cleanup        → Remove temporary files
```

### Input Files

KMZ files should be placed in `curvature-data/` with naming:
- `us-{region}.c_1000.curves.kmz` (US states)
- `{region}.c_1000.curves.kmz` (other regions)

### Output Files

```
tiles/{region}/           # Vector tiles (z/x/y.pbf)
{region}.geojson          # Intermediate GeoJSON
.extracted-roads-{region}.json  # Extraction file
.extract-progress-{region}.json # Progress checkpoint
```

### GeoJSON Properties

Each road feature includes:
- `Name` - Road name from KML
- `curvature` - Curvature score
- `length` - Road length in meters
- `startLat`, `startLng` - Start coordinates
- `endLat`, `endLng` - End coordinates

---

## Road Geometry Extraction

### How It Works

1. **Walk tile directory** - Find all `.pbf` files
2. **Parse MVT format** - Using paulmach/orb library
3. **Extract features** - From "roads" layer
4. **Convert coordinates** - Tile space to geographic
5. **Calculate bounding boxes** - Min/max lat/lng per road
6. **Batch insert** - 50 roads per transaction

### Progress Tracking

Extraction creates checkpoint files for resumability:
- `.extract-progress-{region}.json` - Current progress
- `.extracted-roads-{region}.json` - Extracted roads

### Performance

- **Speed**: ~100-200 tiles/second
- **Memory**: ~50-100 MB during extraction
- **27,574 tiles** (typical state): 2-5 minutes

### Database Storage

```sql
INSERT INTO "RoadGeometry" (id, "roadId", region, "minLat", "maxLat", "minLng", "maxLng", curvature)
VALUES (...)
ON CONFLICT ("roadId", region)
DO UPDATE SET "minLat" = LEAST(...), "maxLat" = GREATEST(...), ...
```

---

## Two-Phase Workflow

For careful validation before database insertion.

### Phase 1: Generate & Extract (No Insertion)

```bash
./tile-service generate -skip-upload -skip-geometry-insertion florida
```

Creates:
- `tiles/florida/` - Vector tiles
- `.extracted-roads-florida.json` - Road data (for review)

### Review Extraction

```bash
# Count roads
jq 'length' .extracted-roads-florida.json

# View first few roads
jq '.[0:3]' .extracted-roads-florida.json

# Validate bounds
jq '.[] | select(.minLat > .maxLat)' .extracted-roads-florida.json
```

### Phase 2: Insert to Database

```bash
./tile-service insert-geometries florida
```

### Multi-State Script

```bash
#!/bin/bash
STATES="florida georgia alabama"

# Phase 1: Extract all
for state in $STATES; do
    ./tile-service generate -skip-upload -skip-geometry-insertion $state
done

# Review files...

# Phase 2: Insert all
for state in $STATES; do
    ./tile-service insert-geometries $state
done
```

---

## Testing

### Unit Tests

```bash
# Run all tests
go test -v

# Run specific test
go test -v -run TestTilePathParsing

# With coverage
go test -cover
```

### Component Tests

```bash
# Test converter
make test-converter

# Test extractor
make test-extractor

# Test API
make test-api
```

### Integration Tests

```bash
# Full pipeline test
make test-integration

# Docker test
make test-docker
```

### Validation Suite

Compare old vs new pipeline output:

```bash
# Analyze KML ground truth
go run ./cmd/analyze-kml/main.go ~/data/df/curvature-data/oregon.kmz

# Compare GeoJSON outputs
go run ./cmd/compare-geojson/main.go old.geojson new.geojson

# Analyze tile content
go run ./cmd/analyze-tiles/main.go ~/data/df/tiles/oregon

# Full validation
./scripts/validate-pipeline.sh oregon
```

---

## Configuration

### Environment Variables

```env
# Database (optional)
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=localdev
DB_NAME=drivefinder
DB_SSLMODE=disable

# Cloudflare R2
S3_ENDPOINT=https://account-id.r2.cloudflarestorage.com
S3_ACCESS_KEY_ID=your_access_key
S3_SECRET_ACCESS_KEY=your_secret_key
S3_REGION=auto
S3_BUCKET=drivefinder-tiles
S3_BUCKET_PATH=tiles

# Paths
CURVATURE_DATA_DIR=./curvature-data
TILES_OUTPUT_DIR=./tiles
```

### Environment Switching

```bash
# Switch to local
./switch-env.sh local

# Switch to production
./switch-env.sh production
```

### Local Environment
- Database: Local PostgreSQL
- Tiles: Saved locally
- Upload: Disabled

### Production Environment
- Database: Supabase
- Tiles: Uploaded to R2
- Upload: Enabled

---

## Database Schema

### TileJob

Tracks tile generation jobs.

```sql
CREATE TABLE "TileJob" (
    id                      TEXT PRIMARY KEY,
    region                  TEXT NOT NULL,
    status                  TEXT DEFAULT 'pending',
    "maxZoom"               INT DEFAULT 16,
    "minZoom"               INT DEFAULT 5,
    "skipUpload"            BOOLEAN DEFAULT false,
    "noCleanup"             BOOLEAN DEFAULT false,
    "extractGeometry"       BOOLEAN DEFAULT true,
    "skipGeometryInsertion" BOOLEAN DEFAULT false,
    "roadsExtracted"        INT,
    "tilesGenerated"        INT,
    "totalSizeBytes"        BIGINT,
    "currentStep"           TEXT,
    "uploadProgress"        INT DEFAULT 0,
    "uploadedBytes"         BIGINT DEFAULT 0,
    "errorMessage"          TEXT,
    "errorLog"              TEXT,
    "createdAt"             TIMESTAMP DEFAULT NOW(),
    "updatedAt"             TIMESTAMP,
    "startedAt"             TIMESTAMP,
    "completedAt"           TIMESTAMP
);
```

### RoadGeometry

Stores road bounding boxes for spatial queries.

```sql
CREATE TABLE "RoadGeometry" (
    id          TEXT PRIMARY KEY,
    "roadId"    TEXT NOT NULL,
    region      TEXT NOT NULL,
    "minLat"    DOUBLE PRECISION NOT NULL,
    "maxLat"    DOUBLE PRECISION NOT NULL,
    "minLng"    DOUBLE PRECISION NOT NULL,
    "maxLng"    DOUBLE PRECISION NOT NULL,
    curvature   TEXT,
    "createdAt" TIMESTAMP DEFAULT NOW(),
    "updatedAt" TIMESTAMP DEFAULT NOW(),

    UNIQUE ("roadId", region)
);

CREATE INDEX ON "RoadGeometry"(region);
CREATE INDEX ON "RoadGeometry"("minLat", "maxLat", "minLng", "maxLng");
```

### Column Naming

PostgreSQL queries use Prisma's camelCase naming:

```sql
-- Correct
SELECT "roadsExtracted", "createdAt" FROM "TileJob"

-- Wrong
SELECT roads_extracted, created_at FROM TileJob
```

---

## Troubleshooting

### KMZ File Not Found

```bash
# Check file exists
ls curvature-data/

# Expected naming
us-oregon.c_1000.curves.kmz
asia-japan.c_1000.curves.kmz
```

### Database Connection Failed

```bash
# Check credentials
cat .env | grep DB_

# Test connection
psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME

# Verify table exists
psql ... -c '\d "TileJob"'
```

### Tippecanoe Failed

```bash
# Check installation
which tippecanoe
tippecanoe --version

# Check disk space
df -h

# Run with debug
./tile-service -debug generate -no-cleanup oregon
```

### R2 Upload Failed

```bash
# Verify credentials
cat .env | grep S3_

# Test with AWS CLI
aws s3 ls s3://drivefinder-tiles/ \
  --endpoint-url $S3_ENDPOINT
```

### Docker Issues

```bash
# Check container status
docker-compose ps

# View logs
docker-compose logs tile-service

# Restart services
docker-compose restart

# Rebuild from scratch
docker-compose down -v
docker-compose up -d --build
```

### Extraction Issues

```bash
# Check progress file
cat .extract-progress-oregon.json

# Restart extraction (delete progress)
rm .extract-progress-oregon.json
./tile-service extract public/tiles/oregon

# Debug mode
./tile-service -debug extract public/tiles/oregon
```

---

## Performance Notes

| Operation | Time | Notes |
|-----------|------|-------|
| KMZ Extraction | 1-2s | Depends on file size |
| KML Conversion | 0.5-1s | Per region |
| Tile Generation | 30-120s | CPU intensive (Tippecanoe) |
| Geometry Extraction | 2-5 min | ~100-200 tiles/sec |
| R2 Upload | Variable | 1-5 MB/s network dependent |

### Memory Usage

- **Tile Generation**: ~2GB peak (Tippecanoe)
- **Geometry Extraction**: ~50-100 MB
- **HTTP Server**: ~50 MB baseline

---

## Related Documentation

| Document | Description |
|----------|-------------|
| [OVERVIEW.md](./OVERVIEW.md) | Application structure summary |
| [CHANGELOG.md](./CHANGELOG.md) | Version history |
| [DOCKER.md](./DOCKER.md) | Docker setup guide |
| [RUNNING_AS_SERVICE.md](./RUNNING_AS_SERVICE.md) | Background service guide |
| [ROAD_GEOMETRY_EXTRACTION.md](./ROAD_GEOMETRY_EXTRACTION.md) | Extraction details |
| [WORKFLOW_GUIDE.md](./WORKFLOW_GUIDE.md) | Two-phase workflow |
| [TESTING.md](./TESTING.md) | Testing guide |
| [TESTING_WORKFLOW.md](./TESTING_WORKFLOW.md) | Testing infrastructure |
| [VALIDATION_SUITE.md](./VALIDATION_SUITE.md) | Pipeline validation |
| [SCHEMA_SYNC.md](./SCHEMA_SYNC.md) | Prisma/Go schema sync |
| [ENVIRONMENT_SWITCHING_PLAN.md](./ENVIRONMENT_SWITCHING_PLAN.md) | Environment management |

---

## Integration with Drive Finder

The tile-service integrates with the Next.js Drive Finder app:

### Tile Loading

```typescript
// CurvatureLayer.tsx
const tileSource = `${tileUrl}/{z}/{x}/{y}.pbf`;
```

### Road Discovery

```typescript
// road.ts router
const roads = await db.roadGeometry.findMany({
  where: {
    minLat: { lte: lat + radius },
    maxLat: { gte: lat - radius },
    minLng: { lte: lng + radius },
    maxLng: { gte: lng - radius },
  }
});
```

### Local Development

```env
# df/.env.local
DATABASE_URL="postgres://postgres:localdev@localhost:5432/postgres"
NEXT_PUBLIC_TILE_URL=http://localhost:8080/tiles
```

---

## License

Private project - Drivefinder
