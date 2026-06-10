package main

import (
	"github.com/baechuer/cityevents/internal/platform/app"
	"github.com/baechuer/cityevents/internal/services/gateway"
)

func main() {
	app.Main("api-gateway", gateway.NewRouter)
}
