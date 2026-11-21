package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	maxExplainRequestBytes    = 4096
	maxUserAnswerLength       = 2000
	maxInsightMistakes        = 50
	maxFollowUpRequestBytes   = 192 * 1024
	maxFollowUpQuestionLength = 500
	maxFollowUpAnswerLength   = 4000
	maxFollowUpHistory        = 5
)

type Server struct {
	repo      SentenceRepository
	explainer Explainer
	analyzer  WeaknessAnalyzer

	discussions DiscussionRepository
	coach       DiscussionCoach

	phrases      PhraseRepository
	phraseWriter PhraseWriter
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
		var parsed []int
		for _, part := range strings.Split(raw, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil {
				http.Error(w, "Invalid levels parameter", http.StatusBadRequest)
				return
			}
			parsed = append(parsed, n)
		}
		var ok bool
		if levels, ok = uniqueLevels(parsed); !ok {
			http.Error(w, "Invalid levels parameter", http.StatusBadRequest)
			return
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

func uniqueLevels(levels []int) ([]int, bool) {
	var out []int
	seen := map[int]bool{}
	for _, n := range levels {
		if n < 1 || n > 5 {
			return nil, false
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out, true
}

var answerPunctuation = strings.NewReplacer(
	"‘", "'",
	"’", "'",
	"ʼ", "'",
	"′", "'",
	"＇", "'",
	"“", `"`,
	"”", `"`,
	"″", `"`,
	"＂", `"`,
	"‐", "-",
	"‑", "-",
	"–", "-",
	"—", "-",
	"，", ",",
)

const terminalPunctuation = ".!?…。．！？ "

func normalizeAnswer(s string) string {
	s = answerPunctuation.Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimRight(s, terminalPunctuation)
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
	resp, err := s.checkAndRecord(r.Context(), uid, req.SentenceID, req.UserAnswer)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "Sentence not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("check answer error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
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
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "Sentence not found", http.StatusNotFound)
			return
		}
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
	var req ExplainRequest
	if !decodeBody(w, r, &req, maxExplainRequestBytes) {
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

func (s *Server) followUpAnswer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req FollowUpRequest
	if !decodeBody(w, r, &req, maxFollowUpRequestBytes) {
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
	if !nonBlankWithin(req.Explanation, maxFollowUpAnswerLength) {
		http.Error(w, "Invalid explanation", http.StatusBadRequest)
		return
	}
	question := strings.TrimSpace(req.Question)
	if !nonBlankWithin(req.Question, maxFollowUpQuestionLength) {
		http.Error(w, "Invalid question", http.StatusBadRequest)
		return
	}
	for _, turn := range req.History {
		if !nonBlankWithin(turn.Question, maxFollowUpQuestionLength) ||
			!nonBlankWithin(turn.Answer, maxFollowUpAnswerLength) {
			http.Error(w, "Invalid history", http.StatusBadRequest)
			return
		}
	}
	history := req.History
	if len(history) > maxFollowUpHistory {
		history = history[len(history)-maxFollowUpHistory:]
	}
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
	answer, err := s.explainer.AnswerFollowUp(r.Context(), FollowUpInput{
		Japanese:      japanese,
		CorrectAnswer: correctAnswer,
		UserAnswer:    userAnswer,
		Explanation:   req.Explanation,
		History:       history,
		Question:      question,
		Language:      req.Language,
	})
	if err != nil {
		log.Printf("follow-up answer error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(answer) == "" {
		log.Printf("follow-up answer was empty for sentence %d", req.SentenceID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, FollowUpResponse{Answer: answer})
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst interface{}, maxBytes int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return false
	}
	return true
}

func nonBlankWithin(text string, limit int) bool {
	return strings.TrimSpace(text) != "" && utf8.RuneCountInString(text) <= limit
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
