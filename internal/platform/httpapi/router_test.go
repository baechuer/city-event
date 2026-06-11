package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/baechuer/cityevents/internal/platform/config"
	"github.com/baechuer/cityevents/internal/platform/observability"
)

func TestEveryServiceExposesHealthEndpoints(t *testing.T) {
	for _, service := range config.Services() {
		t.Run(service.Name, func(t *testing.T) {
			cfg, err := config.Load(service.Name, nil)
			if err != nil {
				t.Fatalf("load config: %v", err)
			}
			router := NewRouter(cfg, nil)

			for _, path := range []string{"/livez", "/readyz"} {
				req := httptest.NewRequest(http.MethodGet, path, nil)
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)

				if rec.Code != http.StatusOK {
					t.Fatalf("%s returned %d, want 200", path, rec.Code)
				}

				var body struct {
					Status  string `json:"status"`
					Service string `json:"service"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatalf("decode %s response: %v", path, err)
				}
				if body.Status != "ok" {
					t.Fatalf("%s status = %q, want ok", path, body.Status)
				}
				if body.Service != service.Name {
					t.Fatalf("%s service = %q, want %q", path, body.Service, service.Name)
				}
			}
		})
	}
}

func TestRootEndpointIdentifiesService(t *testing.T) {
	cfg, err := config.Load("api-gateway", nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	router := NewRouter(cfg, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("root returned %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "api-gateway") {
		t.Fatalf("expected root response to include service name, got %s", rec.Body.String())
	}
}

func TestCORSPreflight(t *testing.T) {
	cfg, err := config.Load("api-gateway", nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	router := NewRouter(cfg, nil)

	req := httptest.NewRequest(http.MethodOptions, "/v1/events", nil)
	req.Header.Set("Origin", "http://127.0.0.1:18088")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight returned %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:18088" {
		t.Fatalf("allow origin = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("allow credentials = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "X-User-ID") {
		t.Fatalf("allow headers = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "X-CSRF-Token") {
		t.Fatalf("allow headers = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, CorrelationIDHeader) {
		t.Fatalf("allow headers = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, TraceParentHeader) || !strings.Contains(got, TraceStateHeader) {
		t.Fatalf("allow headers = %q", got)
	}
}

func TestCORSDoesNotTreatWildcardAsCredentialedOrigin(t *testing.T) {
	envs := map[string]string{
		"CORS_ALLOWED_ORIGINS": "*",
	}
	cfg, err := config.Load("api-gateway", func(key string) string {
		return envs[key]
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	router := NewRouter(cfg, nil)

	req := httptest.NewRequest(http.MethodOptions, "/v1/events", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("wildcard should not be accepted for credentialed CORS, got %q", got)
	}
}

func TestCorrelationIDAndMetrics(t *testing.T) {
	cfg, err := config.Load("api-gateway", nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	router := NewRouter(cfg, nil)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	req.Header.Set(CorrelationIDHeader, "corr-test")
	req.Header.Set(TraceParentHeader, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if got := rec.Header().Get(CorrelationIDHeader); got != "corr-test" {
		t.Fatalf("correlation id = %q", got)
	}
	if got := rec.Header().Get(TraceParentHeader); got != "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01" {
		t.Fatalf("traceparent = %q", got)
	}

	generatedReq := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	generatedRec := httptest.NewRecorder()
	router.ServeHTTP(generatedRec, generatedReq)
	if got := generatedRec.Header().Get(CorrelationIDHeader); got == "" {
		t.Fatal("expected generated correlation id")
	}
	if got := generatedRec.Header().Get(TraceParentHeader); !observability.ValidTraceParent(got) {
		t.Fatalf("expected generated traceparent, got %q", got)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	router.ServeHTTP(metricsRec, metricsReq)
	if metricsRec.Code != http.StatusOK {
		t.Fatalf("metrics status = %d", metricsRec.Code)
	}
	if !strings.Contains(metricsRec.Body.String(), "cityevents_http_requests_total") {
		t.Fatalf("metrics body missing request counter: %s", metricsRec.Body.String())
	}
	if !strings.Contains(metricsRec.Body.String(), "cityevents_http_request_duration_seconds_bucket") {
		t.Fatalf("metrics body missing request duration histogram: %s", metricsRec.Body.String())
	}
	if !strings.Contains(metricsRec.Body.String(), `method="GET",path="/readyz",status="200"`) {
		t.Fatalf("metrics body missing method/path/status labels: %s", metricsRec.Body.String())
	}
}

func TestRateLimitRejectsRepeatedRequests(t *testing.T) {
	cfg, err := config.Load("api-gateway", func(key string) string {
		values := map[string]string{
			"RATE_LIMIT_REQUESTS": "1",
			"RATE_LIMIT_WINDOW":   "1m",
		}
		return values[key]
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	router := NewBaseRouter(cfg, nil)
	router.Get("/limited", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	first := httptest.NewRecorder()
	firstReq := httptest.NewRequest(http.MethodGet, "/limited", nil)
	firstReq.RemoteAddr = "203.0.113.10:5000"
	router.ServeHTTP(first, firstReq)
	if first.Code != http.StatusOK {
		t.Fatalf("first request status = %d body=%s", first.Code, first.Body.String())
	}

	second := httptest.NewRecorder()
	secondReq := httptest.NewRequest(http.MethodGet, "/limited", nil)
	secondReq.RemoteAddr = "203.0.113.10:5001"
	router.ServeHTTP(second, secondReq)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d body=%s", second.Code, second.Body.String())
	}
	if got := second.Header().Get("Retry-After"); got == "" {
		t.Fatalf("expected Retry-After header")
	}
	if !strings.Contains(second.Body.String(), "rate_limited") {
		t.Fatalf("expected rate_limited response, got %s", second.Body.String())
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	router.ServeHTTP(metricsRec, metricsReq)
	if !strings.Contains(metricsRec.Body.String(), `cityevents_rate_limited_requests_total{service="api-gateway",scope="read"} 1`) {
		t.Fatalf("metrics body missing rate limit counter: %s", metricsRec.Body.String())
	}
}

func TestRateLimitUsesSharedStoreAcrossRouters(t *testing.T) {
	cfg, err := config.Load("api-gateway", func(key string) string {
		values := map[string]string{
			"RATE_LIMIT_BACKEND":  "redis",
			"RATE_LIMIT_REQUESTS": "2",
			"RATE_LIMIT_WINDOW":   "1m",
		}
		return values[key]
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Service.Name = "rate-limit-shared-store-test"
	store := newFakeRateLimitStore()

	routerA := newBaseRouterWithRateLimitStore(cfg, nil, store)
	routerA.Get("/limited", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	routerB := newBaseRouterWithRateLimitStore(cfg, nil, store)
	routerB.Get("/limited", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	for i, router := range []http.Handler{routerA, routerB} {
		req := httptest.NewRequest(http.MethodGet, "/limited", nil)
		req.RemoteAddr = "203.0.113.30:5000"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d status = %d body=%s", i+1, rec.Code, rec.Body.String())
		}
	}

	thirdReq := httptest.NewRequest(http.MethodGet, "/limited", nil)
	thirdReq.RemoteAddr = "203.0.113.30:5000"
	third := httptest.NewRecorder()
	routerB.ServeHTTP(third, thirdReq)
	if third.Code != http.StatusTooManyRequests {
		t.Fatalf("third request status = %d body=%s", third.Code, third.Body.String())
	}
	if got := third.Header().Get("Retry-After"); got == "" {
		t.Fatalf("expected Retry-After header")
	}
}

func TestRateLimitStoreErrorFailOpen(t *testing.T) {
	cfg, err := config.Load("api-gateway", func(key string) string {
		values := map[string]string{
			"RATE_LIMIT_BACKEND":         "redis",
			"RATE_LIMIT_REDIS_FAIL_OPEN": "true",
			"RATE_LIMIT_REQUESTS":        "1",
		}
		return values[key]
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Service.Name = "rate-limit-fail-open-test"
	router := newBaseRouterWithRateLimitStore(cfg, nil, &errorRateLimitStore{err: errors.New("redis unavailable")})
	router.Get("/limited", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/limited", nil)
	req.RemoteAddr = "203.0.113.31:5000"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("fail-open request status = %d body=%s", rec.Code, rec.Body.String())
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	router.ServeHTTP(metricsRec, metricsReq)
	if !strings.Contains(metricsRec.Body.String(), `cityevents_rate_limit_store_errors_total{service="rate-limit-fail-open-test",backend="redis",mode="fail_open"} 1`) {
		t.Fatalf("metrics body missing fail-open store error: %s", metricsRec.Body.String())
	}
}

func TestRateLimitStoreErrorFailClosed(t *testing.T) {
	cfg, err := config.Load("api-gateway", func(key string) string {
		values := map[string]string{
			"RATE_LIMIT_BACKEND":         "redis",
			"RATE_LIMIT_REDIS_FAIL_OPEN": "false",
			"RATE_LIMIT_REQUESTS":        "1",
		}
		return values[key]
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Service.Name = "rate-limit-fail-closed-test"
	router := newBaseRouterWithRateLimitStore(cfg, nil, &errorRateLimitStore{err: errors.New("redis unavailable")})
	router.Get("/limited", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/limited", nil)
	req.RemoteAddr = "203.0.113.32:5000"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("fail-closed request status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Retry-After"); got == "" {
		t.Fatalf("expected Retry-After header")
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	router.ServeHTTP(metricsRec, metricsReq)
	if !strings.Contains(metricsRec.Body.String(), `cityevents_rate_limit_store_errors_total{service="rate-limit-fail-closed-test",backend="redis",mode="fail_closed"} 1`) {
		t.Fatalf("metrics body missing fail-closed store error: %s", metricsRec.Body.String())
	}
}

func TestRateLimitSkipsHealthAndPreflight(t *testing.T) {
	cfg, err := config.Load("api-gateway", func(key string) string {
		values := map[string]string{
			"RATE_LIMIT_REQUESTS": "1",
			"RATE_LIMIT_WINDOW":   "1m",
		}
		return values[key]
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	router := NewRouter(cfg, nil)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		req.RemoteAddr = "203.0.113.20:5000"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("health request %d status = %d", i, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodOptions, "/v1/events", nil)
	req.Header.Set("Origin", "http://127.0.0.1:18088")
	req.RemoteAddr = "203.0.113.20:5000"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d", rec.Code)
	}
}

type fakeRateLimitStore struct {
	mu     sync.Mutex
	counts map[string]int
}

func newFakeRateLimitStore() *fakeRateLimitStore {
	return &fakeRateLimitStore{counts: map[string]int{}}
}

func (s *fakeRateLimitStore) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counts[key]++
	if s.counts[key] <= limit {
		return true, 0, nil
	}
	return false, window, nil
}

type errorRateLimitStore struct {
	err error
}

func (s *errorRateLimitStore) Allow(context.Context, string, int, time.Duration) (bool, time.Duration, error) {
	return false, 0, s.err
}

func TestOutboxAndConsumerMetrics(t *testing.T) {
	observability.RecordOutboxMessage("join.confirmed", "pending")
	observability.RecordOutboxMessage("join.confirmed", "sent")
	observability.RecordConsumerMessage("feed-projection", "join.confirmed", "processed")
	text := observability.MetricsText()
	if !strings.Contains(text, "cityevents_outbox_messages_total") || !strings.Contains(text, `routing_key="join.confirmed",state="pending"`) {
		t.Fatalf("outbox metric missing: %s", text)
	}
	if !strings.Contains(text, "cityevents_outbox_messages_total") || !strings.Contains(text, `routing_key="join.confirmed",state="sent"`) {
		t.Fatalf("outbox metric missing: %s", text)
	}
	if !strings.Contains(text, "cityevents_consumer_messages_total") || !strings.Contains(text, `consumer="feed-projection",routing_key="join.confirmed",state="processed"`) {
		t.Fatalf("consumer metric missing: %s", text)
	}
}
