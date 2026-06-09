package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/baechuer/cityevents/internal/platform/config"
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
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight returned %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("allow origin = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "X-User-ID") {
		t.Fatalf("allow headers = %q", got)
	}
}
