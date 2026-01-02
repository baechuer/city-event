//go:build integration
// +build integration

package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/baechuer/real-time-ressys/services/join-service/internal/domain"
	"github.com/baechuer/real-time-ressys/services/join-service/internal/infrastructure/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper: Setup DB connection and reset state.
func setupRepo(t *testing.T) (*postgres.Repository, *pgxpool.Pool) {
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("Skipping integration test: TEST_DB_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)

	// Wipe DB to ensure clean state
	WipeDB(t, pool)

	// Apply migrations
	// We are in services/join-service/internal/infrastructure/postgres
	// Migrations are in services/join-service/migrations
	ApplyMigrations(t, pool, "../../../migrations")

	// RESTART IDENTITY CASCADE ensures that all sequences are reset and
	// dependent data in all related tables is wiped clean for a fresh test run.
	_, err = pool.Exec(context.Background(), "TRUNCATE TABLE joins, event_capacity, outbox, event_bans, processed_messages RESTART IDENTITY CASCADE")
	require.NoError(t, err)

	return postgres.New(pool), pool
}

// TestJoinFlow_CapacityLimits verifies the standard flow: Active -> Waitlist.
func TestJoinFlow_CapacityLimits(t *testing.T) {
	repo, pool := setupRepo(t)
	ctx := context.Background()
	eventID := uuid.New()

	// 1. Initialize event with a strict capacity of 1.
	err := repo.InitCapacity(ctx, eventID, 1)
	require.NoError(t, err)

	// 2. User A joins: Should be 'active' as it's the first person.
	u1 := uuid.New()
	status, err := repo.JoinEvent(ctx, "trace-1", "", eventID, u1)
	assert.NoError(t, err)
	assert.Equal(t, domain.StatusActive, status)

	// Verify that a 'join.created' message exists in the outbox[cite: 79].
	var count int
	pool.QueryRow(ctx, "SELECT count(*) FROM outbox WHERE routing_key='join.created'").Scan(&count)
	assert.Equal(t, 1, count)

	// 3. User B joins: Capacity is full, so they must be 'waitlisted'.
	u2 := uuid.New()
	status, err = repo.JoinEvent(ctx, "trace-2", "", eventID, u2)
	assert.NoError(t, err)
	assert.Equal(t, domain.StatusWaitlisted, status)

	// 4. Verify aggregated stats match current join states[cite: 23].
	stats, _ := repo.GetStats(ctx, eventID)
	assert.Equal(t, 1, stats.ActiveCount)
	assert.Equal(t, 1, stats.WaitlistCount)
}

// TestCancelJoin_PromotesWaitlist verifies that canceling an active user
// automatically promotes the next person in the FIFO queue[cite: 82, 83].
func TestCancelJoin_PromotesWaitlist(t *testing.T) {
	repo, pool := setupRepo(t)
	ctx := context.Background()
	eventID := uuid.New()
	u1, u2 := uuid.New(), uuid.New()

	repo.InitCapacity(ctx, eventID, 1)

	// U1 gets the active slot, U2 goes to waitlist.
	repo.JoinEvent(ctx, "t1", "", eventID, u1)
	repo.JoinEvent(ctx, "t2", "", eventID, u2)

	// U1 cancels their participation.
	err := repo.CancelJoin(ctx, "t3", "", eventID, u1)
	assert.NoError(t, err)

	// Verify U1 is now 'canceled'.
	var s1 string
	pool.QueryRow(ctx, "SELECT status FROM joins WHERE event_id=$1 AND user_id=$2", eventID, u1).Scan(&s1)
	assert.Equal(t, "canceled", s1)

	// Verify U2 was promoted to 'active' automatically[cite: 82].
	var s2 string
	pool.QueryRow(ctx, "SELECT status FROM joins WHERE event_id=$1 AND user_id=$2", eventID, u2).Scan(&s2)
	assert.Equal(t, "active", s2)

	// Verify 'join.promoted' event was recorded for downstream notification[cite: 83].
	var promotedCount int
	pool.QueryRow(ctx, "SELECT count(*) FROM outbox WHERE routing_key='join.promoted'").Scan(&promotedCount)
	assert.Equal(t, 1, promotedCount)
}

// TestJoin_Idempotency ensures a user cannot join the same event twice[cite: 77].
func TestJoin_Idempotency(t *testing.T) {
	repo, _ := setupRepo(t)
	ctx := context.Background()
	eventID := uuid.New()
	userA, userB, userC, userD := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	repo.InitCapacity(ctx, eventID, 1) // Start with capacity 1

	// 1. Join A
	status, err := repo.JoinEvent(context.Background(), "trace1", "", eventID, userA)
	require.NoError(t, err)
	require.Equal(t, domain.StatusActive, status)

	// 2. Join A again -> AlreadyJoined
	_, err = repo.JoinEvent(context.Background(), "trace2", "", eventID, userA)
	require.ErrorIs(t, err, domain.ErrAlreadyJoined)

	// 3. User B joins -> Waitlisted
	status, err = repo.JoinEvent(context.Background(), "trace3", "", eventID, userB)
	require.NoError(t, err)
	require.Equal(t, domain.StatusWaitlisted, status)

	// 4. Cancel A -> B promoted
	err = repo.CancelJoin(context.Background(), "trace4", "", eventID, userA)
	require.NoError(t, err)

	// 5. Check B promoted (by listing participants or join event)
	// Using JoinEvent for "get status" via err check is hacky but confirms status
	status, err = repo.JoinEvent(context.Background(), "trace5", "", eventID, userB)
	// Should be AlreadyJoined (logic) but actually we can check DB or List
	require.ErrorIs(t, err, domain.ErrAlreadyJoined)
	// We can't easily check current status via JoinEvent return value when it errors,
	// assuming JoinEvent returns correct status even on ErrAlreadyJoined is risky unless we changed implementation.
	// Actually JoinEvent returns "" on ErrAlreadyJoined currently in repo.

	// Let's use ListParticipants to verify B is active
	list, _, err := repo.ListParticipants(context.Background(), eventID, 10, nil)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, userB, list[0].UserID)

	// 6. Init Capacity Update
	err = repo.InitCapacity(context.Background(), eventID, 5)
	require.NoError(t, err)

	// 7. Join C -> Active
	status, err = repo.JoinEvent(context.Background(), "trace6", "", eventID, userC)
	require.NoError(t, err)
	require.Equal(t, domain.StatusActive, status)

	// 8. Join D -> Active
	status, err = repo.JoinEvent(context.Background(), "trace7", "", eventID, userD)
	require.NoError(t, err)
	require.Equal(t, domain.StatusActive, status)
}

