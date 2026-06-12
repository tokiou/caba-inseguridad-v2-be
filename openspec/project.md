# Project Context — caba-inseguridad-v2-be

Shared context for OpenSpec change proposals. Keep this current so proposals stay aligned with the
real system.

## What this is

Backend for **CABA Rutas Seguras**: given two points in Buenos Aires (CABA), return a route that
minimizes exposure to crime hotspots. Crime data comes from open CABA datasets.

## Stack

- **Go 1.23** — HTTP API (`github.com/tokiou/caba-inseguridad-routes-go`), `chi` router, `godotenv`.
- **Python 3** — ETL pipeline under `etl/python/`.
- **PostgreSQL + PostGIS + pgRouting** — geospatial crime data (`geom GEOMETRY(Point,4326)`, GiST
  index) and the walkable road graph. Both the ETL load path and the Go read path
  (`GET /api/v1/crimes/nearby`) run on Postgres. MongoDB has been removed from the Go app. The DB
  image is `pgrouting/pgrouting:16-3.4-3.6.1` (PostGIS 3.4 + pgRouting 3.6 on PG16), host port 5434.
- **pgx (jackc/pgx v5)** — Postgres driver + connection pool. **sqlc** will be added for relational
  CRUD (future users / saved-routes).
- **OpenStreetMap + osm2pgrouting** — offline source for the CABA walkable graph (`scripts/osm/`).
  The `.pbf` and the raw `osm_*` tables are build inputs; the Go API only queries the normalized
  `road_nodes` / `road_edges`.

## Capabilities (source of truth in `specs/`)

- `data-pipeline` — XLSX → analyze → normalize → load into PostgreSQL + PostGIS.
- `health-check` — `GET /api/v1/health`.
- `crimes-api` — `GET /api/v1/crimes/nearby` geospatial proximity query.
- `routing` — `GET /api/v1/routes` via OpenRouteService.
- `road-graph` — CABA walkable graph foundation (`road_nodes` / `road_edges`), quality layer
  (`routable_road_edges`), and `GET /api/v1/roadgraph/stats`.
- `risk-scoring` — offline Network Temporal KDE pipeline (`etl/risk_network_kde/`): crime snapping,
  network neighborhoods, per-edge risk scores by time bucket / weekday type
  (`edge_risk_scores` / `edge_risk_score_components`), temporal backtest + activation gate.
- `safe-routes` — `GET /api/v1/routes/safe` (`internal/saferoutes/`): fastest / balanced / safest /
  least_safe_candidate over the local graph via pgRouting, costed with the active model's scores.
- `logging` — structured per-request logging, request-ID correlation, panic recovery.

## Architecture rules (do not skip layers)

```
HTTP request → chi router → handler → service → repository interface → concrete repository → datastore
```

- Handlers parse HTTP params and return JSON. No data access, no business logic.
- Services validate domain rules and call the repository. No HTTP, no datastore details.
- Repositories encapsulate all data access. No HTTP logic.
- `cmd/api/main.go` only wires dependencies and starts the server.

New domains live under `internal/<domain>/` with the file split: `model.go`, `dto.go`,
`repository.go`, `<store>_repository.go`, `service.go`, `handler.go`, plus `*_test.go`. Register
routes in `internal/app/routes.go`.

## Conventions

- GeoJSON / PostGIS coordinate order is always `[longitude, latitude]` — never swap.
- Data access: **pgx for PostGIS/geospatial** (raw SQL), **sqlc for relational CRUD**. sqlc cannot
  analyze PostGIS functions, so geospatial queries stay on pgx.
- Use `slog`; never log secrets (e.g. ORS API key / `Authorization`).
- Quality gate: `go build ./...` and `go test ./...` must pass before a change is archived.
