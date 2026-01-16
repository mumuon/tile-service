# Quick Test Start Guide

Get started testing in 30 seconds.

## Instant Testing (No Setup Required)

```bash
# Run all unit tests
make test

# Run with verbose output
make test-unit

# Watch mode - auto-run on file changes
make test-watch
```

## Test Your Recent Changes

Your recent changes added:
- Road length calculation
- Start/end point extraction
- Curvature parsing
- New API endpoints (regions, cancel job)

### Test 1: New Converter Functions (5 seconds)

```bash
make test-converter
```

This tests:
- Haversine distance calculations
- Road length computation
- Start/end point extraction from LineStrings
- Curvature parsing from descriptions

### Test 2: Geometry Extraction (5 seconds)

```bash
make test-extractor
```

This tests:
- New properties extracted from tiles
- Bounding box calculations
- Tile path parsing

### Test 3: New API Endpoints (10 seconds)

```bash
make test-api
```

This tests:
- `/api/regions` endpoint
- `/api/cancel/{jobId}` endpoint
- Job creation and status

### Test 4: Full Pipeline (1-2 minutes)

```bash
make test-integration
```

This runs the complete pipeline with a test region and verifies all new properties appear in the output.

## What Each Test Verifies

### Unit Tests (`make test`)
- ✅ haversineDistance() calculates correctly
- ✅ calculateLineStringLength() sums distances
- ✅ calculateRoadLength() handles LineString and MultiLineString
- ✅ extractStartEndPoints() finds first/last coordinates
- ✅ parseCurvature() extracts from "c_1000" and "curvature: 500"

### Converter Test (`make test-converter`)
- ✅ Real KML file converts to GeoJSON
- ✅ GeoJSON features have `length` property
- ✅ GeoJSON features have `startLat`, `startLng` properties
- ✅ GeoJSON features have `endLat`, `endLng` properties
- ✅ GeoJSON features have `curvature` property (when applicable)

### Integration Test (`make test-integration`)
- ✅ Complete pipeline runs without errors
- ✅ Tiles are generated
- ✅ Road geometry is extracted
- ✅ All new properties appear in output files

## Debugging

If a test fails:

```bash
# Run specific test with verbose output
go test -v -run TestHaversineDistance

# Keep test files for inspection
./scripts/test/test-integration.sh test-region
# Files in: test-output/ and public/tiles/test-region/

# Check what went wrong
cat test-output/test-region.geojson | jq '.features[0].properties'
```

## Quick Iteration Workflow

```bash
# Terminal 1: Start watch mode
make test-watch

# Terminal 2: Edit code
vim converter.go

# Save file → tests auto-run → fix → repeat
```

## Next Steps

For detailed information, see:
- **TESTING_WORKFLOW.md** - Complete testing guide
- **README.md** - Project overview

## Common Issues

**"fswatch not found"**
```bash
brew install fswatch  # macOS
```

**"jq not found"** (optional, for pretty output)
```bash
brew install jq  # macOS
```

**"Test region not found"**
- You need a `curvature-data/test-region.kmz` file
- Or use a real region: `./scripts/test/test-integration.sh delaware`
