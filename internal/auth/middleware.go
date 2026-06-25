package auth

import (
	"net/http"
	"strings"

	"github.com/grandmastr/hybreed-go/internal/httpx"
)

const bearerPrefix = "Bearer "

// Authenticator returns middleware that requires a valid Bearer access token and
// injects the caller's identity into the request context.
func Authenticator(tm *TokenManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, bearerPrefix) {
				httpx.Error(w, httpx.ErrUnauthorized("missing bearer token"))
				return
			}
			id, email, err := tm.Parse(strings.TrimSpace(strings.TrimPrefix(header, bearerPrefix)))
			if err != nil {
				httpx.Error(w, httpx.ErrUnauthorized("invalid or expired token"))
				return
			}
			ctx := httpx.WithIdentity(r.Context(), httpx.Identity{ID: id, Email: email})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
