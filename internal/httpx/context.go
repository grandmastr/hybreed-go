package httpx

import (
	"context"

	"github.com/google/uuid"
)

// Identity is the authenticated caller, derived from a verified access token and
// injected by the auth middleware.
type Identity struct {
	ID    uuid.UUID
	Email string
}

type ctxKey int

const identityKey ctxKey = iota

// WithIdentity returns a copy of ctx carrying the authenticated identity.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}

// IdentityFrom extracts the authenticated identity from ctx.
func IdentityFrom(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey).(Identity)
	return id, ok
}

// MustIdentity returns the identity or panics — only safe behind auth middleware,
// which guarantees its presence.
func MustIdentity(ctx context.Context) Identity {
	id, ok := IdentityFrom(ctx)
	if !ok {
		panic("httpx: no identity in context (missing auth middleware?)")
	}
	return id
}
