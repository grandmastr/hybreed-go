-- name: SearchFoods :many
SELECT * FROM foods
WHERE name ILIKE '%' || sqlc.arg('query')::text || '%'
ORDER BY
    (name ILIKE sqlc.arg('query')::text || '%') DESC,  -- prefix matches first
    name
LIMIT sqlc.arg('lim');

-- name: GetFoodByBarcode :one
SELECT * FROM foods WHERE barcode = $1;

-- name: CreateFood :one
INSERT INTO foods (name, serving, kcal, protein_g, carbs_g, fat_g, barcode)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: CountFoods :one
SELECT COUNT(*)::bigint AS count FROM foods;
