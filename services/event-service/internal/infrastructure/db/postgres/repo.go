package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/baechuer/real-time-ressys/services/event-service/internal/application/event"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/domain"
)

type Repo struct {
	db *sql.DB
}

func New(db *sql.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Create(ctx context.Context, e *domain.Event) error {
	coverIDsJSON, _ := json.Marshal(e.CoverImageIDs)
	_, err := r.db.ExecContext(ctx, insertEventSQL,
		e.ID, e.OwnerID, e.Title, e.Description, e.City, domain.NormalizeCity(e.City), e.Category,
		e.StartTime, e.EndTime, e.Capacity, string(e.Status),
		e.PublishedAt, e.CanceledAt, e.CreatedAt, e.UpdatedAt, string(coverIDsJSON),
	)
	return err
}

func (r *Repo) GetByID(ctx context.Context, id string) (*domain.Event, error) {
	row := r.db.QueryRowContext(ctx, getEventSQL, id)

	var e domain.Event
	var status string
	var coverIDsJSON string
	err := row.Scan(
		&e.ID, &e.OwnerID, &e.Title, &e.Description, &e.City, &e.Category,
		&e.StartTime, &e.EndTime, &e.Capacity, &e.ActiveParticipants, &status,
		&e.PublishedAt, &e.CanceledAt, &e.CreatedAt, &e.UpdatedAt, &coverIDsJSON,
	)
	if err == sql.ErrNoRows {
		return nil, domain.ErrNotFound("event not found")
	}
	if err != nil {
		return nil, err
	}
	e.Status = domain.EventStatus(status)
	_ = json.Unmarshal([]byte(coverIDsJSON), &e.CoverImageIDs)
	if !e.Status.Valid() {
		return nil, domain.ErrInvalidState("invalid status in db")
	}
	return &e, nil
}

func (r *Repo) Update(ctx context.Context, e *domain.Event) error {
	coverIDsJSON, _ := json.Marshal(e.CoverImageIDs)
	_, err := r.db.ExecContext(ctx, updateEventSQL,
		e.ID,
		e.Title, e.Description, e.City, domain.NormalizeCity(e.City), e.Category,
		e.StartTime, e.EndTime, e.Capacity, string(e.Status),
		e.PublishedAt, e.CanceledAt, e.UpdatedAt, string(coverIDsJSON),
	)
	return err
}

// GetByIDs returns multiple events by their IDs (only published events).
// Used for batch lookups to avoid N+1 queries.
func (r *Repo) GetByIDs(ctx context.Context, ids []string) ([]*domain.Event, error) {
	if len(ids) == 0 {
		return []*domain.Event{}, nil
	}
	if len(ids) > 50 {
		ids = ids[:50] // Limit to prevent abuse
	}

	// Build placeholders: $1, $2, $3, ...
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := `
SELECT id, owner_id, title, description, city, category,
       start_time, end_time, capacity, active_participants, status,
       published_at, canceled_at, created_at, updated_at, cover_image_ids
FROM events
WHERE id IN (` + strings.Join(placeholders, ", ") + `) AND status = 'published'`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.Event
	for rows.Next() {
		var e domain.Event
		var status string
		var coverIDsJSON string
		if err := rows.Scan(
			&e.ID, &e.OwnerID, &e.Title, &e.Description, &e.City, &e.Category,
			&e.StartTime, &e.EndTime, &e.Capacity, &e.ActiveParticipants, &status,
			&e.PublishedAt, &e.CanceledAt, &e.CreatedAt, &e.UpdatedAt, &coverIDsJSON,
		); err != nil {
			return nil, err
		}
		e.Status = domain.EventStatus(status)
		_ = json.Unmarshal([]byte(coverIDsJSON), &e.CoverImageIDs)
		out = append(out, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *Repo) ListByOwner(ctx context.Context, ownerID string, status string, page, pageSize int) ([]*domain.Event, int, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize

	where := []string{"owner_id=$1"}
	args := []any{ownerID}
	argN := 2

	if status != "" {
		where = append(where, fmt.Sprintf("status=$%d", argN))
		args = append(args, status)
		argN++
	} else {
		where = append(where, "status != 'canceled'")
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")

	countSQL := "SELECT COUNT(*) FROM events " + whereSQL
	var total int
	if err := r.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listSQL := `
SELECT id, owner_id, title, description, city, category,
       start_time, end_time, capacity, active_participants, status,
       published_at, canceled_at, created_at, updated_at, cover_image_ids
FROM events
` + whereSQL + `
ORDER BY created_at DESC
LIMIT $` + fmt.Sprintf("%d", argN) + ` OFFSET $` + fmt.Sprintf("%d", argN+1)

	listArgs := append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []*domain.Event
	for rows.Next() {
		var e domain.Event
		var s string
		var coverIDsJSON string
		if err := rows.Scan(
			&e.ID, &e.OwnerID, &e.Title, &e.Description, &e.City, &e.Category,
			&e.StartTime, &e.EndTime, &e.Capacity, &e.ActiveParticipants, &s,
			&e.PublishedAt, &e.CanceledAt, &e.CreatedAt, &e.UpdatedAt, &coverIDsJSON,
		); err != nil {
			return nil, 0, err
		}
		e.Status = domain.EventStatus(s)
		_ = json.Unmarshal([]byte(coverIDsJSON), &e.CoverImageIDs)
		out = append(out, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

// ListPublic is now "legacy compatibility": it returns ONLY the first page (OFFSET=0).
// Your real endpoint should be using keyset pagination via ListPublicTimeKeyset / ListPublicRelevanceKeyset.
func (r *Repo) ListPublic(ctx context.Context, f event.ListFilter) ([]*domain.Event, int, error) {
	if f.PageSize <= 0 {
		f.PageSize = 20
	}
	if f.PageSize > 100 {
		f.PageSize = 100
	}

	city := strings.TrimSpace(f.City)
	category := strings.TrimSpace(f.Category)
	query := strings.TrimSpace(f.Query)

	where := []string{"status = 'published'"}
	args := []any{}
	argN := 1

	add := func(condFmt string, val any) {
		where = append(where, fmt.Sprintf(condFmt, argN))
		args = append(args, val)
		argN++
	}

	if city != "" {
		add("city = $%d", city)
	}
	if category != "" {
		add("category = $%d", category)
	}
	if f.From != nil {
		add("start_time >= $%d", *f.From)
	}
	if f.To != nil {
		add("start_time <= $%d", *f.To)
	}

	// FTS
	if query != "" {
		where = append(where, fmt.Sprintf("search_vector @@ plainto_tsquery('simple', $%d)", argN))
		args = append(args, query)
		argN++
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")

	// count (optional)
	countSQL := "SELECT COUNT(*) FROM events " + whereSQL
	var total int
	if err := r.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// deterministic order
	orderBy := "start_time ASC, id ASC"

	offset := 0

	listSQL := `
SELECT id, owner_id, title, description, city, category,
       start_time, end_time, capacity, active_participants, status,
       published_at, canceled_at, created_at, updated_at, cover_image_ids
FROM events
` + whereSQL + `
ORDER BY ` + orderBy + `
LIMIT $` + fmt.Sprintf("%d", argN) + ` OFFSET $` + fmt.Sprintf("%d", argN+1)

	args = append(args, f.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []*domain.Event
	for rows.Next() {
		var e domain.Event
		var status string
		var coverIDsJSON string
		if err := rows.Scan(
			&e.ID, &e.OwnerID, &e.Title, &e.Description, &e.City, &e.Category,
			&e.StartTime, &e.EndTime, &e.Capacity, &e.ActiveParticipants, &status,
			&e.PublishedAt, &e.CanceledAt, &e.CreatedAt, &e.UpdatedAt, &coverIDsJSON,
		); err != nil {
			return nil, 0, err
		}
		e.Status = domain.EventStatus(status)
		_ = json.Unmarshal([]byte(coverIDsJSON), &e.CoverImageIDs)
		out = append(out, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

// GetCitySuggestions returns city suggestions matching the query, sorted by popularity.
func (r *Repo) GetCitySuggestions(ctx context.Context, query string, limit int) ([]string, error) {
	normalizedQuery := domain.NormalizeCity(query)

	rows, err := r.db.QueryContext(ctx, getCitySuggestionsSQL, normalizedQuery, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cities []string
	for rows.Next() {
		var city string
		if err := rows.Scan(&city); err != nil {
			return nil, err
		}
		cities = append(cities, city)
	}

	return cities, rows.Err()
}
