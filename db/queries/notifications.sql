-- name: UpsertPushToken :exec
-- Register a device's push token to a user; re-owns the device on conflict.
INSERT INTO push_tokens (user_id, token, platform)
VALUES ($1, $2, $3)
ON CONFLICT (token) DO UPDATE SET
    user_id    = EXCLUDED.user_id,
    platform   = EXCLUDED.platform,
    updated_at = now();

-- name: DeletePushToken :exec
DELETE FROM push_tokens WHERE token = $1 AND user_id = $2;

-- name: DeletePushTokenByValue :exec
-- Used by the sender to drop tokens Expo reports as DeviceNotRegistered.
DELETE FROM push_tokens WHERE token = $1;

-- name: ListUserPushTokens :many
SELECT * FROM push_tokens WHERE user_id = $1 ORDER BY created_at;

-- name: GetNotificationPrefs :one
SELECT notification_prefs FROM user_settings WHERE user_id = $1;

-- name: UpdateNotificationPrefs :one
UPDATE user_settings SET notification_prefs = $2, updated_at = now()
WHERE user_id = $1
RETURNING notification_prefs;
