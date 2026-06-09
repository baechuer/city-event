package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/baechuer/cityevents/internal/platform/config"
	"golang.org/x/crypto/bcrypt"
)

func TestAuthHandlersWorkflow(t *testing.T) {
	router, _ := testAuthRouter(t)

	registerBody := `{"email":"USER@example.com","password":"StrongerPass123","displayName":"User"}`
	registerResp := doJSON(router, http.MethodPost, "/v1/auth/register", registerBody, "")
	if registerResp.Code != http.StatusCreated {
		t.Fatalf("register status = %d body=%s", registerResp.Code, registerResp.Body.String())
	}
	token := accessTokenFromBody(t, registerResp.Body.Bytes())

	loginResp := doJSON(router, http.MethodPost, "/v1/auth/login", `{"email":"user@example.com","password":"StrongerPass123"}`, "")
	if loginResp.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", loginResp.Code, loginResp.Body.String())
	}
	loginToken := accessTokenFromBody(t, loginResp.Body.Bytes())

	meResp := doJSON(router, http.MethodGet, "/v1/auth/me", "", loginToken)
	if meResp.Code != http.StatusOK {
		t.Fatalf("me status = %d body=%s", meResp.Code, meResp.Body.String())
	}

	logoutResp := doJSON(router, http.MethodPost, "/v1/auth/logout", "", loginToken)
	if logoutResp.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d body=%s", logoutResp.Code, logoutResp.Body.String())
	}
	secondLogoutResp := doJSON(router, http.MethodPost, "/v1/auth/logout", "", loginToken)
	if secondLogoutResp.Code != http.StatusNoContent {
		t.Fatalf("second logout status = %d body=%s", secondLogoutResp.Code, secondLogoutResp.Body.String())
	}

	revokedResp := doJSON(router, http.MethodGet, "/v1/auth/me", "", loginToken)
	if revokedResp.Code != http.StatusUnauthorized {
		t.Fatalf("revoked me status = %d body=%s token=%s", revokedResp.Code, revokedResp.Body.String(), token)
	}
}

func TestAuthHandlersRegisterValidationAndDuplicate(t *testing.T) {
	router, _ := testAuthRouter(t)

	invalidEmail := doJSON(router, http.MethodPost, "/v1/auth/register", `{"email":"bad","password":"StrongerPass123","displayName":"User"}`, "")
	if invalidEmail.Code != http.StatusBadRequest {
		t.Fatalf("invalid email status = %d", invalidEmail.Code)
	}

	weakPassword := doJSON(router, http.MethodPost, "/v1/auth/register", `{"email":"weak@example.com","password":"weak","displayName":"User"}`, "")
	if weakPassword.Code != http.StatusBadRequest {
		t.Fatalf("weak password status = %d", weakPassword.Code)
	}

	first := doJSON(router, http.MethodPost, "/v1/auth/register", `{"email":"dupe@example.com","password":"StrongerPass123","displayName":"User"}`, "")
	if first.Code != http.StatusCreated {
		t.Fatalf("first register status = %d body=%s", first.Code, first.Body.String())
	}
	duplicate := doJSON(router, http.MethodPost, "/v1/auth/register", `{"email":"DUPE@example.com","password":"StrongerPass123","displayName":"User"}`, "")
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d body=%s", duplicate.Code, duplicate.Body.String())
	}
}

func TestAuthHandlersLoginFailures(t *testing.T) {
	router, _ := testAuthRouter(t)

	unknown := doJSON(router, http.MethodPost, "/v1/auth/login", `{"email":"none@example.com","password":"StrongerPass123"}`, "")
	if unknown.Code != http.StatusUnauthorized {
		t.Fatalf("unknown login status = %d", unknown.Code)
	}

	_ = doJSON(router, http.MethodPost, "/v1/auth/register", `{"email":"user@example.com","password":"StrongerPass123","displayName":"User"}`, "")
	wrong := doJSON(router, http.MethodPost, "/v1/auth/login", `{"email":"user@example.com","password":"WrongPass123"}`, "")
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("wrong login status = %d", wrong.Code)
	}
}

func TestAuthHandlersMeRequiresToken(t *testing.T) {
	router, _ := testAuthRouter(t)
	resp := doJSON(router, http.MethodGet, "/v1/auth/me", "", "")
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("me without token status = %d", resp.Code)
	}

	malformed := doJSON(router, http.MethodGet, "/v1/auth/me", "", "not.a.valid.token")
	if malformed.Code != http.StatusUnauthorized {
		t.Fatalf("me with malformed token status = %d", malformed.Code)
	}
}

func testAuthRouter(t *testing.T) (http.Handler, *MemoryRepository) {
	t.Helper()
	repo := NewMemoryRepository()
	tokens := NewTokenManager("secret", "cityevents-test", 3600000000000)
	svc := NewService(repo, NewPasswordHasher(bcrypt.MinCost), tokens, NewLoginGuard(5, 60000000000))
	cfg, err := config.Load("auth-service", nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return NewHTTPHandler(cfg, nil, svc), repo
}

func doJSON(handler http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	var requestBody *bytes.Reader
	if body == "" {
		requestBody = bytes.NewReader(nil)
	} else {
		requestBody = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, requestBody)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func accessTokenFromBody(t *testing.T, body []byte) string {
	t.Helper()
	var payload struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	if payload.AccessToken == "" {
		t.Fatalf("expected access token in response: %s", string(body))
	}
	return payload.AccessToken
}
