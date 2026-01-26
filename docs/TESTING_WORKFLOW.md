# Testing Workflow Guide

This guide describes the complete testing infrastructure for the tile-service, designed for rapid iteration without Docker rebuilds.

## Quick Reference

```bash
# Quick unit tests (fastest - use during development)
make test                    # Run all unit tests
make test-watch             # Watch mode - auto-run on file changes

# Component testing
make test-converter         # Test KML→GeoJSON conversion
make test-extractor         # Test tile→geometry extraction
make test-api              # Test API endpoints

# Integration testing
make test-integration       # Full pipeline test
make test-docker           # Quick Docker test (no full rebuild)

# Coverage reports
make test-coverage         # Terminal coverage summary
make test-coverage-html    # Open HTML coverage report

# Run everything
make test-all              # All tests except Docker
```

## Testing Pyramid

```
        /\
       /  \    Integration Tests
      /    \   (slowest, most comprehensive)
     /______\
    /        \  Component Tests
   /          \ (medium speed, focused)
  /____________\
 /              \ Unit Tests
/________________\ (fastest, test individual functions)
```

## 1. Unit Testing (Fastest - Seconds)

### Running Unit Tests

```bash
# Simple run
go test ./...

# Verbose output
make test-unit

# With coverage
make test-coverage

# Coverage with HTML report
make test-coverage-html
```

### Watch Mode for Active Development

Watch mode automatically reruns tests when you save a file:

```bash
# Watch all tests
make test-watch

# Watch specific test
./scripts/test/watch-tests.sh TestHaversineDistance
```

**Use this when:**
- Writing new functions
- Fixing bugs in existing code
- Refactoring
- Want instant feedback (< 1 second)

### What Unit Tests Cover

- ✅ Haversine distance calculations (`converter_test.go`)
- ✅ Road length calculations
- ✅ Start/end point extraction
- ✅ Curvature parsing
- ✅ Tile path parsing (`geometry_extractor_test.go`)
- ✅ Bounding box calculations
- ✅ File save/load operations
- ✅ Progress tracking

## 2. Component Testing (Medium - 10-30 seconds)

Component tests verify individual parts of the system with real data.

### Converter Component

Tests KML → GeoJSON conversion with new properties (length, start/end points):

```bash
make test-converter

# Or with specific region
./scripts/test/test-converter.sh delaware
```

**What it tests:**
1. Unit tests for converter functions
2. Extract real KML from KMZ
3. Convert to GeoJSON
4. Verify new properties exist (length, startLat/Lng, endLat/Lng, curvature)
5. Show sample feature

**Expected output:**
```
✓ Found 450 roads with 'length' property
✓ Found 450 roads with 'startLat' property
✓ Found 120 roads with 'curvature' property
```

### Geometry Extractor Component

Tests tile → road geometry extraction:

```bash
make test-extractor

# Or with specific tile directory
./scripts/test/test-extractor.sh public/tiles/oregon
```

**What it tests:**
1. Unit tests for extraction functions
2. Bounding box calculations
3. Single tile extraction with real tiles
4. New properties in extracted geometry

**Prerequisites:**
- Requires tiles to exist (run `make test-integration` first if needed)

### API Server Component

Tests REST API endpoints:

```bash
make test-api

# Or with custom port
./scripts/test/test-api.sh 3001
```

**What it tests:**
1. Health endpoint
2. `/api/regions` - List available regions (NEW)
3. `/api/generate` - Job creation
4. `/api/jobs/{id}` - Job status
5. `/api/cancel/{id}` - Job cancellation (NEW)
6. `/api/jobs` - List active jobs

**Prerequisites:**
- None - starts its own server instance
- Automatically cleans up

## 3. Integration Testing (Slower - 1-2 minutes)

Full pipeline test with real KMZ file:

```bash
# Basic integration test
make test-integration

# With automatic cleanup
make test-integration-cleanup

# Custom region and options
./scripts/test/test-integration.sh oregon --max-zoom 14 --cleanup
```

**What it tests:**
1. Build the service
2. Verify input KMZ file exists
3. Run complete pipeline:
   - Extract KMZ → KML
   - Convert KML → GeoJSON (with new properties)
   - Generate tiles with Tippecanoe
   - Extract road geometry
4. Verify all outputs:
   - GeoJSON has new properties (length, start/end, curvature)
   - Tiles are generated correctly
   - Extracted roads have new properties
5. Performance metrics

**Sample output:**
```
========================================
  Integration Test - Full Pipeline
========================================
Region: test-region
Max zoom: 10

Step 1: Building tile-service...
✓ Build complete (2s)

Step 2: Verifying input files...
✓ Found KMZ file: curvature-data/test-region.kmz

Step 3: Running tile generation pipeline...
✓ Pipeline complete (45s)

Step 4: Verifying outputs...
✓ GeoJSON file created: test-region.geojson
  Features: 345 roads
  Checking new properties in first road...
    ✓ length: 15234.5m
    ✓ start: (45.123456, -122.654321)
    ✓ curvature: 1500

✓ Tiles directory created: public/tiles/test-region
  Tiles: 127 files
  Total size: 2.3M

Step 5: Performance Summary
Build time: 2s
Pipeline time: 45s
Total time: 47s

========================================
  Integration Test Complete ✓
========================================
```

## 4. Docker Testing (Quick - 30 seconds)

Fast Docker testing without full rebuild:

```bash
make test-docker

# Or with specific region
./scripts/test/test-docker.sh oregon
```

**How it works:**
1. Builds Linux binary only (not full Docker image)
2. Uses existing Docker container
3. Copies new binary into container
4. Runs test inside container
5. Verifies output

