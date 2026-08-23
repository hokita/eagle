package app

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
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
func decodeDiscussionBody(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxDiscussionRequestBytes)
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
	if !decodeDiscussionBody(w, r, &req) {
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
	// Server-side hard cap: past maxAIFollowUps the conversation is over no
	// matter what the model would say — and Gemini is never even called.
	if countAITurns(req.Transcript) >= maxAIFollowUps {
		writeJSON(w, CoachReply{Done: true, Message: ""})
		return
	}
	reply, err := s.coach.Reply(r.Context(), q, req.Transcript)
	if err != nil {
		log.Printf("discussion reply error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, reply)
}

// discussionTrimmed reports whether text is non-blank after trimming and
// within limit.
func discussionTrimmed(text string, limit int) bool {
	t := strings.TrimSpace(text)
	return t != "" && len(text) <= limit
}
