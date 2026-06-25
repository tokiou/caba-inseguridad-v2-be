# CLAUDE.md — caba-inseguridad-v2-be

## Project

Backend for **CABA Rutas Seguras**: given two points in Buenos Aires (CABA), return a route that minimizes exposure to crime hotspots. Crime data comes from open CABA datasets loaded into PostgreSQL + PostGIS.

## Stack

- **Go 1.25** — HTTP API (`github.com/tokiou/caba-inseguridad-routes-go`)
- **Python 3** — ETL pipeline under `etl/python/`
- **PostgreSQL + PostGIS** — geospatial crime data (`geom GEOMETRY(Point,4326)`, GiST index)
- **pgx (jackc/pgx v5)** — Postgres driver + pool; **sqlc** generates the relational CRUD (auth)
- **chi** — HTTP router
- **golang-jwt/v5** + **bcrypt** — access-token signing + password hashing (auth)
- **Redis (go-redis v9)** + **ulule/limiter v3** — distributed per-endpoint rate limiting and the
  `/routes/safe` route cache; both opt-in via env flags (see Rate limiting & caching)
- **godotenv** — env loading

> **Data access rule:** **pgx** owns all PostGIS / geospatial queries (raw SQL); **sqlc** owns
> relational CRUD (introduced with the auth capability — `internal/auth/`). sqlc's analyzer does
> not understand PostGIS functions, so its schema points only at the auth migration and geospatial
> queries stay on pgx. Regenerate with `go generate ./internal/auth/...` (config: `sqlc.yaml`).

## Repository layout

```
cmd/api/main.go                   # entrypoint: wires deps, starts server
internal/
  app/app.go                      # App struct (router + pgx pool + optional Redis); flag validation + wiring
  app/routes.go                   # chi route registration
  config/config.go                # Config struct loaded from env
  platform/postgres/pool.go       # pgx connection pool + ping
  platform/redis/client.go        # go-redis v9 client + ping (shared by ratelimit + route cache)
  ratelimit/                      # distributed per-endpoint rate limiting (ulule/limiter + Redis)
    policies.go                   # rate constants (10-M/5-M/30-M/60-M) + per-endpoint key prefixes
    middleware.go                 # NewMiddleware factory + Middlewares bundle + Passthrough (disabled)
    middleware_test.go            # allow/block + prefix isolation via miniredis
  observability/handler.go        # GET /api/v1/debug/stats (pgxpool + cache + runtime); gated, loopback-only
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
    repository.go                 # Repository interface + PostgresRepository (counts + ST_Extent bbox + walk route)
    service.go                    # status orchestration
    handler.go                    # GET /api/v1/roadgraph/stats
    service_test.go / handler_test.go
    repository_integration_test.go # //go:build integration
  saferoutes/                     # GET /api/v1/routes/safe — risk-weighted walking routes
    model.go / dto.go / errors.go # route alternatives, metrics, sentinel errors
    repository.go                 # Repository interface (routing-engine seam) + PostgresRepository (pgr_dijkstra / pgr_ksp, risk-weighted cost)
    risk_aggregation.go           # route risk = 0.75·weighted_avg + 0.25·max, levels, comparisons
    cache.go                      # RouteCache seam + NoopRouteCache + cache-key builder + TTL
    cache_redis.go                # RedisRouteCache (JSON, fail-open) — checked before routing
    service.go                    # validation, time-bucket/weekday resolution, profile orchestration, cache
    handler.go                    # parsing + error mapping (400/404/503/500); rate-limit mw injected
    *_test.go                     # unit + //go:build integration tests
  auth/                           # /api/v1/auth/* — accounts + JWT auth, gates /routes/safe
    model.go / dto.go / errors.go # User/session models, request/response DTOs, sentinel errors
    token.go                      # JWT mint/verify (HS256) + opaque refresh + sha256 hashing
    repository.go                 # Repository interface + PostgresRepository (wraps sqlc authdb)
    service.go                    # register/login/refresh/logout/authenticate, bcrypt, rotation
    handler.go                    # 5 endpoints + refresh cookie + error mapping
    middleware.go                 # bearer-token middleware + WithUser/UserFromContext
    gen.go                        # //go:generate sqlc directive
    db/                           # sqlc-generated package authdb (queries/ holds auth.sql)
    *_test.go                     # unit (token/service/handler/middleware) + integration
  httpx/response.go               # shared JSON helpers
sqlc.yaml                         # sqlc config (auth relational CRUD only)
bench/                            # benchmark suite (here, not scripts/, which is root-owned locally)
  k6/01..07_*.js                  # 7 scenarios (cache hit/miss, no-redis, auth flow, rate limit, …)
  k6/lib/                         # shared: config (coords), auth (login), summary (handleSummary)
  run.sh / snapshot_stats.sh      # orchestrator (boots API per mode, snapshots /debug/stats) + snapshot
  results/                        # committed k6 summaries + server-stat diffs (diffable per commit)
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
etl/risk_network_kde/             # offline Network Temporal KDE risk pipeline (see Risk scoring)
  cli.py                          # snap-crimes / build-neighborhoods / compute-scores / evaluate /
                                  # evaluate-routes / calibrate / finalize / self-test
  config.py / db.py / utils.py    # model params, connection, time-bucket helpers (mirrored in Go)
  snap_crimes.py / graph_loader.py / build_neighborhoods.py
  compute_scores.py / evaluate.py / evaluate_routes.py / calibrate.py / finalize.py
  requirements.txt
migrations/                       # SQL migrations (000001_enable_postgis … 000011_create_auth_tables)
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

# Auth
JWT_SECRET=change_me            # required outside development (refuses to boot if empty/default)
ACCESS_TOKEN_TTL_MINUTES=15
REFRESH_TOKEN_TTL_DAYS=7
REFRESH_COOKIE_NAME=refresh_token
COOKIE_SECURE=false             # true in production (HTTPS)
COOKIE_SAMESITE=lax             # lax | strict | none

# Redis + resilience (rate limiting + route cache). Flags default to false in
# code; .env.example ships them on. RATE_LIMIT_ENABLED and ROUTE_CACHE_ENABLED
# both REQUIRE REDIS_ENABLED=true (invalid combos fail fast at startup).
REDIS_ENABLED=true
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=
REDIS_DB=0
RATE_LIMIT_ENABLED=true
ROUTE_CACHE_ENABLED=true
METRICS_ENABLED=false           # GET /api/v1/debug/stats (loopback-only); on for benchmarking
```

