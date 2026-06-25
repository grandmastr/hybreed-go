-- name: CreateOTP :one
INSERT INTO otp_codes (user_id, code_hash, purpose, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetActiveOTP :one
SELECT * FROM otp_codes
WHERE user_id = $1 AND purpose = $2 AND consumed_at IS NULL
ORDER BY created_at DESC
LIMIT 1;

-- name: IncrementOTPAttempts :exec
UPDATE otp_codes SET attempts = attempts + 1 WHERE id = $1;

-- name: ConsumeOTP :exec
UPDATE otp_codes SET consumed_at = now() WHERE id = $1;

-- name: InvalidateUserOTPs :exec
UPDATE otp_codes SET consumed_at = now()
WHERE user_id = $1 AND purpose = $2 AND consumed_at IS NULL;

-- name: CreateRefreshSession :one
INSERT INTO refresh_sessions (user_id, token_hash, user_agent, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetRefreshSession :one
SELECT * FROM refresh_sessions
WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now();

-- name: RevokeRefreshSession :exec
UPDATE refresh_sessions SET revoked_at = now() WHERE token_hash = $1;

-- name: RevokeAllUserSessions :exec
UPDATE refresh_sessions SET revoked_at = now()
WHERE user_id = $1 AND revoked_at IS NULL;
