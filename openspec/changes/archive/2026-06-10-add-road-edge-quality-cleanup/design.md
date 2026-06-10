# Design — Road graph quality cleanup

## Non-destructive by construction

Cleanup never deletes rows. It flips `is_routable = false` and records `excluded_reason` +
`quality_checked_at`. This keeps every imported edge inspectable, lets us re-tune thresholds and
re-run, and makes the router's input a simple filter (`routable_road_edges`) rather than a lossy
mutation. A `down` migration fully reverses the layer.

## Schema additions (migration 000007)

```sql
ALTER TABLE road_edges
ADD COLUMN IF NOT EXISTS is_routable        BOOLEAN NOT NULL DEFAULT true,
ADD COLUMN IF NOT EXISTS excluded_reason    TEXT,
ADD COLUMN IF NOT EXISTS quality_checked_at TIMESTAMPTZ;
```

- `is_routable` defaults `true` so existing/newly-imported edges are routable until a cleanup pass
  proves otherwise.
- Indexes: `(is_routable)`, `(excluded_reason)`, and a composite `(is_walkable, is_routable)` that
  matches the router's filter and the `routable_road_edges` view predicate.
- `routable_road_edges` view = `WHERE is_walkable AND is_routable`. The router queries the view, never
  raw `road_edges`, so quality filtering is centralized and swappable.

The migration owns the **schema** (columns/indexes/view). The **data classification** lives in
`scripts/osm/cleanup_road_graph.sql`, run on demand after an import — schema and data passes are kept
separate so re-tuning thresholds doesn't require a migration.

## Exclusion rules (cleanup script)

Idempotent: it first **resets** all edges to routable, then re-applies the four rules, so re-running
always yields the same state regardless of prior runs. Each rule only touches rows still
`is_routable = true`, so the **first** matching reason wins (deterministic labelling):

1. `invalid_geometry` — `geom IS NULL OR NOT ST_IsValid(geom)`.
2. `zero_or_negative_length` — `length_meters <= 0`.
3. `self_loop` — `from_node_id = to_node_id`.
4. `suspicious_long_edge_over_5000m` — `length_meters > 5000`.

Rationale for the 5 km rule: a walking *route* can exceed 5 km, but a single indivisible *edge* over
5 km has no intermediate intersections for the router to branch at and yields coarse risk-scoring
granularity, so it is almost certainly an OSM artifact (confirmed: the longest edges are 30+ km with
3 vertices). Threshold lives only in the cleanup script, easy to tune.

The script runs in a single transaction and recreates the view at the end. Validation queries
(counts by reason, quality summary, longest excluded, view count) are documented in the script header
for operators.

## Go stats extension

`GraphStats` gains `RoutableEdges` and `ExcludedEdges` (int64, JSON `routable_edges` /
`excluded_edges`). `statsQuery` adds two scalar sub-counts; `Scan` order updates to match. The
single-round-trip pattern is preserved. `excluded_by_reason` (a map needing a `GROUP BY`) is
deferred — the cleanup script already surfaces that breakdown via SQL, and keeping `GetStats` a single
flat query avoids a second query and a nullable map in the hot status path.

Scan order must stay aligned with the SELECT: nodes, edges, walkable, **routable, excluded**,
risk_scored, then the four bbox bounds.

## Testing

- `service_test.go` / `handler_test.go`: extend the fixtures and JSON assertions to include
  `routable_edges` / `excluded_edges`.
- Integration test: assert `routable_edges > 0` and `excluded_edges >= 0` once the graph is loaded;
  do not assert an exact excluded count (keeps the test robust to threshold tuning).
