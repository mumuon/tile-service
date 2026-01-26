#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BINARY="$SCRIPT_DIR/tile-service"
TILES_BASE="${TILES_DIR:-$HOME/data/df/tiles}"

usage() {
    echo "Usage: $0 <region> [--all] [--min-zoom N] [--max-zoom N] [--samples-per-zoom N]"
    echo ""
    echo "Verify tile integrity for a region."
    echo ""
    echo "Arguments:"
    echo "  <region>              Region name (e.g., washington, california)"
    echo "  --all                 Run all available checks (tiles, merge, upload)"
    echo "  --min-zoom N          Minimum expected zoom level (default: 5)"
    echo "  --max-zoom N          Maximum expected zoom level (default: 16)"
    echo "  --samples-per-zoom N  Tiles to spot-check per zoom on R2 (default: 5)"
    echo ""
    echo "Environment:"
    echo "  TILES_DIR             Override tiles base directory (default: ~/data/df/tiles)"
    echo ""
    echo "Available regions:"
    if [ -d "$TILES_BASE" ]; then
        for d in "$TILES_BASE"/*/; do
            name=$(basename "$d")
            # Skip numeric dirs (stray zoom levels) and merged
            if [[ "$name" =~ ^[0-9]+$ ]] || [ "$name" = "merged" ]; then
                continue
            fi
            echo "  $name"
        done
    else
        echo "  (tiles directory not found: $TILES_BASE)"
    fi
    exit 1
}

# Parse arguments
REGION=""
RUN_ALL=false
MIN_ZOOM=0
MAX_ZOOM=16
SAMPLES=5

while [ $# -gt 0 ]; do
    case "$1" in
        --all)      RUN_ALL=true; shift ;;
        --min-zoom) MIN_ZOOM="$2"; shift 2 ;;
        --max-zoom) MAX_ZOOM="$2"; shift 2 ;;
        --samples-per-zoom) SAMPLES="$2"; shift 2 ;;
        -h|--help)  usage ;;
        -*)         echo "Unknown option: $1"; usage ;;
        *)
            if [ -z "$REGION" ]; then
                REGION="$1"
            else
                echo "Unexpected argument: $1"
                usage
            fi
            shift
            ;;
    esac
done

if [ -z "$REGION" ]; then
    usage
fi

REGION_DIR="$TILES_BASE/$REGION"
MERGED_DIR="$TILES_BASE/merged"

# Build if needed
if [ ! -f "$BINARY" ] || [ "$BINARY" -ot "$SCRIPT_DIR/verify.go" ]; then
    echo "==> Building tile-service..."
    (cd "$SCRIPT_DIR" && go build -o tile-service .)
fi

# Check region exists
if [ ! -d "$REGION_DIR" ]; then
    echo "ERROR: Region directory not found: $REGION_DIR"
    echo ""
    echo "Available regions:"
    for d in "$TILES_BASE"/*/; do
        name=$(basename "$d")
        if [[ "$name" =~ ^[0-9]+$ ]] || [ "$name" = "merged" ]; then
            continue
        fi
        echo "  $name"
    done
    exit 1
fi

FAILED=0

# 1. Tile integrity check
echo ""
echo "===== Tile Integrity: $REGION (zoom $MIN_ZOOM-$MAX_ZOOM) ====="
if "$BINARY" verify tiles "$REGION_DIR" --min-zoom "$MIN_ZOOM" --max-zoom "$MAX_ZOOM"; then
    echo "RESULT: PASS"
else
    echo "RESULT: FAIL"
    FAILED=1
fi

# 2. Merge integrity check (if merged dir exists and --all)
if [ "$RUN_ALL" = true ] && [ -d "$MERGED_DIR" ]; then
    echo ""
    echo "===== Merge Integrity: $REGION ====="
    if "$BINARY" verify merge "$REGION" 2>&1; then
        echo "RESULT: PASS"
    else
        echo "RESULT: FAIL"
        FAILED=1
    fi
elif [ "$RUN_ALL" = true ]; then
    echo ""
    echo "===== Merge Integrity: SKIPPED (no merged directory) ====="
fi

# 3. Upload spot-check (if --all and .env exists)
if [ "$RUN_ALL" = true ] && [ -f "$SCRIPT_DIR/.env" ]; then
    echo ""
    echo "===== Upload Verification: $REGION ($SAMPLES samples/zoom) ====="
    if "$BINARY" verify upload "$REGION" --samples-per-zoom "$SAMPLES" 2>&1; then
        echo "RESULT: PASS"
    else
        echo "RESULT: FAIL"
        FAILED=1
    fi
elif [ "$RUN_ALL" = true ]; then
    echo ""
    echo "===== Upload Verification: SKIPPED (no .env config) ====="
fi

echo ""
if [ $FAILED -eq 0 ]; then
    echo "All checks passed for $REGION."
else
    echo "Some checks FAILED for $REGION."
    exit 1
fi
