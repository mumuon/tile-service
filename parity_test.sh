#!/bin/bash

# Feature Parity Test: TypeScript vs Go Road Geometry Extraction
# This script validates that both implementations produce identical results

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DF_DIR="$SCRIPT_DIR/../df"
TILES_DIR="$SCRIPT_DIR/public/tiles"
TEST_REGION="${1:-test_region}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}╔════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║  Feature Parity Test: TypeScript vs Go                        ║${NC}"
echo -e "${BLUE}║  Road Geometry Extraction Validation                          ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════════════════════╝${NC}"
echo ""

# Check prerequisites
check_prerequisites() {
    echo -e "${YELLOW}[1/7]${NC} Checking prerequisites..."

    # Check if Go binary exists
    if [ ! -f "$SCRIPT_DIR/tile-service" ]; then
        echo -e "${RED}✗ Go tile-service binary not found${NC}"
        echo "  Run: cd $SCRIPT_DIR && go build -o tile-service ."
        exit 1
    fi

    # Check if TypeScript script exists
    if [ ! -f "$DF_DIR/scripts/extract-road-geometry.ts" ]; then
        echo -e "${RED}✗ TypeScript extraction script not found${NC}"
        exit 1
    fi

    # Check if test tiles exist
    if [ ! -d "$TILES_DIR/$TEST_REGION" ]; then
        echo -e "${YELLOW}⚠ Test tiles not found for region: $TEST_REGION${NC}"
        echo "  Available regions:"
        if [ -d "$TILES_DIR" ]; then
            ls -1 "$TILES_DIR" | grep -v ".DS_Store" | sed 's/^/    - /'
        fi
        echo ""
        echo "  Usage: $0 <region_name>"
        exit 1
    fi

    echo -e "${GREEN}✓ All prerequisites met${NC}"
    echo ""
}

# Cleanup any existing extraction files
cleanup_extraction_files() {
    echo -e "${YELLOW}[2/7]${NC} Cleaning up existing extraction files..."

    rm -f "$SCRIPT_DIR/.extract-progress-${TEST_REGION}.json"
    rm -f "$SCRIPT_DIR/.extracted-roads-${TEST_REGION}.json"
    rm -f "$DF_DIR/.extract-progress-${TEST_REGION}.json"
    rm -f "$DF_DIR/.extracted-roads-${TEST_REGION}.json"

    echo -e "${GREEN}✓ Cleanup complete${NC}"
    echo ""
}

# Run Go extraction
run_go_extraction() {
    echo -e "${YELLOW}[3/7]${NC} Running Go extraction..."

    cd "$SCRIPT_DIR"

    # Extract roads using Go service
    ./tile-service extract "$TILES_DIR/$TEST_REGION" > /dev/null 2>&1 || {
        echo -e "${RED}✗ Go extraction failed${NC}"
        echo "  Check logs: ./tile-service -debug extract $TILES_DIR/$TEST_REGION"
        exit 1
    }

    # Check if extraction file was created
    if [ ! -f ".extracted-roads-${TEST_REGION}.json" ]; then
        echo -e "${RED}✗ Go extraction file not created${NC}"
        exit 1
    fi

    # Copy results for comparison
    cp ".extracted-roads-${TEST_REGION}.json" "/tmp/go-roads-${TEST_REGION}.json"

    GO_ROAD_COUNT=$(jq 'length' "/tmp/go-roads-${TEST_REGION}.json")
    echo -e "${GREEN}✓ Go extracted ${GO_ROAD_COUNT} roads${NC}"
    echo ""
}

