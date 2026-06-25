package ratelimit

import (
	"fmt"
	"net/http"

	"github.com/redis/go-redis/v9"
	"github.com/ulule/limiter/v3"
	mhttp "github.com/ulule/limiter/v3/drivers/middleware/stdlib"
	redisstore "github.com/ulule/limiter/v3/drivers/store/redis"
)

// Middleware is a chi-compatible HTTP middleware. It is the plain
// func(http.Handler) http.Handler shape so handlers can accept it without
// importing this package.
type Middleware = func(http.Handler) http.Handler

// NewMiddleware builds a Redis-backed rate-limit middleware for a single
// endpoint. rateFormatted is ulule's "N-M" form (e.g. "10-M" = 10/minute) and
// prefix namespaces this endpoint's counter in Redis so quotas never bleed
// across endpoints. Counters are keyed by client IP (the library default, which
// does not trust X-Forwarded-For); the middleware emits the rate-limit headers
// and a 429 on exceed without invoking the handler.
func NewMiddleware(client *redis.Client, rateFormatted, prefix string) (Middleware, error) {
	rate, err := limiter.NewRateFromFormatted(rateFormatted)
	if err != nil {
		return nil, fmt.Errorf("ratelimit: invalid rate %q: %w", rateFormatted, err)
	}

	store, err := redisstore.NewStoreWithOptions(client, limiter.StoreOptions{Prefix: prefix})
	if err != nil {
		return nil, fmt.Errorf("ratelimit: create redis store (%s): %w", prefix, err)
	}

	return mhttp.NewMiddleware(limiter.New(store, rate)).Handler, nil
}

// Middlewares bundles one rate-limit middleware per rate-limited endpoint.
type Middlewares struct {
	RoutesSafe     Middleware
	AuthLogin      Middleware
	CrimesNearby   Middleware
	RoadgraphStats Middleware
}

// NewMiddlewares builds the Redis-backed middleware for every rate-limited
// endpoint (used when RATE_LIMIT_ENABLED=true).
func NewMiddlewares(client *redis.Client) (Middlewares, error) {
	var m Middlewares
	for _, s := range []struct {
		dst    *Middleware
		rate   string
		prefix string
	}{
		{&m.RoutesSafe, RoutesSafeRate, RoutesSafePrefix},
		{&m.AuthLogin, AuthLoginRate, AuthLoginPrefix},
		{&m.CrimesNearby, CrimesNearbyRate, CrimesNearbyPrefix},
		{&m.RoadgraphStats, RoadgraphStatsRate, RoadgraphStatsPrefix},
	} {
		mw, err := NewMiddleware(client, s.rate, s.prefix)
		if err != nil {
			return Middlewares{}, err
		}
		*s.dst = mw
	}
	return m, nil
}

// Passthrough returns identity middlewares (no rate limiting). Used when
// RATE_LIMIT_ENABLED=false: r.With(identity) is equivalent to an unwrapped
// route, so handler registration stays uniform and no Redis-backed limiter is
// constructed.
func Passthrough() Middlewares {
	id := func(next http.Handler) http.Handler { return next }
	return Middlewares{
		RoutesSafe:     id,
		AuthLogin:      id,
		CrimesNearby:   id,
		RoadgraphStats: id,
	}
}
