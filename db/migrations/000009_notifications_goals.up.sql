-- Remote push device tokens (Expo push tokens) — one row per device, re-owned
-- on conflict so a device that switches accounts re-registers cleanly.
CREATE TABLE push_tokens (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token      text        NOT NULL UNIQUE,
    platform   text        NOT NULL DEFAULT 'ios',   -- ios | android
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_push_tokens_user ON push_tokens (user_id);

-- Per-category notification preferences (the Settings › Notifications toggles)
-- and weekly goals (the Goals & targets sliders) as flexible jsonb.
ALTER TABLE user_settings
    ADD COLUMN notification_prefs jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN goals              jsonb NOT NULL DEFAULT '{}'::jsonb;

-- Date of birth (Profile — personalises HR zones / recovery targets).
ALTER TABLE users ADD COLUMN dob date;
