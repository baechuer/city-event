//go:build integration
// +build integration

package postgres_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/baechuer/real-time-ressys/services/join-service/internal/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestModeration_Kick_PromotesWaitlist_AndWritesOutbox(t *testing.T) {
	repo, pool := setupRepo(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	eventID := uuid.New()
	actorID := uuid.New()

	require.NoError(t, repo.InitCapacity(ctx, eventID, 1))

	u1 := uuid.New()
	u2 := uuid.New()

	st, err := repo.JoinEvent(ctx, "t1", "", eventID, u1)
	require.NoError(t, err)
	require.Equal(t, "active", string(st))

	st, err = repo.JoinEvent(ctx, "t2", "", eventID, u2)
	require.NoError(t, err)
	require.Equal(t, "waitlisted", string(st))

	require.NoError(t, repo.Kick(ctx, "trace-kick", eventID, u1, actorID, "no show"))

	var s1, s2 string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM joins WHERE event_id=$1 AND user_id=$2`, eventID, u1).Scan(&s1))
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM joins WHERE event_id=$1 AND user_id=$2`, eventID, u2).Scan(&s2))
	require.Equal(t, "rejected", s1)
	require.Equal(t, "active", s2)

	var activeCnt, waitCnt int
	require.NoError(t, pool.QueryRow(ctx, `SELECT active_count, waitlist_count FROM event_capacity WHERE event_id=$1`, eventID).Scan(&activeCnt, &waitCnt))
	require.Equal(t, 1, activeCnt)
	require.Equal(t, 0, waitCnt)

	rows, err := pool.Query(ctx, `SELECT routing_key FROM outbox WHERE trace_id=$1`, "trace-kick")
	require.NoError(t, err)
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var k string
		require.NoError(t, rows.Scan(&k))
		keys = append(keys, k)
	}
	sort.Strings(keys)
	require.Equal(t, []string{"join.kicked", "join.promoted"}, keys)
}

func TestModeration_Ban_And_Unban_WritesOutbox_AndEnforcesBanRow(t *testing.T) {
	repo, pool := setupRepo(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	eventID := uuid.New()
	actorID := uuid.New()
	target := uuid.New()

	require.NoError(t, repo.InitCapacity(ctx, eventID, 1))
	_, _ = repo.JoinEvent(ctx, "t1", "", eventID, target)

	require.NoError(t, repo.Ban(ctx, "trace-ban", eventID, target, actorID, "spam", nil))

	var banned bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM event_bans WHERE event_id=$1 AND user_id=$2)`, eventID, target).Scan(&banned))
	require.True(t, banned)

	var st string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM joins WHERE event_id=$1 AND user_id=$2`, eventID, target).Scan(&st))
	require.Equal(t, "rejected", st)

	var hasBanned bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM outbox WHERE trace_id=$1 AND routing_key='join.banned')`, "trace-ban").Scan(&hasBanned))
	require.True(t, hasBanned)

	require.NoError(t, repo.Unban(ctx, "trace-unban", eventID, target, actorID))

	require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM event_bans WHERE event_id=$1 AND user_id=$2)`, eventID, target).Scan(&banned))
	require.False(t, banned)

	var hasUnbanned bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM outbox WHERE trace_id=$1 AND routing_key='join.unbanned')`, "trace-unban").Scan(&hasUnbanned))
	require.True(t, hasUnbanned)
}
func TestModeration_Kick_PromotesWaitlist_UpdatesCounters_AndOutbox(t *testing.T) {
	repo, pool := setupRepo(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	eventID := uuid.New()
	actorID := uuid.New()
	u1 := uuid.New()
	u2 := uuid.New()

	require.NoError(t, repo.InitCapacity(ctx, eventID, 1))

	st, err := repo.JoinEvent(ctx, "t-join-1", "", eventID, u1)
	require.NoError(t, err)
	require.Equal(t, domain.StatusActive, st)

	st, err = repo.JoinEvent(ctx, "t-join-2", "", eventID, u2)
	require.NoError(t, err)
	require.Equal(t, domain.StatusWaitlisted, st)

	trace := "trace-kick-1"
	require.NoError(t, repo.Kick(ctx, trace, eventID, u1, actorID, "no-show"))

	var s1, s2 string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM joins WHERE event_id=$1 AND user_id=$2`, eventID, u1).Scan(&s1))
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM joins WHERE event_id=$1 AND user_id=$2`, eventID, u2).Scan(&s2))
	require.Equal(t, "rejected", s1)
	require.Equal(t, "active", s2)

	var activeCnt, waitCnt int
	require.NoError(t, pool.QueryRow(ctx, `SELECT active_count, waitlist_count FROM event_capacity WHERE event_id=$1`, eventID).Scan(&activeCnt, &waitCnt))
	require.Equal(t, 1, activeCnt)
	require.Equal(t, 0, waitCnt)

	// outbox 至少包含 kicked + promoted（你当前实现是两条）
	var kicked, promoted bool
	rows, err := pool.Query(ctx, `SELECT routing_key FROM outbox WHERE trace_id=$1`, trace)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var rk string
		require.NoError(t, rows.Scan(&rk))
		if rk == "join.kicked" {
			kicked = true
		}
		if rk == "join.promoted" {
			promoted = true
		}
	}
	require.True(t, kicked, "expected outbox join.kicked")
	require.True(t, promoted, "expected outbox join.promoted")
}

func TestModeration_Ban_BlocksJoin_AndWritesOutbox(t *testing.T) {
	repo, pool := setupRepo(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	eventID := uuid.New()
	actorID := uuid.New()
	target := uuid.New()

	require.NoError(t, repo.InitCapacity(ctx, eventID, 10))

	trace := "trace-ban-1"
	require.NoError(t, repo.Ban(ctx, trace, eventID, target, actorID, "spam", nil))

	_, err := repo.JoinEvent(ctx, "t-join-banned", "", eventID, target)
	require.ErrorIs(t, err, domain.ErrBanned)

	var exists bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM event_bans WHERE event_id=$1 AND user_id=$2)`, eventID, target).Scan(&exists))
	require.True(t, exists)

	require.NoError(t, pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM outbox WHERE trace_id=$1 AND routing_key='join.banned')`,
		trace,
	).Scan(&exists))
	require.True(t, exists)
}

func TestModeration_Ban_ExpiresAt_AllowsJoinAfterExpiry(t *testing.T) {
	repo, pool := setupRepo(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	eventID := uuid.New()
	actorID := uuid.New()
	target := uuid.New()

	require.NoError(t, repo.InitCapacity(ctx, eventID, 10))

	exp := time.Now().Add(-1 * time.Minute).UTC()
	require.NoError(t, repo.Ban(ctx, "trace-ban-expired", eventID, target, actorID, "temp", &exp))

	st, err := repo.JoinEvent(ctx, "t-join-after-exp", "", eventID, target)
	require.NoError(t, err)
	require.True(t, st == domain.StatusActive || st == domain.StatusWaitlisted)

	_ = pool
}
