// Package training serves the "Train" tab and the Today load ring: activities
// (runs + lifts), the daily plan, and training-load rollups.
package training

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/grandmastr/hybreed-go/internal/cache"
	"github.com/grandmastr/hybreed-go/internal/format"
	"github.com/grandmastr/hybreed-go/internal/httpx"
	"github.com/grandmastr/hybreed-go/internal/store"
)

const loadCacheTTL = 60 * time.Second

// Service implements the training use cases.
type Service struct {
	pool  *pgxpool.Pool
	q     *store.Queries
	cache *cache.Cache
	log   *slog.Logger
}

// NewService builds the training service.
func NewService(pool *pgxpool.Pool, q *store.Queries, c *cache.Cache, log *slog.Logger) *Service {
	return &Service{pool: pool, q: q, cache: c, log: log}
}

// ── DTOs ────────────────────────────────────────────────────────────────────

type RunFields struct {
	DistanceM  int32       `json:"distanceM"`
	DistanceKm float64     `json:"distanceKm"`
	DurationS  int32       `json:"durationS"`
	Duration   string      `json:"duration"`
	PaceS      int32       `json:"paceS"`
	Pace       string      `json:"pace"`
	AvgHr      int32       `json:"avgHr"`
	Calories   int32       `json:"calories"`
	Route      [][]float64 `json:"route,omitempty"`
}

type Split struct {
	Km    int32  `json:"km"`
	PaceS int32  `json:"paceS"`
	Pace  string `json:"pace"`
	Hr    int32  `json:"hr"`
}

type SetField struct {
	ID       string  `json:"id"`
	WeightKg float64 `json:"weightKg"`
	Reps     int32   `json:"reps"`
	Done     bool    `json:"done"`
}

type ExerciseField struct {
	ID   string     `json:"id"`
	Name string     `json:"name"`
	Note string     `json:"note"`
	Sets []SetField `json:"sets"`
}

type LiftFields struct {
	TotalSets     int64           `json:"totalSets"`
	TotalVolumeKg int64           `json:"totalVolumeKg"`
	Exercises     []ExerciseField `json:"exercises,omitempty"`
}

type ActivityDTO struct {
	ID          string      `json:"id"`
	Kind        string      `json:"kind"`
	Title       string      `json:"title"`
	PerformedAt time.Time   `json:"performedAt"`
	Load        int32       `json:"load"`
	Planned     bool        `json:"planned"`
	Notes       string      `json:"notes,omitempty"`
	Run         *RunFields  `json:"run,omitempty"`
	Lift        *LiftFields `json:"lift,omitempty"`
	Splits      []Split     `json:"splits,omitempty"`
}

type PlanDTO struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Meta  string `json:"meta"`
	Done  bool   `json:"done"`
}

type Balance struct {
	Run  int `json:"run"`
	Lift int `json:"lift"`
}

type LoadSummary struct {
	TodayLoad  int64   `json:"todayLoad"`
	WeeklyLoad int64   `json:"weeklyLoad"`
	LoadTarget int32   `json:"loadTarget"`
	Rest       int     `json:"rest"`
	Streak     int32   `json:"streak"`
	Status     string  `json:"status"`
	Balance    Balance `json:"balance"`
	LoadWeek   []int64 `json:"loadWeek"`
}

// ── Activities ──────────────────────────────────────────────────────────────