**Use this when:**
- Testing Docker-specific issues
- Validating before deployment
- Need to test in Linux environment

**NOT for:**
- Active development (use unit/component tests instead)
- Frequent iteration (too slow)

## Recommended Development Workflow

### Phase 1: Active Development
```bash
# Terminal 1: Watch mode
make test-watch

# Terminal 2: Edit code
vim converter.go

# Tests auto-run when you save
# Fix until tests pass
```

**Iteration time: < 1 second**

### Phase 2: Component Verification
```bash
# After unit tests pass, test component with real data
make test-converter

# Verify output looks correct
cat test-output/test-region.geojson | jq '.features[0].properties'
```

**Iteration time: 10-30 seconds**

### Phase 3: Integration Verification
```bash
# Test full pipeline with small region
./scripts/test/test-integration.sh test-region --max-zoom 10

# If successful, test with real region
./scripts/test/test-integration.sh delaware --cleanup
```

**Iteration time: 1-2 minutes**

### Phase 4: Production Validation
```bash
# Build for production
make build-linux

# Test in Docker
make test-docker

# If all tests pass, deploy
```

**Iteration time: 30-60 seconds**

## Testing New Features

Example: You just added `roadSurface` property

### Step 1: Write Unit Test
```go
// converter_test.go
func TestExtractRoadSurface(t *testing.T) {
    geometry := map[string]interface{}{
        "type": "LineString",
        "properties": map[string]interface{}{
            "surface": "asphalt",
        },
    }

    surface := extractRoadSurface(geometry)
    if surface != "asphalt" {
        t.Errorf("Expected asphalt, got %s", surface)
    }
}
```

### Step 2: Run in Watch Mode
```bash
make test-watch
# Save converter_test.go
# Test runs automatically
# Fix until test passes
```

### Step 3: Implement Feature
```go
// converter.go
func extractRoadSurface(geometry map[string]interface{}) string {
    props := geometry["properties"].(map[string]interface{})
    if surface, ok := props["surface"].(string); ok {
        return surface
    }
    return ""
}
```

### Step 4: Test Component
```bash
make test-converter
# Verify roadSurface appears in GeoJSON output
```

### Step 5: Test Integration
```bash
make test-integration
# Verify end-to-end workflow
```

## Debugging Failed Tests

### Unit Test Failures

```bash
# Run specific test with verbose output
go test -v -run TestHaversineDistance

# Add debug logging
go test -v -run TestHaversineDistance 2>&1 | grep "LOGPREFIX"
```

### Component Test Failures

```bash
# Keep intermediate files for inspection
./scripts/test/test-converter.sh
# Files preserved in test-output/

# Inspect GeoJSON manually
cat test-output/test-region.geojson | jq '.features[0]'
```

### Integration Test Failures

```bash
# Run without cleanup to inspect files
./scripts/test/test-integration.sh test-region

# Check logs
cat /tmp/integration-test-test-region.log

# Inspect intermediate files
ls -lh public/tiles/test-region/
cat test-region.geojson | jq '.features | length'
```

## Continuous Integration

For CI/CD pipelines:

```bash
# Run all tests (no interactive/watch mode)
make test-all

# Generate coverage for reporting
make test-coverage

# Upload coverage.out to coverage service
```

Example GitHub Actions:

```yaml
- name: Run Tests
  run: make test-all

- name: Generate Coverage
  run: make test-coverage

- name: Upload Coverage
  uses: codecov/codecov-action@v3
  with:
    files: ./coverage.out
```

## Test Data

### Creating Test Data

For quick iteration, create minimal test data:

```bash
# Create small test KMZ (10-20 roads)
# Place in curvature-data/test-region.kmz

# Run quick test
./scripts/test/test-integration.sh test-region --max-zoom 10 --cleanup
```

### Using Production Data

```bash
# Test with real region (slower but comprehensive)
./scripts/test/test-integration.sh delaware
```

## Performance Benchmarks

Typical test execution times on modern hardware:

| Test Type | Time | Use Case |
|-----------|------|----------|
| Single unit test | < 100ms | Active development |
| All unit tests | 1-3s | Pre-commit |
| Component test | 10-30s | Feature verification |
| Integration test | 1-2 min | Pre-merge |
| Docker test | 30-60s | Pre-deploy |
| Full test suite | 2-3 min | CI/CD |

## Troubleshooting

### "fswatch not found" (watch mode)

```bash
# macOS
brew install fswatch

# Linux
apt-get install inotify-tools
```

### "jq not found" (test scripts)

```bash
# macOS
brew install jq

# Linux
apt-get install jq
```

### "Container not found" (Docker tests)

```bash
# Create and start container first
./docker-start.sh
```

### Tests pass locally but fail in Docker

```bash
# Test in Docker environment
make test-docker

# Exec into container to debug
docker exec -it tile-service-container sh
```

## Best Practices

1. **Start with unit tests** - Fastest feedback loop
2. **Use watch mode** - Auto-run tests on save
3. **Test incrementally** - Unit → Component → Integration
4. **Keep test data small** - Use test-region for quick tests
5. **Preserve test files** - Don't use `--cleanup` when debugging
6. **Read test output** - Scripts provide detailed verification
7. **Check coverage** - Aim for >80% on new code
8. **Test edge cases** - Zero values, missing properties, etc.

## Summary

The testing infrastructure provides multiple levels of testing for different scenarios:

- **Unit tests**: Instant feedback during development
- **Component tests**: Verify individual parts work correctly
- **Integration tests**: Ensure the full pipeline works end-to-end
- **Docker tests**: Quick validation in production-like environment

Use the fastest test that gives you confidence. For most development work, unit tests in watch mode provide the best iteration speed.
