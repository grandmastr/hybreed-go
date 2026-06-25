package auth

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/grandmastr/hybreed-go/internal/httpx"
)

// Handler exposes the auth HTTP API.
type Handler struct {
	svc    *Service
	log    *slog.Logger
	authMW func(http.Handler) http.Handler
}

// NewHandler builds the auth handler. authMW protects the /me route.
func NewHandler(svc *Service, log *slog.Logger, authMW func(http.Handler) http.Handler) *Handler {
	return &Handler{svc: svc, log: log, authMW: authMW}
}

// Routes mounts the auth endpoints onto r (expected base: /auth).
func (h *Handler) Routes(r chi.Router) {
	r.Post("/register", h.register)
	r.Post("/verify", h.verify)
	r.Post("/resend", h.resend)
	r.Post("/login", h.login)
	r.Post("/refresh", h.refresh)
	r.Post("/logout", h.logout)
	r.Post("/social", h.social)
	r.Post("/forgot", h.forgot)
	r.Post("/reset", h.reset)

	r.Group(func(pr chi.Router) {
		pr.Use(h.authMW)
		pr.Get("/me", h.me)
	})
}

type registerRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	res, err := h.svc.Register(r.Context(), req.Name, req.Email, req.Password)
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, res)
}

type verifyRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

func (h *Handler) verify(w http.ResponseWriter, r *http.Request) {
	var req verifyRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	session, err := h.svc.VerifyEmail(r.Context(), req.Email, req.Code, r.UserAgent())
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, session)
}

type emailRequest struct {
	Email string `json:"email"`
}

func (h *Handler) resend(w http.ResponseWriter, r *http.Request) {
	var req emailRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	if err := h.svc.ResendOTP(r.Context(), req.Email); err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"sent": true})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	session, err := h.svc.Login(r.Context(), req.Email, req.Password, r.UserAgent())
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, session)
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	session, err := h.svc.Refresh(r.Context(), req.RefreshToken, r.UserAgent())
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, session)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	if err := h.svc.Logout(r.Context(), req.RefreshToken); err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.NoContent(w)
}

type socialRequest struct {
	Provider string `json:"provider"`
	Email    string `json:"email"`
	Name     string `json:"name"`
}

func (h *Handler) social(w http.ResponseWriter, r *http.Request) {
	var req socialRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	session, err := h.svc.Social(r.Context(), SocialInput(req), r.UserAgent())
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, session)
}

func (h *Handler) forgot(w http.ResponseWriter, r *http.Request) {
	var req emailRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	if err := h.svc.RequestPasswordReset(r.Context(), req.Email); err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"sent": true})
}

type resetRequest struct {
	Email    string `json:"email"`
	Code     string `json:"code"`
	Password string `json:"password"`
}

func (h *Handler) reset(w http.ResponseWriter, r *http.Request) {
	var req resetRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	session, err := h.svc.ResetPassword(r.Context(), req.Email, req.Code, req.Password, r.UserAgent())
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, session)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	id := httpx.MustIdentity(r.Context())
	user, err := h.svc.Me(r.Context(), id.ID)
	if err != nil {
		httpx.WriteError(w, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, user)
}
