# Tasks — benchmark suite + runtime stats endpoint

## 1. Spec
- [x] 1.1 Write delta `specs/observability/spec.md` (stats endpoint gated+loopback; cache
      instrumentation: counters + `X-Cache`).
- [x] 1.2 `openspec validate add-benchmark-suite --strict` passes.

## 2. Config
- [x] 2.1 `internal/config/config.go` — add `MetricsEnabled` (`METRICS_ENABLED`, default false).
- [x] 2.2 `.env.example` — add `METRICS_ENABLED` with a note (loopback-only, internals).

## 3. Cache instrumentation — `internal/saferoutes/`
- [x] 3.1 `cache.go` — `CacheStats` + `CacheStatsProvider`; `dto.go` — `FromCache bool json:"-"`.
- [x] 3.2 `cache_redis.go` — atomic hits/misses/errors/sets counters + `Stats() CacheStats`.
- [x] 3.3 `service.go` — set `FromCache=true` on a cache hit.
- [x] 3.4 `handler.go` — set `X-Cache: hit|miss` from `response.FromCache`.
- [x] 3.5 Tests — `Stats()` counters (miniredis); `X-Cache` header via the stub service.

## 4. Stats endpoint — `internal/observability/`
- [x] 4.1 `handler.go` — `GET /api/v1/debug/stats`: pgxpool + cache + uptime/goroutines; loopback-only
      (403 otherwise); decoupled via plain-struct closures; `Register(r)`.
- [x] 4.2 `handler_test.go` — non-loopback → 403; loopback → 200 with shape; cache-disabled reported.

## 5. Wiring — `internal/app/app.go`
- [x] 5.1 `newObservabilityHandler` adapts `pool.Stat()` + cache (via `CacheStatsProvider`) into the
      handler's closures; registered (public, self-gated) only when `MetricsEnabled`.

## 6. Benchmark harness — `bench/`
- [x] 6.1 `bench/k6/lib/{config,auth,summary}.js` — coords/creds, login helper, shared handleSummary.
- [x] 6.2 `bench/k6/01..07_*.js` — the seven scenarios.
- [x] 6.3 `bench/run.sh` — build once; per scenario set mode env, (re)start API, snapshot, run k6,
      snapshot, teardown; idempotent register; scenario 7 appends `pool_max_conns=5`.
- [x] 6.4 `bench/snapshot_stats.sh` — curl `/debug/stats` → json (+ optional `pg_stat_activity`).
- [x] 6.5 `bench/results/.gitkeep` + `bench/README.md` (run, modes, scenarios, how to read results).
- [x] 6.6 Removed the old top-level `k6/` (superseded by `bench/`); `bash -n` clean on the scripts.

## 7. Docs
- [x] 7.1 `CLAUDE.md` (observability, `/debug/stats` + `METRICS_ENABLED`, `bench/`, benchmark note) +
      `openspec/project.md` (observability capability).

## 8. Verify
- [x] 8.1 `go build ./...`, `go vet ./...`, `go test ./...` green.
- [x] 8.2 Live (real Redis + Postgres): `/debug/stats` returns pgxpool + cache; `/routes/safe` twice →
      `X-Cache: miss` then `hit`; cache counters then read `hits:1 misses:1 sets:1 hit_rate:0.5`.
- [x] 8.3 `bash -n` on `run.sh`/`snapshot_stats.sh` clean (full k6 run is a manual step — needs k6).
