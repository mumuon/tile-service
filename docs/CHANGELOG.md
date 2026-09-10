# Tile Service - Changelog

A chronological summary of all changes made to the tile-service.

---

## September 2026

### Overture buildings job, run locally only (Sep 10)
- Added `generate-buildings` CLI command + `overture.go`: downloads an Overture buildings
  extract for a bbox via the `overturemaps` CLI, builds a single PMTiles archive with
  Tippecanoe (completeness flags — no feature dropping, unlike the roads job), uploads to
  R2/S3. Has no database dependency by design, so it can't accidentally run against a
  shared/live Postgres instance.
- Dockerfile now provisions `python3`/`overturemaps` in the runtime image alongside the
  existing Tippecanoe build.
- Owner ruling: generation always runs on the owner's machine via the local Compose rig —
  never in hosted/cloud compute. R2 storage is cheap ($0.015/GB-month, no egress) and is
  the only cloud-hosted piece. Runbook: `docs/LOCAL_RUN_R2.md`.
- Verified end-to-end locally (Docker build + a small San Francisco bbox smoke test,
  `--skip-upload`) — produced a valid, decodable PMTiles with real building footprints.

### Zoom-Stable Curvy Roads Tiling (Sep 9)
- Fixed flaky zoom-level rendering of the curvy-roads overlay (density popping/chopping
  between adjacent zooms, seen in PNW screenshots at ~z6-8)
- `tiles.go`: replaced `--preserve-input-order` with `--order-descending-by=curvature`
  so `--drop-densest-as-needed` drops the lowest-curvature roads first and uniformly
  at every zoom, instead of an arbitrary per-zoom subset
- `kmlconv/kmlconv.go`: store `curvature` as a number instead of a string in the
  generated GeoJSON, so tippecanoe's ordering does numeric (not lexicographic)
  comparison
- See DECISIONS.md (2026-09-09) for the local tippecanoe proof and reasoning

## April 2026

### Railway BuildKit cache-mount fixes (Apr 12)
- **5474b03 / 74ddb73 / b04cc5a** — Gave Docker cache mounts service-specific keys (`id=` prefixed with the Railway service id) so BuildKit caching works on Railway's shared builder, and copied the `kmlconv` subpackage into the build context. Resolves cache-mount key collisions on Railway. Bumped into the main repo as `abf2b22b` (May 15).

---

## January 2026

### Docker & Testing Infrastructure (Jan 14)
- **48c69ad** - Add docker scripts, tests, and documentation
  - Added `docker-start.sh`, `docker-generate.sh`, `docker-status.sh`
  - Created comprehensive testing workflow
  - Added DOCKER.md, TESTING_WORKFLOW.md documentation

### Feature Enhancements (Jan 14)
- **62976f7** - Add new features: analyze-tiles improvements, API enhancements, database updates, docker config
  - Enhanced `analyze-tiles` command with detailed tile statistics
  - Added new API endpoints for regions and cancellation
  - Updated database operations for better error handling
  - Improved Docker configuration

### Bug Fixes (Jan 14)
- **07e1980** - Skip .DS_Store files during cleanup to avoid macOS race conditions
- **89eacf1** - Fix stale tiles: fail if directory cleanup fails instead of continuing
- **aac4f8e** - Remove debug logging that ran for every tile

---

## December 2025

### Validation Suite (Dec 18)
- **517e613** - Add validation suite for comparing old vs new tile pipelines
  - Created `analyze-kml` command for ground truth analysis
  - Created `compare-geojson` command for output comparison
  - Created `analyze-tiles` command for tile content analysis
  - Added `validate-pipeline.sh` master script
  - Added VALIDATION_SUITE.md documentation

### KML Converter Fixes (Dec 10)
- **858d49a** - Fix converter to use Folder name in Name property and omit Description
  - Properly extract road names from KML Folder elements
  - Remove unnecessary Description field
  - Improve GeoJSON output consistency

