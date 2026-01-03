package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"net/url"
	"time"

	_ "github.com/lib/pq"

	"github.com/baechuer/real-time-ressys/services/event-service/internal/application/event"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/config"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/infrastructure/caching/redis"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/infrastructure/db/postgres"
	rabbitpub "github.com/baechuer/real-time-ressys/services/event-service/internal/infrastructure/messaging/rabbitmq"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/logger"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/transport/http/handlers"
	authmw "github.com/baechuer/real-time-ressys/services/event-service/internal/transport/http/middleware"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/transport/http/router"
	go_redis "github.com/redis/go-redis/v9"
	zlog "github.com/rs/zerolog/log"
)

// sysClock implements event.Clock interface using system time
type sysClock struct{}

func (sysClock) Now() time.Time { return time.Now().UTC() }

// App holds all dependencies for the service
type App struct {
	Config *config.Config
	Server *http.Server
	DB     *sql.DB

	Publisher  *rabbitpub.Publisher
	RedisCache *redis.Client
}

func main() {
	// 1) Load Config first
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	// 2) Init logger (your project’s logger.Init() takes no args)
	logger.Init()

	u, _ := url.Parse(cfg.DatabaseURL)
	zlog.Info().
		Str("db_user", u.User.Username()).
		Str("db_host", u.Host).
		Str("db_db", u.Path).
		Msg("db config loaded")

	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		zlog.Fatal().Err(err).Msg("db open failed")
	}
	defer db.Close()

	{
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			zlog.Fatal().Err(err).Msg("db ping failed")
		}
	}

	app := NewApp(cfg, db)
	defer func() {
		if app.Publisher != nil {
			_ = app.Publisher.Close()
		}
		if app.RedisCache != nil {
			_ = app.RedisCache.Close()
		}
	}()

	zlog.Info().Str("addr", cfg.HTTPAddr).Msg("listening")
	if err := app.Server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		zlog.Fatal().Err(err).Msg("server crashed")
	}
}

func NewApp(cfg *config.Config, db *sql.DB) *App {
	repo := postgres.New(db)

	// Rabbit publisher (optional)
	var rabbit *rabbitpub.Publisher
	var pub event.EventPublisher = event.NoopPublisher{}

	if cfg.RabbitURL != "" {
		var p *rabbitpub.Publisher
		var err error

		// Retry connecting to RabbitMQ
		for i := 0; i < 15; i++ {
			p, err = rabbitpub.NewPublisher(cfg.RabbitURL, cfg.RabbitExchange)
			if err == nil {
				break
			}
			zlog.Warn().Err(err).Msg("rabbit publisher init failed, retrying in 2s...")
			time.Sleep(2 * time.Second)
		}

		if err != nil {
			zlog.Fatal().Err(err).Msg("rabbit publisher init failed after retries")
		}
		rabbit = p
		pub = p

		// ✅ Start outbox worker only when RabbitMQ is configured
		// Outbox worker will publish pending rows using stable message_id.
		repo.StartOutboxWorker(context.Background(), pub)
	}

	// Redis wiring
	var rc *redis.Client                  // your wrapper
	var cache event.Cache                 // used by application caching
	var rawRedisInstance *go_redis.Client // used by rate limit middleware/router

	if cfg.RedisURL != "" {
		c, err := redis.New(cfg.RedisURL)
		if err != nil {
			zlog.Warn().Err(err).Msg("redis connect failed")
		} else {
			rc = c
			cache = c
			rawRedisInstance = c.GetRawClient()
			zlog.Info().Msg("redis cache ready")
		}
	}

	// ✅ IMPORTANT: event.New signature changed (publisher removed).
	// Publishing is now done via outbox worker, not in request path.
	svc := event.New(repo, sysClock{}, cache, cfg.CacheTTLDetails, cfg.CacheTTLList)

	// ✅ Start consumer to listen for join events (after service is created)
	if cfg.RabbitURL != "" {
		var consumer *rabbitpub.Consumer
		var err error

		for i := 0; i < 15; i++ {
			consumer, err = rabbitpub.NewConsumer(cfg.RabbitURL, cfg.RabbitExchange, svc)
			if err == nil {
				break
			}
			zlog.Warn().Err(err).Msg("rabbit consumer init failed, retrying in 2s...")
			time.Sleep(2 * time.Second)
		}

		if err != nil {
			zlog.Fatal().Err(err).Msg("rabbit consumer init failed after retries")
		}
		consumer.Start(context.Background())
	}

	h := handlers.NewEventsHandler(svc, sysClock{})
	auth := authmw.NewAuth(cfg.JWTSecret, cfg.JWTIssuer, rc)
	z := handlers.NewHealthHandler()

	// router uses rawRedisInstance for middleware (if any)
	httpHandler := router.New(h, auth, z, db, rawRedisInstance, cfg)

	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      httpHandler,
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
		IdleTimeout:  cfg.HTTPIdleTimeout,
	}

	return &App{
		Config:     cfg,
		Server:     srv,
		DB:         db,
		Publisher:  rabbit,
		RedisCache: rc,
	}
}
