package saferoutes

import (
	"context"
	"fmt"
	"time"
)

// routeCacheTTL bounds how long a computed response may be served from cache.
const routeCacheTTL = time.Hour

// RouteCache caches computed safe-route responses. Implementations:
// RedisRouteCache (when ROUTE_CACHE_ENABLED) and NoopRouteCache (otherwise). The
// seam keeps the service free of environment/Redis concerns. Contract: cache is
// best-effort — a Get error is treated as a miss and Set errors are swallowed, so
// cache trouble never fails a request (reachability is validated once at startup).
type RouteCache interface {
	Get(ctx context.Context, key string) (*SafeRoutesResponse, bool, error)
	Set(ctx context.Context, key string, value SafeRoutesResponse, ttl time.Duration) error
}

// CacheStats is a snapshot of route-cache counters, surfaced by the stats
// endpoint. HitRate is hits / (hits + misses), 0 when there were no lookups.
type CacheStats struct {
	Hits    int64   `json:"hits"`
	Misses  int64   `json:"misses"`
	Errors  int64   `json:"errors"`
	Sets    int64   `json:"sets"`
	HitRate float64 `json:"hit_rate"`
}

// CacheStatsProvider is implemented by caches that count their activity
// (RedisRouteCache). The observability handler reads it without importing Redis;
// the no-op cache is not a provider (cache off → stats report it disabled).
type CacheStatsProvider interface {
	Stats() CacheStats
}

// NoopRouteCache is the disabled-cache implementation: every Get misses and Set
// does nothing, so the service always computes against Postgres/PostGIS/pgRouting.
type NoopRouteCache struct{}

func (NoopRouteCache) Get(context.Context, string) (*SafeRoutesResponse, bool, error) {
	return nil, false, nil
}

func (NoopRouteCache) Set(context.Context, string, SafeRoutesResponse, time.Duration) error {
	return nil
}

// routeCacheKey identifies a response by the inputs the scores actually depend
// on: coordinates rounded to 5 decimals (~1.1 m), the resolved time bucket and
// weekday type, and the active model id. Including the model id means a model
// change naturally bypasses stale entries. Datetime is intentionally coarsened to
// (time_bucket, weekday_type) — the only temporal inputs the scores use.
func routeCacheKey(q SafeRoutesQuery, timeBucket, weekdayType string, modelID int64) string {
	return fmt.Sprintf("route:safe:%.5f:%.5f:%.5f:%.5f:%s:%s:%d",
		q.OriginLat, q.OriginLng, q.DestLat, q.DestLng, timeBucket, weekdayType, modelID)
}
