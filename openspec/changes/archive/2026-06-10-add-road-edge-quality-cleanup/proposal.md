# Proposal — Road graph quality cleanup (routability layer)

## Why

The imported CABA walkable graph (`road_nodes` / `road_edges`, 71,314 nodes / 104,309 edges) contains
~183 anomalous edges from the raw OSM/osm2pgrouting output: ~92 zero-length edges and ~91 edges over
5 km (e.g. a single 32 km "edge" with 3 points). Feeding these to a future router would produce bad
paths and meaningless risk-scoring granularity. We need to exclude them from routing — but **without
deleting imported data**, so the graph stays auditable, reversible, and debuggable.

## What

- Add a **quality layer** to `road_edges`: `is_routable`, `excluded_reason`, `quality_checked_at`.
- Add a `routable_road_edges` view = walkable AND routable, the surface a future router will query.
- Add an **idempotent, non-destructive** cleanup script (`scripts/osm/cleanup_road_graph.sql`) that
  marks invalid-geometry, zero/negative-length, self-loop, and >5 km edges as non-routable.
- Extend `GET /api/v1/roadgraph/stats` with `routable_edges` and `excluded_edges`.

## Conceptual distinction

- `is_walkable` — "this edge came from the walking-profile import" (provenance).
- `is_routable` — "this edge is valid and safe for routing algorithms" (quality).
  An edge can be walkable but not routable (bad geometry, zero length, self-loop, suspicious length).

## In scope

- Migration `000007_add_road_edge_quality` (columns + indexes + view) and its `down`.
- `scripts/osm/cleanup_road_graph.sql` (reset → 4 exclusion rules → recreate view), with the
  validation queries documented in the header.
- `internal/roadgraph`: extend `GraphStats`, `statsQuery`, `Scan`, and the unit/integration tests.

## Out of scope (unchanged from the foundation milestone)

- Routing of any kind: Dijkstra, A*, pgRouting shortest-path, `/safe-routes`, route calculation.
- The risk-scoring worker and populating `edge_risk_scores` (stays empty, `risk_scored_edges = 0`).
- Deleting any imported edge (the whole point is non-destructive).
- `excluded_by_reason` breakdown in the stats endpoint — deferred (the cleanup script reports it via
  SQL; the API keeps a single-query `GetStats`).

## Acceptance

1. `road_edges` has `is_routable`, `excluded_reason`, `quality_checked_at`; `routable_road_edges`
   view exists.
2. `cleanup_road_graph.sql` is idempotent and marks (not deletes) zero/negative-length, self-loop,
   invalid-geometry, and >5 km edges as non-routable.
3. `/api/v1/roadgraph/stats` returns `routable_edges` and `excluded_edges`; after cleanup
   `routable_edges > 0`, `excluded_edges >= 0`, `risk_scored_edges = 0`.
4. `/crimes/nearby` still works; `go build ./...` and `go test ./...` pass; no routing/scoring code.
