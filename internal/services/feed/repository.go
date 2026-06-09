package feed

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	ListEvents(context.Context, Query) ([]Event, error)
	GetEvent(context.Context, string) (Event, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) ListEvents(ctx context.Context, query Query) ([]Event, error) {
	query = NormalizeQuery(query)
	rows, err := r.pool.Query(ctx, `
		SELECT event_id, title, city, venue, starts_at, capacity, status, confirmed_count, updated_at
		FROM feed_events
		WHERE status = 'PUBLISHED' AND ($1 = '' OR city = $1)
		ORDER BY starts_at ASC NULLS LAST, event_id ASC
		LIMIT $2 OFFSET $3
	`, query.City, query.Limit, query.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]Event, 0)
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (r *PostgresRepository) GetEvent(ctx context.Context, eventID string) (Event, error) {
	event, err := scanEvent(r.pool.QueryRow(ctx, `
		SELECT event_id, title, city, venue, starts_at, capacity, status, confirmed_count, updated_at
		FROM feed_events
		WHERE event_id = $1 AND status = 'PUBLISHED'
	`, strings.TrimSpace(eventID)))
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, ErrNotFound
	}
	return event, err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanEvent(row scanner) (Event, error) {
	var event Event
	err := row.Scan(&event.EventID, &event.Title, &event.City, &event.Venue, &event.StartsAt, &event.Capacity, &event.Status, &event.ConfirmedCount, &event.UpdatedAt)
	return event, err
}

type MemoryRepository struct {
	events map[string]Event
}

func NewMemoryRepository(events []Event) *MemoryRepository {
	repo := &MemoryRepository{events: map[string]Event{}}
	for _, event := range events {
		repo.events[event.EventID] = event
	}
	return repo
}

func (r *MemoryRepository) ListEvents(_ context.Context, query Query) ([]Event, error) {
	query = NormalizeQuery(query)
	events := make([]Event, 0)
	for _, event := range r.events {
		if event.Status != StatusPublished {
			continue
		}
		if query.City != "" && event.City != query.City {
			continue
		}
		events = append(events, event)
	}
	sort.Slice(events, func(i, j int) bool {
		left, right := events[i], events[j]
		switch {
		case left.StartsAt == nil && right.StartsAt != nil:
			return false
		case left.StartsAt != nil && right.StartsAt == nil:
			return true
		case left.StartsAt != nil && right.StartsAt != nil && !left.StartsAt.Equal(*right.StartsAt):
			return left.StartsAt.Before(*right.StartsAt)
		default:
			return left.EventID < right.EventID
		}
	})
	if query.Offset >= len(events) {
		return []Event{}, nil
	}
	end := query.Offset + query.Limit
	if end > len(events) {
		end = len(events)
	}
	return events[query.Offset:end], nil
}

func (r *MemoryRepository) GetEvent(_ context.Context, eventID string) (Event, error) {
	event, ok := r.events[strings.TrimSpace(eventID)]
	if !ok || event.Status != StatusPublished {
		return Event{}, ErrNotFound
	}
	return event, nil
}