The Postgres container (`pgrouting/pgrouting:16-3.4-3.6.1` — PostGIS 3.4 + pgRouting 3.6 on PG16) maps
host port **5434** → container 5432 (avoids clashes with other local Postgres instances).
`POSTGRES_HOST/PORT/DB/USER/PASSWORD` are also read by docker-compose, the ETL, and the OSM import.
The `redis:7-alpine` service in docker-compose backs rate limiting + the route cache; the host-run API
reaches it at `localhost:6379` (`docker compose up -d redis`).

Copy `.env.example` to `.env` before running.

## Running

```bash
# Start the API (host)
go run ./cmd/api

# Tests (see Makefile): unit, race+goleak, coverage
make test            # go test ./...
make test-race       # -race + goroutine-leak checks (goleak)
make cover           # coverage.out + total %
make test-integration  # //go:build integration — needs a populated PostGIS DB

# Container (multi-stage → distroless static, non-root, ~32 MB)
docker build -t caba-be .
docker run --rm --network host \
  -e DATABASE_URL=postgres://postgres:postgres@localhost:5434/caba_routes?sslmode=disable \
  -e REDIS_ENABLED=true -e RATE_LIMIT_ENABLED=true -e ROUTE_CACHE_ENABLED=true \
  caba-be
```

CI (`.github/workflows/ci.yml`) runs build + vet + `go test -race -coverprofile` on
every push/PR; integration tests stay local (they need a populated dataset).

## Rate limiting & caching

Both are **Redis-backed** and **opt-in** via env flags; both require `REDIS_ENABLED=true` (an invalid
combination fails fast at startup with `invalid config: …`). Code defaults are all `false`, so a bare
`go run ./cmd/api` needs no Redis (the baseline benchmark mode).

- **Rate limiting** (`internal/ratelimit/`, `RATE_LIMIT_ENABLED`): per-endpoint, distributed, via
  `ulule/limiter` with a Redis store, applied as chi middleware **before** the handler. Over the limit
  → `429` with `X-RateLimit-*` / `Retry-After` headers (the handler is never reached). Keyed by client
  **IP** (no `X-Forwarded-For` trust by default); per-user keying is deferred. Each endpoint has its
  own Redis key prefix so quotas don't bleed:

  | Endpoint | Limit | Reason |
  |---|---:|---|
  | `GET /routes/safe` | 10/min | expensive pgRouting route calculation |
  | `POST /auth/login` | 5/min | brute-force protection |
  | `GET /crimes/nearby` | 30/min | lightweight geospatial query |
  | `GET /roadgraph/stats` | 60/min | read-only stats |

  The middleware is injected into each handler's constructor (like the auth middleware) and applied
  with `r.With(...)`; when disabled, `app.New` injects an identity passthrough.

- **Route cache** (`internal/saferoutes/cache*.go`, `ROUTE_CACHE_ENABLED`): a `RouteCache` seam
  (`RedisRouteCache` / `NoopRouteCache`). The service checks the cache after resolving
  bucket/weekday/active-model and **before** snapping/routing; hit → cached `SafeRoutesResponse`, miss
  → compute + store (1 h TTL). Key:
  `route:safe:{olat:.5f}:{olng:.5f}:{dlat:.5f}:{dlng:.5f}:{bucket}:{weekday}:{modelID}` — a model
  change naturally bypasses stale entries. Fail-open: a Redis error is logged and treated as a miss.

