package logging

import (
	"log/slog"
	"os"
	"strings"
)

func New(serviceName, environment string) *slog.Logger {
	level := slog.LevelInfo
	if strings.EqualFold(os.Getenv("LOG_LEVEL"), "debug") {
		level = slog.LevelDebug
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(handler).With(
		slog.String("service", serviceName),
		slog.String("environment", environment),
	)
}