### Geometry Extraction Fixes (Dec 9)
- **dff1b52** - Fix road geometry bounding box calculation
  - Correct coordinate conversion from tile space to geographic
  - Fix bounding box expansion logic for multi-tile roads
  - Improve accuracy of minLat/maxLat/minLng/maxLng

---

## November 2025

### Performance Optimization (Nov 30)
- **308d4b2** - Implement parallel processing and optimize batch operations
  - Added worker pool for concurrent tile processing
  - Batch database insertions (50 roads per transaction)
  - Reduced extraction time by 5-10x
  - Added progress tracking with checkpointing

### Initial Release (Nov 28)
- **3b9ecf8** - Initial tile service with generate and upload commands
  - Core CLI with `generate` and `upload` subcommands
  - KMZ extraction with native Go zip handling
  - KML to GeoJSON conversion with XML parsing
  - Tippecanoe integration for tile generation
  - AWS SDK v2 for Cloudflare R2 uploads
  - PostgreSQL database integration (optional)
  - Structured logging with slog

---

## Summary of Major Milestones

| Date | Milestone |
|------|-----------|
| Nov 2025 | Initial release with generate/upload commands |
| Nov 2025 | Parallel processing and batch optimization |
| Dec 2025 | Geometry extraction and bounding box fixes |
| Dec 2025 | Validation suite for pipeline comparison |
| Jan 2026 | Docker support and testing infrastructure |

---

## Version History

### v0.1.0 (November 2025)
Initial release with core tile generation functionality.

**Features:**
- CLI-based tile generation
- KMZ/KML processing
- Tippecanoe integration
- R2 upload support
- Optional database tracking

### v0.2.0 (December 2025)
Road geometry extraction and validation tools.

**Features:**
- Geometry extraction from tiles
- Bounding box calculation
- Validation suite
- Pipeline comparison tools

### v0.3.0 (January 2026)
Docker support and testing infrastructure.

**Features:**
- Docker Compose setup
- HTTP server mode
- REST API for job management
- Comprehensive testing workflow
- Environment switching support

---

## Command Evolution

### Generate Command
```bash
# v0.1.0
./tile-service generate <region>

# v0.2.0 - Added geometry flags
./tile-service generate -extract-geometry=false <region>
./tile-service generate -skip-geometry-insertion <region>

# v0.3.0 - Full options
./tile-service generate \
  -max-zoom 16 \
  -min-zoom 5 \
  -skip-upload \
  -no-cleanup \
  -extract-geometry \
  -skip-geometry-insertion \
  <region>
```

### New Commands (v0.2.0+)
```bash
# Extract geometries from existing tiles
./tile-service extract <tiles_directory>

# Insert geometries from JSON file
./tile-service insert-geometries <region>

# Start HTTP server (v0.3.0)
./tile-service serve -port 8080
```

---

## Breaking Changes

### v0.2.0
- KML converter now uses Folder name instead of Placemark name
- GeoJSON output format changed (merged segments per road)
- Database schema updated with new RoadGeometry fields

### v0.3.0
- Docker volume paths changed
- Environment variable names updated
- API endpoint paths updated

---

## Migration Notes

### Upgrading to v0.2.0
1. Re-run tile generation to get new GeoJSON format
2. Re-extract road geometries for corrected bounding boxes
3. Verify RoadGeometry table has new columns

### Upgrading to v0.3.0
1. Update Docker volume mounts if using custom paths
2. Update environment variables to new naming convention
3. Rebuild Docker images

---

## Performance Improvements

| Version | Extraction Time (Oregon) | Notes |
|---------|-------------------------|-------|
| v0.1.0 | 10-15 min | Sequential processing |
| v0.2.0 | 2-3 min | Parallel workers, batching |
| v0.3.0 | 2-3 min | Same, Docker optimized |

---

## Contributing

When adding new features or fixes, update this changelog with:
1. Commit hash (short form)
2. Brief description of change
3. Group by month and feature area
