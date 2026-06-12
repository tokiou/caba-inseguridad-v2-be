# Tasks — Add walkable routing over the local road graph

## 1. Domain types & errors (`internal/roadgraph/`)
- [x] 1.1 Add `WalkRouteQuery` DTO (`dto.go`): `FromLat`, `FromLng`, `ToLat`, `ToLng`.
- [x] 1.2 Add route model structs (`model.go`): `WalkRoute`, `RoutePoint`, `GeoJSONLineString`.
- [x] 1.3 Add `errors.go`: `ErrInvalidCoordinates`, `ErrNoRoute`.

## 2. Repository
- [x] 2.1 Extend `Repository` interface with `FindWalkRoute(ctx, WalkRouteQuery) (WalkRoute, error)`.
- [x] 2.2 Implement `FindWalkRoute` on `PostgresRepository`: KNN snap of origin/destination to the
      nearest *routable edge* (entering at its nearer endpoint, to avoid orphan nodes),
      `pgr_dijkstra` (undirected, cost = `length_meters`) over `routable_road_edges`, aggregate
      distance/duration/edge-count and `ST_AsGeoJSON(ST_LineMerge(...))`.
- [x] 2.3 Map an empty path (no edges / NULL geometry) to `ErrNoRoute`; parse the GeoJSON into
      `[][]float64` coordinates.

## 3. Service
- [x] 3.1 Add `WalkRoute(ctx, WalkRouteQuery) (WalkRoute, error)` with CABA-bounds validation and
      distinct-endpoint check; delegate to the repository.

## 4. Handler & wiring
- [x] 4.1 Extend the handler `service` interface with `WalkRoute`; add `GetWalkRoute` handler parsing
      `from_lat`/`from_lng`/`to_lat`/`to_lng`.
- [x] 4.2 Register `GET /roadgraph/route`; map `ErrInvalidCoordinates`→400, `ErrNoRoute`→404,
      else→500 (no datastore detail leaked).

## 5. Tests
- [x] 5.1 Service unit tests: valid route, out-of-CABA 400, same origin/destination 400, no-route
      propagation, repository error propagation.
- [x] 5.2 Handler unit tests: 200 with geometry, 400 on missing/invalid params, 404 on no route,
      500 on repository failure.
- [x] 5.3 Integration test (`//go:build integration`): real `pgr_dijkstra` between two known CABA
      points returns a connected `LineString` with positive distance/duration.

## 6. Verify
- [x] 6.1 `go build ./...` and `go test ./...` green; `go test -tags=integration ./internal/roadgraph/...`
      against the live graph.
- [ ] 6.2 Archive the change into `openspec/specs/graph-routing/` once agreed and implemented.
