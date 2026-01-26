# Environment Switching Plan

## Overview

This plan enables runtime switching between local and production environments for both the tile-service (Go) and drive finder app (Next.js) without requiring service restarts.

---

## Architecture Overview

### Current State
- **Tile Service (Go)**: Connects to DB, generates tiles, uploads to R2
- **DF App (Next.js)**: Loads tiles from R2 CDN, manages jobs via tile service API, uses Prisma with Supabase

### Proposed State
- **Runtime environment switching** for tile operations without restarts
- **Admin toggle** to control which environment is active
- **Smart defaults** based on environment (local = no upload, production = upload to R2)

---

## Phase 1: Tile Service - Runtime Environment Management

### 1.1 Environment Configuration Model

Create environment configs for: `local`, `production`, (future: `test`)

```go
// New file: environment.go
type Environment struct {
    Name           string
    DatabaseURL    string
    S3Endpoint     string
    S3Bucket       string
    S3AccessKey    string
    S3SecretKey    string
    TileBaseDir    string  // local: ~/data/df/tiles, prod: temp
    SkipUpload     bool    // local: true, prod: false
    SkipGeneration bool    // Default for this env
}

type EnvironmentManager struct {
    current      *Environment
    environments map[string]*Environment
    dbPool       map[string]*sql.DB  // Connection pool per env
    mu           sync.RWMutex
}
```

### 1.2 New API Endpoints

Add to existing REST API:

```
GET  /api/environment          - Get current environment
POST /api/environment          - Switch environment (body: {"name": "local|production|test"})
GET  /api/environment/list     - List all available environments
```

### 1.3 Database Connection Management

- Maintain separate connection pools for each environment
- Switch active pool when environment changes
- Lazy initialization (don't connect until needed)
- Health check before switching

### 1.4 Smart Defaults Per Environment

| Setting | Local | Production | Test |
|---------|-------|------------|------|
| Skip Upload | true | false | true |
| Tile Directory | `~/data/df/tiles` | `/tmp` | `/tmp` |
| Database | Local Postgres | Supabase | Test DB |
| S3 Upload | No | Yes (R2) | Yes (Test bucket) |

---

## Phase 2: Drive Finder App - Environment Toggle UI

### 2.1 Environment State Management

**Client-Side** (can switch at runtime):
```typescript
// New file: src/contexts/EnvironmentContext.tsx
type Environment = 'local' | 'production' | 'test';

interface EnvironmentConfig {
  tileUrl: string;          // Where to load tiles from
  tileServiceUrl: string;   // Where tile-service API is
}

const configs: Record<Environment, EnvironmentConfig> = {
  local: {
    tileUrl: 'http://localhost:3000/tiles',  // Next.js serves local files
    tileServiceUrl: 'http://localhost:8080',
  },
  production: {
    tileUrl: 'https://pub-5642ac6487f3494c928f9adab948dbd2.r2.dev/tiles',
    tileServiceUrl: 'http://localhost:8080',  // Still local tile-service
  },
  test: {
    tileUrl: 'https://test-tiles.example.com/tiles',
    tileServiceUrl: 'http://localhost:8080',
  },
};
```

**Server-Side** (tRPC/Prisma):
- **Challenge**: Prisma client initialized at startup
- **Solution Options**:
  1. Keep using production DB, only switch tile operations
  2. Add "restart required" indicator when switching DB
  3. Use multiple Prisma clients (advanced)

**Recommended**: Option 1 - DB stays on production, tile source switches

### 2.2 Admin UI Component

Add to `/admin/tiles.tsx`:

```typescript
// New component: EnvironmentSwitcher
<Paper sx={{ p: 2, mb: 3 }}>
  <Typography variant="h6">Environment</Typography>
  <ToggleButtonGroup
    value={environment}
    exclusive
    onChange={handleEnvironmentChange}
  >
    <ToggleButton value="local">
      <ComputerIcon /> Local
    </ToggleButton>
    <ToggleButton value="production">
      <CloudIcon /> Production
    </ToggleButton>
    <ToggleButton value="test">
      <ScienceIcon /> Test
    </ToggleButton>
  </ToggleButtonGroup>

  <Typography variant="body2" sx={{ mt: 1 }}>
    Tiles: {config.tileUrl}
    Service: {config.tileServiceUrl}
  </Typography>
</Paper>
```

### 2.3 Tile URL Switching

Update `CurvatureLayer.tsx`:

```typescript
const { tileUrl } = useEnvironment();  // From context
const tileSource = `${tileUrl}/{z}/{x}/{y}.pbf`;
```

### 2.4 Persistence

Store environment preference:
1. **localStorage** (immediate, client-only)
2. **User settings DB** (persists across devices)

```typescript
// Sync to both
localStorage.setItem('environment', env);
await api.user.updateSettings.mutate({ environment: env });
```

---

## Phase 3: Local Tile Serving

When in "local" environment, serve tiles from local filesystem:

### 3.1 Next.js Static File Serving

Create symlink or configure Next.js to serve from `~/data/df/tiles`:

```javascript
// next.config.js
module.exports = {
  async rewrites() {
    return [
      {
        source: '/tiles/:region/:z/:x/:y',
        destination: process.env.LOCAL_TILES_PATH
          ? `file://${process.env.LOCAL_TILES_PATH}/:region/:z/:x/:y`
          : '/404',
      },
    ];
  },
};
```

Or use public folder symlink:
```bash
ln -s ~/data/df/tiles public/tiles
```

---

## Implementation Plan

### **Step 1: Tile Service Changes** (Priority: High)

1. Create `environment.go` with environment configs
2. Add environment manager with connection pooling
3. Add `/api/environment` endpoints
4. Update job execution to respect environment settings
5. Load environment configs from `.env` or config file

**Files to modify:**
- New: `environment.go`
- Modify: `main.go` (add environment routes)
- Modify: `job.go` (use environment settings)
- Modify: `database.go` (connection pooling)

### **Step 2: DF App Frontend** (Priority: High)

1. Create `EnvironmentContext.tsx`
2. Add environment switcher component
3. Update `CurvatureLayer.tsx` to use context
4. Update `tiles.ts` router to use context for tile service URL
5. Add UI to admin page

**Files to create:**
- `src/contexts/EnvironmentContext.tsx`
- `src/components/admin/EnvironmentSwitcher.tsx`

**Files to modify:**
- `src/components/CurvatureLayer.tsx`
- `src/server/api/routers/tiles.ts`
- `src/pages/admin/tiles.tsx`
- `src/pages/_app.tsx` (wrap with EnvironmentProvider)

### **Step 3: Local Tile Serving** (Priority: Medium)

1. Configure Next.js to serve local tiles
2. Test tile loading from local directory
3. Update documentation

**Files to modify:**
- `next.config.js`
- `.env.local`

### **Step 4: Testing & Documentation** (Priority: Medium)

1. Test switching between environments
2. Verify tile loading from both sources
3. Test tile generation in both environments
4. Document environment setup in README

---

## Configuration Files

### Tile Service `.env`
```bash
# Environment configs
ENV_LOCAL_DB_URL=postgresql://postgres:postgres@localhost:5432/drivefinder
ENV_LOCAL_S3_ENDPOINT=
ENV_LOCAL_S3_BUCKET=
ENV_LOCAL_TILE_DIR=/Users/mu/data/df/tiles
ENV_LOCAL_SKIP_UPLOAD=true

