package main

import (
	"github.com/baechuer/cityevents/internal/platform/app"
	"github.com/baechuer/cityevents/internal/services/media"
)

func main() {
	app.Main("media-service", media.NewRouter)
}
