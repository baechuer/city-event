package notification

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/baechuer/cityevents/internal/platform/messaging"
)

const (
	DefaultConsumerName = "notification-delivery"
	DevEmailDomain      = "cityevents.local"

	StatusPending    = "PENDING"
	StatusProcessing = "PROCESSING"
	StatusSent       = "SENT"
	StatusFailed     = "FAILED"
)

var (
	ErrUnsupportedRoutingKey = errors.New("unsupported notification routing key")
	ErrMissingRecipient      = errors.New("notification recipient is required")
)

type Notification struct {
	ID               string
	MessageID        string
	RecipientUserID  string
	RecipientEmail   string
	RoutingKey       string
	Subject          string
	Body             string
	Status           string
	LastError        string
	IdempotencyKey   string
	DeliveryAttempts int
	NextAttemptAt    time.Time
	LockedUntil      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type Delivery struct {
	ID                string
	NotificationID    string
	Provider          string
	Status            string
	ProviderMessageID string
	Error             string
	CreatedAt         time.Time
}

type Decision struct {
	RecipientUserID string
	RecipientEmail  string
	Subject         string
	Body            string
}

func Decide(envelope messaging.Envelope) (Decision, error) {
	switch envelope.RoutingKey {
	case "join.confirmed":
		return joinDecision(envelope, "You are confirmed", "Your registration for event %s is confirmed.")
	case "join.waitlisted":
		return joinDecision(envelope, "You are waitlisted", "You are on the waitlist for event %s.")
	case "join.promoted":
		return joinDecision(envelope, "You were promoted", "You were promoted from the waitlist for event %s.")
	case "join.canceled":
		return joinDecision(envelope, "Your registration was canceled", "Your registration for event %s was canceled.")
	case "event.canceled":
		userID := stringField(envelope.Payload, "organizerId")
		if userID == "" {
			return Decision{}, ErrMissingRecipient
		}
		eventID := stringField(envelope.Payload, "eventId")
		return Decision{
			RecipientUserID: userID,
			RecipientEmail:  DevEmailForUser(userID),
			Subject:         "Your event was canceled",
			Body:            fmt.Sprintf("Event %s was canceled. Attendee fan-out requires verified recipient lookup in a later phase.", eventID),
		}, nil
	default:
		return Decision{}, ErrUnsupportedRoutingKey
	}
}

func joinDecision(envelope messaging.Envelope, subject, template string) (Decision, error) {
	userID := stringField(envelope.Payload, "userId")
	if userID == "" {
		return Decision{}, ErrMissingRecipient
	}
	eventID := stringField(envelope.Payload, "eventId")
	return Decision{
		RecipientUserID: userID,
		RecipientEmail:  DevEmailForUser(userID),
		Subject:         subject,
		Body:            fmt.Sprintf(template, eventID),
	}, nil
}

func DevEmailForUser(userID string) string {
	local := strings.TrimSpace(strings.ToLower(userID))
	local = strings.ReplaceAll(local, " ", "-")
	if local == "" {
		local = "unknown"
	}
	return local + "@" + DevEmailDomain
}

func stringField(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func NewID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	encoded := hex.EncodeToString(bytes[:])
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}
