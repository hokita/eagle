package app

import (
	"context"
	"net/http"
	"strings"
)

// TokenVerifier verifies a bearer credential and returns the caller's uid and
// verified email address.
type TokenVerifier interface {
	Verify(ctx context.Context, idToken string) (uid string, email string, err error)
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
// header whose verified email matches allowedEmail. On success it injects the
// verified uid into the request context.
//
// This is a single-user allowlist (one exact email, not a list) matching the
// ALLOWED_EMAIL convention used by the corgi project. A disallowed or missing
// email is rejected with 401 (not 403), matching corgi's authMiddleware, so
// the response doesn't reveal whether the token was invalid or just the
// wrong account.
func requireAuth(v TokenVerifier, allowedEmail string, next http.HandlerFunc) http.HandlerFunc {
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
		uid, email, err := v.Verify(r.Context(), idToken)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if email == "" || email != allowedEmail {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r.WithContext(withUID(r.Context(), uid)))
	}
}
