# observability Specification

## Purpose
TBD - created by archiving change add-benchmark-suite. Update Purpose after archive.
## Requirements
### Requirement: Runtime stats endpoint

The API SHALL expose `GET /api/v1/debug/stats` returning a JSON snapshot of runtime internals:
pgxpool statistics (acquired / idle / total / max connections, acquire count and cumulative duration,
empty- and canceled-acquire counts, new-conns count), route-cache counters (hits, misses, errors,
sets, and derived hit-rate, plus whether the cache is enabled), and basic process info (uptime,
goroutine count). The endpoint SHALL be registered only when `METRICS_ENABLED=true` (default false)
and SHALL reject non-loopback clients with `403`, because it exposes internals. It is a thin
introspection handler (no service/repository layer), reading `pgxpool.Stat()` and the cache counters
directly.

#### Scenario: Disabled by flag

- GIVEN `METRICS_ENABLED=false`
- WHEN a client requests `GET /api/v1/debug/stats`
- THEN the route does not exist and the response is `404`

#### Scenario: Enabled, loopback request returns stats

- GIVEN `METRICS_ENABLED=true`
- WHEN a loopback client requests `GET /api/v1/debug/stats`
- THEN the response is `200` with a JSON body containing `pgxpool` and `cache` objects and runtime info

#### Scenario: Enabled, non-loopback request is rejected

- GIVEN `METRICS_ENABLED=true`
- WHEN a non-loopback client requests `GET /api/v1/debug/stats`
- THEN the response is `403`

#### Scenario: Cache disabled is reported

- GIVEN `METRICS_ENABLED=true` and `ROUTE_CACHE_ENABLED=false`
- WHEN the stats endpoint is queried
- THEN the `cache` object reports `enabled: false` with zeroed counters

### Requirement: Cache observability

The route cache SHALL count hits, misses, errors, and sets, and these counters SHALL be exposed
through the stats endpoint. `GET /api/v1/routes/safe` SHALL set an `X-Cache: hit` or `X-Cache: miss`
response header reflecting whether the response was served from cache, so a load test can split
latency by cache outcome without server access. The header SHALL NOT change the response body, and the
hit/miss flag SHALL NOT be persisted in the cached payload.

#### Scenario: Miss then hit sets the header and counters

- GIVEN `ROUTE_CACHE_ENABLED=true` and an empty cache
- WHEN a client requests `/routes/safe`, then repeats the identical request within the TTL
- THEN the first response carries `X-Cache: miss` and the second carries `X-Cache: hit`
- AND the stats endpoint reports at least one hit and one miss

#### Scenario: Header reflects cache outcome only (no body change)

- WHEN the same `/routes/safe` request is served from cache and recomputed
- THEN both bodies are equivalent and only the `X-Cache` header differs

