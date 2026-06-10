package feedprojection

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/baechuer/cityevents/internal/platform/messaging"
	"github.com/baechuer/cityevents/internal/platform/observability"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const DefaultConsumerName = "feed-projection"

type Projector struct {
	pool         *pgxpool.Pool
	consumerName string
	now          func() time.Time
}

func NewProjector(pool *pgxpool.Pool) *Projector {
	return &Projector{
		pool:         pool,
		consumerName: DefaultConsumerName,
		now:          time.Now,
	}
}

func (p *Projector) HandleEnvelope(ctx context.Context, envelope messaging.Envelope) error {
	if err := envelope.Validate(); err != nil {
		return err
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)

	claimed, err := claimMessage(ctx, tx, p.consumerName, envelope)
	if err != nil {
		return err
	}
	if !claimed {
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		observability.RecordConsumerMessage(p.consumerName, envelope.RoutingKey, "duplicate")
		return nil
	}

	if err := p.apply(ctx, tx, envelope); err != nil {
		observability.RecordConsumerMessage(p.consumerName, envelope.RoutingKey, "failed")
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	observability.RecordConsumerMessage(p.consumerName, envelope.RoutingKey, "processed")
	return nil
}

func (p *Projector) apply(ctx context.Context, tx pgx.Tx, envelope messaging.Envelope) error {
	switch envelope.RoutingKey {
	case "event.published", "event.updated", "event.canceled":
		return upsertEvent(ctx, tx, envelope)
	case "join.confirmed":
		return changeConfirmedCount(ctx, tx, stringField(envelope.Payload, "eventId"), 1)
	case "join.waitlisted":
		return nil
	case "join.canceled":
		if stringField(envelope.Payload, "previousStatus") != "CONFIRMED" {
			return nil
		}
		return changeConfirmedCount(ctx, tx, stringField(envelope.Payload, "eventId"), -1)
	case "join.promoted":
		return changeConfirmedCount(ctx, tx, stringField(envelope.Payload, "eventId"), 1)
	default:
		return fmt.Errorf("unsupported routing key %q", envelope.RoutingKey)
	}
}

func claimMessage(ctx context.Context, tx pgx.Tx, consumerName string, envelope messaging.Envelope) (bool, error) {
	tag, err := tx.Exec(ctx, `
		INSERT INTO processed_messages (consumer_name, message_id, routing_key, processed_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (consumer_name, message_id) DO NOTHING
	`, consumerName, envelope.MessageID, envelope.RoutingKey)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func upsertEvent(ctx context.Context, tx pgx.Tx, envelope messaging.Envelope) error {
	eventID := stringField(envelope.Payload, "eventId")
	if eventID == "" {
		return errors.New("event payload missing eventId")
	}
	startsAt, err := timeField(envelope.Payload, "startsAt")
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO feed_events (event_id, title, city, venue, starts_at, capacity, status, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (event_id) DO UPDATE
		SET title = EXCLUDED.title,
		    city = EXCLUDED.city,
		    venue = EXCLUDED.venue,
		    starts_at = EXCLUDED.starts_at,
		    capacity = EXCLUDED.capacity,
		    status = EXCLUDED.status,
		    updated_at = now()
	`, eventID,
		stringField(envelope.Payload, "title"),
		stringField(envelope.Payload, "city"),
		stringField(envelope.Payload, "venue"),
		startsAt,
		intField(envelope.Payload, "capacity"),
		stringField(envelope.Payload, "status"))
	return err
}

func changeConfirmedCount(ctx context.Context, tx pgx.Tx, eventID string, delta int) error {
	if strings.TrimSpace(eventID) == "" {
		return errors.New("join payload missing eventId")
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO feed_events (event_id, confirmed_count, updated_at)
		VALUES ($1, GREATEST($2, 0), now())
		ON CONFLICT (event_id) DO UPDATE
		SET confirmed_count = GREATEST(feed_events.confirmed_count + $2, 0),
		    updated_at = now()
	`, eventID, delta)
	return err
}

type FeedEvent struct {
	EventID        string
	Title          string
	City           string
	Venue          string
	StartsAt       *time.Time
	Capacity       int
	Status         string
	ConfirmedCount int
	UpdatedAt      time.Time
}

func (p *Projector) GetEvent(ctx context.Context, eventID string) (FeedEvent, error) {
	row := p.pool.QueryRow(ctx, `
		SELECT event_id, title, city, venue, starts_at, capacity, status, confirmed_count, updated_at
		FROM feed_events
		WHERE event_id = $1
	`, eventID)
	var event FeedEvent
	err := row.Scan(&event.EventID, &event.Title, &event.City, &event.Venue, &event.StartsAt, &event.Capacity, &event.Status, &event.ConfirmedCount, &event.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return FeedEvent{}, ErrNotFound
	}
	return event, err
}

func (p *Projector) ProcessedCount(ctx context.Context) (int, error) {
	var count int
	err := p.pool.QueryRow(ctx, `SELECT count(*) FROM processed_messages WHERE consumer_name = $1`, p.consumerName).Scan(&count)
	return count, err
}

var ErrNotFound = errors.New("not found")

func stringField(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func intField(payload map[string]any, key string) int {
	value, ok := payload[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(math.Round(typed))
	case jsonNumber:
		parsed, _ := typed.Int64()
		return int(parsed)
	default:
		var out int
		_, _ = fmt.Sscanf(fmt.Sprint(value), "%d", &out)
		return out
	}
}

type jsonNumber interface {
	Int64() (int64, error)
}

func timeField(payload map[string]any, key string) (*time.Time, error) {
	raw := stringField(payload, key)
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", key, err)
	}
	utc := parsed.UTC()
	return &utc, nil
}

func rollback(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(ctx)
}
