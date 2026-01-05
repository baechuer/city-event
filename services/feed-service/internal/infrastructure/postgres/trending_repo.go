package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TrendingRepo handles trending aggregation
type TrendingRepo struct {
	pool *pgxpool.Pool
}

func NewTrendingRepo(pool *pgxpool.Pool) *TrendingRepo {
	return &TrendingRepo{pool: pool}
}

// RunAggregation aggregates user events into event_trend_stats
func (r *TrendingRepo) RunAggregation(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO event_trend_stats (event_id, city, join_users_24h, join_users_7d, view_users_24h, view_users_7d, updated_at)
		SELECT 
			e.event_id,
			e.city,
			COUNT(DISTINCT ue.actor_key) FILTER (WHERE ue.event_type = 'join' AND ue.bucket_date > CURRENT_DATE - 1),
			COUNT(DISTINCT ue.actor_key) FILTER (WHERE ue.event_type = 'join'),
			COUNT(DISTINCT ue.actor_key) FILTER (WHERE ue.event_type = 'view' AND ue.bucket_date > CURRENT_DATE - 1),
			COUNT(DISTINCT ue.actor_key) FILTER (WHERE ue.event_type = 'view'),
			NOW()
		FROM event_index e
		LEFT JOIN user_events ue ON e.event_id = ue.event_id 
			AND ue.bucket_date > CURRENT_DATE - 7
		WHERE e.status = 'published' AND e.start_time > NOW()
		GROUP BY e.event_id, e.city
		ON CONFLICT (event_id) DO UPDATE SET
			join_users_24h = EXCLUDED.join_users_24h,
			join_users_7d = EXCLUDED.join_users_7d,
			view_users_24h = EXCLUDED.view_users_24h,
			view_users_7d = EXCLUDED.view_users_7d,
			updated_at = NOW()
	`)
	return err
}

// GetTrending returns trending events with online score calculation
func (r *TrendingRepo) GetTrending(ctx context.Context, city string, category string, queryStr string, limit int, afterScore float64, afterStartTime time.Time, afterID string) ([]TrendingEvent, error) {
	fmt.Printf("GetTrending: limit=%d cursor=(score=%f, time=%v, id=%s)\n", limit, afterScore, afterStartTime, afterID)
	query := `
		SELECT 
			e.event_id, e.title, e.city, e.tags, e.start_time, e.cover_image_ids,
			(4.0 * COALESCE(ts.join_users_24h, 0) +
			 2.0 * COALESCE(ts.join_users_7d, 0) +
			 0.5 * COALESCE(ts.view_users_24h, 0) +
			 3.0 / (1 + EXTRACT(EPOCH FROM (e.start_time - NOW())) / 86400)
			) AS trend_score
		FROM event_index e
		LEFT JOIN event_trend_stats ts ON e.event_id = ts.event_id
		WHERE e.status = 'published' AND e.start_time > NOW()
	`
	args := []interface{}{}
	argNum := 1

	if city != "" {
		query += fmt.Sprintf(" AND e.city = $%d", argNum)
		args = append(args, city)
		argNum++
	}

	if category != "" {
		query += fmt.Sprintf(" AND $%d = ANY(e.tags)", argNum)
		args = append(args, category)
		argNum++
	}

	if queryStr != "" {
		query += fmt.Sprintf(" AND (e.title ILIKE $%d OR e.city ILIKE $%d)", argNum, argNum)
		args = append(args, "%"+queryStr+"%")
		argNum++
	}

	if afterID != "" {
		query += fmt.Sprintf(` AND (
			(4.0 * COALESCE(ts.join_users_24h, 0) + 2.0 * COALESCE(ts.join_users_7d, 0) + 0.5 * COALESCE(ts.view_users_24h, 0) + 3.0 / (1 + EXTRACT(EPOCH FROM (e.start_time - NOW())) / 86400)) < $%d
			OR (
				(4.0 * COALESCE(ts.join_users_24h, 0) + 2.0 * COALESCE(ts.join_users_7d, 0) + 0.5 * COALESCE(ts.view_users_24h, 0) + 3.0 / (1 + EXTRACT(EPOCH FROM (e.start_time - NOW())) / 86400)) = $%d
				AND (e.start_time > $%d OR (e.start_time = $%d AND e.event_id < $%d))
			)
		)`, argNum, argNum+1, argNum+2, argNum+3, argNum+4)
		args = append(args, afterScore, afterScore, afterStartTime, afterStartTime, afterID)
		argNum += 5
	}

	query += fmt.Sprintf(" ORDER BY trend_score DESC, e.start_time ASC, e.event_id DESC LIMIT $%d", argNum)
	args = append(args, limit)

	fmt.Printf("Query Params: %v\n", args)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []TrendingEvent
	for rows.Next() {
		var e TrendingEvent
		if err := rows.Scan(&e.EventID, &e.Title, &e.City, &e.Tags, &e.StartTime, &e.CoverImageIDs, &e.TrendScore); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

type TrendingEvent struct {
	EventID       string    `json:"id"`
	Title         string    `json:"title"`
	City          string    `json:"city"`
	Tags          []string  `json:"tags"`
	StartTime     time.Time `json:"start_time"`
	TrendScore    float64   `json:"trend_score"`
	CoverImageIDs []string  `json:"cover_image_ids"`
}

// GetLatest returns newest events ordered by creation time (created_at DESC)
func (r *TrendingRepo) GetLatest(ctx context.Context, city string, category string, queryStr string, limit int, afterStartTime time.Time, afterID string) ([]TrendingEvent, error) {
	query := `
		SELECT 
			e.event_id, e.title, e.city, e.tags, e.start_time, e.cover_image_ids,
			0.0 AS trend_score
		FROM event_index e
		WHERE e.status = 'published' AND e.start_time > NOW()
	`
	args := []interface{}{}
	argNum := 1

	if city != "" {
		query += fmt.Sprintf(" AND e.city = $%d", argNum)
		args = append(args, city)
		argNum++
	}

	if category != "" {
		query += fmt.Sprintf(" AND $%d = ANY(e.tags)", argNum)
		args = append(args, category)
		argNum++
	}

	if queryStr != "" {
		query += fmt.Sprintf(" AND (e.title ILIKE $%d OR e.city ILIKE $%d)", argNum, argNum)
		args = append(args, "%"+queryStr+"%")
		argNum++
	}

	// Keyset pagination: created_at DESC, event_id DESC
	if afterID != "" {
		query += fmt.Sprintf(` AND (e.start_time < $%d OR (e.start_time = $%d AND e.event_id < $%d))`, argNum, argNum+1, argNum+2)
		args = append(args, afterStartTime, afterStartTime, afterID)
		argNum += 3
	}

	query += fmt.Sprintf(" ORDER BY e.start_time DESC, e.event_id DESC LIMIT $%d", argNum)
	args = append(args, limit)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []TrendingEvent
	for rows.Next() {
		var e TrendingEvent
		if err := rows.Scan(&e.EventID, &e.Title, &e.City, &e.Tags, &e.StartTime, &e.CoverImageIDs, &e.TrendScore); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