**Benchmark modes** (selected by the three flags, no code change): baseline `false/false/false` ·
redis-only `true/false/false` · rate-limit `true/true/false` · cache `true/false/true` ·
cache+rate-limit `true/true/true`.

**Measuring it** (`bench/`): `GET /api/v1/debug/stats` (gated by `METRICS_ENABLED`, loopback-only)
exposes pgxpool saturation + cache hit/miss counters; `/routes/safe` sets `X-Cache: hit|miss`.
`bench/run.sh [01..07]` boots the API per mode, runs a k6 scenario (latency/throughput/errors),
snapshots `/debug/stats` before/after, and writes diffable results to `bench/results/`. The 7
scenarios: sin-Redis, cache hit, cache miss, auth flow, login rate limit, token revocation, pool
exhaustion. See `bench/README.md`.

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

## Risk scoring (offline Python pipeline)

Network Temporal KDE (`network_temporal_edge_risk_v1`, deterministic, no ML): crimes snap to their
nearest routable edge, influence propagates over **walking-network distance** (never plain
`ST_DWithin`), weighted by severity / weapon / motorcycle / recency / time bucket / weekday type,
normalized per context with a p95 clamp. 9 contexts: 4 buckets × {weekday, weekend} + `all_day/all`.
Time-bucket boundaries are mirrored in Go (`internal/saferoutes/service.go`) — change both or
neither. A model only activates after passing the temporal backtest gate (train ≤ 2022 vs 2023:
PAI@Top5 ≥ 3, Recall@Top10 ≥ 0.30, TopDecileLift ≥ 3).

```bash
python -m etl.risk_network_kde.cli snap-crimes        --graph-version caba_walking_graph_v1 --max-distance-meters 80
python -m etl.risk_network_kde.cli build-neighborhoods --graph-version caba_walking_graph_v1 --bandwidth-meters 350
python -m etl.risk_network_kde.cli compute-scores      --model network_temporal_edge_risk_v1_eval_2022 --train-until 2022-12-31
python -m etl.risk_network_kde.cli evaluate            --model network_temporal_edge_risk_v1_eval_2022 --test-from 2023-01-01 --test-to 2023-12-31
python -m etl.risk_network_kde.cli evaluate-routes     --model network_temporal_edge_risk_v1_eval_2022 --test-from 2023-01-01 --test-to 2023-12-31
python -m etl.risk_network_kde.cli calibrate ...       # only if the gate fails
python -m etl.risk_network_kde.cli finalize            --base-model network_temporal_edge_risk_v1_eval_2022 --final-model network_temporal_edge_risk_v1 --train-until latest --activate
```

The Go API never computes risk: `/api/v1/routes/safe` reads `edge_risk_scores` /
`edge_risk_score_components` for the active `risk_model_versions` row and costs edges as
`length_meters * (1 + safety_multiplier * risk_score)` (profiles in `route_profiles`). Product
language: always "estimated historical exposure", never safety guarantees.

## Completed milestones

| # | Spec | Status |
|---|------|--------|
| 001 | Initial data pipeline (ETL) | done |
| 002 | Go crimes API + MongoDB geospatial query (since migrated to PostGIS) | done |
| — | Road graph + edge-risk foundation (OSM import, schema, `/roadgraph/stats`) | done |
| — | Walkable graph routing (`GET /api/v1/roadgraph/route`, pgr_dijkstra) | done |
| — | Network Temporal KDE risk scoring + `GET /api/v1/routes/safe` | done |
| — | Route explainability metadata on `/routes/safe` | done |
| — | User accounts + JWT auth (`/api/v1/auth/*`), `/routes/safe` gated | done |
| — | Redis rate limiting (per-endpoint, by IP) + `/routes/safe` route cache + benchmark flags | done |

## Not yet implemented

- ML risk models (LightGBM/XGBoost) — the schema is ready (new `risk_model_versions` rows)
- User avoid-points / community reports / saved routes
- Authorization roles (current auth only distinguishes a valid, active user)
- Per-user (post-auth) rate limiting — current rate limiting is by IP only; user keying deferred
- GCRA / sliding-window-log rate limiting (first iteration uses ulule/limiter fixed window)
- Refresh-token reuse-detection lockout (rotation + `replaced_by` recorded; family revocation deferred)
- Frontend
- Aggregated statistics

## Git & version control

> **RULE — never commit or push without an explicit instruction.** Do **not** run `git commit`,
> `git push`, or otherwise record/publish changes unless the user explicitly asks for it in *that*
> message. Leave finished work as changes in the working tree so the user can review it from their
> Git tooling; **they** decide when to commit and when to push. Approval to commit or push once does
> **not** carry over to later changes — each one needs its own go-ahead.

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

New domains live under `internal/<domain>/` with the same file split: `model.go`, `dto.go`, `repository.go` (the `Repository` interface and its concrete `PostgresRepository` live together in this one file), `service.go`, `handler.go`, plus `*_test.go` files. Register routes in `internal/app/routes.go`.
