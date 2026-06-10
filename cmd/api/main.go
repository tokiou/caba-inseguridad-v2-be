package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tokiou/caba-inseguridad-routes-go/internal/app"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/config"
	loggerplatform "github.com/tokiou/caba-inseguridad-routes-go/internal/platform/logger"
)

func main() {
	cfg := config.Load()

	logger := loggerplatform.New(cfg.LogFormat, cfg.LogLevel)
	slog.SetDefault(logger)

	startCtx, startCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer startCancel()

	application, err := app.New(startCtx, cfg, logger)
	if err != nil {
		logger.Error("could not initialize app", "err", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.HTTPPort),
		Handler:      application.Router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown error", "err", err)
	}

	if err := application.Close(shutdownCtx); err != nil {
		logger.Error("datastore close error", "err", err)
	}

	logger.Info("server stopped")
}
