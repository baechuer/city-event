package http_handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/application/auth"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/domain"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/infrastructure/memory"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/infrastructure/security"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/transport/http/middleware"
)

// -------------------------
// Test wiring (pure unit)
// -------------------------

type fakeUserRepo struct {
	byID    map[string]domain.User
	byEmail map[string]string // email -> id
	tokenV  map[string]int64  // userID -> token_version
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{
		byID:    make(map[string]domain.User),
		byEmail: make(map[string]string),
		tokenV:  make(map[string]int64),
	}
}

func normEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func (r *fakeUserRepo) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	email = normEmail(email)
	if email == "" {
		return domain.User{}, domain.ErrMissingField("email")
	}
	id, ok := r.byEmail[email]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound()
	}
	return r.byID[id], nil
}

func (r *fakeUserRepo) GetByID(ctx context.Context, id string) (domain.User, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.User{}, domain.ErrMissingField("id")
	}
	u, ok := r.byID[id]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound()
	}
	return u, nil
}

func (r *fakeUserRepo) Create(ctx context.Context, u domain.User) (domain.User, error) {
	u.Email = normEmail(u.Email)
	if u.ID == "" {
		return domain.User{}, domain.ErrMissingField("id")
	}
	if u.Email == "" {
		return domain.User{}, domain.ErrMissingField("email")
	}
	if _, ok := r.byEmail[u.Email]; ok {
		return domain.User{}, domain.ErrEmailAlreadyExists()
	}
	if u.Role == "" {
		u.Role = string(domain.RoleUser)
	}
	r.byID[u.ID] = u
	r.byEmail[u.Email] = u.ID
	if _, ok := r.tokenV[u.ID]; !ok {
		r.tokenV[u.ID] = 0
	}
	return u, nil
}

func (r *fakeUserRepo) UpdatePasswordHash(ctx context.Context, userID string, newHash string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return domain.ErrMissingField("user_id")
	}
	if newHash == "" {
		return domain.ErrMissingField("password_hash")
	}
	u, ok := r.byID[userID]
	if !ok {
		return domain.ErrUserNotFound()
	}
	u.PasswordHash = newHash
	r.byID[userID] = u
	return nil
}

func (r *fakeUserRepo) SetEmailVerified(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return domain.ErrMissingField("user_id")
	}
	u, ok := r.byID[userID]
	if !ok {
		return domain.ErrUserNotFound()
	}
	u.EmailVerified = true
	r.byID[userID] = u
	return nil
}

func (r *fakeUserRepo) LockUser(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return domain.ErrMissingField("user_id")
	}
	u, ok := r.byID[userID]
	if !ok {
		return domain.ErrUserNotFound()
	}
	u.Locked = true
	r.byID[userID] = u
	return nil
}

func (r *fakeUserRepo) UnlockUser(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return domain.ErrMissingField("user_id")
	}
	u, ok := r.byID[userID]
	if !ok {
		return domain.ErrUserNotFound()
	}
	u.Locked = false
	r.byID[userID] = u
	return nil
}

func (r *fakeUserRepo) SetRole(ctx context.Context, userID string, role string) error {
	userID = strings.TrimSpace(userID)
	role = strings.TrimSpace(role)
	if userID == "" {
		return domain.ErrMissingField("user_id")
	}
	if !domain.IsValidRole(role) {
		return domain.ErrInvalidRole(role)
	}
	u, ok := r.byID[userID]
	if !ok {
		return domain.ErrUserNotFound()
	}
	u.Role = role
	r.byID[userID] = u
	return nil
}

func (r *fakeUserRepo) CountByRole(ctx context.Context, role string) (int, error) {
	role = strings.TrimSpace(role)
	if role == "" {
		return 0, domain.ErrMissingField("role")
	}
	if !domain.IsValidRole(role) {
		return 0, domain.ErrInvalidRole(role)
	}
	n := 0
	for _, u := range r.byID {
		if u.Role == role {
			n++
		}
	}
	return n, nil
}

func (r *fakeUserRepo) GetTokenVersion(ctx context.Context, userID string) (int64, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return 0, domain.ErrMissingField("user_id")
	}
	if _, ok := r.byID[userID]; !ok {
		return 0, domain.ErrUserNotFound()
	}
	return r.tokenV[userID], nil
}

func (r *fakeUserRepo) BumpTokenVersion(ctx context.Context, userID string) (int64, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return 0, domain.ErrMissingField("user_id")
	}
	if _, ok := r.byID[userID]; !ok {
		return 0, domain.ErrUserNotFound()
	}
	r.tokenV[userID]++
	return r.tokenV[userID], nil
}

