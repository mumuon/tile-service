# Test Scripts

This directory contains all test automation scripts for the tile-service.

## Scripts Overview

| Script | Purpose | Speed | Use Case |
|--------|---------|-------|----------|
| `test-unit.sh` | Run unit tests with coverage | 1-3s | Active development |
| `watch-tests.sh` | Auto-run tests on file changes | Continuous | Live development |
| `test-converter.sh` | Test KML→GeoJSON converter | 10-30s | Verify converter changes |
| `test-extractor.sh` | Test geometry extractor | 5-10s | Verify extraction logic |
| `test-api.sh` | Test API endpoints | 10-15s | Verify API changes |
| `test-integration.sh` | Full pipeline integration test | 1-2 min | Pre-commit validation |
| `test-docker.sh` | Quick Docker environment test | 30-60s | Pre-deploy validation |

## Quick Usage

All scripts can be run directly or via `make` targets:

```bash
# Direct execution
./scripts/test/test-unit.sh -v

# Via Makefile (recommended)
make test-unit
```

## Detailed Script Documentation

### test-unit.sh

Runs all unit tests with optional coverage reporting.

```bash
# Basic run
./scripts/test/test-unit.sh

# Verbose output
./scripts/test/test-unit.sh -v

# With coverage summary
./scripts/test/test-unit.sh -c

# With HTML coverage report (opens in browser)
./scripts/test/test-unit.sh -h
```

**Tests:**
- Distance calculations
- Road geometry processing
- Property extraction
- File I/O operations

**Output:** Test results, optional coverage report

### watch-tests.sh

Continuously monitors .go files and auto-runs tests on changes.

```bash
# Watch all tests
./scripts/test/watch-tests.sh

# Watch specific test
./scripts/test/watch-tests.sh TestHaversineDistance
```

**Requirements:**
- macOS: `brew install fswatch`
- Linux: `apt-get install inotify-tools`

**Use case:** Active development with instant feedback

### test-converter.sh

Tests the KML to GeoJSON converter with new properties.

```bash
# Default test region
./scripts/test/test-converter.sh

# Specific region
./scripts/test/test-converter.sh delaware
```

**Tests:**
1. Unit tests for converter functions
2. Real KML extraction from KMZ
3. GeoJSON generation with new properties
4. Property validation (length, start/end points, curvature)

**Output:**
- Property counts and samples
- First feature with all properties

**Files created:**
- `test-output/{region}.kml`
- `test-output/{region}.geojson`

### test-extractor.sh

Tests geometry extraction from vector tiles.

```bash
# Use test tiles
./scripts/test/test-extractor.sh

# Use specific tile directory
./scripts/test/test-extractor.sh public/tiles/oregon
```

**Tests:**
1. Geometry extractor unit tests
2. Bounding box calculations
3. Real tile parsing (if tiles exist)
4. New property extraction from tiles

**Prerequisites:**
- Tiles must exist (run integration test first if needed)

### test-api.sh

Tests REST API server endpoints.

```bash
# Default port (3000)
./scripts/test/test-api.sh

# Custom port
./scripts/test/test-api.sh 3001
```

**Tests:**
1. Health check endpoint
2. Regions listing (`/api/regions`)
3. Job creation (`POST /api/generate`)
4. Job status (`GET /api/jobs/{id}`)
5. Job cancellation (`POST /api/cancel/{id}`)
6. Job listing (`GET /api/jobs`)

**Cleanup:** Automatically stops server on exit

**Output:**
- API responses (JSON)
- Server log at `/tmp/tile-service-test.log`

### test-integration.sh

Full pipeline integration test from KMZ to tiles.

```bash
# Basic run
./scripts/test/test-integration.sh

# Specific region
./scripts/test/test-integration.sh oregon

# With options
./scripts/test/test-integration.sh oregon --max-zoom 14 --cleanup
```

**Options:**
- `--max-zoom N`: Set maximum zoom level (default: 10)
- `--cleanup`: Remove test files after completion
- `--upload`: Actually upload to R2 (default: skip)

**Tests:**
1. Service build
2. Input file verification
3. Complete pipeline execution
4. Output verification (GeoJSON, tiles, extraction)
5. New properties validation
6. Performance metrics

**Output:**
- Detailed step-by-step progress
- Verification of all outputs
- Performance summary

**Files created:**
- `public/tiles/{region}/` - Generated tiles
- `{region}.geojson` - Converted GeoJSON
- `{region}-extraction.json` - Extracted road geometry

### test-docker.sh

Quick Docker testing without full image rebuild.

```bash
# Default test region
./scripts/test/test-docker.sh

# Specific region
./scripts/test/test-docker.sh oregon
```

**How it works:**
1. Builds Linux binary only
2. Copies to existing container
3. Runs test in container
4. Verifies output

**Prerequisites:**
- Docker container must exist and be running
- Run `./docker-start.sh` first if needed

**Use case:**
- Test in Linux environment
- Pre-deployment validation
- Docker-specific debugging

## Testing Workflow

### During Active Development

```bash
# Terminal 1
./scripts/test/watch-tests.sh

# Terminal 2
vim converter.go
# Save → tests run automatically
```

### Before Committing

```bash
make test-all
# Runs: unit + converter + extractor + integration
```

### Before Deploying

```bash
make test-docker
# Validates in Docker environment
```

## Common Options

All test scripts follow these conventions:

- **Exit codes**: 0 = success, 1 = failure
- **Verbose output**: Detailed step-by-step progress
- **Color coding** (where supported): ✓ = success, ✗ = failure, ⚠ = warning
- **File preservation**: Test files kept for inspection (unless `--cleanup`)

## Troubleshooting

### Script not executable

```bash
chmod +x scripts/test/*.sh
```

### Missing dependencies

```bash
# jq (optional, for pretty JSON)
brew install jq

# fswatch (required for watch mode)
brew install fswatch
```

### Tests fail but code is correct

```bash
# Check for stale test data
rm -rf test-output public/tiles

# Re-run tests
make test-all
```

## Integration with CI/CD

For automated testing pipelines:

```bash
# Run all tests (no interactive components)
make test-all

# Generate coverage for reporting
make test-coverage
# Outputs: coverage.out
```

Example GitHub Actions:

```yaml
- name: Run tests
  run: make test-all

- name: Coverage
  run: make test-coverage
```

## Adding New Tests

When adding a new test script:

1. Create in `scripts/test/test-{name}.sh`
2. Make executable: `chmod +x scripts/test/test-{name}.sh`
3. Add Makefile target in `Makefile`
4. Update this README
5. Update `TESTING_WORKFLOW.md`

## Support

For issues or questions:
- See `TESTING_WORKFLOW.md` for detailed workflow guide
- See `QUICK_TEST_START.md` for quick start
- Check logs in `/tmp/` for detailed output
