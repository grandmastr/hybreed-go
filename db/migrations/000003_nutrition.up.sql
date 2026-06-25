-- ── Per-day nutrition budget + water + macro targets ───────────────────────
CREATE TABLE nutrition_days (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    day                 date NOT NULL DEFAULT CURRENT_DATE,
    base_kcal           integer NOT NULL DEFAULT 2030,
    training_bonus_kcal integer NOT NULL DEFAULT 0,
    water_ml            integer NOT NULL DEFAULT 0,
    water_target_ml     integer NOT NULL DEFAULT 2600,
    protein_target_g    integer NOT NULL DEFAULT 165,
    carbs_target_g      integer NOT NULL DEFAULT 250,
    fat_target_g        integer NOT NULL DEFAULT 78,
    UNIQUE (user_id, day)
);

-- ── Meals (Breakfast / Lunch / Dinner / Snacks / custom) ───────────────────
CREATE TABLE meals (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    day        date NOT NULL DEFAULT CURRENT_DATE,
    slot       text NOT NULL,
    logged_at  timestamptz,
    planned    boolean NOT NULL DEFAULT false,
    position   integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_meals_user_day ON meals (user_id, day, position);

CREATE TABLE meal_items (
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    meal_id   uuid NOT NULL REFERENCES meals(id) ON DELETE CASCADE,
    name      text NOT NULL,
    kcal      integer NOT NULL DEFAULT 0,
    protein_g numeric(7,2) NOT NULL DEFAULT 0,
    carbs_g   numeric(7,2) NOT NULL DEFAULT 0,
    fat_g     numeric(7,2) NOT NULL DEFAULT 0,
    position  integer NOT NULL DEFAULT 0
);
CREATE INDEX idx_meal_items_meal ON meal_items (meal_id, position);

-- ── Searchable food database (search + barcode lookup) ─────────────────────
CREATE TABLE foods (
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name      text NOT NULL,
    serving   text NOT NULL DEFAULT '',
    kcal      integer NOT NULL DEFAULT 0,
    protein_g numeric(7,2) NOT NULL DEFAULT 0,
    carbs_g   numeric(7,2) NOT NULL DEFAULT 0,
    fat_g     numeric(7,2) NOT NULL DEFAULT 0,
    barcode   text UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_foods_name_trgm ON foods USING gin (name gin_trgm_ops);
