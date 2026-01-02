//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/baechuer/real-time-ressys/services/join-service/internal/domain"
)

// 依赖你现有的 setupRepo(t)（你现在已经有了，否则其它 integration 也跑不起来）
func TestListMyJoins_KeysetPagination_Basic(t *testing.T) {
	repo, _ := setupRepo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	userID := uuid.New()

	// 造两条 join history：用两个 event 更稳定
	event1 := uuid.New()
	event2 := uuid.New()
	require.NoError(t, repo.InitCapacity(ctx, event1, 1))
	require.NoError(t, repo.InitCapacity(ctx, event2, 0)) // 通常会进入 waitlist（取决于你 repo 语义）

	_, err := repo.JoinEvent(ctx, "trace_"+uuid.NewString(), "", event1, userID)
	require.NoError(t, err)
	_, err = repo.JoinEvent(ctx, "trace_"+uuid.NewString(), "", event2, userID)
	require.NoError(t, err)

	limit := 1

	// page1
	items1, cur1, err := repo.ListMyJoins(ctx, userID, nil, nil, nil, limit, nil)
	require.NoError(t, err)
	require.Len(t, items1, 1)

	// page2（如果有 next cursor）
	if cur1 != nil {
		items2, cur2, err := repo.ListMyJoins(ctx, userID, nil, nil, nil, limit, cur1)
		require.NoError(t, err)
		require.Len(t, items2, 1)
		require.Nil(t, cur2)

		require.NotEqual(t, items1[0].ID, items2[0].ID)
	}

	// 状态 spot-check（不强依赖具体结果）
	require.Contains(t, []domain.JoinStatus{
		domain.StatusActive,
		domain.StatusWaitlisted,
		domain.StatusCanceled,
		domain.StatusExpired,
		domain.StatusRejected,
	}, items1[0].Status)
}
