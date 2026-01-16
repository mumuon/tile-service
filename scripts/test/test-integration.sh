#!/bin/bash
# Full integration test - runs the complete pipeline locally
# Usage: ./scripts/test/test-integration.sh [region] [options]
#   region: Region name (default: test-region)
#   --max-zoom N: Set max zoom level (default: 10)
#   --cleanup: Clean up test files after completion
#   --upload: Actually upload to R2 (default: skip upload)

set -e

REGION="${1:-test-region}"
MAX_ZOOM="10"
CLEANUP=false
SKIP_UPLOAD="--skip-upload"

# Parse options
shift || true
while [[ $# -gt 0 ]]; do
    case $1 in
        --max-zoom)
            MAX_ZOOM="$2"
            shift 2
            ;;
        --cleanup)
            CLEANUP=true
            shift
            ;;
        --upload)
            SKIP_UPLOAD=""
            shift
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

TEST_OUTPUT="test-output"
TILES_OUTPUT="public/tiles/$REGION"

echo "========================================="
echo "  Integration Test - Full Pipeline"
echo "========================================="
echo "Region: $REGION"
echo "Max zoom: $MAX_ZOOM"
echo "Skip upload: $([ -n "$SKIP_UPLOAD" ] && echo "yes" || echo "no")"
echo "Cleanup: $([ "$CLEANUP" = true ] && echo "yes" || echo "no")"
echo ""

# Cleanup function
cleanup_test() {
    if [ "$CLEANUP" = true ]; then
        echo "Cleaning up test files..."
        rm -rf "$TEST_OUTPUT"
        rm -rf "$TILES_OUTPUT"
        rm -f "${REGION}-extraction.json"
        rm -f "${REGION}-extraction-progress.json"
        echo "✓ Cleanup complete"
    fi
}

trap cleanup_test EXIT

# Step 1: Build the service
echo "Step 1: Building tile-service..."
echo "----------------------------------------"
START_BUILD=$(date +%s)
go build -o tile-service
END_BUILD=$(date +%s)
BUILD_TIME=$((END_BUILD - START_BUILD))
echo "✓ Build complete (${BUILD_TIME}s)"
echo ""

# Step 2: Verify input file exists
echo "Step 2: Verifying input files..."
echo "----------------------------------------"
KMZ_FILE="curvature-data/${REGION}.kmz"

