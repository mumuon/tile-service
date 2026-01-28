#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BINARY="$SCRIPT_DIR/tile-service"
TILES_BASE="${TILES_DIR:-$HOME/data/df/tiles}"
MIN_ZOOM=0
MAX_ZOOM=6
WORKERS=10

# Build if needed
echo "==> Building tile-service..."
(cd "$SCRIPT_DIR" && go build -o tile-service .)

# Discover all regions from existing tile directories
REGIONS=()
for d in "$TILES_BASE"/*/; do
    name=$(basename "$d")
    # Skip numeric dirs (stray zoom levels), merged, and test-region
    if [[ "$name" =~ ^[0-9]+$ ]] || [ "$name" = "merged" ] || [ "$name" = "test-region" ]; then
        continue
    fi
    REGIONS+=("$name")
done

echo "==> Found ${#REGIONS[@]} regions: ${REGIONS[*]}"
echo "==> Generating zoom ${MIN_ZOOM}-${MAX_ZOOM} with ${WORKERS} workers"
echo ""

# Phase 1: Generate zoom 0-5 for all regions
echo "===== PHASE 1: Generate zoom ${MIN_ZOOM}-${MAX_ZOOM} ====="
"$BINARY" generate \
    -workers "$WORKERS" \
    -min-zoom "$MIN_ZOOM" \
    -max-zoom "$MAX_ZOOM" \
    -skip-upload \
    -skip-merge \
    -extract-geometry=false \
    "${REGIONS[@]}"

echo ""
echo "===== PHASE 2: Verify zoom ${MIN_ZOOM}-${MAX_ZOOM} ====="
FAILED=()
for region in "${REGIONS[@]}"; do
    if "$BINARY" verify tiles "$TILES_BASE/$region" --min-zoom "$MIN_ZOOM" --max-zoom "$MAX_ZOOM" 2>&1 | head -1; then
        :
    else
        FAILED+=("$region")
    fi
done

if [ ${#FAILED[@]} -gt 0 ]; then
    echo ""
    echo "VERIFICATION FAILED for: ${FAILED[*]}"
    echo "Aborting merge/upload."
    exit 1
fi
echo ""
echo "All ${#REGIONS[@]} regions verified for zoom ${MIN_ZOOM}-${MAX_ZOOM}."

# Phase 3: Merge zoom 0-5 only
echo ""
echo "===== PHASE 3: Merge zoom ${MIN_ZOOM}-${MAX_ZOOM} ====="
"$BINARY" merge \
    -skip-upload \
    -min-zoom "$MIN_ZOOM" \
    -max-zoom "$MAX_ZOOM"

echo ""
echo "===== PHASE 4: Upload zoom ${MIN_ZOOM}-${MAX_ZOOM} from merged ====="
"$BINARY" upload \
    -min-zoom "$MIN_ZOOM" \
    -max-zoom "$MAX_ZOOM" \
    "$TILES_BASE/merged"

echo ""
echo "===== DONE ====="
echo "Generated, verified, merged, and uploaded zoom ${MIN_ZOOM}-${MAX_ZOOM} for ${#REGIONS[@]} regions."
