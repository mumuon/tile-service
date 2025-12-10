# Tile Service Session Summary

## Session Objective
Build a performant Go-based tile generation service to replace Node.js scripts, supporting multi-job parallelism with Tippecanoe integration.

## Completed Work

### 1. Core Architecture
- **Language**: Go (performant, compiled, single binary)
- **CLI**: Subcommand-based (separate `generate` and `upload` commands)
- **Database**: Supabase (PostgreSQL) for progress tracking
- **Storage**: Cloudflare R2 for tile storage

### 2. Key Features Implemented

#### Generate Command
```bash
./tile-service generate [options] <region>
  -max-zoom int     Maximum zoom level (default 16)
  -min-zoom int     Minimum zoom level (default 5)
  -skip-upload      Skip R2 upload, keep tiles locally
  -no-cleanup       Don't cleanup temporary files
```

**Pipeline**:
1. Extract KMZ from curvature-data directory (handles both `us-{region}` and direct region names)
2. Convert KML to GeoJSON (handles nested folder structures)
3. Generate tiles with Tippecanoe (configurable zoom levels)
4. Upload to R2 (optional)
5. Cleanup temporary files (optional)

#### Upload Command
```bash
./tile-service upload [options] <tiles_directory>
  -min-zoom int     Minimum zoom level to upload (-1 = all)
  -max-zoom int     Maximum zoom level to upload (-1 = all)
```

**Features**:
- Filters locally before uploading (no wasted bandwidth)
- Selective zoom level uploads (e.g., only upload zoom 5-10)
- Region name auto-detected from directory path

### 3. Test Results

| Region | Features | Tiles | Size | Status |
|--------|----------|-------|------|--------|
| Oregon | 214,180 | 113,358 | 51.2 MB | ✅ Generated (zoom 5-16) |
| Washington | - | - | 465 MB | ✅ Generated (zoom 5-16), partial upload (zoom 5-10: 13.4 MB) |
| California | 882,965 | 228,322 | 957 MB | ✅ Generated (zoom 5-16), uploading to R2 |
| Asia-Japan | - | - | - | ⏳ In progress (generate only, no upload) |

### 4. Configuration

**.env file** (Supabase credentials):
```env
DB_HOST=aws-1-us-west-1.pooler.supabase.com
DB_PORT=6543
DB_USER=postgres.ieglgottfhukhgtpuwwb
DB_PASSWORD=IkxoSMgRdY8hX6nW
DB_NAME=postgres
DB_SSLMODE=require
```

**.gitignore** created to protect secrets:
- .env
- tile-service (binary)
- public/tiles/ (generated files)
- *.log

### 5. KML Parser Enhancements
- **Nested folder support**: Handles both Document>Folder>Placemark and Document>Folder>Folder>Placemark structures
- **Explicit XML tags**: Uses lowercase tag names matching KML spec
- **Multi-region support**: Auto-detects region naming patterns (us-*, asia-*, canada-*, etc.)

### 6. Codebase Structure

Core Files:
- `main.go` - Subcommand routing and CLI interface
- `service.go` - Tile generation pipeline orchestration
- `s3.go` - R2 upload handling
- `converter.go` - KML to GeoJSON conversion
- `extractor.go` - KMZ extraction and cleanup
- `database.go` - Supabase connection and progress tracking
- `config.go` - Configuration loading from .env
- `models.go` - Data structures
- `tiles.go` - Tippecanoe integration

### 7. Git Repository
- Initialized local git repo at `/Users/mu/src/t3/tile-service`
- Initial commit with all 17 source files
- .gitignore protecting sensitive files

## Next Session TODO

Based on the user's request, the next task is:
- **Build a nice slick landing page** for the tile service

This would likely be:
1. A web UI to trigger tile generation/upload
2. Progress tracking dashboard (tied to Supabase)
3. Region selection interface
4. Zoom level configuration
5. Upload history/status

## Important Notes

1. **Database Connection**: Successfully tested with Supabase (not local PostgreSQL)
2. **Upload Performance**: 957 MB California tiles uploading to R2 (slow but working)
3. **Region Support**: Flexible - handles US states, international regions, etc.
4. **Zoom Filtering**: Works perfectly for selective uploads
5. **Temporary Files**: Properly cleaned up after generation

## Running the Service

```bash
# Generate tiles for a region (with full upload)
./tile-service generate california

# Generate without uploading
./tile-service generate -skip-upload oregon

# Generate with custom zoom levels
./tile-service generate -max-zoom 8 -min-zoom 5 washington

# Upload pre-generated tiles
./tile-service upload public/tiles/california

# Upload specific zoom levels
./tile-service upload -min-zoom 5 -max-zoom 10 public/tiles/washington

# Show help
./tile-service -help
```

## Performance Notes
- Tippecanoe is CPU intensive but very fast (45-55 seconds for full region)
- R2 upload bandwidth is limiting factor (957 MB takes 30-60+ minutes)
- Memory usage: ~2GB during tile generation, minimal during upload
- No database slowdown (optional connection)

---
**Session completed**: Ready for new session on landing page development
