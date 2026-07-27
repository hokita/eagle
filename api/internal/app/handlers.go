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
	// independent of the overall body size limit.
	maxUserAnswerLength = 2000
)

type Server struct {
	repo      SentenceRepository
	explainer Explainer
}

func NewServer(repo SentenceRepository, explainer Explainer) *Server {
	return &Server{repo: repo, explainer: explainer}
}

func (s *Server) getRandomSentence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid, _ := uidFromContext(r.Context())
	level := 0
	if raw := r.URL.Query().Get("level"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 5 {
			http.Error(w, "Invalid level parameter", http.StatusBadRequest)
			return
		}
		level = n
	}
	var levels []int
	if level != 0 {
		levels = []int{level}
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
	isCorrect := strings.EqualFold(strings.TrimSpace(req.UserAnswer), strings.TrimSpace(correct))
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
	explanation, err := s.explainer.Explain(r.Context(), japanese, correctAnswer, userAnswer)
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
