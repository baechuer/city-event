package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, CorrelationIDHeader) {
		t.Fatalf("allow headers = %q", got)
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
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if got := rec.Header().Get(CorrelationIDHeader); got != "corr-test" {
		t.Fatalf("correlation id = %q", got)
	}

	generatedReq := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	generatedRec := httptest.NewRecorder()
	router.ServeHTTP(generatedRec, generatedReq)
	if got := generatedRec.Header().Get(CorrelationIDHeader); got == "" {
		t.Fatal("expected generated correlation id")
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
