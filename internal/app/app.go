package app

import (
	"context"
	"net/http"

	"github.com/tokiou/caba-inseguridad-routes-go/internal/config"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/crimes"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/health"
	mongoplatform "github.com/tokiou/caba-inseguridad-routes-go/internal/platform/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type App struct {
	Router      http.Handler
	MongoClient *mongo.Client
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	mongoClient, err := mongoplatform.NewClient(ctx, cfg.MongoURI)
	if err != nil {
		return nil, err
	}

	crimesCollection := mongoClient.
		Database(cfg.MongoDatabase).
		Collection(cfg.MongoCrimesCollection)

	crimesRepository := crimes.NewMongoRepository(crimesCollection)
	crimesService := crimes.NewService(crimesRepository)
	crimesHandler := crimes.NewHandler(crimesService)

	healthHandler := health.NewHandler()

	router := NewRouter(healthHandler, crimesHandler)

	return &App{
		Router:      router,
		MongoClient: mongoClient,
	}, nil
}
