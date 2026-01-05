package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/baechuer/real-time-ressys/services/join-service/internal/config"
	"github.com/baechuer/real-time-ressys/services/join-service/internal/infrastructure/postgres"
	"github.com/baechuer/real-time-ressys/services/join-service/internal/infrastructure/rabbitmq"
	"github.com/baechuer/real-time-ressys/services/join-service/internal/infrastructure/redis"
	"github.com/baechuer/real-time-ressys/services/join-service/internal/pkg/logger"
	"github.com/baechuer/real-time-ressys/services/join-service/internal/security"
	"github.com/baechuer/real-time-ressys/services/join-service/internal/service"
	"github.com/baechuer/real-time-ressys/services/join-service/internal/transport/rest"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load failed: %v\n", err)
		os.Exit(1)
	}

	// If your logger reads LOG_LEVEL from env, ensure cfg.LogLevel takes effect
	if cfg.LogLevel != "" {
		_ = os.Setenv("LOG_LEVEL", cfg.LogLevel)
	}

	logger.Init()
	log := logger.Logger.With().
		Str("service", "join-service").
		Str("env", cfg.AppEnv).
		Logger()

	// Root ctx with signal cancellation
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ---- Postgres ----
	dbPool, err := pgxpool.New(rootCtx, cfg.DBDSN)
	if err != nil {
		log.Fatal().Err(err).Msg("postgres pool create failed")
	}
	defer dbPool.Close()

	{
		pingCtx, cancel := context.WithTimeout(rootCtx, 5*time.Second)
		defer cancel()

		if err := dbPool.Ping(pingCtx); err != nil {
			log.Fatal().Err(err).Msg("postgres ping failed")
		}
		log.Info().Msg("postgres connected")
	}

	repo := postgres.New(dbPool)

	// ---- Redis ----
	// NOTE: this assumes your redis package exposes New(addr, pass, db) and returns a type
	// implementing domain.CacheRepository (and optionally has Client.Ping)
	cache := redis.New(cfg.RedisAddr, cfg.RedisPass, cfg.RedisDB)

	// If your redis struct exposes Client, keep the ping. If not, remove this block.
	{
		pingCtx, cancel := context.WithTimeout(rootCtx, 2*time.Second)
		defer cancel()

		// Best-effort ping; don't kill service if redis is optional
		if cache != nil && cache.Client != nil {
			if err := cache.Client.Ping(pingCtx).Err(); err != nil {
				log.Warn().Err(err).Msg("redis ping failed (continuing)")
			} else {
				log.Info().Msg("redis connected")
			}
		} else {
			log.Info().Msg("redis configured")
		}
	}

	// ---- Application service ----
	svc := service.NewJoinService(repo, cache)
	h := rest.NewHandler(svc)

	// ---- JWT verifier ----
	verifier := security.NewHS256Verifier(cfg.JWTSecret)

	// ---- Router ----
	httpHandler := rest.NewRouter(rest.RouterDeps{
		Cache:     cache,
		Handler:   h,
		Verifier:  verifier,
		JWTIssuer: cfg.JWTIssuer,
		RLLimit:   cfg.RLLimit,
		RLWindow:  cfg.RLWindow,
	})

	// ---- MQ consumer (inbound snapshots from event-service) ----
	// This matches the consumer signature I gave you: NewConsumer(url, exchange, repo)
	mqConsumer := rabbitmq.NewConsumer(cfg.RabbitURL, cfg.RabbitExchange, repo)
	mqConsumer.Start(rootCtx)

	// ---- Outbox worker (outbound join.* events) ----
	if cfg.OutboxEnabled {
		repo.StartOutboxWorker(rootCtx, cfg.RabbitURL, cfg.RabbitExchange)
		log.Info().Msg("outbox worker started")
	}

	// ---- HTTP server ----
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           httpHandler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Start server
	errCh := make(chan error, 1)
	go func() {
		log.Info().Int("port", cfg.Port).Msg("http server starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for shutdown signal or server crash
	select {
	case <-rootCtx.Done():
		log.Info().Msg("shutdown signal received")
	case err := <-errCh:
		log.Error().Err(err).Msg("http server crashed")
	}

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	log.Info().Msg("shutdown complete")
}
