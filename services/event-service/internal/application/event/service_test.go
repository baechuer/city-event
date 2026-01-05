package event

import (
	"context"
	"testing"
	"time"

	"github.com/baechuer/real-time-ressys/services/event-service/internal/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// --- Mocks & Helpers ---

type fakeClock struct{ t time.Time }

func (c fakeClock) Now() time.Time { return c.t }

// Mock Cache
type mockCache struct {
	store map[string]any
}

func newMockCache() *mockCache { return &mockCache{store: make(map[string]any)} }

func (m *mockCache) Get(ctx context.Context, key string, dest any) (bool, error) {
	return false, nil
}
func (m *mockCache) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	m.store[key] = val
	return nil
}
func (m *mockCache) Delete(ctx context.Context, keys ...string) error {
	for _, k := range keys {
		delete(m.store, k)
	}
	return nil
}

// memRepo 实现了 EventRepo 和 TxEventRepo (为了简化测试)
type memRepo struct {
	byID map[string]*domain.Event
}

func newMemRepo() *memRepo { return &memRepo{byID: map[string]*domain.Event{}} }

func (m *memRepo) Create(ctx context.Context, e *domain.Event) error {
	m.byID[e.ID] = e
	return nil
}

func (m *memRepo) GetByID(ctx context.Context, id string) (*domain.Event, error) {
	e, ok := m.byID[id]
	if !ok {
		return nil, domain.ErrNotFound("event not found")
	}
	return e, nil
}

func (m *memRepo) GetByIDForUpdate(ctx context.Context, id string) (*domain.Event, error) {
	return m.GetByID(ctx, id)
}

func (m *memRepo) Update(ctx context.Context, e *domain.Event) error {
	m.byID[e.ID] = e
	return nil
}

func (m *memRepo) InsertOutbox(ctx context.Context, msg OutboxMessage) error {
	return nil
}

// 模拟事务逻辑
func (m *memRepo) WithTx(ctx context.Context, fn func(r TxEventRepo) error) error {
	return fn(m)
}

func (m *memRepo) ListPublicTimeKeyset(ctx context.Context, f ListFilter, hasCursor bool, afterStart time.Time, afterID string) ([]*domain.Event, error) {
	return []*domain.Event{}, nil
}

func (m *memRepo) ListPublicRelevanceKeyset(ctx context.Context, f ListFilter, hasCursor bool, afterRank float64, afterStart time.Time, afterID string) ([]*domain.Event, []float64, error) {
	return []*domain.Event{}, []float64{}, nil
}

func (m *memRepo) ListByOwner(ctx context.Context, ownerID string, status string, page, pageSize int) ([]*domain.Event, int, error) {
	return []*domain.Event{}, 0, nil
}

func (m *memRepo) IncrementParticipantCount(ctx context.Context, eventID uuid.UUID) error {
	// Simple no-op or simulate update if needed
	return nil
}

func (m *memRepo) DecrementParticipantCount(ctx context.Context, eventID uuid.UUID) error {
	return nil
}

func (m *memRepo) GetCitySuggestions(ctx context.Context, query string, limit int) ([]string, error) {
	return []string{}, nil
}

func (m *memRepo) GetByIDs(ctx context.Context, ids []string) ([]*domain.Event, error) {
	var result []*domain.Event
	for _, id := range ids {
		if e, ok := m.byID[id]; ok && e.Status == domain.StatusPublished {
			result = append(result, e)
		}
	}
	return result, nil
}

func mustTime(t *testing.T, s string) time.Time {
	tt, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return tt.UTC()
}

// --- Test Cases ---

func TestService_Cancel_Success(t *testing.T) {
	now := mustTime(t, "2025-12-25T10:00:00Z")
	repo := newMemRepo()
	// 修复点：移除多余的 NoopPublisher{} 参数
	svc := New(repo, fakeClock{t: now}, newMockCache(), 0, 0)

	eventID := "evt_123"
	ownerID := "user_A"
	repo.byID[eventID] = &domain.Event{
		ID:      eventID,
		OwnerID: ownerID,
		Status:  domain.StatusPublished,
	}

	t.Run("owner_can_cancel", func(t *testing.T) {
		ev, err := svc.Cancel(context.Background(), eventID, ownerID, "user", "")
		assert.NoError(t, err)
		assert.Equal(t, domain.StatusCanceled, ev.Status)
		assert.NotNil(t, ev.CanceledAt)
	})

	t.Run("admin_can_cancel_any_event", func(t *testing.T) {
		repo.byID[eventID].Status = domain.StatusPublished
		ev, err := svc.Cancel(context.Background(), eventID, "admin_user", "admin", "")
		assert.NoError(t, err)
		assert.Equal(t, domain.StatusCanceled, ev.Status)
	})
}

