package app

import "net/http"

// NewMux wires all HTTP routes with CORS and auth, matching the API's
// public surface exactly.
func NewMux(srv *Server, verifier TokenVerifier, allowedEmails []string, frontendURL string) *http.ServeMux {
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return withCORS(frontendURL, requireAuth(verifier, allowedEmails, h))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/sentence/random", auth(srv.getRandomSentence))
	mux.HandleFunc("/api/answer/check", auth(srv.checkAnswer))
	mux.HandleFunc("/api/mistakes", auth(srv.getMistakes))
	mux.HandleFunc("/api/mistakes/insight", auth(srv.getMistakesInsight))
	mux.HandleFunc("/api/answer/explain", auth(srv.explainAnswer))
	mux.HandleFunc("/api/sentence/report", auth(srv.reportSentence))
	mux.HandleFunc("/api/liveness", livenessHandler)
	return mux
}
