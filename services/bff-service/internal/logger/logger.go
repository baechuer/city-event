package logger

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/baechuer/real-time-ressys/services/bff-service/middleware"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
)

var Log zerolog.Logger

func Init() {
	InitWithWriter(os.Stdout)
}

func InitWithWriter(w io.Writer) {
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}

	format := os.Getenv("LOG_FORMAT") // "json" or "console"
	if format == "" {
		format = "console"
	}

	var l zerolog.Logger
	if format == "json" {
		l = zerolog.New(w).With().Timestamp().Logger().Level(level)
	} else {
		l = zerolog.New(zerolog.ConsoleWriter{
			Out:        w,
			TimeFormat: time.RFC3339,
		}).With().Timestamp().Logger().Level(level)
	}

	Log = l
	zlog.Logger = l
}

// Ctx returns a logger with Request-ID context if available
func Ctx(ctx context.Context) *zerolog.Logger {
	reqID := middleware.GetRequestID(ctx)
	if reqID != "" {
		l := Log.With().Str("request_id", reqID).Logger()
		return &l
	}
	return &Log
}