// TestHandleEventCanceled verifies bulk expiration logic when an event is canceled[cite: 90, 94, 97].
func TestHandleEventCanceled(t *testing.T) {
	repo, pool := setupRepo(t)
	ctx := context.Background()
	eventID := uuid.New()

	// Initialize capacity and add an active user to be expired.
	err := repo.InitCapacity(ctx, eventID, 10)
	require.NoError(t, err)

	u1 := uuid.New()
	// User must be successfully joined as 'active' before testing the cancel flow.
	status, err := repo.JoinEvent(ctx, "trace-setup", "", eventID, u1)
	require.NoError(t, err)
	require.Equal(t, domain.StatusActive, status)

	// Execute bulk cancellation logic[cite: 86].
	err = repo.HandleEventCanceled(ctx, "trace-cancel-action", eventID, "Organiser cancelled")
	assert.NoError(t, err)

	// Verify that notification messages were queued for all affected users.
	var outboxCount int
	// NOTE: We check for 'email.event_canceled' which is the key used in repository.go.
	err = pool.QueryRow(ctx,
		"SELECT count(*) FROM outbox WHERE routing_key = 'email.event_canceled' AND trace_id = 'trace-cancel-action'",
	).Scan(&outboxCount)

	assert.NoError(t, err)
	assert.Greater(t, outboxCount, 0, "Outbox should contain cancellation notifications")

	// Ensure user status was correctly migrated to 'expired'[cite: 94].
	var finalStatus string
	err = pool.QueryRow(ctx, "SELECT status FROM joins WHERE event_id = $1 AND user_id = $2", eventID, u1).Scan(&finalStatus)
	assert.Equal(t, "expired", finalStatus)
}

// TestProcessedMessages_Deduplication verifies the idempotency fence for incoming messages[cite: 56].
func TestProcessedMessages_Deduplication(t *testing.T) {
	repo, _ := setupRepo(t)
	ctx := context.Background()

	msgID := uuid.NewString()
	handler := "test_snapshot"

	// 1. First delivery should be accepted[cite: 56].
	ok, err := repo.TryMarkProcessed(ctx, msgID, handler)
	assert.NoError(t, err)
	assert.True(t, ok)

	// 2. Immediate redelivery (same ID) should be rejected by the fence[cite: 56].
	ok, err = repo.TryMarkProcessed(ctx, msgID, handler)
	assert.NoError(t, err)
	assert.False(t, ok)
}
