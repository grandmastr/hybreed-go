-- name: ListExerciseLibrary :many
SELECT * FROM exercise_library ORDER BY muscle, position, name;
