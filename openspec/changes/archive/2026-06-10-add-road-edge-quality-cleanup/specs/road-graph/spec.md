# Road Graph — Delta

## ADDED Requirements

### Requirement: Edge routability quality layer

`road_edges` SHALL carry a non-destructive quality layer: a boolean `is_routable` (default `true`),
a nullable `excluded_reason`, and a nullable `quality_checked_at`. A `routable_road_edges` view SHALL
expose exactly the edges that are both `is_walkable` and `is_routable`. Quality classification SHALL
mark edges (never delete them), so the layer is auditable and reversible. `is_walkable` denotes
import provenance; `is_routable` denotes fitness for routing.

#### Scenario: Quality columns and view exist

- WHEN the migration is applied
- THEN `road_edges` has `is_routable`, `excluded_reason`, and `quality_checked_at`
- AND a `routable_road_edges` view returns rows where `is_walkable = true AND is_routable = true`
- AND there are indexes on `is_routable`, `excluded_reason`, and `(is_walkable, is_routable)`

#### Scenario: Cleanup marks anomalous edges non-routable without deleting

- GIVEN an imported graph containing zero/negative-length, self-loop, invalid-geometry, or
  over-5000 m edges
- WHEN the cleanup script runs
- THEN those edges have `is_routable = false` with `excluded_reason` set to
  `zero_or_negative_length`, `self_loop`, `invalid_geometry`, or `suspicious_long_edge_over_5000m`
  respectively
- AND no `road_edges` row is deleted (the total edge count is unchanged)

#### Scenario: Cleanup is idempotent

- GIVEN the cleanup script has already run
- WHEN it runs again
- THEN the set of routable and excluded edges is identical to the previous run (it resets quality
  state before re-applying the rules)

## MODIFIED Requirements

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
- AND `excluded_edges` is greater than or equal to zero and equals `edges_count - routable_edges`
  among walkable edges
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
