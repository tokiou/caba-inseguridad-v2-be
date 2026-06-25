# Route Cache — Delta Specification

## ADDED Requirements

### Requirement: Route cache seam

The safe-routes service SHALL depend on a `RouteCache` interface — never on a concrete cache or on
environment variables — with two implementations: a Redis-backed cache and a no-op cache. The
interface SHALL expose `Get(ctx, key) (*SafeRoutesResponse, bool, error)` and
`Set(ctx, key, value, ttl) error`. `app.New` SHALL inject the Redis implementation when
`ROUTE_CACHE_ENABLED=true` (which requires `REDIS_ENABLED=true`) and the no-op implementation
otherwise.

#### Scenario: No-op cache is injected when disabled

- GIVEN `ROUTE_CACHE_ENABLED=false`
- WHEN `/routes/safe` is requested twice with identical parameters
- THEN both requests are computed against PostgreSQL/PostGIS/pgRouting (the no-op cache always
  reports a miss and stores nothing)

### Requirement: Cache hit skips recomputation

When `ROUTE_CACHE_ENABLED=true`, the service SHALL check the cache after resolving the time bucket,
weekday type, and active model but **before** snapping/routing/aggregating. On a hit it SHALL return
the cached `SafeRoutesResponse` without calling the routing repository; on a miss it SHALL compute the
response and store it with a 1-hour TTL. The cache key SHALL be
`route:safe:{originLat:.5f}:{originLng:.5f}:{destLat:.5f}:{destLng:.5f}:{timeBucket}:{weekdayType}:{modelID}`,
so a change of active model naturally bypasses stale entries.

#### Scenario: Second identical request is served from cache

- GIVEN `ROUTE_CACHE_ENABLED=true` and a `/routes/safe` request that has been computed and cached
- WHEN an identical request (same rounded coordinates, time bucket, weekday type, and active model)
  arrives within the TTL
- THEN it is served from the cache and the routing repository is not queried

#### Scenario: Active-model change bypasses old entries

- GIVEN a cached response computed under model A
- WHEN model B becomes the active model and the same request arrives
- THEN the key (which includes the model id) does not match A's entry, so the response is recomputed

### Requirement: Cache is best-effort (fail-open)

A Redis error during cache `Get` or `Set` SHALL NOT fail the request. The Redis cache SHALL log such
errors and report a miss (for `Get`) or a no-op (for `Set`); the service SHALL treat a `Get` error as
a miss and compute normally. Reachability is validated once at startup (see redis-runtime), so
request-time errors are transient.

#### Scenario: Redis blip during a cached request

- GIVEN `ROUTE_CACHE_ENABLED=true` and Redis returns an error on `Get`
- WHEN `/routes/safe` is requested with valid parameters
- THEN the service computes the response normally and returns `200` (the error is logged, not surfaced)
