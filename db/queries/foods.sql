-- name: SearchFoods :many
-- Word-order-independent: every whitespace-separated token in the query must
-- appear somewhere in the name, so "greek yogurt" matches "Yogurt, Greek, plain".
-- Shorter names rank higher (less qualified = more likely what the user meant).
-- The caller must reject an empty query (ILIKE ALL over an empty set is TRUE).
SELECT * FROM foods
WHERE name ILIKE ALL (
        SELECT '%' || w || '%'
        FROM unnest(string_to_array(lower(btrim(sqlc.arg('query')::text)), ' ')) AS w
        WHERE w <> ''
    )
ORDER BY
    (name ILIKE sqlc.arg('query')::text || '%') DESC,  -- whole-query prefix first
    length(name),
    name
LIMIT sqlc.arg('lim');

-- name: GetFoodByBarcode :one
SELECT * FROM foods WHERE barcode = $1;

-- name: CreateFood :one
INSERT INTO foods (name, serving, kcal, protein_g, carbs_g, fat_g, barcode)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpsertFood :batchexec
-- Idempotent catalogue insert keyed on lower(name) (see idx_foods_lower_name).
INSERT INTO foods (name, serving, kcal, protein_g, carbs_g, fat_g, barcode)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (lower(name)) DO UPDATE SET
    serving   = EXCLUDED.serving,
    kcal      = EXCLUDED.kcal,
    protein_g = EXCLUDED.protein_g,
    carbs_g   = EXCLUDED.carbs_g,
    fat_g     = EXCLUDED.fat_g,
    barcode   = COALESCE(EXCLUDED.barcode, foods.barcode);

-- name: CountFoods :one
SELECT COUNT(*)::bigint AS count FROM foods;
