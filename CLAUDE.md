# CLAUDE.md — caba-inseguridad-v2-be

## Project

Backend for **CABA Rutas Seguras**: given two points in Buenos Aires (CABA), return a route that minimizes exposure to crime hotspots. Crime data comes from open CABA datasets loaded into PostgreSQL + PostGIS.

## Stack

- **Go 1.25** — HTTP API (`github.com/tokiou/caba-inseguridad-routes-go`)
- **Python 3** — ETL pipeline under `etl/python/`
- **PostgreSQL + PostGIS** — geospatial crime data (`geom GEOMETRY(Point,4326)`, GiST index)
- **pgx (jackc/pgx v5)** — Postgres driver + pool; **sqlc** to be added for relational CRUD
- **chi** — HTTP router
- **godotenv** — env loading

> **Data access rule:** **pgx** owns all PostGIS / geospatial queries (raw SQL); **sqlc** owns
> relational CRUD (introduced with the future users / saved-routes capability). sqlc's analyzer does
> not understand PostGIS functions, so geospatial queries stay on pgx.

## Repository layout

```
cmd/api/main.go                   # entrypoint: wires deps, starts server
internal/
  app/app.go                      # App struct (router + pgx pool)
  app/routes.go                   # chi route registration
  config/config.go                # Config struct loaded from env
  platform/postgres/pool.go       # pgx connection pool + ping
  health/handler.go               # GET /api/v1/health
  crimes/
    model.go                      # Crime + GeoJSON structs
    dto.go                        # NearbyCrimesQuery + NearbyCrimesResponse
    repository.go                 # Repository interface + PostgresRepository (PostGIS ST_DWithin)
    service.go                    # business validation
    handler.go                    # HTTP parsing + JSON responses
    service_test.go
    handler_test.go
    repository_integration_test.go # //go:build integration — live PostGIS test
  roadgraph/                      # walkable road graph status (foundation)
    model.go                      # GraphStats
    repository.go                 # Repository interface
    postgres_repository.go        # PostgresRepository (counts + ST_Extent bbox)
    service.go                    # status orchestration
    handler.go                    # GET /api/v1/roadgraph/stats
    service_test.go / handler_test.go
    postgres_repository_integration_test.go # //go:build integration
  httpx/response.go               # shared JSON helpers
scripts/osm/                      # offline OSM → road graph import (not queried by the API)
  download_caba_osm.sh            # fetch CABA .pbf into data/osm/
  import_osm_graph.sh             # osm2pgrouting (foot profile) → osm_ways / osm_ways_vertices_pgr
  mapconfig_foot.xml              # osm2pgrouting pedestrian highway-class config
  normalize_osm_graph.sql         # raw osm_* tables → road_nodes / road_edges (idempotent)
  cleanup_road_graph.sql          # mark anomalous edges is_routable=false (non-destructive, idempotent)
etl/python/
  analyze_raw_data.py             # raw quality report
  normalize_crimes.py             # XLSX → JSONL normalization
  load_to_postgres.py             # upsert into PostgreSQL + PostGIS (active loader)
  load_to_mongo.py                # legacy MongoDB loader (kept for reference)
  requirements.txt
migrations/                       # SQL migrations (000001_enable_postgis … 000006_seed_risk_model_v1)
data/
  raw/                            # source XLSX files (not committed)
  processed/                      # generated JSONL/JSON artifacts
openspec/                         # OpenSpec — spec-driven development (see Development workflow)
  project.md                      # shared project context for change proposals
  specs/                          # source of truth: current capabilities (one dir per capability)
  changes/                        # proposed changes (one folder each); archive/ holds completed ones
```

## Architecture rules

The request flow is strictly layered — do not skip layers:

```
HTTP request → chi router → handler → service → repository interface → PostgresRepository → PostgreSQL/PostGIS
```

- **Handlers** parse HTTP params and return JSON. No data access, no business logic.
- **Services** validate domain rules and call the repository. No HTTP, no datastore details.
- **Repositories** encapsulate all data access. No HTTP logic.
- **`main.go`** only wires dependencies and starts the server.

## Environment variables

```env
APP_ENV=development
HTTP_PORT=8080
DATABASE_URL=postgres://postgres:postgres@localhost:5434/caba_routes?sslmode=disable
LOG_LEVEL=info          # debug | info | warn | error
LOG_FORMAT=json         # json (prod) | text (colored dev console)
```

The Postgres container (`pgrouting/pgrouting:16-3.4-3.6.1` — PostGIS 3.4 + pgRouting 3.6 on PG16) maps
host port **5434** → container 5432 (avoids clashes with other local Postgres instances).
`POSTGRES_HOST/PORT/DB/USER/PASSWORD` are also read by docker-compose, the ETL, and the OSM import.

Copy `.env.example` to `.env` before running.

## Running

```bash
# Start the API
go run ./cmd/api

# Run tests
go test ./...
```

## ETL pipeline (Python)

```bash
cd etl/python
pip install -r requirements.txt
python analyze_raw_data.py    # → data/processed/raw_data_report.json
python normalize_crimes.py    # → data/processed/crimes_normalized.jsonl + rejected_rows.jsonl
python load_to_postgres.py    # upserts into PostgreSQL + PostGIS (idempotent; run from repo root)
```

Normalized schema includes `source_id` (unique key), `location` as GeoJSON Point with `[lng, lat]`, and boolean fields `weapon_used` / `motorcycle_used`.

