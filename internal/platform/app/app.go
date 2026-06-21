package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/baechuer/cityevents/internal/platform/config"
	"github.com/baechuer/cityevents/internal/platform/httpapi"
	"github.com/baechuer/cityevents/internal/platform/logging"
)

type RouterFactory func(context.Context, config.Config, *slog.Logger) (http.Handler, func(context.Context) error, error)

func Main(serviceName string, factories ...RouterFactory) {
	os.Exit(Run(serviceName, factories...))
}

func Run(serviceName string, factories ...RouterFactory) int {
	cfg, err := config.Load(serviceName, os.Getenv)
	if err != nil {
		slog.Error("config load failed", slog.String("service", serviceName), slog.String("error", err.Error()))
		return 1
	}

	logger := logging.New(cfg.Service.Name, cfg.Environment)

	if startupCheckOnly() {
		_ = httpapi.NewRouter(cfg, logger)
		logger.Info("startup check passed", slog.String("http_addr", cfg.HTTPAddr))
		return 0
	}

	factory := defaultRouterFactory
	if len(factories) > 0 && factories[0] != nil {
		factory = factories[0]
	}

	router, cleanup, err := factory(context.Background(), cfg, logger)
	if err != nil {
		logger.Error("router setup failed", slog.String("error", err.Error()))
		return 1
	}
	if cleanup != nil {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
			defer cancel()
			if err := cleanup(ctx); err != nil {
				logger.Error("service cleanup failed", slog.String("error", err.Error()))
			}
		}()
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("service listening", slog.String("http_addr", cfg.HTTPAddr))
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", slog.String("error", err.Error()))
			return 1
		}
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown failed", slog.String("error", err.Error()))
		return 1
	}

	logger.Info("service stopped")
	return 0
}

func defaultRouterFactory(_ context.Context, cfg config.Config, logger *slog.Logger) (http.Handler, func(context.Context) error, error) {
	return httpapi.NewRouter(cfg, logger), nil, nil
}

func startupCheckOnly() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("CITYEVENTS_STARTUP_CHECK_ONLY")))
	return value == "1" || value == "true" || value == "yes"
}
