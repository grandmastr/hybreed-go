// Command seed populates the database with the demo dataset that mirrors the
// Hybreed app's mock data (data/hybreed.ts): the food database plus a fully
// fleshed-out demo athlete. It is idempotent — safe to run repeatedly.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/grandmastr/hybreed-go/internal/auth"
	"github.com/grandmastr/hybreed-go/internal/config"
	"github.com/grandmastr/hybreed-go/internal/store"
)

const (
	demoEmail    = "alex.carter@hybreed.app"
	demoName     = "Alex Carter"
	demoPassword = "trainhard"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seed failed:", err)
		os.Exit(1)
	}
}

func run() error {
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

	if err := seedFoods(ctx, q); err != nil {
		return fmt.Errorf("seed foods: %w", err)
	}
	if err := seedDemoAthlete(ctx, pool, q); err != nil {
		return fmt.Errorf("seed athlete: %w", err)
	}

	fmt.Println("✓ seed complete")
	fmt.Printf("  demo login: %s / %s\n", demoEmail, demoPassword)
	return nil
}

func seedFoods(ctx context.Context, q *store.Queries) error {
	count, err := q.CountFoods(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		fmt.Printf("• foods already seeded (%d rows)\n", count)
		return nil
	}

	foods := []store.CreateFoodParams{
		{Name: "Banana", Serving: "1 medium · 118g", Kcal: 105, ProteinG: store.Num(1), CarbsG: store.Num(27), FatG: store.Num(0)},
		{Name: "Chicken breast, grilled", Serving: "100g", Kcal: 165, ProteinG: store.Num(31), CarbsG: store.Num(0), FatG: store.Num(4)},
		{Name: "Oats, rolled", Serving: "1 cup cooked", Kcal: 154, ProteinG: store.Num(6), CarbsG: store.Num(27), FatG: store.Num(3)},
		{Name: "Whole milk", Serving: "250 ml", Kcal: 149, ProteinG: store.Num(8), CarbsG: store.Num(12), FatG: store.Num(8)},
		{Name: "Almonds", Serving: "28g · handful", Kcal: 164, ProteinG: store.Num(6), CarbsG: store.Num(6), FatG: store.Num(14)},
		{Name: "Brown rice", Serving: "1 cup cooked", Kcal: 216, ProteinG: store.Num(5), CarbsG: store.Num(45), FatG: store.Num(2)},
		{Name: "RXBAR Chocolate Sea Salt", Serving: "1 bar · 52g", Kcal: 210, ProteinG: store.Num(12), CarbsG: store.Num(24), FatG: store.Num(9), Barcode: store.Ptr("851770003120")},
	}
	for _, f := range foods {
		if _, err := q.CreateFood(ctx, f); err != nil {
			return err
		}
	}
	fmt.Printf("• seeded %d foods\n", len(foods))
	return nil
}

