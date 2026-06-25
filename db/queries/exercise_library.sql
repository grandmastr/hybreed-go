-- name: ListExerciseLibrary :many
SELECT * FROM exercise_library ORDER BY muscle, name, position, id;

-- name: UpsertExerciseLibrary :exec
INSERT INTO exercise_library (
  external_id, name, muscle, equipment, position,
  difficulty, force, grips, secondary_muscles, steps, video_urls, youtube_url, details
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (external_id) DO UPDATE SET
  name = EXCLUDED.name,
  muscle = EXCLUDED.muscle,
  equipment = EXCLUDED.equipment,
  position = EXCLUDED.position,
  difficulty = EXCLUDED.difficulty,
  force = EXCLUDED.force,
  grips = EXCLUDED.grips,
  secondary_muscles = EXCLUDED.secondary_muscles,
  steps = EXCLUDED.steps,
  video_urls = EXCLUDED.video_urls,
  youtube_url = EXCLUDED.youtube_url,
  details = EXCLUDED.details;
