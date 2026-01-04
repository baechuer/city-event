package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/application/auth"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/config"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/domain"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/infrastructure/db/postgres"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/infrastructure/memory"
	rabbitmq_pub "github.com/baechuer/real-time-ressys/services/auth-service/internal/infrastructure/messaging/rabbitmq"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/infrastructure/oauth"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/infrastructure/redis"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/infrastructure/security"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/logger"
	http_handlers "github.com/baechuer/real-time-ressys/services/auth-service/internal/transport/http/handlers"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/transport/http/middleware"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/transport/http/response"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/transport/http/router"
)

/*
========================
 Public entry (prod)
========================
*/

func NewServer() (*http.Server, func(), error) {
	return newServer(defaultDeps())
}

// NewServerWithDeps allows injecting dependencies for testing
func NewServerWithDeps(deps Deps) (*http.Server, func(), error) {
	return newServer(deps)
}

/*
========================
 Dependency injection
========================
*/

type Deps struct {
	LoadConfig func() (*config.Config, error)

	NewDB func(addr string, debug bool) (DBCloser, error)

	NewRedis func(addr, password string, db int) RedisClient

	NewPublisher func(rabbitURL string) (Publisher, error)

	NewRouter func(router.Deps) (http.Handler, error)

	NewOAuthProvider func() auth.OAuthProvider
}

type DBCloser interface {
	Close() error
}

type RedisClient interface {
	Ping(ctx context.Context) error
	Close() error
}

type Publisher interface{}

/*
========================
 Core bootstrap logic
========================
*/

