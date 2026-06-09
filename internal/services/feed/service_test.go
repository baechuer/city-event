package feed

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNormalizeQueryBoundsLimitAndOffset(t *testing.T) {
	query := NormalizeQuery(Query{City: " Sydney ", Limit: 500, Offset: -1})
	if query.City != "Sydney" {
		t.Fatalf("city = %q", query.City)
	}
	if query.Limit != MaxLimit {
		t.Fatalf("limit = %d, want %d", query.Limit, MaxLimit)
	}
	if query.Offset != 0 {
		t.Fatalf("offset = %d, want 0", query.Offset)
	}
}

func TestServiceListUsesCacheAndFallsBackOnCacheError(t *testing.T) {
	start := time.Now().UTC().Add(24 * time.Hour)
	repo := NewMemoryRepository([]Event{{
		EventID:  "event-1",
		Title:    "Tech",
		City:     "Sydney",
		Venue:    "Town Hall",
		StartsAt: &start,
		Status:   StatusPublished,
	}})
	cache := NewMemoryCache()
	service := NewService(repo, cache)

	events, err := service.ListEvents(context.Background(), Query{})
	if err != nil {
		t.Fatalf("first list: %v", err)
	}
	if len(events) != 1 || cache.sets != 1 {
		t.Fatalf("first list events=%d cache sets=%d", len(events), cache.sets)
	}
	events, err = service.ListEvents(context.Background(), Query{})
	if err != nil {
		t.Fatalf("cached list: %v", err)
	}
	if len(events) != 1 || cache.gets < 2 {
		t.Fatalf("cached list events=%d cache gets=%d", len(events), cache.gets)
	}

	cache.err = errors.New("redis unavailable")
	events, err = service.ListEvents(context.Background(), Query{})
	if err != nil {
		t.Fatalf("fallback list: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("fallback events=%d, want 1", len(events))
	}
}

func TestMemoryRepositoryFiltersOrdersAndPaginates(t *testing.T) {
	first := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	second := first.Add(time.Hour)
	repo := NewMemoryRepository([]Event{
		{EventID: "event-2", City: "Sydney", StartsAt: &second, Status: StatusPublished},
		{EventID: "event-1", City: "Sydney", StartsAt: &first, Status: StatusPublished},
		{EventID: "event-3", City: "Melbourne", StartsAt: &first, Status: StatusPublished},
		{EventID: "event-4", City: "Sydney", StartsAt: &first, Status: "CANCELED"},
	})
	events, err := repo.ListEvents(context.Background(), Query{City: "Sydney", Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(events) != 1 || events[0].EventID != "event-2" {
		t.Fatalf("events = %+v, want event-2", events)
	}
}

func TestServiceDetailFallsBackOnCacheError(t *testing.T) {
	repo := NewMemoryRepository([]Event{{EventID: "event-1", Status: StatusPublished}})
	cache := NewMemoryCache()
	cache.err = errors.New("redis unavailable")
	service := NewService(repo, cache)
	event, err := service.GetEvent(context.Background(), "event-1")
	if err != nil {
		t.Fatalf("get event: %v", err)
	}
	if event.EventID != "event-1" {
		t.Fatalf("event id = %s", event.EventID)
	}
}
