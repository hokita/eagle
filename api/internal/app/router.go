package app

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type MCPConfig struct {
	Token    string
	OwnerUID string
}

func MCPConfigFromEnv() MCPConfig {
	return MCPConfig{Token: os.Getenv("EAGLE_MCP_TOKEN"), OwnerUID: os.Getenv("MCP_OWNER_UID")}
}

func (c MCPConfig) ownerSet() bool {
	return strings.TrimSpace(c.OwnerUID) != ""
}

func (c MCPConfig) Enabled() bool {
	return len(c.Token) >= minMCPTokenLength && c.ownerSet()
}

func (c MCPConfig) LogStatus() {
	switch {
	case c.Enabled():
		log.Printf("MCP enabled")
	case !c.ownerSet():
		log.Printf("MCP disabled: MCP_OWNER_UID is not set")
	default:
		log.Printf("MCP disabled: EAGLE_MCP_TOKEN is not set or too short")
	}
}

// NewMux wires all HTTP routes with CORS and auth, matching the API's
// public surface exactly.
func NewMux(srv *Server, verifier TokenVerifier, allowedEmails []string, frontendURL string, mcpCfg MCPConfig) *http.ServeMux {
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return withCORS(frontendURL, requireAuth(verifier, allowedEmails, h))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/sentence/random", auth(srv.getRandomSentence))
	mux.HandleFunc("/api/answer/check", auth(srv.checkAnswer))
	mux.HandleFunc("/api/mistakes", auth(srv.getMistakes))
	mux.HandleFunc("/api/mistakes/insight", auth(srv.getMistakesInsight))
	mux.HandleFunc("/api/answer/explain", auth(srv.explainAnswer))
	mux.HandleFunc("/api/answer/followup", auth(srv.followUpAnswer))
	mux.HandleFunc("/api/sentence/report", auth(srv.reportSentence))
	mux.HandleFunc("/api/phrases", auth(srv.phrasesHandler))
	mux.HandleFunc("/api/phrases/", auth(srv.phrasesHandler))
	mux.HandleFunc("/api/discussion/question", auth(srv.getDiscussionQuestion))
	mux.HandleFunc("/api/discussion/reply", auth(srv.discussionReply))
	mux.HandleFunc("/api/discussion/complete", auth(srv.discussionComplete))
	mux.HandleFunc("/api/discussion/sessions", auth(srv.discussionSessions))
	mux.HandleFunc("/api/discussion/sessions/", auth(srv.discussionSessions))
	mux.HandleFunc("/api/liveness", livenessHandler)

	matcher := NewMCPTokenMatcher(mcpCfg.Token)
	if !mcpCfg.ownerSet() {
		matcher = nil
	}
	mux.Handle("/mcp", requireMCPToken(matcher, newMCPHandler(srv, mcpCfg.OwnerUID, NewRateLimiter(120, time.Hour, time.Now))))
	return mux
}
