// Package nutrition serves the "Fuel" tab: the daily calorie/macro/water summary,
// meal logging, and the searchable food database (with Redis-cached lookups).
package nutrition

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/grandmastr/hybreed-go/internal/cache"
	"github.com/grandmastr/hybreed-go/internal/httpx"
	"github.com/grandmastr/hybreed-go/internal/store"
)

const (
	foodSearchTTL  = 5 * time.Minute
	barcodeTTL     = time.Hour
	defaultSearchN = 10
)

// Service implements the nutrition use cases.
type Service struct {
	pool  *pgxpool.Pool
	q     *store.Queries
	cache *cache.Cache
	log   *slog.Logger
}

// NewService builds the nutrition service.
func NewService(pool *pgxpool.Pool, q *store.Queries, c *cache.Cache, log *slog.Logger) *Service {
	return &Service{pool: pool, q: q, cache: c, log: log}
}

// ── DTOs ────────────────────────────────────────────────────────────────────

type Macro struct {
	G      float64 `json:"g"`
	Target int32   `json:"target"`
}

type Water struct {
	Ml     int32 `json:"ml"`
	Target int32 `json:"target"`
}

type Macros struct {
	Protein Macro `json:"protein"`
	Carbs   Macro `json:"carbs"`
	Fat     Macro `json:"fat"`
}

type FoodItemDTO struct {
	Name     string  `json:"name"`
	Kcal     int32   `json:"kcal"`
	ProteinG float64 `json:"p"`
	CarbsG   float64 `json:"c"`
	FatG     float64 `json:"f"`
}

type MealDTO struct {
	ID      string        `json:"id"`
	Slot    string        `json:"slot"`
	Time    string        `json:"time"`
	Kcal    int64         `json:"kcal"`
	Planned bool          `json:"planned"`
	Items   []FoodItemDTO `json:"items"`
}

type Summary struct {
	Day           string    `json:"day"`
	Budget        int64     `json:"budget"`
	Base          int32     `json:"base"`
	TrainingBonus int32     `json:"trainingBonus"`
	Consumed      int64     `json:"consumed"`
	Remaining     int64     `json:"remaining"`
	Macros        Macros    `json:"macros"`
	Water         Water     `json:"water"`
	Meals         []MealDTO `json:"meals"`
}

type FoodDTO struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Serving  string  `json:"serving"`
	Kcal     int32   `json:"kcal"`
	ProteinG float64 `json:"p"`
	CarbsG   float64 `json:"c"`
	FatG     float64 `json:"f"`
	Barcode  *string `json:"barcode,omitempty"`
}

// ── Daily summary ───────────────────────────────────────────────────────────

// GetSummary returns the full Fuel-tab payload for a day.
func (s *Service) GetSummary(ctx context.Context, userID uuid.UUID, day time.Time) (Summary, error) {
	nd, err := s.q.UpsertNutritionDay(ctx, store.UpsertNutritionDayParams{UserID: userID, Day: store.DateOf(day)})
	if err != nil {
		return Summary{}, fmt.Errorf("ensure nutrition day: %w", err)
	}
	consumed, err := s.q.DayConsumed(ctx, store.DayConsumedParams{UserID: userID, Day: store.DateOf(day)})
	if err != nil {
		return Summary{}, fmt.Errorf("day consumed: %w", err)
	}
	meals, err := s.listMeals(ctx, userID, day)
	if err != nil {
		return Summary{}, err
	}

	budget := int64(nd.BaseKcal) + int64(nd.TrainingBonusKcal)
	return Summary{
		Day:           store.DateValue(nd.Day).Format("2006-01-02"),
		Budget:        budget,
		Base:          nd.BaseKcal,
		TrainingBonus: nd.TrainingBonusKcal,
		Consumed:      consumed.Kcal,
		Remaining:     budget - consumed.Kcal,
		Macros: Macros{
			Protein: Macro{G: consumed.ProteinG, Target: nd.ProteinTargetG},
			Carbs:   Macro{G: consumed.CarbsG, Target: nd.CarbsTargetG},
			Fat:     Macro{G: consumed.FatG, Target: nd.FatTargetG},
		},
		Water: Water{Ml: nd.WaterMl, Target: nd.WaterTargetMl},
		Meals: meals,
	}, nil
}

