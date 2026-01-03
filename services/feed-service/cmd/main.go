package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/baechuer/real-time-ressys/services/feed-service/internal/api"
	"github.com/baechuer/real-time-ressys/services/feed-service/internal/api/handlers"
	"github.com/baechuer/real-time-ressys/services/feed-service/internal/config"
	"github.com/baechuer/real-time-ressys/services/feed-service/internal/infrastructure/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Database
	pool, err := pgxpool.New(context.Background(), cfg.DBAddr)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	// Repositories
	trackRepo := postgres.NewTrackRepo(pool)
	trendingRepo := postgres.NewTrendingRepo(pool)
	profileRepo := postgres.NewProfileRepo(pool)

	// Handlers
	trackHandler := handlers.NewTrackHandler(trackRepo)
	feedHandler := handlers.NewFeedHandler(trendingRepo, profileRepo)

	// Router
	router := api.NewRouter(cfg, trackHandler, feedHandler)

	// Start workers
	go runOutboxWorker(trackRepo)
	go runAggregationWorker(trendingRepo)
	go runProfileRebuildWorker(profileRepo)

	// HTTP Server
	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		log.Printf("feed-service starting on %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

// runOutboxWorker processes track outbox in a loop
func runOutboxWorker(repo *postgres.TrackRepo) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		processed, err := repo.ProcessOutbox(ctx, 100)
		if err != nil {
			log.Printf("outbox worker error: %v", err)
		} else if processed > 0 {
			log.Printf("outbox worker processed %d events", processed)
		}
		cancel()
	}
}

// runAggregationWorker updates trending stats periodically
func runAggregationWorker(repo *postgres.TrendingRepo) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// Run immediately on startup
	runAggregation(repo)

	for range ticker.C {
		runAggregation(repo)
	}
}

func runAggregation(repo *postgres.TrendingRepo) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := repo.RunAggregation(ctx); err != nil {
		log.Printf("aggregation worker error: %v", err)
	} else {
		log.Println("trending aggregation complete")
	}
}

// runProfileRebuildWorker rebuilds user profiles daily
func runProfileRebuildWorker(repo *postgres.ProfileRepo) {
	// Run at 3 AM daily
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 3, 0, 0, 0, now.Location())
		time.Sleep(time.Until(next))

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		if err := repo.RebuildProfile(ctx); err != nil {
			log.Printf("profile rebuild error: %v", err)
		} else {
			log.Println("user profile rebuild complete")
		}
		cancel()
	}
}
