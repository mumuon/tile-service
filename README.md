# Tile Service

A high-performance, production-ready Go service for generating vector tiles from road geometry data and uploading them to Cloudflare R2 storage.

## Features

- **Direct CLI mode**: Call tile service as a command-line tool with region name and optional zoom level parameters
- **Configurable zoom levels**: Set custom min/max zoom levels per region (default 5-16)
- **Skip upload option**: Generate and save tiles locally without uploading to R2
- **Debug mode**: Comprehensive logging with structured slog output
- **Graceful shutdown**: SIGINT/SIGTERM handling with in-flight job completion
- **Native Go KML parsing**: Built-in XML parsing for KML/KMZ extraction (no external Python dependencies)
- **Tippecanoe integration**: Subprocess calls to Tippecanoe for high-quality vector tile generation
- **AWS SDK v2**: Efficient Cloudflare R2 uploads via AWS SDK (replaces rclone)
- **Optional database tracking**: Logs tile generation progress to PostgreSQL (falls back gracefully if unavailable)
- **Cross-platform builds**: Easy compilation for Linux, macOS, and Windows

## Architecture

### Tile Generation Pipeline

```
./tile-service washington [-max-zoom 14] [-skip-upload] [-no-cleanup]
              ↓
       Extract KMZ
              ↓
      Convert KML to GeoJSON
              ↓
    Generate tiles with Tippecanoe
              ↓
    (Optional: Upload to R2)
              ↓
    (Optional: Cleanup temp files)
```

### Key Components

1. **CLI Entry**: Direct region-based command-line invocation with customizable parameters
2. **KMZ Extractor**: Go native zip handling for KMZ file extraction
3. **KML to GeoJSON Converter**: Native Go XML parsing (no external Python dependencies)
4. **Tippecanoe Wrapper**: Subprocess calls with configurable zoom levels
5. **R2 Uploader**: AWS SDK v2 with Cloudflare R2 custom endpoint resolution
6. **Optional Database Layer**: PostgreSQL progress tracking (gracefully optional)

## Requirements

### System

- Go 1.21+ (for building from source)
- PostgreSQL 12+ (for job database)
- Tippecanoe (for tile generation)
- 4GB+ RAM (for concurrent tile generation)
- Sufficient disk space (tiles can be 100MB+ per region)

### Dependencies

- `github.com/lib/pq` - PostgreSQL driver
- `github.com/aws/aws-sdk-go-v2` - AWS SDK v2 for R2 uploads

## Installation

### 1. Clone and Build

```bash
cd tile-service
make build-all
```

For specific platforms:
```bash
make build-linux    # Linux AMD64
make build-macos    # macOS ARM64
make build-windows  # Windows AMD64
```

### 2. Install Tippecanoe

**macOS:**
```bash
brew install tippecanoe
```

**Linux (Ubuntu/Debian):**
```bash
apt-get install tippecanoe
```

**From source:**
```bash
git clone https://github.com/mapbox/tippecanoe.git
cd tippecanoe
make install
```

### 3. Configure Environment

Copy the example configuration:

```bash
cp .env.example .env
```

Edit `.env` with your R2 credentials (database is optional for local-only tile generation):

```env
# Cloudflare R2 (required for uploads)
S3_ENDPOINT=https://your-account-id.r2.cloudflarestorage.com
S3_ACCESS_KEY_ID=your_access_key
S3_SECRET_ACCESS_KEY=your_secret_key
S3_REGION=auto
S3_BUCKET=drivefinder-tiles
S3_BUCKET_PATH=tiles

# Database (optional - for progress tracking)
DB_HOST=your-db-host
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password
DB_NAME=drivefinder
DB_SSLMODE=disable
```

**Note**: If database credentials are missing, the service will warn but continue with local-only operation (no progress tracking).

### 4. Prepare Data

Create a `curvature-data` directory containing KMZ files:

```bash
mkdir -p curvature-data
# Copy your .kmz files here, named like: us-maryland.c_1000.curves.kmz
```

## Usage

### Basic Invocation

The service is a command-line tool that takes a region name as an argument:

```bash
./tile-service washington
```

### Command-line Options

```
tile-service [options] <region>

Arguments:
  <region>          Region name (e.g., washington, maryland, japan)

Options:
  -config string    Path to .env configuration file (default ".env")
  -max-zoom int     Maximum zoom level for tiles (default 16)
  -min-zoom int     Minimum zoom level for tiles (default 5)
  -skip-upload      Skip R2 upload, save tiles locally only
  -no-cleanup       Don't cleanup temporary files after completion
  -debug            Enable debug logging
  -help             Show help message
```

### Example Commands

```bash
# Generate tiles for Washington with default settings
./tile-service washington

# Generate tiles with custom max zoom level (less detail, smaller file)
./tile-service -max-zoom 14 washington

# Generate tiles without uploading to R2 (test mode)
./tile-service -skip-upload maryland

# Generate tiles with debug logging and keep temp files for inspection
./tile-service -debug -no-cleanup -skip-upload california

# Generate tiles with custom config file
./tile-service -config /etc/tile-service/.env washington

# See all options
./tile-service -help
```

