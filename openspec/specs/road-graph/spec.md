# Road Graph Specification

## Purpose

Provide the foundation for CABA safe-walking routing: a clean, queryable walkable road graph
(`road_nodes` / `road_edges`) built offline from OpenStreetMap, and a read-only status endpoint to
verify the import. Risk storage and scoring belong to the `risk-scoring` capability; routing over
the graph belongs to `graph-routing` (plain walk routes) and `safe-routes` (risk-weighted).

> Data access: the walkable graph lives in **PostgreSQL + PostGIS + pgRouting**. The
> OSM `.pbf` and the raw `osm_*` tables produced by `osm2pgrouting` are offline build inputs — the Go
> API queries only `road_nodes` / `road_edges` via pgx (`internal/roadgraph`).

## Requirements

### Requirement: Walkable road graph schema

The system SHALL persist a walkable road graph as two tables: `road_nodes` (graph vertices) and
`road_edges` (walkable segments between two nodes). `road_nodes` SHALL carry a unique
`source_node_id` (the originating graph-tool vertex id) and a `geometry(Point, 4326)`. `road_edges`
SHALL carry a unique `source_edge_id`, `from_node_id`/`to_node_id` foreign keys to `road_nodes`,
`length_meters`, `walk_duration_seconds`, an `is_walkable` flag, and a `geometry(LineString, 4326)`.
Risk attaches to edges, not nodes.

#### Scenario: Graph tables and indexes exist

- WHEN the migrations are applied
- THEN `road_nodes` and `road_edges` exist with the columns above
- AND there is a GiST index on `road_nodes.geom` and on `road_edges.geom`
- AND there are B-tree indexes on `road_edges.from_node_id` and `road_edges.to_node_id`
- AND `road_nodes.source_node_id` and `road_edges.source_edge_id` are unique

#### Scenario: Edges reference existing nodes

- GIVEN a row in `road_edges`
- THEN its `from_node_id` and `to_node_id` reference existing `road_nodes(id)` rows via foreign keys

### Requirement: Offline OSM import and normalization

The walkable graph SHALL be built offline from an OpenStreetMap `.pbf` extract of CABA via
`osm2pgrouting`, with the tool-generated tables normalized into `road_nodes` / `road_edges`. The
`.pbf` file and the raw generated tables SHALL NOT be queried by the Go API. Normalization SHALL be
idempotent.

#### Scenario: Normalization populates the clean tables

- GIVEN `osm2pgrouting` has imported CABA OSM data into raw tables (`osm_ways`,
  `osm_ways_vertices_pgr`)
- WHEN the normalization SQL runs
- THEN each raw vertex becomes a `road_nodes` row (`source_node_id` = vertex id, `geom` in SRID 4326)
- AND each raw way becomes a `road_edges` row whose `from_node_id`/`to_node_id` resolve via
  `source_node_id`, with `length_meters` and `walk_duration_seconds` populated

#### Scenario: Re-running normalization is idempotent

- GIVEN `road_nodes` / `road_edges` already populated from a prior run
- WHEN the normalization SQL runs again
- THEN no duplicate rows are created (conflicts on `source_node_id` / `source_edge_id` are ignored)

#### Scenario: Walking duration derived from length when unavailable

- GIVEN a raw way without a usable walking-cost column
- WHEN it is normalized
- THEN `walk_duration_seconds` is computed as `ST_Length(geom::geography) / 1.4` (1.4 m/s)

### Requirement: Edge routability quality layer

`road_edges` SHALL carry a non-destructive quality layer: a boolean `is_routable` (default `true`),
a nullable `excluded_reason`, and a nullable `quality_checked_at`. A `routable_road_edges` view SHALL
expose exactly the edges that are both `is_walkable` and `is_routable`. Quality classification SHALL
mark edges (never delete them), so the layer is auditable and reversible. `is_walkable` denotes
import provenance; `is_routable` denotes fitness for routing. After the per-edge rules, edges
outside the **largest connected component** of the surviving routable graph SHALL be marked
non-routable (`disconnected_component`), so nearest-edge snapping can never strand a route request
on an isolated island.

#### Scenario: Quality columns and view exist

- WHEN the migration is applied
- THEN `road_edges` has `is_routable`, `excluded_reason`, and `quality_checked_at`
- AND a `routable_road_edges` view returns rows where `is_walkable = true AND is_routable = true`
- AND there are indexes on `is_routable`, `excluded_reason`, and `(is_walkable, is_routable)`

#### Scenario: Cleanup marks anomalous edges non-routable without deleting

- GIVEN an imported graph containing zero/negative-length, self-loop, invalid-geometry,
  over-5000 m, or disconnected-island edges
- WHEN the cleanup script runs
- THEN those edges have `is_routable = false` with `excluded_reason` set to
  `zero_or_negative_length`, `self_loop`, `invalid_geometry`, `suspicious_long_edge_over_5000m`,
  or `disconnected_component` respectively
- AND no `road_edges` row is deleted (the total edge count is unchanged)

#### Scenario: Cleanup is idempotent

- GIVEN the cleanup script has already run
- WHEN it runs again
- THEN the set of routable and excluded edges is identical to the previous run (it resets quality
  state before re-applying the rules)

#### Scenario: Routable graph is a single connected component

- GIVEN the cleanup script has run
- WHEN `pgr_connectedComponents` is computed over `routable_road_edges`
- THEN exactly one component remains
- AND any two routable edges are mutually reachable

### Requirement: Graph status endpoint

The API SHALL expose `GET /api/v1/roadgraph/stats` returning the graph's `nodes_count`,
`edges_count`, `walkable_edges`, `routable_edges`, `excluded_edges`, `risk_scored_edges`, and
bounding box (`min_lat`, `min_lng`, `max_lat`, `max_lng`) as JSON. It is a read-only status probe; an
empty graph is a valid state.

#### Scenario: Stats after a successful import and cleanup

- GIVEN the OSM graph has been imported, normalized, and cleaned
- WHEN a client sends `GET /api/v1/roadgraph/stats`
- THEN the response is HTTP 200
- AND `nodes_count`, `edges_count`, `walkable_edges`, and `routable_edges` are greater than zero
- AND `excluded_edges` is greater than or equal to zero
- AND `risk_scored_edges` is `0` (no scoring has run yet)
- AND the bounding box lies within CABA

#### Scenario: Stats on an empty graph

- GIVEN the graph tables exist but are empty
- WHEN the endpoint is queried
- THEN the response is HTTP 200 with all counts `0` (including `routable_edges` and `excluded_edges`)
  and a zero-valued bounding box (not an error)

#### Scenario: Datastore failure is not leaked

- GIVEN the underlying query fails
- WHEN the endpoint is queried
- THEN the response is HTTP 500 with a generic error and no datastore detail in the body

### Requirement: Layered architecture for the road-graph status path

The request flow SHALL be `handler → service → repository interface → PostgresRepository →
PostgreSQL/PostGIS`, living under `internal/roadgraph/`. Handlers MUST NOT contain data-access logic;
the repository MUST NOT contain HTTP logic. PostGIS access uses pgx. This capability SHALL NOT
implement routing (`/safe-routes`, Dijkstra, A*, pgRouting path queries).

#### Scenario: Layer boundaries respected

- WHEN the road-graph status capability is implemented
- THEN HTTP parsing lives only in the handler, orchestration in the service, and PostGIS access
  behind the `Repository` interface implemented by `PostgresRepository`

#### Scenario: No routing in this milestone

- WHEN this capability is complete
- THEN no `/safe-routes` endpoint and no shortest-path query exist
- AND the existing `/crimes/nearby` endpoint continues to work unchanged
