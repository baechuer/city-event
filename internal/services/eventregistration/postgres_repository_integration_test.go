//go:build integration

package eventregistration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/baechuer/cityevents/internal/platform/identity"
	"github.com/baechuer/cityevents/internal/platform/observability"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresCreateUpdateCancelOutbox(t *testing.T) {
	ctx := context.Background()
	pool := setupEventPostgres(t, ctx)
	repo := NewPostgresRepository(pool)
	svc := NewService(repo)

	event := createPostgresEvent(t, svc, "organizer-1", 10)
	assertOutboxCount(t, ctx, repo, RoutingEventPublished, 1)

	title := "Updated Meetup"
	capacity := 12
	updated, err := svc.UpdateEvent(ctx, event.Event.ID, "organizer-1", identity.RoleOrganizer, UpdateEventCommand{Title: &title, Capacity: &capacity})
	if err != nil {
		t.Fatalf("update event: %v", err)
	}
	if updated.Event.Title != title || updated.Event.Capacity != capacity {
		t.Fatalf("unexpected updated event: %+v", updated.Event)
	}
	assertOutboxCount(t, ctx, repo, RoutingEventUpdated, 1)

	if _, err := svc.UpdateEvent(ctx, event.Event.ID, "other-organizer", identity.RoleOrganizer, UpdateEventCommand{Title: &title}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected non-organizer update to fail, got %v", err)
	}

	canceled, err := svc.CancelEvent(ctx, event.Event.ID, "organizer-1", identity.RoleOrganizer)
	if err != nil {
		t.Fatalf("cancel event: %v", err)
	}
	if canceled.Event.Status != EventStatusCanceled {
		t.Fatalf("event status = %s, want canceled", canceled.Event.Status)
	}
	assertOutboxCount(t, ctx, repo, RoutingEventCanceled, 1)

	if _, err := svc.JoinEvent(ctx, event.Event.ID, "user-1", ""); !errors.Is(err, ErrEventCanceled) {
		t.Fatalf("expected canceled event join to fail, got %v", err)
	}
}

