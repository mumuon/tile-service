# Tile Service - Overview

A high-performance Go service for generating vector tiles from road curvature data and serving them over HTTP.

---

## Application Structure

```
tile-service/
├── main.go                    # CLI entry point and subcommand routing
├── service.go                 # Tile generation pipeline orchestration
├── api.go                     # REST API server and handlers
├── database.go                # PostgreSQL operations
├── extractor.go               # KMZ extraction (archive/zip)
├── converter.go               # KML to GeoJSON conversion (encoding/xml)
├── tiles.go                   # Tippecanoe subprocess integration
├── s3.go                      # AWS SDK v2 for R2 uploads
├── geometry_extractor.go      # MVT tile parsing for road extraction
├── config.go                  # Configuration loading
├── models.go                  # Data structures
├── environment.go             # Runtime environment management
│
├── cmd/                       # Standalone tools
│   ├── analyze-kml/           # KML ground truth analyzer
│   ├── analyze-tiles/         # Tile content analyzer
│   └── compare-geojson/       # GeoJSON comparison tool
│
├── scripts/                   # Utility scripts
│   ├── test/                  # Testing scripts
│   └── validate-pipeline.sh   # Master validation script
│
├── docs/                      # Documentation
│
├── docker-compose.yml         # Docker service definitions
├── Dockerfile                 # Multi-stage build
├── docker-start.sh            # Quick start script
├── docker-generate.sh         # Generate tiles script
├── docker-status.sh           # Check service status
│
├── Makefile                   # Build and test commands
├── go.mod / go.sum            # Go dependencies
│
├── tiles/                     # Generated tiles (Docker volume)
└── curvature-data/            # KMZ source files (Docker volume)
```

---

## Technology Stack

| Technology | Purpose |
|------------|---------|
| Go 1.21+ | Core language |
| PostgreSQL 15 | Database for job tracking and road geometry |
| Tippecanoe | Vector tile generation |
| AWS SDK v2 | Cloudflare R2 uploads |
| paulmach/orb | MVT tile parsing |
| Docker | Containerization |

---

## Core Features

### 1. Tile Generation Pipeline
Complete workflow from KMZ to vector tiles.

- **KMZ Extraction**: Native Go zip handling
- **KML to GeoJSON**: XML parsing with nested folder support
- **Tippecanoe Integration**: Subprocess calls with configurable zoom (5-16)
- **R2 Upload**: AWS SDK v2 with custom endpoint resolution
- **Road Geometry Extraction**: MVT parsing for database storage

### 2. HTTP Server Mode
Persistent background service for tile serving and job management.

- Serves tiles at `http://localhost:8080/tiles/{region}/{z}/{x}/{y}.pbf`
- REST API for tile generation jobs
- Health checks and monitoring
- Auto-restart with Docker

### 3. CLI Mode
Direct command-line invocation for tile operations.

```bash
./tile-service generate <region>      # Generate tiles
./tile-service extract <tiles_dir>    # Extract road geometry
./tile-service upload <tiles_dir>     # Upload to R2
./tile-service insert-geometries <region>  # Insert to database
./tile-service serve                  # Start HTTP server
```

### 4. Two-Phase Workflow
Review and validate before database insertion.

- **Phase 1**: Generate tiles and extract to JSON file
- **Review**: Inspect `.extracted-roads-{region}.json`
- **Phase 2**: Insert geometries to database

### 5. Docker Support
Containerized deployment with PostgreSQL.

- **tile-service**: Go service on port 8080
- **postgres**: PostgreSQL 15 on port 5432
- Volume mounts for tiles and curvature data
- Health checks and auto-restart

### 6. Validation Suite
Tools for comparing old vs new pipeline output.

- `analyze-kml`: Ground truth from KMZ
- `compare-geojson`: Feature and coordinate comparison
- `analyze-tiles`: Tile content analysis

---

## Pipeline Stages

```
KMZ File (curvature-data/)
    ↓
┌───────────────────────────────────────┐
│ Phase 1: Extract KMZ                  │
│   - Unzip archive                     │
│   - Locate doc.kml                    │
└───────────────────────────────────────┘
    ↓
┌───────────────────────────────────────┐
│ Phase 2: Convert KML → GeoJSON        │
│   - Parse XML with nested folders     │
│   - Extract road properties           │
│   - Calculate length, curvature       │
│   - Output: {region}.geojson          │
└───────────────────────────────────────┘
    ↓
┌───────────────────────────────────────┐
│ Phase 3: Generate Tiles               │
│   - Run Tippecanoe                    │
│   - Zoom levels 5-16                  │
│   - Output: tiles/{z}/{x}/{y}.pbf     │
└───────────────────────────────────────┘
    ↓
┌───────────────────────────────────────┐
│ Phase 4: Extract Road Geometry        │
│   - Parse MVT tiles                   │
│   - Calculate bounding boxes          │
│   - Output: .extracted-roads-{}.json  │
└───────────────────────────────────────┘
    ↓
┌───────────────────────────────────────┐
│ Phase 5: Insert to Database           │
│   - Batch upsert RoadGeometry         │
│   - 50 roads per transaction          │
└───────────────────────────────────────┘
    ↓
┌───────────────────────────────────────┐
│ Phase 6: Upload to R2 (Optional)      │
│   - AWS SDK v2                        │
│   - Parallel uploads                  │
└───────────────────────────────────────┘
    ↓
┌───────────────────────────────────────┐
│ Phase 7: Cleanup                      │
│   - Remove temp files                 │
│   - Remove extraction progress        │
└───────────────────────────────────────┘
```

