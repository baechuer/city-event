//go:build integration

package auth

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/baechuer/cityevents/internal/platform/config"
	"github.com/baechuer/cityevents/internal/platform/identity"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func TestPostgresRepositoryUserAndRevocation(t *testing.T) {
	ctx := context.Background()
	pool := setupAuthPostgres(t, ctx)
	repo := NewPostgresRepository(pool)

	user := User{
		ID:           NewID(),
		Email:        "repo@example.com",
		DisplayName:  "Repo User",
		PasswordHash: "hash",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := repo.CreateUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := repo.CreateUser(ctx, user); !errors.Is(err, ErrDuplicateEmail) {
		t.Fatalf("expected duplicate email, got %v", err)
	}

	byEmail, err := repo.FindUserByEmail(ctx, "REPO@example.com")
	if err != nil {
		t.Fatalf("find by email: %v", err)
	}
	if byEmail.ID != user.ID {
		t.Fatalf("find by email id = %q, want %q", byEmail.ID, user.ID)
	}
	if byEmail.Role != identity.RoleUser {
		t.Fatalf("find by email role = %q, want USER", byEmail.Role)
	}

	byID, err := repo.FindUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if byID.Email != user.Email {
		t.Fatalf("find by id email = %q, want %q", byID.Email, user.Email)
	}

	updated, err := repo.UpdateUserRole(ctx, user.ID, identity.RoleOrganizer)
	if err != nil {
		t.Fatalf("update role: %v", err)
	}
	if updated.Role != identity.RoleOrganizer {
		t.Fatalf("updated role = %q, want ORGANIZER", updated.Role)
	}

	if err := repo.RevokeToken(ctx, "token-1", user.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("revoke token: %v", err)
	}
	revoked, err := repo.IsTokenRevoked(ctx, "token-1")
	if err != nil {
		t.Fatalf("is token revoked: %v", err)
	}
	if !revoked {
		t.Fatalf("expected token to be revoked")
	}
}

func TestAuthPostgresWorkflowIntegration(t *testing.T) {
	ctx := context.Background()
	pool := setupAuthPostgres(t, ctx)
	repo := NewPostgresRepository(pool)

	tokens := NewTokenManager("secret", "cityevents-test", time.Hour)
	svc := NewService(repo, NewPasswordHasher(bcrypt.MinCost), tokens, NewLoginGuard(5, time.Minute))
	cfg, err := config.Load("auth-service", nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	router := NewHTTPHandler(cfg, nil, svc)

	registerResp := doJSON(router, http.MethodPost, "/v1/auth/register", `{"email":"pg@example.com","password":"StrongerPass123","displayName":"PG User"}`, "")
	if registerResp.Code != http.StatusCreated {
		t.Fatalf("register status = %d body=%s", registerResp.Code, registerResp.Body.String())
	}

	loginResp := doJSON(router, http.MethodPost, "/v1/auth/login", `{"email":"pg@example.com","password":"StrongerPass123"}`, "")
	if loginResp.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", loginResp.Code, loginResp.Body.String())
	}
	token := accessTokenFromBody(t, loginResp.Body.Bytes())

	meResp := doJSON(router, http.MethodGet, "/v1/auth/me", "", token)
	if meResp.Code != http.StatusOK {
		t.Fatalf("me status = %d body=%s", meResp.Code, meResp.Body.String())
	}

	logoutResp := doJSON(router, http.MethodPost, "/v1/auth/logout", "", token)
	if logoutResp.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d body=%s", logoutResp.Code, logoutResp.Body.String())
	}

	revokedResp := doJSON(router, http.MethodGet, "/v1/auth/me", "", token)
	if revokedResp.Code != http.StatusUnauthorized {
		t.Fatalf("revoked me status = %d body=%s", revokedResp.Code, revokedResp.Body.String())
	}
}

func setupAuthPostgres(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://cityevents:cityevents@localhost:5432/cityevents?sslmode=disable"
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}

	if _, err := pool.Exec(ctx, `DROP TABLE IF EXISTS revoked_tokens; DROP TABLE IF EXISTS auth_users;`); err != nil {
		t.Fatalf("reset auth tables: %v", err)
	}

	migrationPath := filepath.Join("..", "..", "..", "migrations", "auth", "001_init.sql")
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	return pool
}
