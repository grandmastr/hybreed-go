ALTER TABLE users DROP COLUMN IF EXISTS dob;
ALTER TABLE user_settings DROP COLUMN IF EXISTS goals;
ALTER TABLE user_settings DROP COLUMN IF EXISTS notification_prefs;
DROP TABLE IF EXISTS push_tokens;
