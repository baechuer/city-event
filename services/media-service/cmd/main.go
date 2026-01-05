package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/baechuer/cityevents/services/media-service/internal/cleanup"
	"github.com/baechuer/cityevents/services/media-service/internal/config"
	"github.com/baechuer/cityevents/services/media-service/internal/handler"
	"github.com/baechuer/cityevents/services/media-service/internal/messaging"
	"github.com/baechuer/cityevents/services/media-service/internal/repository"
	"github.com/baechuer/cityevents/services/media-service/internal/storage"
)

func main() {
	log := zerolog.New(os.Stdout).With().Timestamp().Logger()

	cfg := config.Load()
	log.Info().Str("addr", cfg.HTTPAddr).Msg("starting media-service")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect to PostgreSQL
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer pool.Close()

	// Initialize S3 client
	s3Client, err := storage.NewS3Client(cfg, log)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create S3 client")
	}

	// Ensure buckets exist
	if err := s3Client.EnsureBuckets(ctx); err != nil {
		log.Error().Err(err).Msg("failed to ensure buckets exist")
	}

	// Initialize RabbitMQ publisher
	publisher, err := messaging.NewPublisher(cfg, log)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create RabbitMQ publisher")
	}
	defer publisher.Close()

	// Initialize repository and handler
	uploadRepo := repository.NewUploadRepository(pool)
	uploadHandler := handler.NewUploadHandler(uploadRepo, s3Client, publisher, cfg, log)

	// Initialize and run cleaner
	cleaner := cleanup.NewCleaner(uploadRepo, s3Client, log)
	go cleaner.Run(ctx)

	// Setup router
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Health check
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("db not ready"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// API routes
	r.Route("/media/v1", func(r chi.Router) {
		r.Post("/request-upload", uploadHandler.RequestUpload)
		r.Post("/complete", uploadHandler.CompleteUpload)
		r.Get("/status/{id}", uploadHandler.GetStatus)
	})

	// Start server
	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	log.Info().Str("addr", cfg.HTTPAddr).Msg("media-service started")

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("shutting down media-service")
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}
