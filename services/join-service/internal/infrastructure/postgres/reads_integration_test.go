//go:build integration
// +build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/baechuer/real-time-ressys/services/join-service/internal/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestReads_ListMyJoins_KeysetPaging(t *testing.T) {
	repo, pool := setupRepo(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userID := uuid.New()
	e1 := uuid.New()
	e2 := uuid.New()

	require.NoError(t, repo.InitCapacity(ctx, e1, 10))
	require.NoError(t, repo.InitCapacity(ctx, e2, 10))

	_, err := repo.JoinEvent(ctx, "t1", "", e1, userID)
	require.NoError(t, err)
	_, err = repo.JoinEvent(ctx, "t2", "", e2, userID)
	require.NoError(t, err)

	var j1, j2 uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM joins WHERE event_id=$1 AND user_id=$2`, e1, userID).Scan(&j1))
	require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM joins WHERE event_id=$1 AND user_id=$2`, e2, userID).Scan(&j2))

	t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	_, err = pool.Exec(ctx, `UPDATE joins SET created_at=$1, updated_at=$1 WHERE id=$2`, t1, j1)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE joins SET created_at=$1, updated_at=$1 WHERE id=$2`, t2, j2)
	require.NoError(t, err)

	records, next, err := repo.ListMyJoins(ctx, userID, []domain.JoinStatus{}, nil, nil, 1, nil)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, e2, records[0].EventID)
	require.NotNil(t, next)

	records2, next2, err := repo.ListMyJoins(ctx, userID, []domain.JoinStatus{}, nil, nil, 1, next)
	require.NoError(t, err)
	require.Len(t, records2, 1)
	require.Equal(t, e1, records2[0].EventID)
	require.Nil(t, next2)
}

func TestReads_ListParticipants_And_Waitlist_FilterStatusOnly(t *testing.T) {
	repo, pool := setupRepo(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	eventID := uuid.New()
	require.NoError(t, repo.InitCapacity(ctx, eventID, 1))

	u1 := uuid.New()
	u2 := uuid.New()

	st, err := repo.JoinEvent(ctx, "t1", "", eventID, u1)
	require.NoError(t, err)
	require.Equal(t, "active", string(st))

	st, err = repo.JoinEvent(ctx, "t2", "", eventID, u2)
	require.NoError(t, err)
	require.Equal(t, "waitlisted", string(st))

	part, _, err := repo.ListParticipants(ctx, eventID, 10, nil)
	require.NoError(t, err)
	require.Len(t, part, 1)
	require.Equal(t, u1, part[0].UserID)

	wait, _, err := repo.ListWaitlist(ctx, eventID, 10, nil)
	require.NoError(t, err)
	require.Len(t, wait, 1)
	require.Equal(t, u2, wait[0].UserID)
}
