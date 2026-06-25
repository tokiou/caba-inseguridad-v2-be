# Design — benchmark suite + runtime stats endpoint

## Two measurement layers

Client-side load (k6) alone can't see why a mode is faster. We pair it with a server-side stats
endpoint so every run captures both:

- **Client (k6)** — latency p50/p90/p95/p99, throughput (RPS), error rate, 429s; split by `X-Cache`
  tag where relevant.
- **Server (`/debug/stats`)** — pgxpool saturation and cache hit-rate, snapshotted before/after each
  run and diffed.

The "which wins" answer for cache: a **hit** run should show much lower p95 **and** near-flat pgxpool
pressure (`EmptyAcquireCount`, `AcquiredConns` barely move — the DB isn't touched); a **miss** run
should look like baseline plus a small `SET`. You only see that by crossing both layers.

## Metrics and their source

| Metric | Source | Field |
|---|---|---|
| latency p50/p90/p95/p99 | k6 | `http_req_duration` |
| throughput (RPS) | k6 | `http_reqs` |
| error rate, 429 | k6 | `http_req_failed`, status checks |
| hit vs miss latency | k6 + `X-Cache` | request tag |
| pgxpool pressure | `/debug/stats` | `AcquiredConns`, `IdleConns`, `TotalConns`, `MaxConns`, `AcquireCount`, `AcquireDuration`, `EmptyAcquireCount`, `CanceledAcquireCount`, `NewConnsCount` |
| cache effectiveness | `/debug/stats` | `hits`, `misses`, `errors`, `sets`, `hit_rate` |
| DB pressure (optional) | psql | `pg_stat_activity` active/waiting counts |

## `/debug/stats` endpoint

`GET /api/v1/debug/stats` → JSON `{ uptime_seconds, goroutines, pgxpool{...}, cache{...} }`. It is an
introspection endpoint, so — like `health` — it is a thin handler with no service/repository layer; it
reads `*pgxpool.Pool.Stat()` and the cache's counters directly.

**Gating (it leaks internals):**

- Registered only when `METRICS_ENABLED=true` (config flag, default false). When off, the route does
  not exist → 404.
- The handler additionally rejects non-loopback clients (`127.0.0.1` / `::1`) with 403, so even if
  enabled it isn't reachable off-box. Sufficient for a local dev/bench tool; no auth.

When the route cache is the no-op (cache disabled), the `cache` block reports `enabled: false` and
zeroed counters.

## Cache instrumentation

- `RedisRouteCache` gains four `atomic.Int64` counters (hits, misses, errors, sets) incremented in
  `Get`/`Set`, plus `Stats() CacheStats`. A small `CacheStatsProvider` interface
  (`Stats() CacheStats`) lets the debug handler read them without importing Redis. `NoopRouteCache`
  is not a provider (cache off → handler reports disabled).
- **`X-Cache` header.** The service must tell the handler whether a response was a hit. We add a
  `FromCache bool` field to `SafeRoutesResponse` tagged `json:"-"` (never serialized, never stored in
  the cached JSON), set to `true` right after a cache hit. The handler maps it to
  `X-Cache: hit|miss`. This keeps the cache logic in the service and the header in the handler, and
  avoids a signature change.

## Wiring

`app.New` already holds the pool and builds the cache. When `MetricsEnabled`, it constructs the
observability handler with the pool and — if the cache is a `CacheStatsProvider` — that provider, and
registers it (public group, but self-gated by loopback). When the cache is the no-op, the provider is
nil and the endpoint reports the cache disabled.

## Benchmark harness (`bench/`)

```
bench/
  k6/
    lib/config.js        # BASE, creds, coord helpers (fixed vs unique-per-iter)
    lib/auth.js          # login() helper returning the access token
    01_no_redis.js       # baseline: cache off, varied coords
    02_cache_hit.js      # cache on, FIXED coords → all hits after warmup
    03_cache_miss.js     # cache on, UNIQUE coords per iter → all misses
    04_auth_flow.js      # register→login→me→safe→refresh→logout, checks per step
    05_login_ratelimit.js# POST /auth/login in a burst → assert 5 then 429
    06_token_revocation.js # login→logout→refresh=401; refresh reuse=401
    07_pool_exhaustion.js# ramp VUs ≫ pool_max_conns, cache off, DB-bound
  run.sh                 # per mode: set env, (re)start API, warmup, k6, snapshot, teardown
  snapshot_stats.sh      # curl /debug/stats → json (+ optional pg_stat_activity)
  results/<ISO-ts>/      # k6 handleSummary json + server before/after + comparison.md
  README.md
```

`run.sh` builds the binary once, then per scenario sets the mode's env (the five modes already exist),
waits on `/health`, snapshots `/debug/stats`, runs the k6 script (`--summary-export` via
`handleSummary`), snapshots again, kills the API, and writes the diff. It registers the bench user
idempotently (POST `/auth/register`, ignore 409). Committing `results/` makes runs diffable across
commits (regression tracking).

## The 7 scenarios

| # | Scenario | Mode / setup | Proves | Key metric | Pass |
|---|---|---|---|---|---|
| 1 | Sin Redis | baseline, varied coords | raw DB cost (control) | p95, RPS, `AcquiredConns` | the reference |
| 2 | Cache hit | cache on, fixed coords | cache-served path | p95 hit, flat pool | p95 ≪ #1, `EmptyAcquire≈0` |
| 3 | Cache miss | cache on, unique coords | miss overhead | p95 miss vs #1 | ≈ #1 + set |
| 4 | Auth flow | full chain | e2e latency + rotation | p95/step, checks | rotates, all 2xx |
| 5 | Login rate limit | burst POST /auth/login | limiter sheds load | #429, 429 latency | 5 ok → 429; 429 ≪ login |
| 6 | Token revocation | login→logout→refresh; reuse | revocation + rotation | refresh status | post-logout 401; reuse 401 |
| 7 | Pool exhaustion | small `pool_max_conns`, high VUs, cache off | degradation under saturation | `EmptyAcquireCount`, `AcquireDuration` p99 | bounded, no crash, clear error |

**Scenario 6 caveat (by design, not a bug):** access tokens are stateless — logout revokes the
*refresh session*, not the access token (it stays valid until its 15-min `exp`). The scenario asserts
**refresh** fails after logout and that a rotated (reused) refresh token is rejected — it does **not**
assert the access token dies instantly.

**Scenario 7 setup:** force saturation via the DSN, e.g.
`DATABASE_URL=...?pool_max_conns=5`, ramp VUs well above 5 against an uncached `/routes/safe`
(baseline) or `/crimes/nearby`. Watch `EmptyAcquireCount` and `AcquireDuration` climb. The expected
finding is bounded degradation (waits + latency) rather than a crash; if latency grows unbounded, that
*justifies* adding an acquire/query timeout (out of scope here).

## Rigor

Warmup discarded, ≥3 runs per cell, fixed dataset and coords, same host, report percentiles (not
averages). `run.sh` does a warmup pass before the measured run and records the mode + git SHA in each
result dir.
