package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ProfileRepo handles user tag profile with VIEW swap
type ProfileRepo struct {
	pool *pgxpool.Pool
}

func NewProfileRepo(pool *pgxpool.Pool) *ProfileRepo {
	return &ProfileRepo{pool: pool}
}

// RebuildProfile rebuilds user_tag_profile using VIEW swap pattern
func (r *ProfileRepo) RebuildProfile(ctx context.Context) error {
	// 1. Get current active table
	var activeTable string
	err := r.pool.QueryRow(ctx, `SELECT active_table FROM user_tag_profile_config WHERE id = 1`).Scan(&activeTable)
	if err != nil {
		return err
	}

	// 2. Determine next table
	nextTable := "a"
	if activeTable == "a" {
		nextTable = "b"
	}
	nextTableName := "user_tag_profile_" + nextTable

	// 3. Truncate and rebuild next table
	_, err = r.pool.Exec(ctx, `TRUNCATE `+nextTableName)
	if err != nil {
		return err
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO `+nextTableName+` (actor_key, tag, weight, updated_at)
		SELECT 
			ue.actor_key,
			unnest(e.tags) as tag,
			SUM(CASE WHEN ue.event_type = 'join' THEN 1.0 ELSE 0.3 END) as weight,
			NOW()
		FROM user_events ue
		JOIN event_index e ON ue.event_id = e.event_id
		WHERE ue.bucket_date > CURRENT_DATE - 30
		GROUP BY ue.actor_key, tag
	`)
	if err != nil {
		return err
	}

	// 4. Atomic swap: update view and config in transaction
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `DROP VIEW IF EXISTS user_tag_profile`)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `CREATE VIEW user_tag_profile AS SELECT * FROM `+nextTableName)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `UPDATE user_tag_profile_config SET active_table = $1 WHERE id = 1`, nextTable)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetUserProfile returns tag weights for a user
func (r *ProfileRepo) GetUserProfile(ctx context.Context, actorKey string) (map[string]float64, error) {
	rows, err := r.pool.Query(ctx, `SELECT tag, weight FROM user_tag_profile WHERE actor_key = $1`, actorKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]float64)
	for rows.Next() {
		var tag string
		var weight float64
		if err := rows.Scan(&tag, &weight); err != nil {
			return nil, err
		}
		result[tag] = weight
	}
	return result, rows.Err()
}