type fakeHasher struct{}

func (h fakeHasher) Hash(password string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", domain.ErrMissingField("password")
	}
	return "hash:" + password, nil
}
func (h fakeHasher) Compare(hash string, password string) error {
	if hash != "hash:"+password {
		return domain.ErrInvalidCredentials()
	}
	return nil
}

func newTestAuthHandler(t *testing.T, secureCookies bool) *AuthHandler {
	t.Helper()

	repo := newFakeUserRepo()
	hasher := fakeHasher{}
	signer := security.NewStubSigner()

	sessionStore := memory.NewSessionStore()
	ottStore := memory.NewOneTimeTokenStore()
	publisher := memory.NewNoopPublisher()

	svc := auth.NewService(
		repo,
		hasher,
		signer,
		sessionStore,
		ottStore,
		publisher,
		auth.Config{
			AccessTTL:             15 * time.Minute,
			RefreshTTL:            7 * 24 * time.Hour,
			VerifyEmailBaseURL:    "http://localhost/verify?token=",
			PasswordResetBaseURL:  "http://localhost/reset?token=",
			VerifyEmailTokenTTL:   24 * time.Hour,
			PasswordResetTokenTTL: 30 * time.Minute,
		},
	)

	return NewAuthHandler(svc, 7*24*time.Hour, secureCookies)
}

// -------------------------
// Helpers
// -------------------------

// tries to locate {"user":...,"tokens":...} either at top-level or under {"data":{...}}
func decodeAuthEnvelope(t *testing.T, res *http.Response) (user any, tokens any) {
	t.Helper()

	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	// 1) top-level
	if u, ok := out["user"]; ok {
		if tk, ok2 := out["tokens"]; ok2 {
			return u, tk
		}
	}

	// 2) data envelope
	if d, ok := out["data"]; ok {
		if dm, ok2 := d.(map[string]any); ok2 {
			u, uok := dm["user"]
			tk, tok := dm["tokens"]
			if uok && tok {
				return u, tk
			}
		}
	}

	t.Fatalf("expected user/tokens in response (top-level or data envelope), got keys=%v", keysOf(out))
	return nil, nil
}

func keysOf(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// -------------------------
// Existing tests (fixed)
// -------------------------

func TestAuthHandler_Register_InvalidJSON(t *testing.T) {
	h := newTestAuthHandler(t, false)

	req := httptest.NewRequest(http.MethodPost, "/auth/v1/register", strings.NewReader("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Register(rr, req)
	res := rr.Result()
	defer res.Body.Close()

	if res.StatusCode < 400 || res.StatusCode >= 600 {
		t.Fatalf("expected 4xx/5xx, got %d", res.StatusCode)
	}
}

func TestAuthHandler_Register_ValidationFails(t *testing.T) {
	h := newTestAuthHandler(t, false)

	req := httptest.NewRequest(http.MethodPost, "/auth/v1/register", mustJSONBody(t, map[string]any{
		"email":    "",
		"password": "123456789012",
	}))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Register(rr, req)
	res := rr.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
}

func TestAuthHandler_Register_SetsRefreshCookie_AndReturns201(t *testing.T) {
	h := newTestAuthHandler(t, false)

	req := httptest.NewRequest(http.MethodPost, "/auth/v1/register", mustJSONBody(t, map[string]any{
		"email":    "  user@example.com ",
		"password": "123456789012",
	}))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Register(rr, req)
	res := rr.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", res.StatusCode)
	}

	ck := readCookie(res, security.RefreshCookieName)
	if ck == nil {
		t.Fatalf("expected refresh cookie to be set")
	}
	if !ck.HttpOnly {
		t.Fatalf("expected HttpOnly cookie")
	}
	// Path should now be /
	if ck.Path != "/" {
		t.Fatalf("expected cookie Path=/, got %q", ck.Path)
	}
	if ck.MaxAge <= 0 {
		t.Fatalf("expected MaxAge > 0, got %d", ck.MaxAge)
	}

	u, tk := decodeAuthEnvelope(t, res)
	if u == nil || tk == nil {
		t.Fatalf("expected non-nil user and tokens")
	}

	um := u.(map[string]any)
	if hasPw, ok := um["has_password"].(bool); !ok || !hasPw {
		t.Fatalf("expected has_password: true in register response, got: %v", um["has_password"])
	}
}

func TestAuthHandler_Login_OK_SetsCookie(t *testing.T) {
	h := newTestAuthHandler(t, true)

	// first register to create the user
	{
		req := httptest.NewRequest(http.MethodPost, "/auth/v1/register", mustJSONBody(t, map[string]any{
			"email":    "user2@example.com",
			"password": "123456789012",
		}))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.Register(rr, req)
		if rr.Result().StatusCode != http.StatusCreated {
			t.Fatalf("setup register expected 201, got %d", rr.Result().StatusCode)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/v1/login", mustJSONBody(t, map[string]any{
		"email":    " user2@example.com ",
		"password": "123456789012",
	}))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Login(rr, req)
	res := rr.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}

	// secure=true -> expect __Host- prefix
	ck := readCookie(res, "__Host-"+security.RefreshCookieName)
	if ck == nil {
		t.Fatalf("expected refresh cookie (__Host-...) to be set")
	}
	if ck.Secure != true {
		t.Fatalf("expected Secure cookie (secureCookies=true)")
	}
	if ck.Path != "/" {
		t.Fatalf("expected Path=/, got %q", ck.Path)
	}
}

