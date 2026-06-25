# Design — Redis-backed rate limiting and route cache

## Deviations from the source spec (deliberate)

The source spec is followed in intent; three structural choices diverge to fit the existing
codebase, each noted here.

1. **Redis client = `github.com/redis/go-redis/v9`, not `go-redis/v8`.** The spec imports
   `github.com/go-redis/redis/v8`, but `ulule/limiter/v3@v3.11.2`'s redis store driver
   (`drivers/store/redis`) is built against `github.com/redis/go-redis/v9` (its `go.mod` requires
   `v9.0.4`). `redisstore.NewStoreWithOptions` takes a `*redis.Client` from the **v9** package; a v8
   client does not satisfy it. We use v9 (`v9.21.0`).
2. **Config lives in `internal/config`, not `internal/ratelimit/config.go`.** The project
   centralizes all env loading in `internal/config/config.go` (the layer rule, and how auth added
   its config). We extend that struct instead of a package-local `RedisConfigFromEnv` + bespoke
   `getenv`, exactly as the spec's own note ("Usar el helper de env que ya exista en el proyecto")
   anticipates.
3. **Redis client constructor lives in `internal/platform/redis`, not `internal/ratelimit/redis.go`.**
   Both the limiter *and* the route cache need a Redis client; putting it in `ratelimit` would make
   the cache depend on the rate-limit package. Instead it mirrors `internal/platform/postgres/pool.go`
   (`NewPool`) — infrastructure clients live under `internal/platform/`.

## Module layout

```
internal/platform/redis/client.go   NewClient(ctx, addr, password, db) (*redis.Client, error) — ping-or-fail
internal/ratelimit/
  policies.go    rate-format constants (10-M, 5-M, 30-M, 60-M) + per-endpoint key prefixes
  middleware.go  NewMiddleware(client, rate, prefix) (func(http.Handler) http.Handler, error);
                 Middlewares bundle (one per endpoint) + Passthrough() identity bundle
  *_test.go      allow/block via miniredis, prefix isolation, bad-rate error
internal/saferoutes/
  cache.go       RouteCache interface + NoopRouteCache + cache-key builder + TTL
  cache_redis.go RedisRouteCache (JSON marshal, best-effort, logs its own errors)
  cache_test.go  noop miss, redis get/set round-trip (miniredis), key determinism
```

Config + wiring changes: `internal/config/config.go`, `internal/app/app.go` (Redis lifecycle,
flag validation, build limiters + cache, inject), and the four handler constructors.

## Rate limiting

**Library.** `ulule/limiter/v3` with `drivers/store/redis` and `drivers/middleware/stdlib`. A rate is
parsed from the `"N-M"` format (`10-M` = 10/minute). `mhttp.NewMiddleware(instance).Handler` is a
`func(http.Handler) http.Handler`, so it drops straight into chi via `r.With(...)`.

**Per-endpoint isolation.** One `limiter.Store` per endpoint, each with its own `Prefix`, so counters
never share state:

| Endpoint | Rate | Redis key prefix |
|---|---:|---|
| `GET /routes/safe` | `10-M` | `safe-routes:ratelimit:routes-safe` |
| `POST /auth/login` | `5-M` | `safe-routes:ratelimit:auth-login` |
| `GET /crimes/nearby` | `30-M` | `safe-routes:ratelimit:crimes-nearby` |
| `GET /roadgraph/stats` | `60-M` | `safe-routes:ratelimit:roadgraph-stats` |

Only `/roadgraph/stats` is limited, not `/roadgraph/route`; only `/auth/login`, not the other
`/auth/*` endpoints.

**Where the middleware is applied.** The codebase registers routes via the `Registrar` interface —
each handler owns its `Register(chi.Router)`. Rather than centralize (chi can't add middleware to a
route after registration, and one registrar can hold routes with different limits, e.g. auth), we
follow the **existing auth precedent**: the auth handler already receives a
`func(http.Handler) http.Handler` and applies it to a single route (`r.With(h.middleware).Get("/auth/me", …)`).
Each rate-limited handler gains one injected middleware and wraps exactly its target route:

```go
func (h *Handler) Register(r chi.Router) {
    r.With(h.rateLimit).Get("/crimes/nearby", h.GetNearby)
}
```

The handler stays ignorant of *what* the middleware does (it's a generic `func(http.Handler) http.Handler`).

**Enabled vs disabled.** When `RATE_LIMIT_ENABLED=false`, `app.New` injects `ratelimit.Passthrough()`
— identity middlewares (`func(next) http.Handler { return next }`). No Redis-backed limiter is
constructed. `r.With(identity).Get(...)` is behaviorally identical to an unwrapped route, so handler
code stays uniform (matching how auth always wraps `/auth/me`). This satisfies the spec's "no rate
limit middleware if disabled" while avoiding an `if` in every `Register`.

**Key = client IP.** `ulule/limiter`'s default `GetIPKey` derives the key from the request IP
(`RemoteAddr`), **not** trusting `X-Forwarded-For` by default. We keep that secure default; honoring
forwarded headers is deferred until there's a trusted reverse proxy in front. (`internal/app/routes.go`
deliberately does not log client IPs; the limiter uses them only as an opaque counter key.)

