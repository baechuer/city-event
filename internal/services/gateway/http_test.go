package gateway

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/baechuer/cityevents/internal/platform/config"
	"github.com/baechuer/cityevents/internal/platform/identity"
)

func TestGatewayInjectsTrustedIdentityAndStripsSpoofedHeaders(t *testing.T) {
	auth := newGatewayAuthStub(t)
	var sawEvent bool
	event := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawEvent = true
		if r.URL.Path != "/v1/events" {
			t.Fatalf("path = %q, want /v1/events", r.URL.Path)
		}
		if got := r.Header.Get(identity.HeaderUserID); got != "user-123" {
			t.Fatalf("user id header = %q, want trusted user-123", got)
		}
		if got := r.Header.Get(identity.HeaderUserRole); got != string(identity.RoleOrganizer) {
			t.Fatalf("role header = %q, want ORGANIZER", got)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer event.Close()

	router := testGatewayRouter(t, auth.URL, event.URL, "http://127.0.0.1:1", "http://127.0.0.1:1")
	req := httptest.NewRequest(http.MethodPost, "/v1/events", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set(identity.HeaderUserID, "spoofed-user")
	req.Header.Set(identity.HeaderUserRole, string(identity.RoleAdmin))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !sawEvent {
		t.Fatal("event upstream was not called")
	}
}

func TestGatewayRejectsProtectedEventRoutesWithoutToken(t *testing.T) {
	auth := newGatewayAuthStub(t)
	var eventCalls atomic.Int32
	event := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		eventCalls.Add(1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer event.Close()

	router := testGatewayRouter(t, auth.URL, event.URL, "http://127.0.0.1:1", "http://127.0.0.1:1")
	req := httptest.NewRequest(http.MethodPost, "/v1/events", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if eventCalls.Load() != 0 {
		t.Fatalf("event upstream calls = %d, want 0", eventCalls.Load())
	}
}

func TestGatewayAllowsPublicEventListWithoutAuth(t *testing.T) {
	authCalls := atomic.Int32{}
	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authCalls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer auth.Close()
	event := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(identity.HeaderUserID) != "" || r.Header.Get(identity.HeaderUserRole) != "" {
			t.Fatalf("public request should not include identity headers")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer event.Close()

	router := testGatewayRouter(t, auth.URL, event.URL, "http://127.0.0.1:1", "http://127.0.0.1:1")
	req := httptest.NewRequest(http.MethodGet, "/v1/events", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if authCalls.Load() != 0 {
		t.Fatalf("auth calls = %d, want 0", authCalls.Load())
	}
}

func TestGatewayRejectsInvalidTokenBeforeProxying(t *testing.T) {
	auth := newGatewayAuthStub(t)
	var eventCalls atomic.Int32
	event := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		eventCalls.Add(1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer event.Close()

	router := testGatewayRouter(t, auth.URL, event.URL, "http://127.0.0.1:1", "http://127.0.0.1:1")
	req := httptest.NewRequest(http.MethodPost, "/v1/events", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if eventCalls.Load() != 0 {
		t.Fatalf("event upstream calls = %d, want 0", eventCalls.Load())
	}
}

func newGatewayAuthStub(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/me" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer valid-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":{"id":"user-123","role":"ORGANIZER"}}`))
	}))
	t.Cleanup(server.Close)
	return server
}

func testGatewayRouter(t *testing.T, authURL, eventURL, feedURL, mediaURL string) http.Handler {
	t.Helper()
	cfg, err := config.Load("api-gateway", func(key string) string {
		switch key {
		case "AUTH_SERVICE_URL":
			return authURL
		case "EVENT_SERVICE_URL":
			return eventURL
		case "FEED_SERVICE_URL":
			return feedURL
		case "MEDIA_SERVICE_URL":
			return mediaURL
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	router, err := NewHTTPHandler(cfg, nil, http.DefaultTransport)
	if err != nil {
		t.Fatalf("new gateway handler: %v", err)
	}
	return router
}