### Process Flow

Each invocation follows this pipeline:

1. **Extract KMZ**: Unzip and locate the KML file for the region
2. **Convert KML**: Parse KML to GeoJSON format
3. **Generate Tiles**: Run Tippecanoe with specified zoom levels
4. **Upload to R2**: Transfer tiles to Cloudflare R2 storage
5. **Cleanup**: Remove temporary working files

Exit with status code 0 on success, 1 on failure.

## Docker

### Build Docker Image

```bash
make docker-build
```

### Run in Docker

```bash
# Create .env file first
make docker-run

# Or manually
docker run --env-file .env \
  -v ./curvature-data:/app/curvature-data \
  -v ./public/tiles:/app/public/tiles \
  drivefinder/tile-service:latest
```

### Docker Compose

Create a `docker-compose.yml`:

```yaml
version: '3.8'
services:
  tile-service:
    image: drivefinder/tile-service:latest
    env_file: .env
    volumes:
      - ./curvature-data:/app/curvature-data
      - ./public/tiles:/app/public/tiles
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "ps", "aux"]
      interval: 30s
      timeout: 10s
      retries: 3
```

Run with:
```bash
docker-compose up -d
```

## Database Schema

The service expects a `tile_jobs` table in PostgreSQL:

```sql
CREATE TABLE tile_jobs (
  id SERIAL PRIMARY KEY,
  region VARCHAR(50) NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  roads_extracted INT DEFAULT 0,
  tiles_generated INT DEFAULT 0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  started_at TIMESTAMP,
  completed_at TIMESTAMP,
  total_size_bytes BIGINT,
  error_message TEXT
);
```

## Monitoring

The service logs all operations with structured logging:

```
time=2024-11-28T12:34:56Z level=INFO msg="starting R2 upload" region=maryland tiles_dir=/path/to/tiles
time=2024-11-28T12:35:02Z level=INFO msg="directory upload completed" total_bytes=251342891
time=2024-11-28T12:35:03Z level=INFO msg="job completed" job_id=42 status=completed
```

### Log Levels

- `INFO` - Normal operation events
- `WARN` - Recoverable warnings
- `ERROR` - Job failures and errors

Enable debug logging with `-debug` flag for detailed operation tracing.

## Performance Characteristics

- **Extraction**: ~1-2 seconds per KMZ file (depends on file size)
- **KML to GeoJSON**: ~0.5-1 second per region
- **Tile Generation**: ~30-120 seconds per region (Tippecanoe subprocess)
- **R2 Upload**: 1-5 MB/second (depends on network)
- **Memory**: ~200-500 MB per concurrent worker

## Deployment

### VPS Deployment

1. Build binary for Linux:
   ```bash
   make build-linux
   ```

2. Copy to VPS:
   ```bash
   scp tile-service-linux-amd64 user@vps:/opt/tile-service/
   scp .env user@vps:/opt/tile-service/.env
   ```

3. Create systemd service in `/etc/systemd/system/tile-service.service`:
   ```ini
   [Unit]
   Description=Tile Generation Service
   After=network-online.target
   Wants=network-online.target

   [Service]
   Type=simple
   User=tile-service
   WorkingDirectory=/opt/tile-service
   ExecStart=/opt/tile-service/tile-service -config .env
   Restart=on-failure
   RestartSec=10

   [Install]
   WantedBy=multi-user.target
   ```

4. Enable and start:
   ```bash
   systemctl enable tile-service
   systemctl start tile-service
   ```

### Kubernetes Deployment

Use the provided `Dockerfile` to create container images:

```bash
docker build -t your-registry/tile-service:0.1.0 .
docker push your-registry/tile-service:0.1.0
```

Create a Kubernetes deployment manifest with appropriate resource limits and environment variables.

## Development

### Building

```bash
# Build for current platform
make build

# Build all platforms
make build-all

# Clean
make clean

# Test compilation
make test-build
```

### Modifying

Key files:

- `main.go` - Worker pool orchestration and polling loop
- `database.go` - PostgreSQL operations
- `extractor.go` - KMZ extraction (archive/zip)
- `converter.go` - KML to GeoJSON conversion (encoding/xml)
- `tiles.go` - Tippecanoe subprocess calls
- `s3.go` - AWS SDK v2 for R2 uploads
- `config.go` - Configuration loading
- `models.go` - Data structures

## Troubleshooting

### "KMZ file not found"
- Ensure `CURVATURE_DATA_DIR` environment variable is set correctly
- KMZ files should be named `us-{region}.c_1000.curves.kmz`

### "failed to connect to database"
- Verify PostgreSQL is running and accessible
- Check `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`
- Ensure `tile_jobs` table exists

### "Tippecanoe failed"
- Verify Tippecanoe is installed: `which tippecanoe`
- Check disk space: `df -h`
- Review error message in logs

### "R2 upload failed"
- Verify S3 credentials in `.env`
- Check R2 bucket exists and is accessible
- Verify network connectivity to R2

## License

Private project - Drivefinder