func seedDemoAthlete(ctx context.Context, pool *pgxpool.Pool, q *store.Queries) error {
	if existing, err := q.GetUserByEmail(ctx, demoEmail); err == nil {
		fmt.Printf("• demo athlete already exists (%s)\n", existing.ID)
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	hash, err := auth.HashPassword(demoPassword)
	if err != nil {
		return err
	}
	user, err := q.CreateUser(ctx, store.CreateUserParams{
		Name:          demoName,
		Email:         demoEmail,
		PasswordHash:  store.Ptr(hash),
		EmailVerified: true,
	})
	if err != nil {
		return err
	}
	if _, err := q.CreateUserSettings(ctx, user.ID); err != nil {
		return err
	}
	if _, err := q.UpdateUserSettings(ctx, store.UpdateUserSettingsParams{
		UserID:        user.ID,
		ConnectedApps: store.Ptr(int32(3)),
		BodyWeightKg:  store.Num(78.4),
	}); err != nil {
		return err
	}
	if _, err := q.UpdateUserProfile(ctx, store.UpdateUserProfileParams{
		ID:     user.ID,
		Handle: store.Ptr("Hybrid Athlete · Block 3"),
		Status: store.Ptr("Productive"),
	}); err != nil {
		return err
	}

	if err := seedPRs(ctx, q, user.ID); err != nil {
		return err
	}
	if err := seedActivities(ctx, pool, q, user.ID); err != nil {
		return err
	}
	if err := seedPlan(ctx, q, user.ID); err != nil {
		return err
	}
	if err := seedNutrition(ctx, pool, q, user.ID); err != nil {
		return err
	}

	fmt.Printf("• seeded demo athlete (%s)\n", user.ID)
	return nil
}

func seedPRs(ctx context.Context, q *store.Queries, userID uuid.UUID) error {
	prs := []store.CreatePersonalRecordParams{
		{UserID: userID, Label: "5K run", Value: "21:48", Icon: "run", Position: 0},
		{UserID: userID, Label: "Bench press", Value: "92.5 kg", Icon: "dumbbell", Position: 1},
		{UserID: userID, Label: "Longest run", Value: "21.1 km", Icon: "pin", Position: 2},
		{UserID: userID, Label: "Best week load", Value: "805", Icon: "bolt", Position: 3},
	}
	for _, pr := range prs {
		if _, err := q.CreatePersonalRecord(ctx, pr); err != nil {
			return err
		}
	}
	return nil
}

func seedActivities(ctx context.Context, pool *pgxpool.Pool, q *store.Queries, userID uuid.UUID) error {
	now := time.Now().UTC()
	day := func(offset, hour, min int) time.Time {
		d := now.AddDate(0, 0, offset)
		return time.Date(d.Year(), d.Month(), d.Day(), hour, min, 0, 0, time.UTC)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	qtx := q.WithTx(tx)

	// Today's tempo run with splits.
	run, err := qtx.CreateActivity(ctx, store.CreateActivityParams{
		UserID: userID, Kind: "run", Title: "Morning Tempo", PerformedAt: store.TS(day(0, 6, 40)), Load: 185,
	})
	if err != nil {
		return err
	}
	if _, err := qtx.CreateRunDetail(ctx, store.CreateRunDetailParams{
		ActivityID: run.ID, DistanceM: 6200, DurationS: 1710, AvgPaceSPerKm: 275, AvgHr: 162, Calories: 486,
	}); err != nil {
		return err
	}
	splits := []store.CreateRunSplitParams{
		{ActivityID: run.ID, Km: 1, PaceS: 285, Hr: 148},
		{ActivityID: run.ID, Km: 2, PaceS: 278, Hr: 156},
		{ActivityID: run.ID, Km: 3, PaceS: 272, Hr: 161},
		{ActivityID: run.ID, Km: 4, PaceS: 268, Hr: 164},
		{ActivityID: run.ID, Km: 5, PaceS: 270, Hr: 166},
		{ActivityID: run.ID, Km: 6, PaceS: 264, Hr: 168},
	}
	for _, sp := range splits {
		if _, err := qtx.CreateRunSplit(ctx, sp); err != nil {
			return err
		}
	}

	// Yesterday's pull day.
	lift, err := qtx.CreateActivity(ctx, store.CreateActivityParams{
		UserID: userID, Kind: "lift", Title: "Pull Day", PerformedAt: store.TS(day(-1, 18, 30)), Load: 198,
	})
	if err != nil {
		return err
	}
	if err := seedExercise(ctx, qtx, lift.ID, 0, "Deadlift", "Last: 140kg × 5", []store.CreateLiftSetParams{
		{WeightKg: store.Num(120), Reps: 5, Done: true}, {WeightKg: store.Num(140), Reps: 5, Done: true},
	}); err != nil {
		return err
	}
	if err := seedExercise(ctx, qtx, lift.ID, 1, "Barbell Row", "Last: 70kg × 8", []store.CreateLiftSetParams{
		{WeightKg: store.Num(70), Reps: 8, Done: true}, {WeightKg: store.Num(70), Reps: 8, Done: true},
	}); err != nil {
		return err
	}

	// Long run earlier in the week.
	long, err := qtx.CreateActivity(ctx, store.CreateActivityParams{
		UserID: userID, Kind: "run", Title: "Long Run", PerformedAt: store.TS(day(-4, 7, 0)), Load: 256,
	})
	if err != nil {
		return err
	}
	if _, err := qtx.CreateRunDetail(ctx, store.CreateRunDetailParams{
		ActivityID: long.ID, DistanceM: 16400, DurationS: 5040, AvgPaceSPerKm: 307, AvgHr: 154, Calories: 1180,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func seedExercise(ctx context.Context, q *store.Queries, activityID uuid.UUID, pos int32, name, note string, sets []store.CreateLiftSetParams) error {
	ex, err := q.CreateExercise(ctx, store.CreateExerciseParams{ActivityID: activityID, Name: name, Note: note, Position: pos})
	if err != nil {
		return err
	}
	for i, s := range sets {
		s.ExerciseID = ex.ID
		s.Position = int32(i)
		if _, err := q.CreateLiftSet(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func seedPlan(ctx context.Context, q *store.Queries, userID uuid.UUID) error {
	today := store.DateOf(time.Now().UTC())
	items := []store.CreatePlanItemParams{
		{UserID: userID, PlanDate: today, Kind: "run", Title: "Tempo Run", Meta: "6 km · Zone 3", Position: 0},
		{UserID: userID, PlanDate: today, Kind: "lift", Title: "Push Day", Meta: "Chest · Shoulders · Triceps", Position: 1},
	}
	for _, it := range items {
		created, err := q.CreatePlanItem(ctx, it)
		if err != nil {
			return err
		}
		if it.Kind == "run" {
			if _, err := q.SetPlanItemDone(ctx, store.SetPlanItemDoneParams{ID: created.ID, UserID: userID, Done: true}); err != nil {
				return err
			}
		}
	}
	return nil
}

func seedNutrition(ctx context.Context, pool *pgxpool.Pool, q *store.Queries, userID uuid.UUID) error {
	now := time.Now().UTC()
	today := store.DateOf(now)

	// Tune the day's budget/water beyond the schema defaults to match the mock.
	if _, err := pool.Exec(ctx, `
		INSERT INTO nutrition_days (user_id, day, base_kcal, training_bonus_kcal, water_ml, water_target_ml, protein_target_g, carbs_target_g, fat_target_g)
		VALUES ($1, $2, 2030, 420, 1600, 2600, 165, 250, 78)
		ON CONFLICT (user_id, day) DO UPDATE
		SET base_kcal = EXCLUDED.base_kcal, training_bonus_kcal = EXCLUDED.training_bonus_kcal, water_ml = EXCLUDED.water_ml
	`, userID, today); err != nil {
		return err
	}

	at := func(h, m int) time.Time { return time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, time.UTC) }
	meals := []struct {
		slot    string
		logged  time.Time
		planned bool
		pos     int32
		items   []store.CreateMealItemParams
	}{
		{slot: "Breakfast", logged: at(8, 15), pos: 0, items: []store.CreateMealItemParams{
			{Name: "Greek yogurt & berries", Kcal: 240, ProteinG: store.Num(22), CarbsG: store.Num(28), FatG: store.Num(6)},
			{Name: "Sourdough toast, 2 eggs", Kcal: 280, ProteinG: store.Num(18), CarbsG: store.Num(24), FatG: store.Num(13)},
		}},
		{slot: "Lunch", logged: at(12, 45), pos: 1, items: []store.CreateMealItemParams{
			{Name: "Chicken & rice bowl", Kcal: 560, ProteinG: store.Num(46), CarbsG: store.Num(62), FatG: store.Num(14)},
			{Name: "Apple", Kcal: 120, ProteinG: store.Num(0), CarbsG: store.Num(25), FatG: store.Num(0)},
		}},
		{slot: "Snacks", logged: at(16, 20), pos: 3, items: []store.CreateMealItemParams{
			{Name: "Protein shake", Kcal: 220, ProteinG: store.Num(40), CarbsG: store.Num(8), FatG: store.Num(3)},
			{Name: "Trail mix", Kcal: 420, ProteinG: store.Num(12), CarbsG: store.Num(36), FatG: store.Num(26)},
		}},
		{slot: "Dinner", planned: true, pos: 2},
	}

	for _, m := range meals {
		loggedAt := store.TS(m.logged)
		if m.planned {
			loggedAt.Valid = false
		}
		meal, err := q.CreateMeal(ctx, store.CreateMealParams{
			UserID: userID, Day: today, Slot: m.slot, LoggedAt: loggedAt, Planned: m.planned, Position: m.pos,
		})
		if err != nil {
			return err
		}
		for i, item := range m.items {
			item.MealID = meal.ID
			item.Position = int32(i)
			if _, err := q.CreateMealItem(ctx, item); err != nil {
				return err
			}
		}
	}
	return nil
}