func TestPostgresOutboxIncludesCorrelationID(t *testing.T) {
	ctx := observability.ContextWithCorrelationID(context.Background(), "corr-phase-9")
	pool := setupEventPostgres(t, ctx)
	svc := NewService(NewPostgresRepository(pool))

	event, err := svc.CreateEvent(ctx, CreateEventCommand{
		OrganizerID: "organizer-1",
		Role:        identity.RoleOrganizer,
		Title:       "Tech Meetup",
		Description: "Monthly meetup",
		City:        "Sydney",
		Venue:       "Town Hall",
		StartsAt:    time.Now().UTC().Add(24 * time.Hour),
		Capacity:    10,
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	var correlationID string
	if err := pool.QueryRow(context.Background(), `
		SELECT payload->>'correlationId'
		FROM outbox_messages
		WHERE aggregate_id = $1 AND routing_key = $2
	`, event.Event.ID, RoutingEventPublished).Scan(&correlationID); err != nil {
		t.Fatalf("read outbox correlation id: %v", err)
	}
	if correlationID != "corr-phase-9" {
		t.Fatalf("correlation id = %q", correlationID)
	}
}

func TestPostgresJoinWaitlistCancelPromoteOutbox(t *testing.T) {
	ctx := context.Background()
	pool := setupEventPostgres(t, ctx)
	repo := NewPostgresRepository(pool)
	svc := NewService(repo)

	event := createPostgresEvent(t, svc, "organizer-1", 1)
	first, err := svc.JoinEvent(ctx, event.Event.ID, "user-1", "join-1")
	if err != nil {
		t.Fatalf("first join: %v", err)
	}
	if first.Registration.Status != RegistrationStatusConfirmed {
		t.Fatalf("first status = %s", first.Registration.Status)
	}
	second, err := svc.JoinEvent(ctx, event.Event.ID, "user-2", "join-2")
	if err != nil {
		t.Fatalf("second join: %v", err)
	}
	if second.Registration.Status != RegistrationStatusWaitlisted {
		t.Fatalf("second status = %s", second.Registration.Status)
	}
	assertOutboxCount(t, ctx, repo, RoutingJoinConfirmed, 1)
	assertOutboxCount(t, ctx, repo, RoutingJoinWaitlisted, 1)

	duplicate, err := svc.JoinEvent(ctx, event.Event.ID, "user-1", "join-1")
	if err != nil {
		t.Fatalf("duplicate join: %v", err)
	}
	if !duplicate.Existing || duplicate.Registration.ID != first.Registration.ID {
		t.Fatalf("duplicate join did not return existing registration")
	}
	assertOutboxCount(t, ctx, repo, RoutingJoinConfirmed, 1)

	canceled, err := svc.CancelJoin(ctx, event.Event.ID, "user-1")
	if err != nil {
		t.Fatalf("cancel join: %v", err)
	}
	if canceled.Promoted == nil || canceled.Promoted.UserID != "user-2" {
		t.Fatalf("expected user-2 promotion, got %+v", canceled.Promoted)
	}
	assertOutboxCount(t, ctx, repo, RoutingJoinCanceled, 1)
	assertOutboxCount(t, ctx, repo, RoutingJoinPromoted, 1)

	confirmed, err := repo.ConfirmedCount(ctx, event.Event.ID)
	if err != nil {
		t.Fatalf("confirmed count: %v", err)
	}
	if confirmed != 1 {
		t.Fatalf("confirmed count = %d, want 1", confirmed)
	}
}

func TestPostgresOrganizerCancelRegistrationPromotesWaitlist(t *testing.T) {
	ctx := context.Background()
	pool := setupEventPostgres(t, ctx)
	repo := NewPostgresRepository(pool)
	svc := NewService(repo)

	event := createPostgresEvent(t, svc, "organizer-1", 1)
	if _, err := svc.JoinEvent(ctx, event.Event.ID, "user-1", "join-1"); err != nil {
		t.Fatalf("join user-1: %v", err)
	}
	if _, err := svc.JoinEvent(ctx, event.Event.ID, "user-2", "join-2"); err != nil {
		t.Fatalf("join user-2: %v", err)
	}
	if _, err := svc.CancelRegistration(ctx, event.Event.ID, "organizer-2", identity.RoleOrganizer, "user-1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected non-owner organizer moderation to fail, got %v", err)
	}

	result, err := svc.CancelRegistration(ctx, event.Event.ID, "organizer-1", identity.RoleOrganizer, "user-1")
	if err != nil {
		t.Fatalf("organizer cancel registration: %v", err)
	}
	if result.Promoted == nil || result.Promoted.UserID != "user-2" {
		t.Fatalf("expected user-2 promotion, got %+v", result)
	}
	status, err := svc.GetJoinStatus(ctx, event.Event.ID, "user-1")
	if err != nil {
		t.Fatalf("user-1 status: %v", err)
	}
	if status != RegistrationStatusCanceled {
		t.Fatalf("user-1 status = %s, want CANCELED", status)
	}
	status, err = svc.GetJoinStatus(ctx, event.Event.ID, "user-2")
	if err != nil {
		t.Fatalf("user-2 status: %v", err)
	}
	if status != RegistrationStatusConfirmed {
		t.Fatalf("user-2 status = %s, want CONFIRMED", status)
	}
}

func TestPostgresCapacityReductionBelowConfirmedRejected(t *testing.T) {
	ctx := context.Background()
	pool := setupEventPostgres(t, ctx)
	repo := NewPostgresRepository(pool)
	svc := NewService(repo)
	event := createPostgresEvent(t, svc, "organizer-1", 2)

	if _, err := svc.JoinEvent(ctx, event.Event.ID, "user-1", ""); err != nil {
		t.Fatalf("join user-1: %v", err)
	}
	if _, err := svc.JoinEvent(ctx, event.Event.ID, "user-2", ""); err != nil {
		t.Fatalf("join user-2: %v", err)
	}

	newCapacity := 1
	if _, err := svc.UpdateEvent(ctx, event.Event.ID, "organizer-1", identity.RoleOrganizer, UpdateEventCommand{Capacity: &newCapacity}); !errors.Is(err, ErrCapacityBelowConfirmed) {
		t.Fatalf("expected capacity reduction error, got %v", err)
	}
}

func TestPostgresListExcludesCanceledAndDetailIncludesViewerStatus(t *testing.T) {
	ctx := context.Background()
	pool := setupEventPostgres(t, ctx)
	repo := NewPostgresRepository(pool)
	svc := NewService(repo)

	active := createPostgresEvent(t, svc, "organizer-1", 10)
	canceled := createPostgresEvent(t, svc, "organizer-1", 10)
	if _, err := svc.CancelEvent(ctx, canceled.Event.ID, "organizer-1", identity.RoleOrganizer); err != nil {
		t.Fatalf("cancel event: %v", err)
	}
	if _, err := svc.JoinEvent(ctx, active.Event.ID, "viewer-1", ""); err != nil {
		t.Fatalf("join viewer: %v", err)
	}

	list, err := svc.ListPublishedEvents(ctx)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(list) != 1 || list[0].Event.ID != active.Event.ID {
		t.Fatalf("list = %+v, want only active event", list)
	}

	detail, err := svc.GetEventDetail(ctx, active.Event.ID, "viewer-1")
	if err != nil {
		t.Fatalf("event detail: %v", err)
	}
	if detail.ViewerJoinStatus != RegistrationStatusConfirmed {
		t.Fatalf("viewer status = %s, want confirmed", detail.ViewerJoinStatus)
	}
}

func TestPostgresConcurrentJoinCapacityInvariant(t *testing.T) {
	ctx := context.Background()
	pool := setupEventPostgres(t, ctx)
	repo := NewPostgresRepository(pool)
	svc := NewService(repo)
	event := createPostgresEvent(t, svc, "organizer-1", 10)

	var wg sync.WaitGroup
	errs := make(chan error, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		userID := fmt.Sprintf("user-%03d", i)
		go func() {
			defer wg.Done()
			_, err := svc.JoinEvent(ctx, event.Event.ID, userID, "")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("join error: %v", err)
		}
	}

	confirmed, err := repo.ConfirmedCount(ctx, event.Event.ID)
	if err != nil {
		t.Fatalf("confirmed count: %v", err)
	}
	waitlisted, err := repo.WaitlistedCount(ctx, event.Event.ID)
	if err != nil {
		t.Fatalf("waitlisted count: %v", err)
	}
	if confirmed != 10 || waitlisted != 90 {
		t.Fatalf("confirmed=%d waitlisted=%d, want 10 and 90", confirmed, waitlisted)
	}
}

func TestPostgresConcurrentSameUserJoinCreatesOneActiveRegistration(t *testing.T) {
	ctx := context.Background()
	pool := setupEventPostgres(t, ctx)
	repo := NewPostgresRepository(pool)
	svc := NewService(repo)
	event := createPostgresEvent(t, svc, "organizer-1", 10)

	var wg sync.WaitGroup
	errs := make(chan error, 25)
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.JoinEvent(ctx, event.Event.ID, "same-user", "same-key")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("join error: %v", err)
		}
	}

	count, err := repo.ActiveRegistrationCount(ctx, event.Event.ID, "same-user")
	if err != nil {
		t.Fatalf("active count: %v", err)
	}
	if count != 1 {
		t.Fatalf("active registration count = %d, want 1", count)
	}
}

