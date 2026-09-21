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
	// maxFollowUpRequestBytes bounds a /api/answer/followup body. It is larger
	// than the explain cap because the request carries the whole thread back:
	// the explanation, every earlier question and answer, and the new
	// question. Sized from the multibyte worst case of the rune limits below —
	// the explanation plus the maxFollowUpHistory turns a client sends, each
	// answer at maxFollowUpAnswerLength, their questions, and the learner's
	// translation, at up to 4 UTF-8 bytes per rune — so a thread whose every
	// field passes its own validation is never rejected for the size of the
	// body carrying it.
	maxFollowUpRequestBytes = 192 * 1024
	// maxFollowUpQuestionLength bounds one free-text question, in runes — the
	// same unit the frontend textarea's maxLength approximates, so text the
	// client accepts is never rejected server-side for its length.
	maxFollowUpQuestionLength = 500
	// maxFollowUpAnswerLength bounds the model's own text coming back from the
	// client: the explanation being asked about and each earlier answer in the
	// thread. Generously above what maxExplainOutputTokens can produce.
	maxFollowUpAnswerLength = 4000
	// maxFollowUpHistory caps how many earlier question/answer pairs are sent
	// on to Gemini, bounding both prompt size and cost per question. Extra
	// turns are dropped rather than rejected — a learner who keeps asking is
	// using the feature, not abusing it, so the oldest questions fall out of
	// the model's context while the thread they read stays whole.
	maxFollowUpHistory = 5
)

type Server struct {
	repo      SentenceRepository
	explainer Explainer
	analyzer  WeaknessAnalyzer

	discussions DiscussionRepository
	coach       DiscussionCoach
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

// answerPunctuation folds the typographic punctuation that phone keyboards,
// word processors and Japanese IMEs substitute for the ASCII characters on a
// plain keyboard. A learner who types "It’s" with a curly apostrophe wrote the
// same sentence as "It's" — the two are near-indistinguishable on screen, so
// grading them apart reads as the app being broken rather than as a mistake to
// learn from.
var answerPunctuation = strings.NewReplacer(
	"‘", "'", // ‘ left single quotation mark
	"’", "'", // ’ right single quotation mark, the curly apostrophe
	"ʼ", "'", // ʼ modifier letter apostrophe
	"′", "'", // ′ prime
	"＇", "'", // ＇ fullwidth apostrophe
	"“", `"`, // “ left double quotation mark
	"”", `"`, // ” right double quotation mark
	"″", `"`, // ″ double prime
	"＂", `"`, // ＂ fullwidth quotation mark
	"‐", "-", // ‐ hyphen
	"‑", "-", // ‑ non-breaking hyphen
	"–", "-", // – en dash
	"—", "-", // — em dash
	"，", ",", // ， fullwidth comma
)

// terminalPunctuation is the sentence-ending punctuation dropped from the end
// of both answers before they are compared, in the ASCII, fullwidth and
// Japanese forms an IME can produce. English word order already carries the
// difference between a question and a statement, so a final "." left off — or
// a "。" left in — is a keyboard slip, not a translation mistake. Only the very
// end of the answer is trimmed: punctuation inside the sentence still counts.
const terminalPunctuation = ".!?…。．！？ "

// normalizeAnswer puts an answer into the form the grader compares. Runs of
// whitespace (including internal newlines from multi-line input) collapse to a
// single space, typographic punctuation folds to its ASCII equivalent, and
// sentence-ending punctuation is dropped, so answers that differ only in
// formatting compare as equal.
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
	isCorrect := strings.EqualFold(normalizeAnswer(req.UserAnswer), normalizeAnswer(correct))
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

// followUpAnswer answers one free-text question about an explanation the
// learner was just shown. Nothing on the explain path is stored, so the
// client sends the thread it has on screen back with each question; only the
// Japanese sentence and the reference answer are loaded server-side by
// sentence_id, exactly as explainAnswer loads them, so an authenticated
// caller still cannot pass arbitrary text off as the sentence being studied.
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
	// The most recent turns are the ones the new question follows on from.
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
	// An empty answer has nothing to render, so it is reported as the failure
	// it is rather than shown as a blank turn in the thread.
	if strings.TrimSpace(answer) == "" {
		log.Printf("follow-up answer was empty for sentence %d", req.SentenceID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, FollowUpResponse{Answer: answer})
}

// decodeBody bounds and strictly decodes a JSON request body, writing the 400
// response itself and returning false when it cannot.
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

// nonBlankWithin reports whether text is non-blank after trimming and within
// limit. The limit is a rune count — the same unit the frontend textareas'
// character-based maxLength approximates — so multibyte input the client
// accepts is never rejected here for its length.
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
