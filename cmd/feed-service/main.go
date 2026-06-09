package main

import (
	"github.com/baechuer/cityevents/internal/platform/app"
	"github.com/baechuer/cityevents/internal/services/feed"
)

func main() {
	app.Main("feed-service", feed.NewRouter)
}
