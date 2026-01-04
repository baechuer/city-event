package redis

import (
	"context"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/application/auth"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/domain"
)

// CachedUserRepo decorates an auth.UserRepo with a Redis cache for token_version.
// - Read path: Redis -> DB fallback -> Redis set
// - Write path (bump): DB -> Redis set (best effort)
type CachedUserRepo struct {
	inner   auth.UserRepo
	rdb     *goredis.Client
	ttl     time.Duration
	keyPref string
}

func NewCachedUserRepo(inner auth.UserRepo, client *Client, ttl time.Duration) *CachedUserRepo {
	var rdb *goredis.Client
	if client != nil {
		rdb = client.rdb
	}
	return &CachedUserRepo{
		inner:   inner,
		rdb:     rdb,
		ttl:     ttl,
		keyPref: "tokenver:",
	}
}

func (c *CachedUserRepo) key(userID string) string {
	return c.keyPref + userID
}

func (c *CachedUserRepo) GetTokenVersion(ctx context.Context, userID string) (int64, error) {
	// 1) Try Redis
	if c.rdb != nil {
		s, err := c.rdb.Get(ctx, c.key(userID)).Result()
		if err == nil {
			if v, perr := strconv.ParseInt(s, 10, 64); perr == nil {
				return v, nil
			}
			// parse error -> fall back to DB
		} else if err != goredis.Nil {
			// redis error -> fall back to DB (do NOT fail auth)
		}
	}

	// 2) DB source of truth
	v, err := c.inner.GetTokenVersion(ctx, userID)
	if err != nil {
		return 0, err
	}

	// 3) Best-effort cache fill
	if c.rdb != nil {
		_ = c.rdb.Set(ctx, c.key(userID), strconv.FormatInt(v, 10), c.ttl).Err()
	}

	return v, nil
}

func (c *CachedUserRepo) BumpTokenVersion(ctx context.Context, userID string) (int64, error) {
	// 1) DB bump
	v, err := c.inner.BumpTokenVersion(ctx, userID)
	if err != nil {
		return 0, err
	}

	// 2) Best-effort cache update (SET beats DEL)
	if c.rdb != nil {
		_ = c.rdb.Set(ctx, c.key(userID), strconv.FormatInt(v, 10), c.ttl).Err()
	}

	return v, nil
}

/*
Below: delegate all other auth.UserRepo methods to inner.
This keeps the change isolated to bootstrap wiring.
*/

func (c *CachedUserRepo) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	return c.inner.GetByEmail(ctx, email)
}
func (c *CachedUserRepo) GetByID(ctx context.Context, id string) (domain.User, error) {
	return c.inner.GetByID(ctx, id)
}
func (c *CachedUserRepo) Create(ctx context.Context, u domain.User) (domain.User, error) {
	return c.inner.Create(ctx, u)
}
func (c *CachedUserRepo) UpdatePasswordHash(ctx context.Context, userID string, newHash string) error {
	return c.inner.UpdatePasswordHash(ctx, userID, newHash)
}
func (c *CachedUserRepo) SetEmailVerified(ctx context.Context, userID string) error {
	return c.inner.SetEmailVerified(ctx, userID)
}
func (c *CachedUserRepo) LockUser(ctx context.Context, userID string) error {
	return c.inner.LockUser(ctx, userID)
}
func (c *CachedUserRepo) UnlockUser(ctx context.Context, userID string) error {
	return c.inner.UnlockUser(ctx, userID)
}
func (c *CachedUserRepo) SetRole(ctx context.Context, userID string, role string) error {
	return c.inner.SetRole(ctx, userID, role)
}
func (c *CachedUserRepo) CountByRole(ctx context.Context, role string) (int, error) {
	return c.inner.CountByRole(ctx, role)
}
func (c *CachedUserRepo) UpdateAvatarImageID(ctx context.Context, userID string, avatarImageID *string) (*string, error) {
	return c.inner.UpdateAvatarImageID(ctx, userID, avatarImageID)
}
