package response

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/domain"
)

// ---------- helpers ----------

func mustDecodeJSONLine(t *testing.T, b []byte, dst any) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(b))
	if err := dec.Decode(dst); err != nil {
		t.Fatalf("decode json: %v, body=%q", err, string(b))
	}
}

func newReqWithBody(t *testing.T, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// ---------- DecodeJSON tests ----------

type decodeDst struct {
	A string `json:"a"`
	B int    `json:"b"`
}

func TestDecodeJSON_OK_SingleObject(t *testing.T) {
	req := newReqWithBody(t, `{"a":"x","b":1}`)

	var dst decodeDst
	if err := DecodeJSON(req, &dst); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	if dst.A != "x" || dst.B != 1 {
		t.Fatalf("unexpected dst: %+v", dst)
	}
}

func TestDecodeJSON_RejectsUnknownFields(t *testing.T) {
	req := newReqWithBody(t, `{"a":"x","b":1,"c":"oops"}`)

	var dst decodeDst
	err := DecodeJSON(req, &dst)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !domain.Is(err, "invalid_json") {
		t.Fatalf("expected invalid_json, got %v", err)
	}
}

func TestDecodeJSON_InvalidJSON_ReturnsInvalidJSON(t *testing.T) {
	req := newReqWithBody(t, `{"a":"x",`)

	var dst decodeDst
	err := DecodeJSON(req, &dst)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !domain.Is(err, "invalid_json") {
		t.Fatalf("expected invalid_json, got %v", err)
	}
}

func TestDecodeJSON_MultipleJSONValues_ReturnsInvalidJSON(t *testing.T) {
	req := newReqWithBody(t, `{}`+`{}`)

	var dst map[string]any
	err := DecodeJSON(req, &dst)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !domain.Is(err, "invalid_json") {
		t.Fatalf("expected invalid_json, got %v", err)
	}
}

// ---------- WriteError / status mapping tests ----------

func TestWriteError_DomainError_MapsStatusCodeAndPayload(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	// inject request_id
	req = req.WithContext(context.WithValue(req.Context(), "request_id", "req-123"))

	rr := httptest.NewRecorder()

	WriteError(rr, req, domain.ErrMissingField("email"))

	res := rr.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("expected content-type json, got %q", ct)
	}

	var body ErrorBody
	mustDecodeJSONLine(t, rr.Body.Bytes(), &body)

	if body.Error.Code != "missing_field" {
		t.Fatalf("expected code missing_field, got %q", body.Error.Code)
	}
	if body.Error.Message == "" {
		t.Fatalf("expected non-empty message")
	}
	if body.Error.Meta == nil || body.Error.Meta["field"] != "email" {
		t.Fatalf("expected meta.field=email, got %+v", body.Error.Meta)
	}
	if body.Error.RequestID != "req-123" {
		t.Fatalf("expected request_id req-123, got %q", body.Error.RequestID)
	}
}

func TestWriteError_NonDomainError_HidesDetailsAndReturns500(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rr := httptest.NewRecorder()

	WriteError(rr, req, assertErr("boom"))

	res := rr.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", res.StatusCode)
	}

	var body ErrorBody
	mustDecodeJSONLine(t, rr.Body.Bytes(), &body)

	if body.Error.Code != "internal_error" {
		t.Fatalf("expected internal_error, got %q", body.Error.Code)
	}
	if body.Error.Message != "internal error" {
		t.Fatalf("expected message 'internal error', got %q", body.Error.Message)
	}
	// meta should be empty/nil
	if len(body.Error.Meta) != 0 {
		t.Fatalf("expected empty meta, got %+v", body.Error.Meta)
	}
}

func TestStatusFromKind_Mapping(t *testing.T) {
	cases := []struct {
		kind domain.ErrKind
		want int
	}{
		{domain.KindValidation, http.StatusBadRequest},
		{domain.KindAuth, http.StatusUnauthorized},
		{domain.KindForbidden, http.StatusForbidden},
		{domain.KindNotFound, http.StatusNotFound},
		{domain.KindConflict, http.StatusConflict},
		{domain.KindRateLimited, http.StatusTooManyRequests},
		{domain.KindInfrastructure, http.StatusServiceUnavailable},
		{domain.KindInternal, http.StatusInternalServerError},
		{"unknown", http.StatusInternalServerError},
	}

	for _, tc := range cases {
		if got := statusFromKind(tc.kind); got != tc.want {
			t.Fatalf("kind=%q expected %d got %d", tc.kind, tc.want, got)
		}
	}
}

// ---------- RequestIDFromContext tests ----------

func TestRequestIDFromContext_NoValue_ReturnsEmpty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if got := RequestIDFromContext(req); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestRequestIDFromContext_WrongType_ReturnsEmpty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req = req.WithContext(context.WithValue(req.Context(), "request_id", 123))
	if got := RequestIDFromContext(req); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestRequestIDFromContext_StringValue_ReturnsIt(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req = req.WithContext(context.WithValue(req.Context(), "request_id", "rid-1"))
	if got := RequestIDFromContext(req); got != "rid-1" {
		t.Fatalf("expected rid-1, got %q", got)
	}
}

// ---------- Success helpers tests ----------

func TestWriteJSON_SetsDefaultContentType(t *testing.T) {
	rr := httptest.NewRecorder()

	WriteJSON(rr, http.StatusOK, map[string]any{"ok": true})

	res := rr.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("expected json content-type, got %q", ct)
	}
	// body should be valid json
	var m map[string]any
	mustDecodeJSONLine(t, rr.Body.Bytes(), &m)
	if m["ok"] != true {
		t.Fatalf("expected ok=true, got %+v", m)
	}
}

func TestWriteJSON_DoesNotOverrideExistingContentType(t *testing.T) {
	rr := httptest.NewRecorder()
	rr.Header().Set("Content-Type", "application/custom")

	WriteJSON(rr, http.StatusCreated, map[string]any{"x": 1})

	if ct := rr.Result().Header.Get("Content-Type"); ct != "application/custom" {
		t.Fatalf("expected preserve content-type, got %q", ct)
	}
}

func TestOK_WrapsEnvelope(t *testing.T) {
	rr := httptest.NewRecorder()

	OK(rr, map[string]any{"x": 1})

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Result().StatusCode)
	}

	var env Envelope
	mustDecodeJSONLine(t, rr.Body.Bytes(), &env)

	// env.Data will be decoded as map[string]any
	m, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected map data, got %T", env.Data)
	}
	if m["x"] != float64(1) { // json numbers -> float64
		t.Fatalf("expected x=1, got %+v", m)
	}
}

func TestCreated_WrapsEnvelopeAnd201(t *testing.T) {
	rr := httptest.NewRecorder()

	Created(rr, map[string]any{"y": "z"})

	if rr.Result().StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Result().StatusCode)
	}

	var env Envelope
	mustDecodeJSONLine(t, rr.Body.Bytes(), &env)

	m, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected map data, got %T", env.Data)
	}
	if m["y"] != "z" {
		t.Fatalf("expected y=z, got %+v", m)
	}
}

func TestNoContent_Writes204AndNoBody(t *testing.T) {
	rr := httptest.NewRecorder()
	NoContent(rr)

	res := rr.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.StatusCode)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("expected empty body, got %q", rr.Body.String())
	}
}

// ---------- tiny helper error type ----------

type assertErr string

func (e assertErr) Error() string { return string(e) }