func TestService_Publish_Validation(t *testing.T) {
	now := mustTime(t, "2025-12-25T10:00:00Z")
	repo := newMemRepo()
	// 修复点：移除多余的 NoopPublisher{} 参数
	svc := New(repo, fakeClock{t: now}, newMockCache(), 0, 0)

	t.Run("cannot_publish_with_start_time_in_past", func(t *testing.T) {
		eventID := "evt_past"
		repo.byID[eventID] = &domain.Event{
			ID:        eventID,
			OwnerID:   "owner",
			Status:    domain.StatusDraft,
			StartTime: now.Add(-1 * time.Hour),
			EndTime:   now.Add(1 * time.Hour),
		}

		_, err := svc.Publish(context.Background(), eventID, "owner", "user")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot publish event in the past")
	})
}

func TestService_ListPublic_CursorLogic(t *testing.T) {
	now := mustTime(t, "2025-12-25T10:00:00Z")
	repo := newMemRepo()
	// 修复点：移除多余的 NoopPublisher{} 参数
	svc := New(repo, fakeClock{t: now}, newMockCache(), 0, 0)

	t.Run("time_sort_without_cursor", func(t *testing.T) {
		f := ListFilter{Sort: "time"}
		res, err := svc.ListPublic(context.Background(), f)
		assert.NoError(t, err)
		assert.Equal(t, "", res.NextCursor)
	})

	t.Run("relevance_sort_requires_query", func(t *testing.T) {
		f := ListFilter{Sort: "relevance", Query: ""}
		_, err := svc.ListPublic(context.Background(), f)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "required when sort=relevance")
	})
}

