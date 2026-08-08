package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
)

const (
	// maxExplainRequestBytes bounds the /api/answer/explain request body so
	// an authenticated caller can't exhaust memory or inflate Gemini request
	// size via an arbitrarily large payload.
	maxExplainRequestBytes = 4096
	// maxUserAnswerLength bounds the submitted translation attempt itself,
	// independent of the overall body size limit. Enforced both when an
	// answer is checked (so oversized text is never persisted to Firestore
	// and later fanned into the weakness-insight prompt) and when it is
	// explained.
	maxUserAnswerLength = 2000
	// maxInsightMistakes bounds how many recent mistakes are sent to the
	// weakness analyzer, keeping prompt size and Gemini cost predictable as a
	// learner's mistake history grows.
	maxInsightMistakes = 50
)

type Server struct {
	repo      SentenceRepository
	explainer Explainer
	analyzer  WeaknessAnalyzer
}

func NewServer(repo SentenceRepository, explainer Explainer, analyzer WeaknessAnalyzer) *Server {
	return &Server{repo: repo, explainer: explainer, analyzer: analyzer}
}

func (s *Server) getRandomSentence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid, _ := uidFromContext(r.Context())
	var levels []int
	if raw := r.URL.Query().Get("levels"); raw != "" {
		seen := map[int]bool{}
		for _, part := range strings.Split(raw, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil || n < 1 || n > 5 {
				http.Error(w, "Invalid levels parameter", http.StatusBadRequest)
				return
			}
			if !seen[n] {
				seen[n] = true
				levels = append(levels, n)
			}
		}
	}
	sentence, err := s.repo.RandomCandidate(r.Context(), uid, levels)
	if errors.Is(err, ErrNoCandidate) {
		http.Error(w, "No sentences found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("random candidate error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, sentence)
}

// normalizeWhitespace collapses any run of whitespace (including internal
// newlines from multi-line input) into a single space, and trims the ends,
// so answers that differ only in whitespace formatting compare as equal.
func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func (s *Server) checkAnswer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid, _ := uidFromContext(r.Context())
	var req CheckAnswerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.UserAnswer) > maxUserAnswerLength {
		http.Error(w, "Invalid user_answer", http.StatusBadRequest)
		return
	}
	correct, err := s.repo.CorrectAnswer(r.Context(), req.SentenceID)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "Sentence not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("correct answer error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	histories, err := s.repo.ListIncorrectHistories(r.Context(), uid, req.SentenceID)
	if err != nil {
		log.Printf("list histories error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	isCorrect := strings.EqualFold(normalizeWhitespace(req.UserAnswer), normalizeWhitespace(correct))
	answer := ""
	if !isCorrect {
		answer = req.UserAnswer
	}
	if err := s.repo.RecordAnswer(r.Context(), uid, req.SentenceID, isCorrect, answer); err != nil {
		log.Printf("record answer error: %v", err)
	}
	writeJSON(w, CheckAnswerResponse{
		IsCorrect:     isCorrect,
		CorrectAnswer: correct,
		Histories:     histories,
	})
}

func (s *Server) getMistakes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid, _ := uidFromContext(r.Context())
	mistakes, err := s.repo.ListMistakes(r.Context(), uid)
	if err != nil {
		log.Printf("list mistakes error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, ListMistakesResponse{Mistakes: mistakes})
}

func (s *Server) getMistakesInsight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	language := r.URL.Query().Get("language")
	if !validExplainLanguages[language] {
		http.Error(w, "Invalid language", http.StatusBadRequest)
		return
	}
	uid, _ := uidFromContext(r.Context())
	mistakes, err := s.repo.ListMistakesForInsight(r.Context(), uid)
	if err != nil {
		log.Printf("list mistakes error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if len(mistakes) == 0 {
		writeJSON(w, MistakesInsightResponse{Insight: ""})
		return
	}
	if len(mistakes) > maxInsightMistakes {
		mistakes = mistakes[:maxInsightMistakes]
	}
	insight, err := s.analyzer.Analyze(r.Context(), mistakes, language)
	if err != nil {
		log.Printf("analyze mistakes error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(insight) == "" {
		log.Printf("analyze mistakes returned an empty insight for %d mistakes", len(mistakes))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, MistakesInsightResponse{Insight: insight})
}

func (s *Server) reportSentence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req ReportSentenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if err := s.repo.Report(r.Context(), req.SentenceID); err != nil {
		log.Printf("report error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) explainAnswer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxExplainRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var req ExplainRequest
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	userAnswer := strings.TrimSpace(req.UserAnswer)
	if userAnswer == "" || len(userAnswer) > maxUserAnswerLength {
		http.Error(w, "Invalid user_answer", http.StatusBadRequest)
		return
	}
	if !validExplainLanguages[req.Language] {
		http.Error(w, "Invalid language", http.StatusBadRequest)
		return
	}
	// The Japanese sentence and reference answer are always loaded
	// server-side by sentence_id, never trusted from the client — otherwise
	// an authenticated caller could submit arbitrary text for Gemini to
	// process under this app's own API key.
	japanese, correctAnswer, err := s.repo.GetSentence(r.Context(), req.SentenceID)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "Sentence not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("get sentence error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	explanation, err := s.explainer.Explain(r.Context(), japanese, correctAnswer, userAnswer, req.Language)
	if err != nil {
		log.Printf("explain answer error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, ExplainResponse{Explanation: explanation})
}

func livenessHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "OK")
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode error: %v", err)
	}
}
