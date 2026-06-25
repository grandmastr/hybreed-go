-- name: GetNutritionDay :one
SELECT * FROM nutrition_days WHERE user_id = $1 AND day = sqlc.arg('day');

-- name: UpsertNutritionDay :one
INSERT INTO nutrition_days (user_id, day)
VALUES ($1, sqlc.arg('day'))
ON CONFLICT (user_id, day) DO UPDATE SET day = EXCLUDED.day
RETURNING *;

-- name: AddWater :one
INSERT INTO nutrition_days (user_id, day, water_ml)
VALUES ($1, sqlc.arg('day'), sqlc.arg('delta_ml'))
ON CONFLICT (user_id, day)
DO UPDATE SET water_ml = GREATEST(0, nutrition_days.water_ml + EXCLUDED.water_ml)
RETURNING *;

-- name: DayConsumed :one
SELECT
    COALESCE(SUM(mi.kcal), 0)::bigint     AS kcal,
    COALESCE(SUM(mi.protein_g), 0)::float8 AS protein_g,
    COALESCE(SUM(mi.carbs_g), 0)::float8   AS carbs_g,
    COALESCE(SUM(mi.fat_g), 0)::float8     AS fat_g
FROM meals m
JOIN meal_items mi ON mi.meal_id = m.id
WHERE m.user_id = $1 AND m.day = sqlc.arg('day');

-- name: ListMealsForDay :many
SELECT * FROM meals
WHERE user_id = $1 AND day = sqlc.arg('day')
ORDER BY position, created_at;

-- name: CreateMeal :one
INSERT INTO meals (user_id, day, slot, logged_at, planned, position)
VALUES ($1, sqlc.arg('day'), $2, $3, $4, $5)
RETURNING *;

-- name: DeleteMeal :exec
DELETE FROM meals WHERE id = $1 AND user_id = $2;

-- name: ListMealItemsForDay :many
SELECT mi.* FROM meal_items mi
JOIN meals m ON m.id = mi.meal_id
WHERE m.user_id = $1 AND m.day = sqlc.arg('day')
ORDER BY mi.meal_id, mi.position;

-- name: ListMealItems :many
SELECT * FROM meal_items WHERE meal_id = $1 ORDER BY position;

-- name: CreateMealItem :one
INSERT INTO meal_items (meal_id, name, kcal, protein_g, carbs_g, fat_g, position)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: MealOwnedBy :one
SELECT EXISTS (SELECT 1 FROM meals WHERE id = $1 AND user_id = $2) AS owned;

-- name: SumCaloriesInRange :one
SELECT COALESCE(SUM(mi.kcal), 0)::bigint AS kcal
FROM meals m
JOIN meal_items mi ON mi.meal_id = m.id
WHERE m.user_id = $1 AND m.day >= sqlc.arg('start') AND m.day < sqlc.arg('stop');
