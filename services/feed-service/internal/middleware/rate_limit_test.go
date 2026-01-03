package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)

	// First 5 should be allowed
	for i := 0; i < 5; i++ {
		if !rl.Allow("test-key") {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// 6th should be denied
	if rl.Allow("test-key") {
		t.Error("6th request should be denied")
	}
}

func TestRateLimiter_DifferentKeys(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)

	// key1 uses 2 requests
	rl.Allow("key1")
	rl.Allow("key1")

	// key1 should be denied
	if rl.Allow("key1") {
		t.Error("key1 should be denied")
	}

	// key2 should still be allowed
	if !rl.Allow("key2") {
		t.Error("key2 should be allowed")
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	actorLimiter := NewRateLimiter(2, time.Minute)
	ipLimiter := NewRateLimiter(100, time.Minute)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	limited := RateLimit(actorLimiter, ipLimiter)(handler)

	// Simulate requests without anon_id (IP-only limiting)
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		rr := httptest.NewRecorder()
		limited.ServeHTTP(rr, req)
		// All should pass since we're using IP limiter which allows 100
	}
}
