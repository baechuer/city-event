package http_handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewHealthHandler_ReturnsNonNil(t *testing.T) {
	h := NewHealthHandler(nil)
	if h == nil {
		t.Fatalf("expected non-nil handler")
	}
}

func TestHealthz_ReturnsOK_JSON(t *testing.T) {
	h := NewHealthHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	h.Healthz(rr, req)

	res := rr.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.StatusCode)
	}

	gotCT := res.Header.Get("Content-Type")
	wantCT := "application/json; charset=utf-8"
	if gotCT != wantCT {
		t.Fatalf("expected Content-Type %q, got %q", wantCT, gotCT)
	}

	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status=ok, got %q", body["status"])
	}
}
