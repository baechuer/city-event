package notification

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/baechuer/cityevents/internal/platform/messaging"
)

func TestDecideJoinNotifications(t *testing.T) {
	tests := []struct {
		routingKey string
		subject    string
	}{
		{"join.confirmed", "confirmed"},
		{"join.waitlisted", "waitlisted"},
		{"join.promoted", "promoted"},
		{"join.canceled", "canceled"},
	}
	for _, tc := range tests {
		t.Run(tc.routingKey, func(t *testing.T) {
			envelope := testNotificationEnvelope(t, "msg-1", tc.routingKey, map[string]any{
				"userId":  "user-1",
				"eventId": "event-1",
			})
			decision, err := Decide(envelope)
			if err != nil {
				t.Fatalf("decide: %v", err)
			}
			if decision.RecipientUserID != "user-1" || decision.RecipientEmail != "user-1@cityevents.local" {
				t.Fatalf("unexpected recipient: %+v", decision)
			}
			if !strings.Contains(strings.ToLower(decision.Subject), tc.subject) {
				t.Fatalf("subject = %q, want contains %q", decision.Subject, tc.subject)
			}
		})
	}
}

func TestDecideEventCanceledUsesOrganizerAndDocumentsFanoutLimit(t *testing.T) {
	envelope := testNotificationEnvelope(t, "msg-1", "event.canceled", map[string]any{
		"organizerId": "organizer-1",
		"eventId":     "event-1",
	})
	decision, err := Decide(envelope)
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if decision.RecipientUserID != "organizer-1" {
		t.Fatalf("recipient = %s", decision.RecipientUserID)
	}
	if !strings.Contains(decision.Body, "requires verified recipient lookup") {
		t.Fatalf("body should document fan-out limitation: %s", decision.Body)
	}
}

func TestDecideRejectsUnsupportedAndMissingRecipient(t *testing.T) {
	unsupported := testNotificationEnvelope(t, "msg-1", "event.published", map[string]any{"eventId": "event-1"})
	if _, err := Decide(unsupported); !errors.Is(err, ErrUnsupportedRoutingKey) {
		t.Fatalf("unsupported error = %v", err)
	}

	missing := testNotificationEnvelope(t, "msg-2", "join.confirmed", map[string]any{"eventId": "event-1"})
	if _, err := Decide(missing); !errors.Is(err, ErrMissingRecipient) {
		t.Fatalf("missing recipient error = %v", err)
	}
}

func TestDevEmailForUser(t *testing.T) {
	if got := DevEmailForUser(" User One "); got != "user-one@cityevents.local" {
		t.Fatalf("email = %s", got)
	}
}

func testNotificationEnvelope(t *testing.T, messageID, routingKey string, payload map[string]any) messaging.Envelope {
	t.Helper()
	envelope, err := messaging.NewEnvelope(messageID, routingKey, "registration", "aggregate-1", "corr-1", time.Now().UTC(), payload)
	if err != nil {
		t.Fatalf("new envelope: %v", err)
	}
	return envelope
}
