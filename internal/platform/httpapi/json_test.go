package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONLimitedRejectsLargeBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"abcdef"}`))
	rec := httptest.NewRecorder()

	var payload struct {
		Name string `json:"name"`
	}
	err := DecodeJSONLimited(rec, req, &payload, 8)
	if !errors.Is(err, ErrRequestBodyTooLarge) {
		t.Fatalf("DecodeJSONLimited error = %v, want ErrRequestBodyTooLarge", err)
	}
}

func TestDecodeJSONLimitedRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "unknown field", body: `{"name":"ok","extra":true}`},
		{name: "trailing json", body: `{"name":"ok"}{"name":"again"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			var payload struct {
				Name string `json:"name"`
			}
			err := DecodeJSONLimited(rec, req, &payload, 1024)
			if !errors.Is(err, ErrInvalidJSON) {
				t.Fatalf("DecodeJSONLimited error = %v, want ErrInvalidJSON", err)
			}
		})
	}
}
