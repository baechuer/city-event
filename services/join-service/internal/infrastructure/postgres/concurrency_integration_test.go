//go:build integration
// +build integration

package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/baechuer/real-time-ressys/services/join-service/internal/domain"
	"github.com/stretchr/testify/require"

	"github.com/google/uuid"
)

func listAllParticipants(ctx context.Context, repo domain.JoinRepository, eventID uuid.UUID) ([]domain.JoinRecord, error) {
	var (
		cur  *domain.KeysetCursor
		out  []domain.JoinRecord
		seen = 0
	)
	for {
		items, next, err := repo.ListParticipants(ctx, eventID, 100, cur)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		seen += len(items)
		if next == nil || len(items) == 0 {
			return out, nil
		}
		cur = next
	}
}

func listAllWaitlist(ctx context.Context, repo domain.JoinRepository, eventID uuid.UUID) ([]domain.JoinRecord, error) {
	var (
		cur  *domain.KeysetCursor
		out  []domain.JoinRecord
		seen = 0
	)
	for {
		items, next, err := repo.ListWaitlist(ctx, eventID, 100, cur)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		seen += len(items)
		if next == nil || len(items) == 0 {
			return out, nil
		}
		cur = next
	}
}

func listAllMyJoins(ctx context.Context, repo domain.JoinRepository, userID uuid.UUID) ([]domain.JoinRecord, error) {
	var (
		cur *domain.KeysetCursor
		out []domain.JoinRecord
	)
	for {
		items, next, err := repo.ListMyJoins(ctx, userID, nil, nil, nil, 100, cur)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if next == nil || len(items) == 0 {
			return out, nil
		}
		cur = next
	}
}

func TestConcurrentJoin_DoesNotOversellCapacity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	repo, pool := setupRepo(t)

	eventID := uuid.New()
	capacity := 10
	require.NoError(t, repo.InitCapacity(ctx, eventID, capacity))

	n := 50 // 故意 > capacity + waitlistMax，允许部分 ErrEventFull
	var wg sync.WaitGroup
	wg.Add(n)

	type res struct {
		status domain.JoinStatus
		err    error
	}

	ch := make(chan res, n)

	for i := 0; i < n; i++ {
		userID := uuid.New()
		go func(uid uuid.UUID) {
			defer wg.Done()
			st, err := repo.JoinEvent(ctx, "trace-concurrent", "", eventID, uid)
			ch <- res{status: st, err: err}
		}(userID)
	}

	wg.Wait()
	close(ch)

	var (
		okActive     int
		okWaitlisted int
		fullErrors   int
		otherErrors  []error
	)

	for r := range ch {
		if r.err == nil {
			switch r.status {
			case domain.StatusActive:
				okActive++
			case domain.StatusWaitlisted:
				okWaitlisted++
			default:
				otherErrors = append(otherErrors, errors.New("unexpected status: "+string(r.status)))
			}
			continue
		}
		if errors.Is(r.err, domain.ErrEventFull) {
			fullErrors++
			continue
		}
		otherErrors = append(otherErrors, r.err)
	}

	require.Empty(t, otherErrors, "should not see unexpected errors in concurrent join")

	stats, err := repo.GetStats(ctx, eventID)
	require.NoError(t, err)

	participants, err := listAllParticipants(ctx, repo, eventID)
	require.NoError(t, err)
	waitlist, err := listAllWaitlist(ctx, repo, eventID)
	require.NoError(t, err)

	// 核心不变量
	require.LessOrEqual(t, stats.ActiveCount, capacity, "must not oversell capacity")
	require.GreaterOrEqual(t, stats.ActiveCount, 0)
	require.GreaterOrEqual(t, stats.WaitlistCount, 0)

	require.Equal(t, len(participants), stats.ActiveCount, "participants list should match stats")
	require.Equal(t, len(waitlist), stats.WaitlistCount, "waitlist list should match stats")

	// waitlist 上限（你 domain 有 WaitlistMax）
	require.LessOrEqual(t, stats.WaitlistCount, domain.WaitlistMax(capacity), "must not exceed waitlist max")

	// 结果对账：成功的 active/waitlisted 应该与读出来的 stats 一致
	require.Equal(t, okActive, stats.ActiveCount)
	require.Equal(t, okWaitlisted, stats.WaitlistCount)

	_ = pool // pool 由 setupRepo 持有即可（避免 unused）
	_ = fullErrors
}

