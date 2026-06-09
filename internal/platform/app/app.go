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

func Main(serviceName string) {
	os.Exit(Run(serviceName))
}

func Run(serviceName string) int {
	cfg, err := config.Load(serviceName, os.Getenv)
	if err != nil {
		slog.Error("config load failed", slog.String("service", serviceName), slog.String("error", err.Error()))
		return 1
	}

	logger := logging.New(cfg.Service.Name, cfg.Environment)
	router := httpapi.NewRouter(cfg, logger)

	if startupCheckOnly() {
		logger.Info("startup check passed", slog.String("http_addr", cfg.HTTPAddr))
		return 0
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
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

func startupCheckOnly() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("CITYEVENTS_STARTUP_CHECK_ONLY")))
	return value == "1" || value == "true" || value == "yes"
}