func TestAuthHandler_Refresh_NoCookie_Returns401(t *testing.T) {
	h := newTestAuthHandler(t, false)

	req := httptest.NewRequest(http.MethodPost, "/auth/v1/refresh", nil)
	rr := httptest.NewRecorder()

	h.Refresh(rr, req)
	res := rr.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.StatusCode)
	}
}

func TestAuthHandler_Logout_ClearsCookie_Returns204(t *testing.T) {
	h := newTestAuthHandler(t, true)

	req := httptest.NewRequest(http.MethodPost, "/auth/v1/logout", nil)
	rr := httptest.NewRecorder()

	h.Logout(rr, req)
	res := rr.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.StatusCode)
	}

	// secure=true -> expect __Host-
	ck := readCookie(res, "__Host-"+security.RefreshCookieName)
	if ck == nil {
		t.Fatalf("expected refresh cookie to be cleared (Set-Cookie)")
	}
	if ck.MaxAge != -1 {
		t.Fatalf("expected MaxAge=-1, got %d", ck.MaxAge)
	}
	if ck.Path != "/" {
		t.Fatalf("expected Path=/, got %q", ck.Path)
	}
	if ck.Secure != true {
		t.Fatalf("expected Secure cookie (secureCookies=true)")
	}
}

func TestAuthHandler_Me_NoContext_Returns401(t *testing.T) {
	h := newTestAuthHandler(t, false)

	req := httptest.NewRequest(http.MethodGet, "/auth/v1/me", nil)
	rr := httptest.NewRecorder()

	h.Me(rr, req)
	res := rr.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.StatusCode)
	}
}

func TestAuthHandler_BanUser_MissingID_Returns400(t *testing.T) {
	h := newTestAuthHandler(t, false)

	req := httptest.NewRequest(http.MethodPost, "/auth/v1/mod/ban/", nil)
	rr := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.BanUser(rr, req)
	res := rr.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
}

// -------------------------
// NEW high-value unit cases
// -------------------------

func TestAuthHandler_Login_InvalidCredentials_Returns401(t *testing.T) {
	h := newTestAuthHandler(t, false)

	// register
	{
		req := httptest.NewRequest(http.MethodPost, "/auth/v1/register", mustJSONBody(t, map[string]any{
			"email":    "u3@example.com",
			"password": "123456789012",
		}))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.Register(rr, req)
		if rr.Result().StatusCode != http.StatusCreated {
			t.Fatalf("setup register expected 201, got %d", rr.Result().StatusCode)
		}
	}

	// wrong password
	req := httptest.NewRequest(http.MethodPost, "/auth/v1/login", mustJSONBody(t, map[string]any{
		"email":    "u3@example.com",
		"password": "WRONG_PASSWORD_123",
	}))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Login(rr, req)
	res := rr.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.StatusCode)
	}

	// should NOT set refresh cookie on failure
	if ck := readCookie(res, security.RefreshCookieName); ck != nil && ck.MaxAge > 0 {
		t.Fatalf("expected no refresh cookie on login failure, got MaxAge=%d", ck.MaxAge)
	}
}

func TestAuthHandler_Register_DuplicateEmail_Returns409(t *testing.T) {
	h := newTestAuthHandler(t, false)

	body := map[string]any{"email": "dup@example.com", "password": "123456789012"}

	// first ok
	{
		req := httptest.NewRequest(http.MethodPost, "/auth/v1/register", mustJSONBody(t, body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.Register(rr, req)
		if rr.Result().StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d", rr.Result().StatusCode)
		}
	}

	// second should conflict
	req := httptest.NewRequest(http.MethodPost, "/auth/v1/register", mustJSONBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Register(rr, req)
	res := rr.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", res.StatusCode)
	}
}

func TestAuthHandler_Refresh_WithCookie_ButInvalidToken_Returns401(t *testing.T) {
	h := newTestAuthHandler(t, false)

	req := httptest.NewRequest(http.MethodPost, "/auth/v1/refresh", nil)
	req.AddCookie(&http.Cookie{
		Name:  security.RefreshCookieName,
		Value: "not-a-real-refresh-token",
		Path:  "/auth/v1",
	})
	rr := httptest.NewRecorder()

	h.Refresh(rr, req)
	res := rr.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.StatusCode)
	}
}

