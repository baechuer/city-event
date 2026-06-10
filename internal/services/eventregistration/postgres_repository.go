package eventregistration

import (
	"context"
	"errors"
	"time"

	"github.com/baechuer/cityevents/internal/platform/identity"
	"github.com/baechuer/cityevents/internal/platform/observability"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateEvent(ctx context.Context, event Event) (EventDetail, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return EventDetail{}, err
	}
	defer rollback(ctx, tx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO events (id, organizer_id, title, description, city, venue, starts_at, capacity, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, event.ID, event.OrganizerID, event.Title, event.Description, event.City, event.Venue, event.StartsAt, event.Capacity, event.Status, event.CreatedAt, event.UpdatedAt); err != nil {
		return EventDetail{}, err
	}
	if err := insertOutbox(ctx, tx, "event", event.ID, RoutingEventPublished, eventPayload(event), event.CreatedAt); err != nil {
		return EventDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return EventDetail{}, err
	}
	return r.GetEventDetail(ctx, event.ID, "")
}

func (r *PostgresRepository) UpdateEvent(ctx context.Context, eventID, organizerID string, role identity.Role, cmd UpdateEventCommand, now time.Time) (EventDetail, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return EventDetail{}, err
	}
	defer rollback(ctx, tx)

	event, err := selectEventForUpdate(ctx, tx, eventID)
	if err != nil {
		return EventDetail{}, err
	}
	if event.OrganizerID != organizerID && !identity.CanAdmin(role) {
		return EventDetail{}, ErrForbidden
	}
	if event.Status == EventStatusCanceled {
		return EventDetail{}, ErrEventCanceled
	}
	confirmed, err := confirmedCountTx(ctx, tx, eventID)
	if err != nil {
		return EventDetail{}, err
	}

	if cmd.Title != nil {
		event.Title = *cmd.Title
	}
	if cmd.Description != nil {
		event.Description = *cmd.Description
	}
	if cmd.City != nil {
		event.City = *cmd.City
	}
	if cmd.Venue != nil {
		event.Venue = *cmd.Venue
	}
	if cmd.StartsAt != nil {
		event.StartsAt = cmd.StartsAt.UTC()
	}
	if cmd.Capacity != nil {
		if err := ValidateCapacityChange(*cmd.Capacity, confirmed); err != nil {
			return EventDetail{}, err
		}
		event.Capacity = *cmd.Capacity
	}
	event.UpdatedAt = now.UTC()
	if err := event.Validate(now); err != nil {
		return EventDetail{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE events
		SET title = $2, description = $3, city = $4, venue = $5, starts_at = $6, capacity = $7, updated_at = $8
		WHERE id = $1
	`, event.ID, event.Title, event.Description, event.City, event.Venue, event.StartsAt, event.Capacity, event.UpdatedAt); err != nil {
		return EventDetail{}, err
	}
	if err := insertOutbox(ctx, tx, "event", event.ID, RoutingEventUpdated, eventPayload(event), now); err != nil {
		return EventDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return EventDetail{}, err
	}
	return r.GetEventDetail(ctx, eventID, "")
}

func (r *PostgresRepository) CancelEvent(ctx context.Context, eventID, organizerID string, role identity.Role, now time.Time) (EventDetail, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return EventDetail{}, err
	}
	defer rollback(ctx, tx)

	event, err := selectEventForUpdate(ctx, tx, eventID)
	if err != nil {
		return EventDetail{}, err
	}
	if event.OrganizerID != organizerID && !identity.CanAdmin(role) {
		return EventDetail{}, ErrForbidden
	}
	if event.Status != EventStatusCanceled {
		event.Status = EventStatusCanceled
		event.UpdatedAt = now.UTC()
		if _, err := tx.Exec(ctx, `
			UPDATE events
			SET status = $2, updated_at = $3
			WHERE id = $1
		`, event.ID, event.Status, event.UpdatedAt); err != nil {
			return EventDetail{}, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE event_registrations
			SET status = 'CANCELED', updated_at = $2
			WHERE event_id = $1 AND status IN ('CONFIRMED', 'WAITLISTED')
		`, event.ID, now.UTC()); err != nil {
			return EventDetail{}, err
		}
		if err := insertOutbox(ctx, tx, "event", event.ID, RoutingEventCanceled, eventPayload(event), now); err != nil {
			return EventDetail{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return EventDetail{}, err
	}
	return r.GetEventDetail(ctx, eventID, "")
}

func (r *PostgresRepository) ListPublishedEvents(ctx context.Context) ([]EventDetail, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT e.id, e.organizer_id, e.title, e.description, e.city, e.venue, e.starts_at, e.capacity, e.status, e.created_at, e.updated_at,
		       COALESCE(c.confirmed_count, 0)
		FROM events e
		LEFT JOIN (
			SELECT event_id, count(*) AS confirmed_count
			FROM event_registrations
			WHERE status = 'CONFIRMED'
			GROUP BY event_id
		) c ON c.event_id = e.id
		WHERE e.status = 'PUBLISHED'
		ORDER BY e.starts_at ASC, e.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var details []EventDetail
	for rows.Next() {
		var detail EventDetail
		if err := scanEventWithCount(rows, &detail); err != nil {
			return nil, err
		}
		detail.ViewerJoinStatus = RegistrationStatusNotJoined
		details = append(details, detail)
	}
	return details, rows.Err()
}

func (r *PostgresRepository) GetEventDetail(ctx context.Context, eventID, viewerID string) (EventDetail, error) {
	var detail EventDetail
	row := r.pool.QueryRow(ctx, `
		SELECT e.id, e.organizer_id, e.title, e.description, e.city, e.venue, e.starts_at, e.capacity, e.status, e.created_at, e.updated_at,
		       COALESCE(c.confirmed_count, 0)
		FROM events e
		LEFT JOIN (
			SELECT event_id, count(*) AS confirmed_count
			FROM event_registrations
			WHERE status = 'CONFIRMED'
			GROUP BY event_id
		) c ON c.event_id = e.id
		WHERE e.id = $1
	`, eventID)
	if err := scanEventWithCount(row, &detail); err != nil {
		return EventDetail{}, err
	}
	status := RegistrationStatusNotJoined
	if viewerID != "" {
		var err error
		status, err = r.GetJoinStatus(ctx, eventID, viewerID)
		if err != nil {
			return EventDetail{}, err
		}
	}
	detail.ViewerJoinStatus = status
	return detail, nil
}

func (r *PostgresRepository) JoinEvent(ctx context.Context, eventID, userID, idempotencyKey string, now time.Time) (JoinResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return JoinResult{}, err
	}
	defer rollback(ctx, tx)

	event, err := selectEventForUpdate(ctx, tx, eventID)
	if err != nil {
		return JoinResult{}, err
	}
	if existing, ok, err := activeRegistrationTx(ctx, tx, eventID, userID); err != nil {
		return JoinResult{}, err
	} else if ok {
		if err := tx.Commit(ctx); err != nil {
			return JoinResult{}, err
		}
		return JoinResult{Registration: existing, Existing: true}, nil
	}
	confirmed, err := confirmedCountTx(ctx, tx, eventID)
	if err != nil {
		return JoinResult{}, err
	}
	status, err := DecideJoinStatus(event, confirmed, now)
	if err != nil {
		return JoinResult{}, err
	}

	reg := Registration{
		ID:             NewID(),
		EventID:        eventID,
		UserID:         userID,
		Status:         status,
		IdempotencyKey: idempotencyKey,
		CreatedAt:      now.UTC(),
		UpdatedAt:      now.UTC(),
	}
	if status == RegistrationStatusWaitlisted {
		reg.WaitlistPosition, err = nextWaitlistPositionTx(ctx, tx, eventID)
		if err != nil {
			return JoinResult{}, err
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO event_registrations (id, event_id, user_id, status, waitlist_position, idempotency_key, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, 0), $6, $7, $8)
	`, reg.ID, reg.EventID, reg.UserID, reg.Status, reg.WaitlistPosition, reg.IdempotencyKey, reg.CreatedAt, reg.UpdatedAt); err != nil {
		return JoinResult{}, err
	}
	routing := RoutingJoinConfirmed
	if status == RegistrationStatusWaitlisted {
		routing = RoutingJoinWaitlisted
	}
	if err := insertOutbox(ctx, tx, "registration", reg.ID, routing, registrationPayload(reg), now); err != nil {
		return JoinResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return JoinResult{}, err
	}
	return JoinResult{Registration: reg}, nil
}

func (r *PostgresRepository) CancelJoin(ctx context.Context, eventID, userID string, now time.Time) (CancelJoinResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CancelJoinResult{}, err
	}
	defer rollback(ctx, tx)

	if _, err := selectEventForUpdate(ctx, tx, eventID); err != nil {
		return CancelJoinResult{}, err
	}
	reg, ok, err := activeRegistrationTx(ctx, tx, eventID, userID)
	if err != nil {
		return CancelJoinResult{}, err
	}
	if !ok {
		status, err := latestStatusTx(ctx, tx, eventID, userID)
		if err != nil {
			return CancelJoinResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return CancelJoinResult{}, err
		}
		return CancelJoinResult{Status: status, AlreadyNoActiveReg: true}, nil
	}

	oldStatus := reg.Status
	reg.Status = RegistrationStatusCanceled
	reg.UpdatedAt = now.UTC()
	reg.WaitlistPosition = 0
	if _, err := tx.Exec(ctx, `
		UPDATE event_registrations
		SET status = 'CANCELED', waitlist_position = NULL, updated_at = $2
		WHERE id = $1
	`, reg.ID, reg.UpdatedAt); err != nil {
		return CancelJoinResult{}, err
	}
	if err := insertOutbox(ctx, tx, "registration", reg.ID, RoutingJoinCanceled, registrationTransitionPayload(reg, oldStatus), now); err != nil {
		return CancelJoinResult{}, err
	}

	result := CancelJoinResult{Status: RegistrationStatusCanceled, Canceled: &reg}
	if oldStatus == RegistrationStatusConfirmed {
		promoted, ok, err := firstWaitlistedTx(ctx, tx, eventID)
		if err != nil {
			return CancelJoinResult{}, err
		}
		if ok {
			promoted.Status = RegistrationStatusConfirmed
			promoted.WaitlistPosition = 0
			promoted.UpdatedAt = now.UTC()
			if _, err := tx.Exec(ctx, `
				UPDATE event_registrations
				SET status = 'CONFIRMED', waitlist_position = NULL, updated_at = $2
				WHERE id = $1
			`, promoted.ID, promoted.UpdatedAt); err != nil {
				return CancelJoinResult{}, err
			}
			if err := insertOutbox(ctx, tx, "registration", promoted.ID, RoutingJoinPromoted, registrationTransitionPayload(promoted, RegistrationStatusWaitlisted), now); err != nil {
				return CancelJoinResult{}, err
			}
			result.Promoted = &promoted
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return CancelJoinResult{}, err
	}
	return result, nil
}

func (r *PostgresRepository) GetJoinStatus(ctx context.Context, eventID, userID string) (RegistrationStatus, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer rollback(ctx, tx)

	if _, err := selectEventTx(ctx, tx, eventID); err != nil {
		return "", err
	}
	status, err := latestStatusTx(ctx, tx, eventID, userID)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return status, nil
}

func (r *PostgresRepository) CountOutboxByRoutingKey(ctx context.Context, routingKey string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM outbox_messages WHERE routing_key = $1`, routingKey).Scan(&count)
	return count, err
}

func (r *PostgresRepository) ConfirmedCount(ctx context.Context, eventID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM event_registrations
		WHERE event_id = $1 AND status = 'CONFIRMED'
	`, eventID).Scan(&count)
	return count, err
}

func (r *PostgresRepository) WaitlistedCount(ctx context.Context, eventID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM event_registrations
		WHERE event_id = $1 AND status = 'WAITLISTED'
	`, eventID).Scan(&count)
	return count, err
}

