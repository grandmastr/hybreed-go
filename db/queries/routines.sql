-- name: ListRoutines :many
SELECT * FROM routines WHERE user_id = $1 ORDER BY position, created_at;

-- name: GetRoutine :one
SELECT * FROM routines WHERE id = $1 AND user_id = $2;

-- name: CreateRoutine :one
INSERT INTO routines (user_id, name, note, position)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateRoutine :one
UPDATE routines SET name = $3, note = $4, updated_at = now()
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: DeleteRoutine :exec
DELETE FROM routines WHERE id = $1 AND user_id = $2;

-- name: ListRoutineExercises :many
SELECT * FROM routine_exercises WHERE routine_id = $1 ORDER BY position;

-- name: CreateRoutineExercise :one
INSERT INTO routine_exercises (routine_id, name, note, target_sets, position)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: DeleteRoutineExercises :exec
DELETE FROM routine_exercises WHERE routine_id = $1;
