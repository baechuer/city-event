//go:build integration

package infra

import (
	"fmt"
	"os"
)

type Env struct {
	PostgresDSN string
	RedisAddr   string
	RabbitURL   string
}

func LoadEnv() (Env, error) {
	pg := getenv("IT_PG_DSN", "postgres://postgres:postgres@localhost:5432/auth_db?sslmode=disable")
	redis := getenv("IT_REDIS_ADDR", "localhost:6379")

	// ✅ compose: RABBITMQ_DEFAULT_USER=guest, PASS=guest
	rabbit := getenv("IT_RABBIT_URL", "amqp://guest:guest@localhost:5672/")

	return Env{
		PostgresDSN: pg,
		RedisAddr:   redis,
		RabbitURL:   rabbit,
	}, nil
}

func getenv(k, def string) string {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v
}

func (e Env) String() string {
	return fmt.Sprintf("Env{PostgresDSN=%q RedisAddr=%q RabbitURL=%q}", e.PostgresDSN, e.RedisAddr, e.RabbitURL)
}
