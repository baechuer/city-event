package postgres

const insertEventSQL = `
INSERT INTO events (
  id, owner_id, title, description, city, category,
  start_time, end_time, capacity, status,
  published_at, canceled_at, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
`

const getEventSQL = `
SELECT id, owner_id, title, description, city, category,
       start_time, end_time, capacity, active_participants, status,
       published_at, canceled_at, created_at, updated_at
FROM events WHERE id = $1
`

const updateEventSQL = `
UPDATE events SET
  title=$2, description=$3, city=$4, category=$5,
  start_time=$6, end_time=$7, capacity=$8, status=$9,
  published_at=$10, canceled_at=$11, updated_at=$12
WHERE id=$1
`
