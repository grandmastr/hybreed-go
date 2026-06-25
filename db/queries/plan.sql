-- name: ListPlanItems :many
SELECT * FROM plan_items
WHERE user_id = $1 AND plan_date = sqlc.arg('plan_date')
ORDER BY position, created_at;

-- name: CreatePlanItem :one
INSERT INTO plan_items (user_id, plan_date, kind, title, meta, position)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: SetPlanItemDone :one
UPDATE plan_items SET done = $3
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: DeletePlanItem :exec
DELETE FROM plan_items WHERE id = $1 AND user_id = $2;
