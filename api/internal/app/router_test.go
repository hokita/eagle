package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type tokenVerifier map[string]string

func (v tokenVerifier) Verify(_ context.Context, token string) (string, string, error) {
	email, ok := v[token]
	if !ok {
		return "", "", errors.New("bad token")
	}
	return "uid-1", email, nil
}

const mcpInitBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`

func muxFor(cfg MCPConfig) http.Handler {
	srv := NewServer(&fakeRepo{}, &fakeExplainer{}, &fakeAnalyzer{})
	verifier := tokenVerifier{"firebase-ok": testAllowedEmail}
	return NewMux(srv, verifier, []string{testAllowedEmail}, "http://localhost:3000", cfg)
}

func doReq(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestMuxMCPAuth(t *testing.T) {
	tokA := strings.Repeat("a", 64)
	tokB := strings.Repeat("b", 64)
	short := strings.Repeat("c", 42)
	cfg := MCPConfig{Token: tokA, OwnerUID: "owner-uid"}
	oversized := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"pad":"` + strings.Repeat("x", 70<<10) + `"}}`

	tests := []struct {
		name   string
		cfg    MCPConfig
		method string
		path   string
		token  string
		body   string
		want   int
	}{
		{"no header", cfg, http.MethodPost, "/mcp", "", mcpInitBody, http.StatusUnauthorized},
		{"wrong token", cfg, http.MethodPost, "/mcp", tokB, mcpInitBody, http.StatusUnauthorized},
		{"short token configured and sent", MCPConfig{Token: short, OwnerUID: "owner-uid"}, http.MethodPost, "/mcp", short, mcpInitBody, http.StatusUnauthorized},
		{"firebase token on mcp", cfg, http.MethodPost, "/mcp", "firebase-ok", mcpInitBody, http.StatusUnauthorized},
		{"right token", cfg, http.MethodPost, "/mcp", tokA, mcpInitBody, http.StatusOK},
		{"right token blank owner", MCPConfig{Token: tokA, OwnerUID: ""}, http.MethodPost, "/mcp", tokA, mcpInitBody, http.StatusUnauthorized},
		{"right token whitespace owner", MCPConfig{Token: tokA, OwnerUID: "  "}, http.MethodPost, "/mcp", tokA, mcpInitBody, http.StatusUnauthorized},
		{"oversized body", cfg, http.MethodPost, "/mcp", tokA, oversized, http.StatusRequestEntityTooLarge},
		{"mcp token on rest", cfg, http.MethodGet, "/api/mistakes", tokA, "", http.StatusUnauthorized},
		{"firebase token on rest", cfg, http.MethodGet, "/api/mistakes", "firebase-ok", "", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doReq(muxFor(tt.cfg), tt.method, tt.path, tt.token, tt.body)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body.String())
			}
			switch {
			case tt.want == http.StatusUnauthorized && tt.path == "/mcp":
				if got := rec.Body.String(); !strings.Contains(got, `"error":"Invalid token"`) {
					t.Fatalf("body = %q", got)
				}
			case tt.name == "right token":
				var resp struct {
					Result struct {
						ServerInfo struct {
							Name string `json:"name"`
						} `json:"serverInfo"`
					} `json:"result"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatalf("decode: %v: %s", err, rec.Body.String())
				}
				if resp.Result.ServerInfo.Name != "eagle" {
					t.Fatalf("serverInfo.name = %q", resp.Result.ServerInfo.Name)
				}
			}
		})
	}
}

func TestMuxMCPTokenRotation(t *testing.T) {
	tokA := strings.Repeat("a", 64)
	tokB := strings.Repeat("b", 64)

	before := muxFor(MCPConfig{Token: tokA, OwnerUID: "owner-uid"})
	if rec := doReq(before, http.MethodPost, "/mcp", tokB, mcpInitBody); rec.Code != http.StatusUnauthorized {
		t.Fatalf("B on A mux = %d, want 401", rec.Code)
	}

	after := muxFor(MCPConfig{Token: tokB, OwnerUID: "owner-uid"})
	if rec := doReq(after, http.MethodPost, "/mcp", tokA, mcpInitBody); rec.Code != http.StatusUnauthorized {
		t.Fatalf("A on B mux = %d, want 401", rec.Code)
	}
	if rec := doReq(after, http.MethodPost, "/mcp", tokB, mcpInitBody); rec.Code != http.StatusOK {
		t.Fatalf("B on B mux = %d, want 200", rec.Code)
	}
}

func TestMCPConfigEnabled(t *testing.T) {
	long := strings.Repeat("a", 64)
	tests := []struct {
		cfg  MCPConfig
		want bool
	}{
		{MCPConfig{Token: long, OwnerUID: "u"}, true},
		{MCPConfig{Token: long, OwnerUID: " "}, false},
		{MCPConfig{Token: "short", OwnerUID: "u"}, false},
		{MCPConfig{}, false},
	}
	for _, tt := range tests {
		if got := tt.cfg.Enabled(); got != tt.want {
			t.Errorf("%+v Enabled() = %v, want %v", tt.cfg, got, tt.want)
		}
	}
}
