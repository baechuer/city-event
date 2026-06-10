package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/baechuer/cityevents/internal/platform/identity"
	"golang.org/x/crypto/bcrypt"
)

func TestServiceRegisterLoginCurrentLogoutWorkflow(t *testing.T) {
	svc, _ := testService()
	ctx := context.Background()

	registered, err := svc.Register(ctx, RegisterCommand{
		Email:       "USER@example.com",
		Password:    "StrongerPass123",
		DisplayName: "User",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if registered.User.Email != "user@example.com" {
		t.Fatalf("expected normalized email, got %q", registered.User.Email)
	}
	if registered.User.Role != string(identity.RoleUser) {
		t.Fatalf("registered role = %q, want USER", registered.User.Role)
	}

	loggedIn, err := svc.Login(ctx, LoginCommand{Email: "user@example.com", Password: "StrongerPass123"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	current, err := svc.CurrentUser(ctx, loggedIn.AccessToken)
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	if current.ID != registered.User.ID {
		t.Fatalf("current user id = %q, want %q", current.ID, registered.User.ID)
	}
	if current.Role != string(identity.RoleUser) {
		t.Fatalf("current user role = %q, want USER", current.Role)
	}

	if err := svc.Logout(ctx, loggedIn.AccessToken, loggedIn.RefreshToken); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if err := svc.Logout(ctx, loggedIn.AccessToken, ""); err != nil {
		t.Fatalf("second logout should be safe: %v", err)
	}
	if _, err := svc.CurrentUser(ctx, loggedIn.AccessToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected revoked token to be unauthorized, got %v", err)
	}
}

func TestServiceRefreshRotatesTokenAndDetectsReuse(t *testing.T) {
	svc, _ := testService()
	ctx := context.Background()

	registered, err := svc.Register(ctx, RegisterCommand{
		Email:       "refresh@example.com",
		Password:    "StrongerPass123",
		DisplayName: "Refresh User",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if registered.RefreshToken == "" {
		t.Fatalf("expected refresh token")
	}

	refreshed, err := svc.Refresh(ctx, registered.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if refreshed.AccessToken == "" || refreshed.RefreshToken == "" {
		t.Fatalf("expected rotated access and refresh tokens")
	}
	if refreshed.RefreshToken == registered.RefreshToken {
		t.Fatalf("refresh token was not rotated")
	}
	if _, err := svc.Refresh(ctx, registered.RefreshToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected reused refresh token to be unauthorized, got %v", err)
	}
	if _, err := svc.Refresh(ctx, refreshed.RefreshToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected token family to be revoked after reuse, got %v", err)
	}
}

func TestServiceDoesNotStorePlaintextPassword(t *testing.T) {
	svc, repo := testService()
	ctx := context.Background()

	_, err := svc.Register(ctx, RegisterCommand{
		Email:       "hash@example.com",
		Password:    "StrongerPass123",
		DisplayName: "Hash User",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	user, err := repo.FindUserByEmail(ctx, "hash@example.com")
	if err != nil {
		t.Fatalf("find user: %v", err)
	}
	if user.PasswordHash == "StrongerPass123" {
		t.Fatalf("plaintext password was stored")
	}
	if !svc.hasher.Verify(user.PasswordHash, "StrongerPass123") {
		t.Fatalf("stored hash should verify original password")
	}
}

func TestServiceRejectsDuplicateRegistration(t *testing.T) {
	svc, repo := testService()
	ctx := context.Background()

	cmd := RegisterCommand{Email: "dupe@example.com", Password: "StrongerPass123", DisplayName: "User"}
	if _, err := svc.Register(ctx, cmd); err != nil {
		t.Fatalf("first register: %v", err)
	}
	for i := 0; i < 20; i++ {
		if _, err := svc.Register(ctx, cmd); !errors.Is(err, ErrDuplicateEmail) {
			t.Fatalf("attempt %d expected duplicate email, got %v", i, err)
		}
	}
	if count := repo.CountUsersByEmail("dupe@example.com"); count != 1 {
		t.Fatalf("expected one user after duplicates, got %d", count)
	}
}

func TestServiceConcurrentDuplicateRegistrationCreatesOneUser(t *testing.T) {
	svc, repo := testService()
	ctx := context.Background()
	cmd := RegisterCommand{Email: "race@example.com", Password: "StrongerPass123", DisplayName: "User"}

	var wg sync.WaitGroup
	errs := make(chan error, 25)
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Register(ctx, cmd)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	successes := 0
	for err := range errs {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, ErrDuplicateEmail) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("expected one successful registration, got %d", successes)
	}
	if count := repo.CountUsersByEmail("race@example.com"); count != 1 {
		t.Fatalf("expected one persisted user, got %d", count)
	}
}

func TestServiceRateLimitsRepeatedWrongLogin(t *testing.T) {
	svc, _ := testService()
	ctx := context.Background()
	_, err := svc.Register(ctx, RegisterCommand{Email: "rate@example.com", Password: "StrongerPass123", DisplayName: "User"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	for i := 0; i < 3; i++ {
		_, err := svc.Login(ctx, LoginCommand{Email: "rate@example.com", Password: "WrongPass123"})
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d expected invalid credentials, got %v", i, err)
		}
	}
	_, err = svc.Login(ctx, LoginCommand{Email: "rate@example.com", Password: "WrongPass123"})
	if !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("expected rate limit, got %v", err)
	}
}

func TestServiceSeedAdminAndRoleUpdates(t *testing.T) {
	svc, repo := testService()
	ctx := context.Background()

	admin, err := svc.EnsureSeedAdmin(ctx, SeedAdminCommand{
		Email:       "ADMIN@example.com",
		Password:    "AdminPass12345",
		DisplayName: "Admin",
	})
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	if admin.Email != "admin@example.com" || admin.Role != string(identity.RoleAdmin) {
		t.Fatalf("unexpected admin: %+v", admin)
	}

	userResult, err := svc.Register(ctx, RegisterCommand{
		Email:       "organizer@example.com",
		Password:    "StrongerPass123",
		DisplayName: "Organizer",
	})
	if err != nil {
		t.Fatalf("register user: %v", err)
	}

	updated, err := svc.UpdateUserRole(ctx, UpdateRoleCommand{
		ActorUserID:  admin.ID,
		TargetUserID: userResult.User.ID,
		Role:         identity.RoleOrganizer,
	})
	if err != nil {
		t.Fatalf("update role: %v", err)
	}
	if updated.Role != string(identity.RoleOrganizer) {
		t.Fatalf("updated role = %q, want ORGANIZER", updated.Role)
	}

	plain, err := svc.Register(ctx, RegisterCommand{
		Email:       "plain@example.com",
		Password:    "StrongerPass123",
		DisplayName: "Plain",
	})
	if err != nil {
		t.Fatalf("register plain: %v", err)
	}
	if _, err := svc.UpdateUserRole(ctx, UpdateRoleCommand{
		ActorUserID:  plain.User.ID,
		TargetUserID: userResult.User.ID,
		Role:         identity.RoleAdmin,
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected non-admin role update to be forbidden, got %v", err)
	}

	stored, err := repo.FindUserByEmail(ctx, "organizer@example.com")
	if err != nil {
		t.Fatalf("find organizer: %v", err)
	}
	if stored.Role != identity.RoleOrganizer {
		t.Fatalf("stored role = %q, want ORGANIZER", stored.Role)
	}
}

func testService() (*Service, *MemoryRepository) {
	repo := NewMemoryRepository()
	hasher := NewPasswordHasher(bcrypt.MinCost)
	tokens := NewTokenManager("secret", "cityevents-test", time.Hour)
	guard := NewLoginGuard(3, time.Minute)
	return NewService(repo, hasher, tokens, guard), repo
}
