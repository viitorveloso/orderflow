// Command api is the orderflow HTTP server entrypoint. It loads configuration,
// connects to Postgres, applies migrations, wires the layers together, and
// serves HTTP with sensible timeouts and graceful shutdown.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yourusername/orderflow/internal/auth"
	"github.com/yourusername/orderflow/internal/config"
	"github.com/yourusername/orderflow/internal/database"
	"github.com/yourusername/orderflow/internal/httpapi"
	"github.com/yourusername/orderflow/internal/repository"
	"github.com/yourusername/orderflow/internal/service"
	"github.com/yourusername/orderflow/migrations"
)

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := newLogger(cfg)
	slog.SetDefault(logger)

	// Cancel the context on SIGINT/SIGTERM so startup and shutdown can react.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := database.RunMigrations(ctx, db, migrations.FS); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	logger.Info("database ready; migrations applied")

	tokens, err := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTTTL)
	if err != nil {
		return err
	}

	// Wire repositories -> services -> HTTP handlers.
	userRepo := repository.NewUserRepo(db)
	productRepo := repository.NewProductRepo(db)
	orderRepo := repository.NewOrderRepo(db)

	authSvc := service.NewAuthService(userRepo, tokens)
	productSvc := service.NewProductService(productRepo)
	orderSvc := service.NewOrderService(orderRepo, productRepo)

	srv := httpapi.NewServer(authSvc, productSvc, orderSvc, tokens, logger)

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", httpServer.Addr, "env", cfg.Env)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Block until the server fails or a shutdown signal arrives.
	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	// Give in-flight requests up to 10s to complete.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	logger.Info("server stopped cleanly")
	return nil
}

func newLogger(cfg *config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if cfg.IsProduction() {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}