// ListActivities returns the activity history, newest first.
func (s *Service) ListActivities(ctx context.Context, userID uuid.UUID, kind *string, limit, offset int32) ([]ActivityDTO, error) {
	rows, err := s.q.ListActivities(ctx, store.ListActivitiesParams{
		UserID: userID,
		Kind:   kind,
		Lim:    limit,
		Off:    offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list activities: %w", err)
	}
	out := make([]ActivityDTO, 0, len(rows))
	for _, a := range rows {
		dto, err := s.summary(ctx, a)
		if err != nil {
			return nil, err
		}
		out = append(out, dto)
	}
	return out, nil
}

// GetActivity returns a single activity with full run/lift detail.
func (s *Service) GetActivity(ctx context.Context, userID, id uuid.UUID) (ActivityDTO, error) {
	a, err := s.q.GetActivity(ctx, store.GetActivityParams{ID: id, UserID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ActivityDTO{}, httpx.ErrNotFound("activity not found")
		}
		return ActivityDTO{}, fmt.Errorf("get activity: %w", err)
	}
	return s.detail(ctx, a)
}

// CreateActivityInput is the validated payload for a new activity.
type CreateActivityInput struct {
	Kind        string
	Title       string
	PerformedAt time.Time
	Load        int32
	Planned     bool
	Notes       string
	Run         *RunInput
	Lift        *LiftInput
}

type RunInput struct {
	DistanceM     int32
	DurationS     int32
	AvgPaceSPerKm int32
	AvgHr         int32
	Calories      int32
	Splits        []SplitInput
	Route         [][]float64
}

type SplitInput struct {
	Km    int32
	PaceS int32
	Hr    int32
}

type LiftInput struct {
	Exercises []ExerciseInput
}

type ExerciseInput struct {
	Name string
	Note string
	Sets []SetInput
}

type SetInput struct {
	WeightKg float64
	Reps     int32
	Done     bool
}

// CreateActivity persists a run or lift (with its children) in one transaction.
func (s *Service) CreateActivity(ctx context.Context, userID uuid.UUID, in CreateActivityInput) (ActivityDTO, error) {
	if in.Title == "" {
		return ActivityDTO{}, httpx.ErrValidation("title is required")
	}
	if in.PerformedAt.IsZero() {
		in.PerformedAt = time.Now().UTC()
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ActivityDTO{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit
	qtx := s.q.WithTx(tx)

	activity, err := qtx.CreateActivity(ctx, store.CreateActivityParams{
		UserID:      userID,
		Kind:        in.Kind,
		Title:       in.Title,
		PerformedAt: store.TS(in.PerformedAt),
		Load:        in.Load,
		Planned:     in.Planned,
		Notes:       in.Notes,
	})
	if err != nil {
		return ActivityDTO{}, fmt.Errorf("create activity: %w", err)
	}

	switch in.Kind {
	case "run":
		if in.Run == nil {
			return ActivityDTO{}, httpx.ErrValidation("run payload is required for kind=run")
		}
		if _, err := qtx.CreateRunDetail(ctx, store.CreateRunDetailParams{
			ActivityID:    activity.ID,
			DistanceM:     in.Run.DistanceM,
			DurationS:     in.Run.DurationS,
			AvgPaceSPerKm: in.Run.AvgPaceSPerKm,
			AvgHr:         in.Run.AvgHr,
			Calories:      in.Run.Calories,
			Route:         mustRouteJSON(in.Run.Route),
		}); err != nil {
			return ActivityDTO{}, fmt.Errorf("create run detail: %w", err)
		}
		for _, sp := range in.Run.Splits {
			if _, err := qtx.CreateRunSplit(ctx, store.CreateRunSplitParams{
				ActivityID: activity.ID,
				Km:         sp.Km,
				PaceS:      sp.PaceS,
				Hr:         sp.Hr,
			}); err != nil {
				return ActivityDTO{}, fmt.Errorf("create run split: %w", err)
			}
		}
	case "lift":
		if in.Lift == nil {
			return ActivityDTO{}, httpx.ErrValidation("lift payload is required for kind=lift")
		}
		for i, ex := range in.Lift.Exercises {
			exercise, err := qtx.CreateExercise(ctx, store.CreateExerciseParams{
				ActivityID: activity.ID,
				Name:       ex.Name,
				Note:       ex.Note,
				Position:   int32(i),
			})
			if err != nil {
				return ActivityDTO{}, fmt.Errorf("create exercise: %w", err)
			}
			for j, set := range ex.Sets {
				if _, err := qtx.CreateLiftSet(ctx, store.CreateLiftSetParams{
					ExerciseID: exercise.ID,
					WeightKg:   store.Num(set.WeightKg),
					Reps:       set.Reps,
					Done:       set.Done,
					Position:   int32(j),
				}); err != nil {
					return ActivityDTO{}, fmt.Errorf("create lift set: %w", err)
				}
			}
		}
	default:
		return ActivityDTO{}, httpx.ErrValidation("kind must be 'run' or 'lift'")
	}

	if err := tx.Commit(ctx); err != nil {
		return ActivityDTO{}, fmt.Errorf("commit tx: %w", err)
	}

	s.invalidateLoad(ctx, userID, in.PerformedAt)
	return s.detail(ctx, activity)
}

// DeleteActivity removes an activity (cascades to its children).
func (s *Service) DeleteActivity(ctx context.Context, userID, id uuid.UUID) error {
	if err := s.q.DeleteActivity(ctx, store.DeleteActivityParams{ID: id, UserID: userID}); err != nil {
		return fmt.Errorf("delete activity: %w", err)
	}
	s.invalidateLoad(ctx, userID, time.Now().UTC())
	return nil
}

// ── Exercise library ──────────────────────────────────────────────────────

// ExerciseLibraryItem is a selectable exercise for the lift logger.
type ExerciseLibraryItem struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Muscle           string   `json:"muscle"`
	Equipment        string   `json:"equipment"`
	Difficulty       string   `json:"difficulty,omitempty"`
	Force            string   `json:"force,omitempty"`
	SecondaryMuscles []string `json:"secondaryMuscles,omitempty"`
	Steps            []string `json:"steps,omitempty"`
	VideoURLs        []string `json:"videoUrls,omitempty"`
	YoutubeURL       string   `json:"youtubeUrl,omitempty"`
}

// ListExerciseLibrary returns the catalog of selectable exercises, collapsing
// the MuscleWiki name variations down to one entry per exercise name.
func (s *Service) ListExerciseLibrary(ctx context.Context) ([]ExerciseLibraryItem, error) {
	rows, err := s.q.ListExerciseLibrary(ctx)
	if err != nil {
		return nil, fmt.Errorf("list exercise library: %w", err)
	}
	seen := make(map[string]bool, len(rows))
	out := make([]ExerciseLibraryItem, 0, len(rows))
	for _, e := range rows {
		if seen[e.Name] {
			continue
		}
		seen[e.Name] = true
		var secondary, steps, videos []string
		_ = json.Unmarshal(e.SecondaryMuscles, &secondary)
		_ = json.Unmarshal(e.Steps, &steps)
		_ = json.Unmarshal(e.VideoUrls, &videos)
		out = append(out, ExerciseLibraryItem{
			ID:               e.ID.String(),
			Name:             e.Name,
			Muscle:           e.Muscle,
			Equipment:        e.Equipment,
			Difficulty:       e.Difficulty,
			Force:            e.Force,
			SecondaryMuscles: secondary,
			Steps:            steps,
			VideoURLs:        videos,
			YoutubeURL:       e.YoutubeUrl,
		})
	}
	return out, nil
}

func (s *Service) summary(ctx context.Context, a store.Activity) (ActivityDTO, error) {
	dto := baseActivity(a)
	switch a.Kind {
	case "run":
		rd, err := s.q.GetRunDetail(ctx, a.ID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return ActivityDTO{}, fmt.Errorf("get run detail: %w", err)
		}
		if err == nil {
			dto.Run = runFields(rd)
		}
	case "lift":
		ls, err := s.q.LiftSummary(ctx, a.ID)
		if err != nil {
			return ActivityDTO{}, fmt.Errorf("lift summary: %w", err)
		}
		dto.Lift = &LiftFields{TotalSets: ls.TotalSets, TotalVolumeKg: ls.TotalVolumeKg}
	}
	return dto, nil
}

func (s *Service) detail(ctx context.Context, a store.Activity) (ActivityDTO, error) {
	dto, err := s.summary(ctx, a)
	if err != nil {
		return ActivityDTO{}, err
	}
	switch a.Kind {
	case "run":
		if dto.Run != nil {
			if rd, err := s.q.GetRunDetail(ctx, a.ID); err == nil && len(rd.Route) > 0 {
				var route [][]float64
				if json.Unmarshal(rd.Route, &route) == nil {
					dto.Run.Route = route
				}
			}
		}
		splits, err := s.q.ListRunSplits(ctx, a.ID)
		if err != nil {
			return ActivityDTO{}, fmt.Errorf("list splits: %w", err)
		}
		for _, sp := range splits {
			dto.Splits = append(dto.Splits, Split{Km: sp.Km, PaceS: sp.PaceS, Pace: format.Pace(int(sp.PaceS)), Hr: sp.Hr})
		}
	case "lift":
		exercises, err := s.q.ListExercisesByActivity(ctx, a.ID)
		if err != nil {
			return ActivityDTO{}, fmt.Errorf("list exercises: %w", err)
		}
		if dto.Lift == nil {
			dto.Lift = &LiftFields{}
		}
		for _, ex := range exercises {
			sets, err := s.q.ListLiftSetsByExercise(ctx, ex.ID)
			if err != nil {
				return ActivityDTO{}, fmt.Errorf("list sets: %w", err)
			}
			ef := ExerciseField{ID: ex.ID.String(), Name: ex.Name, Note: ex.Note, Sets: make([]SetField, 0, len(sets))}
			for _, set := range sets {
				ef.Sets = append(ef.Sets, SetField{ID: set.ID.String(), WeightKg: store.Float(set.WeightKg), Reps: set.Reps, Done: set.Done})
			}
			dto.Lift.Exercises = append(dto.Lift.Exercises, ef)
		}
	}
	return dto, nil
}

func baseActivity(a store.Activity) ActivityDTO {
	return ActivityDTO{
		ID:          a.ID.String(),
		Kind:        a.Kind,
		Title:       a.Title,
		PerformedAt: store.TimeOf(a.PerformedAt),
		Load:        a.Load,
		Planned:     a.Planned,
		Notes:       a.Notes,
	}
}

func mustRouteJSON(route [][]float64) []byte {
	if len(route) == 0 {
		return []byte("[]")
	}
	b, err := json.Marshal(route)
	if err != nil {
		return []byte("[]")
	}
	return b
}

func runFields(rd store.RunDetail) *RunFields {
	return &RunFields{
		DistanceM:  rd.DistanceM,
		DistanceKm: format.Km(int(rd.DistanceM)),
		DurationS:  rd.DurationS,
		Duration:   format.Clock(int(rd.DurationS)),
		PaceS:      rd.AvgPaceSPerKm,
		Pace:       format.Pace(int(rd.AvgPaceSPerKm)),
		AvgHr:      rd.AvgHr,
		Calories:   rd.Calories,
	}
}

// ── Plan ────────────────────────────────────────────────────────────────────

// ListPlan returns the plan items for a day.
func (s *Service) ListPlan(ctx context.Context, userID uuid.UUID, day time.Time) ([]PlanDTO, error) {
	rows, err := s.q.ListPlanItems(ctx, store.ListPlanItemsParams{UserID: userID, PlanDate: store.DateOf(day)})
	if err != nil {
		return nil, fmt.Errorf("list plan: %w", err)
	}
	out := make([]PlanDTO, 0, len(rows))
	for _, p := range rows {
		out = append(out, planDTO(p))
	}
	return out, nil
}

// CreatePlanItem adds a plan entry.
func (s *Service) CreatePlanItem(ctx context.Context, p store.CreatePlanItemParams) (PlanDTO, error) {
	row, err := s.q.CreatePlanItem(ctx, p)
	if err != nil {
		return PlanDTO{}, fmt.Errorf("create plan item: %w", err)
	}
	return planDTO(row), nil
}

// SetPlanDone toggles a plan item's completion.
func (s *Service) SetPlanDone(ctx context.Context, userID, id uuid.UUID, done bool) (PlanDTO, error) {
	row, err := s.q.SetPlanItemDone(ctx, store.SetPlanItemDoneParams{ID: id, UserID: userID, Done: done})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PlanDTO{}, httpx.ErrNotFound("plan item not found")
		}
		return PlanDTO{}, fmt.Errorf("set plan done: %w", err)
	}
	return planDTO(row), nil
}

