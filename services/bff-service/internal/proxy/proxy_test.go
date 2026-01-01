package proxy_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/baechuer/real-time-ressys/services/bff-service/internal/proxy"
	"github.com/baechuer/real-time-ressys/services/bff-service/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxy_PathRewrite(t *testing.T) {
	// 1. Mock Upstream with result channel
	hostCh := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/auth/v1/login", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		hostCh <- r.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	// 2. Create Proxy
	p, err := proxy.New(upstream.URL, "/api/auth", "/auth/v1")
	require.NoError(t, err)

	// 3. Send Request
	req := httptest.NewRequest("POST", "http://bff/api/auth/login", nil)
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify Host rewritten correctly
	select {
	case gotHost := <-hostCh:
		u, _ := url.Parse(upstream.URL)
		assert.Equal(t, u.Host, gotHost)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for upstream request")
	}
}

func TestProxy_HeadersPropagation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-req-id", r.Header.Get("X-Request-Id"))
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	p, err := proxy.New(upstream.URL, "/api/events", "/event/v1/events")
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "http://bff/api/events", nil)
	// Inject ID into usage context
	ctx := middleware.SetRequestIDForTest(req.Context(), "test-req-id")

	w := httptest.NewRecorder()
	p.ServeHTTP(w, req.WithContext(ctx))

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestProxy_UpstreamDown(t *testing.T) {
	// Create a proxy to a port that is definitely closed
	p, err := proxy.New("http://localhost:54321", "/api/fail", "/v1")
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "http://bff/api/fail", nil)
	ctx := middleware.SetRequestIDForTest(req.Context(), "req-123")
	w := httptest.NewRecorder()

	// This should fail quickly and trigger ErrorHandler
	// But httputil might retry or take time depending on OS TCP timeout if not set?
	// The default transport dial timeout is usually 30s.
	// For unit test safety, we configure transport with short timeout,
	// but our `proxy.New` doesn't expose transport customization easily (encapsulated).
	// To avoid slow tests, we can use a Cancel context.
	ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	p.ServeHTTP(w, req.WithContext(ctx))

	// Verify 502 Bad Gateway
	assert.Equal(t, http.StatusBadGateway, w.Code)
	assert.Contains(t, w.Body.String(), "upstream_unavailable")
	assert.Contains(t, w.Body.String(), "req-123")
}
