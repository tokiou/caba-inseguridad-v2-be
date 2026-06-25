package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/auth"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/config"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/crimes"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/health"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/observability"
	postgresplatform "github.com/tokiou/caba-inseguridad-routes-go/internal/platform/postgres"
	redisplatform "github.com/tokiou/caba-inseguridad-routes-go/internal/platform/redis"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/ratelimit"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/roadgraph"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/routes"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/saferoutes"
)

type App struct {
	Router http.Handler
	pool   *pgxpool.Pool
	redis  *redis.Client // nil when REDIS_ENABLED=false
}

func New(ctx context.Context, cfg config.Config, log *slog.Logger) (*App, error) {
	startedAt := time.Now()

	if err := validateAuthConfig(cfg, log); err != nil {
		return nil, err
	}
	if err := validateRedisConfig(cfg); err != nil {
		return nil, err
	}

	pool, err := postgresplatform.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	// Redis is optional. It is only connected when enabled; the two feature
	// flags require it (validated above), so when either is on the client is
	// non-nil. A failed ping fails startup (inside NewClient).
	var redisClient *redis.Client
	if cfg.RedisEnabled {
		redisClient, err = redisplatform.NewClient(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
		if err != nil {
			pool.Close()
			return nil, err
		}
	}

	// Rate-limit middleware per endpoint (identity passthrough when disabled),
	// and the route cache (no-op when disabled). Handlers/services stay ignorant
	// of the toggles.
	limiters := ratelimit.Passthrough()
	if cfg.RateLimitEnabled {
		limiters, err = ratelimit.NewMiddlewares(redisClient)
		if err != nil {
			pool.Close()
			closeRedis(redisClient)
			return nil, err
		}
	}

	var routeCache saferoutes.RouteCache = saferoutes.NoopRouteCache{}
	if cfg.RouteCacheEnabled {
		routeCache = saferoutes.NewRedisRouteCache(redisClient, log)
	}

	crimesRepo := crimes.NewRepository(pool)
	crimesService := crimes.NewService(crimesRepo)
	crimesHandler := crimes.NewHandler(crimesService, limiters.CrimesNearby, log)

	orsClient := routes.NewORSClient(cfg.ORSAPIKey, cfg.ORSBaseURL, &http.Client{Timeout: 10 * time.Second})
	routesService := routes.NewService(orsClient)
	routesHandler := routes.NewHandler(routesService, log)

	roadGraphRepo := roadgraph.NewRepository(pool)
	roadGraphService := roadgraph.NewService(roadGraphRepo)
	roadGraphHandler := roadgraph.NewHandler(roadGraphService, limiters.RoadgraphStats, log)

	safeRoutesRepo := saferoutes.NewRepository(pool)
	safeRoutesService := saferoutes.NewService(safeRoutesRepo, routeCache)
	safeRoutesHandler := saferoutes.NewHandler(safeRoutesService, limiters.RoutesSafe, log)

	healthHandler := health.NewHandler()

	tokenManager := auth.NewTokenManager(cfg.JWTSecret, cfg.AccessTokenTTL)
	authRepo := auth.NewRepository(pool)
	authService := auth.NewService(authRepo, tokenManager, cfg.RefreshTokenTTL, log)
	authMiddleware := auth.Middleware(authService, log)
	authHandler := auth.NewHandler(authService, authMiddleware, limiters.AuthLogin, refreshCookieConfig(cfg), log)

	public := []Registrar{healthHandler, crimesHandler, routesHandler, roadGraphHandler, authHandler}
	protected := []Registrar{safeRoutesHandler}

	// Benchmark introspection endpoint — only mounted when enabled (it leaks
	// internals; the handler also rejects non-loopback clients).
	if cfg.MetricsEnabled {
		public = append(public, newObservabilityHandler(pool, routeCache, startedAt, log))
	}

	return &App{
		Router: NewRouter(log, authMiddleware, public, protected),
		pool:   pool,
		redis:  redisClient,
	}, nil
}

// Close releases the Postgres pool and the Redis client (if any). The context is
// accepted for a uniform shutdown signature; pgxpool.Close blocks until
// connections are returned.
func (a *App) Close(_ context.Context) error {
	a.pool.Close()
	closeRedis(a.redis)
	return nil
}

func closeRedis(c *redis.Client) {
	if c != nil {
		_ = c.Close()
	}
}

// newObservabilityHandler adapts the pgxpool and route-cache stats into the
// plain-struct closures the observability handler consumes, keeping that package
// decoupled from pgxpool and saferoutes. The cache closure is nil unless the
// active cache reports stats (the no-op cache does not), so the endpoint reports
// the cache as disabled in that case.
func newObservabilityHandler(pool *pgxpool.Pool, cache saferoutes.RouteCache, started time.Time, log *slog.Logger) *observability.Handler {
	poolStats := func() observability.PoolStats {
		s := pool.Stat()
		return observability.PoolStats{
			AcquiredConns:        s.AcquiredConns(),
			IdleConns:            s.IdleConns(),
			TotalConns:           s.TotalConns(),
			MaxConns:             s.MaxConns(),
			AcquireCount:         s.AcquireCount(),
			EmptyAcquireCount:    s.EmptyAcquireCount(),
			CanceledAcquireCount: s.CanceledAcquireCount(),
			NewConnsCount:        s.NewConnsCount(),
			AcquireDurationMS:    float64(s.AcquireDuration().Milliseconds()),
		}
	}

	var cacheStats observability.CacheStatsFunc
	if p, ok := cache.(saferoutes.CacheStatsProvider); ok {
		cacheStats = func() observability.CacheStats {
			cs := p.Stats()
			return observability.CacheStats{
				Hits:    cs.Hits,
				Misses:  cs.Misses,
				Errors:  cs.Errors,
				Sets:    cs.Sets,
				HitRate: cs.HitRate,
			}
		}
	}

	return observability.NewHandler(poolStats, cacheStats, started, log)
}

// validateAuthConfig refuses to start with a missing or placeholder JWT secret
// outside development, where it would silently sign tokens with a guessable key.
func validateAuthConfig(cfg config.Config, log *slog.Logger) error {
	weak := cfg.JWTSecret == "" || cfg.JWTSecret == "change_me"
	if !weak {
		return nil
	}
	if cfg.IsDevelopment() {
		log.Warn("JWT_SECRET is empty or the default placeholder; acceptable only in development")
		return nil
	}
	return errors.New("app: JWT_SECRET must be set to a non-default value outside development")
}

// validateRedisConfig enforces the feature-flag matrix: rate limiting and the
// route cache both require Redis. Checked before any Redis connection so a
// misconfiguration fails fast with a clear message.
func validateRedisConfig(cfg config.Config) error {
	if cfg.RateLimitEnabled && !cfg.RedisEnabled {
		return errors.New("invalid config: RATE_LIMIT_ENABLED=true requires REDIS_ENABLED=true")
	}
	if cfg.RouteCacheEnabled && !cfg.RedisEnabled {
		return errors.New("invalid config: ROUTE_CACHE_ENABLED=true requires REDIS_ENABLED=true")
	}
	return nil
}

func refreshCookieConfig(cfg config.Config) auth.CookieConfig {
	return auth.CookieConfig{
		Name:     cfg.RefreshCookieName,
		Path:     "/api/v1/auth",
		Secure:   cfg.CookieSecure,
		SameSite: parseSameSite(cfg.CookieSameSite),
		MaxAge:   int(cfg.RefreshTokenTTL.Seconds()),
	}
}

func parseSameSite(s string) http.SameSite {
	switch strings.ToLower(s) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}