// AddWater adds (or subtracts) water for a day and returns the new totals.
func (s *Service) AddWater(ctx context.Context, userID uuid.UUID, day time.Time, deltaMl int32) (Water, error) {
	nd, err := s.q.AddWater(ctx, store.AddWaterParams{UserID: userID, Day: store.DateOf(day), DeltaMl: deltaMl})
	if err != nil {
		return Water{}, fmt.Errorf("add water: %w", err)
	}
	return Water{Ml: nd.WaterMl, Target: nd.WaterTargetMl}, nil
}

func (s *Service) listMeals(ctx context.Context, userID uuid.UUID, day time.Time) ([]MealDTO, error) {
	meals, err := s.q.ListMealsForDay(ctx, store.ListMealsForDayParams{UserID: userID, Day: store.DateOf(day)})
	if err != nil {
		return nil, fmt.Errorf("list meals: %w", err)
	}
	items, err := s.q.ListMealItemsForDay(ctx, store.ListMealItemsForDayParams{UserID: userID, Day: store.DateOf(day)})
	if err != nil {
		return nil, fmt.Errorf("list meal items: %w", err)
	}
	itemsByMeal := make(map[uuid.UUID][]store.MealItem, len(meals))
	for _, it := range items {
		itemsByMeal[it.MealID] = append(itemsByMeal[it.MealID], it)
	}

	out := make([]MealDTO, 0, len(meals))
	for _, m := range meals {
		dto := MealDTO{ID: m.ID.String(), Slot: m.Slot, Planned: m.Planned, Time: timeOrDash(m.LoggedAt), Items: []FoodItemDTO{}}
		var total int64
		for _, it := range itemsByMeal[m.ID] {
			dto.Items = append(dto.Items, FoodItemDTO{
				Name:     it.Name,
				Kcal:     it.Kcal,
				ProteinG: store.Float(it.ProteinG),
				CarbsG:   store.Float(it.CarbsG),
				FatG:     store.Float(it.FatG),
			})
			total += int64(it.Kcal)
		}
		dto.Kcal = total
		out = append(out, dto)
	}
	return out, nil
}

// timeOrDash renders a logged-at timestamp as "HH:MM" (UTC) or an em dash when
// absent (e.g. a planned, not-yet-eaten meal).
func timeOrDash(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return "—"
	}
	return ts.Time.UTC().Format("15:04")
}

// ── Meals ───────────────────────────────────────────────────────────────────

// CreateMealInput is the validated payload for logging a meal.
type CreateMealInput struct {
	Slot     string
	Day      time.Time
	Planned  bool
	LoggedAt *time.Time
	Position int32
	Items    []ItemInput
}

type ItemInput struct {
	Name     string
	Kcal     int32
	ProteinG float64
	CarbsG   float64
	FatG     float64
}

