package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/tokiou/caba-inseguridad-routes-go/internal/app"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/config"
)

func main() {
	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	application, err := app.New(ctx, cfg)
	if err != nil {
		log.Fatalf("could not initialize app: %v", err)
	}

	defer func() {
		disconnectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := application.MongoClient.Disconnect(disconnectCtx); err != nil {
			log.Printf("could not disconnect MongoDB: %v", err)
		}
	}()

	addr := fmt.Sprintf(":%s", cfg.HTTPPort)

	log.Printf("server listening on %s", addr)

	if err := http.ListenAndServe(addr, application.Router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
