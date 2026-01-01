package main

import (
	"net/http"

	"github.com/baechuer/real-time-ressys/services/bff-service/internal/api"
	"github.com/baechuer/real-time-ressys/services/bff-service/internal/config"
	"github.com/baechuer/real-time-ressys/services/bff-service/internal/logger"
	zlog "github.com/rs/zerolog/log"
)

func main() {
	// 1. Load Config
	cfg := config.Load()

	// 1.5 Init Logger
	logger.Init()
	zlog.Info().Msg("logger initialized")

	// 2. Setup Router
	r := api.NewRouter(cfg)

	// 3. Start Server
	zlog.Info().Str("port", cfg.Port).Msg("BFF Service starting")
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		zlog.Fatal().Err(err).Msg("Server failed")
	}
}
