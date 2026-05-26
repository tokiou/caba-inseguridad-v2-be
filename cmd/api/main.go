package main

import (
	"log"
	"net/http"

	"github.com/tokiou/caba-inseguridad-routes-go/internal/app"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/health"
)

func main() {
	healthHandler := health.NewHandler()

	router := app.NewRouter(healthHandler)

	addr := ":8080"

	log.Printf("server listening on %s", addr)

	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}