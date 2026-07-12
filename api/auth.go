package main

import (
	"context"
	"net/http"
	"strings"
)

// TokenVerifier verifies a bearer credential and returns the caller's uid.
type TokenVerifier interface {
	Verify(ctx context.Context, idToken string) (string, error)
}

type ctxKey string

const uidCtxKey ctxKey = "uid"

func withUID(ctx context.Context, uid string) context.Context {
	return context.WithValue(ctx, uidCtxKey, uid)
}

func uidFromContext(ctx context.Context) (string, bool) {
	uid, ok := ctx.Value(uidCtxKey).(string)
	return uid, ok
}

// requireAuth wraps a handler, requiring a valid "Authorization: Bearer <token>"
// header. On success it injects the verified uid into the request context.
func requireAuth(v TokenVerifier, next http.HandlerFunc) http.HandlerFunc {
	const prefix = "Bearer "
	return func(w http.ResponseWriter, r *http.Request) {
		authz := r.Header.Get("Authorization")
		if !strings.HasPrefix(authz, prefix) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		idToken := strings.TrimSpace(strings.TrimPrefix(authz, prefix))
		if idToken == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		uid, err := v.Verify(r.Context(), idToken)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r.WithContext(withUID(r.Context(), uid)))
	}
}