func TestAuthHandler_VerifyEmailConfirmGET_MissingToken_Returns400(t *testing.T) {
	h := newTestAuthHandler(t, false)

	req := httptest.NewRequest(http.MethodGet, "/auth/v1/verify/confirm", nil) // no token query
	rr := httptest.NewRecorder()

	h.VerifyEmailConfirmGET(rr, req)
	res := rr.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
}

func TestAuthHandler_PasswordResetValidate_MissingToken_Returns400(t *testing.T) {
	h := newTestAuthHandler(t, false)

	req := httptest.NewRequest(http.MethodGet, "/auth/v1/password/reset/validate", nil)
	rr := httptest.NewRecorder()

	h.PasswordResetValidate(rr, req)
	res := rr.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
}
func TestAuthHandler_Me_OK(t *testing.T) {
	h := newTestAuthHandler(t, false)

	// 1) register
	reqReg := httptest.NewRequest(
		http.MethodPost,
		"/auth/v1/register",
		mustJSONBody(t, map[string]any{
			"email":    "me@example.com",
			"password": "123456789012",
		}),
	)
	reqReg.Header.Set("Content-Type", "application/json")

	rrReg := httptest.NewRecorder()
	h.Register(rrReg, reqReg)

	if rrReg.Result().StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body=%s", rrReg.Result().StatusCode, rrReg.Body.String())
	}

	userID := mustExtractUserIDFromRegisterBody(t, rrReg.Body)

	// 2) /me with auth ctx
	req := httptest.NewRequest(http.MethodGet, "/auth/v1/me", nil)
	req = req.WithContext(middleware.WithUser(req.Context(), userID, "user"))

	rr := httptest.NewRecorder()
	h.Me(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Result().StatusCode, rr.Body.String())
	}
}

func TestAuthHandler_Admin_OK(t *testing.T) {
	h := newTestAuthHandler(t, false)

	req := httptest.NewRequest(http.MethodGet, "/auth/v1/admin", nil)
	req = req.WithContext(middleware.WithUser(req.Context(), "admin-id", "admin"))
	rr := httptest.NewRecorder()

	h.Admin(rr, req)
	res := rr.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
}
func TestAuthHandler_AdminSetUserRole_OK(t *testing.T) {
	h := newTestAuthHandler(t, false)

	// 1) register target
	reqReg := httptest.NewRequest(
		http.MethodPost,
		"/auth/v1/register",
		mustJSONBody(t, map[string]any{
			"email":    "target@example.com",
			"password": "123456789012",
		}),
	)
	reqReg.Header.Set("Content-Type", "application/json")

	rrReg := httptest.NewRecorder()
	h.Register(rrReg, reqReg)

	if rrReg.Result().StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body=%s", rrReg.Result().StatusCode, rrReg.Body.String())
	}

	targetID := mustExtractUserIDFromRegisterBody(t, rrReg.Body)

	// 2) admin set role
	req := httptest.NewRequest(
		http.MethodPost,
		"/auth/v1/admin/users/"+targetID+"/role",
		mustJSONBody(t, map[string]any{"role": "moderator"}),
	)
	req.Header.Set("Content-Type", "application/json")

	// chi URL param "id"
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", targetID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// admin ctx
	req = req.WithContext(middleware.WithUser(req.Context(), "admin-id", "admin"))

	rr := httptest.NewRecorder()
	h.AdminSetUserRole(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Result().StatusCode, rr.Body.String())
	}
}

