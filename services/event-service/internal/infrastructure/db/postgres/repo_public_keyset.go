package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
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

	where, args, argN, _ := buildPublicBaseWhere(f)

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

	where, args, argN, qPos := buildPublicBaseWhere(f)

	if qPos == 0 {
		return nil, nil, domain.ErrValidation("q required for relevance sort")
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")

	// cursor predicate for ORDER BY rank DESC, start_time ASC, id ASC
	cursorSQL := ""
	if hasCursor {
		cursorSQL = fmt.Sprintf(`
AND (
  ts_rank_cd(search_vector, to_tsquery('simple', $%d)) < $%d
  OR (
    ts_rank_cd(search_vector, to_tsquery('simple', $%d)) = $%d
    AND (start_time, id) > ($%d, $%d)
  )
)`, qPos, argN, qPos, argN, argN+1, argN+2)
		args = append(args, afterRank, afterStart.UTC(), afterID)
		argN += 3
	}

	q := `
SELECT
  id, owner_id, title, description, city, category,
  start_time, end_time, capacity, active_participants, status,
  published_at, canceled_at, created_at, updated_at,
  ts_rank_cd(search_vector, to_tsquery('simple', $` + fmt.Sprintf("%d", qPos) + `)) AS rank
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
			&e.StartTime, &e.EndTime, &e.Capacity, &e.ActiveParticipants, &status,
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

func buildPublicBaseWhere(f event.ListFilter) ([]string, []any, int, int) {
	where := []string{"status = 'published'"}
	args := []any{}
	argN := 1
	qPos := 0

	add := func(condFmt string, v any) {
		where = append(where, fmt.Sprintf(condFmt, argN))
		args = append(args, v)
		argN++
	}

	// Conditional: Hide expired events
	if f.ExcludeExpired {
		where = append(where, "end_time > NOW()")
	}

	city := strings.TrimSpace(f.City)
	category := strings.TrimSpace(f.Category)

	if city != "" {
		// Clean city term for ILIKE and append %
		cleanCity := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ' {
				return r
			}
			return -1
		}, city)
		if cleanCity != "" {
			add("city ILIKE $%d", cleanCity+"%")
		}
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

	// FTS match with prefix support
	if f.Query != "" {
		q := fmtTsQuery(f.Query)
		if q != "" {
			qPos = argN
			add("search_vector @@ to_tsquery('simple', $%d)", q)
		}
	}

	return where, args, argN, qPos
}

func scanEvents(rows *sql.Rows) ([]*domain.Event, error) {
	var out []*domain.Event
	for rows.Next() {
		var e domain.Event
		var status string
		if err := rows.Scan(
			&e.ID, &e.OwnerID, &e.Title, &e.Description, &e.City, &e.Category,
			&e.StartTime, &e.EndTime, &e.Capacity, &e.ActiveParticipants, &status,
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
func formatRelevanceCursor(rank float64, t time.Time, id string) string {
	// keep cursor stable (8dp)
	return strconv.FormatFloat(rank, 'f', 8, 64) + "|" + t.Format(time.RFC3339Nano) + "|" + id
}

func fmtTsQuery(q string) string {
	// 1. Remove special FTS characters to prevent syntax errors
	// (Postgres to_tsquery is strict)
	q = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ' {
			return r
		}
		return -1
	}, q)

	// 2. Tokenize and append :* for prefix matching
	words := strings.Fields(q)
	if len(words) == 0 {
		return ""
	}

	for i, w := range words {
		words[i] = w + ":*"
	}

	return strings.Join(words, " & ")
}
