DROP INDEX IF EXISTS idx_exercise_library_external;
ALTER TABLE exercise_library
  DROP COLUMN external_id,
  DROP COLUMN difficulty,
  DROP COLUMN force,
  DROP COLUMN grips,
  DROP COLUMN secondary_muscles,
  DROP COLUMN steps,
  DROP COLUMN video_urls,
  DROP COLUMN youtube_url,
  DROP COLUMN details;
CREATE UNIQUE INDEX idx_exercise_library_name ON exercise_library (name);
