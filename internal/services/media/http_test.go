package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/baechuer/cityevents/internal/platform/authn"
	"github.com/baechuer/cityevents/internal/platform/config"
	"github.com/baechuer/cityevents/internal/platform/identity"
)

func TestMediaHandlersWorkflow(t *testing.T) {
	router := testMediaRouter(t)
	body := `{"eventId":"event-1","filename":"banner.jpg","contentType":"image/jpeg","sizeBytes":10}`
	create := doMediaJSONAsRole(router, http.MethodPost, "/v1/media/uploads", body, "organizer-1", identity.RoleOrganizer)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", create.Code, create.Body.String())
	}
	var intent UploadIntent
	decodeMediaBody(t, create.Body.Bytes(), &intent)
	if intent.Asset.ID == "" || intent.UploadURL == "" || intent.UploadMethod == "" {
		t.Fatalf("intent = %+v", intent)
	}

	forbidden := doMediaJSON(router, http.MethodPost, "/v1/media/"+intent.Asset.ID+"/uploaded", "", "other-user")
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("forbidden status = %d body=%s", forbidden.Code, forbidden.Body.String())
	}

	uploaded := doMediaJSON(router, http.MethodPost, "/v1/media/"+intent.Asset.ID+"/uploaded", "", "organizer-1")
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

func TestMediaHandlersAuthorizeUploadIntentByEventOwnership(t *testing.T) {
	router := testMediaRouter(t)
	body := `{"eventId":"event-1","filename":"banner.jpg","contentType":"image/jpeg","sizeBytes":10}`

	plainUser := doMediaJSON(router, http.MethodPost, "/v1/media/uploads", body, "user-1")
	if plainUser.Code != http.StatusForbidden {
		t.Fatalf("plain user upload status = %d body=%s", plainUser.Code, plainUser.Body.String())
	}

	otherOrganizer := doMediaJSONAsRole(router, http.MethodPost, "/v1/media/uploads", body, "organizer-2", identity.RoleOrganizer)
	if otherOrganizer.Code != http.StatusForbidden {
		t.Fatalf("other organizer upload status = %d body=%s", otherOrganizer.Code, otherOrganizer.Body.String())
	}

	admin := doMediaJSONAsRole(router, http.MethodPost, "/v1/media/uploads", body, "admin-1", identity.RoleAdmin)
	if admin.Code != http.StatusCreated {
		t.Fatalf("admin upload status = %d body=%s", admin.Code, admin.Body.String())
	}

	missing := doMediaJSONAsRole(router, http.MethodPost, "/v1/media/uploads", `{"eventId":"missing","filename":"banner.jpg","contentType":"image/jpeg","sizeBytes":10}`, "admin-1", identity.RoleAdmin)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing event upload status = %d body=%s", missing.Code, missing.Body.String())
	}
}

func TestMediaHandlersRejectTokenDeniedByAuthService(t *testing.T) {
	router := testMediaRouterWithIntrospector(t, testMediaIntrospector{
		denyUserIDs: map[string]bool{"organizer-1": true},
	})

	body := `{"eventId":"event-1","filename":"banner.jpg","contentType":"image/jpeg","sizeBytes":10}`
	resp := doMediaJSONAsRole(router, http.MethodPost, "/v1/media/uploads", body, "organizer-1", identity.RoleOrganizer)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token status = %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestMediaHandlersUseCurrentRoleFromAuthService(t *testing.T) {
	router := testMediaRouterWithIntrospector(t, testMediaIntrospector{
		roleByUserID: map[string]identity.Role{"organizer-1": identity.RoleUser},
	})

	body := `{"eventId":"event-1","filename":"banner.jpg","contentType":"image/jpeg","sizeBytes":10}`
	resp := doMediaJSONAsRole(router, http.MethodPost, "/v1/media/uploads", body, "organizer-1", identity.RoleOrganizer)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("stale organizer token status = %d body=%s", resp.Code, resp.Body.String())
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

	spoofed := doMediaJSONWithHeaders(router, http.MethodPost, "/v1/media/uploads", body, "", map[string]string{"X-User-ID": "spoofed"})
	if spoofed.Code != http.StatusUnauthorized {
		t.Fatalf("spoofed header status = %d body=%s", spoofed.Code, spoofed.Body.String())
	}

	missing := doMediaJSON(router, http.MethodGet, "/v1/media/missing", "", "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d", missing.Code)
	}
}