// DeletePlanItem removes a plan entry.
func (s *Service) DeletePlanItem(ctx context.Context, userID, id uuid.UUID) error {
	if err := s.q.DeletePlanItem(ctx, store.DeletePlanItemParams{ID: id, UserID: userID}); err != nil {
		return fmt.Errorf("delete plan item: %w", err)
	}
	return nil
}

func planDTO(p store.PlanItem) PlanDTO {
	return PlanDTO{ID: p.ID.String(), Kind: p.Kind, Title: p.Title, Meta: p.Meta, Done: p.Done}
}

// ── Load summary ────────────────────────────────────────────────────────────

// GetLoadSummary computes (and caches) the Today load ring data.
func (s *Service) GetLoadSummary(ctx context.Context, userID uuid.UUID) (LoadSummary, error) {
	now := time.Now().UTC()
	cacheKey := loadKey(userID, now)

	var cached LoadSummary
	if s.cache.Get(ctx, cacheKey, &cached) {
		return cached, nil
	}

	user, err := s.q.GetUserByID(ctx, userID)
	if err != nil {
		return LoadSummary{}, fmt.Errorf("get user: %w", err)
	}

	startToday := startOfDay(now)
	startTomorrow := startToday.AddDate(0, 0, 1)
	startWeek := startToday.AddDate(0, 0, -6) // 7-day window incl. today

	todayLoad, err := s.q.SumLoadInRange(ctx, store.SumLoadInRangeParams{UserID: userID, Start: store.TS(startToday), Stop: store.TS(startTomorrow)})
	if err != nil {
		return LoadSummary{}, fmt.Errorf("today load: %w", err)
	}
	weeklyLoad, err := s.q.SumLoadInRange(ctx, store.SumLoadInRangeParams{UserID: userID, Start: store.TS(startWeek), Stop: store.TS(startTomorrow)})
	if err != nil {
		return LoadSummary{}, fmt.Errorf("weekly load: %w", err)
	}
	dayRows, err := s.q.LoadByDayInRange(ctx, store.LoadByDayInRangeParams{UserID: userID, Start: store.TS(startWeek), Stop: store.TS(startTomorrow)})
	if err != nil {
		return LoadSummary{}, fmt.Errorf("load by day: %w", err)
	}
	kindRows, err := s.q.LoadByKindInRange(ctx, store.LoadByKindInRangeParams{UserID: userID, Start: store.TS(startWeek), Stop: store.TS(startTomorrow)})
	if err != nil {
		return LoadSummary{}, fmt.Errorf("load by kind: %w", err)
	}

	// Streak is computed from activity history (consecutive active days), not a
	// stored counter; fall back to 0 if the lookup fails.
	streak, err := s.q.GetActivityStreak(ctx, userID)
	if err != nil {
		streak = 0
	}

	summary := LoadSummary{
		TodayLoad:  todayLoad,
		WeeklyLoad: weeklyLoad,
		LoadTarget: user.LoadTarget,
		Streak:     streak,
		Status:     user.Status,
		Rest:       recoveryScore(todayLoad, user.LoadTarget),
		Balance:    balance(kindRows),
		LoadWeek:   loadWeek(dayRows, startWeek),
	}

	s.cache.Set(ctx, cacheKey, summary, loadCacheTTL)
	return summary, nil
}

