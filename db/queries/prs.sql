-- name: ListPersonalRecords :many
SELECT * FROM personal_records WHERE user_id = $1 ORDER BY position;

-- name: CreatePersonalRecord :one
INSERT INTO personal_records (user_id, label, value, icon, position)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: DeletePersonalRecord :exec
DELETE FROM personal_records WHERE id = $1 AND user_id = $2;
