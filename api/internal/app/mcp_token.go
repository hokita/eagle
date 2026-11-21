package app

import (
	"crypto/sha256"
	"crypto/subtle"
	"log"
	"net/http"
	"strings"
)

const minMCPTokenLength = 43

func NewMCPTokenMatcher(configured string) func(token string) bool {
	if configured == "" {
		return nil
	}
	if len(configured) < minMCPTokenLength {
		log.Printf("MCP_TOKEN is shorter than %d characters; MCP is disabled", minMCPTokenLength)
		return nil
	}
	want := sha256.Sum256([]byte(configured))
	return func(token string) bool {
		got := sha256.Sum256([]byte(token))
		return subtle.ConstantTimeCompare(got[:], want[:]) == 1
	}
}

func requireMCPToken(matches func(string) bool, next http.Handler) http.Handler {
	const prefix = "Bearer "
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token := ""
		if strings.HasPrefix(header, prefix) {
			token = strings.TrimSpace(header[len(prefix):])
		}
		if matches == nil || token == "" || !matches(token) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"Invalid token"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
