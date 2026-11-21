package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testMCPToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestMCPTokenMatcher(t *testing.T) {
	if NewMCPTokenMatcher("") != nil {
		t.Error("empty configured token should disable MCP")
	}
	if NewMCPTokenMatcher(strings.Repeat("a", 42)) != nil {
		t.Error("42-char configured token should disable MCP")
	}

	matches := NewMCPTokenMatcher(testMCPToken)
	if matches == nil {
		t.Fatal("matcher is nil for a valid token")
	}
	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{"same", testMCPToken, true},
		{"plus one char", testMCPToken + "a", false},
		{"minus last char", testMCPToken[:len(testMCPToken)-1], false},
		{"empty", "", false},
		{"upper case", strings.ToUpper(testMCPToken), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matches(tt.token); got != tt.want {
				t.Errorf("matches(%q) = %v, want %v", tt.token, got, tt.want)
			}
		})
	}
}

func TestRequireMCPToken(t *testing.T) {
	short := strings.Repeat("a", 42)
	tests := []struct {
		name       string
		matcher    func(string) bool
		authHeader string
		wantStatus int
	}{
		{"no header", NewMCPTokenMatcher(testMCPToken), "", http.StatusUnauthorized},
		{"basic", NewMCPTokenMatcher(testMCPToken), "Basic x", http.StatusUnauthorized},
		{"empty bearer", NewMCPTokenMatcher(testMCPToken), "Bearer ", http.StatusUnauthorized},
		{"wrong token", NewMCPTokenMatcher(testMCPToken), "Bearer nope", http.StatusUnauthorized},
		{"short configured", NewMCPTokenMatcher(short), "Bearer " + short, http.StatusUnauthorized},
		{"right token", NewMCPTokenMatcher(testMCPToken), "Bearer " + testMCPToken, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := requireMCPToken(tt.matcher, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			h.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus != http.StatusUnauthorized {
				return
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q", ct)
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body not JSON: %v", err)
			}
			if body["error"] != "Invalid token" || len(body) != 1 {
				t.Errorf("body = %v", body)
			}
		})
	}
}
