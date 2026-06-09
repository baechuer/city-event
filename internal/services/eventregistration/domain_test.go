package eventregistration

import (
	"errors"
	"testing"
	"time"
)

func TestNewEventValidation(t *testing.T) {
	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	_, err := NewEvent("organizer-1", "Tech Meetup", "desc", "Sydney", "Town Hall", now.Add(time.Hour), 10, now)
	if err != nil {
		t.Fatalf("expected valid event: %v", err)
	}

	cases := []struct {
		name string
		mut  func(*Event)
		want error
	}{
		{name: "missing title", mut: func(e *Event) { e.Title = "" }, want: ErrInvalidEvent},
		{name: "missing city", mut: func(e *Event) { e.City = "" }, want: ErrInvalidEvent},
		{name: "missing venue", mut: func(e *Event) { e.Venue = "" }, want: ErrInvalidEvent},
		{name: "past start", mut: func(e *Event) { e.StartsAt = now.Add(-time.Second) }, want: ErrEventStarted},
		{name: "non-positive capacity", mut: func(e *Event) { e.Capacity = 0 }, want: ErrInvalidCapacity},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event, _ := NewEvent("organizer-1", "Tech Meetup", "desc", "Sydney", "Town Hall", now.Add(time.Hour), 10, now)
			tc.mut(&event)
			if err := event.Validate(now); !errors.Is(err, tc.want) {
				t.Fatalf("Validate() = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestValidateCapacityChange(t *testing.T) {
	if err := ValidateCapacityChange(5, 5); err != nil {
		t.Fatalf("expected equal capacity to confirmed count to be valid: %v", err)
	}
	if err := ValidateCapacityChange(4, 5); !errors.Is(err, ErrCapacityBelowConfirmed) {
		t.Fatalf("expected capacity below confirmed to fail, got %v", err)
	}
	if err := ValidateCapacityChange(0, 0); !errors.Is(err, ErrInvalidCapacity) {
		t.Fatalf("expected invalid capacity, got %v", err)
	}
}

func TestDecideJoinStatus(t *testing.T) {
	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	event, _ := NewEvent("organizer-1", "Tech Meetup", "desc", "Sydney", "Town Hall", now.Add(time.Hour), 2, now)

	status, err := DecideJoinStatus(event, 1, now)
	if err != nil || status != RegistrationStatusConfirmed {
		t.Fatalf("status = %s err=%v, want confirmed", status, err)
	}
	status, err = DecideJoinStatus(event, 2, now)
	if err != nil || status != RegistrationStatusWaitlisted {
		t.Fatalf("status = %s err=%v, want waitlisted", status, err)
	}
	event.Status = EventStatusCanceled
	if _, err := DecideJoinStatus(event, 0, now); !errors.Is(err, ErrEventCanceled) {
		t.Fatalf("expected canceled event to reject join, got %v", err)
	}
}
