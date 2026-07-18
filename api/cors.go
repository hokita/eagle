package main

import (
	"net/http"
	"strings"
)

// withCORS wraps a handler with CORS headers. allowedOrigins is a
// comma-separated list of exact origins permitted to call the API (Firebase
// Hosting serves the same site on both {project}.web.app and
// {project}.firebaseapp.com, so both must be listed). If allowedOrigins is
// empty (matching corgi's dev-only behavior when FRONTEND_URL is unset), any
// origin is allowed. Otherwise, only a request whose Origin header exactly
// matches one of the listed origins gets it reflected back — echoing a
// fixed origin regardless of the request would make the browser reject the
// real request after a seemingly-successful preflight.
// OPTIONS preflight requests are answered directly without invoking next,
// since a preflight has no Authorization header and must not hit auth.
func withCORS(allowedOrigins string, next http.HandlerFunc) http.HandlerFunc {
	if allowedOrigins == "" {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next(w, r)
		}
	}

	allowed := map[string]bool{}
	for _, origin := range strings.Split(allowedOrigins, ",") {
		allowed[strings.TrimSpace(origin)] = true
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}
