// Package api assembles the HTTP router: middleware, health probes and the
// versioned /v1 surface mounted from each domain handler.
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/grandmastr/hybreed-go/internal/athlete"
	"github.com/grandmastr/hybreed-go/internal/auth"
	"github.com/grandmastr/hybreed-go/internal/cache"
	"github.com/grandmastr/hybreed-go/internal/config"
	"github.com/grandmastr/hybreed-go/internal/home"
	"github.com/grandmastr/hybreed-go/internal/httpx"
	"github.com/grandmastr/hybreed-go/internal/nutrition"
	"github.com/grandmastr/hybreed-go/internal/training"
)

// Version is the API version, surfaced on the root endpoint (overridable at build time).
var Version = "dev"

// Deps are everything NewRouter needs to wire the HTTP surface.
type Deps struct {
	Config    config.Config
	Logger    *slog.Logger
	Pool      *pgxpool.Pool
	Cache     *cache.Cache
	AuthMW    func(http.Handler) http.Handler
	Auth      *auth.Handler
	Athlete   *athlete.Handler
	Training  *training.Handler
	Nutrition *nutrition.Handler
	Home      *home.Handler
}

// NewRouter builds the application HTTP handler.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(d.Logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   d.Config.CORSAllowedOrigins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		ExposedHeaders:   []string{"X-Request-Id"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	h := health{pool: d.Pool, cache: d.Cache}
	r.Get("/healthz", h.live)
	r.Get("/readyz", h.ready)
	r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{
			"name":    "hybreed-api",
			"version": Version,
			"status":  "ok",
		})
	})

	r.Route("/v1", func(v chi.Router) {
		v.Route("/auth", d.Auth.Routes)

		v.Group(func(pr chi.Router) {
			pr.Use(d.AuthMW)
			d.Athlete.Routes(pr)
			d.Training.Routes(pr)
			d.Nutrition.Routes(pr)
			d.Home.Routes(pr)
		})
	})

	return r
}