**Response.** The stdlib middleware writes `429 Too Many Requests` and the rate-limit headers
(`X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, and `Retry-After`) itself —
handlers are never reached and are not modified.

**Middleware order.** chi global middleware (RequestID → CORS → logging → recovery) runs first; the
per-route rate limiter runs next; then, for `/routes/safe`, the auth middleware (mounted on the
protected group); then the handler. Because this iteration keys by IP, putting the limiter before
auth is correct and means unauthenticated floods are shed before any auth work. (A future per-user
limiter would move after auth.)

## Route cache

**Seam.** The safe-routes service depends on an interface so it never reads env vars:

```go
type RouteCache interface {
    Get(ctx context.Context, key string) (*SafeRoutesResponse, bool, error)
    Set(ctx context.Context, key string, value SafeRoutesResponse, ttl time.Duration) error
}
```

`app.New` injects `RedisRouteCache` when `ROUTE_CACHE_ENABLED=true`, else `NoopRouteCache` (always
miss, set is a no-op).

**Where it sits in the flow.** The service resolves the time bucket / weekday type and looks up the
active model (cheap, indexed), builds the key, and checks the cache **before** the expensive work
(snap + three `pgr_dijkstra` + `pgr_ksp` + risk aggregation + time-of-day). On a hit it returns the
stored response; on a miss it computes and `Set`s with a 1-hour TTL. The active-model id is part of
the key, so when a new model is activated old entries simply never match (no explicit invalidation).

**Key shape.**

```
route:safe:{originLat:.5f}:{originLng:.5f}:{destLat:.5f}:{destLng:.5f}:{timeBucket}:{weekdayType}:{modelID}
```

Coordinates are rounded to 5 decimals (~1.1 m) so byte-identical requests (and k6's fixed coords) hit;
`datetime` is intentionally coarsened to its `(time_bucket, weekday_type)` — the inputs the scores
actually depend on. **Tradeoff:** two requests in the same bucket/weekday with slightly different
coords or timestamps may share a cached entry, so the echoed `datetime`/`origin`/`destination`
reflect the first computation. This is acceptable for an exposure estimate and ideal for benchmarking;
a future edge-based key (snap first, key on edge ids) would tighten it.

**Best-effort, fail-open.** Redis errors at request time must not fail the request. `RedisRouteCache`
logs Get/Set errors at warn and reports a miss / no-op; the service treats any `Get` error as a miss
and proceeds to compute. Startup already pinged Redis (ping-or-fail), so request-time errors mean a
transient blip, not misconfiguration. The Noop and Redis caches share this fail-open contract.

## Redis runtime & feature flags

**Lifecycle.** When `REDIS_ENABLED=true`, `app.New` builds the client and `Ping`s it; a failed ping
returns an error and the server does not start (fail-fast). When `REDIS_ENABLED=false` the client is
`nil` and no connection is attempted. `App.Close` closes the client if present.

**Validation (fail-fast, clear message).** Checked in `app.New` before touching Redis:

```
RATE_LIMIT_ENABLED=true  requires REDIS_ENABLED=true
ROUTE_CACHE_ENABLED=true requires REDIS_ENABLED=true
```

→ `invalid config: RATE_LIMIT_ENABLED=true requires REDIS_ENABLED=true` (and the analogous cache
message), mirroring the existing `validateAuthConfig` pattern.

**Defaults.** All three flags default to `false` in code, so a fresh `go run ./cmd/api` with no `.env`
needs no Redis (the baseline benchmark mode). `.env.example` ships them `true` (with `REDIS_ENABLED=true`)
as the normal dev config, consistent with docker-compose now providing Redis.

**Benchmark modes** (emergent from the flags — no code per mode):

| Mode | `REDIS_ENABLED` | `RATE_LIMIT_ENABLED` | `ROUTE_CACHE_ENABLED` |
|---|---|---|---|
| baseline | false | false | false |
| redis-only | true | false | false |
| rate-limit | true | true | false |
| cache | true | false | true |
| cache + rate-limit | true | true | true |

## docker-compose

A `redis:7-alpine` service (`--appendonly yes`, `redis_data` volume, host `6379:6379`) is added next
to `postgres`. There is **no `api` service** in compose (the API runs on the host via `go run`), so
the API reaches Redis at `localhost:6379` — `.env.example` uses that. The spec's `REDIS_ADDR: redis:6379`
applies only if/when the API is containerized.

## Testing

- `internal/ratelimit`: `miniredis` (in-process Redis, supports the EVAL/Lua the store uses) drives
  real allow-then-block behavior without an external server; plus prefix-isolation and
  invalid-rate-format error tests.
- `internal/platform/redis`: `NewClient` against an unreachable address returns an error
  (Redis-down → controlled failure).
- `internal/saferoutes`: `NoopRouteCache` always misses; `RedisRouteCache` get/set round-trip via
  miniredis; key builder is deterministic; a service test with a fake cache asserts a hit skips the
  routing repository calls and a miss computes then stores.
- `internal/config` / `internal/app`: flag-matrix validation (`RATE_LIMIT_ENABLED`/`ROUTE_CACHE_ENABLED`
  without `REDIS_ENABLED` → error; valid combos → nil).
- k6 (`scripts/k6/`): `rate_limit.js` (logs in, fires 12 requests at `/routes/safe`, asserts a `429`
  appears after the 10th) and `benchmark_safe.js` (sustained load for comparing modes; no 429
  assertion).
