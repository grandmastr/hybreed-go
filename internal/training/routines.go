package training

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/grandmastr/hybreed-go/internal/httpx"
	"github.com/grandmastr/hybreed-go/internal/store"
)

// ── DTOs ──────────────────────────────────────────────────────────────────

// RoutineExerciseDTO is one exercise in a routine, with how many sets to target.
type RoutineExerciseDTO struct {
	Name       string `json:"name"`
	Note       string `json:"note"`
	TargetSets int32  `json:"targetSets"`
}

// RoutineDTO is a reusable workout template.
type RoutineDTO struct {
	ID        string               `json:"id"`
	Name      string               `json:"name"`
	Note      string               `json:"note"`
	Exercises []RoutineExerciseDTO `json:"exercises"`
}

// RoutineInput is the validated payload for creating/updating a routine.
type RoutineInput struct {
	Name      string
	Note      string
	Exercises []RoutineExerciseDTO
}

// ── Service ───────────────────────────────────────────────────────────────

// ListRoutines returns the user's routines (each with its exercises).
func (s *Service) ListRoutines(ctx context.Context, userID uuid.UUID) ([]RoutineDTO, error) {
	rows, err := s.q.ListRoutines(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list routines: %w", err)
	}
	out := make([]RoutineDTO, 0, len(rows))
	for _, r := range rows {
		dto, err := s.routineDTO(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, dto)
	}
	return out, nil
}

// GetRoutine returns a single routine.
func (s *Service) GetRoutine(ctx context.Context, userID, id uuid.UUID) (RoutineDTO, error) {
	r, err := s.q.GetRoutine(ctx, store.GetRoutineParams{ID: id, UserID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RoutineDTO{}, httpx.ErrNotFound("routine not found")
		}
		return RoutineDTO{}, fmt.Errorf("get routine: %w", err)
	}
	return s.routineDTO(ctx, r)
}

// CreateRoutine persists a new routine with its exercises in one transaction.
func (s *Service) CreateRoutine(ctx context.Context, userID uuid.UUID, in RoutineInput) (RoutineDTO, error) {
	if in.Name == "" {
		return RoutineDTO{}, httpx.ErrValidation("routine name is required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RoutineDTO{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit
	qtx := s.q.WithTx(tx)

	routine, err := qtx.CreateRoutine(ctx, store.CreateRoutineParams{UserID: userID, Name: in.Name, Note: in.Note, Position: 0})
	if err != nil {
		return RoutineDTO{}, fmt.Errorf("create routine: %w", err)
	}
	if err := insertRoutineExercises(ctx, qtx, routine.ID, in.Exercises); err != nil {
		return RoutineDTO{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RoutineDTO{}, fmt.Errorf("commit: %w", err)
	}
	return s.routineDTO(ctx, routine)
}

// UpdateRoutine renames a routine and replaces its exercise list.
func (s *Service) UpdateRoutine(ctx context.Context, userID, id uuid.UUID, in RoutineInput) (RoutineDTO, error) {
	if in.Name == "" {
		return RoutineDTO{}, httpx.ErrValidation("routine name is required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RoutineDTO{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit
	qtx := s.q.WithTx(tx)

	routine, err := qtx.UpdateRoutine(ctx, store.UpdateRoutineParams{ID: id, UserID: userID, Name: in.Name, Note: in.Note})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RoutineDTO{}, httpx.ErrNotFound("routine not found")
		}
		return RoutineDTO{}, fmt.Errorf("update routine: %w", err)
	}
	if err := qtx.DeleteRoutineExercises(ctx, routine.ID); err != nil {
		return RoutineDTO{}, fmt.Errorf("clear routine exercises: %w", err)
	}
	if err := insertRoutineExercises(ctx, qtx, routine.ID, in.Exercises); err != nil {
		return RoutineDTO{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RoutineDTO{}, fmt.Errorf("commit: %w", err)
	}
	return s.routineDTO(ctx, routine)
}

// DeleteRoutine removes a routine (cascades to its exercises).
func (s *Service) DeleteRoutine(ctx context.Context, userID, id uuid.UUID) error {
	if err := s.q.DeleteRoutine(ctx, store.DeleteRoutineParams{ID: id, UserID: userID}); err != nil {
		return fmt.Errorf("delete routine: %w", err)
	}
	return nil
}

func insertRoutineExercises(ctx context.Context, q store.Querier, routineID uuid.UUID, exercises []RoutineExerciseDTO) error {
	for i, e := range exercises {
		sets := e.TargetSets
		if sets < 1 {
			sets = 1
		}
		if _, err := q.CreateRoutineExercise(ctx, store.CreateRoutineExerciseParams{
			RoutineID: routineID, Name: e.Name, Note: e.Note, TargetSets: sets, Position: int32(i),
		}); err != nil {
			return fmt.Errorf("create routine exercise: %w", err)
		}
	}
	return nil
}

func (s *Service) routineDTO(ctx context.Context, r store.Routine) (RoutineDTO, error) {
	exs, err := s.q.ListRoutineExercises(ctx, r.ID)
	if err != nil {
		return RoutineDTO{}, fmt.Errorf("list routine exercises: %w", err)
	}
	dto := RoutineDTO{ID: r.ID.String(), Name: r.Name, Note: r.Note, Exercises: make([]RoutineExerciseDTO, 0, len(exs))}
	for _, e := range exs {
		dto.Exercises = append(dto.Exercises, RoutineExerciseDTO{Name: e.Name, Note: e.Note, TargetSets: e.TargetSets})
	}
	return dto, nil
}

// ── HTTP handlers ─────────────────────────────────────────────────────────

type routineRequest struct {
	Name      string               `json:"name"`
	Note      string               `json:"note"`
	Exercises []RoutineExerciseDTO `json:"exercises"`
}

func (req routineRequest) toInput() RoutineInput {
	return RoutineInput{Name: req.Name, Note: req.Note, Exercises: req.Exercises}
}

func (h *Handler) listRoutines(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	items, err := h.svc.ListRoutines(r.Context(), id.ID)
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) getRoutine(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	rid, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, httpx.ErrBadRequest("invalid id"))
		return
	}
	routine, err := h.svc.GetRoutine(r.Context(), id.ID, rid)
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, routine)
}

func (h *Handler) createRoutine(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	var req routineRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	routine, err := h.svc.CreateRoutine(r.Context(), id.ID, req.toInput())
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, routine)
}

func (h *Handler) updateRoutine(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	rid, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, httpx.ErrBadRequest("invalid id"))
		return
	}
	var req routineRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	routine, err := h.svc.UpdateRoutine(r.Context(), id.ID, rid, req.toInput())
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, routine)
}

func (h *Handler) deleteRoutine(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	rid, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, httpx.ErrBadRequest("invalid id"))
		return
	}
	if err := h.svc.DeleteRoutine(r.Context(), id.ID, rid); err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.NoContent(w)
}
