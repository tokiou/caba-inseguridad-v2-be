# Road Graph — Delta

## ADDED Requirements

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

### Requirement: Risk model versioning

The system SHALL persist named, versioned risk models in `risk_model_versions`, each with a unique
`name`, a `status` constrained to `draft` / `active` / `archived`, a JSONB `parameters` object, and
timestamps. A first model SHALL be seeded as `active`.

#### Scenario: Status is constrained

- WHEN a `risk_model_versions` row is inserted with a `status` outside `{draft, active, archived}`
- THEN the insert is rejected by a check constraint

#### Scenario: Seed model is active

- WHEN the migrations are applied
- THEN a model named `v1_crime_density_distance_decay` exists with `status = 'active'`
- AND its `parameters` include `crime_search_radius_meters`, `risk_sensitivity_default`, and
  `walking_speed_meters_per_second`

### Requirement: Per-edge risk score storage

The system SHALL store per-edge risk in `edge_risk_scores`, keyed by `(edge_id, model_version_id)`,
with a `risk_score` constrained to `[0, 1]`, a non-negative `crime_count`, a `weighted_crime_score`,
and a `computed_at`. Rows SHALL cascade-delete with their edge or model version. This milestone only
creates the table; it is populated by a future scoring milestone.

#### Scenario: Score range is constrained

- WHEN an `edge_risk_scores` row is inserted with `risk_score` outside `[0, 1]` or a negative
  `crime_count`
- THEN the insert is rejected by a check constraint

#### Scenario: Scores cascade with their edge

- GIVEN an edge with risk scores
- WHEN the `road_edges` row is deleted
- THEN its `edge_risk_scores` rows are removed

#### Scenario: No scores before the scoring milestone

- WHEN the graph has been imported but no scoring has run
- THEN `edge_risk_scores` is empty and the graph-status `risk_scored_edges` count is `0`

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

### Requirement: Graph status endpoint

The API SHALL expose `GET /api/v1/roadgraph/stats` returning the graph's `nodes_count`,
`edges_count`, `walkable_edges`, `risk_scored_edges`, and bounding box (`min_lat`, `min_lng`,
`max_lat`, `max_lng`) as JSON. It is a read-only status probe; an empty graph is a valid state.

#### Scenario: Stats after a successful import

- GIVEN the OSM graph has been imported and normalized
- WHEN a client sends `GET /api/v1/roadgraph/stats`
- THEN the response is HTTP 200
- AND `nodes_count`, `edges_count`, and `walkable_edges` are greater than zero
- AND `risk_scored_edges` is `0` (no scoring has run yet)
- AND the bounding box lies within CABA

#### Scenario: Stats on an empty graph

- GIVEN the graph tables exist but are empty
- WHEN the endpoint is queried
- THEN the response is HTTP 200 with all counts `0` and a zero-valued bounding box (not an error)

#### Scenario: Datastore failure is not leaked

- GIVEN the underlying query fails
- WHEN the endpoint is queried
- THEN the response is HTTP 500 with a generic error and no datastore detail in the body

### Requirement: Layered architecture for the road-graph status path

The request flow SHALL be `handler → service → repository interface → PostgresRepository →
PostgreSQL/PostGIS`, living under `internal/roadgraph/`. Handlers MUST NOT contain data-access logic;
the repository MUST NOT contain HTTP logic. PostGIS access uses pgx. This milestone SHALL NOT
implement routing (`/safe-routes`, Dijkstra, A*, pgRouting path queries).

#### Scenario: Layer boundaries respected

- WHEN the road-graph status capability is implemented
- THEN HTTP parsing lives only in the handler, orchestration in the service, and PostGIS access
  behind the `Repository` interface implemented by `PostgresRepository`

#### Scenario: No routing in this milestone

- WHEN this change is complete
- THEN no `/safe-routes` endpoint and no shortest-path query exist
- AND the existing `/crimes/nearby` endpoint continues to work unchanged
