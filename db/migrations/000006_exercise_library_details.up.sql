-- Expand the exercise library with the richer fields imported from the MuscleWiki
-- catalog (db/seed/exercise_library.json, loaded via cmd/seedexercises).
DROP INDEX IF EXISTS idx_exercise_library_name; -- names repeat across variations now

ALTER TABLE exercise_library
  ADD COLUMN external_id       integer,
  ADD COLUMN difficulty        text  NOT NULL DEFAULT '',
  ADD COLUMN force             text  NOT NULL DEFAULT '',
  ADD COLUMN grips             text  NOT NULL DEFAULT '',
  ADD COLUMN secondary_muscles jsonb NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN steps             jsonb NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN video_urls        jsonb NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN youtube_url       text  NOT NULL DEFAULT '',
  ADD COLUMN details           text  NOT NULL DEFAULT '';

-- Drop the small hand-seeded catalog; the imported one replaces it.
DELETE FROM exercise_library;

ALTER TABLE exercise_library ALTER COLUMN external_id SET NOT NULL;
CREATE UNIQUE INDEX idx_exercise_library_external ON exercise_library (external_id);
