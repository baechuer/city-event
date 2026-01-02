package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/baechuer/real-time-ressys/services/bff-service/internal/api"
	"github.com/baechuer/real-time-ressys/services/bff-service/internal/config"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestRouter_Integration(t *testing.T) {
	// 1. Start Fake Microservices
	fakeAuth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"token":"fake-jwt"}`))
	}))
	defer fakeAuth.Close()

	fakeEvent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Must match downstream.dataEnvelope + pagination fields
		w.Write([]byte(`{"data":{"items":[],"next_cursor":"","has_more":false}}`))
	}))
	defer fakeEvent.Close()

	fakeJoin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer fakeJoin.Close()

	// 2. Setup Config pointing to fakes
	secret := "secret"
	cfg := &config.Config{
		Port:            "8080",
		AuthServiceURL:  fakeAuth.URL,
		EventServiceURL: fakeEvent.URL,
		JoinServiceURL:  fakeJoin.URL,
		JWTSecret:       secret,
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
		// Generate Token
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"uid": uuid.New().String(),
		})
		tokenString, _ := token.SignedString([]byte(secret))

		req := httptest.NewRequest("GET", "/api/events", nil)
		req.Header.Set("Authorization", "Bearer "+tokenString)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if !assert.Equal(t, http.StatusOK, w.Code) {
			t.Logf("Status code mismatch: got %d, body: %s", w.Code, w.Body.String())
		}
		if !assert.JSONEq(t, `{"items":[],"next_cursor":"","has_more":false}`, w.Body.String()) {
			t.Logf("JSON mismatch. Body: %s", w.Body.String())
		}
	})

	t.Run("404 for unknown", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/unknown", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
