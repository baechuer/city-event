package eventregistration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/baechuer/cityevents/internal/platform/identity"
)

func TestServiceCreateJoinWaitlistCancelPromote(t *testing.T) {
	svc, repo := testEventService()
	ctx := context.Background()

	event := createTestEvent(t, svc, "organizer-1", 1)
	first, err := svc.JoinEvent(ctx, event.Event.ID, "user-1", "")
	if err != nil {
		t.Fatalf("join first: %v", err)
	}
	if first.Registration.Status != RegistrationStatusConfirmed {
		t.Fatalf("first join status = %s", first.Registration.Status)
	}
	second, err := svc.JoinEvent(ctx, event.Event.ID, "user-2", "")
	if err != nil {
		t.Fatalf("join second: %v", err)
	}
	if second.Registration.Status != RegistrationStatusWaitlisted {
		t.Fatalf("second join status = %s", second.Registration.Status)
	}

	canceled, err := svc.CancelJoin(ctx, event.Event.ID, "user-1")
	if err != nil {
		t.Fatalf("cancel join: %v", err)
	}
	if canceled.Promoted == nil || canceled.Promoted.UserID != "user-2" {
		t.Fatalf("expected user-2 promotion, got %+v", canceled.Promoted)
	}
	if confirmed := repo.ConfirmedCount(event.Event.ID); confirmed != 1 {
		t.Fatalf("confirmed count = %d, want 1", confirmed)
	}
}

func TestServiceDuplicateJoinReturnsExistingRegistration(t *testing.T) {
	svc, repo := testEventService()
	ctx := context.Background()
	event := createTestEvent(t, svc, "organizer-1", 10)

	first, err := svc.JoinEvent(ctx, event.Event.ID, "user-1", "key-1")
	if err != nil {
		t.Fatalf("first join: %v", err)
	}
	second, err := svc.JoinEvent(ctx, event.Event.ID, "user-1", "key-1")
	if err != nil {
		t.Fatalf("second join: %v", err)
	}
	if !second.Existing {
		t.Fatalf("expected duplicate join to return existing result")
	}
	if first.Registration.ID != second.Registration.ID {
		t.Fatalf("duplicate join returned different registration")
	}
	if count := repo.ActiveRegistrationCount(event.Event.ID, "user-1"); count != 1 {
		t.Fatalf("active registration count = %d, want 1", count)
	}
}

func TestServiceRejectsCanceledEventJoin(t *testing.T) {
	svc, _ := testEventService()
	ctx := context.Background()
	event := createTestEvent(t, svc, "organizer-1", 10)
	if _, err := svc.CancelEvent(ctx, event.Event.ID, "organizer-1", identity.RoleOrganizer); err != nil {
		t.Fatalf("cancel event: %v", err)
	}
	if _, err := svc.JoinEvent(ctx, event.Event.ID, "user-1", ""); !errors.Is(err, ErrEventCanceled) {
		t.Fatalf("expected canceled event join to fail, got %v", err)
	}
}

func TestServiceCancelOutboxIncludesPreviousStatus(t *testing.T) {
	svc, repo := testEventService()
	ctx := context.Background()
	event := createTestEvent(t, svc, "organizer-1", 1)
	if _, err := svc.JoinEvent(ctx, event.Event.ID, "user-1", ""); err != nil {
		t.Fatalf("join first: %v", err)
	}
	if _, err := svc.JoinEvent(ctx, event.Event.ID, "user-2", ""); err != nil {
		t.Fatalf("join second: %v", err)
	}
	if _, err := svc.CancelJoin(ctx, event.Event.ID, "user-1"); err != nil {
		t.Fatalf("cancel join: %v", err)
	}

	repo.mu.Lock()
	defer repo.mu.Unlock()
	var sawCancel, sawPromote bool
	for _, msg := range repo.outbox {
		if msg.RoutingKey == RoutingJoinCanceled && msg.Payload["previousStatus"] == RegistrationStatusConfirmed {
			sawCancel = true
		}
		if msg.RoutingKey == RoutingJoinPromoted && msg.Payload["previousStatus"] == RegistrationStatusWaitlisted {
			sawPromote = true
		}
	}
	if !sawCancel {
		t.Fatal("join.canceled outbox did not include previous CONFIRMED status")
	}
	if !sawPromote {
		t.Fatal("join.promoted outbox did not include previous WAITLISTED status")
	}
}

func TestServiceCapacityReductionBelowConfirmedRejected(t *testing.T) {
	svc, _ := testEventService()
	ctx := context.Background()
	event := createTestEvent(t, svc, "organizer-1", 2)
	_, _ = svc.JoinEvent(ctx, event.Event.ID, "user-1", "")
	_, _ = svc.JoinEvent(ctx, event.Event.ID, "user-2", "")

	newCapacity := 1
	if _, err := svc.UpdateEvent(ctx, event.Event.ID, "organizer-1", identity.RoleOrganizer, UpdateEventCommand{Capacity: &newCapacity}); !errors.Is(err, ErrCapacityBelowConfirmed) {
		t.Fatalf("expected capacity reduction to fail, got %v", err)
	}
}

func TestServiceConcurrentJoinCapacityInvariant(t *testing.T) {
	svc, repo := testEventService()
	ctx := context.Background()
	event := createTestEvent(t, svc, "organizer-1", 10)

	var wg sync.WaitGroup
	errs := make(chan error, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		userID := "user-" + NewID()
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
	if confirmed := repo.ConfirmedCount(event.Event.ID); confirmed != 10 {
		t.Fatalf("confirmed count = %d, want 10", confirmed)
	}
	if waitlisted := repo.WaitlistedCount(event.Event.ID); waitlisted != 90 {
		t.Fatalf("waitlisted count = %d, want 90", waitlisted)
	}
}

func TestServiceConcurrentSameUserJoinCreatesOneActiveRegistration(t *testing.T) {
	svc, repo := testEventService()
	ctx := context.Background()
	event := createTestEvent(t, svc, "organizer-1", 10)

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
	if count := repo.ActiveRegistrationCount(event.Event.ID, "same-user"); count != 1 {
		t.Fatalf("active registration count = %d, want 1", count)
	}
}

func testEventService() (*Service, *MemoryRepository) {
	repo := NewMemoryRepository()
	svc := NewService(repo)
	svc.now = func() time.Time {
		return time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	}
	return svc, repo
}

func createTestEvent(t *testing.T, svc *Service, organizerID string, capacity int) EventDetail {
	t.Helper()
	event, err := svc.CreateEvent(context.Background(), CreateEventCommand{
		OrganizerID: organizerID,
		Role:        identity.RoleOrganizer,
		Title:       "Tech Meetup",
		Description: "Monthly meetup",
		City:        "Sydney",
		Venue:       "Town Hall",
		StartsAt:    time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC).Add(24 * time.Hour),
		Capacity:    capacity,
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	return event
}
