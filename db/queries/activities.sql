-- name: CreateActivity :one
INSERT INTO activities (user_id, kind, title, performed_at, load, planned, notes)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetActivity :one
SELECT * FROM activities WHERE id = $1 AND user_id = $2;

-- name: ListActivities :many
SELECT * FROM activities
WHERE user_id = sqlc.arg('user_id')
  AND (sqlc.narg('kind')::text IS NULL OR kind = sqlc.narg('kind')::text)
ORDER BY performed_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: DeleteActivity :exec
DELETE FROM activities WHERE id = $1 AND user_id = $2;

-- ── Run details ─────────────────────────────────────────────────────────────
-- name: CreateRunDetail :one
INSERT INTO run_details (activity_id, distance_m, duration_s, avg_pace_s_per_km, avg_hr, calories)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetRunDetail :one
SELECT * FROM run_details WHERE activity_id = $1;

-- name: CreateRunSplit :one
INSERT INTO run_splits (activity_id, km, pace_s, hr)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListRunSplits :many
SELECT * FROM run_splits WHERE activity_id = $1 ORDER BY km;

-- ── Lift details ────────────────────────────────────────────────────────────
-- name: CreateExercise :one
INSERT INTO exercises (activity_id, name, note, position)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListExercisesByActivity :many
SELECT * FROM exercises WHERE activity_id = $1 ORDER BY position;

-- name: CreateLiftSet :one
INSERT INTO lift_sets (exercise_id, weight_kg, reps, done, position)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListLiftSetsByExercise :many
SELECT * FROM lift_sets WHERE exercise_id = $1 ORDER BY position;

-- name: LiftSummary :one
SELECT
    COALESCE(COUNT(ls.id), 0)::bigint                       AS total_sets,
    COALESCE(SUM(ls.weight_kg * ls.reps), 0)::bigint        AS total_volume_kg
FROM exercises e
LEFT JOIN lift_sets ls ON ls.exercise_id = e.id
WHERE e.activity_id = $1;

-- ── Training-load rollups ───────────────────────────────────────────────────
-- name: SumLoadInRange :one
SELECT COALESCE(SUM(load), 0)::bigint AS load
FROM activities
WHERE user_id = $1 AND planned = false
  AND performed_at >= sqlc.arg('start') AND performed_at < sqlc.arg('stop');

-- name: LoadByDayInRange :many
SELECT performed_at::date AS day, COALESCE(SUM(load), 0)::bigint AS load
FROM activities
WHERE user_id = $1 AND planned = false
  AND performed_at >= sqlc.arg('start') AND performed_at < sqlc.arg('stop')
GROUP BY day
ORDER BY day;

-- name: LoadByKindInRange :many
SELECT kind, COALESCE(SUM(load), 0)::bigint AS load
FROM activities
WHERE user_id = $1 AND planned = false
  AND performed_at >= sqlc.arg('start') AND performed_at < sqlc.arg('stop')
GROUP BY kind;

-- name: WeeklyLoadTrend :many
SELECT date_trunc('week', performed_at)::date AS week, COALESCE(SUM(load), 0)::bigint AS load
FROM activities
WHERE user_id = $1 AND planned = false AND performed_at >= sqlc.arg('start')
GROUP BY week
ORDER BY week;

-- name: MonthlyDistance :one
SELECT COALESCE(SUM(rd.distance_m), 0)::bigint AS distance_m
FROM activities a
JOIN run_details rd ON rd.activity_id = a.id
WHERE a.user_id = $1 AND a.performed_at >= sqlc.arg('start') AND a.performed_at < sqlc.arg('stop');

-- name: MonthlyVolume :one
SELECT COALESCE(SUM(ls.weight_kg * ls.reps), 0)::bigint AS volume_kg
FROM activities a
JOIN exercises e ON e.activity_id = a.id
JOIN lift_sets ls ON ls.exercise_id = e.id
WHERE a.user_id = $1 AND a.performed_at >= sqlc.arg('start') AND a.performed_at < sqlc.arg('stop');

-- name: CountSessionsInRange :one
SELECT COUNT(*)::bigint AS sessions
FROM activities
WHERE user_id = $1 AND performed_at >= sqlc.arg('start') AND performed_at < sqlc.arg('stop');
