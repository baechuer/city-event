package health

import (
	"encoding/json"
	"net/http"
	"time"
)

type Response struct {
	Status      string            `json:"status"`
	Service     string            `json:"service"`
	Environment string            `json:"environment"`
	Timestamp   time.Time         `json:"timestamp"`
	Checks      map[string]string `json:"checks"`
}

func Live(serviceName, environment string) Response {
	return Response{
		Status:      "ok",
		Service:     serviceName,
		Environment: environment,
		Timestamp:   time.Now().UTC(),
		Checks: map[string]string{
			"process": "ok",
		},
	}
}

func Ready(serviceName, environment string) Response {
	return Response{
		Status:      "ok",
		Service:     serviceName,
		Environment: environment,
		Timestamp:   time.Now().UTC(),
		Checks: map[string]string{
			"config": "ok",
		},
	}
}

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
