# Tasks — Road graph quality cleanup

## Migration (schema)

1. `migrations/000007_add_road_edge_quality.up.sql` — `ALTER TABLE road_edges ADD COLUMN IF NOT
   EXISTS is_routable BOOLEAN NOT NULL DEFAULT true, excluded_reason TEXT, quality_checked_at
   TIMESTAMPTZ`; indexes `(is_routable)`, `(excluded_reason)`, `(is_walkable, is_routable)`;
   `CREATE OR REPLACE VIEW routable_road_edges AS SELECT * FROM road_edges WHERE is_walkable AND
   is_routable`. `.down.sql` drops view → indexes → columns (reverse order).
2. Apply against the local DB; confirm columns, indexes, and view exist.

## Cleanup script (data classification)

3. `scripts/osm/cleanup_road_graph.sql` — single transaction, idempotent: reset all edges to
   routable, then mark `invalid_geometry`, `zero_or_negative_length`, `self_loop`,
   `suspicious_long_edge_over_5000m` (each only on still-routable rows), then recreate the view.
   Document the validation queries (count by reason, quality summary, longest excluded, view count)
   in the header.
4. Run it; verify ~183 edges excluded (zero-length + >5 km), no rows deleted, view count ==
   routable count.

## Go stats extension (`internal/roadgraph`)

5. `model.go` — add `RoutableEdges int64 json:"routable_edges"`, `ExcludedEdges int64
   json:"excluded_edges"` to `GraphStats`.
6. `postgres_repository.go` — add `(SELECT COUNT(*) FROM road_edges WHERE is_routable)` and
   `(... WHERE NOT is_routable)` to `statsQuery`; update `Scan` order to match.

## Tests

7. `service_test.go` — extend the expected `GraphStats` fixture with routable/excluded.
   `handler_test.go` — assert the JSON contains `routable_edges` / `excluded_edges`.
   `postgres_repository_integration_test.go` — assert `routable_edges > 0`, `excluded_edges >= 0`
   when the graph is loaded (no exact excluded count).

## Validate, document, archive

8. `go build ./...`; `go test ./...` (unit) green; `go test -tags=integration ./internal/roadgraph/...`
   green. Smoke `GET /api/v1/roadgraph/stats` shows `routable_edges` + `excluded_edges`; confirm
   `/crimes/nearby` still works.
9. Update `CLAUDE.md` (cleanup script + routable view) and merge the delta into
   `openspec/specs/road-graph/spec.md`; move this change to
   `openspec/changes/archive/2026-06-10-add-road-edge-quality-cleanup/`.
