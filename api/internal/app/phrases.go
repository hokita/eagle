package app

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"
)

const (
	// maxSavePhraseRequestBytes bounds a /api/phrases body: the phrase and the
	// note at their rune limits, at up to 4 UTF-8 bytes per rune, plus room for
	// the JSON around them.
	maxSavePhraseRequestBytes = 4096
	// maxPhraseLength bounds the phrase itself, in runes. A phrase is a few
	// words lifted from a sentence, so anything near this is a paste mistake.
	maxPhraseLength = 200
	// maxPhraseNoteLength bounds the learner's own note on what the phrase
	// means or when to use it, in runes.
	maxPhraseNoteLength = 500
)

// SavePhraseRequest is a phrase the learner picked up from a sentence they
// just answered, with an optional note in their own words.
type SavePhraseRequest struct {
	SentenceID int    `json:"sentence_id"`
	Phrase     string `json:"phrase"`
	Note       string `json:"note"`
}

// SavedPhrase is a phrase as stored. The sentence it came from is copied in
// at save time, loaded server-side by sentence_id, so a saved phrase keeps
// the example it was learned from even if that sentence is later edited.
type SavedPhrase struct {
	ID         string `json:"id"`
	SentenceID int    `json:"sentence_id"`
	Phrase     string `json:"phrase"`
	Note       string `json:"note"`
	Japanese   string `json:"japanese"`
	English    string `json:"english"`
	CreatedAt  string `json:"created_at"`
}

func (s *Server) savePhrase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid, _ := uidFromContext(r.Context())
	var req SavePhraseRequest
	if !decodeBody(w, r, &req, maxSavePhraseRequestBytes) {
		return
	}
	phrase := strings.TrimSpace(req.Phrase)
	if !nonBlankWithin(phrase, maxPhraseLength) {
		http.Error(w, "Invalid phrase", http.StatusBadRequest)
		return
	}
	note := strings.TrimSpace(req.Note)
	if utf8.RuneCountInString(note) > maxPhraseNoteLength {
		http.Error(w, "Invalid note", http.StatusBadRequest)
		return
	}
	japanese, english, err := s.repo.GetSentence(r.Context(), req.SentenceID)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "Sentence not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("get sentence error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	saved, err := s.repo.SavePhrase(r.Context(), uid, SavedPhrase{
		SentenceID: req.SentenceID,
		Phrase:     phrase,
		Note:       note,
		Japanese:   japanese,
		English:    english,
	})
	if err != nil {
		log.Printf("save phrase error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, saved)
}
