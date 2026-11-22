package app

import (
	"context"
	"net/http"
	"slices"
	"strings"
)

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

func requireAuth(v TokenVerifier, allowedEmails []string, next http.HandlerFunc) http.HandlerFunc {
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
		if email == "" || !slices.Contains(allowedEmails, email) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r.WithContext(withUID(r.Context(), uid)))
	}
}