// CreateMeal logs a meal (and any inline items) in one transaction.
func (s *Service) CreateMeal(ctx context.Context, userID uuid.UUID, in CreateMealInput) (MealDTO, error) {
	if in.Slot == "" {
		return MealDTO{}, httpx.ErrValidation("slot is required")
	}
	if in.Day.IsZero() {
		in.Day = time.Now().UTC()
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return MealDTO{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit
	qtx := s.q.WithTx(tx)

	loggedAt := store.TS(time.Now().UTC())
	if in.LoggedAt != nil {
		loggedAt = store.TS(*in.LoggedAt)
	}
	if in.Planned {
		loggedAt.Valid = false
	}

	meal, err := qtx.CreateMeal(ctx, store.CreateMealParams{
		UserID:   userID,
		Day:      store.DateOf(in.Day),
		Slot:     in.Slot,
		LoggedAt: loggedAt,
		Planned:  in.Planned,
		Position: in.Position,
	})
	if err != nil {
		return MealDTO{}, fmt.Errorf("create meal: %w", err)
	}

	dto := MealDTO{ID: meal.ID.String(), Slot: meal.Slot, Planned: meal.Planned, Time: timeOrDash(meal.LoggedAt), Items: []FoodItemDTO{}}
	var total int64
	for i, it := range in.Items {
		created, err := qtx.CreateMealItem(ctx, store.CreateMealItemParams{
			MealID:   meal.ID,
			Name:     it.Name,
			Kcal:     it.Kcal,
			ProteinG: store.Num(it.ProteinG),
			CarbsG:   store.Num(it.CarbsG),
			FatG:     store.Num(it.FatG),
			Position: int32(i),
		})
		if err != nil {
			return MealDTO{}, fmt.Errorf("create meal item: %w", err)
		}
		dto.Items = append(dto.Items, FoodItemDTO{Name: created.Name, Kcal: created.Kcal, ProteinG: store.Float(created.ProteinG), CarbsG: store.Float(created.CarbsG), FatG: store.Float(created.FatG)})
		total += int64(created.Kcal)
	}
	dto.Kcal = total

	if err := tx.Commit(ctx); err != nil {
		return MealDTO{}, fmt.Errorf("commit tx: %w", err)
	}
	return dto, nil
}

// AddMealItem appends a food item to an existing meal owned by the user.
func (s *Service) AddMealItem(ctx context.Context, userID, mealID uuid.UUID, it ItemInput) (FoodItemDTO, error) {
	owned, err := s.q.MealOwnedBy(ctx, store.MealOwnedByParams{ID: mealID, UserID: userID})
	if err != nil {
		return FoodItemDTO{}, fmt.Errorf("check meal owner: %w", err)
	}
	if !owned {
		return FoodItemDTO{}, httpx.ErrNotFound("meal not found")
	}
	existing, err := s.q.ListMealItems(ctx, mealID)
	if err != nil {
		return FoodItemDTO{}, fmt.Errorf("list meal items: %w", err)
	}
	created, err := s.q.CreateMealItem(ctx, store.CreateMealItemParams{
		MealID:   mealID,
		Name:     it.Name,
		Kcal:     it.Kcal,
		ProteinG: store.Num(it.ProteinG),
		CarbsG:   store.Num(it.CarbsG),
		FatG:     store.Num(it.FatG),
		Position: int32(len(existing)),
	})
	if err != nil {
		return FoodItemDTO{}, fmt.Errorf("create meal item: %w", err)
	}
	return FoodItemDTO{Name: created.Name, Kcal: created.Kcal, ProteinG: store.Float(created.ProteinG), CarbsG: store.Float(created.CarbsG), FatG: store.Float(created.FatG)}, nil
}

// DeleteMeal removes a meal owned by the user.
func (s *Service) DeleteMeal(ctx context.Context, userID, mealID uuid.UUID) error {
	if err := s.q.DeleteMeal(ctx, store.DeleteMealParams{ID: mealID, UserID: userID}); err != nil {
		return fmt.Errorf("delete meal: %w", err)
	}
	return nil
}

// ── Foods ───────────────────────────────────────────────────────────────────

// SearchFoods returns foods matching query (Redis-cached for a few minutes).
func (s *Service) SearchFoods(ctx context.Context, query string, limit int) ([]FoodDTO, error) {
	if limit <= 0 {
		limit = defaultSearchN
	}
	// An empty query would match the whole catalogue (ILIKE ALL of an empty set),
	// so short-circuit to no results.
	if strings.TrimSpace(query) == "" {
		return []FoodDTO{}, nil
	}
	key := fmt.Sprintf("foods:search:%d:%s", limit, query)
	var cached []FoodDTO
	if s.cache.Get(ctx, key, &cached) {
		return cached, nil
	}
	rows, err := s.q.SearchFoods(ctx, store.SearchFoodsParams{Query: query, Lim: int32(limit)})
	if err != nil {
		return nil, fmt.Errorf("search foods: %w", err)
	}
	out := make([]FoodDTO, 0, len(rows))
	for _, f := range rows {
		out = append(out, toFoodDTO(f))
	}
	s.cache.Set(ctx, key, out, foodSearchTTL)
	return out, nil
}

// GetByBarcode looks up a food by barcode (Redis-cached for an hour).
func (s *Service) GetByBarcode(ctx context.Context, code string) (FoodDTO, error) {
	key := "foods:barcode:" + code
	var cached FoodDTO
	if s.cache.Get(ctx, key, &cached) {
		return cached, nil
	}
	f, err := s.q.GetFoodByBarcode(ctx, store.Ptr(code))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return s.barcodeFromOpenFoodFacts(ctx, key, code)
		}
		return FoodDTO{}, fmt.Errorf("get food by barcode: %w", err)
	}
	dto := toFoodDTO(f)
	s.cache.Set(ctx, key, dto, barcodeTTL)
	return dto, nil
}

