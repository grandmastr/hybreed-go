-- Extensions
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- ── Users ──────────────────────────────────────────────────────────────────
CREATE TABLE users (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name           text        NOT NULL,
    email          citext      NOT NULL UNIQUE,
    password_hash  text,                                   -- null for social-only accounts
    email_verified boolean     NOT NULL DEFAULT false,
    handle         text        NOT NULL DEFAULT 'Hybrid Athlete',
    status         text        NOT NULL DEFAULT 'Productive',
    streak         integer     NOT NULL DEFAULT 0,
    load_target    integer     NOT NULL DEFAULT 820,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);

-- ── Per-user settings (Profile › Settings) ─────────────────────────────────
CREATE TABLE user_settings (
    user_id        uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    units          text        NOT NULL DEFAULT 'metric',  -- metric | imperial
    notifications  boolean     NOT NULL DEFAULT true,
    connected_apps integer     NOT NULL DEFAULT 0,
    body_weight_kg numeric(6,2),
    updated_at     timestamptz NOT NULL DEFAULT now()
);

-- ── One-time passcodes (email verification / OTP login) ────────────────────
CREATE TABLE otp_codes (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash  text        NOT NULL,                        -- sha256(code)
    purpose    text        NOT NULL DEFAULT 'verify_email', -- verify_email | login
    attempts   integer     NOT NULL DEFAULT 0,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_otp_codes_user_purpose ON otp_codes (user_id, purpose) WHERE consumed_at IS NULL;

-- ── Refresh-token sessions (opaque tokens; we store only the hash) ─────────
CREATE TABLE refresh_sessions (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash text        NOT NULL UNIQUE,                 -- sha256(refresh token)
    user_agent text        NOT NULL DEFAULT '',
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_refresh_sessions_user ON refresh_sessions (user_id) WHERE revoked_at IS NULL;
