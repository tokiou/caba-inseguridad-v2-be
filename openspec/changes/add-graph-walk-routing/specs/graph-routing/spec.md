# Graph Routing Specification

## ADDED Requirements

### Requirement: Walkable route endpoint over the local graph

The API SHALL expose `GET /api/v1/roadgraph/route` accepting `from_lat`, `from_lng`, `to_lat`, and
`to_lng`, and returning the shortest walkable path between the two points computed over the local
road graph (`routable_road_edges`) via pgRouting. The response SHALL contain the origin and
destination (each echoing the requested `lat`/`lng` and the `snapped_node_id` actually routed),
`distance_meters`, `duration_seconds`, `edge_count`, and a GeoJSON `LineString` `geometry` with
`[longitude, latitude]` coordinates. Routing SHALL read only `routable_road_edges` (never raw
`road_edges`), use distance (`length_meters`) as cost, and treat the graph as undirected.

#### Scenario: Successful walkable route

- GIVEN two valid CABA coordinates connected on the walkable graph
- WHEN a client sends `GET /api/v1/roadgraph/route?from_lat=-34.6037&from_lng=-58.3816&to_lat=-34.5895&to_lng=-58.4201`
- THEN the response is HTTP 200
- AND the body contains `from`, `to`, `distance_meters`, `duration_seconds`, `edge_count`, and a
  GeoJSON `LineString` `geometry`
- AND `from.snapped_node_id` and `to.snapped_node_id` are populated graph node ids
- AND `distance_meters` and `duration_seconds` are greater than zero

#### Scenario: Coordinates are snapped into the routable graph

- GIVEN a coordinate that does not lie exactly on a graph node
- WHEN the route is requested
- THEN the origin/destination is snapped to the nearest routable edge via a GiST KNN lookup and the
  graph is entered at that edge's nearer endpoint (so the entry node always participates in the
  routable graph)
- AND the chosen node id is returned as `snapped_node_id`

### Requirement: Routing input validation

The service SHALL validate that both endpoints are within the CABA bounding box
(`lat ∈ [-35, -34]`, `lng ∈ [-59, -58]`) and are distinct, returning HTTP 400 with error code
`invalid_request` otherwise.

#### Scenario: Missing or unparseable coordinate

- GIVEN a request missing any of `from_lat`/`from_lng`/`to_lat`/`to_lng`, or with a non-numeric value
- WHEN the endpoint is queried
- THEN the response is HTTP 400 with error code `invalid_request`

#### Scenario: Endpoint out of CABA bounds

- GIVEN an origin or destination outside `lat ∈ [-35, -34]` / `lng ∈ [-59, -58]`
- WHEN the endpoint is queried
- THEN the response is HTTP 400 with error code `invalid_request`

#### Scenario: Origin equals destination

- GIVEN identical origin and destination coordinates
- WHEN the endpoint is queried
- THEN the response is HTTP 400 with error code `invalid_request`

### Requirement: No-route and failure handling

When the snapped origin and destination are not connected on the routable graph, the API SHALL
return HTTP 404 with error code `route_not_found`. Datastore or query failures SHALL return HTTP 500
with a generic error and no datastore detail in the body.

#### Scenario: Disconnected endpoints

- GIVEN two graph nodes with no walkable path between them
- WHEN the route is requested
- THEN the response is HTTP 404 with error code `route_not_found`

#### Scenario: Datastore failure is not leaked

- GIVEN the underlying routing query fails
- WHEN the endpoint is queried
- THEN the response is HTTP 500 with a generic error and no datastore detail in the body

### Requirement: Layered architecture for graph routing

The flow SHALL be `handler → service → repository interface → PostgresRepository →
PostgreSQL/PostGIS/pgRouting`, living under `internal/roadgraph/`. The handler MUST NOT contain
data-access logic and the repository MUST NOT contain HTTP logic. All pgRouting/PostGIS SQL lives
behind the `Repository` interface implemented by `PostgresRepository`, using pgx.

#### Scenario: Layer boundaries respected

- WHEN the graph-routing capability is implemented or modified
- THEN HTTP parsing lives only in the handler, validation/defaults in the service, and all
  pgRouting/PostGIS access behind the `Repository` interface

#### Scenario: Risk weighting is not part of this capability

- WHEN this capability is complete
- THEN routing cost is plain walking distance and `edge_risk_scores` is not read
- AND the existing ORS-based `/api/v1/routes` endpoint continues to work unchanged
