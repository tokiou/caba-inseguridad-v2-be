// Package ratelimit provides distributed, per-endpoint rate limiting backed by
// Redis via github.com/ulule/limiter/v3. Limits are applied as chi middleware
// before the handler; each endpoint has its own Redis key prefix so quotas never
// bleed across endpoints.
package ratelimit

// Rate policies in ulule/limiter's "N-M" format ("M" = per minute). Tuning is
// here, not scattered across wiring.
const (
	RoutesSafeRate     = "10-M" // expensive: pgr_dijkstra ×3 + pgr_ksp + risk joins
	AuthLoginRate      = "5-M"  // brute-force protection
	CrimesNearbyRate   = "30-M" // lightweight geospatial query
	RoadgraphStatsRate = "60-M" // read-only stats
)

// Redis key prefixes — one counter namespace per endpoint so consuming one
// endpoint's quota does not consume another's.
const (
	RoutesSafePrefix     = "safe-routes:ratelimit:routes-safe"
	AuthLoginPrefix      = "safe-routes:ratelimit:auth-login"
	CrimesNearbyPrefix   = "safe-routes:ratelimit:crimes-nearby"
	RoadgraphStatsPrefix = "safe-routes:ratelimit:roadgraph-stats"
)
