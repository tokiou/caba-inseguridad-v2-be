package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/auth"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/config"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/crimes"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/health"
	postgresplatform "github.com/tokiou/caba-inseguridad-routes-go/internal/platform/postgres"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/roadgraph"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/routes"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/saferoutes"
)

type App struct {
	Router http.Handler
	pool   *pgxpool.Pool
}

func New(ctx context.Context, cfg config.Config, log *slog.Logger) (*App, error) {
	if err := validateAuthConfig(cfg, log); err != nil {
		return nil, err
	}

	pool, err := postgresplatform.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	crimesRepo := crimes.NewRepository(pool)
	crimesService := crimes.NewService(crimesRepo)
	crimesHandler := crimes.NewHandler(crimesService, log)

	orsClient := routes.NewORSClient(cfg.ORSAPIKey, cfg.ORSBaseURL, &http.Client{Timeout: 10 * time.Second})
	routesService := routes.NewService(orsClient)
	routesHandler := routes.NewHandler(routesService, log)

	roadGraphRepo := roadgraph.NewRepository(pool)
	roadGraphService := roadgraph.NewService(roadGraphRepo)
	roadGraphHandler := roadgraph.NewHandler(roadGraphService, log)

	safeRoutesRepo := saferoutes.NewRepository(pool)
	safeRoutesService := saferoutes.NewService(safeRoutesRepo)
	safeRoutesHandler := saferoutes.NewHandler(safeRoutesService, log)

	healthHandler := health.NewHandler()

	tokenManager := auth.NewTokenManager(cfg.JWTSecret, cfg.AccessTokenTTL)
	authRepo := auth.NewRepository(pool)
	authService := auth.NewService(authRepo, tokenManager, cfg.RefreshTokenTTL, log)
	authMiddleware := auth.Middleware(authService, log)
	authHandler := auth.NewHandler(authService, authMiddleware, refreshCookieConfig(cfg), log)

	public := []Registrar{healthHandler, crimesHandler, routesHandler, roadGraphHandler, authHandler}
	protected := []Registrar{safeRoutesHandler}

	return &App{
		Router: NewRouter(log, authMiddleware, public, protected),
		pool:   pool,
	}, nil
}

// Close releases the Postgres connection pool. The context is accepted for a
// uniform shutdown signature; pgxpool.Close blocks until connections are returned.
func (a *App) Close(_ context.Context) error {
	a.pool.Close()
	return nil
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