func TestService_Create_Permissions(t *testing.T) {
	now := mustTime(t, "2025-12-25T10:00:00Z")
	repo := newMemRepo()
	svc := New(repo, fakeClock{t: now}, newMockCache(), 0, 0)

	tests := []struct {
		role    string
		wantErr bool
	}{
		{"user", false},
		{"organizer", false},
		{"admin", false},
		{"", true},
	}

	for _, tt := range tests {
		t.Run("role_"+tt.role, func(t *testing.T) {
			_, err := svc.Create(context.Background(), CreateCmd{
				ActorID:     "user_1",
				ActorRole:   tt.role,
				Title:       "Valid Title",
				Description: "This is a valid description long enough.",
				City:        "Singapore",
				Category:    "Tech",
				StartTime:   now.Add(24 * time.Hour),
				EndTime:     now.Add(26 * time.Hour),
				Capacity:    100,
			})
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestService_Update_CacheInvalidation(t *testing.T) {
	now := mustTime(t, "2025-12-25T10:00:00Z")
	repo := newMemRepo()
	cache := newMockCache()
	svc := New(repo, fakeClock{t: now}, cache, 0, 0)

	eventID := "evt_update_1"
	// 设置开始和结束时间在未来，防止触发 "ended event cannot be updated"
	repo.byID[eventID] = &domain.Event{
		ID:        eventID,
		OwnerID:   "owner_1",
		Status:    domain.StatusPublished,
		StartTime: now.Add(1 * time.Hour),
		EndTime:   now.Add(2 * time.Hour),
	}

	cache.store[cacheKeyEventDetails(eventID)] = "old_data"

	t.Run("should_delete_cache_on_update", func(t *testing.T) {
		newTitle := "Updated Event Title"
		_, err := svc.Update(context.Background(), UpdateCmd{
			EventID:   eventID,
			ActorID:   "owner_1",
			ActorRole: "user",
			Title:     &newTitle,
		})
		assert.NoError(t, err)

		_, exists := cache.store[cacheKeyEventDetails(eventID)]
		assert.False(t, exists, "Cache key should be deleted after update")
	})
}

func TestService_GetPublic_CacheFlow(t *testing.T) {
	now := mustTime(t, "2025-12-25T10:00:00Z")
	repo := newMemRepo()
	cache := newMockCache()
	svc := New(repo, fakeClock{t: now}, cache, 0, 0)

	eventID := "evt_flow"
	repo.byID[eventID] = &domain.Event{
		ID:     eventID,
		Status: domain.StatusPublished,
		Title:  "Public Event",
	}

	t.Run("not_found_if_not_published", func(t *testing.T) {
		repo.byID["draft_1"] = &domain.Event{ID: "draft_1", Status: domain.StatusDraft}
		_, err := svc.GetPublic(context.Background(), "draft_1")
		assert.Error(t, err)
	})

	t.Run("db_result_should_be_cached", func(t *testing.T) {
		_, err := svc.GetPublic(context.Background(), eventID)
		assert.NoError(t, err)

		key := cacheKeyEventDetails(eventID)
		assert.NotNil(t, cache.store[key], "Result should be stored in cache")
	})
}
func TestService_ListPublic_CursorGeneration(t *testing.T) {
	now := mustTime(t, "2025-12-25T10:00:00Z")
	repo := newMemRepo()
	New(repo, fakeClock{t: now}, newMockCache(), 0, 0)

	t.Run("should_generate_time_cursor_from_last_item", func(t *testing.T) {
		lastEventTime := now.Add(5 * time.Hour)
		lastEventID := "last_uuid"

		expectedCursor := formatTimeCursor(lastEventTime.UTC(), lastEventID)

		assert.Contains(t, expectedCursor, "Z|last_uuid")
		assert.Contains(t, expectedCursor, "2025-12-25")
	})
}
func TestListFilter_Normalization(t *testing.T) {
	t.Run("apply_default_pagesize", func(t *testing.T) {
		f := ListFilter{PageSize: 0}
		err := f.Normalize()
		assert.NoError(t, err)
		assert.Equal(t, 20, f.PageSize)
	})

	t.Run("cap_max_pagesize", func(t *testing.T) {
		f := ListFilter{PageSize: 500}
		err := f.Normalize()
		assert.NoError(t, err)
		assert.Equal(t, 100, f.PageSize)
	})

	t.Run("validate_time_range", func(t *testing.T) {
		from := time.Now()
		to := from.Add(-1 * time.Hour)
		f := ListFilter{From: &from, To: &to}
		err := f.Normalize()
		assert.Error(t, err, "Should fail if 'to' is before 'from'")
	})
}

func TestService_Cancel_StateValidation(t *testing.T) {
	now := mustTime(t, "2025-12-25T10:00:00Z")
	repo := newMemRepo()
	svc := New(repo, fakeClock{t: now}, newMockCache(), 0, 0)

	eventID := "evt_canceled"
	repo.byID[eventID] = &domain.Event{
		ID:      eventID,
		OwnerID: "owner",
		Status:  domain.StatusCanceled,
	}

	t.Run("cannot_cancel_already_canceled", func(t *testing.T) {
		_, err := svc.Cancel(context.Background(), eventID, "owner", "user", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already canceled")
	})
}

func TestService_Unpublish(t *testing.T) {
	now := mustTime(t, "2025-12-25T10:00:00Z")
	repo := newMemRepo()
	cache := newMockCache()
	svc := New(repo, fakeClock{t: now}, cache, 0, 0)

	eventID := "evt_unpub"
	ownerID := "owner_1"
	repo.byID[eventID] = &domain.Event{
		ID:      eventID,
		OwnerID: ownerID,
		Status:  domain.StatusPublished,
	}
	cache.store[cacheKeyEventDetails(eventID)] = "cached_data"

	// 1. Not Owner
	t.Run("non_owner_cannot_unpublish", func(t *testing.T) {
		repo.byID["evt_other"] = &domain.Event{ID: "evt_other", OwnerID: "other", Status: domain.StatusPublished}
		_, err := svc.Unpublish(context.Background(), "evt_other", "intruder", "user", "")
		assert.Error(t, err)
	})

	// 2. Success
	t.Run("owner_can_unpublish", func(t *testing.T) {
		ev, err := svc.Unpublish(context.Background(), eventID, ownerID, "user", "")
		assert.NoError(t, err)
		assert.Equal(t, domain.StatusDraft, ev.Status)
		_, exists := cache.store[cacheKeyEventDetails(eventID)]
		assert.False(t, exists, "Cache should be invalidated")
	})

	// 3. Already Draft
	t.Run("cannot_unpublish_draft", func(t *testing.T) {
		_, err := svc.Unpublish(context.Background(), eventID, ownerID, "user", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be published")
	})
}
