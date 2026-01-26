# Testing Guide

## Overview

This document describes the test suite for the road geometry extraction feature, including unit tests and feature parity validation between the TypeScript and Go implementations.

## Test Suite

### Unit Tests (`geometry_extractor_test.go`)

Go unit tests that validate individual components of the geometry extractor.

**Running unit tests:**
```bash
cd tile-service

# Run all tests
go test -v

# Run specific test
go test -v -run TestTilePathParsing

# Run with coverage
go test -cover
```

**Test Coverage:**

1. **TestGeometryExtractorBasic**
   - Validates extractor initialization
   - Ensures basic object creation works

2. **TestTilePathParsing**
   - Tests parsing of tile file paths to extract Z/X/Y coordinates
   - Validates error handling for invalid paths
   - Covers various path formats and edge cases

3. **TestProgressSaveLoad**
   - Tests progress file persistence
   - Validates JSON serialization/deserialization
   - Ensures resumability works correctly

4. **TestRoadsSaveLoad**
   - Tests road data persistence to JSON
   - Validates data integrity after save/load cycle
   - Checks all field types (including optional curvature)

5. **TestFindPBFFiles**
   - Tests recursive file discovery
   - Validates filtering of .pbf files
   - Ensures non-.pbf files are ignored

6. **TestCleanupExtractionFiles**
   - Tests cleanup of temporary extraction files
   - Validates file deletion

### Feature Parity Test (`parity_test.sh`)

Bash script that compares extraction results between TypeScript and Go implementations to ensure feature parity.

**Running parity test:**
```bash
cd tile-service

# Test with specific region (must have existing tiles)
./parity_test.sh oregon

# Or use default test region
./parity_test.sh test_region
```

**What it tests:**

1. **Prerequisites Check**
   - Verifies Go binary exists
   - Verifies TypeScript script exists
   - Checks for test tiles

2. **Extraction Execution**
   - Runs Go extraction
   - Runs TypeScript extraction
   - Captures both results

3. **Result Comparison**
   - Compares road counts
   - Validates data structure
   - Checks field types
   - Compares bounding boxes (with floating point tolerance)

4. **Data Validation**
   - Ensures required fields present
   - Validates numeric types
   - Checks bounds logic (minLat < maxLat, etc.)

5. **Report Generation**
   - Provides detailed pass/fail report
   - Shows side-by-side comparison
   - Saves comparison files for debugging

**Expected Output:**
```
╔════════════════════════════════════════════════════════════════╗
║  Feature Parity Test: TypeScript vs Go                        ║
║  Road Geometry Extraction Validation                          ║
╚════════════════════════════════════════════════════════════════╝

[1/7] Checking prerequisites...
✓ All prerequisites met

[2/7] Cleaning up existing extraction files...
✓ Cleanup complete

[3/7] Running Go extraction...
✓ Go extracted 1,322 roads

[4/7] Running TypeScript extraction...
✓ TypeScript extracted 1,322 roads

[5/7] Comparing extraction results...
✓ Road count matches: 1,322 roads

  Sampling first 5 roads for detailed comparison...
  ✓ Road 0 (US Route 26): Bounds match
  ✓ Road 1 (Highway 101): Bounds match
  ✓ Road 2 (I-84): Bounds match
  ✓ Road 3 (Highway 35): Bounds match
  ✓ Road 4 (Gorge Road): Bounds match

✓ Sample comparison passed

[6/7] Validating data structure...
✓ All required fields present
✓ Data types valid
✓ Bounds logic valid

[7/7] Performance comparison...
  Go Service:     ~2-3 minutes
  TypeScript:     ~10-15 minutes

╔════════════════════════════════════════════════════════════════╗
║  PARITY TEST RESULTS                                           ║
╚════════════════════════════════════════════════════════════════╝

✓ All parity tests passed!

Summary:
  - Road count matches between Go and TypeScript
  - Data structure is identical
  - Bounds calculations are consistent
  - Required fields are present

The Go service has feature parity with the TypeScript script.
```

## Test Requirements

### For Unit Tests

- Go 1.21+ installed
- `paulmach/orb` library dependencies
- Write permissions for test files

### For Parity Tests

- Both Go binary (`tile-service`) and TypeScript script available
- Test tiles in `public/tiles/<region>/` directory
- `jq` command-line JSON processor
- Node.js and TypeScript for running TS script
- Database credentials (for full integration test)