func (s *Service) invalidateLoad(ctx context.Context, userID uuid.UUID, when time.Time) {
	s.cache.Delete(ctx, loadKey(userID, when), loadKey(userID, time.Now().UTC()))
}

func loadKey(userID uuid.UUID, t time.Time) string {
	return fmt.Sprintf("load:%s:%s", userID, t.UTC().Format("2006-01-02"))
}

func startOfDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// recoveryScore is a simple readiness heuristic: lighter today vs target → more rest.
func recoveryScore(todayLoad int64, target int32) int {
	if target <= 0 {
		return 100
	}
	rest := 100 - int(todayLoad*100/int64(target))
	if rest < 0 {
		return 0
	}
	if rest > 100 {
		return 100
	}
	return rest
}

func balance(rows []store.LoadByKindInRangeRow) Balance {
	var run, lift int64
	for _, r := range rows {
		switch r.Kind {
		case "run":
			run += r.Load
		case "lift":
			lift += r.Load
		}
	}
	total := run + lift
	if total == 0 {
		return Balance{Run: 0, Lift: 0}
	}
	runPct := int(run * 100 / total)
	return Balance{Run: runPct, Lift: 100 - runPct}
}

// loadWeek expands sparse per-day rows into a fixed 7-slot array (oldest→today).
func loadWeek(rows []store.LoadByDayInRangeRow, startWeek time.Time) []int64 {
	byDay := make(map[string]int64, len(rows))
	for _, r := range rows {
		byDay[store.DateValue(r.Day).Format("2006-01-02")] = r.Load
	}
	out := make([]int64, 7)
	for i := 0; i < 7; i++ {
		out[i] = byDay[startWeek.AddDate(0, 0, i).Format("2006-01-02")]
	}
	return out
}
