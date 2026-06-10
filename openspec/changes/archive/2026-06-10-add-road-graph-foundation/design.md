# Design — CABA road graph + edge-risk foundation

## Pipeline overview

```
data/osm/caba.osm.pbf        (offline input, already downloaded — full CABA, verified)
        │  osm2pgrouting (foot profile)            scripts/osm/import_osm_graph.sh
        ▼
osm_ways / osm_ways_vertices_pgr   (raw, tool-owned — NOT queried by the Go API)
        │  normalization SQL (ON CONFLICT, idempotent)   scripts/osm/normalize_osm_graph.sql
        ▼
road_nodes / road_edges            (clean internal schema — the app's source of truth)
        │  [future] risk worker writes
        ▼
edge_risk_scores  ×  risk_model_versions
        │  [future] pgRouting / Dijkstra over safe_cost = walk_duration * (1 + risk * sensitivity)
        ▼
[future] /safe-routes
```

The app domain depends only on `road_nodes` / `road_edges` / `risk_*`, never on the `osm_*` raw table
names. We can re-import / swap the OSM tool without touching Go code.

## DECISION (resolved) — routing toolchain: Option A

`osm2pgrouting` requires the **pgRouting** extension in the database, and the CLI itself must be
installed. Initial state of this environment:

- DB image was `postgis/postgis:16-3.4` — **bundles PostGIS but NOT pgRouting** (`CREATE EXTENSION
  pgrouting` will fail; the `.so` is not in the image).
- `osm2pgrouting`, `osmium`, `osmconvert` are **not installed** on the host.

**Chosen — Option A (confirmed with the user):** switch the DB service image to
**`pgrouting/pgrouting:16-3.4-3.6`** (official image = PostGIS 3.4 + pgRouting 3.6 on PG16), keeping
the same host port `5434`, db `caba_routes`, credentials, and the `postgres_data` volume. Then install
`osm2pgrouting` on the host (`apt-get install osm2pgrouting`) **or** run it from a throwaway container
on the compose network. This keeps the existing crime data intact and unblocks both this import and
future pgRouting path queries. `CREATE EXTENSION IF NOT EXISTS pgrouting;` is added as an early
migration (alongside the existing PostGIS one).

Alternatives considered (rejected):
- **Option B — keep image, no pgRouting:** import the graph without `osm2pgrouting` (e.g. `osmium
  export` + a custom splitter, or `osm2po`). Avoids the image change now but reinvents intersection
  splitting and still needs pgRouting later for routing — so it only defers the problem.
- **Option C — pgRouting extension into current image:** not practical; `postgis/postgis` has no
  pgRouting package layer.

**This change's code (migrations, scripts, Go package, endpoint) is written to be tool-agnostic and
can be implemented immediately. Only the actual import *run* (step that populates `road_nodes`/
`road_edges`) is blocked on Option A. Acceptance criterion #3 (non-zero stats) is verified after the
toolchain is in place.**

## Migration naming — align with existing convention

The repo uses `golang-migrate` paired files (`000001_*.up.sql` / `.down.sql`), not the single-file
`010_*.sql` names suggested in the source spec. We follow the existing convention:

- `000003_enable_pgrouting.{up,down}.sql` — `CREATE EXTENSION IF NOT EXISTS pgrouting;` (mirrors
  `000001_enable_postgis`; `down` is a no-op comment, like the PostGIS one). Requires the
  `pgrouting/pgrouting` image (Option A).
- `000004_create_road_graph_tables.{up,down}.sql` — `road_nodes`, `road_edges` + indexes.
- `000005_create_risk_model_tables.{up,down}.sql` — `risk_model_versions`, `edge_risk_scores` +
  constraints/indexes (must follow 000004: `edge_risk_scores` FKs `road_edges`).
- `000006_seed_risk_model_v1.{up,down}.sql` — seed `v1_crime_density_distance_decay` (`up` inserts
  `ON CONFLICT (name) DO NOTHING`; `down` deletes that one row by name).

Every `up` is idempotent (`CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`,
`ON CONFLICT`); every `down` drops in reverse dependency order.

## osm2pgrouting output & the normalization mapping

`osm2pgrouting` splits OSM ways at intersections and produces:
- `ways_vertices_pgr(id, the_geom, …)` — graph vertices (our nodes).
- `ways(gid, osm_id, source, target, name, length, length_m, cost, reverse_cost, cost_s,
  reverse_cost_s, x1,y1,x2,y2, the_geom, class_id, …)` — graph edges.

Per the source spec we import into the default names and **rename** to `osm_ways` /
`osm_ways_vertices_pgr` so they never collide with internal tables. The normalization SQL must be
written **after inspecting the real columns** (`information_schema.columns`), because column presence
varies by `osm2pgrouting` version:

