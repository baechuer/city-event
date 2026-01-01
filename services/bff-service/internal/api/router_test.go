package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/baechuer/real-time-ressys/services/bff-service/internal/api"
	"github.com/baechuer/real-time-ressys/services/bff-service/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestRouter_Integration(t *testing.T) {
	// 1. Start Fake Microservices
	fakeAuth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/auth/v1/login", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"token":"fake-jwt"}`))
	}))
	defer fakeAuth.Close()

	fakeEvent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/event/v1/events", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"items":[]}`))
	}))
	defer fakeEvent.Close()

	// 2. Setup Config pointing to fakes
	cfg := &config.Config{
		Port:            "8080",
		AuthServiceURL:  fakeAuth.URL,
		EventServiceURL: fakeEvent.URL,
	}

	// 3. Create Router
	router := api.NewRouter(cfg)

	t.Run("Healthz", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/healthz", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "OK", w.Body.String())
	})

	t.Run("Auth Proxy", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/auth/login", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"token":"fake-jwt"}`, w.Body.String())
	})

	t.Run("Event Proxy", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/events", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"items":[]}`, w.Body.String())
	})

	t.Run("404 for unknown", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/unknown", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
