# Running the Overture Buildings Job Locally + Uploading to R2

**Owner ruling (2026-09-10): generation runs on the owner's machine, never in the cloud.
R2 storage is cheap and fine — R2 compute/generation is not. This job has no scheduled
or hosted path. It is a manual, local, one-off run.**

This is the runbook for the `generate-buildings` job: download an Overture buildings
extract for a bounding box, build a single PMTiles archive with Tippecanoe, upload it to
R2. It uses the same local Docker Compose rig as regular tile generation
(`docker-compose.yml`, `docker-start.sh`) — nothing new to install beyond what that rig
already needs.

## 1. Start the local rig

```bash
cd tile-service
./docker-start.sh
```

This builds the image (now including the `overturemaps` CLI — see Dockerfile) and starts
`tile-service` + its own `postgres` container. The compose Postgres (`drivefinder-postgres`,
port 5432, database `postgres`) is **not** the production database — the `generate-buildings`
job doesn't touch Postgres at all, but the rig-isolation rule still applies to everything
else running in this compose file. Never point `DB_HOST` in `tile-service/.env` at a
production or shared host.

## 2. Smoke test with a small bbox (no credentials needed)

Run this before touching R2 credentials at all — it downloads a real (tiny) Overture
extract and proves the pipeline end to end using `--skip-upload`:

```bash
docker-compose exec tile-service /app/tile-service generate-buildings \
  -bbox=-122.4215,37.7749,-122.4150,37.7800 \
  -skip-upload \
  smoketest
```

Expected: a `buildings-smoketest.pmtiles` file appears under `~/data/df/tiles/` (the
compose volume mount), a few hundred KB, containing real San Francisco building
footprints. Verify it decodes:

```bash
docker-compose exec tile-service tippecanoe-decode /app/tiles/buildings-smoketest.pmtiles | head
```

If this doesn't produce valid GeoJSON output, stop — do not proceed to a larger run.

## 3. R2 credentials (owner step — STOP HERE until you have them)

Everything above needs zero credentials. This step does. Create/verify an R2 bucket and
an API token in the Cloudflare dashboard, then fill in `tile-service/.env`:

```bash
# R2 endpoint uses your Cloudflare account ID, NOT a region-based S3 host
S3_ENDPOINT=https://<your-account-id>.r2.cloudflarestorage.com
S3_ACCESS_KEY_ID=<r2-access-key-id>
S3_SECRET_ACCESS_KEY=<r2-secret-access-key>
S3_REGION=auto
S3_BUCKET=drivefinder-tiles
S3_BUCKET_PATH=tiles
```

Notes:
- `S3_REGION=auto` is required for R2 — it ignores the value but the S3 client needs
  *something* set, and Wasabi-style region strings (e.g. `us-west-1`) will not work
  against the R2 endpoint.
- These credentials are never committed. `.env` is already gitignored in this repo;
  double-check before running anything if you're unsure.
- The uploaded object lands at `s3://<bucket>/buildings/<region>.pmtiles` (the job always
  prefixes with `buildings/`, distinct from the `tiles/` prefix regular road tiles use).

Once `.env` has real credentials, re-run the smoke test **without** `-skip-upload` to
confirm the upload path works before committing to a bigger extract:

```bash
docker-compose exec tile-service /app/tile-service generate-buildings \
  -bbox=-122.4215,37.7749,-122.4150,37.7800 \
  smoketest
```

## 4. The real run (owner-gated, west-coast / target region)

```bash
docker-compose exec tile-service /app/tile-service generate-buildings \
  -bbox=<minLon>,<minLat>,<maxLon>,<maxLat> \
  -min-zoom=0 -max-zoom=14 \
  <region-name>
```

Pick a bbox that matches the actual coverage you need (e.g. west coast US) rather than a
whole-country or global extract — see the disk/time estimates below before choosing scope.

### Before a big run: raise the temp directory ceiling

`docker-compose.yml` mounts `/tmp` as a 2 GB RAM-backed `tmpfs` for fast Tippecanoe scratch
space on the regular (roads) tile job. That's too small for anything beyond a smoke test
here — a city- or state-sized buildings extract's scratch data (raw GeoJSON download +
Tippecanoe's on-disk merge files) will not fit in 2 GB. Before a real run, either:

- Bump the `tmpfs` size in `docker-compose.yml` (`/tmp:size=<N>G`) to comfortably exceed
  the estimated GeoJSON size below, or
- Point `TEMP_DIR` at a bind-mounted host directory with real disk headroom instead of
  the RAM-backed tmpfs (add a volume mount and set `TEMP_DIR=/app/scratch` in the
  container environment).

The job always creates a per-run subdirectory under `TEMP_DIR` and removes it on
completion (`os.RemoveAll` in a `defer`, unless `-no-cleanup` is passed for debugging) —
so scratch space is bounded to one run's worth at a time, never accumulates across runs.

## 5. Disk, time, and cost expectations

These are estimates to size the real run, not the smoke test numbers above (a ~0.5 km²
smoke-test bbox produced a ~220 KB PMTiles in under 2 seconds — not representative of a
region-sized run).

| Scope | Overture GeoJSON download (raw) | Tippecanoe wall time (laptop, single core) | Final PMTiles |
|---|---|---|---|
| One city (e.g. SF proper, ~120 km²) | ~150–400 MB | ~1–3 min | ~30–80 MB |
| One metro area (e.g. SF Bay Area, ~18,000 km²) | ~3–8 GB | ~20–60 min | ~0.5–1.5 GB |
| US West Coast (CA+OR+WA coastal band) | ~15–40 GB | several hours | ~3–8 GB |

These ranges come from Overture's published building-density averages (dense urban areas
run higher; rural/coastal stretches run much lower) — **treat them as planning numbers,
not guarantees.** Run the smoke test, then a one-city-sized bbox, and check the actual
GeoJSON size before committing disk/time to a larger extract; extrapolate from that
measurement rather than the table above once you have a real data point for your target
region's building density.

**R2 storage cost is the cheap part.** R2 pricing is $0.015/GB-month storage, **zero**
egress fees (that's the whole reason to use R2 over S3 here). For a 5 GB PMTiles file:

```
5 GB × $0.015/GB-month = $0.075/month
```

Even the largest plausible single-region PMTiles (say 10 GB) is $0.15/month. This is a
rounding error next to the Mapbox/tile-serving costs this basemap migration is meant to
eliminate — the constraint on this job is owner laptop time and disk, not R2 spend.

## 6. Cleanup checklist after a run

- Scratch dir under `TEMP_DIR`: removed automatically unless you passed `-no-cleanup`.
  If you did pass it (for debugging a failed run), delete it manually once done.
- The raw GeoJSON download only ever exists inside the per-run scratch dir — it is never
  copied to `OUTPUT_DIR` or uploaded. Only the final `.pmtiles` file persists.
- Local `.pmtiles` file in `OUTPUT_DIR` (`~/data/df/tiles/` by default): keep it if you
  want a local copy for testing against the frontend, otherwise it's safe to delete once
  it's confirmed uploaded to R2 (`s3://<bucket>/buildings/<region>.pmtiles`).