func newServer(deps Deps) (*http.Server, func(), error) {
	// 0) config
	cfg, err := deps.LoadConfig()
	if err != nil {
		return nil, nil, err
	}

	// 1) db
	db, err := deps.NewDB(cfg.DBAddr, cfg.DBDebug)
	if err != nil {
		return nil, nil, err
	}

	cleanupFns := []func(){
		func() { _ = db.Close() },
	}

	// 2) user repo
	sqlDB, ok := db.(*sql.DB)
	if !ok {
		runCleanup(cleanupFns)
		return nil, nil, errors.New("bootstrap: NewDB did not return *sql.DB")
	}

	userRepo := postgres.NewUserRepo(sqlDB)

	// 3) redis (best-effort)
	var redisCli RedisClient
	if deps.NewRedis != nil {
		c := deps.NewRedis(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := c.Ping(ctx); err != nil {
			logger.Logger.Warn().Err(err).Msg("redis unavailable; cache disabled")
			_ = c.Close()
		} else {
			logger.Logger.Info().Msg("redis connected")
			redisCli = c
			cleanupFns = append(cleanupFns, func() { _ = c.Close() })
		}
	}

	// wrap repo with cache
	var userRepoCached auth.UserRepo = userRepo
	if redisCli != nil {
		userRepoCached = redis.NewCachedUserRepo(
			userRepo,
			redisCli.(*redis.Client),
			cfg.TokenVersionCacheTTL,
		)
	}

	// 4) session + OTT stores
	var sessionStore auth.SessionStore
	var ottStore auth.OneTimeTokenStore
	var oauthStateStore auth.OAuthStateStore

	if redisCli != nil {
		sessionStore = redis.NewRedisSessionStore(redisCli.(*redis.Client))
		ottStore = redis.NewOneTimeTokenStore(redisCli.(*redis.Client))
		oauthStateStore = redis.NewOAuthStateStore(redisCli.(*redis.Client), cfg.OAuthStateTTL)
	} else {
		sessionStore = memory.NewSessionStore()
		ottStore = memory.NewOneTimeTokenStore()
		oauthStateStore = memory.NewOAuthStateStore()
	}

	// OAuth Repo
	oauthRepo := postgres.NewOAuthIdentityRepo(sqlDB)

	// OAuth Client
	var googleClient auth.OAuthProvider
	if deps.NewOAuthProvider != nil {
		googleClient = deps.NewOAuthProvider()
	} else {
		googleClient = oauth.NewGoogleClient(
			cfg.GoogleClientID,
			cfg.GoogleClientSecret,
			cfg.OAuthCallbackURL,
		)
	}

	// 5) publisher
	pub, err := deps.NewPublisher(cfg.RabbitURL)
	if err != nil {
		if cfg.Env == "dev" {
			logger.Logger.Warn().Err(err).Msg("rabbitmq unavailable; using noop publisher")
			pub = memory.NewNoopPublisher()
		} else {
			runCleanup(cleanupFns)
			return nil, nil, err
		}
	} else {
		// Configure exchange if supported
		if p, ok := pub.(interface{ SetExchange(string) }); ok {
			p.SetExchange(cfg.RabbitExchange)
		}
	}

	if c, ok := pub.(interface{ Close() error }); ok {
		cleanupFns = append(cleanupFns, func() { _ = c.Close() })
	}

	// 6) security
	logger.Logger.Info().Str("issuer", cfg.JWTIssuer).Msg("initializing jwt signer")
	hasher := security.NewBcryptHasher(12)
	signer := security.NewJWTSigner(cfg.JWTSecret, cfg.JWTIssuer)

	// seed (dev only)
	if cfg.Env == "dev" {
		postgres.SeedUsers(context.Background(), userRepo, hasher)
	}

	// 7) service
	authSvc := auth.NewService(
		userRepoCached,
		hasher,
		signer,
		sessionStore,
		ottStore,
		pub.(auth.EventPublisher),
		auth.Config{
			AccessTTL:             cfg.AccessTokenTTL,
			RefreshTTL:            cfg.RefreshTokenTTL,
			VerifyEmailBaseURL:    cfg.VerifyEmailBaseURL,
			PasswordResetBaseURL:  cfg.PasswordResetBaseURL,
			VerifyEmailTokenTTL:   cfg.VerifyEmailTokenTTL,
			PasswordResetTokenTTL: cfg.PasswordResetTokenTTL,
		},
	)

	authSvc = authSvc.WithAudit(func(action string, fields map[string]string) {
		evt := logger.Logger.Info().
			Bool("audit", true).
			Str("action", action)
		for k, v := range fields {
			evt = evt.Str(k, v)
		}
		evt.Msg("audit")
	})

	// 8) handlers + middleware
	secureCookies := cfg.Env != "dev"

	authH := http_handlers.NewAuthHandler(authSvc, cfg.RefreshTokenTTL, secureCookies, cfg.CDNBaseURL)
	healthH := http_handlers.NewHealthHandler(sqlDB)

	oauthH := http_handlers.NewOAuthHandler(http_handlers.OAuthHandlerConfig{
		Service:          authSvc,
		GoogleClient:     googleClient,
		StateStore:       oauthStateStore,
		OAuthIdentities:  oauthRepo,
		FrontendOrigin:   cfg.FrontendOrigin,
		AllowedRedirects: cfg.AllowedRedirects,
		IsSecure:         secureCookies,
	})

	authMW := middleware.Auth(signer, userRepoCached, response.WriteError)
	modMW := middleware.RequireAtLeast(string(domain.RoleModerator), response.WriteError)
	adminMW := middleware.RequireAtLeast("admin", response.WriteError)

	// rate limit (fail-open)
	var fwLimiter *redis.FixedWindowLimiter
	if redisCli != nil {
		fwLimiter = redis.NewFixedWindowLimiter(redisCli.(*redis.Client))
	}

	rl := func(key string, limit int, window time.Duration) func(http.Handler) http.Handler {
		if fwLimiter == nil {
			return nil
		}
		return middleware.RateLimitFixedWindow(
			fwLimiter,
			middleware.FixedWindowConfig{
				RouteKey: key,
				Limit:    limit,
				Window:   window,
			},
			response.WriteError,
		)
	}

	// 9) router
	mux, err := deps.NewRouter(router.Deps{
		RequestIDMW:    middleware.RequestID,
		Auth:           authH,
		OAuth:          oauthH,
		Health:         healthH,
		AuthMW:         authMW,
		ModMW:          modMW,
		AdminMW:        adminMW,
		InternalAuthMW: middleware.InternalAuth(cfg.InternalSecret),

		RLRegister:             rl("auth.register", 3, time.Minute),
		RLLogin:                rl("auth.login", 5, time.Minute),
		RLRefresh:              rl("auth.refresh", 10, time.Minute),
		RLLogout:               rl("auth.logout", 30, time.Minute),
		RLVerifyEmailRequest:   rl("auth.verify_email.request", 3, 10*time.Minute),
		RLPasswordResetRequest: rl("auth.password_reset.request", 3, 10*time.Minute),
		RLPasswordChange:       rl("auth.password.change", 5, time.Minute),
		RLSessionsRevoke:       rl("auth.sessions.revoke", 5, time.Minute),
		RLModActions:           rl("auth.mod.actions", 30, time.Minute),
		RLAdminActions:         rl("auth.admin.actions", 60, time.Minute),
	})
	if err != nil {
		runCleanup(cleanupFns)
		return nil, nil, err
	}

	// 10) server
	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      mux,
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
		IdleTimeout:  cfg.HTTPIdleTimeout,
	}

	cleanup := func() {
		runCleanup(cleanupFns)
	}

	return srv, cleanup, nil
}

/*
========================
 Default deps (prod)
========================
*/

func defaultDeps() Deps {
	return Deps{
		LoadConfig: config.Load,
		NewDB: func(addr string, debug bool) (DBCloser, error) {
			return config.NewDB(addr, debug)
		},
		NewRedis: func(addr, password string, db int) RedisClient {
			return redis.New(addr, password, db)
		},
		NewPublisher: func(url string) (Publisher, error) {
			return rabbitmq_pub.NewPublisher(url)
		},
		NewRouter: func(d router.Deps) (http.Handler, error) {
			return router.New(d)
		},
	}
}

/*
========================
 helpers
========================
*/

func runCleanup(fns []func()) {
	for i := len(fns) - 1; i >= 0; i-- {
		fns[i]()
	}
}