func testMediaRouter(t *testing.T) http.Handler {
	t.Helper()
	return testMediaRouterWithIntrospector(t, testMediaIntrospector{})
}

func testMediaRouterWithIntrospector(t *testing.T, introspector authn.Introspector) http.Handler {
	t.Helper()
	cfg, err := config.Load("media-service", nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return NewHTTPHandlerWithIntrospector(cfg, nil, NewService(NewMemoryRepository(), NewMemoryStorage(), "bucket", ownerEventAuthorizer{
		organizers: map[string]string{"event-1": "organizer-1"},
	}), introspector)
}

func doMediaJSON(handler http.Handler, method, path, body, userID string) *httptest.ResponseRecorder {
	return doMediaJSONAsRoleWithHeaders(handler, method, path, body, userID, identity.RoleUser, nil)
}

func doMediaJSONAsRole(handler http.Handler, method, path, body, userID string, role identity.Role) *httptest.ResponseRecorder {
	return doMediaJSONAsRoleWithHeaders(handler, method, path, body, userID, role, nil)
}

func doMediaJSONWithHeaders(handler http.Handler, method, path, body, userID string, headers map[string]string) *httptest.ResponseRecorder {
	return doMediaJSONAsRoleWithHeaders(handler, method, path, body, userID, identity.RoleUser, headers)
}

func doMediaJSONAsRoleWithHeaders(handler http.Handler, method, path, body, userID string, role identity.Role, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if userID != "" {
		req.Header.Set("Authorization", "Bearer "+testMediaAccessToken(userID, role))
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func testMediaAccessToken(userID string, role identity.Role) string {
	manager := authn.NewTokenManager("dev-secret-change-me", "cityevents", time.Hour)
	token, _, err := manager.Sign(authn.Subject{
		UserID: userID,
		Email:  userID + "@example.com",
		Role:   role,
	})
	if err != nil {
		panic(err)
	}
	return token
}

type ownerEventAuthorizer struct {
	organizers map[string]string
}

func (a ownerEventAuthorizer) AuthorizeCreateMedia(_ context.Context, request EventMediaAuthorization) error {
	organizerID, ok := a.organizers[request.EventID]
	if !ok {
		return ErrNotFound
	}
	role := identity.NormalizeRole(string(request.Role))
	if identity.CanAdmin(role) {
		return nil
	}
	if role == identity.RoleOrganizer && request.UserID == organizerID {
		return nil
	}
	return ErrForbidden
}

func decodeMediaBody(t *testing.T, body []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("decode body %s: %v", string(body), err)
	}
}

type testMediaIntrospector struct {
	denyUserIDs  map[string]bool
	roleByUserID map[string]identity.Role
}

func (i testMediaIntrospector) Introspect(_ context.Context, token string) (authn.Subject, error) {
	manager := authn.NewTokenManager("dev-secret-change-me", "cityevents", time.Hour)
	claims, err := manager.Verify(token)
	if err != nil {
		return authn.Subject{}, err
	}
	if i.denyUserIDs[claims.UserID] {
		return authn.Subject{}, authn.ErrInvalidToken
	}
	role := claims.Role
	if currentRole, ok := i.roleByUserID[claims.UserID]; ok {
		role = currentRole
	}
	if !identity.ValidRole(role) {
		return authn.Subject{}, errors.New("invalid role")
	}
	return authn.Subject{
		UserID: claims.UserID,
		Email:  claims.Email,
		Role:   role,
	}, nil
}
