package main

import "net/http"

// withCORS wraps a handler with CORS headers. If allowedOrigin is empty
// (matching corgi's dev-only behavior when FRONTEND_URL is unset), it
// allows any origin; otherwise it restricts to exactly that origin.
// OPTIONS preflight requests are answered directly without invoking next,
// since a preflight has no Authorization header and must not hit auth.
func withCORS(allowedOrigin string, next http.HandlerFunc) http.HandlerFunc {
	origin := allowedOrigin
	if origin == "" {
		origin = "*"
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}
