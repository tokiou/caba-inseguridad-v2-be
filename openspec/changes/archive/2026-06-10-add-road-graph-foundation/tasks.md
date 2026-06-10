# Tasks — CABA road graph + edge-risk foundation

> Ordering: toolchain → schema → import tooling → Go status capability → run+validate → docs/archive.
> Task 0 (Option A: pgRouting image + `osm2pgrouting`) and task 11 (the actual import run) depend on
> the toolchain. Tasks 1–10 and 12 are written tool-agnostic and can proceed immediately.

## Toolchain (Option A — confirmed)

0. `docker-compose.yml` — switch the Postgres service image from `postgis/postgis:16-3.4` to
   `pgrouting/pgrouting:16-3.4-3.6`, keeping host port `5434`, db `caba_routes`, credentials, and the
   `postgres_data` volume. Recreate the container (data preserved). Install `osm2pgrouting` on the host
   or run it from a throwaway container on the compose network. Add a `migrations/000003_enable_pgrouting`
   pair (`CREATE EXTENSION IF NOT EXISTS pgrouting;`).

## Migrations (schema)

1. `migrations/000004_create_road_graph_tables.up.sql` — `road_nodes` (BIGSERIAL pk,
   `source_node_id BIGINT UNIQUE`, `geom GEOMETRY(Point,4326) NOT NULL`, `created_at`) and
   `road_edges` (BIGSERIAL pk, `source_edge_id BIGINT UNIQUE`, `from_node_id`/`to_node_id` FK →
   `road_nodes(id)`, `street_name`, `highway_type`, `length_meters`, `walk_duration_seconds`,
   `is_walkable DEFAULT true`, `geom GEOMETRY(LineString,4326) NOT NULL`, `created_at`). Add GiST
   indexes on both `geom`, B-tree on `source_node_id`, `from_node_id`, `to_node_id`, `source_edge_id`,
   `is_walkable`. All `IF NOT EXISTS`. `.down.sql` drops both (edges first).
2. `migrations/000005_create_risk_model_tables.up.sql` — `risk_model_versions` (name UNIQUE, status
   with `CHECK (status IN ('draft','active','archived'))`, `parameters JSONB DEFAULT '{}'`,
   `created_at`, `activated_at`) and `edge_risk_scores` (pk `(edge_id, model_version_id)`, FKs with
   `ON DELETE CASCADE`, `risk_score` with `CHECK 0..1`, `crime_count` `CHECK >= 0`,
   `weighted_crime_score`, `computed_at`). Indexes on `edge_risk_scores(model_version_id)` and
   `(risk_score)`. `.down.sql` drops `edge_risk_scores` then `risk_model_versions`.
3. `migrations/000006_seed_risk_model_v1.up.sql` — insert `v1_crime_density_distance_decay` as
   `active` with `parameters` `{crime_search_radius_meters:100, risk_sensitivity_default:2.0,
   walking_speed_meters_per_second:1.4}`, `activated_at = now()`, `ON CONFLICT (name) DO NOTHING`.
   `.down.sql` deletes that row by name.
4. Apply migrations against the local DB and confirm the four tables, indexes, constraints, and the
   seed `active` model exist.

## OSM import scripts (`scripts/osm/`)

5. `download_caba_osm.sh` — `mkdir -p data/osm`; download `buenos_aires_city-latest.osm.pbf` →
   `data/osm/caba.osm.pbf`; skip if it exists unless `--force`. (File already present.)
6. `mapconfig_foot.xml` — osm2pgrouting foot profile (pedestrian highway classes in, motorway/trunk
   out). `import_osm_graph.sh` — validate `data/osm/caba.osm.pbf` exists and the `POSTGRES_*` env vars
   are set; run `osm2pgrouting` with the foot config into the DB; rename generated `ways` /
   `ways_vertices_pgr` → `osm_ways` / `osm_ways_vertices_pgr`; never hardcode credentials; clear logs.
7. `normalize_osm_graph.sql` — insert vertices → `road_nodes` and ways → `road_edges` per the design
   mapping (`length_meters` via `ST_Length(geom::geography)` fallback, `walk_duration_seconds =
   length/1.4`, `highway_type NULL`, `is_walkable true`), idempotent via `ON CONFLICT`. **Write the
   final column mapping only after inspecting** `information_schema.columns` for the two `osm_*`
   tables.

## Go status capability (`internal/roadgraph/`)

8. `model.go` (`GraphStats`), `errors.go` (e.g. `ErrEmptyGraph` if needed), `repository.go`
   (`Repository` interface), `postgres_repository.go` (`PostgresRepository` + `NewRepository` +
   `GetStats`, single combined stats query with `COALESCE`d `ST_Extent`), `service.go` (`Service` +
   `NewService` + `GetStats`), `handler.go` (`Handler` + `NewHandler` + `GetStats` + `Register`,
   `httpx.WriteJSON`, slog).
9. Wire in `internal/app/app.go` (build repo→service→handler from the shared pool + logger) and
   register `GET /api/v1/roadgraph/stats` in `internal/app/routes.go` under the `/api/v1` group.

## Tests

10. `service_test.go` (fake repo: returns value; propagates error) and `handler_test.go` (fake
    service: 200 + valid JSON; error → 500), mirroring the `crimes` tests. Add
    `postgres_repository_integration_test.go` (`//go:build integration`) that runs `GetStats` and
    skips with a clear message when the graph is empty.

## Run, validate, document

11. Provision pgRouting + `osm2pgrouting` per design's OPEN DECISION (Option A), then run
    `download` → `import` → `normalize`. Verify `GET /api/v1/roadgraph/stats` returns **non-zero**
    `nodes_count` / `edges_count` / `walkable_edges`, `risk_scored_edges = 0`, and a CABA bounding box.
    Confirm `/crimes/nearby` still works. `go build ./...` and `go test ./...` green.
12. Update `openspec/project.md` (add `road-graph` capability + toolchain note) and `CLAUDE.md`
    (new `internal/roadgraph` domain, `scripts/osm/`, pgRouting image). Archive: merge the delta into
    `openspec/specs/road-graph/spec.md`, then move this folder to
    `openspec/changes/archive/<YYYY-MM-DD>-add-road-graph-foundation/`.
