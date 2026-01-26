# Two-Phase Workflow Guide

## Overview

The tile service now supports a **two-phase workflow** for road geometry extraction:
1. **Extract Phase**: Generate tiles and extract geometries to intermediate file
2. **Insert Phase**: Review and insert geometries from file into database

This allows you to review extracted data before committing to the database.

## Use Cases

### Standard Workflow (One Command)
For most cases where you want immediate database insertion:

```bash
./tile-service generate -skip-upload florida
```

This does everything: tiles → geometries → database in one go.

---

### Two-Phase Workflow (Extract Then Insert)
For careful state regeneration where you want to review before inserting:

#### Phase 1: Generate & Extract (No Database Insertion)
```bash
./tile-service generate -skip-upload -skip-geometry-insertion florida
```

**What happens:**
- ✅ Extracts KMZ
- ✅ Converts KML → GeoJSON
- ✅ Generates tiles (saved to `./public/tiles/florida/`)
- ✅ Extracts road geometries to `.extracted-roads-florida.json`
- ❌ Does NOT insert to database
- ❌ Does NOT upload to R2
- ❌ Does NOT cleanup extraction file

**Results:**
- Tiles: `./public/tiles/florida/`
- Geometries: `.extracted-roads-florida.json` (intermediate file)

#### Review the Extraction File

```bash
# Check how many roads were extracted
jq 'length' .extracted-roads-florida.json

# View first few roads
jq '.[0:3]' .extracted-roads-florida.json

# Check specific road
jq '.[] | select(.roadId == "US Route 1")' .extracted-roads-florida.json

# Validate bounds
jq '.[] | select(.minLat > .maxLat or .minLng > .maxLng)' .extracted-roads-florida.json
```

Expected format:
```json
[
  {
    "roadId": "US Route 1",
    "region": "florida",
    "minLat": 25.7617,
    "maxLat": 30.3322,
    "minLng": -81.3792,
    "maxLng": -80.0560,
    "curvature": "1250"
  }
]
```

#### Phase 2: Insert to Database

Once you've reviewed and are happy with the extraction:

```bash
# Insert using region name
./tile-service insert-geometries florida

# Or using file path
./tile-service insert-geometries .extracted-roads-florida.json
```

**What happens:**
- Loads roads from `.extracted-roads-florida.json`
- Batch inserts to `RoadGeometry` table (50 per transaction)
- Cleans up extraction file after success

---

## Multi-State Regeneration Script

For regenerating multiple states with two-phase workflow:

```bash
#!/bin/bash
# regen-states.sh

STATES="florida georgia alabama mississippi louisiana"

echo "=== PHASE 1: Extract All States ==="
for state in $STATES; do
    echo "Extracting $state..."
    ./tile-service generate -skip-upload -skip-geometry-insertion $state

    if [ $? -eq 0 ]; then
        COUNT=$(jq 'length' .extracted-roads-$state.json)
        echo "✓ $state: $COUNT roads extracted"
    else
        echo "✗ $state: FAILED"
    fi
    echo ""
done

echo ""
echo "=== Review extraction files before continuing ==="
echo "Files created:"
ls -lh .extracted-roads-*.json
echo ""
read -p "Continue with database insertion? (y/n) " -n 1 -r
echo ""

if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "=== PHASE 2: Insert All States ==="
    for state in $STATES; do
        echo "Inserting $state..."
        ./tile-service insert-geometries $state

        if [ $? -eq 0 ]; then
            echo "✓ $state: Inserted to database"
        else
            echo "✗ $state: FAILED"
        fi
        echo ""
    done

    echo "=== Verify in database ==="
    psql -h $DB_HOST -U $DB_USER -d $DB_NAME \
      -c "SELECT region, COUNT(*) FROM \"RoadGeometry\" WHERE region IN ('florida', 'georgia', 'alabama', 'mississippi', 'louisiana') GROUP BY region ORDER BY region;"
else
    echo "Cancelled. Extraction files preserved for review."
fi
```

---

## Verification & Troubleshooting

### Check Extraction File Exists

```bash
ls -lh .extracted-roads-florida.json
```

### Validate JSON Format

```bash
jq empty .extracted-roads-florida.json && echo "Valid JSON" || echo "Invalid JSON"
```

### Compare Before/After in Database

```bash
# Before insertion
psql ... -c "SELECT COUNT(*) FROM \"RoadGeometry\" WHERE region = 'florida';"

# Insert
./tile-service insert-geometries florida

# After insertion
psql ... -c "SELECT COUNT(*) FROM \"RoadGeometry\" WHERE region = 'florida';"
```

### Re-insert if Needed

If insertion fails or you need to retry:

```bash
# The extraction file is preserved on failure
./tile-service insert-geometries florida

# Force re-extraction if needed
./tile-service extract public/tiles/florida
./tile-service insert-geometries florida
```

