# CLAUDE.md — caba-inseguridad-v2-be

## Project

Backend for **CABA Rutas Seguras**: given two points in Buenos Aires (CABA), return a route that minimizes exposure to crime hotspots. Crime data comes from open CABA datasets loaded into MongoDB.

## Stack

- **Go 1.23** — HTTP API (`github.com/tokiou/caba-inseguridad-routes-go`)
- **Python 3** — ETL pipeline under `etl/python/`
- **MongoDB** — geospatial crime data (`2dsphere` index on `location`)
- **chi** — HTTP router
- **godotenv** — env loading

## Repository layout

```
cmd/api/main.go                   # entrypoint: wires deps, starts server
internal/
  app/app.go                      # App struct (router + mongo client)
  app/routes.go                   # chi route registration
  config/config.go                # Config struct loaded from env
  platform/mongo/client.go        # MongoDB connect + ping
  health/handler.go               # GET /api/v1/health
  crimes/
    model.go                      # Crime + GeoJSON structs
    dto.go                        # NearbyCrimesQuery + NearbyCrimesResponse
    repository.go                 # Repository interface
    mongo_repository.go           # $nearSphere implementation
    service.go                    # business validation
    handler.go                    # HTTP parsing + JSON responses
    service_test.go
    handler_test.go
  httpx/response.go               # shared JSON helpers
etl/python/
  analyze_raw_data.py             # raw quality report
  normalize_crimes.py             # XLSX → JSONL normalization
  load_to_postgres.py             # upsert into PostgreSQL + PostGIS (active loader)
  load_to_mongo.py                # legacy MongoDB loader (kept for reference)
  requirements.txt
migrations/                       # SQL migrations (000001_enable_postgis, 000002_create_crimes)
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
HTTP request → chi router → handler → service → repository interface → MongoRepository → MongoDB
```

- **Handlers** parse HTTP params and return JSON. No MongoDB, no business logic.
- **Services** validate domain rules and call the repository. No HTTP, no MongoDB details.
- **Repositories** encapsulate all data access. No HTTP logic.
- **`main.go`** only wires dependencies and starts the server.

## Environment variables

```env
APP_ENV=development
HTTP_PORT=8080
MONGO_URI=mongodb://localhost:27017
MONGO_DATABASE=caba_routes
MONGO_CRIMES_COLLECTION=crimes
LOG_LEVEL=info          # debug | info | warn | error
LOG_FORMAT=json         # json (prod) | text (colored dev console)
```

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
python load_to_mongo.py       # upserts into MongoDB (idempotent)
```

Normalized schema includes `source_id` (unique key), `location` as GeoJSON Point with `[lng, lat]`, and boolean fields `weapon_used` / `motorcycle_used`.

## Completed milestones

| # | Spec | Status |
|---|------|--------|
| 001 | Initial data pipeline (ETL) | done |
| 002 | Go crimes API + MongoDB geospatial query | done |

## Not yet implemented

- OpenRouteService integration
- Safe route calculation / risk scoring
- Route endpoint
- Frontend
- Authentication
- Aggregated statistics

## Development workflow

**This project uses [OpenSpec](https://openspec.dev/) (spec-driven development). Write and agree on a
spec BEFORE implementing any feature.** The previous `docs/sdd/` flow is retired; specs now live under
`openspec/` (see `openspec/README.md`).

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
