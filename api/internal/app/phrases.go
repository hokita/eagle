package app

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
)

const (
	// maxSavePhraseRequestBytes bounds a /api/phrases body: the phrase at its
	// rune limit, at up to 4 UTF-8 bytes per rune, plus room for the JSON
	// around it.
	maxSavePhraseRequestBytes = 4096
	// maxPhraseLength bounds the phrase itself, in runes. A phrase is a few
	// words lifted from a sentence, so anything near this is a paste mistake.
	maxPhraseLength = 200
	// maxPhraseList bounds how many phrases the Phrases page loads, keeping
	// the read cost of one page view predictable.
	maxPhraseList = 500
)

// SavePhraseRequest is a phrase the learner picked up while answering a
// sentence.
type SavePhraseRequest struct {
	SentenceID int    `json:"sentence_id"`
	Phrase     string `json:"phrase"`
}

// SavedPhrase is a phrase as stored, with the meaning and example sentences
// the AI wrote for it. Meaning is empty and Examples is empty when the AI
// could not write them at save time; the learner can ask again.
//
// The sentence it was saved from is kept so the AI can tell which sense of
// the phrase was meant, but it is not shown to the learner, so it is left
// out of the JSON.
type SavedPhrase struct {
	ID         string   `json:"id"`
	Phrase     string   `json:"phrase"`
	Meaning    string   `json:"meaning"`
	Examples   []string `json:"examples"`
	CreatedAt  string   `json:"created_at"`
	SentenceID int      `json:"-"`
	Japanese   string   `json:"-"`
	English    string   `json:"-"`
}

type ListPhrasesResponse struct {
	Phrases []SavedPhrase `json:"phrases"`
}

// PhraseRepository stores a user's saved phrases.
type PhraseRepository interface {
	// SavePhrase stores the phrase and returns it with its ID and creation
	// time filled in.
	SavePhrase(ctx context.Context, uid string, phrase SavedPhrase) (*SavedPhrase, error)
	// ListPhrases returns up to limit phrases, newest first.
	ListPhrases(ctx context.Context, uid string, limit int) ([]SavedPhrase, error)
	// GetPhrase returns ErrNotFound for a missing id.
	GetPhrase(ctx context.Context, uid, id string) (*SavedPhrase, error)
	// UpdatePhraseContent replaces the meaning and examples, returning
	// ErrNotFound for a missing id.
	UpdatePhraseContent(ctx context.Context, uid, id string, content PhraseContent) error
	// DeletePhrase returns ErrNotFound for a missing id.
	DeletePhrase(ctx context.Context, uid, id string) error
}

// WithPhrases attaches the saved-phrase dependencies, the same way
// WithDiscussion does, so NewServer's call sites stay unchanged.
func (s *Server) WithPhrases(repo PhraseRepository, writer PhraseWriter) *Server {
	s.phrases = repo
	s.phraseWriter = writer
	return s
}

// phrasesHandler serves the whole /api/phrases tree:
//
//	GET    /api/phrases                list the learner's phrases
//	POST   /api/phrases                save a phrase
//	DELETE /api/phrases/{id}           delete one
//	POST   /api/phrases/{id}/examples  have the AI write the meaning and examples again
func (s *Server) phrasesHandler(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/phrases"), "/")
	parts := strings.Split(rest, "/")
	switch {
	case rest == "":
		switch r.Method {
		case http.MethodGet:
			s.listPhrases(w, r)
		case http.MethodPost:
			s.savePhrase(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	case len(parts) == 1 && validPhraseID(parts[0]):
		if r.Method != http.MethodDelete {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.deletePhrase(w, r, parts[0])
	case len(parts) == 2 && validPhraseID(parts[0]) && parts[1] == "examples":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.rewritePhrase(w, r, parts[0])
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

// validPhraseID rejects path segments Firestore cannot use as a document ID,
// so they read as a missing phrase rather than as a server error.
func validPhraseID(id string) bool {
	return id != "" && id != "." && id != ".." && len(id) <= 128
}

func (s *Server) savePhrase(w http.ResponseWriter, r *http.Request) {
	uid, _ := uidFromContext(r.Context())
	var req SavePhraseRequest
	if !decodeBody(w, r, &req, maxSavePhraseRequestBytes) {
		return
	}
	phrase := strings.Join(strings.Fields(req.Phrase), " ")
	if !nonBlankWithin(phrase, maxPhraseLength) {
		http.Error(w, "Invalid phrase", http.StatusBadRequest)
		return
	}
	// The sentence is loaded server-side by sentence_id, never trusted from
	// the client, as on the explain path: it goes into the AI's prompt.
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
	toSave := SavedPhrase{
		Phrase:     phrase,
		Examples:   []string{},
		SentenceID: req.SentenceID,
		Japanese:   japanese,
		English:    english,
	}
	// A phrase the AI could not write about is still worth keeping: the
	// learner typed it, and the Phrases page lets them ask the AI again.
	if content, err := s.writePhraseContent(r.Context(), toSave); err != nil {
		log.Printf("write phrase content error: %v", err)
	} else {
		toSave.Meaning = content.Meaning
		toSave.Examples = content.Examples
	}
	saved, err := s.phrases.SavePhrase(r.Context(), uid, toSave)
	if err != nil {
		log.Printf("save phrase error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, saved)
}

func (s *Server) listPhrases(w http.ResponseWriter, r *http.Request) {
	uid, _ := uidFromContext(r.Context())
	phrases, err := s.phrases.ListPhrases(r.Context(), uid, maxPhraseList)
	if err != nil {
		log.Printf("list phrases error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, ListPhrasesResponse{Phrases: phrases})
}

func (s *Server) deletePhrase(w http.ResponseWriter, r *http.Request, id string) {
	uid, _ := uidFromContext(r.Context())
	err := s.phrases.DeletePhrase(r.Context(), uid, id)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "Phrase not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("delete phrase error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// rewritePhrase has the AI write a phrase's meaning and examples again. It
// backs both "New examples" and retrying a phrase saved without a meaning.
// Unlike saving, a failure here is reported: there is nothing else to show.
func (s *Server) rewritePhrase(w http.ResponseWriter, r *http.Request, id string) {
	uid, _ := uidFromContext(r.Context())
	phrase, err := s.phrases.GetPhrase(r.Context(), uid, id)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "Phrase not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("get phrase error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	content, err := s.writePhraseContent(r.Context(), *phrase)
	if err != nil {
		log.Printf("write phrase content error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	err = s.phrases.UpdatePhraseContent(r.Context(), uid, id, *content)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "Phrase not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("update phrase error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	phrase.Meaning = content.Meaning
	phrase.Examples = content.Examples
	writeJSON(w, phrase)
}

func (s *Server) writePhraseContent(ctx context.Context, p SavedPhrase) (*PhraseContent, error) {
	if s.phraseWriter == nil {
		return nil, errors.New("no phrase writer configured")
	}
	content, err := s.phraseWriter.Write(ctx, PhraseSource{
		Phrase:   p.Phrase,
		Japanese: p.Japanese,
		English:  p.English,
	})
	if err != nil {
		return nil, err
	}
	return cleanPhraseContent(p.Phrase, content)
}
