# Action: Regenerate Regions Missing Road IDs

## Problem
Three regions have tiles generated before the `id` property was added. Roads in these regions cannot be favorited because they lack the `id` property needed to identify them in the database.

## Affected Regions

| Region | Generated | Issue |
|--------|-----------|-------|
| oregon | Nov 28, 2025 | Missing: id, length, startLat, startLng, endLat, endLng |
| alaska | Dec 10, 2025 | Missing: id, length, startLat, startLng, endLat, endLng |
| asia-japan | Nov 28, 2025 | Missing: id, length, startLat, startLng, endLat, endLng |

## Steps to Fix

### 1. Regenerate the three regions
```bash
cd ~/src/t3/tile-service
./tile-service generate oregon alaska asia-japan --workers 3 --skip-upload
```

### 2. Merge all regions
```bash
./tile-service merge --all
```

### 3. Upload merged tiles to R2
```bash
./tile-service upload public/tiles/merged --workers 100
```

## Verification
After regeneration, verify tiles have the required properties:
```bash
./cmd/analyze-tiles/analyze-tiles --tile public/tiles/oregon/14/2872/6018.pbf
```

Expected output should include:
- `id: <uuid>`
- `length: <number>`
- `startLat: <number>`
- `startLng: <number>`
- `endLat: <number>`
- `endLng: <number>`

## Notes
- Current upload (started Jan 25) should complete first (~3.9M tiles)
- Regeneration will only upload the 3 affected regions' tiles
- The merge step combines all regional tiles so overlapping areas show all roads
