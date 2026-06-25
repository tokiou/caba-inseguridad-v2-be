# Add Redis-backed rate limiting and route cache

## Why

`/routes/safe` is now gated by auth, but every endpoint is still unbounded: one client — a buggy
frontend, an abusive script — can issue unlimited requests. The expensive endpoints hurt the most:
`/routes/safe` runs several `pgr_dijkstra` + a `pgr_ksp` query with risk joins per call, so a burst
can saturate PostgreSQL/PostGIS/pgRouting and degrade the service for everyone. We need to bound the
request rate per client, and we want to **cache** the expensive `/routes/safe` result so repeated
identical requests don't re-run the routing engine.

Both needs point at the same infrastructure — **Redis** — which also makes the limits and the cache
work across multiple API instances (a single in-process counter would not). Introducing Redis now
also realizes the route cache that the safe-route design documented but deferred, and gives us a way
to **measure** each layer: we want to benchmark the API with k6 across well-defined modes (baseline,
rate-limit only, cache only, both) without editing code.

## What changes

1. **Redis runtime** — a Redis client created at startup when `REDIS_ENABLED=true` (ping-or-fail),
   a new `internal/platform/redis` constructor mirroring `internal/platform/postgres`, new config
   (`REDIS_ENABLED`, `REDIS_ADDR`, `REDIS_PASSWORD`, `REDIS_DB`) and feature flags
   (`RATE_LIMIT_ENABLED`, `ROUTE_CACHE_ENABLED`). A `redis` service is added to docker-compose.
2. **Rate limiting** (`internal/ratelimit/`) — distributed, per-endpoint rate limiting via
   `github.com/ulule/limiter/v3` with a Redis store, applied as chi middleware **before** the
   handler: `/routes/safe` 10/min, `/auth/login` 5/min, `/crimes/nearby` 30/min,
   `/roadgraph/stats` 60/min. Each endpoint gets its own Redis key prefix so limits never bleed
   across endpoints. Over the limit returns `429` with the library's rate-limit headers; handlers
   are untouched. Keyed by **client IP**.
3. **Route cache** (`internal/saferoutes/`) — a `RouteCache` seam with a Redis implementation and a
   no-op implementation. With `ROUTE_CACHE_ENABLED=true` the service serves a cached
   `SafeRoutesResponse` for an identical (origin, destination, time bucket, weekday type, active
   model) request instead of recomputing; otherwise it injects the no-op and always computes.
4. **Feature-flag matrix** — `RATE_LIMIT_ENABLED` / `ROUTE_CACHE_ENABLED` both require
   `REDIS_ENABLED`; invalid combinations fail fast at startup with a clear message. The flags select
   five benchmark modes (baseline / redis-only / rate-limit / cache / cache+rate-limit) so k6 runs
   can compare layers without code changes.
5. **Tooling & docs** — k6 scripts under `scripts/k6/`, README "Rate limiting" + "Benchmark modes"
   sections, and `.env.example` entries.

New dependencies: `github.com/ulule/limiter/v3`, `github.com/redis/go-redis/v9` (the client ulule's
store actually requires — **not** the `go-redis/v8` named in the source spec, see design), and
`github.com/alicebob/miniredis/v2` (test-only).

## In scope

- Redis client lifecycle + config + docker-compose `redis` service.
- Per-endpoint **IP** rate limiting on the four endpoints, `429` + headers, per-endpoint key
  isolation, behind `RATE_LIMIT_ENABLED`.
- Redis route cache for `/routes/safe` behind a swappable interface (Redis / no-op), behind
  `ROUTE_CACHE_ENABLED`.
- Startup config validation for the flag matrix; fail-fast when a feature needs Redis but it's off.
- Unit tests (middleware allow/block via miniredis, prefix isolation, cache get/set + key, flag
  validation, Redis-down failure) and k6 scripts.
- README + `.env.example` updates.

## Out of scope

- **Per-user (post-auth) rate limiting.** This iteration is **by IP only**. The source spec is
  explicit ("En esta iteración usar rate limit por IP"); per-user keying is the documented next
  step (it requires placing the limiter after the auth middleware or a custom key getter). The
  branch is named for both; only IP ships now.
- GCRA / token-bucket / custom Lua sliding-window — `ulule/limiter` fixed-window is the chosen first
  implementation.
- Trusting `X-Forwarded-For` — the IP comes from the connection; forwarded headers are honored only
  behind a trusted reverse proxy, configured later.
- Redis-backed token/jti revocation, caching endpoints other than `/routes/safe`, and cache
  invalidation on model activation (TTL-only for now).
- An `api` service in docker-compose (the API still runs on the host via `go run`).
