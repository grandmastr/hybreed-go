// Command seedfoods loads the food catalogue from a USDA FoodData Central
// export (db/seed/foods.json — SR Legacy + Foundation whole foods, per 100 g)
// into the foods table. It is idempotent (upsert by lower(name)), so it is safe
// to re-run and can target any database. Run after migrating:
//
//	DATABASE_URL=... go run ./cmd/seedfoods
//	DATABASE_URL=... go run ./cmd/seedfoods path/to/foods.json
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/grandmastr/hybreed-go/internal/store"
)

const (
	defaultFile = "db/seed/foods.json"
	batchSize   = 1000
)

// seedFood mirrors a record in db/seed/foods.json.
type seedFood struct {
	Name     string  `json:"name"`
	Serving  string  `json:"serving"`
	Kcal     int32   `json:"kcal"`
	ProteinG float64 `json:"protein_g"`
	CarbsG   float64 `json:"carbs_g"`
	FatG     float64 `json:"fat_g"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seedfoods failed:", err)
		os.Exit(1)
	}
}

func run() error {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL not set")
	}
	file := defaultFile
	if len(os.Args) > 1 {
		file = os.Args[1]
	}

	raw, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read %s: %w", file, err)
	}
	var foods []seedFood
	if err := json.Unmarshal(raw, &foods); err != nil {
		return fmt.Errorf("parse %s: %w", file, err)
	}

	params := make([]store.UpsertFoodParams, 0, len(foods))
	for _, f := range foods {
		if f.Name == "" {
			continue
		}
		params = append(params, store.UpsertFoodParams{
			Name:     f.Name,
			Serving:  f.Serving,
			Kcal:     f.Kcal,
			ProteinG: store.Num(f.ProteinG),
			CarbsG:   store.Num(f.CarbsG),
			FatG:     store.Num(f.FatG),
			Barcode:  nil,
		})
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()
	q := store.New(pool)

	start := time.Now()
	for i := 0; i < len(params); i += batchSize {
		end := i + batchSize
		if end > len(params) {
			end = len(params)
		}
		var firstErr error
		br := q.UpsertFood(ctx, params[i:end])
		br.Exec(func(_ int, err error) {
			if err != nil && firstErr == nil {
				firstErr = err
			}
		})
		if err := br.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		if firstErr != nil {
			return fmt.Errorf("upsert batch %d-%d: %w", i, end, firstErr)
		}
		fmt.Printf("  %d/%d…\n", end, len(params))
	}

	var total int64
	if total, err = q.CountFoods(ctx); err != nil {
		return fmt.Errorf("count: %w", err)
	}
	fmt.Printf("seeded %d foods in %s — foods table now has %d rows\n", len(params), time.Since(start).Round(time.Millisecond), total)
	return nil
}
