// Package home serves the "Today" tab: an aggregate of the training-load ring,
// the nutrition brief, the daily plan, and the unified activity/meal timeline.
package home

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/grandmastr/hybreed-go/internal/httpx"
	"github.com/grandmastr/hybreed-go/internal/nutrition"
	"github.com/grandmastr/hybreed-go/internal/training"
)

// Service composes the training and nutrition services into the Today payload.
type Service struct {
	training  *training.Service
	nutrition *nutrition.Service
	log       *slog.Logger
}

// NewService builds the home service.
func NewService(t *training.Service, n *nutrition.Service, log *slog.Logger) *Service {
	return &Service{training: t, nutrition: n, log: log}
}

// ── DTOs ────────────────────────────────────────────────────────────────────

type NutritionBrief struct {
	Budget        int64            `json:"budget"`
	Base          int32            `json:"base"`
	TrainingBonus int32            `json:"trainingBonus"`
	Consumed      int64            `json:"consumed"`
	Remaining     int64            `json:"remaining"`
	Macros        nutrition.Macros `json:"macros"`
	Water         nutrition.Water  `json:"water"`
}

type TimelineItem struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Time    string `json:"time"`
	Title   string `json:"title"`
	Meta    string `json:"meta"`
	Load    *int32 `json:"load,omitempty"`
	Kcal    *int64 `json:"kcal,omitempty"`
	Planned bool   `json:"planned"`
}

type TodayDTO struct {
	Date      string               `json:"date"`
	Load      training.LoadSummary `json:"load"`
	Nutrition NutritionBrief       `json:"nutrition"`
	Plan      []training.PlanDTO   `json:"plan"`
	Timeline  []TimelineItem       `json:"timeline"`
}

// GetToday assembles the Today-tab payload.
func (s *Service) GetToday(ctx context.Context, userID uuid.UUID) (TodayDTO, error) {
	now := time.Now().UTC()

	load, err := s.training.GetLoadSummary(ctx, userID)
	if err != nil {
		return TodayDTO{}, err
	}
	nut, err := s.nutrition.GetSummary(ctx, userID, now)
	if err != nil {
		return TodayDTO{}, err
	}
	plan, err := s.training.ListPlan(ctx, userID, now)
	if err != nil {
		return TodayDTO{}, err
	}
	activities, err := s.training.ListActivities(ctx, userID, nil, 50, 0)
	if err != nil {
		return TodayDTO{}, err
	}

	return TodayDTO{
		Date:      now.Format("2006-01-02"),
		Load:      load,
		Nutrition: NutritionBrief{Budget: nut.Budget, Base: nut.Base, TrainingBonus: nut.TrainingBonus, Consumed: nut.Consumed, Remaining: nut.Remaining, Macros: nut.Macros, Water: nut.Water},
		Plan:      plan,
		Timeline:  buildTimeline(activities, nut.Meals, now),
	}, nil
}

func buildTimeline(activities []training.ActivityDTO, meals []nutrition.MealDTO, now time.Time) []TimelineItem {
	items := make([]TimelineItem, 0, len(activities)+len(meals))

	for _, a := range activities {
		if !sameDay(a.PerformedAt, now) {
			continue
		}
		meta := ""
		switch {
		case a.Run != nil:
			meta = fmt.Sprintf("%.1f km · %s · %s/km", a.Run.DistanceKm, a.Run.Duration, a.Run.Pace)
		case a.Lift != nil:
			meta = fmt.Sprintf("%d sets · %d kg", a.Lift.TotalSets, a.Lift.TotalVolumeKg)
		}
		load := a.Load
		items = append(items, TimelineItem{
			ID:      a.ID,
			Kind:    a.Kind,
			Time:    a.PerformedAt.UTC().Format("15:04"),
			Title:   a.Title,
			Meta:    meta,
			Load:    &load,
			Planned: a.Planned,
		})
	}

	for _, m := range meals {
		if len(m.Items) == 0 && !m.Planned {
			continue
		}
		kcal := m.Kcal
		items = append(items, TimelineItem{
			ID:      m.ID,
			Kind:    "meal",
			Time:    m.Time,
			Title:   m.Slot,
			Meta:    fmt.Sprintf("%d kcal", m.Kcal),
			Kcal:    &kcal,
			Planned: m.Planned,
		})
	}

	sort.SliceStable(items, func(i, j int) bool { return items[i].Time < items[j].Time })
	return items
}

func sameDay(a, b time.Time) bool {
	a, b = a.UTC(), b.UTC()
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

// ── Handler ─────────────────────────────────────────────────────────────────

// Handler exposes the home HTTP API.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler builds the home handler.
func NewHandler(svc *Service, log *slog.Logger) *Handler { return &Handler{svc: svc, log: log} }

// Routes mounts the home endpoints (behind auth middleware).
func (h *Handler) Routes(r chi.Router) {
	r.Get("/home/today", h.today)
}

func (h *Handler) today(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	today, err := h.svc.GetToday(r.Context(), id.ID)
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, today)
}
