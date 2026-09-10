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

<!-- Add new decisions above this line, newest first -->
