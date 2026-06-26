-- name: CreateUser :one
INSERT INTO users (name, email, password_hash, email_verified)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: SetEmailVerified :exec
UPDATE users SET email_verified = true, updated_at = now() WHERE id = $1;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1;

-- name: UpdateUserProfile :one
UPDATE users
SET name        = COALESCE(sqlc.narg('name'), name),
    handle      = COALESCE(sqlc.narg('handle'), handle),
    status      = COALESCE(sqlc.narg('status'), status),
    load_target = COALESCE(sqlc.narg('load_target'), load_target),
    dob         = COALESCE(sqlc.narg('dob'), dob),
    updated_at  = now()
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: GetUserSettings :one
SELECT * FROM user_settings WHERE user_id = $1;

-- name: CreateUserSettings :one
INSERT INTO user_settings (user_id) VALUES ($1)
ON CONFLICT (user_id) DO UPDATE SET user_id = EXCLUDED.user_id
RETURNING *;

-- name: UpdateUserSettings :one
UPDATE user_settings
SET units          = COALESCE(sqlc.narg('units'), units),
    notifications  = COALESCE(sqlc.narg('notifications'), notifications),
    connected_apps = COALESCE(sqlc.narg('connected_apps'), connected_apps),
    body_weight_kg = COALESCE(sqlc.narg('body_weight_kg'), body_weight_kg),
    goals          = COALESCE(sqlc.narg('goals'), goals),
    updated_at     = now()
WHERE user_id = sqlc.arg('user_id')
RETURNING *;