func (r *PostgresRepository) ActiveRegistrationCount(ctx context.Context, eventID, userID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM event_registrations
		WHERE event_id = $1 AND user_id = $2 AND status IN ('CONFIRMED', 'WAITLISTED')
	`, eventID, userID).Scan(&count)
	return count, err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanEvent(row scanner) (Event, error) {
	var event Event
	err := row.Scan(&event.ID, &event.OrganizerID, &event.Title, &event.Description, &event.City, &event.Venue, &event.StartsAt, &event.Capacity, &event.Status, &event.CreatedAt, &event.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, ErrNotFound
	}
	return event, err
}

func scanEventWithCount(row scanner, detail *EventDetail) error {
	err := row.Scan(&detail.Event.ID, &detail.Event.OrganizerID, &detail.Event.Title, &detail.Event.Description, &detail.Event.City, &detail.Event.Venue, &detail.Event.StartsAt, &detail.Event.Capacity, &detail.Event.Status, &detail.Event.CreatedAt, &detail.Event.UpdatedAt, &detail.ConfirmedCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func scanRegistration(row scanner) (Registration, error) {
	var reg Registration
	err := row.Scan(&reg.ID, &reg.EventID, &reg.UserID, &reg.Status, &reg.WaitlistPosition, &reg.IdempotencyKey, &reg.CreatedAt, &reg.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Registration{}, ErrNotFound
	}
	return reg, err
}

func selectEventTx(ctx context.Context, tx pgx.Tx, eventID string) (Event, error) {
	return scanEvent(tx.QueryRow(ctx, `
		SELECT id, organizer_id, title, description, city, venue, starts_at, capacity, status, created_at, updated_at
		FROM events
		WHERE id = $1
	`, eventID))
}

func selectEventForUpdate(ctx context.Context, tx pgx.Tx, eventID string) (Event, error) {
	return scanEvent(tx.QueryRow(ctx, `
		SELECT id, organizer_id, title, description, city, venue, starts_at, capacity, status, created_at, updated_at
		FROM events
		WHERE id = $1
		FOR UPDATE
	`, eventID))
}

func confirmedCountTx(ctx context.Context, tx pgx.Tx, eventID string) (int, error) {
	var count int
	err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM event_registrations
		WHERE event_id = $1 AND status = 'CONFIRMED'
	`, eventID).Scan(&count)
	return count, err
}

