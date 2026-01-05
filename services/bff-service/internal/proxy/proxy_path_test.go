package proxy_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/baechuer/real-time-ressys/services/bff-service/internal/proxy"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
)

func TestProxy_PathRewriting(t *testing.T) {
	// Track what path the fake auth server receives
	var receivedPath string

	fakeAuth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		t.Logf("Fake Auth received: Method=%s Path=%s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer fakeAuth.Close()

	// Create proxy like in router.go (with the fixed stripPrefix)
	authProxy, err := proxy.New(fakeAuth.URL, "/api/auth", "/auth/v1")
	assert.NoError(t, err)

	// Create router with Chi (mimicking router.go structure)
	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) {
		r.Mount("/auth", authProxy)
	})

	testCases := []struct {
		name         string
		requestPath  string
		expectedPath string
	}{
		{
			name:         "register endpoint",
			requestPath:  "/api/auth/register",
			expectedPath: "/auth/v1/register",
		},
		{
			name:         "login endpoint",
			requestPath:  "/api/auth/login",
			expectedPath: "/auth/v1/login",
		},
		{
			name:         "oauth callback",
			requestPath:  "/api/auth/oauth/google/callback",
			expectedPath: "/auth/v1/oauth/google/callback",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			receivedPath = ""
			req := httptest.NewRequest("POST", tc.requestPath, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			t.Logf("Request: %s -> Received by upstream: %s", tc.requestPath, receivedPath)
			assert.Equal(t, tc.expectedPath, receivedPath, fmt.Sprintf("Expected upstream to receive %s but got %s", tc.expectedPath, receivedPath))
		})
	}
}
