package app

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"
)

// WithDiscussion attaches the discussion-practice dependencies. A chained
// setter rather than new NewServer parameters so the existing constructor's
// many call sites stay unchanged.
func (s *Server) WithDiscussion(repo DiscussionRepository, coach DiscussionCoach) *Server {
	s.discussions = repo
	s.coach = coach
	return s
}

type DiscussionReplyRequest struct {
	QuestionID int                 `json:"question_id"`
	Transcript []DiscussionMessage `json:"transcript"`
}

func (s *Server) getDiscussionQuestion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q, err := s.discussions.RandomQuestion(r.Context())
	if errors.Is(err, ErrNoCandidate) {
		http.Error(w, "No questions found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("random discussion question error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, q)
}

// decodeDiscussionBody bounds and strictly decodes a discussion request
// body. Returns false after writing the 400 response itself.
func decodeDiscussionBody(w http.ResponseWriter, r *http.Request, dst interface{}, maxBytes int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return false
	}
	return true
}

// loadDiscussionQuestion fetches the question by id, writing the error
// response itself when it fails (nil result means "already handled").
func (s *Server) loadDiscussionQuestion(w http.ResponseWriter, r *http.Request, id int) *DiscussionQuestion {
	q, err := s.discussions.GetQuestion(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "Question not found", http.StatusNotFound)
		return nil
	}
	if err != nil {
		log.Printf("get discussion question error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return nil
	}
	return q
}

func (s *Server) discussionReply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req DiscussionReplyRequest
	if !decodeDiscussionBody(w, r, &req, maxDiscussionRequestBytes) {
		return
	}
	if err := validateTranscript(req.Transcript); err != nil {
		http.Error(w, "Invalid transcript", http.StatusBadRequest)
		return
	}
	if req.Transcript[len(req.Transcript)-1].Role != "user" {
		http.Error(w, "Invalid transcript", http.StatusBadRequest)
		return
	}
	q := s.loadDiscussionQuestion(w, r, req.QuestionID)
	if q == nil {
		return
	}
	// Every session is exactly discussionFollowUps questions long, decided
	// here rather than by the model: once they are answered the conversation
	// is over and Gemini is never even called.
	if countAITurns(req.Transcript) >= discussionFollowUps {
		writeJSON(w, DiscussionReplyResponse{Done: true})
		return
	}
	reply, err := s.coach.Reply(r.Context(), q, req.Transcript)
	if err != nil {
		log.Printf("discussion reply error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, DiscussionReplyResponse{Done: false, Message: reply.Message})
}

// discussionTrimmed reports whether text is non-blank after trimming and
// within limit. The limit is a rune count — the same unit the frontend
// textareas' character-based maxLength approximates — so multibyte input
// the client accepts is never rejected here for its length.
func discussionTrimmed(text string, limit int) bool {
	t := strings.TrimSpace(text)
	return t != "" && utf8.RuneCountInString(text) <= limit
}

// DiscussionReplyResponse is the wire shape of a reply turn. "done" is the
// server's decision (see discussionFollowUps), so it lives here rather than
// on CoachReply; a done response carries no message.
type DiscussionReplyResponse struct {
	Done    bool   `json:"done"`
	Message string `json:"message"`
}

// DiscussionCompleteRequest carries only what the learner produced. The
// summary is generated here rather than sent up, so a client cannot decide
// what gets stored as its own feedback.
type DiscussionCompleteRequest struct {
	QuestionID   int                 `json:"question_id"`
	Transcript   []DiscussionMessage `json:"transcript"`
	ReflectionJA string              `json:"reflection_ja"`
}

type DiscussionCompleteResponse struct {
	SessionID      string   `json:"session_id"`
	NaturalEnglish string   `json:"natural_english"`
	Phrases        []Phrase `json:"phrases"`
}

// discussionComplete is the single closing step: it summarizes the session
// and persists it in one round trip. There is nothing for the learner to do
// between the two, so splitting them would only cost a request.
func (s *Server) discussionComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid, _ := uidFromContext(r.Context())
	var req DiscussionCompleteRequest
	if !decodeDiscussionBody(w, r, &req, maxDiscussionRequestBytes) {
		return
	}
	if err := validateTranscript(req.Transcript); err != nil {
		http.Error(w, "Invalid transcript", http.StatusBadRequest)
		return
	}
	// The reflection cannot be skipped, so it is always present here.
	if !discussionTrimmed(req.ReflectionJA, maxReflectionLength) {
		http.Error(w, "Invalid reflection_ja", http.StatusBadRequest)
		return
	}
	q := s.loadDiscussionQuestion(w, r, req.QuestionID)
	if q == nil {
		return
	}
	summary, err := s.coach.Summarize(r.Context(), q, req.Transcript, req.ReflectionJA)
	if err != nil {
		log.Printf("discussion summarize error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	session := &DiscussionSession{
		QuestionID:     q.ID,
		QuestionEN:     q.QuestionEN,
		Topic:          q.Topic,
		Transcript:     req.Transcript,
		ReflectionJA:   req.ReflectionJA,
		NaturalEnglish: summary.NaturalEnglish,
		Phrases:        summary.Phrases,
	}
	sessionID, err := s.discussions.SaveSession(r.Context(), uid, session)
	if err != nil {
		log.Printf("save discussion session error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, DiscussionCompleteResponse{
		SessionID:      sessionID,
		NaturalEnglish: summary.NaturalEnglish,
		Phrases:        summary.Phrases,
	})
}

type DiscussionSessionsResponse struct {
	Sessions []DiscussionSessionSummary `json:"sessions"`
}

// discussionSessions serves both the list (no path suffix) and the detail
// (suffix = session id). One handler because ServeMux's trailing-slash
// pattern would otherwise split them across two registrations anyway.
func (s *Server) discussionSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid, _ := uidFromContext(r.Context())
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/discussion/sessions"), "/")
	if strings.Contains(id, "/") {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	if id == "" {
		sessions, err := s.discussions.ListSessions(r.Context(), uid, maxDiscussionSessionList)
		if err != nil {
			log.Printf("list discussion sessions error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, DiscussionSessionsResponse{Sessions: sessions})
		return
	}
	session, err := s.discussions.GetSession(r.Context(), uid, id)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("get discussion session error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, session)
}