**Installing jq:**
```bash
# macOS
brew install jq

# Ubuntu/Debian
apt-get install jq

# Or check: https://stedolan.github.io/jq/download/
```

## Creating Test Tiles

For comprehensive testing, you'll need actual tile data:

```bash
# Generate test tiles with minimal zoom for faster testing
./tile-service generate -max-zoom 7 -skip-upload -no-cleanup test_region

# This creates:
# - public/tiles/test_region/ directory
# - Smaller tile set for faster testing
```

## Continuous Integration

### GitHub Actions Example

```yaml
name: Test Road Geometry Extraction

on: [push, pull_request]

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - name: Run unit tests
        run: |
          cd tile-service
          go test -v -cover

  parity-test:
    runs-on: ubuntu-latest
    needs: unit-tests
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
      - uses: actions/setup-node@v3
      - name: Install dependencies
        run: |
          sudo apt-get install -y jq
          cd df && npm install
      - name: Generate test tiles
        run: |
          cd tile-service
          ./tile-service generate -max-zoom 7 -skip-upload test_region
      - name: Run parity test
        run: |
          cd tile-service
          ./parity_test.sh test_region
```

## Debugging Failed Tests

### Unit Test Failures

```bash
# Run with verbose output
go test -v

# Run specific test with debugging
go test -v -run TestName

# Check for race conditions
go test -race

# Generate coverage report
go test -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Parity Test Failures

When the parity test fails, comparison files are saved:

```bash
# Check the comparison files
cat /tmp/go-roads-test_region.json | jq '.[0]'
cat /tmp/ts-roads-test_region.json | jq '.[0]'

# Compare sorted versions
jq -S 'sort_by(.roadId)' /tmp/go-roads-test_region.json > /tmp/go-sorted.json
jq -S 'sort_by(.roadId)' /tmp/ts-roads-test_region.json > /tmp/ts-sorted.json
diff /tmp/go-sorted.json /tmp/ts-sorted.json
```

### Common Issues

**Issue: "test tiles not found"**
```bash
# Generate test tiles first
./tile-service generate -max-zoom 7 -skip-upload -no-cleanup maryland

# Or use existing region
ls public/tiles/  # See available regions
./parity_test.sh <region_name>
```

**Issue: "jq command not found"**
```bash
# Install jq
brew install jq  # macOS
sudo apt-get install jq  # Linux
```

**Issue: "extraction file not created"**
```bash
# Run with debug logging
./tile-service -debug extract public/tiles/oregon

# Check for errors in output
```

**Issue: "road count mismatch"**
```bash
# This could indicate:
# 1. Different tile versions
# 2. Algorithm difference
# 3. Coordinate conversion issue

# Investigate by comparing first few roads:
jq '.[0:5]' /tmp/go-roads-region.json
jq '.[0:5]' /tmp/ts-roads-region.json
```

## Performance Benchmarking

### Benchmark Extraction Speed

```bash
# Time Go extraction
time ./tile-service extract public/tiles/oregon

# Time TypeScript extraction
cd ../df
time npx ts-node scripts/extract-road-geometry.ts oregon --extract-only

# Compare results
```

### Memory Profiling

```bash
# Go memory profiling
go test -memprofile=mem.out
go tool pprof mem.out

# Watch memory during extraction
watch -n 1 'ps aux | grep tile-service | grep -v grep'
```

## Reporting Issues

When reporting test failures, include:

1. Test output (full verbose logs)
2. Go version: `go version`
3. Operating system
4. Test region and tile count
5. Comparison files (if parity test)
6. Steps to reproduce

## Future Test Enhancements

1. **Integration Tests**
   - End-to-end database insertion
   - API endpoint validation
   - Full pipeline testing

2. **Performance Tests**
   - Benchmark tile processing speed
   - Memory usage profiling
   - Large region stress testing

3. **Edge Case Tests**
   - Empty tiles
   - Corrupted PBF files
   - Missing roads layer
   - Extreme coordinates

4. **Regression Tests**
   - Known-good result snapshots
   - Automatic comparison on changes

## References

- [Go Testing Package](https://pkg.go.dev/testing)
- [Table-Driven Tests](https://dave.cheney.net/2019/05/07/prefer-table-driven-tests)
- [jq Manual](https://stedolan.github.io/jq/manual/)
