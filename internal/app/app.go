package app

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/tokiou/caba-inseguridad-routes-go/internal/config"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/crimes"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/health"
	mongoplatform "github.com/tokiou/caba-inseguridad-routes-go/internal/platform/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type App struct {
	Router      http.Handler
	mongoClient *mongo.Client
}

func New(ctx context.Context, cfg config.Config, log *slog.Logger) (*App, error) {
	mongoClient, err := mongoplatform.NewClient(ctx, cfg.MongoURI)
	if err != nil {
		return nil, err
	}

	crimesCollection := mongoClient.
		Database(cfg.MongoDatabase).
		Collection(cfg.MongoCrimesCollection)

	crimesRepo := crimes.NewMongoRepository(crimesCollection)
	crimesService := crimes.NewService(crimesRepo)
	crimesHandler := crimes.NewHandler(crimesService, log)
	healthHandler := health.NewHandler()

	return &App{
		Router:      NewRouter(healthHandler, crimesHandler),
		mongoClient: mongoClient,
	}, nil
}

func (a *App) Close(ctx context.Context) error {
	return a.mongoClient.Disconnect(ctx)
}
