#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
REGION=${1:-delaware}
KMZ_PATH="${KMZ_PATH:-$HOME/data/df/curvature-data/$REGION.kmz}"
OLD_GEOJSON="${OLD_GEOJSON:-../df/output/$REGION.geojson}"
NEW_GEOJSON="${NEW_GEOJSON:-./output/$REGION.geojson}"
OLD_TILES="${OLD_TILES:-$HOME/data/df/tiles/$REGION}"
NEW_TILES="${NEW_TILES:-$HOME/data/df/tiles/$REGION}"

echo "============================================================================="
echo "                    TILE PIPELINE VALIDATION SUITE"
echo "============================================================================="
echo "Region: $REGION"
echo "KMZ:    $KMZ_PATH"
echo ""

# Check if tools are built
echo -e "${BLUE}[1/5]${NC} Checking tools..."
cd "$(dirname "$0")/.."

if [ ! -f "./cmd/analyze-kml/main.go" ]; then
    echo -e "${RED}Error: analyze-kml tool not found${NC}"
    exit 1
fi

if [ ! -f "./cmd/compare-geojson/main.go" ]; then
    echo -e "${RED}Error: compare-geojson tool not found${NC}"
    exit 1
fi

if [ ! -f "./cmd/analyze-tiles/main.go" ]; then
    echo -e "${RED}Error: analyze-tiles tool not found${NC}"
    exit 1
fi

# Build tools
echo -e "${BLUE}Building validation tools...${NC}"
go build -o ./bin/analyze-kml ./cmd/analyze-kml
go build -o ./bin/compare-geojson ./cmd/compare-geojson
go build -o ./bin/analyze-tiles ./cmd/analyze-tiles
echo -e "${GREEN}✓ Tools built${NC}"
echo ""

# Step 1: Analyze source KML
echo "============================================================================="
echo -e "${BLUE}[2/5]${NC} Analyzing source KML (ground truth)..."
echo "============================================================================="
if [ ! -f "$KMZ_PATH" ]; then
    echo -e "${RED}Error: KMZ file not found at $KMZ_PATH${NC}"
    echo "Set KMZ_PATH environment variable or pass region as argument"
    exit 1
fi

./bin/analyze-kml "$KMZ_PATH"
echo ""

# Step 2: Compare GeoJSON outputs
echo "============================================================================="
echo -e "${BLUE}[3/5]${NC} Comparing GeoJSON outputs (KML parsing stage)..."
echo "============================================================================="

GEOJSON_COMPARISON_POSSIBLE=true

if [ ! -f "$OLD_GEOJSON" ]; then
    echo -e "${YELLOW}Warning: OLD GeoJSON not found at $OLD_GEOJSON${NC}"
    echo "Run the old pipeline first: cd ../df && ./scripts/generate-tiles.sh $REGION"
    GEOJSON_COMPARISON_POSSIBLE=false
fi

if [ ! -f "$NEW_GEOJSON" ]; then
    echo -e "${YELLOW}Warning: NEW GeoJSON not found at $NEW_GEOJSON${NC}"
    echo "Run the new pipeline first to generate GeoJSON"
    GEOJSON_COMPARISON_POSSIBLE=false
fi

if [ "$GEOJSON_COMPARISON_POSSIBLE" = true ]; then
    ./bin/compare-geojson "$OLD_GEOJSON" "$NEW_GEOJSON"
else
    echo -e "${YELLOW}Skipping GeoJSON comparison (files not found)${NC}"
fi
echo ""

# Step 3: Analyze OLD tiles
echo "============================================================================="
echo -e "${BLUE}[4/5]${NC} Analyzing OLD tiles (tippecanoe output)..."
echo "============================================================================="

if [ -d "$OLD_TILES" ]; then
    echo -e "${GREEN}OLD Tiles:${NC}"
    ./bin/analyze-tiles "$OLD_TILES"
else
    echo -e "${YELLOW}Warning: OLD tiles not found at $OLD_TILES${NC}"
    echo "Run old pipeline first: cd ../df && ./scripts/generate-tiles.sh $REGION"
fi
echo ""

# Step 4: Analyze NEW tiles
echo "============================================================================="
echo -e "${BLUE}[5/5]${NC} Analyzing NEW tiles (tippecanoe output)..."
echo "============================================================================="

if [ -d "$NEW_TILES" ]; then
    echo -e "${GREEN}NEW Tiles:${NC}"
    ./bin/analyze-tiles "$NEW_TILES"
else
    echo -e "${YELLOW}Warning: NEW tiles not found at $NEW_TILES${NC}"
    echo "Run new pipeline first: ./docker-generate.sh $REGION --skip-upload"
fi
echo ""

# Summary
echo "============================================================================="
echo -e "${GREEN}VALIDATION COMPLETE${NC}"
echo "============================================================================="
echo ""
echo "Next Steps:"
echo "  1. Review ground truth counts from KML analysis"
echo "  2. Check if GeoJSON has same coordinate count (no data loss in parsing)"
echo "  3. Compare tile feature counts to GeoJSON (identify tippecanoe drops)"
echo "  4. If NEW has fewer features than OLD in tiles, adjust tippecanoe params"
echo ""
echo "Key Questions to Answer:"
echo "  - Does NEW GeoJSON preserve all coordinates from source?"
echo "  - What % of features does tippecanoe drop?"
echo "  - Does OLD vs NEW use different tippecanoe parameters?"
echo ""
echo "To run with custom paths:"
echo "  KMZ_PATH=/path/to/file.kmz \\"
echo "  OLD_GEOJSON=/path/to/old.geojson \\"
echo "  NEW_GEOJSON=/path/to/new.geojson \\"
echo "  OLD_TILES=/path/to/old/tiles \\"
echo "  NEW_TILES=/path/to/new/tiles \\"
echo "  ./scripts/validate-pipeline.sh <region>"
echo ""