## OSM road graph (offline import)

Builds the walkable graph for CABA from OpenStreetMap. The `.pbf` and the raw `osm_*` tables are
build inputs — never queried by the API; only the normalized `road_nodes` / `road_edges` are.

```bash
scripts/osm/download_caba_osm.sh          # → data/osm/caba.osm.pbf (skip if present; --force to redo)
scripts/osm/import_osm_graph.sh           # osm2pgrouting (foot profile) → osm_ways / osm_ways_vertices_pgr
psql "$DATABASE_URL" -f scripts/osm/normalize_osm_graph.sql   # → road_nodes / road_edges (idempotent)
psql "$DATABASE_URL" -f scripts/osm/cleanup_road_graph.sql    # mark non-routable edges (idempotent)
```

osm2pgrouting 2.x reads OSM XML, so `import_osm_graph.sh` converts the `.pbf` with `osmium` first; it
runs both tools inside a throwaway container by default (set `OSM_IMPORT_NATIVE=1` to use host
binaries). `cleanup_road_graph.sql` marks anomalous edges (zero-length, self-loops, invalid geometry,
>5 km) `is_routable = false` **without deleting** them; routing reads the `routable_road_edges` view.
Check the result with `GET /api/v1/roadgraph/stats` (`routable_edges` / `excluded_edges`).

## Completed milestones

| # | Spec | Status |
|---|------|--------|
| 001 | Initial data pipeline (ETL) | done |
| 002 | Go crimes API + MongoDB geospatial query (since migrated to PostGIS) | done |
| — | Road graph + edge-risk foundation (OSM import, schema, `/roadgraph/stats`) | done |

## Not yet implemented

- Safe route calculation / risk-adjusted routing (Dijkstra / A* / pgRouting over the graph)
- Edge risk scoring worker (populating `edge_risk_scores`)
- `/safe-routes` endpoint
- Frontend
- Authentication
- Aggregated statistics

## Git & version control

> **RULE — never push without an explicit instruction.** Do **not** run `git push` (or otherwise
> publish changes to the remote) unless the user explicitly asks for it in that message. Approval to
> push once does not carry over to later changes — each push needs its own go-ahead. Committing
> locally is fine when finishing a unit of work, but pushing is always the user's call.

## Development workflow

> **RULE — spec before code, always.** Before implementing **anything** — a feature, a fix, a
> refactor, any change — first create an OpenSpec change under `openspec/changes/<name>/` that records
> **what** is changing and **why**. No code until the change is written (and, for non-trivial work,
> agreed). The artifact scales with the change: a small fix can be a short `proposal.md`; a feature
> gets the full proposal + tasks + design + delta spec. The point is that every change leaves a
> written trace of its rationale.

**This project uses [OpenSpec](https://openspec.dev/) (spec-driven development).** The previous
`docs/sdd/` flow is retired; specs now live under `openspec/` (see `openspec/README.md`).

### Starting a new feature (OpenSpec)

1. **Propose** — create `openspec/changes/<change-name>/` with:
   - `proposal.md` — why, what, in/out of scope.
   - `tasks.md` — ordered implementation tasks.
   - `design.md` — technical decisions/trade-offs (when non-trivial).
   - `specs/<capability>/spec.md` — **delta spec** with `## ADDED`, `## MODIFIED`, and/or
     `## REMOVED Requirements` (each `### Requirement:` has at least one `#### Scenario:` in
     GIVEN/WHEN/THEN form).
2. **Review & agree** on the proposal before writing code.
3. **Implement** the tasks following the layer rules below; keep `go build ./...` and `go test ./...`
   green.
4. **Archive** — merge the delta into `openspec/specs/`, then move the change folder to
   `openspec/changes/archive/<YYYY-MM-DD>-<change-name>/` so `specs/` always reflects current state.

`openspec/specs/` is the source of truth for current, implemented behavior; `openspec/project.md`
holds shared context (stack, architecture, conventions) for proposals.

For design questions or architecture trade-offs before coding, invoke the `senior-backend-architect`
subagent directly. The spec-* subagents may assist within the OpenSpec flow (e.g. drafting a
proposal, planning tasks, reviewing a diff), but the OpenSpec artifacts above — not `docs/sdd/` — are
the deliverables.

### Subagent reference

| Agent | When to use |
|---|---|
| `spec-analyst` | Ambiguous requirements — turn a vague idea into a concrete spec |
| `spec-architect` | System design, new domain/module, API contract decisions |
| `spec-planner` | Break a spec into ordered tasks before implementation |
| `spec-developer` | Implement a well-defined spec or task |
| `spec-tester` | Generate test suites for completed code |
| `spec-reviewer` | Code review pass on a PR or diff |
| `spec-validator` | Quality scoring; call after implementation to get a 0–100 score |
| `spec-orchestrator` | Multi-phase coordination when running agents manually |
| `senior-backend-architect` | Production-grade Go patterns, performance, observability |
| `refactor-agent` | Improve structure/readability without changing behaviour |

### Layer rules

Every new domain must follow the existing layered structure — no shortcuts:

```
handler → service → repository interface → concrete repository (Mongo / Postgres) → datastore
```

New domains live under `internal/<domain>/` with the same file split: `model.go`, `dto.go`, `repository.go`, `<store>_repository.go`, `service.go`, `handler.go`, plus `*_test.go` files. Register routes in `internal/app/routes.go`.