// barcodeFromOpenFoodFacts handles a local barcode miss: it asks Open Food Facts,
// and on a hit persists the product into the foods table (so future scans are a
// DB lookup) before returning it. A 404 maps to "no product for this barcode".
func (s *Service) barcodeFromOpenFoodFacts(ctx context.Context, cacheKey, code string) (FoodDTO, error) {
	off, err := lookupOpenFoodFacts(ctx, code)
	if err != nil {
		s.log.Warn("open food facts lookup failed", "barcode", code, "err", err)
		return FoodDTO{}, httpx.ErrNotFound("no product for this barcode")
	}
	if off == nil {
		return FoodDTO{}, httpx.ErrNotFound("no product for this barcode")
	}

	created, err := s.q.CreateFood(ctx, store.CreateFoodParams{
		Name:     off.Name,
		Serving:  "100 g",
		Kcal:     off.Kcal,
		ProteinG: store.Num(off.ProteinG),
		CarbsG:   store.Num(off.CarbsG),
		FatG:     store.Num(off.FatG),
		Barcode:  store.Ptr(code),
	})
	if err != nil {
		// Couldn't persist (e.g. a concurrent insert won the unique barcode);
		// still return the product we fetched.
		s.log.Warn("persist open food facts product failed", "barcode", code, "err", err)
		dto := FoodDTO{Name: off.Name, Serving: "100 g", Kcal: off.Kcal, ProteinG: off.ProteinG, CarbsG: off.CarbsG, FatG: off.FatG, Barcode: store.Ptr(code)}
		s.cache.Set(ctx, cacheKey, dto, barcodeTTL)
		return dto, nil
	}
	dto := toFoodDTO(created)
	s.cache.Set(ctx, cacheKey, dto, barcodeTTL)
	return dto, nil
}

// PhotoEstimate is the stubbed AI photo-estimate response.
type PhotoEstimate struct {
	Title string        `json:"title"`
	Total int           `json:"total"`
	Items []EstimateRow `json:"items"`
}

type EstimateRow struct {
	Name string `json:"name"`
	Qty  string `json:"qty"`
	Kcal int    `json:"kcal"`
	P    int    `json:"p"`
	C    int    `json:"c"`
	F    int    `json:"f"`
}

// EstimatePhoto returns a stubbed plate estimate.
//
// TODO: replace with a real vision model call (the request would carry an image
// URL or upload reference).
func (s *Service) EstimatePhoto(_ context.Context) PhotoEstimate {
	return PhotoEstimate{
		Title: "Grilled salmon plate",
		Total: 612,
		Items: []EstimateRow{
			{Name: "Salmon fillet", Qty: "~180g", Kcal: 367, P: 40, C: 0, F: 22},
			{Name: "Roasted potatoes", Qty: "~140g", Kcal: 162, P: 3, C: 30, F: 4},
			{Name: "Asparagus", Qty: "~90g", Kcal: 28, P: 3, C: 5, F: 0},
			{Name: "Olive oil drizzle", Qty: "~6g", Kcal: 55, P: 0, C: 0, F: 6},
		},
	}
}

func toFoodDTO(f store.Food) FoodDTO {
	return FoodDTO{
		ID:       f.ID.String(),
		Name:     f.Name,
		Serving:  f.Serving,
		Kcal:     f.Kcal,
		ProteinG: store.Float(f.ProteinG),
		CarbsG:   store.Float(f.CarbsG),
		FatG:     store.Float(f.FatG),
		Barcode:  f.Barcode,
	}
}