func activeRegistrationTx(ctx context.Context, tx pgx.Tx, eventID, userID string) (Registration, bool, error) {
	reg, err := scanRegistration(tx.QueryRow(ctx, `
		SELECT id, event_id, user_id, status, COALESCE(waitlist_position, 0), idempotency_key, created_at, updated_at
		FROM event_registrations
		WHERE event_id = $1 AND user_id = $2 AND status IN ('CONFIRMED', 'WAITLISTED')
		ORDER BY created_at ASC
		LIMIT 1
	`, eventID, userID))
	if errors.Is(err, ErrNotFound) {
		return Registration{}, false, nil
	}
	return reg, err == nil, err
}

func latestStatusTx(ctx context.Context, tx pgx.Tx, eventID, userID string) (RegistrationStatus, error) {
	var status RegistrationStatus
	err := tx.QueryRow(ctx, `
		SELECT status
		FROM event_registrations
		WHERE event_id = $1 AND user_id = $2
		ORDER BY updated_at DESC, created_at DESC
		LIMIT 1
	`, eventID, userID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return RegistrationStatusNotJoined, nil
	}
	return status, err
}

func nextWaitlistPositionTx(ctx context.Context, tx pgx.Tx, eventID string) (int, error) {
	var next int
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(waitlist_position), 0) + 1
		FROM event_registrations
		WHERE event_id = $1
	`, eventID).Scan(&next)
	return next, err
}

func firstWaitlistedTx(ctx context.Context, tx pgx.Tx, eventID string) (Registration, bool, error) {
	reg, err := scanRegistration(tx.QueryRow(ctx, `
		SELECT id, event_id, user_id, status, COALESCE(waitlist_position, 0), idempotency_key, created_at, updated_at
		FROM event_registrations
		WHERE event_id = $1 AND status = 'WAITLISTED'
		ORDER BY waitlist_position ASC, created_at ASC
		LIMIT 1
		FOR UPDATE
	`, eventID))
	if errors.Is(err, ErrNotFound) {
		return Registration{}, false, nil
	}
	return reg, err == nil, err
}

func insertOutbox(ctx context.Context, tx pgx.Tx, aggregateType, aggregateID, routingKey string, payload map[string]any, now time.Time) error {
	if correlationID := observability.CorrelationIDFromContext(ctx); correlationID != "" {
		if _, ok := payload["correlationId"]; !ok {
			payload["correlationId"] = correlationID
		}
	}
	raw, err := marshalPayload(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_messages (id, aggregate_type, aggregate_id, routing_key, payload, status, created_at, available_at)
		VALUES ($1, $2, $3, $4, $5, 'PENDING', $6, $6)
	`, NewID(), aggregateType, aggregateID, routingKey, raw, now.UTC())
	return err
}

func rollback(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(ctx)
}
