# Database Schema Synchronization

This document tracks the synchronization between the Prisma schema and Go structs to ensure consistency.

## TileJob Model

### Prisma Schema (df/prisma/schema.prisma)
```prisma
model TileJob {
    id              String   @id @default(cuid())
    region          String
    status          String   @default("pending")

    // Job Options
    maxZoom                Int     @default(16)
    minZoom                Int     @default(5)
    skipUpload             Boolean @default(false)
    noCleanup              Boolean @default(false)
    extractGeometry        Boolean @default(true)
    skipGeometryInsertion  Boolean @default(false)

    // Generation metrics
    roadsExtracted  Int?
    tilesGenerated  Int?
    totalSizeBytes  BigInt?

    // Timing
    startedAt       DateTime?
    completedAt     DateTime?

    // Progress tracking
    currentStep     String?
    uploadProgress  Int      @default(0)
    uploadedBytes   BigInt   @default(0)

    // Error handling
    errorMessage    String?
    errorLog        String?

    createdAt       DateTime @default(now())
    updatedAt       DateTime @updatedAt
}
```

### Go Struct (tile-service/models.go)
```go
type TileJob struct {
    ID                    string
    Region                string
    Status                string
    MaxZoom               int
    MinZoom               int
    SkipUpload            bool
    NoCleanup             bool
    ExtractGeometry       bool
    SkipGeometryInsertion bool
    CurrentStep           *string      // Nullable
    RoadsExtracted        *int         // Nullable
    TilesGenerated        *int         // Nullable
    TotalSizeBytes        *int64       // Nullable
    UploadProgress        int
    UploadedBytes         int64
    ErrorMessage          *string      // Nullable
    ErrorLog              *string      // Nullable
    CreatedAt             time.Time
    UpdatedAt             time.Time
    StartedAt             *time.Time   // Nullable
    CompletedAt           *time.Time   // Nullable
}
```

## SQL Column Naming Convention

**Important**: PostgreSQL queries must use camelCase column names with double quotes to match Prisma's default naming.

❌ Wrong:
```sql
SELECT id, region, roads_extracted, created_at FROM "TileJob"
```

✅ Correct:
```sql
SELECT id, region, "roadsExtracted", "createdAt" FROM "TileJob"
```

### Column Name Mapping

| Prisma Field | SQL Column Name | Go Struct Field | Type |
|--------------|----------------|-----------------|------|
| id | id | ID | string |
| region | region | Region | string |
| status | status | Status | string |
| maxZoom | "maxZoom" | MaxZoom | int |
| minZoom | "minZoom" | MinZoom | int |
| skipUpload | "skipUpload" | SkipUpload | bool |
| noCleanup | "noCleanup" | NoCleanup | bool |
| extractGeometry | "extractGeometry" | ExtractGeometry | bool |
| skipGeometryInsertion | "skipGeometryInsertion" | SkipGeometryInsertion | bool |
| currentStep | "currentStep" | CurrentStep | *string |
| roadsExtracted | "roadsExtracted" | RoadsExtracted | *int |
| tilesGenerated | "tilesGenerated" | TilesGenerated | *int |
| totalSizeBytes | "totalSizeBytes" | TotalSizeBytes | *int64 |
| uploadProgress | "uploadProgress" | UploadProgress | int |
| uploadedBytes | "uploadedBytes" | UploadedBytes | int64 |
| errorMessage | "errorMessage" | ErrorMessage | *string |
| errorLog | "errorLog" | ErrorLog | *string |
| createdAt | "createdAt" | CreatedAt | time.Time |
| updatedAt | "updatedAt" | UpdatedAt | time.Time |
| startedAt | "startedAt" | StartedAt | *time.Time |
| completedAt | "completedAt" | CompletedAt | *time.Time |

## Synchronization Workflow

When making schema changes:

1. **Update Prisma schema** (`df/prisma/schema.prisma`)
   ```bash
   cd df
   npx prisma db push
   ```

2. **Update Go structs** (`tile-service/models.go`)
   - Match field names (Prisma camelCase → Go PascalCase)
   - Use pointers for nullable fields (Prisma `Type?` → Go `*Type`)

3. **Update SQL queries** (`tile-service/database.go`, `tile-service/api.go`)
   - Use camelCase with double quotes for all non-simple columns
   - Simple columns (id, region, status) don't need quotes

4. **Rebuild and test**
   ```bash
   cd tile-service
   go build -o tile-service .
   ./tile-service serve -port 8080
   ```

## Common Issues

### Column Does Not Exist
If you see errors like `column "started_at" does not exist`:
- You're using snake_case instead of camelCase
- Add double quotes around the column name: `"startedAt"`

### Type Mismatch
If you see nil pointer errors:
- Check if the field is nullable in Prisma (`Type?`)
- Use pointer types in Go (`*Type`) for nullable fields

### Default Values
- Prisma defaults are only applied on INSERT
- Go code should handle missing values appropriately
- Use `COALESCE()` in SQL when needed
