-- ── Activities (base record for a run or a lift) ───────────────────────────
CREATE TABLE activities (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind         text        NOT NULL CHECK (kind IN ('run', 'lift')),
    title        text        NOT NULL,
    performed_at timestamptz NOT NULL DEFAULT now(),
    load         integer     NOT NULL DEFAULT 0,           -- training-load score
    planned      boolean     NOT NULL DEFAULT false,
    notes        text        NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_activities_user_time ON activities (user_id, performed_at DESC);
CREATE INDEX idx_activities_user_kind ON activities (user_id, kind);

-- ── Run details (1:1 with a 'run' activity) ────────────────────────────────
CREATE TABLE run_details (
    activity_id      uuid PRIMARY KEY REFERENCES activities(id) ON DELETE CASCADE,
    distance_m       integer NOT NULL DEFAULT 0,           -- metres
    duration_s       integer NOT NULL DEFAULT 0,           -- seconds
    avg_pace_s_per_km integer NOT NULL DEFAULT 0,
    avg_hr           integer NOT NULL DEFAULT 0,
    calories         integer NOT NULL DEFAULT 0
);

CREATE TABLE run_splits (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id uuid    NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    km          integer NOT NULL,
    pace_s      integer NOT NULL,
    hr          integer NOT NULL DEFAULT 0
);
CREATE INDEX idx_run_splits_activity ON run_splits (activity_id, km);

-- ── Lift details (exercises + sets for a 'lift' activity) ───────────────────
CREATE TABLE exercises (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id uuid    NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    name        text    NOT NULL,
    note        text    NOT NULL DEFAULT '',
    position    integer NOT NULL DEFAULT 0
);
CREATE INDEX idx_exercises_activity ON exercises (activity_id, position);

CREATE TABLE lift_sets (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    exercise_id uuid        NOT NULL REFERENCES exercises(id) ON DELETE CASCADE,
    weight_kg   numeric(7,2) NOT NULL DEFAULT 0,
    reps        integer     NOT NULL DEFAULT 0,
    done        boolean     NOT NULL DEFAULT false,
    position    integer     NOT NULL DEFAULT 0
);
CREATE INDEX idx_lift_sets_exercise ON lift_sets (exercise_id, position);

-- ── Today's plan ───────────────────────────────────────────────────────────
CREATE TABLE plan_items (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan_date  date    NOT NULL DEFAULT CURRENT_DATE,
    kind       text    NOT NULL CHECK (kind IN ('run', 'lift', 'meal')),
    title      text    NOT NULL,
    meta       text    NOT NULL DEFAULT '',
    done       boolean NOT NULL DEFAULT false,
    position   integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_plan_items_user_date ON plan_items (user_id, plan_date, position);

-- ── Personal records (Profile › PRs) ───────────────────────────────────────
CREATE TABLE personal_records (
    id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id  uuid    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label    text    NOT NULL,
    value    text    NOT NULL,
    icon     text    NOT NULL DEFAULT 'bolt',
    position integer NOT NULL DEFAULT 0
);
CREATE INDEX idx_personal_records_user ON personal_records (user_id, position);
