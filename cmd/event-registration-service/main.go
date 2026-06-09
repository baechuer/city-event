package main

import (
	"github.com/baechuer/cityevents/internal/platform/app"
	eventregistration "github.com/baechuer/cityevents/internal/services/eventregistration"
)

func main() {
	app.Main("event-registration-service", eventregistration.NewRouter)
}
