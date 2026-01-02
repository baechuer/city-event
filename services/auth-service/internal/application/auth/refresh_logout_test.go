package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/domain"
)

func TestLogout_Empty_NoOp(t *testing.T) {
	t.Parallel()

	svc, _, _, _, sessions, _, _, _ := newSvcForTest(t)

	if err := svc.Logout(context.Background(), ""); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if len(sessions.revoked) != 0 {
		t.Fatalf("expected no revoke calls, got %v", sessions.revoked)
	}
}

func TestLogout_RevokesToken(t *testing.T) {
	t.Parallel()

	svc, _, _, _, sessions, _, _, _ := newSvcForTest(t)

	if err := svc.Logout(context.Background(), "rft:u1"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if len(sessions.revoked) != 1 || sessions.revoked[0] != "rft:u1" {
		t.Fatalf("expected revoked rft:u1, got %v", sessions.revoked)
	}
}

func TestRefresh_Empty_Invalid(t *testing.T) {
	t.Parallel()

	svc, _, _, _, _, _, _, _ := newSvcForTest(t)

	_, _, err := svc.Refresh(context.Background(), "")
	if err == nil {
		t.Fatalf("expected error")
	}
	requireDomainCode(t, err, domainCode(domain.ErrRefreshTokenInvalid()))
}

func TestRefresh_TokenNotFound_Invalid(t *testing.T) {
	t.Parallel()

	svc, _, _, _, sessions, _, _, _ := newSvcForTest(t)
	sessions.getUserErr = errors.New("not found")

	_, _, err := svc.Refresh(context.Background(), "refresh-invalid")
	if err == nil {
		t.Fatalf("expected error")
	}
	requireDomainCode(t, err, domainCode(domain.ErrRefreshTokenInvalid()))
}

func TestRefresh_UserMissing_Invalid(t *testing.T) {
	t.Parallel()

	svc, users, _, _, sessions, _, _, _ := newSvcForTest(t)

	// token maps to u1, but user repo can't find u1
	sessions.byToken["rft:u1"] = "u1"
	users.getByIDErr = errors.New("no user")

	_, _, err := svc.Refresh(context.Background(), "rft:u1")
	if err == nil {
		t.Fatalf("expected error")
	}
	requireDomainCode(t, err, domainCode(domain.ErrRefreshTokenInvalid()))
}

func TestRefresh_LockedUser_ReturnsAccountLocked(t *testing.T) {
	t.Parallel()

	svc, users, _, _, sessions, _, _, _ := newSvcForTest(t)
	sessions.byToken["rft:u1"] = "u1"
	users.byID["u1"] = domain.User{ID: "u1", Email: "e@x.com", Role: "user", Locked: true}

	_, _, err := svc.Refresh(context.Background(), "rft:u1")
	if err == nil {
		t.Fatalf("expected error")
	}
	requireDomainCode(t, err, domainCode(domain.ErrAccountLocked()))
}

func TestRefresh_Success_RotatesAndIssuesAccess(t *testing.T) {
	t.Parallel()

	svc, users, _, _, sessions, _, _, _ := newSvcForTest(t)

	users.byID["u1"] = domain.User{ID: "u1", Email: "e@x.com", Role: "user", Locked: false}
	sessions.byToken["rft:u1"] = "u1"

	toks, _, err := svc.Refresh(context.Background(), "rft:u1")
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if toks.AccessToken == "" {
		t.Fatalf("expected access token")
	}
	if toks.RefreshToken == "" || toks.RefreshToken == "rft:u1" {
		t.Fatalf("expected rotated refresh token, got %q", toks.RefreshToken)
	}
	if _, ok := sessions.byToken["rft:u1"]; ok {
		t.Fatalf("expected old refresh removed")
	}
	// new token should exist
	if _, ok := sessions.byToken[toks.RefreshToken]; !ok {
		t.Fatalf("expected new refresh stored")
	}

	// sanity: expires in matches TTL
	if toks.ExpiresIn != int64((15 * time.Minute).Seconds()) {
		t.Fatalf("unexpected expires_in: got %d", toks.ExpiresIn)
	}
}
func TestRefresh_RotateFails_ReturnsRefreshTokenInvalid(t *testing.T) {
	t.Parallel()

	svc, users, _, _, sessions, _, _, _ := newSvcForTest(t)

	users.byID["u1"] = domain.User{ID: "u1", Email: "e@x.com", Role: "user", Locked: false}
	sessions.byToken["rft:u1"] = "u1"
	sessions.rotateErr = errors.New("rotate fail")

	_, _, err := svc.Refresh(context.Background(), "rft:u1")
	requireErrCode(t, err, "refresh_token_invalid")
}

func TestRefresh_SignFails_ReturnsTokenSignFailed(t *testing.T) {
	t.Parallel()

	svc, users, _, signer, sessions, _, _, _ := newSvcForTest(t)

	users.byID["u1"] = domain.User{ID: "u1", Email: "e@x.com", Role: "user", Locked: false}
	sessions.byToken["rft:u1"] = "u1"

	signer.signFn = func(userID, role string, ttl time.Duration) (string, error) {
		return "", errors.New("sign fail")
	}

	_, _, err := svc.Refresh(context.Background(), "rft:u1")
	requireErrCode(t, err, "token_sign_failed")
}
