package postgres

const insertEventSQL = `
INSERT INTO events (
  id, owner_id, title, description, city, city_norm, category,
  start_time, end_time, capacity, status,
  published_at, canceled_at, created_at, updated_at, cover_image_ids
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
`

const getEventSQL = `
SELECT id, owner_id, title, description, city, category,
       start_time, end_time, capacity, active_participants, status,
       published_at, canceled_at, created_at, updated_at, cover_image_ids
FROM events WHERE id = $1
`

const updateEventSQL = `
UPDATE events SET
  title=$2, description=$3, city=$4, city_norm=$5, category=$6,
  start_time=$7, end_time=$8, capacity=$9, status=$10,
  published_at=$11, canceled_at=$12, updated_at=$13, cover_image_ids=$14
WHERE id=$1
`

const getCitySuggestionsSQL = `
SELECT city
FROM (
  SELECT city, city_norm, COUNT(*) as cnt
  FROM events
  WHERE status = 'published'
    AND city_norm LIKE $1 || '%'
    AND start_time >= NOW() - INTERVAL '180 days'
  GROUP BY city, city_norm
) t
ORDER BY cnt DESC, city ASC
LIMIT $2
`
