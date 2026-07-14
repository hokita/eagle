package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
)

type Server struct {
	repo SentenceRepository
}

func NewServer(repo SentenceRepository) *Server {
	return &Server{repo: repo}
}

func (s *Server) getRandomSentence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid, _ := uidFromContext(r.Context())
	sentence, err := s.repo.RandomCandidate(r.Context(), uid)
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
