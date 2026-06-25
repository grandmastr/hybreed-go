package training

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/grandmastr/hybreed-go/internal/httpx"
	"github.com/grandmastr/hybreed-go/internal/store"
)

// Handler exposes the training HTTP API.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler builds the training handler.
func NewHandler(svc *Service, log *slog.Logger) *Handler { return &Handler{svc: svc, log: log} }

// Routes mounts the training endpoints (expects to run behind auth middleware).
func (h *Handler) Routes(r chi.Router) {
	r.Route("/activities", func(ar chi.Router) {
		ar.Get("/", h.listActivities)
		ar.Post("/", h.createActivity)
		ar.Get("/{id}", h.getActivity)
		ar.Delete("/{id}", h.deleteActivity)
	})
	r.Route("/training", func(tr chi.Router) {
		tr.Get("/load", h.getLoad)
		tr.Get("/plan", h.listPlan)
		tr.Post("/plan", h.createPlan)
		tr.Patch("/plan/{id}", h.setPlanDone)
		tr.Delete("/plan/{id}", h.deletePlan)
	})
}

func (h *Handler) listActivities(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())

	var kind *string
	if k := r.URL.Query().Get("kind"); k == "run" || k == "lift" {
		kind = &k
	}
	limit := queryInt(r, "limit", 20, 1, 100)
	offset := queryInt(r, "offset", 0, 0, 1_000_000)

	activities, err := h.svc.ListActivities(r.Context(), id.ID, kind, int32(limit), int32(offset))
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": activities})
}

func (h *Handler) getActivity(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	activityID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, httpx.ErrBadRequest("invalid id"))
		return
	}
	activity, err := h.svc.GetActivity(r.Context(), id.ID, activityID)
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, activity)
}

type splitReq struct {
	Km    int32 `json:"km"`
	PaceS int32 `json:"paceS"`
	Hr    int32 `json:"hr"`
}

type setReq struct {
	WeightKg float64 `json:"weightKg"`
	Reps     int32   `json:"reps"`
	Done     bool    `json:"done"`
}

type exerciseReq struct {
	Name string   `json:"name"`
	Note string   `json:"note"`
	Sets []setReq `json:"sets"`
}

type createActivityRequest struct {
	Kind        string     `json:"kind"`
	Title       string     `json:"title"`
	PerformedAt *time.Time `json:"performedAt"`
	Load        int32      `json:"load"`
	Planned     bool       `json:"planned"`
	Notes       string     `json:"notes"`
	Run         *struct {
		DistanceM     int32      `json:"distanceM"`
		DurationS     int32      `json:"durationS"`
		AvgPaceSPerKm int32      `json:"avgPaceSPerKm"`
		AvgHr         int32      `json:"avgHr"`
		Calories      int32      `json:"calories"`
		Splits        []splitReq `json:"splits"`
	} `json:"run"`
	Lift *struct {
		Exercises []exerciseReq `json:"exercises"`
	} `json:"lift"`
}

func (h *Handler) createActivity(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	var req createActivityRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}

	in := CreateActivityInput{
		Kind:    req.Kind,
		Title:   req.Title,
		Load:    req.Load,
		Planned: req.Planned,
		Notes:   req.Notes,
	}
	if req.PerformedAt != nil {
		in.PerformedAt = *req.PerformedAt
	}
	if req.Run != nil {
		run := &RunInput{
			DistanceM:     req.Run.DistanceM,
			DurationS:     req.Run.DurationS,
			AvgPaceSPerKm: req.Run.AvgPaceSPerKm,
			AvgHr:         req.Run.AvgHr,
			Calories:      req.Run.Calories,
		}
		for _, sp := range req.Run.Splits {
			run.Splits = append(run.Splits, SplitInput(sp))
		}
		in.Run = run
	}
	if req.Lift != nil {
		lift := &LiftInput{}
		for _, ex := range req.Lift.Exercises {
			ei := ExerciseInput{Name: ex.Name, Note: ex.Note}
			for _, set := range ex.Sets {
				ei.Sets = append(ei.Sets, SetInput(set))
			}
			lift.Exercises = append(lift.Exercises, ei)
		}
		in.Lift = lift
	}

	activity, err := h.svc.CreateActivity(r.Context(), id.ID, in)
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, activity)
}

func (h *Handler) deleteActivity(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	activityID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, httpx.ErrBadRequest("invalid id"))
		return
	}
	if err := h.svc.DeleteActivity(r.Context(), id.ID, activityID); err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) getLoad(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	summary, err := h.svc.GetLoadSummary(r.Context(), id.ID)
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, summary)
}

func (h *Handler) listPlan(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	plan, err := h.svc.ListPlan(r.Context(), id.ID, queryDate(r, "date"))
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": plan})
}

type createPlanRequest struct {
	Kind     string     `json:"kind"`
	Title    string     `json:"title"`
	Meta     string     `json:"meta"`
	Date     *time.Time `json:"date"`
	Position int32      `json:"position"`
}

func (h *Handler) createPlan(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	var req createPlanRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	if req.Title == "" {
		httpx.Error(w, httpx.ErrValidation("title is required"))
		return
	}
	if req.Kind != "run" && req.Kind != "lift" && req.Kind != "meal" {
		httpx.Error(w, httpx.ErrValidation("kind must be run, lift or meal"))
		return
	}
	day := time.Now().UTC()
	if req.Date != nil {
		day = *req.Date
	}
	item, err := h.svc.CreatePlanItem(r.Context(), store.CreatePlanItemParams{
		UserID:   id.ID,
		PlanDate: store.DateOf(day),
		Kind:     req.Kind,
		Title:    req.Title,
		Meta:     req.Meta,
		Position: req.Position,
	})
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, item)
}

type setPlanDoneRequest struct {
	Done bool `json:"done"`
}

func (h *Handler) setPlanDone(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	planID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, httpx.ErrBadRequest("invalid id"))
		return
	}
	var req setPlanDoneRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	item, err := h.svc.SetPlanDone(r.Context(), id.ID, planID, req.Done)
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, item)
}

func (h *Handler) deletePlan(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	planID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, httpx.ErrBadRequest("invalid id"))
		return
	}
	if err := h.svc.DeletePlanItem(r.Context(), id.ID, planID); err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.NoContent(w)
}

// ── query helpers ───────────────────────────────────────────────────────────

func queryInt(r *http.Request, key string, def, min, max int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

func queryDate(r *http.Request, key string) time.Time {
	v := r.URL.Query().Get(key)
	if v == "" {
		return time.Now().UTC()
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return time.Now().UTC()
	}
	return t
}