func TestAuthHandler_AdminSetUserRole_InvalidRole(t *testing.T) {
	h := newTestAuthHandler(t, false)

	req := httptest.NewRequest(http.MethodPost, "/auth/v1/admin/users/u/role",
		mustJSONBody(t, map[string]any{"role": "supergod"}),
	)
	req.Header.Set("Content-Type", "application/json")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "u")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(middleware.WithUser(req.Context(), "admin", "admin"))

	rr := httptest.NewRecorder()
	h.AdminSetUserRole(rr, req)

	if rr.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Result().StatusCode)
	}
}
func TestAuthHandler_PasswordChange_ClearsCookie(t *testing.T) {
	h := newTestAuthHandler(t, true)

	// 1) register
	reqReg := httptest.NewRequest(
		http.MethodPost,
		"/auth/v1/register",
		mustJSONBody(t, map[string]any{
			"email":    "pw@example.com",
			"password": "oldpassword123",
		}),
	)
	reqReg.Header.Set("Content-Type", "application/json")

	rrReg := httptest.NewRecorder()
	h.Register(rrReg, reqReg)

	if rrReg.Result().StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body=%s", rrReg.Result().StatusCode, rrReg.Body.String())
	}

	userID := mustExtractUserIDFromRegisterBody(t, rrReg.Body)

	// 2) password change
	req := httptest.NewRequest(
		http.MethodPost,
		"/auth/v1/password/change",
		mustJSONBody(t, map[string]any{
			"old_password": "oldpassword123",
			"new_password": "newpassword123", // len>=12 ✅
		}),
	)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(middleware.WithUser(req.Context(), userID, "user"))

	rr := httptest.NewRecorder()
	h.PasswordChange(rr, req)

	res := rr.Result()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body=%s", res.StatusCode, rr.Body.String())
	}

	// secure=true -> __Host-
	ck := readCookie(res, "__Host-"+security.RefreshCookieName)
	if ck == nil {
		t.Fatalf("expected refresh cookie to be set")
	}
	if ck.MaxAge != -1 {
		t.Fatalf("expected refresh cookie cleared (MaxAge=-1), got %d", ck.MaxAge)
	}
	if ck.Path != "/" {
		t.Fatalf("expected Path=/, got %q", ck.Path)
	}
}

func TestAuthHandler_SessionsRevoke_OK(t *testing.T) {
	h := newTestAuthHandler(t, true)

	req := httptest.NewRequest(http.MethodPost, "/auth/v1/sessions/revoke", nil)
	req = req.WithContext(middleware.WithUser(req.Context(), "u1", "user"))
	rr := httptest.NewRecorder()

	h.SessionsRevoke(rr, req)

	if rr.Result().StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Result().StatusCode)
	}
}

func TestAuthHandler_MeStatus_HasPassword_Logic(t *testing.T) {
	h := newTestAuthHandler(t, false)

	// Case 1: Regular user
	reqReg := httptest.NewRequest(http.MethodPost, "/auth/v1/register", mustJSONBody(t, map[string]any{
		"email":    "user_pw@example.com",
		"password": "123456789012",
	}))
	reqReg.Header.Set("Content-Type", "application/json")
	rrReg := httptest.NewRecorder()
	h.Register(rrReg, reqReg)
	if rrReg.Result().StatusCode != http.StatusCreated {
		t.Fatalf("Register failed: %d", rrReg.Result().StatusCode)
	}

	userID := mustExtractUserIDFromRegisterBody(t, rrReg.Body)
	req := httptest.NewRequest(http.MethodGet, "/auth/v1/me/status", nil)
	req = req.WithContext(middleware.WithUser(req.Context(), userID, "user"))
	rr := httptest.NewRecorder()
	h.MeStatus(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("MeStatus failed: %d", rr.Result().StatusCode)
	}

	var data struct {
		Data struct {
			HasPassword bool `json:"has_password"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &data); err != nil {
		t.Fatalf("JSON decode failed: %v", err)
	}
	if !data.Data.HasPassword {
		t.Errorf("expected has_password: true for password user, body: %s", rr.Body.String())
	}

	// Case 2: OAuth user (no password)
	oauthUserID := "u-oauth-123"
	repo := newFakeUserRepo()
	repo.byID[oauthUserID] = domain.User{
		ID:           oauthUserID,
		Email:        "oauth@example.com",
		Role:         "user",
		PasswordHash: "", // Empty for OAuth
	}
	repo.byEmail["oauth@example.com"] = oauthUserID

	svc := auth.NewService(repo, fakeHasher{}, security.NewStubSigner(), memory.NewSessionStore(), memory.NewOneTimeTokenStore(), memory.NewNoopPublisher(), auth.Config{})
	h2 := NewAuthHandler(svc, 7*24*time.Hour, false)

	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/auth/v1/me/status", nil)
	req2 = req2.WithContext(middleware.WithUser(req2.Context(), oauthUserID, "user"))
	h2.MeStatus(rr2, req2)

	var data2 struct {
		Data struct {
			HasPassword bool `json:"has_password"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr2.Body.Bytes(), &data2); err != nil {
		t.Fatalf("JSON decode failed: %v", err)
	}
	if data2.Data.HasPassword {
		t.Errorf("expected has_password: false for OAuth user, body: %s", rr2.Body.String())
	}
}
