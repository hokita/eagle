package app

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
)

const (
	maxSavePhraseRequestBytes = 4096
	maxPhraseLength           = 200
	maxPhraseList             = 500
)

type SavePhraseRequest struct {
	SentenceID int    `json:"sentence_id"`
	Phrase     string `json:"phrase"`
}

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

type PhraseRepository interface {
	SavePhrase(ctx context.Context, uid string, phrase SavedPhrase) (*SavedPhrase, error)
	ListPhrases(ctx context.Context, uid string, limit int) ([]SavedPhrase, error)
	GetPhrase(ctx context.Context, uid, id string) (*SavedPhrase, error)
	UpdatePhraseContent(ctx context.Context, uid, id string, content PhraseContent) error
	DeletePhrase(ctx context.Context, uid, id string) error
}

func (s *Server) WithPhrases(repo PhraseRepository, writer PhraseWriter) *Server {
	s.phrases = repo
	s.phraseWriter = writer
	return s
}

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
