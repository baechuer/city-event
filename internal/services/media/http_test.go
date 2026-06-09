package media

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/baechuer/cityevents/internal/platform/config"
)

func TestMediaHandlersWorkflow(t *testing.T) {
	router := testMediaRouter(t)
	body := `{"eventId":"event-1","filename":"banner.jpg","contentType":"image/jpeg","sizeBytes":10}`
	create := doMediaJSON(router, http.MethodPost, "/v1/media/uploads", body, "user-1")
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", create.Code, create.Body.String())
	}
	var intent UploadIntent
	decodeMediaBody(t, create.Body.Bytes(), &intent)
	if intent.Asset.ID == "" || intent.UploadURL == "" {
		t.Fatalf("intent = %+v", intent)
	}

	forbidden := doMediaJSON(router, http.MethodPost, "/v1/media/"+intent.Asset.ID+"/uploaded", "", "other-user")
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("forbidden status = %d body=%s", forbidden.Code, forbidden.Body.String())
	}

	uploaded := doMediaJSON(router, http.MethodPost, "/v1/media/"+intent.Asset.ID+"/uploaded", "", "user-1")
	if uploaded.Code != http.StatusOK {
		t.Fatalf("uploaded status = %d body=%s", uploaded.Code, uploaded.Body.String())
	}
	if !bytes.Contains(uploaded.Body.Bytes(), []byte(StatusUploaded)) {
		t.Fatalf("uploaded body = %s", uploaded.Body.String())
	}

	detail := doMediaJSON(router, http.MethodGet, "/v1/media/"+intent.Asset.ID, "", "")
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status = %d body=%s", detail.Code, detail.Body.String())
	}
}

func TestMediaHandlersValidation(t *testing.T) {
	router := testMediaRouter(t)
	body := `{"eventId":"event-1","filename":"banner.gif","contentType":"image/gif","sizeBytes":10}`
	invalid := doMediaJSON(router, http.MethodPost, "/v1/media/uploads", body, "user-1")
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid status = %d body=%s", invalid.Code, invalid.Body.String())
	}

	unauthorized := doMediaJSON(router, http.MethodPost, "/v1/media/uploads", body, "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	missing := doMediaJSON(router, http.MethodGet, "/v1/media/missing", "", "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d", missing.Code)
	}
}

func testMediaRouter(t *testing.T) http.Handler {
	t.Helper()
	cfg, err := config.Load("media-service", nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return NewHTTPHandler(cfg, nil, NewService(NewMemoryRepository(), NewMemoryStorage(), "bucket"))
}

func doMediaJSON(handler http.Handler, method, path, body, userID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeMediaBody(t *testing.T, body []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("decode body %s: %v", string(body), err)
	}
}
