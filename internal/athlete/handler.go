package athlete

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/grandmastr/hybreed-go/internal/httpx"
	"github.com/grandmastr/hybreed-go/internal/store"
)

// Handler exposes the athlete HTTP API under /me.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler builds the athlete handler.
func NewHandler(svc *Service, log *slog.Logger) *Handler { return &Handler{svc: svc, log: log} }

// Routes mounts the /me endpoints (expects to run behind auth middleware).
func (h *Handler) Routes(r chi.Router) {
	r.Route("/me", func(mr chi.Router) {
		mr.Get("/profile", h.getProfile)
		mr.Patch("/profile", h.updateProfile)
		mr.Get("/settings", h.getSettings)
		mr.Patch("/settings", h.updateSettings)
		mr.Get("/stats", h.getStats)
		mr.Get("/prs", h.listPRs)
		mr.Post("/prs", h.createPR)
		mr.Delete("/prs/{id}", h.deletePR)
		mr.Post("/push-tokens", h.registerPushToken)
		mr.Delete("/push-tokens", h.unregisterPushToken)
		mr.Get("/notification-prefs", h.getNotificationPrefs)
		mr.Put("/notification-prefs", h.putNotificationPrefs)
		mr.Post("/push-tokens/test", h.testPush)
	})
}

func (h *Handler) getProfile(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	profile, err := h.svc.GetProfile(r.Context(), id.ID)
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, profile)
}

type updateProfileRequest struct {
	Name       *string `json:"name"`
	Handle     *string `json:"handle"`
	Status     *string `json:"status"`
	LoadTarget *int32  `json:"loadTarget"`
	Dob        *string `json:"dob"`
}

func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	var req updateProfileRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	var dob pgtype.Date
	if req.Dob != nil {
		t, err := time.Parse("2006-01-02", *req.Dob)
		if err != nil {
			httpx.Error(w, httpx.ErrValidation("dob must be YYYY-MM-DD"))
			return
		}
		dob = pgtype.Date{Time: t, Valid: true}
	}
	profile, err := h.svc.UpdateProfile(r.Context(), id.ID, store.UpdateUserProfileParams{
		Name:       req.Name,
		Handle:     req.Handle,
		Status:     req.Status,
		LoadTarget: req.LoadTarget,
		Dob:        dob,
	})
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, profile)
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	settings, err := h.svc.GetSettings(r.Context(), id.ID)
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, settings)
}

type updateSettingsRequest struct {
	Units         *string  `json:"units"`
	Notifications *bool    `json:"notifications"`
	ConnectedApps *int32   `json:"connectedApps"`
	BodyWeightKg  *float64 `json:"bodyWeightKg"`
	Goals         *Goals   `json:"goals"`
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	var req updateSettingsRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	var bodyWeight pgtype.Numeric
	if req.BodyWeightKg != nil {
		bodyWeight = store.Num(*req.BodyWeightKg)
	}
	var goals []byte
	if req.Goals != nil {
		b, err := json.Marshal(req.Goals)
		if err != nil {
			httpx.Error(w, httpx.ErrBadRequest("invalid goals"))
			return
		}
		goals = b
	}
	settings, err := h.svc.UpdateSettings(r.Context(), id.ID, store.UpdateUserSettingsParams{
		Units:         req.Units,
		Notifications: req.Notifications,
		ConnectedApps: req.ConnectedApps,
		BodyWeightKg:  bodyWeight,
		Goals:         goals,
	})
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, settings)
}

func (h *Handler) getStats(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	stats, err := h.svc.GetStats(r.Context(), id.ID)
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, stats)
}

func (h *Handler) listPRs(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	prs, err := h.svc.ListPRs(r.Context(), id.ID)
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": prs})
}

type createPRRequest struct {
	Label    string `json:"label"`
	Value    string `json:"value"`
	Icon     string `json:"icon"`
	Position int32  `json:"position"`
}

func (h *Handler) createPR(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	var req createPRRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	if req.Label == "" || req.Value == "" {
		httpx.Error(w, httpx.ErrValidation("label and value are required"))
		return
	}
	if req.Icon == "" {
		req.Icon = "bolt"
	}
	pr, err := h.svc.CreatePR(r.Context(), store.CreatePersonalRecordParams{
		UserID:   id.ID,
		Label:    req.Label,
		Value:    req.Value,
		Icon:     req.Icon,
		Position: req.Position,
	})
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, pr)
}

func (h *Handler) deletePR(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	prID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, httpx.ErrBadRequest("invalid id"))
		return
	}
	if err := h.svc.DeletePR(r.Context(), id.ID, prID); err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.NoContent(w)
}

type pushTokenRequest struct {
	Token    string `json:"token"`
	Platform string `json:"platform"`
}

func (h *Handler) registerPushToken(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	var req pushTokenRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	if req.Token == "" {
		httpx.Error(w, httpx.ErrValidation("token is required"))
		return
	}
	if req.Platform == "" {
		req.Platform = "ios"
	}
	if err := h.svc.RegisterDevice(r.Context(), id.ID, req.Token, req.Platform); err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"token": req.Token, "platform": req.Platform})
}

type unregisterPushTokenRequest struct {
	Token string `json:"token"`
}

func (h *Handler) unregisterPushToken(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	var req unregisterPushTokenRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	if req.Token == "" {
		httpx.Error(w, httpx.ErrValidation("token is required"))
		return
	}
	if err := h.svc.UnregisterDevice(r.Context(), id.ID, req.Token); err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getNotificationPrefs(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	prefs, err := h.svc.GetNotificationPrefs(r.Context(), id.ID)
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, prefs)
}

func (h *Handler) putNotificationPrefs(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	var req NotifyPrefs
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	saved, err := h.svc.UpdateNotificationPrefs(r.Context(), id.ID, req)
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, saved)
}

func (h *Handler) testPush(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	sent, err := h.svc.TestPush(r.Context(), id.ID)
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	if sent == 0 {
		httpx.Error(w, httpx.ErrBadRequest("no registered devices"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"sent": sent})
}