func TestPostgresConcurrentCancelDoesNotOverPromote(t *testing.T) {
	ctx := context.Background()
	pool := setupEventPostgres(t, ctx)
	repo := NewPostgresRepository(pool)
	svc := NewService(repo)
	event := createPostgresEvent(t, svc, "organizer-1", 2)

	for i := 0; i < 5; i++ {
		if _, err := svc.JoinEvent(ctx, event.Event.ID, fmt.Sprintf("user-%d", i), ""); err != nil {
			t.Fatalf("join user-%d: %v", i, err)
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		userID := fmt.Sprintf("user-%d", i)
		go func() {
			defer wg.Done()
			if _, err := svc.CancelJoin(ctx, event.Event.ID, userID); err != nil {
				t.Errorf("cancel %s: %v", userID, err)
			}
		}()
	}
	wg.Wait()

	confirmed, err := repo.ConfirmedCount(ctx, event.Event.ID)
	if err != nil {
		t.Fatalf("confirmed count: %v", err)
	}
	if confirmed != 2 {
		t.Fatalf("confirmed count = %d, want 2 after two promotions", confirmed)
	}
}

func TestPostgresRepeatedCancelIsSafe(t *testing.T) {
	ctx := context.Background()
	pool := setupEventPostgres(t, ctx)
	repo := NewPostgresRepository(pool)
	svc := NewService(repo)
	event := createPostgresEvent(t, svc, "organizer-1", 1)

	if _, err := svc.JoinEvent(ctx, event.Event.ID, "user-1", ""); err != nil {
		t.Fatalf("join: %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, err := svc.CancelJoin(ctx, event.Event.ID, "user-1"); err != nil {
			t.Fatalf("cancel attempt %d: %v", i, err)
		}
	}
	count, err := repo.ActiveRegistrationCount(ctx, event.Event.ID, "user-1")
	if err != nil {
		t.Fatalf("active count: %v", err)
	}
	if count != 0 {
		t.Fatalf("active registration count = %d, want 0", count)
	}
}

func setupEventPostgres(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("EVENT_REGISTRATION_TEST_DATABASE_URL")
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
		DROP TABLE IF EXISTS event_audit_events;
		DROP TABLE IF EXISTS outbox_messages;
		DROP TABLE IF EXISTS event_registrations;
		DROP TABLE IF EXISTS events;
	`); err != nil {
		t.Fatalf("reset event-registration tables: %v", err)
	}

	for _, name := range []string{"001_init.sql", "002_outbox_relay.sql", "003_outbox_dead_state.sql", "004_audit_events.sql"} {
		migrationPath := filepath.Join("..", "..", "..", "migrations", "eventregistration", name)
		migration, err := os.ReadFile(migrationPath)
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(migration)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
	return pool
}

func createPostgresEvent(t *testing.T, svc *Service, organizerID string, capacity int) EventDetail {
	t.Helper()
	event, err := svc.CreateEvent(context.Background(), CreateEventCommand{
		OrganizerID: organizerID,
		Role:        identity.RoleOrganizer,
		Title:       "Tech Meetup",
		Description: "Monthly meetup",
		City:        "Sydney",
		Venue:       "Town Hall",
		StartsAt:    time.Now().UTC().Add(24 * time.Hour),
		Capacity:    capacity,
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	return event
}

func assertOutboxCount(t *testing.T, ctx context.Context, repo *PostgresRepository, routingKey string, want int) {
	t.Helper()
	count, err := repo.CountOutboxByRoutingKey(ctx, routingKey)
	if err != nil {
		t.Fatalf("outbox count %s: %v", routingKey, err)
	}
	if count != want {
		t.Fatalf("outbox count %s = %d, want %d", routingKey, count, want)
	}
}
