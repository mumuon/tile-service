#!/bin/bash
# Validates that new properties are present in generated tiles and database

set -e

REGION="${1:-delaware}"
TILES_DIR="$HOME/data/df/tiles/$REGION"

echo "=== Validating New Properties for $REGION ==="
echo ""

# Check if tiles directory exists
if [ ! -d "$TILES_DIR" ]; then
    echo "ERROR: Tiles directory not found: $TILES_DIR"
    echo "Please generate tiles first with: ./docker-generate.sh $REGION"
    exit 1
fi

# Find a sample tile to inspect
SAMPLE_TILE=$(find "$TILES_DIR" -name "*.pbf" | head -n 1)
if [ -z "$SAMPLE_TILE" ]; then
    echo "ERROR: No .pbf tiles found in $TILES_DIR"
    exit 1
fi

echo "Using sample tile: $SAMPLE_TILE"
echo ""

# 1. Check tiles with analyze-tiles
echo "[1/3] Checking tile properties..."

# Check if analyze-tiles binary exists
if [ ! -f "./cmd/analyze-tiles/analyze-tiles" ]; then
    echo "Building analyze-tiles..."
    cd cmd/analyze-tiles && go build && cd ../..
fi

# Analyze tile and extract properties
PROPS=$(./cmd/analyze-tiles/analyze-tiles --tile "$SAMPLE_TILE" 2>&1 | grep -E "Name|length|startLat|startLng|endLat|endLng" || true)

if echo "$PROPS" | grep -q "length"; then
    echo "✓ Tiles contain 'length' property"
else
    echo "✗ Tiles missing 'length' property"
    exit 1
fi

if echo "$PROPS" | grep -q "startLat"; then
    echo "✓ Tiles contain 'startLat' property"
else
    echo "✗ Tiles missing 'startLat' property"
    exit 1
fi

if echo "$PROPS" | grep -q "endLat"; then
    echo "✓ Tiles contain 'endLat' property"
else
    echo "✗ Tiles missing 'endLat' property"
    exit 1
fi

echo ""

# 2. Check database schema (if DATABASE_URL is set)
if [ -n "$DATABASE_URL" ]; then
    echo "[2/3] Checking database schema..."

    SCHEMA=$(psql "$DATABASE_URL" -c "\d \"RoadGeometry\"" 2>&1 || echo "")

    if echo "$SCHEMA" | grep -q "length"; then
        echo "✓ Database has 'length' column"
    else
        echo "✗ Database missing 'length' column"
        exit 1
    fi

    if echo "$SCHEMA" | grep -q "startLat"; then
        echo "✓ Database has 'startLat' column"
    else
        echo "✗ Database missing 'startLat' column"
        exit 1
    fi

    if echo "$SCHEMA" | grep -q "endLat"; then
        echo "✓ Database has 'endLat' column"
    else
        echo "✗ Database missing 'endLat' column"
        exit 1
    fi

    echo ""

    # 3. Check database data
    echo "[3/3] Checking database data..."

    COUNT=$(psql "$DATABASE_URL" -t -c "SELECT COUNT(*) FROM \"RoadGeometry\" WHERE region='$REGION' AND length IS NOT NULL" 2>&1 || echo "0")
    TOTAL=$(psql "$DATABASE_URL" -t -c "SELECT COUNT(*) FROM \"RoadGeometry\" WHERE region='$REGION'" 2>&1 || echo "0")

    COUNT=$(echo "$COUNT" | tr -d ' ')
    TOTAL=$(echo "$TOTAL" | tr -d ' ')

    echo "Roads with length data: $COUNT / $TOTAL"

    if [ "$COUNT" -gt "0" ]; then
        echo "✓ Database contains roads with length data"

        # Show a sample
        echo ""
        echo "Sample road data:"
        psql "$DATABASE_URL" -c "SELECT \"roadId\", length, \"startLat\", \"endLat\" FROM \"RoadGeometry\" WHERE region='$REGION' AND length IS NOT NULL LIMIT 3" 2>&1 || true
    else
        echo "⚠ WARNING: No roads have length data in database"
        echo "  This might be expected if tiles were generated before this implementation"
        echo "  Regenerate tiles with: ./docker-generate.sh $REGION"
    fi
else
    echo "[2/3] Skipping database checks (DATABASE_URL not set)"
    echo "[3/3] Skipping database data checks (DATABASE_URL not set)"
fi

echo ""
echo "=== Validation Complete ==="
