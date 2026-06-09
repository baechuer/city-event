package main

import (
	"github.com/baechuer/cityevents/internal/platform/app"
	"github.com/baechuer/cityevents/internal/services/auth"
)

func main() {
	app.Main("auth-service", auth.NewRouter)
}
