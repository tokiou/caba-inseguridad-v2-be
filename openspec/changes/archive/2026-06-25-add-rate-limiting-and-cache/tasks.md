# Tasks — Redis-backed rate limiting and route cache

## 1. Spec
- [x] 1.1 Write delta `specs/rate-limiting/spec.md` (distributed store, per-endpoint limits + key
      isolation, IP key, 429 + headers, toggle, middleware-before-handler).
- [x] 1.2 Write delta `specs/route-cache/spec.md` (seam interface, toggle → noop, hit/miss + key +
      TTL, fail-open).
- [x] 1.3 Write delta `specs/redis-runtime/spec.md` (client lifecycle ping-or-fail, flag validation,
      benchmark-mode matrix).
- [x] 1.4 `openspec validate add-rate-limiting-and-cache --strict` passes.

## 2. Dependencies & config
- [x] 2.1 `go get github.com/ulule/limiter/v3@v3.11.2 github.com/redis/go-redis/v9` +
      `github.com/alicebob/miniredis/v2` (test). NOTE: ulule's store requires go-redis **v9**, not
      the **v8** named in the source spec — see design.md.
- [x] 2.2 Extend `internal/config/config.go`: `RedisEnabled`, `RedisAddr`, `RedisPassword`, `RedisDB`,
      `RateLimitEnabled`, `RouteCacheEnabled`; flag defaults `false`. (Config centralized here, not in
      `internal/ratelimit/config.go`, per project convention — see design.md.)
- [x] 2.3 Add `.env.example` entries (Redis + feature flags), set to the cache+rate-limit dev config.

## 3. Redis platform client
- [x] 3.1 `internal/platform/redis/client.go` — `NewClient(ctx, addr, password, db)` v9 client +
      `Ping`-or-fail (mirrors `postgres.NewPool`). (In `internal/platform/redis`, shared by limiter +
      cache, not `internal/ratelimit/redis.go` — see design.md.)
- [x] 3.2 `client_test.go` — unreachable address → error (Redis-down → controlled failure).

## 4. Rate limiting — `internal/ratelimit/`
- [x] 4.1 `policies.go` — rate constants + the four key prefixes.
- [x] 4.2 `middleware.go` — `NewMiddleware`; `Middlewares` bundle via `NewMiddlewares`; `Passthrough()`.
- [x] 4.3 `middleware_test.go` — allow→429 (miniredis); distinct prefixes; per-IP; invalid rate; bundle.

## 5. Route cache — `internal/saferoutes/`
- [x] 5.1 `cache.go` — `RouteCache`, `NoopRouteCache`, `routeCacheKey`, `routeCacheTTL`.
- [x] 5.2 `cache_redis.go` — `RedisRouteCache` (JSON, fail-open, logs own errors).
- [x] 5.3 `service.go` — inject `RouteCache`; check after model lookup, store on miss. `NewService(repo, cache)`.
- [x] 5.4 `cache_test.go` — noop miss; redis round-trip; key determinism; hit skips routing, miss
      computes + stores (fake cache).

## 6. Wiring — `internal/app/app.go`
- [x] 6.1 `validateRedisConfig(cfg)` — the two flag-matrix rules with the exact error messages.
- [x] 6.2 Build the Redis client when `REDIS_ENABLED` (stored on `App`, closed on shutdown).
- [x] 6.3 Build limiters (`NewMiddlewares` / `Passthrough()`); build cache (Redis / Noop).
- [x] 6.4 Inject per-endpoint limiter into `crimes`/`roadgraph`/`saferoutes`/`auth` handlers; cache
      into the saferoutes service. `app_test.go` covers flag-matrix validation.

## 7. Handlers apply the middleware
- [x] 7.1 `crimes.NewHandler(..., rateLimit)` → `r.With(h.rateLimit).Get("/crimes/nearby", …)`.
- [x] 7.2 `roadgraph.NewHandler(..., rateLimit)` → wrap only `/roadgraph/stats`.
- [x] 7.3 `saferoutes.NewHandler(..., rateLimit)` → wrap `/routes/safe`.
- [x] 7.4 `auth.NewHandler(..., loginRateLimit)` → wrap only `/auth/login`.
- [x] 7.5 Updated existing handler unit tests for the new constructor arg (passthrough).

## 8. Infra, tooling, docs
- [x] 8.1 `docker-compose.yml` — add `redis:7-alpine` service + `redis_data` volume + healthcheck.
- [x] 8.2 `k6/rate_limit.js` (login → 12× `/routes/safe`, assert 429) and `k6/benchmark_safe.js`.
      NOTE: under `k6/`, not `scripts/k6/` — `scripts/` is root-owned in the dev env (see design.md).
- [x] 8.3 Rate-limiting + benchmark-modes docs: `k6/README.md` (+ in CLAUDE.md). NOTE: repo has no
      root README; CLAUDE.md is the canonical project doc, so the sections went there + `k6/README.md`.
- [x] 8.4 Updated `CLAUDE.md` (env vars, `internal/ratelimit` + `internal/platform/redis`, route
      cache, docker-compose redis, milestones / not-yet-implemented) and `openspec/project.md`.

## 9. Verify
- [x] 9.1 `go mod tidy`, `go build ./...`, `go vet ./...` clean.
- [x] 9.2 `go test ./...` green (incl. miniredis-backed limiter/cache + flag-matrix tests).
- [x] 9.3 Manual (live, real Redis + Postgres): `/roadgraph/stats` → 60×200 then 429 at request #61;
      `/crimes/nearby` returns `X-RateLimit-Limit: 30`; un-limited `/health` stays 200; invalid-config
      and unreachable-Redis both fail startup with a clear error.
