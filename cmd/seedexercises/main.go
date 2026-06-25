// Command seedexercises loads the exercise catalog from a MuscleWiki export
// (db/seed/exercise_library.json) into the exercise_library table. It is
// idempotent (upsert by external id) — safe to re-run. Run once after migrating:
//
//	DATABASE_URL=... go run ./cmd/seedexercises
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/grandmastr/hybreed-go/internal/config"
	"github.com/grandmastr/hybreed-go/internal/store"
)

const defaultFile = "db/seed/exercise_library.json"

// mwExercise mirrors a record in the MuscleWiki export.
type mwExercise struct {
	ID         int      `json:"id"`
	Name       string   `json:"exercise_name"`
	VideoURL   []string `json:"videoURL"`
	Steps      []string `json:"steps"`
	Category   string   `json:"Category"`
	Difficulty string   `json:"Difficulty"`
	Force      string   `json:"Force"`
	Grips      string   `json:"Grips"`
	Target     struct {
		Primary   []string `json:"Primary"`
		Secondary []string `json:"Secondary"`
		Tertiary  []string `json:"Tertiary"`
	} `json:"target"`
	YoutubeURL string `json:"youtubeURL"`
	Details    string `json:"details"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seedexercises failed:", err)
		os.Exit(1)
	}
}

func run() error {
	path := defaultFile
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	var exercises []mwExercise
	if err := json.Unmarshal(raw, &exercises); err != nil {
		return fmt.Errorf("parse exercises: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()
	q := store.New(pool)

	for i, e := range exercises {
		muscle := ""
		if len(e.Target.Primary) > 0 {
			muscle = e.Target.Primary[0]
		}
		secondary := append([]string{}, e.Target.Secondary...)
		secondary = append(secondary, e.Target.Tertiary...)

		if err := q.UpsertExerciseLibrary(ctx, store.UpsertExerciseLibraryParams{
			ExternalID:       int32(e.ID),
			Name:             e.Name,
			Muscle:           muscle,
			Equipment:        e.Category,
			Position:         int32(i),
			Difficulty:       e.Difficulty,
			Force:            e.Force,
			Grips:            e.Grips,
			SecondaryMuscles: jsonArray(secondary),
			Steps:            jsonArray(e.Steps),
			VideoUrls:        jsonArray(e.VideoURL),
			YoutubeUrl:       e.YoutubeURL,
			Details:          e.Details,
		}); err != nil {
			return fmt.Errorf("upsert %q (#%d): %w", e.Name, e.ID, err)
		}
	}

	fmt.Printf("✓ loaded %d exercises into exercise_library\n", len(exercises))
	return nil
}

// jsonArray marshals a string slice to a jsonb array, never null.
func jsonArray(v []string) []byte {
	if len(v) == 0 {
		return []byte("[]")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("[]")
	}
	return b
}
