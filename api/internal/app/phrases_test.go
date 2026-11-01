package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postPhrase(srv *Server, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := authed(httptest.NewRequest(http.MethodPost, "/api/phrases", strings.NewReader(body)), "u1")
	srv.savePhrase(rec, req)
	return rec
}

func TestSavePhraseOK(t *testing.T) {
	repo := &fakeRepo{sentenceJapanese: "時間がありません。", sentenceEnglish: "I don't have time."}
	srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})

	rec := postPhrase(srv, `{"sentence_id":1,"phrase":"  don't have time  ","note":" 時間がない "}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(repo.savedPhrases) != 1 {
		t.Fatalf("expected one save, got %d", len(repo.savedPhrases))
	}
	call := repo.savedPhrases[0]
	want := SavedPhrase{
		SentenceID: 1,
		Phrase:     "don't have time",
		Note:       "時間がない",
		Japanese:   "時間がありません。",
		English:    "I don't have time.",
	}
	if call.uid != "u1" || call.phrase != want {
		t.Fatalf("unexpected save: uid=%q phrase=%+v", call.uid, call.phrase)
	}
	var got SavedPhrase
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != "phrase-1" || got.Phrase != "don't have time" || got.English != "I don't have time." {
		t.Fatalf("unexpected body: %+v", got)
	}
}

func TestSavePhraseNoteIsOptional(t *testing.T) {
	repo := &fakeRepo{}
	srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})

	rec := postPhrase(srv, `{"sentence_id":1,"phrase":"have time"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if repo.savedPhrases[0].phrase.Note != "" {
		t.Fatalf("expected empty note, got %q", repo.savedPhrases[0].phrase.Note)
	}
}

// The sentence is loaded server-side, never trusted from the client.
func TestSavePhraseIgnoresClientSuppliedSentence(t *testing.T) {
	repo := &fakeRepo{sentenceJapanese: "犬", sentenceEnglish: "dog"}
	srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})

	rec := postPhrase(srv, `{"sentence_id":1,"phrase":"dog","english":"cat"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown field, got %d", rec.Code)
	}
	if len(repo.savedPhrases) != 0 {
		t.Fatalf("expected nothing saved")
	}
}

func TestSavePhraseRejectsInvalidInput(t *testing.T) {
	cases := map[string]string{
		"blank phrase":    `{"sentence_id":1,"phrase":"   "}`,
		"missing phrase":  `{"sentence_id":1}`,
		"phrase too long": `{"sentence_id":1,"phrase":"` + strings.Repeat("a", maxPhraseLength+1) + `"}`,
		"note too long":   `{"sentence_id":1,"phrase":"ok","note":"` + strings.Repeat("あ", maxPhraseNoteLength+1) + `"}`,
		"malformed":       `{`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepo{}
			srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})
			rec := postPhrase(srv, body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", rec.Code)
			}
			if len(repo.savedPhrases) != 0 {
				t.Fatalf("expected nothing saved")
			}
		})
	}
}

// Limits are in runes, so a multibyte phrase at the limit is accepted.
func TestSavePhraseAcceptsMultibyteAtLimit(t *testing.T) {
	repo := &fakeRepo{}
	srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})

	rec := postPhrase(srv, `{"sentence_id":1,"phrase":"ok","note":"`+strings.Repeat("あ", maxPhraseNoteLength)+`"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
}

func TestSavePhraseSentenceNotFound(t *testing.T) {
	repo := &fakeRepo{sentenceErr: ErrNotFound}
	srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})

	rec := postPhrase(srv, `{"sentence_id":99,"phrase":"have time"}`)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if len(repo.savedPhrases) != 0 {
		t.Fatalf("expected nothing saved")
	}
}

func TestSavePhraseRepoError(t *testing.T) {
	repo := &fakeRepo{savePhraseErr: errors.New("boom")}
	srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})

	rec := postPhrase(srv, `{"sentence_id":1,"phrase":"have time"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestSavePhraseMethodNotAllowed(t *testing.T) {
	srv := NewServer(&fakeRepo{}, &fakeExplainer{}, &fakeAnalyzer{})
	rec := httptest.NewRecorder()
	srv.savePhrase(rec, authed(httptest.NewRequest(http.MethodGet, "/api/phrases", nil), "u1"))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