---

## API Endpoints

### Tile Serving
| Endpoint | Description |
|----------|-------------|
| `GET /tiles/{region}/{z}/{x}/{y}.pbf` | Serve vector tile |
| `GET /health` | Health check |

### Job Management
| Endpoint | Description |
|----------|-------------|
| `POST /api/generate` | Submit tile generation job |
| `GET /api/jobs` | List all jobs |
| `GET /api/jobs/{id}` | Get job status |
| `GET /api/stream/{id}` | Stream job updates (SSE) |
| `POST /api/cancel/{id}` | Cancel running job |
| `GET /api/regions` | List available regions |

### Environment
| Endpoint | Description |
|----------|-------------|
| `GET /api/environment` | Get current environment |
| `POST /api/environment` | Switch environment |
| `GET /api/environment/list` | List environments |

---

## Database Schema

### TileJob
```sql
CREATE TABLE "TileJob" (
    id                    TEXT PRIMARY KEY,
    region                TEXT NOT NULL,
    status                TEXT DEFAULT 'pending',
    "maxZoom"             INT DEFAULT 16,
    "minZoom"             INT DEFAULT 5,
    "skipUpload"          BOOLEAN DEFAULT false,
    "noCleanup"           BOOLEAN DEFAULT false,
    "extractGeometry"     BOOLEAN DEFAULT true,
    "skipGeometryInsertion" BOOLEAN DEFAULT false,
    "roadsExtracted"      INT,
    "tilesGenerated"      INT,
    "totalSizeBytes"      BIGINT,
    "currentStep"         TEXT,
    "uploadProgress"      INT DEFAULT 0,
    "uploadedBytes"       BIGINT DEFAULT 0,
    "errorMessage"        TEXT,
    "errorLog"            TEXT,
    "createdAt"           TIMESTAMP DEFAULT NOW(),
    "updatedAt"           TIMESTAMP,
    "startedAt"           TIMESTAMP,
    "completedAt"         TIMESTAMP
);
```

### RoadGeometry
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

    CONSTRAINT "RoadGeometry_roadId_region_key" UNIQUE ("roadId", region)
);
```

---

## Configuration

### Environment Variables
```env
# Database (optional for local-only)
DB_HOST=your-db-host
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password
DB_NAME=drivefinder
DB_SSLMODE=disable

# Cloudflare R2 (required for uploads)
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

---

## Performance Characteristics

| Operation | Time | Notes |
|-----------|------|-------|
| KMZ Extraction | 1-2s | Depends on file size |
| KML to GeoJSON | 0.5-1s | Per region |
| Tile Generation | 30-120s | Tippecanoe, CPU intensive |
| Geometry Extraction | 2-5min | ~100-200 tiles/second |
| R2 Upload | 1-5 MB/s | Network dependent |
| Memory Usage | 200-500MB | Per concurrent worker |

---

## Key Directories

| Directory | Contents |
|-----------|----------|
| `curvature-data/` | KMZ source files |
| `tiles/` | Generated vector tiles |
| `cmd/` | Standalone analysis tools |
| `scripts/` | Utility and test scripts |
| `docs/` | Documentation |

---

## External Dependencies

### Tippecanoe
Vector tile generation tool from Mapbox/Felt.

```bash
# macOS
brew install tippecanoe

# Ubuntu/Debian
apt-get install tippecanoe
```

### Cloudflare R2
Object storage for tile hosting.

- Bucket: `drivefinder-tiles`
- Path: `tiles/`
- Public access enabled

---

## Related Documentation

- [DOCKER.md](./DOCKER.md) - Docker setup and usage
- [RUNNING_AS_SERVICE.md](./RUNNING_AS_SERVICE.md) - Background service guide
- [ROAD_GEOMETRY_EXTRACTION.md](./ROAD_GEOMETRY_EXTRACTION.md) - Extraction details
- [WORKFLOW_GUIDE.md](./WORKFLOW_GUIDE.md) - Two-phase workflow
- [TESTING.md](./TESTING.md) - Testing guide
- [VALIDATION_SUITE.md](./VALIDATION_SUITE.md) - Pipeline validation
- [CHANGELOG.md](./CHANGELOG.md) - Version history