func TestConcurrentJoin_SameUser_OneRowOnly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	repo, _ := setupRepo(t)

	eventID := uuid.New()
	require.NoError(t, repo.InitCapacity(ctx, eventID, 1))

	userID := uuid.New()

	n := 30
	var wg sync.WaitGroup
	wg.Add(n)

	errs := make(chan error, n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, err := repo.JoinEvent(ctx, "trace-same-user", "", eventID, userID)
			// 允许：nil（幂等返回成功） or ErrAlreadyJoined（你 domain 里有这个）
			if err != nil && !errors.Is(err, domain.ErrAlreadyJoined) {
				errs <- err
				return
			}
			errs <- nil
		}()
	}

	wg.Wait()
	close(errs)

	for e := range errs {
		require.NoError(t, e)
	}

	// 断言：DB 里只有 1 行 join 记录（用 repo 的 read，不硬查表名）
	my, err := listAllMyJoins(ctx, repo, userID)
	require.NoError(t, err)

	count := 0
	var st domain.JoinStatus
	for _, it := range my {
		if it.EventID == eventID {
			count++
			st = it.Status
		}
	}
	require.Equal(t, 1, count, "same user+event must have exactly one join row")
	require.True(t, st == domain.StatusActive || st == domain.StatusWaitlisted)
}

func TestConcurrentCancel_PromotesWaitlist_NoDuplicates_NoNegativeCounts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo, _ := setupRepo(t)

	eventID := uuid.New()
	capacity := 1
	require.NoError(t, repo.InitCapacity(ctx, eventID, capacity))

	// 先填满：1 active + k waitlisted（不超过 waitlist max，避免全是 ErrEventFull）
	user1 := uuid.New()
	user2 := uuid.New()
	user3 := uuid.New()

	// 1) Join 1
	status, err := repo.JoinEvent(context.Background(), "t1", "", eventID, user1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	require.Equal(t, domain.StatusActive, status)

	// 2) Join 2 (Waitlist)
	status, err = repo.JoinEvent(context.Background(), "t2", "", eventID, user2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	require.Equal(t, domain.StatusWaitlisted, status)

	// 3) Fill waitlist until full. WaitlistMax(1) is 20.
	// We already have user2 in waitlist (1/20).
	for i := 0; i < 19; i++ {
		_, err = repo.JoinEvent(context.Background(), "t-fill", "", eventID, uuid.New())
		require.NoError(t, err)
	}

	// 4) Join 3 (Full) - This should now actually fail.
	status, err = repo.JoinEvent(context.Background(), "t3", "", eventID, user3)
	if !errors.Is(err, domain.ErrEventFull) {
		t.Fatalf("expected full, got %v", err)
	}
	if status != "" {
		t.Errorf("expected empty status on error, got %s", status)
	}

	// 5) Cancel 1 -> Promotes 2
	err = repo.CancelJoin(context.Background(), "c1", "", eventID, user1)
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}

	// Verify Join 2 is now active
	rec, err := repo.GetByEventAndUser(context.Background(), eventID, user2)
	require.NoError(t, err)
	require.Equal(t, domain.StatusActive, rec.Status) // User2 should now be active

	// 并发：cancel active + 多个新 join 抢占（测试 promote/计数一致性）
	var wg sync.WaitGroup
	wg.Add(1 + 10)

	errs := make(chan error, 32)

	go func() {
		defer wg.Done()
		err := repo.CancelJoin(ctx, "trace-cancel", "", eventID, user1)
		// cancel 不应该报错
		errs <- err
	}()

	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			uid := uuid.New()
			_, err := repo.JoinEvent(ctx, "trace-join-after-cancel", "", eventID, uid)
			// 允许 full（极端情况下 waitlist 被顶满）
			if err != nil && !errors.Is(err, domain.ErrEventFull) {
				errs <- err
				return
			}
			errs <- nil
		}()
	}

	wg.Wait()
	close(errs)

	for e := range errs {
		require.NoError(t, e)
	}

	stats, err := repo.GetStats(ctx, eventID)
	require.NoError(t, err)

	participants, err := listAllParticipants(ctx, repo, eventID)
	require.NoError(t, err)
	waitlist, err := listAllWaitlist(ctx, repo, eventID)
	require.NoError(t, err)

	require.Equal(t, len(participants), stats.ActiveCount)
	require.Equal(t, len(waitlist), stats.WaitlistCount)

	require.LessOrEqual(t, stats.ActiveCount, capacity)
	require.GreaterOrEqual(t, stats.ActiveCount, 0)
	require.GreaterOrEqual(t, stats.WaitlistCount, 0)
	require.LessOrEqual(t, stats.WaitlistCount, domain.WaitlistMax(capacity))

	// 不允许重复 user 出现在 active+waitlist
	seen := map[uuid.UUID]struct{}{}
	for _, p := range participants {
		if _, ok := seen[p.UserID]; ok {
			t.Fatalf("duplicate user in participants: %s", p.UserID)
		}
		seen[p.UserID] = struct{}{}
	}
	for _, w := range waitlist {
		if _, ok := seen[w.UserID]; ok {
			t.Fatalf("user appears in both participants and waitlist: %s", w.UserID)
		}
		seen[w.UserID] = struct{}{}
	}
}
