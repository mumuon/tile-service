#!/bin/bash
# Test the geometry extractor (tiles -> road geometry with new properties)
# Usage: ./scripts/test/test-extractor.sh [tile-directory]

set -e

TILE_DIR="${1:-test-tiles/test-region}"

echo "========================================="
echo "  Testing Geometry Extractor Component"
echo "========================================="
echo "Tile directory: $TILE_DIR"
echo ""

# Step 1: Run unit tests
echo "Step 1: Running geometry extractor unit tests..."
echo "----------------------------------------"
go test -v -run "TestGeometryExtractorBasic|TestTilePathParsing|TestProgressSaveLoad|TestRoadsSaveLoad|TestFindPBFFiles|TestCleanupExtractionFiles"
echo ""

# Step 2: Run bounding box tests
echo "Step 2: Running bounding box calculation tests..."
echo "----------------------------------------"
go test -v -run "TestBoundAccessors|TestCalculateBoundsComprehensive|TestRoadSegmentBoundingBox"
echo ""

# Step 3: Test with real tiles (if available)
if [ -d "$TILE_DIR" ]; then
    echo "Step 3: Testing extraction from real tiles..."
    echo "----------------------------------------"

    # Count tiles
    TILE_COUNT=$(find "$TILE_DIR" -name "*.pbf" | wc -l | tr -d ' ')
    echo "Found $TILE_COUNT tile files in $TILE_DIR"
    echo ""

    # Run single tile extraction test
    echo "Running single tile extraction test..."
    go test -v -run "TestSingleTileExtraction"
    echo ""

    # Run new properties test
    echo "Running new properties extraction test..."
    go test -v -run "TestExtractRoadWithNewProperties"
    echo ""

    # Show sample tile
    SAMPLE_TILE=$(find "$TILE_DIR" -name "*.pbf" | head -1)
    if [ -n "$SAMPLE_TILE" ]; then
        echo "Sample tile: $SAMPLE_TILE"
        echo "Size: $(du -h "$SAMPLE_TILE" | cut -f1)"
    fi
else
    echo "Step 3: Skipping real tile tests..."
    echo "----------------------------------------"
    echo "Warning: Tile directory not found at $TILE_DIR"
    echo "To test with real tiles:"
    echo "  1. Generate tiles first: ./tile-service -skip-upload test-region"
    echo "  2. Then run: ./scripts/test/test-extractor.sh public/tiles/test-region"
fi

echo ""
echo "========================================="
echo "  Extractor Test Complete"
echo "========================================="
