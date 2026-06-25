-- Reusable workout routines (Hevy-style): a named, ordered list of exercises a
-- user can start a workout from.
CREATE TABLE routines (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       text        NOT NULL,
    note       text        NOT NULL DEFAULT '',
    position   integer     NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_routines_user ON routines (user_id, position, created_at);

CREATE TABLE routine_exercises (
    id          uuid    PRIMARY KEY DEFAULT gen_random_uuid(),
    routine_id  uuid    NOT NULL REFERENCES routines(id) ON DELETE CASCADE,
    name        text    NOT NULL,
    note        text    NOT NULL DEFAULT '',
    target_sets integer NOT NULL DEFAULT 3,
    position    integer NOT NULL DEFAULT 0
);
CREATE INDEX idx_routine_exercises_routine ON routine_exercises (routine_id, position);
