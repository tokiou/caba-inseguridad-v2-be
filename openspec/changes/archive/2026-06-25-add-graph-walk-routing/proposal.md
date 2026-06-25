# Add walkable routing over the local road graph

## Why

The `road-graph` capability imported, cleaned, and exposed the CABA walkable graph
(`road_nodes` / `routable_road_edges`) but explicitly left routing as future work. The existing
`routing` capability routes by delegating to OpenRouteService (an external API) — it does not use our
own graph, and it is not where crime-weighted scoring can plug in.

We need a first routing capability **over our own graph**: given two CABA points, return the shortest
walkable path using pgRouting (`pgr_dijkstra`) over `routable_road_edges`. This is the foundation the
future risk-scoring milestone builds on — once `edge_risk_scores` is populated, the same query swaps
its cost expression from plain distance to a risk-weighted cost. Standing this up now validates the
graph topology end-to-end (snapping, connectivity, path reconstruction) before risk is added.

## What changes

- **New capability `graph-routing`** with endpoint `GET /api/v1/roadgraph/route` returning the
  shortest walkable path between two points: total distance, walking duration, edge count, the two
  snapped graph nodes, and a GeoJSON `LineString` geometry.
- **New code in `internal/roadgraph/`** (same package as the stats path, since both are pure
  graph concerns): DTO, route model structs, errors, repository method `FindWalkRoute`, service
  validation/orchestration, handler + route registration.
- **Repository query**: snap origin/destination to the nearest `road_nodes` via a GiST KNN lookup,
  then `pgr_dijkstra` (undirected, cost = `length_meters`) over `routable_road_edges`, aggregating
  the path edges into total distance, duration, and a merged `LineString`.
- **Tests**: unit tests for the service (validation, defaults, no-route mapping) and handler
  (param parsing, status codes, error bodies), plus an `//go:build integration` test that runs a real
  `pgr_dijkstra` against the live graph.

## In scope

- Shortest walkable path by distance over the existing routable graph.
- Nearest-node snapping for arbitrary CABA coordinates.
- `404` when no path connects the snapped nodes; `400` for invalid/out-of-CABA input.

## Out of scope

- Crime-weighted / risk-adjusted cost (future scoring milestone — `edge_risk_scores` is still empty).
- Alternative routes, turn-by-turn instructions, or A* heuristics.
- Changing or replacing the ORS-based `/api/v1/routes` endpoint (separate capability).
- Authentication, persistence of computed routes.
