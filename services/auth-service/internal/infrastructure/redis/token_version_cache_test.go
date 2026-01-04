package redis

import (
	"context"
	"testing"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/domain"
)

// fake repo that only implements token version methods + required methods used by CachedUserRepo delegates
type fakeUserRepo struct {
	getTV func(ctx context.Context, userID string) (int64, error)
	bump  func(ctx context.Context, userID string) (int64, error)
}

func (f *fakeUserRepo) GetTokenVersion(ctx context.Context, userID string) (int64, error) {
	return f.getTV(ctx, userID)
}
func (f *fakeUserRepo) BumpTokenVersion(ctx context.Context, userID string) (int64, error) {
	return f.bump(ctx, userID)
}

// below methods won't be called in these tests; keep stubs to satisfy interface
func (f *fakeUserRepo) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	return domain.User{}, nil
}
func (f *fakeUserRepo) GetByID(ctx context.Context, id string) (domain.User, error) {
	return domain.User{}, nil
}
func (f *fakeUserRepo) Create(ctx context.Context, u domain.User) (domain.User, error) { return u, nil }
func (f *fakeUserRepo) UpdatePasswordHash(ctx context.Context, userID string, newHash string) error {
	return nil
}
func (f *fakeUserRepo) SetEmailVerified(ctx context.Context, userID string) error { return nil }
func (f *fakeUserRepo) LockUser(ctx context.Context, userID string) error         { return nil }
func (f *fakeUserRepo) UnlockUser(ctx context.Context, userID string) error       { return nil }
func (f *fakeUserRepo) SetRole(ctx context.Context, userID string, role string) error {
	return nil
}
func (f *fakeUserRepo) CountByRole(ctx context.Context, role string) (int, error) { return 0, nil }
func (f *fakeUserRepo) UpdateAvatarImageID(ctx context.Context, userID string, avatarImageID *string) (*string, error) {
	return nil, nil
}

func TestCachedUserRepo_Passthrough_WhenRedisNil(t *testing.T) {
	t.Parallel()

	var gotGet, gotBump int

	inner := &fakeUserRepo{
		getTV: func(ctx context.Context, userID string) (int64, error) {
			gotGet++
			if userID != "u1" {
				t.Fatalf("unexpected userID: %q", userID)
			}
			return 7, nil
		},
		bump: func(ctx context.Context, userID string) (int64, error) {
			gotBump++
			if userID != "u1" {
				t.Fatalf("unexpected userID: %q", userID)
			}
			return 8, nil
		},
	}

	// client=nil should NOT panic, and should just call inner
	c := NewCachedUserRepo(inner, nil, 0)

	v, err := c.GetTokenVersion(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if v != 7 {
		t.Fatalf("expected 7, got %d", v)
	}

	v2, err := c.BumpTokenVersion(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if v2 != 8 {
		t.Fatalf("expected 8, got %d", v2)
	}

	if gotGet != 1 || gotBump != 1 {
		t.Fatalf("expected inner calls get=1 bump=1, got get=%d bump=%d", gotGet, gotBump)
	}
}
