# Add benchmark suite + runtime stats endpoint

## Why

We added rate limiting and a route cache but have no way to **measure** their impact. "Cache hit is
faster" stays a guess until we can see latency split by hit/miss **and** the pressure each mode puts
on PostgreSQL/pgxpool. We want a repeatable harness that loads the API across the runtime modes and
the key flows, captures client-side metrics (latency, throughput, errors) **and** server-side metrics
(pgxpool saturation, cache hit-rate), and stores comparable results so we can answer "with vs without
cache — which wins, and at what cost" — and catch regressions over time.

## What changes

1. **Runtime stats endpoint** — `GET /api/v1/debug/stats` returns pgxpool `Stat()` (acquired / idle /
   total / max conns, acquire count + duration, empty/canceled acquires), route-cache counters
   (hits / misses / errors / sets + hit-rate), and basic runtime (uptime, goroutines). Gated by
   `METRICS_ENABLED` and restricted to loopback — it exposes internals.
2. **Cache instrumentation** — the Redis route cache counts hits/misses/errors/sets (atomic), and
   `/routes/safe` sets an `X-Cache: hit|miss` response header so a load test can split latency by
   cache outcome without server access.
3. **Benchmark harness** (`bench/`) — k6 scripts for the 7 scenarios, a `run.sh` orchestrator (boots
   the API per mode, warms up, runs k6, snapshots `/debug/stats` before/after, tears down), a
   `snapshot_stats.sh`, and a committed `results/` tree (k6 `handleSummary` JSON + server-stat diffs +
   a generated comparison) so runs are diffable across commits. Consolidates the earlier `k6/` scripts
   here.

New config: `METRICS_ENABLED` (default `false`). **No new Go deps** (JSON endpoint, stdlib only). Pool
sizing for the exhaustion scenario uses pgx's DSN params (`pool_max_conns`) — no code.

## In scope

- `/debug/stats` JSON endpoint (gated by `METRICS_ENABLED`, loopback-only) with pgxpool + cache +
  runtime metrics.
- Cache hit/miss/error/set counters + `X-Cache` response header on `/routes/safe`.
- `bench/` harness: 7 k6 scenarios, `run.sh` orchestrator, `snapshot_stats.sh`, committed `results/`
  layout, README. Moves the existing `k6/` scripts under `bench/`.
- Reuses the five runtime modes as-is (from `add-rate-limiting-and-cache`).

## Out of scope

- **Prometheus/Grafana** — chose lightweight JSON snapshots; the endpoint shape leaves the door open.
- **pg_stat_statements** — the snapshot uses `pg_stat_activity` (no extension needed); statements are
  read only if the extension happens to be present.
- **A pool-acquire / query timeout** — scenario 7 measures the *current* saturation behavior; a
  bounded-degradation timeout is a likely follow-up the benchmark will justify, not something to
  pre-build.
- CI integration / automated regression gates (results are committed for manual diffing now).
- Authn on `/debug/stats` beyond the loopback + flag gate (it is a local dev/bench tool).
