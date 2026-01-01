package main

import (
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	customMiddleware "github.com/baechuer/real-time-ressys/services/bff-service/middleware"
)

func main() {
	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = "8080"
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(customMiddleware.RequestID)
	r.Use(customMiddleware.SecurityHeaders)

	r.Get("/api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	log.Printf("BFF Service starting on :%s", port)
	http.ListenAndServe(":"+port, r)
}