---

## Command Reference

### Generate with Immediate Insertion (Default)
```bash
./tile-service generate -skip-upload florida
```
- Extracts geometries
- Inserts to database immediately
- Cleans up extraction file

### Generate without Insertion (Two-Phase)
```bash
./tile-service generate -skip-upload -skip-geometry-insertion florida
```
- Extracts geometries to `.extracted-roads-florida.json`
- Does NOT insert to database
- Preserves extraction file

### Insert from File
```bash
# By region name
./tile-service insert-geometries florida

# By file path
./tile-service insert-geometries .extracted-roads-florida.json

# With debug logging
./tile-service -debug insert-geometries florida
```

### Extract from Existing Tiles (Standalone)
```bash
# Extract and insert immediately
./tile-service extract public/tiles/florida

# Just extraction (file only)
# Not directly supported - use generate -skip-geometry-insertion instead
```

---

## Workflow Comparison

| Feature | Standard | Two-Phase |
|---------|----------|-----------|
| **Command** | `generate -skip-upload` | `generate -skip-upload -skip-geometry-insertion` + `insert-geometries` |
| **Steps** | 1 command | 2 commands |
| **Review** | No | Yes (JSON file) |
| **Rollback** | Delete from DB | Don't run phase 2 |
| **Safety** | Lower | Higher |
| **Speed** | Faster | Slightly slower |
| **Use Case** | Quick regen | Careful state regen |

---

## Best Practices

1. **Use Two-Phase for Production States**
   - Critical states with lots of roads
   - When changing extraction logic
   - For major version updates

2. **Use Standard for Testing**
   - Development regions
   - Small test datasets
   - Quick iterations

3. **Batch Processing**
   - Extract all states first
   - Review all extraction files
   - Insert in batch if satisfied

4. **Backup Strategy**
   ```bash
   # Before regeneration
   psql ... -c "COPY \"RoadGeometry\" TO '/tmp/roadgeometry-backup.csv' CSV HEADER;"

   # Or just save extraction files
   mkdir -p backups/$(date +%Y%m%d)
   cp .extracted-roads-*.json backups/$(date +%Y%m%d)/
   ```

5. **Validation Checks**
   ```bash
   # Check for invalid bounds
   jq '.[] | select(.minLat > .maxLat)' .extracted-roads-florida.json

   # Check for missing required fields
   jq '.[] | select(.roadId == null or .region == null)' .extracted-roads-florida.json

   # Check curvature values
   jq '.[] | select(.curvature != null) | .curvature' .extracted-roads-florida.json | sort -u
   ```

---

## Example: Florida Full Regeneration

```bash
# 1. Generate tiles and extract geometries (no DB insertion, no upload)
./tile-service generate -skip-upload -skip-geometry-insertion florida

# 2. Review extraction file
echo "Roads extracted: $(jq 'length' .extracted-roads-florida.json)"
jq '.[0:5]' .extracted-roads-florida.json

# 3. Check database current state
psql ... -c "SELECT COUNT(*) FROM \"RoadGeometry\" WHERE region = 'florida';"

# 4. Insert to database
./tile-service insert-geometries florida

# 5. Verify insertion
psql ... -c "SELECT COUNT(*) FROM \"RoadGeometry\" WHERE region = 'florida';"

# 6. Test nearby roads API
curl "http://localhost:3000/api/trpc/road.getNearbyRoads?input={...}"

# 7. Later: Upload tiles to R2
./tile-service upload public/tiles/florida
```

---

## Cleanup

```bash
# Remove extraction files manually (if not auto-cleaned)
rm .extracted-roads-*.json
rm .extract-progress-*.json

# Remove tiles if needed
rm -rf public/tiles/florida
```

---

## Troubleshooting

**Problem: "extraction file not found"**
```bash
# Check if it exists
ls -la .extracted-roads-florida.json

# Regenerate if needed
./tile-service generate -skip-upload -skip-geometry-insertion florida
```

**Problem: "failed to insert road geometries"**
```bash
# Check database connection
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c "\conninfo"

# Check table exists
psql ... -c "\d \"RoadGeometry\""

# Try with debug logging
./tile-service -debug insert-geometries florida
```

**Problem: Want to re-insert after changes**
```bash
# Delete existing region data
psql ... -c "DELETE FROM \"RoadGeometry\" WHERE region = 'florida';"

# Re-insert
./tile-service insert-geometries florida
```

---

## Summary

✅ **Use this command for Florida full regen:**
```bash
./tile-service generate -skip-upload -skip-geometry-insertion florida
# Review .extracted-roads-florida.json
./tile-service insert-geometries florida
```

This gives you full control with intermediate file inspection!