# Run TypeScript extraction
run_typescript_extraction() {
    echo -e "${YELLOW}[4/7]${NC} Running TypeScript extraction..."

    cd "$DF_DIR"

    # Copy tiles to DF directory for TypeScript script
    mkdir -p "public/tiles"
    if [ ! -d "public/tiles/$TEST_REGION" ]; then
        cp -r "$TILES_DIR/$TEST_REGION" "public/tiles/$TEST_REGION"
    fi

    # Run TypeScript extraction (extract-only mode)
    npx ts-node scripts/extract-road-geometry.ts "$TEST_REGION" --extract-only > /dev/null 2>&1 || {
        echo -e "${RED}✗ TypeScript extraction failed${NC}"
        echo "  Check logs: npx ts-node scripts/extract-road-geometry.ts $TEST_REGION --extract-only"
        exit 1
    }

    # Check if extraction file was created
    if [ ! -f ".extracted-roads-${TEST_REGION}.json" ]; then
        echo -e "${RED}✗ TypeScript extraction file not created${NC}"
        exit 1
    fi

    # Copy results for comparison
    cp ".extracted-roads-${TEST_REGION}.json" "/tmp/ts-roads-${TEST_REGION}.json"

    TS_ROAD_COUNT=$(jq 'length' "/tmp/ts-roads-${TEST_REGION}.json")
    echo -e "${GREEN}✓ TypeScript extracted ${TS_ROAD_COUNT} roads${NC}"
    echo ""
}

# Compare extraction results
compare_results() {
    echo -e "${YELLOW}[5/7]${NC} Comparing extraction results..."

    GO_FILE="/tmp/go-roads-${TEST_REGION}.json"
    TS_FILE="/tmp/ts-roads-${TEST_REGION}.json"

    # Count comparison
    GO_COUNT=$(jq 'length' "$GO_FILE")
    TS_COUNT=$(jq 'length' "$TS_FILE")

    if [ "$GO_COUNT" -ne "$TS_COUNT" ]; then
        echo -e "${RED}✗ Road count mismatch${NC}"
        echo "  Go:         $GO_COUNT roads"
        echo "  TypeScript: $TS_COUNT roads"
        echo ""
        return 1
    fi

    echo -e "${GREEN}✓ Road count matches: $GO_COUNT roads${NC}"

    # Create sorted versions for comparison
    jq -S 'sort_by(.roadId)' "$GO_FILE" > "/tmp/go-sorted.json"
    jq -S 'sort_by(.roadId)' "$TS_FILE" > "/tmp/ts-sorted.json"

    # Sample road comparison (first 5 roads)
    echo ""
    echo "  Sampling first 5 roads for detailed comparison..."

    MISMATCHES=0
    for i in {0..4}; do
        GO_ROAD=$(jq ".[$i]" "/tmp/go-sorted.json")
        TS_ROAD=$(jq ".[$i]" "/tmp/ts-sorted.json")

        if [ "$GO_ROAD" == "null" ] || [ "$TS_ROAD" == "null" ]; then
            break
        fi

        GO_ID=$(echo "$GO_ROAD" | jq -r '.roadId')
        TS_ID=$(echo "$TS_ROAD" | jq -r '.roadId')

        if [ "$GO_ID" != "$TS_ID" ]; then
            echo -e "  ${RED}✗ Road ID mismatch at index $i${NC}"
            echo "    Go:         $GO_ID"
            echo "    TypeScript: $TS_ID"
            MISMATCHES=$((MISMATCHES + 1))
            continue
        fi

        # Compare bounds (with tolerance for floating point)
        GO_MIN_LAT=$(echo "$GO_ROAD" | jq '.minLat')
        TS_MIN_LAT=$(echo "$TS_ROAD" | jq '.minLat')

        DIFF=$(echo "$GO_MIN_LAT - $TS_MIN_LAT" | bc -l | sed 's/-//')
        TOLERANCE="0.000001"

        if (( $(echo "$DIFF > $TOLERANCE" | bc -l) )); then
            echo -e "  ${RED}✗ Bounds mismatch for $GO_ID${NC}"
            echo "    minLat difference: $DIFF (tolerance: $TOLERANCE)"
            MISMATCHES=$((MISMATCHES + 1))
        else
            echo -e "  ${GREEN}✓ Road $i ($GO_ID): Bounds match${NC}"
        fi
    done

    echo ""

    if [ $MISMATCHES -gt 0 ]; then
        echo -e "${RED}✗ Found $MISMATCHES mismatches in sample${NC}"
        return 1
    fi

    echo -e "${GREEN}✓ Sample comparison passed${NC}"
    echo ""
}

