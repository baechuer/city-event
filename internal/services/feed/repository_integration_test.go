//go:build integration

package feed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func TestPostgresFeedRepositoryListAndDetail(t *testing.T) {
	ctx := context.Background()
	pool := setupFeedPostgres(t, ctx)
	repo := NewPostgresRepository(pool)
	insertFeedEvent(t, ctx, pool, "event-2", "Sydney", time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC), StatusPublished, 2)
	insertFeedEvent(t, ctx, pool, "event-1", "Sydney", time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC), StatusPublished, 1)
	insertFeedEvent(t, ctx, pool, "event-3", "Melbourne", time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC), StatusPublished, 3)
	insertFeedEvent(t, ctx, pool, "event-4", "Sydney", time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC), "CANCELED", 4)

	events, err := repo.ListEvents(ctx, Query{City: "Sydney", Limit: 10})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 2 || events[0].EventID != "event-1" || events[1].EventID != "event-2" {
		t.Fatalf("events = %+v", events)
	}
	detail, err := repo.GetEvent(ctx, "event-1")
	if err != nil {
		t.Fatalf("get event: %v", err)
	}
	if detail.ConfirmedCount != 1 {
		t.Fatalf("confirmed count = %d, want 1", detail.ConfirmedCount)
	}
	if _, err := repo.GetEvent(ctx, "event-4"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("canceled detail error = %v, want not found", err)
	}
}

func TestFeedServiceRedisCacheAndFallback(t *testing.T) {
	ctx := context.Background()
	pool := setupFeedPostgres(t, ctx)
	repo := NewPostgresRepository(pool)
	insertFeedEvent(t, ctx, pool, "event-1", "Sydney", time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC), StatusPublished, 1)

	redisClient := setupFeedRedis(t, ctx)
	service := NewService(repo, NewRedisCache(redisClient))
	query := Query{City: "Sydney", Limit: 10}
	events, err := service.ListEvents(ctx, query)
	if err != nil {
		t.Fatalf("first list: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}

	if _, err := pool.Exec(ctx, `DELETE FROM feed_events`); err != nil {
		t.Fatalf("clear feed events: %v", err)
	}
	events, err = service.ListEvents(ctx, query)
	if err != nil {
		t.Fatalf("cached list: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("cached events = %d, want 1", len(events))
	}

	if err := redisClient.Set(ctx, listCacheKey(Query{Limit: DefaultLimit}), "{bad-json", time.Minute).Err(); err != nil {
		t.Fatalf("write bad cache: %v", err)
	}
	insertFeedEvent(t, ctx, pool, "event-2", "Sydney", time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC), StatusPublished, 2)
	events, err = service.ListEvents(ctx, Query{})
	if err != nil {
		t.Fatalf("bad cache fallback list: %v", err)
	}
	if len(events) != 1 || events[0].EventID != "event-2" {
		t.Fatalf("bad cache fallback events = %+v", events)
	}

	unavailable := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = unavailable.Close() })
	service = NewService(repo, NewRedisCache(unavailable))
	if _, err := service.GetEvent(ctx, "event-2"); err != nil {
		t.Fatalf("detail fallback with redis unavailable: %v", err)
	}
}

func TestPostgresFeedRepositoryHundredEventBoundedRead(t *testing.T) {
	ctx := context.Background()
	pool := setupFeedPostgres(t, ctx)
	repo := NewPostgresRepository(pool)
	start := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 100; i++ {
		insertFeedEvent(t, ctx, pool, fmt.Sprintf("event-%03d", i), "Sydney", start.Add(time.Duration(i)*time.Minute), StatusPublished, i)
	}
	events, err := repo.ListEvents(ctx, Query{City: "Sydney", Limit: 500})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != MaxLimit {
		t.Fatalf("events = %d, want bounded %d", len(events), MaxLimit)
	}
}

func setupFeedPostgres(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("PHASE5_TEST_DATABASE_URL")
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
	if _, err := pool.Exec(ctx, `SELECT pg_advisory_lock(424242)`); err != nil {
		t.Fatalf("take integration schema lock: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `SELECT pg_advisory_unlock(424242)`)
	})
	if _, err := pool.Exec(ctx, `
		DROP TABLE IF EXISTS processed_messages;
		DROP TABLE IF EXISTS feed_events;
	`); err != nil {
		t.Fatalf("reset feed tables: %v", err)
	}
	path := filepath.Join("..", "..", "..", "migrations", "feed", "001_init.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(raw)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	return pool
}

func setupFeedRedis(t *testing.T, ctx context.Context) *redis.Client {
	t.Helper()
	redisURL := os.Getenv("PHASE5_TEST_REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}
	client, err := NewRedisClient(redisURL)
	if err != nil {
		t.Fatalf("new redis client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping redis: %v", err)
	}
	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flush redis: %v", err)
	}
	return client
}

func insertFeedEvent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID, city string, startsAt time.Time, status string, confirmed int) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO feed_events (event_id, title, city, venue, starts_at, capacity, status, confirmed_count, updated_at)
		VALUES ($1, $2, $3, $4, $5, 10, $6, $7, now())
	`, eventID, "Event "+eventID, city, "Town Hall", startsAt, status, confirmed); err != nil {
		t.Fatalf("insert feed event: %v", err)
	}
}
