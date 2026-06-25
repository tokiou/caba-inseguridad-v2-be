# Redis Runtime — Delta Specification

## ADDED Requirements

### Requirement: Redis client lifecycle

When `REDIS_ENABLED=true`, the app SHALL construct a Redis client at startup and `Ping` it; if the
ping fails, `app.New` SHALL return an error and the server SHALL NOT start (fail-fast). When
`REDIS_ENABLED=false`, the app SHALL NOT attempt any Redis connection and the client SHALL be nil.
The client SHALL be closed on shutdown when present. The constructor SHALL live in
`internal/platform/redis` (mirroring `internal/platform/postgres`).

#### Scenario: Redis unavailable fails startup

- GIVEN `REDIS_ENABLED=true` and no reachable Redis at `REDIS_ADDR`
- WHEN the app starts
- THEN `app.New` returns an error and the HTTP server does not begin listening

#### Scenario: Redis disabled needs no Redis

- GIVEN `REDIS_ENABLED=false`, `RATE_LIMIT_ENABLED=false`, `ROUTE_CACHE_ENABLED=false`
- WHEN the app starts
- THEN it starts successfully without contacting Redis (the baseline benchmark mode)

### Requirement: Feature flags require Redis

`app.New` SHALL validate the feature-flag matrix before initializing Redis and SHALL fail fast with a
clear message when a feature that needs Redis is enabled without it:

- `RATE_LIMIT_ENABLED=true` requires `REDIS_ENABLED=true`
- `ROUTE_CACHE_ENABLED=true` requires `REDIS_ENABLED=true`

#### Scenario: Rate limit without Redis is rejected

- GIVEN `RATE_LIMIT_ENABLED=true` and `REDIS_ENABLED=false`
- WHEN the app starts
- THEN it fails with `invalid config: RATE_LIMIT_ENABLED=true requires REDIS_ENABLED=true`

#### Scenario: Route cache without Redis is rejected

- GIVEN `ROUTE_CACHE_ENABLED=true` and `REDIS_ENABLED=false`
- WHEN the app starts
- THEN it fails with `invalid config: ROUTE_CACHE_ENABLED=true requires REDIS_ENABLED=true`

### Requirement: Benchmark modes selectable by configuration

The three flags SHALL compose into the following runtime modes with no code change, so k6 can compare
layers across runs:

| Mode | `REDIS_ENABLED` | `RATE_LIMIT_ENABLED` | `ROUTE_CACHE_ENABLED` |
|---|---|---|---|
| baseline | false | false | false |
| redis-only | true | false | false |
| rate-limit | true | true | false |
| cache | true | false | true |
| cache + rate-limit | true | true | true |

#### Scenario: Cache-only mode applies no rate limit

- GIVEN `REDIS_ENABLED=true`, `RATE_LIMIT_ENABLED=false`, `ROUTE_CACHE_ENABLED=true`
- WHEN a client floods `/routes/safe` with valid authenticated requests
- THEN no request is rejected with `429`, and repeated identical requests are served from the cache

#### Scenario: Rate-limit-only mode applies no cache

- GIVEN `REDIS_ENABLED=true`, `RATE_LIMIT_ENABLED=true`, `ROUTE_CACHE_ENABLED=false`
- WHEN a client makes repeated identical `/routes/safe` requests within the limit
- THEN each is computed fresh (no cache), and the 11th within a minute is rejected with `429`
