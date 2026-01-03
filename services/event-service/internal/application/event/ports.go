package event

import (
	"context"
	"time"

	"github.com/baechuer/real-time-ressys/services/event-service/internal/domain"
	"github.com/google/uuid"
)

type EventRepo interface {
	Create(ctx context.Context, e *domain.Event) error
	GetByID(ctx context.Context, id string) (*domain.Event, error)
	GetByIDs(ctx context.Context, ids []string) ([]*domain.Event, error)
	Update(ctx context.Context, e *domain.Event) error

	ListByOwner(ctx context.Context, ownerID string, status string, page, pageSize int) ([]*domain.Event, int, error)

	// Keyset - time
	ListPublicTimeKeyset(
		ctx context.Context,
		f ListFilter,
		hasCursor bool,
		afterStart time.Time,
		afterID string,
	) ([]*domain.Event, error)

	// Keyset - relevance
	ListPublicRelevanceKeyset(
		ctx context.Context,
		f ListFilter,
		hasCursor bool,
		afterRank float64,
		afterStart time.Time,
		afterID string,
	) ([]*domain.Event, []float64, error)

	// Participant count management
	IncrementParticipantCount(ctx context.Context, eventID uuid.UUID) error
	DecrementParticipantCount(ctx context.Context, eventID uuid.UUID) error

	// City autocomplete suggestions
	GetCitySuggestions(ctx context.Context, query string, limit int) ([]string, error)

	// WithTx runs fn in a DB transaction.
	// The TxEventRepo must be used for all reads/writes inside the callback.
	WithTx(ctx context.Context, fn func(r TxEventRepo) error) error
}

type TxEventRepo interface {
	GetByIDForUpdate(ctx context.Context, id string) (*domain.Event, error)
	Update(ctx context.Context, e *domain.Event) error

	// InsertOutbox persists the message for eventual publish (Outbox pattern).
	InsertOutbox(ctx context.Context, msg OutboxMessage) error
}

// EventPublisher is used by the outbox worker (infrastructure layer).
// It MUST set AMQP MessageId = msg.MessageID and publish msg.Body as-is.
type EventPublisher interface {
	PublishEvent(ctx context.Context, routingKey, messageID string, body []byte) error
}

// OutboxMessage is what we persist in the outbox table.
// Body MUST be a JSON-encoded DomainEventEnvelope.
type OutboxMessage struct {
	MessageID  string
	RoutingKey string
	Body       []byte
	CreatedAt  time.Time
}

// Cache defines the behavior for caching (Redis)
type Cache interface {
	// Get attempts to retrieve a value.
	// Returns (found=true, nil) if hit.
	// Returns (found=false, nil) if miss (not an error).
	// Returns (false, err) if connection error / redis down.
	Get(ctx context.Context, key string, dest any) (bool, error)

	// Set saves a value with TTL.
	Set(ctx context.Context, key string, val any, ttl time.Duration) error

	// Delete removes keys.
	Delete(ctx context.Context, keys ...string) error
}