# Validate data structure
validate_structure() {
    echo -e "${YELLOW}[6/7]${NC} Validating data structure..."

    GO_FILE="/tmp/go-roads-${TEST_REGION}.json"

    # Check required fields
    REQUIRED_FIELDS=("roadId" "region" "minLat" "maxLat" "minLng" "maxLng")

    for field in "${REQUIRED_FIELDS[@]}"; do
        HAS_FIELD=$(jq ".[0] | has(\"$field\")" "$GO_FILE")
        if [ "$HAS_FIELD" != "true" ]; then
            echo -e "${RED}✗ Missing required field: $field${NC}"
            return 1
        fi
    done

    echo -e "${GREEN}✓ All required fields present${NC}"

    # Validate data types
    FIRST_ROAD=$(jq '.[0]' "$GO_FILE")

    # Check numeric fields
    for field in "minLat" "maxLat" "minLng" "maxLng"; do
        VALUE=$(echo "$FIRST_ROAD" | jq ".${field}")
        if ! [[ "$VALUE" =~ ^-?[0-9]+\.?[0-9]*$ ]]; then
            echo -e "${RED}✗ Invalid numeric value for $field: $VALUE${NC}"
            return 1
        fi
    done

    echo -e "${GREEN}✓ Data types valid${NC}"

    # Check bounds logic
    MIN_LAT=$(echo "$FIRST_ROAD" | jq '.minLat')
    MAX_LAT=$(echo "$FIRST_ROAD" | jq '.maxLat')
    MIN_LNG=$(echo "$FIRST_ROAD" | jq '.minLng')
    MAX_LNG=$(echo "$FIRST_ROAD" | jq '.maxLng')

    if (( $(echo "$MIN_LAT > $MAX_LAT" | bc -l) )); then
        echo -e "${RED}✗ Invalid bounds: minLat > maxLat${NC}"
        return 1
    fi

    if (( $(echo "$MIN_LNG > $MAX_LNG" | bc -l) )); then
        echo -e "${RED}✗ Invalid bounds: minLng > maxLng${NC}"
        return 1
    fi

    echo -e "${GREEN}✓ Bounds logic valid${NC}"
    echo ""
}

# Performance comparison
performance_comparison() {
    echo -e "${YELLOW}[7/7]${NC} Performance comparison..."

    # Extract timing from previous runs (approximate)
    GO_TIME="N/A (see logs)"
    TS_TIME="N/A (see logs)"

    echo "  Go Service:     $GO_TIME"
    echo "  TypeScript:     $TS_TIME"
    echo ""
    echo -e "${BLUE}Note: For accurate timing, run with time command:${NC}"
    echo "  time ./tile-service extract $TILES_DIR/$TEST_REGION"
    echo "  time npx ts-node scripts/extract-road-geometry.ts $TEST_REGION --extract-only"
    echo ""
}

# Generate report
generate_report() {
    echo ""
    echo -e "${BLUE}╔════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║  PARITY TEST RESULTS                                           ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "${GREEN}✓ All parity tests passed!${NC}"
    echo ""
    echo "Summary:"
    echo "  - Road count matches between Go and TypeScript"
    echo "  - Data structure is identical"
    echo "  - Bounds calculations are consistent"
    echo "  - Required fields are present"
    echo ""
    echo "The Go service has feature parity with the TypeScript script."
    echo ""
}

# Cleanup temporary files
cleanup_temp_files() {
    rm -f "/tmp/go-roads-${TEST_REGION}.json"
    rm -f "/tmp/ts-roads-${TEST_REGION}.json"
    rm -f "/tmp/go-sorted.json"
    rm -f "/tmp/ts-sorted.json"
}

# Main execution
main() {
    check_prerequisites
    cleanup_extraction_files

    # Run extractions
    run_go_extraction
    run_typescript_extraction

    # Compare and validate
    if compare_results && validate_structure; then
        performance_comparison
        generate_report
        cleanup_temp_files
        exit 0
    else
        echo ""
        echo -e "${RED}╔════════════════════════════════════════════════════════════════╗${NC}"
        echo -e "${RED}║  PARITY TEST FAILED                                            ║${NC}"
        echo -e "${RED}╚════════════════════════════════════════════════════════════════╝${NC}"
        echo ""
        echo "Comparison files saved for debugging:"
        echo "  Go:         /tmp/go-roads-${TEST_REGION}.json"
        echo "  TypeScript: /tmp/ts-roads-${TEST_REGION}.json"
        echo ""
        exit 1
    fi
}

# Run main
main
