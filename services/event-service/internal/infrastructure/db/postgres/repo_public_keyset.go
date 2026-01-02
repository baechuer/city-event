package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/baechuer/real-time-ressys/services/event-service/internal/application/event"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/domain"
)

func (r *Repo) ListPublicTimeKeyset(
	ctx context.Context,
	f event.ListFilter,
	hasCursor bool,
	afterStart time.Time,
	afterID string,
) ([]*domain.Event, error) {

	where, args, argN := buildPublicBaseWhere(f)

	if hasCursor {
		where = append(where, fmt.Sprintf("(start_time, id) > ($%d, $%d)", argN, argN+1))
		args = append(args, afterStart.UTC(), afterID)
		argN += 2
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")

	q := `
SELECT id, owner_id, title, description, city, category,
       start_time, end_time, capacity, active_participants, status,
       published_at, canceled_at, created_at, updated_at
FROM events
` + whereSQL + `
ORDER BY start_time ASC, id ASC
LIMIT $` + fmt.Sprintf("%d", argN)

	args = append(args, f.PageSize)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEvents(rows)
}

func (r *Repo) ListPublicRelevanceKeyset(
	ctx context.Context,
	f event.ListFilter,
	hasCursor bool,
	afterRank float64,
	afterStart time.Time,
	afterID string,
) ([]*domain.Event, []float64, error) {

	where, args, argN := buildPublicBaseWhere(f)

	// FTS match
	qPos := argN
	where = append(where, fmt.Sprintf("search_vector @@ plainto_tsquery('simple', $%d)", argN))
	args = append(args, f.Query)
	argN++

	whereSQL := "WHERE " + strings.Join(where, " AND ")

	// cursor predicate for ORDER BY rank DESC, start_time ASC, id ASC
	cursorSQL := ""
	if hasCursor {
		cursorSQL = fmt.Sprintf(`
AND (
  ts_rank_cd(search_vector, plainto_tsquery('simple', $%d)) < $%d
  OR (
    ts_rank_cd(search_vector, plainto_tsquery('simple', $%d)) = $%d
    AND (start_time, id) > ($%d, $%d)
  )
)`, qPos, argN, qPos, argN, argN+1, argN+2)
		args = append(args, afterRank, afterStart.UTC(), afterID)
		argN += 3
	}

	q := `
SELECT
  id, owner_id, title, description, city, category,
  start_time, end_time, capacity, status,
  published_at, canceled_at, created_at, updated_at,
  ts_rank_cd(search_vector, plainto_tsquery('simple', $` + fmt.Sprintf("%d", qPos) + `)) AS rank
FROM events
` + whereSQL + cursorSQL + `
ORDER BY rank DESC, start_time ASC, id ASC
LIMIT $` + fmt.Sprintf("%d", argN)

	args = append(args, f.PageSize)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var items []*domain.Event
	var ranks []float64

	for rows.Next() {
		var e domain.Event
		var status string
		var rank float64
		if err := rows.Scan(
			&e.ID, &e.OwnerID, &e.Title, &e.Description, &e.City, &e.Category,
			&e.StartTime, &e.EndTime, &e.Capacity, &status,
			&e.PublishedAt, &e.CanceledAt, &e.CreatedAt, &e.UpdatedAt,
			&rank,
		); err != nil {
			return nil, nil, err
		}
		e.Status = domain.EventStatus(status)
		items = append(items, &e)
		ranks = append(ranks, rank)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	return items, ranks, nil
}

func buildPublicBaseWhere(f event.ListFilter) ([]string, []any, int) {
	where := []string{"status = 'published'"}
	args := []any{}
	argN := 1

	add := func(condFmt string, v any) {
		where = append(where, fmt.Sprintf(condFmt, argN))
		args = append(args, v)
		argN++
	}

	city := strings.TrimSpace(f.City)
	category := strings.TrimSpace(f.Category)

	if city != "" {
		add("city = $%d", city)
	}
	if category != "" {
		add("category = $%d", category)
	}
	if f.From != nil {
		add("start_time >= $%d", f.From.UTC())
	}
	if f.To != nil {
		add("start_time <= $%d", f.To.UTC())
	}

	return where, args, argN
}

func scanEvents(rows *sql.Rows) ([]*domain.Event, error) {
	var out []*domain.Event
	for rows.Next() {
		var e domain.Event
		var status string
		if err := rows.Scan(
			&e.ID, &e.OwnerID, &e.Title, &e.Description, &e.City, &e.Category,
			&e.StartTime, &e.EndTime, &e.Capacity, &status,
			&e.PublishedAt, &e.CanceledAt, &e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, err
		}
		e.Status = domain.EventStatus(status)
		out = append(out, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
