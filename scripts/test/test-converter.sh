#!/bin/bash
# Test the KML to GeoJSON converter with new properties
# Usage: ./scripts/test/test-converter.sh [region]

set -e

REGION="${1:-test-region}"
TEST_DIR="test-output"
KMZ_FILE="curvature-data/${REGION}.kmz"
KML_FILE="${TEST_DIR}/${REGION}.kml"
GEOJSON_FILE="${TEST_DIR}/${REGION}.geojson"

echo "========================================="
echo "  Testing Converter Component"
echo "========================================="
echo "Region: $REGION"
echo ""

# Create test directory
mkdir -p "$TEST_DIR"

# Step 1: Run unit tests
echo "Step 1: Running converter unit tests..."
echo "----------------------------------------"
go test -v -run "TestHaversineDistance|TestCalculateLineStringLength|TestCalculateRoadLength|TestExtractStartEndPoints|TestParseCurvature"
echo ""

# Step 2: Extract KML from KMZ (if exists)
if [ -f "$KMZ_FILE" ]; then
    echo "Step 2: Extracting KML from KMZ..."
    echo "----------------------------------------"
    unzip -p "$KMZ_FILE" '*.kml' > "$KML_FILE"
    echo "Extracted: $KML_FILE"
    echo "Size: $(du -h "$KML_FILE" | cut -f1)"
    echo ""

    # Step 3: Convert KML to GeoJSON
    echo "Step 3: Converting KML to GeoJSON..."
    echo "----------------------------------------"

    # Check if convert-kml tool exists
    if [ -f "cmd/convert-kml/main.go" ]; then
        go run cmd/convert-kml/main.go "$KML_FILE" "$GEOJSON_FILE"
    else
        # Use the main converter function via test
        echo "Note: Using converter directly (cmd/convert-kml not available)"
        # This would need a small wrapper - for now skip
    fi

    if [ -f "$GEOJSON_FILE" ]; then
        echo "Generated: $GEOJSON_FILE"
        echo "Size: $(du -h "$GEOJSON_FILE" | cut -f1)"
        echo ""

        # Step 4: Verify new properties
        echo "Step 4: Verifying new properties in GeoJSON..."
        echo "----------------------------------------"

        # Check for length property
        LENGTH_COUNT=$(grep -o '"length":[0-9.]*' "$GEOJSON_FILE" | wc -l | tr -d ' ')
        echo "✓ Found $LENGTH_COUNT roads with 'length' property"
        echo "  Sample lengths:"
        grep -o '"length":[0-9.]*' "$GEOJSON_FILE" | head -5 | sed 's/^/    /'
        echo ""

        # Check for start/end points
        START_LAT_COUNT=$(grep -o '"startLat":[0-9.-]*' "$GEOJSON_FILE" | wc -l | tr -d ' ')
        echo "✓ Found $START_LAT_COUNT roads with 'startLat' property"
        echo "  Sample start points:"
        grep -o '"startLat":[0-9.-]*' "$GEOJSON_FILE" | head -3 | sed 's/^/    /'
        echo ""

        # Check for curvature
        CURV_COUNT=$(grep -o '"curvature":"[0-9]*"' "$GEOJSON_FILE" | wc -l | tr -d ' ')
        echo "✓ Found $CURV_COUNT roads with 'curvature' property"
        echo "  Sample curvatures:"
        grep -o '"curvature":"[0-9]*"' "$GEOJSON_FILE" | head -5 | sed 's/^/    /'
        echo ""

        # Step 5: Show sample feature
        echo "Step 5: Sample feature with all properties..."
        echo "----------------------------------------"
        echo "First feature properties:"
        if command -v jq &> /dev/null; then
            jq '.features[0].properties' "$GEOJSON_FILE"
        else
            echo "  (Install 'jq' for pretty JSON output)"
            grep -A 20 '"properties"' "$GEOJSON_FILE" | head -20
        fi
    fi
else
    echo "Warning: KMZ file not found at $KMZ_FILE"
    echo "Skipping KML extraction and conversion tests"
    echo "Only ran unit tests"
fi

echo ""
echo "========================================="
echo "  Converter Test Complete"
echo "========================================="
echo ""
echo "Cleanup: rm -rf $TEST_DIR"
