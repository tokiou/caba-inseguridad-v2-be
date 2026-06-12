package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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

	return &App{
		Router: NewRouter(log, healthHandler, crimesHandler, routesHandler, roadGraphHandler, safeRoutesHandler),
		pool:   pool,
	}, nil
}

// Close releases the Postgres connection pool. The context is accepted for a
// uniform shutdown signature; pgxpool.Close blocks until connections are returned.
func (a *App) Close(_ context.Context) error {
	a.pool.Close()
	return nil
}
