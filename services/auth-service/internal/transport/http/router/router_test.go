package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------- fakes ----------

type fakeHealth struct{}

func (fakeHealth) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (fakeHealth) Readyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

type fakeAuth struct{}

func (fakeAuth) write(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(msg))
}

func (a fakeAuth) Register(w http.ResponseWriter, r *http.Request) { a.write(w, 200, "register") }
func (a fakeAuth) Login(w http.ResponseWriter, r *http.Request)    { a.write(w, 200, "login") }
func (a fakeAuth) Refresh(w http.ResponseWriter, r *http.Request)  { a.write(w, 200, "refresh") }
func (a fakeAuth) Logout(w http.ResponseWriter, r *http.Request)   { a.write(w, 200, "logout") }
func (a fakeAuth) Me(w http.ResponseWriter, r *http.Request)       { a.write(w, 200, "me") }
func (a fakeAuth) Admin(w http.ResponseWriter, r *http.Request)    { a.write(w, 200, "admin") }

func (a fakeAuth) BanUser(w http.ResponseWriter, r *http.Request)   { a.write(w, 200, "ban") }
func (a fakeAuth) UnbanUser(w http.ResponseWriter, r *http.Request) { a.write(w, 200, "unban") }
func (a fakeAuth) AdminSetUserRole(w http.ResponseWriter, r *http.Request) {
	a.write(w, 200, "set_role")
}
func (a fakeAuth) AdminUserStatus(w http.ResponseWriter, r *http.Request) {
	a.write(w, 200, "user_status")
}

func (a fakeAuth) VerifyEmailRequest(w http.ResponseWriter, r *http.Request) {
	a.write(w, 200, "verify_email_request")
}
func (a fakeAuth) VerifyEmailConfirmGET(w http.ResponseWriter, r *http.Request) {
	a.write(w, 200, "verify_email_confirm_get")
}
func (a fakeAuth) VerifyEmailConfirmPOST(w http.ResponseWriter, r *http.Request) {
	a.write(w, 200, "verify_email_confirm_post")
}

func (a fakeAuth) PasswordResetRequest(w http.ResponseWriter, r *http.Request) {
	a.write(w, 200, "pw_reset_request")
}
func (a fakeAuth) PasswordResetConfirm(w http.ResponseWriter, r *http.Request) {
	a.write(w, 200, "pw_reset_confirm")
}
func (a fakeAuth) PasswordResetValidate(w http.ResponseWriter, r *http.Request) {
	a.write(w, 200, "pw_reset_validate")
}

func (a fakeAuth) PasswordChange(w http.ResponseWriter, r *http.Request) {
	a.write(w, 200, "pw_change")
}
func (a fakeAuth) AdminRevokeSessions(w http.ResponseWriter, r *http.Request) {
	a.write(w, 200, "admin_revoke_sessions")
}
func (a fakeAuth) SessionsRevoke(w http.ResponseWriter, r *http.Request) {
	a.write(w, 200, "sessions_revoke")
}

func (a fakeAuth) MeStatus(w http.ResponseWriter, r *http.Request)        { a.write(w, 200, "me_status") }
func (a fakeAuth) InternalGetUser(w http.ResponseWriter, r *http.Request) {}
func (a fakeAuth) UpdateAvatar(w http.ResponseWriter, r *http.Request) {
	a.write(w, 200, "update_avatar")
}

// Middleware helper
func noopMW(next http.Handler) http.Handler { return next }

func headerMW(key, val string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(key, val)
			next.ServeHTTP(w, r)
		})
	}
}

// ---------- tests ----------

func TestNew_NilHealth_ReturnsError(t *testing.T) {
	_, err := New(Deps{
		Health:      nil,
		Auth:        fakeAuth{},
		RequestIDMW: noopMW,
		AuthMW:      noopMW, ModMW: noopMW, AdminMW: noopMW, InternalAuthMW: noopMW,
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestNew_NilAuth_ReturnsError(t *testing.T) {
	_, err := New(Deps{
		Health:      fakeHealth{},
		Auth:        nil,
		RequestIDMW: noopMW,
		AuthMW:      noopMW, ModMW: noopMW, AdminMW: noopMW,
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestNew_NilMiddlewares_ReturnError(t *testing.T) {
	// 测试 AuthMW 为空
	_, err := New(Deps{
		Health: fakeHealth{}, Auth: fakeAuth{},
		RequestIDMW: noopMW,
		AuthMW:      nil, ModMW: noopMW, AdminMW: noopMW, InternalAuthMW: noopMW,
	})
	if err == nil {
		t.Fatalf("expected error for nil AuthMW")
	}

	// 测试 RequestIDMW 为空（对应你之前报错的关键点）
	_, err = New(Deps{
		Health: fakeHealth{}, Auth: fakeAuth{},
		RequestIDMW: nil,
		AuthMW:      noopMW, ModMW: noopMW, AdminMW: noopMW, InternalAuthMW: noopMW,
	})
	if err == nil {
		t.Fatalf("expected error for nil RequestIDMW")
	}
}

func TestNew_HealthzRoute_Works(t *testing.T) {
	h, err := New(Deps{
		Health:      fakeHealth{},
		Auth:        fakeAuth{},
		RequestIDMW: noopMW,
		AuthMW:      noopMW, ModMW: noopMW, AdminMW: noopMW, InternalAuthMW: noopMW,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, rr.Code)
	}
	if rr.Body.String() != "ok" {
		t.Fatalf("expected body %q, got %q", "ok", rr.Body.String())
	}
}

func TestNew_LoginRoute_DispatchesToHandler(t *testing.T) {
	h, err := New(Deps{
		Health:      fakeHealth{},
		Auth:        fakeAuth{},
		RequestIDMW: noopMW,
		AuthMW:      noopMW, ModMW: noopMW, AdminMW: noopMW, InternalAuthMW: noopMW,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/v1/login", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "login" {
		t.Fatalf("expected body %q, got %q", "login", rr.Body.String())
	}
}

func TestNew_MeRoute_UsesAuthMW(t *testing.T) {
	h, err := New(Deps{
		Health:         fakeHealth{},
		Auth:           fakeAuth{},
		RequestIDMW:    noopMW,
		AuthMW:         headerMW("X-AuthMW", "1"),
		AdminMW:        noopMW,
		ModMW:          noopMW,
		InternalAuthMW: noopMW,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/v1/me", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("X-AuthMW") != "1" {
		t.Fatalf("expected AuthMW header set")
	}
}

func TestNew_AdminRoute_UsesAuthMWAndAdminMW(t *testing.T) {
	h, err := New(Deps{
		Health:         fakeHealth{},
		Auth:           fakeAuth{},
		RequestIDMW:    noopMW,
		AuthMW:         headerMW("X-AuthMW", "1"),
		AdminMW:        headerMW("X-AdminMW", "1"),
		ModMW:          noopMW,
		InternalAuthMW: noopMW,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/v1/admin", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("X-AdminMW") != "1" {
		t.Fatalf("expected AdminMW header set")
	}
}

func TestNew_AdminSubroute_SetRole_UsesAuthMWAndAdminMW(t *testing.T) {
	h, err := New(Deps{
		Health:         fakeHealth{},
		Auth:           fakeAuth{},
		RequestIDMW:    noopMW,
		AuthMW:         headerMW("X-AuthMW", "1"),
		AdminMW:        headerMW("X-AdminMW", "1"),
		ModMW:          noopMW,
		InternalAuthMW: noopMW,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/v1/admin/users/u-1/role", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "set_role" {
		t.Fatalf("expected body %q, got %q", "set_role", rr.Body.String())
	}
}
