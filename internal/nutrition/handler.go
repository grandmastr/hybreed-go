package nutrition

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/grandmastr/hybreed-go/internal/httpx"
)

// Handler exposes the nutrition + foods HTTP API.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler builds the nutrition handler.
func NewHandler(svc *Service, log *slog.Logger) *Handler { return &Handler{svc: svc, log: log} }

// Routes mounts the nutrition + foods endpoints (behind auth middleware).
func (h *Handler) Routes(r chi.Router) {
	r.Route("/nutrition", func(nr chi.Router) {
		nr.Get("/summary", h.getSummary)
		nr.Post("/water", h.addWater)
		nr.Post("/meals", h.createMeal)
		nr.Post("/meals/{id}/items", h.addMealItem)
		nr.Delete("/meals/{id}", h.deleteMeal)
	})
	r.Route("/foods", func(fr chi.Router) {
		fr.Get("/", h.searchFoods)
		fr.Get("/barcode/{code}", h.barcode)
		fr.Post("/estimate", h.estimate)
	})
}

func (h *Handler) getSummary(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	summary, err := h.svc.GetSummary(r.Context(), id.ID, queryDate(r))
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, summary)
}

type addWaterRequest struct {
	Ml int32 `json:"ml"`
}

func (h *Handler) addWater(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	var req addWaterRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	water, err := h.svc.AddWater(r.Context(), id.ID, queryDate(r), req.Ml)
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, water)
}

type itemRequest struct {
	Name     string  `json:"name"`
	Kcal     int32   `json:"kcal"`
	ProteinG float64 `json:"p"`
	CarbsG   float64 `json:"c"`
	FatG     float64 `json:"f"`
}

type createMealRequest struct {
	Slot     string        `json:"slot"`
	Date     *time.Time    `json:"date"`
	Planned  bool          `json:"planned"`
	LoggedAt *time.Time    `json:"loggedAt"`
	Position int32         `json:"position"`
	Items    []itemRequest `json:"items"`
}

func (h *Handler) createMeal(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	var req createMealRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	in := CreateMealInput{
		Slot:     req.Slot,
		Planned:  req.Planned,
		LoggedAt: req.LoggedAt,
		Position: req.Position,
	}
	if req.Date != nil {
		in.Day = *req.Date
	}
	for _, it := range req.Items {
		in.Items = append(in.Items, ItemInput(it))
	}
	meal, err := h.svc.CreateMeal(r.Context(), id.ID, in)
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, meal)
}

func (h *Handler) addMealItem(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	mealID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, httpx.ErrBadRequest("invalid id"))
		return
	}
	var req itemRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	if req.Name == "" {
		httpx.Error(w, httpx.ErrValidation("name is required"))
		return
	}
	item, err := h.svc.AddMealItem(r.Context(), id.ID, mealID, ItemInput(req))
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, item)
}

func (h *Handler) deleteMeal(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	mealID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, httpx.ErrBadRequest("invalid id"))
		return
	}
	if err := h.svc.DeleteMeal(r.Context(), id.ID, mealID); err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) searchFoods(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	limit := defaultSearchN
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
			limit = n
		}
	}
	foods, err := h.svc.SearchFoods(r.Context(), query, limit)
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": foods})
}

func (h *Handler) barcode(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	food, err := h.svc.GetByBarcode(r.Context(), code)
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, food)
}

func (h *Handler) estimate(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, h.svc.EstimatePhoto(r.Context()))
}

func queryDate(r *http.Request) time.Time {
	v := r.URL.Query().Get("date")
	if v == "" {
		return time.Now().UTC()
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return time.Now().UTC()
	}
	return t
}