- **nodes:** `source_node_id ← osm_ways_vertices_pgr.id`, `geom ← the_geom` (re-stamped SRID 4326).
- **edges:** `source_edge_id ← osm_ways.gid` (unique per split segment), `from/to_node_id` via join on
  `source_node_id = osm_ways.source / .target`.
  - `length_meters ← COALESCE(length_m, ST_Length(the_geom::geography))`.
  - `walk_duration_seconds ← ST_Length(the_geom::geography) / 1.4` (1.4 m/s walking speed). We do **not**
    reuse `cost`/`cost_s` because under a foot profile those may be distance- or vehicle-derived;
    deriving from geography length is unambiguous and store-independent.
  - `street_name ← NULLIF(name,'')`.
  - `highway_type ← NULL` this milestone. `osm2pgrouting` keeps the highway class only as a
    `class_id` FK into its `configuration`/`osm_way_classes` tables; resolving it is a follow-up
    backfill, out of scope here.
  - `is_walkable ← true`. Because we import with a **foot profile** (only pedestrian-relevant classes),
    every imported edge is walkable; the column exists so a later pass can flip edges off without a
    schema change.
- Both inserts are idempotent via `ON CONFLICT (source_node_id|source_edge_id) DO NOTHING`, so
  re-running normalization after a re-import is safe.

Edges are stored once with `from_node_id → to_node_id`. Walking is undirected; the future routing
layer will treat edges as bidirectional (or pgRouting will use `cost`/`reverse_cost`). No directional
duplication is stored now.

## Foot profile for osm2pgrouting

`osm2pgrouting` needs a `mapconfig` XML listing the highway classes to import. The default ships a
car-oriented config. We add `scripts/osm/mapconfig_foot.xml` enabling pedestrian classes
(`footway`, `path`, `pedestrian`, `steps`, `living_street`, `residential`, `service`, `unclassified`,
`tertiary`, `secondary`, `primary`, `cycleway` where foot-allowed, …) and excluding `motorway`/
`motorway_link`/`trunk`. This keeps the graph walkable-by-construction (hence `is_walkable = true`).

## Go package `internal/roadgraph` — status only

Mirrors the `crimes` layering. This milestone exposes a single read operation; no writes, no routing.

```go
// model.go
type GraphStats struct {
    NodesCount      int64   `json:"nodes_count"`
    EdgesCount      int64   `json:"edges_count"`
    WalkableEdges   int64   `json:"walkable_edges"`
    RiskScoredEdges int64   `json:"risk_scored_edges"`
    MinLat          float64 `json:"min_lat"`
    MinLng          float64 `json:"min_lng"`
    MaxLat          float64 `json:"max_lat"`
    MaxLng          float64 `json:"max_lng"`
}

// repository.go
type Repository interface { GetStats(ctx context.Context) (GraphStats, error) }

// postgres_repository.go
type PostgresRepository struct { pool *pgxpool.Pool }
func NewRepository(pool *pgxpool.Pool) *PostgresRepository
```

This domain has two concrete files (`repository.go` for the interface, `postgres_repository.go` for
the implementation), following the project's documented file split for **new** domains — unlike
`crimes`, which kept a single `repository.go` only because it was a single-datastore migration of
pre-existing code.

### Stats query & empty-graph handling

```sql
SELECT
  (SELECT COUNT(*) FROM road_nodes)                          AS nodes_count,
  (SELECT COUNT(*) FROM road_edges)                          AS edges_count,
  (SELECT COUNT(*) FROM road_edges WHERE is_walkable)        AS walkable_edges,
  (SELECT COUNT(*) FROM edge_risk_scores)                    AS risk_scored_edges,
  COALESCE(ST_YMin(ext), 0), COALESCE(ST_XMin(ext), 0),
  COALESCE(ST_YMax(ext), 0), COALESCE(ST_XMax(ext), 0)
FROM (SELECT ST_Extent(geom) AS ext FROM road_nodes) e;
```

Before the import, `road_nodes` is empty → `ST_Extent` is `NULL` → `COALESCE` yields a zero-valued
bounding box and all counts `0`. The endpoint returns **HTTP 200** with zeros (it is a status probe;
"empty graph" is a valid, expected state, not an error). Datastore failures wrap as
`fmt.Errorf("roadgraph: get stats: %w", err)` and surface as HTTP 500 with a generic body.

## Endpoint

`GET /api/v1/roadgraph/stats`, registered in `internal/app/routes.go` under the existing `/api/v1`
group, wired in `internal/app/app.go` with the shared `*pgxpool.Pool` and `slog.Logger`, using
`httpx.WriteJSON`. Dev/admin status probe; no auth (none exists project-wide yet).

## Testing

- **service_test.go** — fake repository: `GetStats` returns the repo value; repo error propagates.
- **handler_test.go** — fake service: 200 with valid JSON body (`nodes_count` etc. present); service
  error → 500 generic body. Mirrors `crimes/handler_test.go`.
- **postgres_repository_integration_test.go** (`//go:build integration`) — against live PostGIS
  (`DATABASE_URL`, default `localhost:5434`): `GetStats` runs without error and returns
  non-negative counts. Asserting non-zero counts is gated on the OSM import having run, so the test
  skips with a clear message when the graph is empty. Excluded from default `go test ./...`.
