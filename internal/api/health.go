package api

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/grandmastr/hybreed-go/internal/cache"
	"github.com/grandmastr/hybreed-go/internal/httpx"
)

type health struct {
	pool  *pgxpool.Pool
	cache *cache.Cache
}

// live is a liveness probe — the process is up.
func (h health) live(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ready is a readiness probe — dependencies (Postgres, Redis) are reachable.
func (h health) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	checks := map[string]string{"postgres": "ok", "redis": "ok"}
	ready := true

	if err := h.pool.Ping(ctx); err != nil {
		checks["postgres"] = "unavailable"
		ready = false
	}
	if err := h.cache.Ping(ctx); err != nil {
		checks["redis"] = "unavailable"
		ready = false
	}

	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}
	httpx.JSON(w, status, map[string]any{"ready": ready, "checks": checks})
}
