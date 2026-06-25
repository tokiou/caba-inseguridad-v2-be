# Rate Limiting — Delta Specification

## ADDED Requirements

### Requirement: Distributed Redis-backed rate limiting

The API SHALL enforce request rate limits using a Redis-backed store via
`github.com/ulule/limiter/v3`, so that limits are shared across all API instances pointing at the
same Redis. The limiter SHALL be applied as chi middleware **before** the route handler; handlers
SHALL NOT be modified to produce the limited response. Rate limiting SHALL only be active when
`RATE_LIMIT_ENABLED=true` (which requires `REDIS_ENABLED=true`).

#### Scenario: Limit shared across instances

- GIVEN `RATE_LIMIT_ENABLED=true` and two API instances using the same Redis
- WHEN a client exceeds an endpoint's limit through requests split across both instances
- THEN the limit is enforced on the combined count, not per-instance

#### Scenario: Disabled by flag

- GIVEN `RATE_LIMIT_ENABLED=false`
- WHEN a client issues many requests to any endpoint
- THEN no request is rejected with `429` by the rate limiter and no rate-limit counter is created

### Requirement: Per-endpoint limits with isolated counters

The API SHALL apply these per-endpoint limits, each backed by its own Redis key prefix so consuming
one endpoint's quota SHALL NOT consume another's:

| Endpoint | Limit | Key prefix |
|---|---:|---|
| `GET /routes/safe` | 10 / minute | `safe-routes:ratelimit:routes-safe` |
| `POST /auth/login` | 5 / minute | `safe-routes:ratelimit:auth-login` |
| `GET /crimes/nearby` | 30 / minute | `safe-routes:ratelimit:crimes-nearby` |
| `GET /roadgraph/stats` | 60 / minute | `safe-routes:ratelimit:roadgraph-stats` |

Only the endpoints listed SHALL be limited. `GET /roadgraph/route` and the non-login `/auth/*`
endpoints SHALL NOT be rate limited in this iteration.

#### Scenario: Each endpoint has an independent quota

- GIVEN `RATE_LIMIT_ENABLED=true` and a client that has exhausted `/crimes/nearby` (30 in a minute)
- WHEN the same client requests `/roadgraph/stats`
- THEN the `/roadgraph/stats` request is served normally because its counter is separate

#### Scenario: Unlisted endpoints are not limited

- GIVEN `RATE_LIMIT_ENABLED=true`
- WHEN a client issues 100 requests to `GET /roadgraph/route` within a minute
- THEN none are rejected by the rate limiter

### Requirement: Rate limit keyed by client IP

In this iteration the limiter SHALL key counters by the requesting client's IP address, derived from
the connection. The limiter SHALL NOT trust `X-Forwarded-For` (or similar forwarded headers) unless a
trusted reverse proxy is explicitly configured; the secure default (connection IP) SHALL be used.

#### Scenario: Two clients have separate quotas

- GIVEN `RATE_LIMIT_ENABLED=true` and IP A has exhausted an endpoint's limit
- WHEN a request to the same endpoint arrives from IP B
- THEN IP B's request is served (its counter is independent of IP A's)

### Requirement: Over-limit response is 429 with rate-limit headers

When a request exceeds the endpoint's limit, the middleware SHALL respond `429 Too Many Requests`
without invoking the handler, and SHALL include the rate-limit headers the library emits
(`X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, and `Retry-After`).

#### Scenario: 11th request to /routes/safe in a minute is rejected

- GIVEN `RATE_LIMIT_ENABLED=true` and an authenticated client that has made 10 `/routes/safe`
  requests within a minute from one IP
- WHEN it makes an 11th `/routes/safe` request within that minute
- THEN the response is `429 Too Many Requests`
- AND no routing or risk computation is performed
- AND the response carries the rate-limit headers

#### Scenario: 6th login attempt in a minute is rejected

- GIVEN `RATE_LIMIT_ENABLED=true` and 5 `POST /auth/login` requests from one IP within a minute
- WHEN a 6th login request arrives within that minute
- THEN the response is `429 Too Many Requests` and the login handler is not invoked
