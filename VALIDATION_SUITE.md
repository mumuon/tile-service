# Tile Pipeline Validation Suite

This validation suite helps you compare the old df Python/TypeScript scripts with the new Go tile-service to identify where data might be lost.

## Tools Included

### 1. `analyze-kml` - Ground Truth Analyzer
Parses KMZ/KML files to establish what SHOULD be in the output.

**Usage:**
```bash
go run ./cmd/analyze-kml/main.go ~/data/df/curvature-data/delaware.kmz
```

**Output:**
- Number of Folders (semantic roads)
- Number of Placemarks (road segments)
- Segment distribution
- Expected behavior for OLD vs NEW pipelines

### 2. `compare-geojson` - GeoJSON Comparison Tool
Compares OLD and NEW GeoJSON outputs to validate the KML parsing stage.

**Usage:**
```bash
go run ./cmd/compare-geojson/main.go old/delaware.geojson new/delaware.geojson
```

**Checks:**
- Feature count comparison
- Coordinate point count (detects data loss!)
- Geometry type distribution
- Missing road names
- Property completeness

**Critical Check:** If coordinate counts differ, data is being lost during parsing!

### 3. `analyze-tiles` - Tile Content Analyzer
Walks through a tile directory and counts features to measure tippecanoe dropping.

**Usage:**
```bash
go run ./cmd/analyze-tiles/main.go ~/data/df/tiles/delaware
```

**Output:**
- Total tiles and features
- Unique road IDs found
- Feature distribution by zoom level
- Layers found in tiles

### 4. `validate-pipeline.sh` - Master Validation Script
Runs all validation tools in sequence to provide a complete picture.

**Usage:**
```bash
cd tile-service
./scripts/validate-pipeline.sh delaware
```

**With custom paths:**
```bash
KMZ_PATH=~/data/df/curvature-data/ny-vermont.kmz \
OLD_GEOJSON=../df/output/ny-vermont.geojson \
NEW_GEOJSON=./output/ny-vermont.geojson \
OLD_TILES=~/data/df/tiles/ny-vermont \
NEW_TILES=~/data/df/tiles/ny-vermont \
./scripts/validate-pipeline.sh ny-vermont
```

## Validation Workflow

### Step 1: Run OLD Pipeline
```bash
cd df
./scripts/generate-tiles.sh delaware
```

This generates:
- `df/output/delaware.geojson` (intermediate GeoJSON)
- `~/data/df/tiles/delaware/` (tiles)

### Step 2: Run NEW Pipeline
```bash
cd tile-service
./docker-generate.sh delaware --skip-upload
```

This generates:
- `tile-service/output/delaware.geojson` (intermediate GeoJSON)
- `~/data/df/tiles/delaware/` (tiles)

### Step 3: Run Validation Suite
```bash
cd tile-service
./scripts/validate-pipeline.sh delaware
```

### Step 4: Interpret Results

**Check 1: Ground Truth (KML Analysis)**
- Note the Folder count (semantic roads) vs Placemark count (segments)
- This tells you what to expect in outputs

**Check 2: GeoJSON Comparison**
- ✅ **Coordinate counts match**: No data loss in KML parsing - merging is working correctly
- ❌ **Coordinate counts differ**: Data loss during parsing - investigate `converter.go`
- ℹ️  **Feature count differs**: Expected if OLD creates 1 feature/segment, NEW creates 1 feature/road

**Check 3: Tile Analysis**
- Compare tile feature counts to GeoJSON feature counts
- Calculate tippecanoe drop rate: `(GeoJSON features - Tile features) / GeoJSON features * 100%`
- If OLD and NEW have similar drop rates, tippecanoe is working consistently
- If NEW has higher drop rate, investigate tippecanoe parameters in `tiles.go`

## Common Issues

### Issue: NEW has 2-3x fewer database entries than OLD
**Cause:** Folder-level merging (intentional design change)
- OLD: 1 Placemark → 1 feature → 1 database entry
- NEW: All Placemarks in Folder → 1 merged feature → 1 database entry

**Solution:** This is correct behavior IF coordinate counts match in GeoJSON comparison. The application gets the same data, just organized differently.

### Issue: Coordinate data loss in GeoJSON
**Cause:** Bug in `converter.go` KML parsing
**Solution:** Debug the KML→GeoJSON conversion, ensure all Placemarks are processed

### Issue: Tippecanoe dropping too many features
**Cause:** Aggressive dropping parameters or geometry issues
**Solution:**
1. Check tippecanoe parameters in `tiles.go:28-46`
2. Try adding `--no-feature-limit` or `--no-tile-size-limit`
3. Test with `--minimum-zoom=0` to see if features appear at lower zooms

### Issue: Zero-coordinate bug
**Cause:** `geometry_extractor.go:263` rejects roads with ANY coordinate == 0
**Solution:** Change OR to AND condition (only reject if ALL coordinates are zero)

## Interpreting Drop Rates

**Tippecanoe Drop Rate Guidelines:**
- 0-10%: Normal, usually low-zoom tiles where detail isn't needed
- 10-30%: Moderate, check if important roads are being dropped
- 30%+: High, investigate tippecanoe parameters

**To reduce dropping:**
```go
// In tiles.go, add these parameters:
"--drop-smallest-as-needed",  // Drop small features first
"--no-tiny-polygon-reduction", // Preserve small geometries
"--full-detail=12",            // Full detail up to zoom 12
```

## Quick Reference

```bash
# Build all tools
cd tile-service
go build -o bin/analyze-kml ./cmd/analyze-kml
go build -o bin/compare-geojson ./cmd/compare-geojson
go build -o bin/analyze-tiles ./cmd/analyze-tiles

# Run individual tools
./bin/analyze-kml ~/data/df/curvature-data/delaware.kmz
./bin/compare-geojson old.geojson new.geojson
./bin/analyze-tiles ~/data/df/tiles/delaware

# Run full validation
./scripts/validate-pipeline.sh delaware
```

## Files Created by This Suite

```
tile-service/
├── cmd/
│   ├── analyze-kml/main.go      # KML ground truth analyzer
│   ├── compare-geojson/main.go  # GeoJSON comparison tool
│   └── analyze-tiles/main.go    # Tile content analyzer
├── scripts/
│   └── validate-pipeline.sh     # Master validation script
└── VALIDATION_SUITE.md          # This file
```

## Next Steps After Validation

1. **If GeoJSON coordinate counts match**: Parsing is correct, investigate tile generation
2. **If tile counts are similar**: Both pipelines work the same, difference is in database extraction
3. **If database counts differ significantly**: Compare extraction logic in TypeScript vs Go

The validation suite isolates each stage of the pipeline so you can pinpoint exactly where issues occur.
