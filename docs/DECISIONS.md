# Architectural Decisions

This document captures significant architectural decisions, the context behind them, and the reasoning for future reference.

## Format

Each decision follows this structure:
- **Date**: When the decision was made
- **Context**: What situation prompted this decision
- **Decision**: What we decided to do
- **Reasoning**: Why we chose this approach over alternatives
- **Consequences**: What this means going forward
- **References**: Links to related commits, changelog entries, and affected files

---

## Decisions

### 2024-XX-XX: [Template - Remove This]

**Context**: Describe the situation or problem that required a decision.

**Decision**: State clearly what was decided.

**Reasoning**: Explain why this approach was chosen. What alternatives were considered? What trade-offs were made?

**Consequences**: What are the implications? What does this enable or constrain going forward?

**References**:
- Commits: `abc1234`, `def5678`
- Changelog: See CHANGELOG.md entry for YYYY-MM-DD
- Files changed: `cmd/server/main.go`, `internal/tiles/generator.go`
- Related decisions: [Decision Name](#decision-name) (if applicable)

---

### 2026-09-09: Order tippecanoe drops by curvature to fix flaky zoom-level rendering

**Context**: Owner screenshots (PNW, ~z6-8) showed the green curvy-roads overlay
popping/chopping between adjacent zooms — different road fragments appearing and
disappearing as the map zoomed in and out, instead of a stable set of roads that
simply gains detail. `tiles.go` used `--drop-densest-as-needed` (spatial density
based dropping) together with `--preserve-input-order`, which made the surviving
subset of roads effectively arbitrary — whatever order they happened to be
extracted in — and independently recomputed at every zoom level. Adjacent zooms
therefore kept different subsets of roads.

**Decision**: Replace `--preserve-input-order` with `--order-descending-by=curvature`.
Tippecanoe uses this ordering both to sort features and to decide what
`--drop-densest-as-needed` drops first, so the lowest-curvature roads are dropped
first and uniformly at every zoom — the highest-curvature (most interesting) roads
become zoom-stable. Also changed the `curvature` GeoJSON property from a string to
a number in `kmlconv.go`, since tippecanoe's ordering does numeric comparison only
for numeric attributes; a string property sorts lexicographically ("150" > "9000"),
which would have silently defeated the ordering fix. `--drop-densest-as-needed` and
`--extend-zooms-if-still-dropping` were kept — dropping is still needed to keep
tile sizes bounded, only the drop *order* changed.

**Reasoning**: This is tippecanoe's documented mechanism for exactly this problem
— consistent feature retention across zoom levels — and requires no new drop
strategy, no simplification-based alternative, and no schema change. Verified
locally against a real tippecanoe v2.79.0 binary: a single-tile lineage (same
6/10/22 → 7/20/44 → 8/41/89 tiles) fed the same synthetic clustered data with old
vs. new flags. Of the top-20 highest-curvature roads, the old flags kept 18/20 at
z6, dropping to 13/20 by z8 (losing 5 of the most important roads by the time you
zoom in). The new flags kept 18/20 at z6 and 15/20 at z8 — fewer top roads lost,
and a *consistent* (importance-ranked) set doing the dropping rather than an
arbitrary one. The client-side style (`df/src/components/CurvatureLayer.tsx`) was
also checked — its zoom-dependent paint properties already use `interpolate`
expressions with no hard zoom-band steps, so styling was not contributing to the
flakiness; generation-side per-zoom feature-set instability was the dominant
factor.

**Consequences**: Changed flags only take effect the next time a region is
regenerated — this merge does not retroactively fix already-generated tiles on
Railway. Regenerating live regions (`master` branch, Railway tile-service) is an
owner-gated step, not part of this change. The `curvature` MVT tile property is
now a number instead of a string; client code already calls `.toString()` on it
(`CurvatureInteractionHandler.tsx`), so this is not a breaking change for the map
overlay. The DB-facing `RoadGeometry.Curvature` extraction path
(`geometry_extractor.go`) already handled a numeric `curvature` property via its
`float64` branch (formatting to `%.2f`), so no code change was needed there —
stored values go from e.g. `"1000"` to `"1000.00"`.

**References**:
- Files changed: `tiles.go`, `kmlconv/kmlconv.go`
- Changelog: See CHANGELOG.md entry for 2026-09-09

---

### 2026-09-10: Raise Tippecanoe's per-tile byte budget to stop byte-size-driven drops of top-curvature roads at z6-8

**Context**: The 2026-09-09 fix (order drops by curvature) made the *set* of roads
dropped at each zoom consistent and importance-ranked, but did not stop drops from
happening: 15/20 of the top-20 highest-curvature roads survived at z8, a real
improvement over 13/20 but still short of full zoom stability. Morning-review
decision 11 asked for the same feature-overlap metric to reach ≥18/20 at z8, via
one of: `--drop-rate` tuning, per-zoom `--minimum-detail`, or raising the tile-size
limit for the roads layer.

**Decision**: Add `--maximum-tile-bytes=1500000` to the `tiles.go` Tippecanoe
invocation (up from Tippecanoe's 500KB default). `--drop-rate` and
`--minimum-detail` were tried first and ruled out — both are inert once
`--drop-densest-as-needed` is driving the drop decision, because that flag drops
purely to fit the tile's byte budget, not based on a target detail level or a
per-zoom retention rate. The byte budget was the actual constraint causing drops,
so it's the lever that has to move.

**Reasoning**: Verified locally against a real tippecanoe v2.79.0 binary. Built a
denser synthetic stress rig than the 2026-09-09 proof (6,000 clustered LineStrings,
15-40 vertices each, same z6/7/8 tile lineage pattern: 6/10/22 → 7/20/45 → 8/41/91)
specifically so the default 500KB budget would still be triggering real drops even
with curvature-ordering in place (baseline reproduced 6/20 top-curvature roads
surviving at z8 under this harsher load — worse than the 15/20 seen in production
regions, because the synthetic data is deliberately denser to make the byte-budget
effect visible). Results holding minimum/maximum-zoom, ordering, and
drop-densest-as-needed constant, varying only the extra flag:

| Variant | z8 retention (top-20) | Total tile-dir size |
|---|---|---|
| baseline (500KB default) | 6/20 | 9.1M |
| `--drop-rate=1` | 6/20 (no change) | 9.1M |
| `--minimum-detail=4` or `=6` | 6/20 (no change) | 9.1M |
| `--maximum-tile-bytes=600000` | 8/20 | 6.2M |
| `--maximum-tile-bytes=750000` | 12/20 | 7.1M |
| `--maximum-tile-bytes=1000000` | 18/20 | 6.1M |
| `--maximum-tile-bytes=1200000`+ | 20/20 (no drops needed) | 3.1M |

Shipped at 1.5MB — comfortably past the 1.2MB point where this rig stopped
dropping at all, giving margin for real regions denser than the synthetic case
without sitting exactly on the threshold.

**Consequences**: Low-zoom (z6-8) roads tiles for dense curvy-road regions can now
be up to 3x larger than before (1.5MB vs 500KB) before Tippecanoe drops anything.
At z6-8 a viewport typically holds a handful of tiles, so worst case is a few MB of
one-time overlay load, not a per-tile-zoom-level cost multiplied across many tiles
the way it would be at z14-16. This only affects the roads layer's own Tippecanoe
invocation (`tiles.go`); the Overture buildings job already uses distinct
completeness flags with no dropping and is unaffected. As with the 2026-09-09 fix,
this only takes effect the next time a region is regenerated — regenerating live
Railway regions remains an owner-gated, prod-first step, not part of this change.

**References**:
- Files changed: `tiles.go`
- Changelog: See CHANGELOG.md entry for 2026-09-10
- Related decisions: [Order tippecanoe drops by curvature to fix flaky zoom-level rendering](#2026-09-09-order-tippecanoe-drops-by-curvature-to-fix-flaky-zoom-level-rendering)

---

<!-- Add new decisions above this line, newest first -->