ENV_PRODUCTION_DB_URL=postgresql://...supabase.co:5432/postgres
ENV_PRODUCTION_S3_ENDPOINT=https://9f61089b6bdd0170ba01ff5296a39ca6.r2.cloudflarestorage.com
ENV_PRODUCTION_S3_BUCKET=drivefinder-tiles
ENV_PRODUCTION_TILE_DIR=/tmp/tiles
ENV_PRODUCTION_SKIP_UPLOAD=false

# Default environment
DEFAULT_ENVIRONMENT=local
```

### DF App `.env.local`
```bash
# Environment configs (for local dev)
LOCAL_TILE_URL=http://localhost:3000/tiles
LOCAL_TILE_SERVICE_URL=http://localhost:8080
LOCAL_TILES_PATH=/Users/mu/data/df/tiles

PRODUCTION_TILE_URL=https://pub-5642ac6487f3494c928f9adab948dbd2.r2.dev/tiles
PRODUCTION_TILE_SERVICE_URL=http://localhost:8080
```

---

## Trade-offs & Decisions

### Database Switching
- ❌ **Full runtime switching**: Complex with Prisma, NextAuth
- ✅ **Tile-only switching**: Simple, covers main use case
- ✅ **DB stays on production**: TileJob records are centralized

### Tile Source
- ✅ **Local**: Fast, no upload needed, direct filesystem access
- ✅ **Production**: Cloudflare R2 CDN, same as prod users see
- ✅ **Easy toggle**: Switch in admin UI

### Tile Service API
- ✅ **Always local**: Tile generation happens on dev machine
- ✅ **Environment aware**: Service knows which DB/S3 to use

---

## Questions to Resolve

1. **Database strategy**: Keep DF app on production DB, or support full local DB?
   - **Recommendation**: Production DB only, simpler and centralizes job history

2. **Local tile serving**: Next.js static serving or separate HTTP server?
   - **Recommendation**: Next.js public folder symlink, simplest

3. **Environment persistence**: localStorage only, or sync to DB?
   - **Recommendation**: Both - localStorage for instant, DB for cross-device

4. **Default environment**: What should be default for new users?
   - **Recommendation**: Production (safer), with clear UI to switch to local

---

## Current DF App Architecture (from exploration)

### Tech Stack
- **Framework**: Next.js 15.5.7 (React 18.2.0) with TypeScript
- **Backend API**: tRPC 10.43.6 (type-safe RPC)
- **Database ORM**: Prisma 5.6.0 with PostgreSQL
- **Authentication**: NextAuth.js 4.24.11 (Google OAuth)
- **Maps**: Mapbox GL JS 3.15.0, MapLibre GL 2.4.0, Deck.gl 8.9.0
- **UI Components**: Material-UI (MUI) 7.3.5
- **Styling**: Tailwind CSS 3.3.5 + Emotion

### Admin Pages
- `/admin/tiles.tsx` - Tile generation and management dashboard
- `/admin/usage.tsx` - User tier management and tile usage limits

### Key Files
- `src/components/CurvatureLayer.tsx` - Tile layer component (loads tiles)
- `src/server/api/routers/tiles.ts` - Tile job management API
- `src/env.js` - Environment variable validation
- `prisma/schema.prisma` - Database models

### Current Environment Variables
| Variable | Purpose |
|----------|---------|
| `NEXT_PUBLIC_TILE_URL` | Client-side tile source URL |
| `TILE_SERVICE_URL` | Backend tile generation API |
| `DATABASE_URL` | Prisma pooled connection |
| `DIRECT_URL` | Prisma direct connection |
| `NEXTAUTH_SECRET` | NextAuth session signing |
| `GOOGLE_CLIENT_ID` | OAuth provider |
| `NEXT_PUBLIC_MAPBOX_ACCESS_TOKEN` | Mapbox GL JS token |

---

## Next Steps

When ready to implement:
1. Review and approve plan
2. Choose implementation order (tile-service first or DF app first)
3. Create detailed implementation tasks
4. Begin development
