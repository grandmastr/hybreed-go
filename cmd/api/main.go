// Command api is the Hybreed HTTP backend entrypoint.
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

	"github.com/grandmastr/hybreed-go/internal/api"
	"github.com/grandmastr/hybreed-go/internal/athlete"
	"github.com/grandmastr/hybreed-go/internal/auth"
	"github.com/grandmastr/hybreed-go/internal/cache"
	"github.com/grandmastr/hybreed-go/internal/config"
	"github.com/grandmastr/hybreed-go/internal/database"
	"github.com/grandmastr/hybreed-go/internal/home"
	"github.com/grandmastr/hybreed-go/internal/nutrition"
	"github.com/grandmastr/hybreed-go/internal/store"
	"github.com/grandmastr/hybreed-go/internal/training"
)

// version is overridable at build time: -ldflags "-X main.version=1.2.3".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := newLogger(cfg)
	api.Version = version

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Postgres.
	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()
	logger.Info("connected to postgres")

	if cfg.AutoMigrate {
		if err := database.Migrate(cfg.DatabaseURL, logger); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}

	// Redis.
	redis, err := cache.New(ctx, cfg.RedisURL, logger)
	if err != nil {
		return fmt.Errorf("connect redis: %w", err)
	}
	defer func() { _ = redis.Close() }()
	logger.Info("connected to redis")

	// Wire services + handlers.
	q := store.New(pool)
	tokens := auth.NewTokenManager(cfg.JWTSecret, cfg.AccessTokenTTL)
	authMW := auth.Authenticator(tokens)

	authSvc := auth.NewService(pool, q, tokens, cfg, logger)
	athleteSvc := athlete.NewService(q, logger)
	trainingSvc := training.NewService(pool, q, redis, logger)
	nutritionSvc := nutrition.NewService(pool, q, redis, logger)
	homeSvc := home.NewService(trainingSvc, nutritionSvc, logger)

	router := api.NewRouter(api.Deps{
		Config:    cfg,
		Logger:    logger,
		Pool:      pool,
		Cache:     redis,
		AuthMW:    authMW,
		Auth:      auth.NewHandler(authSvc, logger, authMW),
		Athlete:   athlete.NewHandler(athleteSvc, logger),
		Training:  training.NewHandler(trainingSvc, logger),
		Nutrition: nutrition.NewHandler(nutritionSvc, logger),
		Home:      home.NewHandler(homeSvc, logger),
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", cfg.HTTPAddr, "env", cfg.Env, "version", version)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("server stopped cleanly")
	return nil
}

func newLogger(cfg config.Config) *slog.Logger {
	var level slog.Level
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if cfg.IsProduction() {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}
