package http_handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

type HealthHandler struct {
	db *sql.DB
}

func NewHealthHandler(db *sql.DB) *HealthHandler {
	return &HealthHandler{db: db}
}

// Healthz handles GET /healthz
func (h *HealthHandler) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Readyz handles GET /readyz
func (h *HealthHandler) Readyz(w http.ResponseWriter, r *http.Request) {
	if h.db != nil {
		if err := h.db.PingContext(r.Context()); err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "unavailable",
				"error":  "database unavailable",
			})
			return
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}