if [ ! -f "$KMZ_FILE" ]; then
    echo "Error: KMZ file not found at $KMZ_FILE"
    echo ""
    echo "Available regions:"
    ls -1 curvature-data/*.kmz 2>/dev/null | sed 's/.*\//  /' || echo "  (none)"
    exit 1
fi

echo "✓ Found KMZ file: $KMZ_FILE"
echo "  Size: $(du -h "$KMZ_FILE" | cut -f1)"
echo ""

# Step 3: Run the full pipeline
echo "Step 3: Running tile generation pipeline..."
echo "----------------------------------------"
echo "Command: ./tile-service -max-zoom $MAX_ZOOM $SKIP_UPLOAD -debug $REGION"
echo ""

START_PIPELINE=$(date +%s)

./tile-service \
    -max-zoom "$MAX_ZOOM" \
    $SKIP_UPLOAD \
    -no-cleanup \
    -debug \
    "$REGION" 2>&1 | tee /tmp/integration-test-${REGION}.log

END_PIPELINE=$(date +%s)
PIPELINE_TIME=$((END_PIPELINE - START_PIPELINE))

echo ""
echo "✓ Pipeline complete (${PIPELINE_TIME}s)"
echo ""

# Step 4: Verify outputs
echo "Step 4: Verifying outputs..."
echo "----------------------------------------"

# Check for GeoJSON file
GEOJSON_FILE="${REGION}.geojson"
if [ -f "$GEOJSON_FILE" ]; then
    echo "✓ GeoJSON file created: $GEOJSON_FILE"
    echo "  Size: $(du -h "$GEOJSON_FILE" | cut -f1)"

    # Count features
    if command -v jq &> /dev/null; then
        FEATURE_COUNT=$(jq '.features | length' "$GEOJSON_FILE")
        echo "  Features: $FEATURE_COUNT roads"

        # Check for new properties in first feature
        echo ""
        echo "  Checking new properties in first road..."
        SAMPLE_PROPS=$(jq '.features[0].properties' "$GEOJSON_FILE")

        if echo "$SAMPLE_PROPS" | jq -e '.length' > /dev/null 2>&1; then
            LENGTH=$(echo "$SAMPLE_PROPS" | jq -r '.length')
            echo "    ✓ length: ${LENGTH}m"
        else
            echo "    ✗ length: missing"
        fi

        if echo "$SAMPLE_PROPS" | jq -e '.startLat' > /dev/null 2>&1; then
            START_LAT=$(echo "$SAMPLE_PROPS" | jq -r '.startLat')
            START_LNG=$(echo "$SAMPLE_PROPS" | jq -r '.startLng')
            echo "    ✓ start: ($START_LAT, $START_LNG)"
        else
            echo "    ✗ start point: missing"
        fi

        if echo "$SAMPLE_PROPS" | jq -e '.curvature' > /dev/null 2>&1; then
            CURV=$(echo "$SAMPLE_PROPS" | jq -r '.curvature')
            echo "    ✓ curvature: $CURV"
        else
            echo "    ℹ curvature: not set (may be normal)"
        fi
    fi
else
    echo "⚠ Warning: GeoJSON file not found (may have been cleaned up)"
fi

echo ""

# Check for tiles
if [ -d "$TILES_OUTPUT" ]; then
    echo "✓ Tiles directory created: $TILES_OUTPUT"

    TILE_COUNT=$(find "$TILES_OUTPUT" -name "*.pbf" | wc -l | tr -d ' ')
    TOTAL_SIZE=$(du -sh "$TILES_OUTPUT" | cut -f1)

    echo "  Tiles: $TILE_COUNT files"
    echo "  Total size: $TOTAL_SIZE"
    echo ""
    echo "  Tile structure:"
    find "$TILES_OUTPUT" -type d | head -10 | sed 's/^/    /'

    # Check a sample tile for new properties
    SAMPLE_TILE=$(find "$TILES_OUTPUT" -name "*.pbf" | head -1)
    if [ -n "$SAMPLE_TILE" ]; then
        echo ""
        echo "  Sample tile: $SAMPLE_TILE"
        echo "  Size: $(du -h "$SAMPLE_TILE" | cut -f1)"

        # Note: Would need tippecanoe-decode to inspect tile properties
        if command -v tippecanoe-decode &> /dev/null; then
            echo "  Properties in tile:"
            tippecanoe-decode "$SAMPLE_TILE" | head -20 | grep -o '"[^"]*":' | sort -u | sed 's/^/    /'
        fi
    fi
else
    echo "✗ Error: Tiles directory not found at $TILES_OUTPUT"
    exit 1
fi

echo ""

# Check for extraction files (if geometry extraction ran)
EXTRACTION_FILE="${REGION}-extraction.json"
if [ -f "$EXTRACTION_FILE" ]; then
    echo "✓ Road extraction file: $EXTRACTION_FILE"
    echo "  Size: $(du -h "$EXTRACTION_FILE" | cut -f1)"

    if command -v jq &> /dev/null; then
        ROAD_COUNT=$(jq '. | length' "$EXTRACTION_FILE")
        echo "  Extracted roads: $ROAD_COUNT"

        # Check first road for new properties
        echo ""
        echo "  Checking first extracted road..."
        SAMPLE_ROAD=$(jq '.[0]' "$EXTRACTION_FILE")

        if echo "$SAMPLE_ROAD" | jq -e '.length' > /dev/null 2>&1; then
            LENGTH=$(echo "$SAMPLE_ROAD" | jq -r '.length')
            echo "    ✓ length: ${LENGTH}m"
        else
            echo "    ℹ length: not set"
        fi

        if echo "$SAMPLE_ROAD" | jq -e '.startLat' > /dev/null 2>&1; then
            echo "    ✓ start/end points: present"
        else
            echo "    ℹ start/end points: not set"
        fi
    fi
    echo ""
fi

# Step 5: Performance summary
echo "Step 5: Performance Summary"
echo "----------------------------------------"
echo "Build time: ${BUILD_TIME}s"
echo "Pipeline time: ${PIPELINE_TIME}s"
echo "Total time: $((BUILD_TIME + PIPELINE_TIME))s"
echo ""

echo "========================================="
echo "  Integration Test Complete ✓"
echo "========================================="
echo ""
echo "Log file: /tmp/integration-test-${REGION}.log"
echo ""

if [ "$CLEANUP" = false ]; then
    echo "Test files preserved in:"
    echo "  - Tiles: $TILES_OUTPUT"
    [ -f "$GEOJSON_FILE" ] && echo "  - GeoJSON: $GEOJSON_FILE"
    [ -f "$EXTRACTION_FILE" ] && echo "  - Extraction: $EXTRACTION_FILE"
    echo ""
    echo "To clean up: rm -rf $TILES_OUTPUT $GEOJSON_FILE $EXTRACTION_FILE"
fi
