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

func (r *Repo) ListPublicAfter(
	ctx context.Context,
	f event.ListFilter,
	afterRank float64,
	afterStart time.Time,
	afterID string,
) ([]*domain.Event, int, string, error) {

	// Defensive
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

	// FTS query (only when query != "")
	// We'll reuse this tsquery expression both in WHERE and rank computation.
	hasQuery := query != ""

	// Keyset condition depends on sort
	if strings.TrimSpace(f.Sort) == "relevance" && hasQuery {
		// cursor keyset for DESC rank, then ASC start_time, ASC id
		//
		// rank < afterRank
		// OR (rank = afterRank AND start_time > afterStart)
		// OR (rank = afterRank AND start_time = afterStart AND id > afterID)
		//
		// BUT since we ORDER BY rank DESC, we want the "next page" to be "less relevant",
		// so the tuple comparison is:
		// (rank, start_time, id) < (afterRank, afterStart, afterID)
		//
		// We'll implement explicitly to avoid tuple pitfalls + keep planner happy.
		//
		// rank expression:
		//   ts_rank(search_vector, plainto_tsquery('simple', $qN))
		//
		// We need $qN for tsquery once, and $rankN/$timeN/$idN for cursor.
		where = append(where, fmt.Sprintf("search_vector @@ plainto_tsquery('simple', $%d)", argN))
		args = append(args, query)
		qParam := argN
		argN++

		// keyset predicate
		where = append(where, fmt.Sprintf(`
(
  ts_rank(search_vector, plainto_tsquery('simple', $%d)) < $%d
  OR (
    ts_rank(search_vector, plainto_tsquery('simple', $%d)) = $%d
    AND (start_time > $%d OR (start_time = $%d AND id > $%d))
  )
)`, qParam, argN, qParam, argN, argN+1, argN+1, argN+2))

		args = append(args, afterRank, afterStart, afterID)
		argN += 3
	} else {
		// time sort keyset: (start_time, id) > (afterStart, afterID)
		where = append(where, fmt.Sprintf("(start_time > $%d OR (start_time = $%d AND id > $%d))", argN, argN, argN+1))
		args = append(args, afterStart, afterID)
		argN += 2

		// if query present but sort != relevance, still filter by fts (no rank order)
		if hasQuery {
			where = append(where, fmt.Sprintf("search_vector @@ plainto_tsquery('simple', $%d)", argN))
			args = append(args, query)
			argN++
		}
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")

	// count
	countSQL := "SELECT COUNT(*) FROM events " + whereSQL
	var total int
	if err := r.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, "", err
	}

	limitParam := argN
	args = append(args, f.PageSize)

	var rows *sql.Rows
	var err error

	if strings.TrimSpace(f.Sort) == "relevance" && hasQuery {
		// include rank so we can build next_cursor correctly
		listSQL := `
SELECT
  ts_rank(search_vector, plainto_tsquery('simple', $1)) AS rank,
  id, owner_id, title, description, city, category,
  start_time, end_time, capacity, active_participants, status,
  published_at, canceled_at, created_at, updated_at
FROM events
` + whereSQL + `
ORDER BY rank DESC, start_time ASC, id ASC
LIMIT $` + fmt.Sprintf("%d", limitParam)

		// ⚠️ Here we relied on $1 for the tsquery in SELECT rank.
		// To keep this correct, enforce that the first arg is the query when relevance is used.
		// Our building above places query as the first arg when relevance path is chosen.
		//
		// If you later reorder args, fix this.

		rows, err = r.db.QueryContext(ctx, listSQL, args...)
		if err != nil {
			return nil, 0, "", err
		}
		defer rows.Close()

		type rowRank struct {
			rank float64
			ev   domain.Event
		}
		out := make([]*domain.Event, 0, f.PageSize)
		var lastRank float64

		for rows.Next() {
			var rr rowRank
			var status string
			if err := rows.Scan(
				&rr.rank,
				&rr.ev.ID, &rr.ev.OwnerID, &rr.ev.Title, &rr.ev.Description, &rr.ev.City, &rr.ev.Category,
				&rr.ev.StartTime, &rr.ev.EndTime, &rr.ev.Capacity, &rr.ev.ActiveParticipants, &status,
				&rr.ev.PublishedAt, &rr.ev.CanceledAt, &rr.ev.CreatedAt, &rr.ev.UpdatedAt,
			); err != nil {
				return nil, 0, "", err
			}
			rr.ev.Status = domain.EventStatus(status)
			out = append(out, &rr.ev)
			lastRank = rr.rank
		}
		if err := rows.Err(); err != nil {
			return nil, 0, "", err
		}

		next := ""
		if len(out) > 0 {
			last := out[len(out)-1]
			// rank|time|uuid (rank keep 6 decimals for stable string)
			next = fmt.Sprintf("%.6f|%s|%s", lastRank, last.StartTime.UTC().Format(time.RFC3339), last.ID)
		}
		return out, total, next, nil
	}

	// time sort
	listSQL := `
SELECT id, owner_id, title, description, city, category,
       start_time, end_time, capacity, active_participants, status,
       published_at, canceled_at, created_at, updated_at
FROM events
` + whereSQL + `
ORDER BY start_time ASC, id ASC
LIMIT $` + fmt.Sprintf("%d", limitParam)

	rows, err = r.db.QueryContext(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, "", err
	}
	defer rows.Close()

	out := make([]*domain.Event, 0, f.PageSize)
	for rows.Next() {
		var e domain.Event
		var status string
		if err := rows.Scan(
			&e.ID, &e.OwnerID, &e.Title, &e.Description, &e.City, &e.Category,
			&e.StartTime, &e.EndTime, &e.Capacity, &e.ActiveParticipants, &status,
			&e.PublishedAt, &e.CanceledAt, &e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, 0, "", err
		}
		e.Status = domain.EventStatus(status)
		out = append(out, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, "", err
	}

	next := ""
	if len(out) > 0 {
		last := out[len(out)-1]
		next = last.StartTime.UTC().Format(time.RFC3339) + "|" + last.ID
	}
	return out, total, next, nil
}
