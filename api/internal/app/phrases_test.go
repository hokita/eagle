package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakePhraseRepo struct {
	saved     []SavedPhrase
	saveErr   error
	list      []SavedPhrase
	listErr   error
	listLimit int
	get       *SavedPhrase
	getErr    error
	updated   []PhraseContent
	updateErr error
	deleted   []string
	deleteErr error
	uids      []string
}

func (f *fakePhraseRepo) SavePhrase(_ context.Context, uid string, p SavedPhrase) (*SavedPhrase, error) {
	f.uids = append(f.uids, uid)
	f.saved = append(f.saved, p)
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	p.ID = "phrase-1"
	p.CreatedAt = "2026-11-01T00:00:00Z"
	return &p, nil
}
func (f *fakePhraseRepo) ListPhrases(_ context.Context, uid string, limit int) ([]SavedPhrase, error) {
	f.uids = append(f.uids, uid)
	f.listLimit = limit
	if f.list == nil {
		return []SavedPhrase{}, f.listErr
	}
	return f.list, f.listErr
}
func (f *fakePhraseRepo) GetPhrase(_ context.Context, uid, id string) (*SavedPhrase, error) {
	f.uids = append(f.uids, uid)
	if f.getErr != nil {
		return nil, f.getErr
	}
	p := *f.get
	return &p, nil
}
func (f *fakePhraseRepo) UpdatePhraseContent(_ context.Context, uid, id string, c PhraseContent) error {
	f.uids = append(f.uids, uid)
	f.updated = append(f.updated, c)
	return f.updateErr
}
func (f *fakePhraseRepo) DeletePhrase(_ context.Context, uid, id string) error {
	f.uids = append(f.uids, uid)
	f.deleted = append(f.deleted, id)
	return f.deleteErr
}

type fakePhraseWriter struct {
	content *PhraseContent
	err     error
	got     []PhraseSource
}

func (f *fakePhraseWriter) Write(_ context.Context, src PhraseSource) (*PhraseContent, error) {
	f.got = append(f.got, src)
	if f.err != nil {
		return nil, f.err
	}
	c := *f.content
	return &c, nil
}

func goodContent() *PhraseContent {
	return &PhraseContent{
		Meaning: "as little as possible",
		Examples: []string{
			"Let's keep the meeting to a minimum today.",
			"I keep my spending to a minimum before payday.",
			"Keep your luggage to a minimum.",
		},
	}
}

type phraseFixture struct {
	repo    *fakeRepo
	phrases *fakePhraseRepo
	writer  *fakePhraseWriter
	srv     *Server
}

func newPhraseFixture() *phraseFixture {
	f := &phraseFixture{
		repo:    &fakeRepo{sentenceJapanese: "損失は最小限に抑えられる。", sentenceEnglish: "We can minimize the loss."},
		phrases: &fakePhraseRepo{},
		writer:  &fakePhraseWriter{content: goodContent()},
	}
	f.srv = NewServer(f.repo, &fakeExplainer{}, &fakeAnalyzer{}).WithPhrases(f.phrases, f.writer)
	return f
}

func (f *phraseFixture) do(method, path, body string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	rec := httptest.NewRecorder()
	f.srv.phrasesHandler(rec, authed(httptest.NewRequest(method, path, reader), "u1"))
	return rec
}

func decodePhrase(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var got map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return got
}

func TestSavePhraseWritesMeaningAndExamples(t *testing.T) {
	f := newPhraseFixture()

	rec := f.do(http.MethodPost, "/api/phrases", `{"sentence_id":1,"phrase":"  to a   minimum "}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	// The AI is given the phrase and the sentence, loaded server-side.
	want := PhraseSource{Phrase: "to a minimum", Japanese: "損失は最小限に抑えられる。", English: "We can minimize the loss."}
	if len(f.writer.got) != 1 || f.writer.got[0] != want {
		t.Fatalf("unexpected writer input: %+v", f.writer.got)
	}
	saved := f.phrases.saved[0]
	if f.phrases.uids[0] != "u1" || saved.Phrase != "to a minimum" || saved.SentenceID != 1 ||
		saved.English != "We can minimize the loss." || saved.Meaning != "as little as possible" || len(saved.Examples) != 3 {
		t.Fatalf("unexpected save: %+v", saved)
	}
	got := decodePhrase(t, rec)
	if got["id"] != "phrase-1" || got["meaning"] != "as little as possible" || len(got["examples"].([]interface{})) != 3 {
		t.Fatalf("unexpected body: %v", got)
	}
	// The source sentence is kept for the AI but never sent to the page.
	for _, key := range []string{"japanese", "english", "sentence_id", "note"} {
		if _, ok := got[key]; ok {
			t.Fatalf("response should not include %q: %v", key, got)
		}
	}
}

func TestSavePhraseStillSavesWhenTheAIFails(t *testing.T) {
	f := newPhraseFixture()
	f.writer.err = errors.New("gemini down")

	rec := f.do(http.MethodPost, "/api/phrases", `{"sentence_id":1,"phrase":"to a minimum"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	got := decodePhrase(t, rec)
	if got["meaning"] != "" || len(got["examples"].([]interface{})) != 0 {
		t.Fatalf("expected empty meaning and examples, got %v", got)
	}
}

func TestSavePhraseStillSavesWhenTheAIOutputIsUnusable(t *testing.T) {
	f := newPhraseFixture()
	f.writer.content = &PhraseContent{Meaning: "as little as possible", Examples: []string{"Nothing relevant here."}}

	rec := f.do(http.MethodPost, "/api/phrases", `{"sentence_id":1,"phrase":"to a minimum"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if f.phrases.saved[0].Meaning != "" {
		t.Fatalf("expected unusable output to be discarded, got %+v", f.phrases.saved[0])
	}
}

func TestSavePhraseRejectsInvalidInput(t *testing.T) {
	cases := map[string]string{
		"blank phrase":    `{"sentence_id":1,"phrase":"   "}`,
		"missing phrase":  `{"sentence_id":1}`,
		"phrase too long": `{"sentence_id":1,"phrase":"` + strings.Repeat("a", maxPhraseLength+1) + `"}`,
		"note is gone":    `{"sentence_id":1,"phrase":"ok","note":"x"}`,
		"client sentence": `{"sentence_id":1,"phrase":"ok","english":"x"}`,
		"malformed":       `{`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			f := newPhraseFixture()
			rec := f.do(http.MethodPost, "/api/phrases", body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", rec.Code)
			}
			if len(f.phrases.saved) != 0 || len(f.writer.got) != 0 {
				t.Fatalf("expected nothing saved and no AI call")
			}
		})
	}
}

func TestSavePhraseSentenceNotFound(t *testing.T) {
	f := newPhraseFixture()
	f.repo.sentenceErr = ErrNotFound

	rec := f.do(http.MethodPost, "/api/phrases", `{"sentence_id":99,"phrase":"to a minimum"}`)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if len(f.phrases.saved) != 0 || len(f.writer.got) != 0 {
		t.Fatalf("expected nothing saved and no AI call")
	}
}

func TestSavePhraseRepoError(t *testing.T) {
	f := newPhraseFixture()
	f.phrases.saveErr = errors.New("boom")

	rec := f.do(http.MethodPost, "/api/phrases", `{"sentence_id":1,"phrase":"to a minimum"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestListPhrases(t *testing.T) {
	f := newPhraseFixture()
	f.phrases.list = []SavedPhrase{
		{ID: "b", Phrase: "to a minimum", Meaning: "m", Examples: []string{"e"}, English: "hidden"},
		{ID: "a", Phrase: "in the worst case", Examples: []string{}},
	}

	rec := f.do(http.MethodGet, "/api/phrases", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if f.phrases.uids[0] != "u1" || f.phrases.listLimit != maxPhraseList {
		t.Fatalf("unexpected list call: uid=%v limit=%d", f.phrases.uids, f.phrases.listLimit)
	}
	if strings.Contains(rec.Body.String(), "hidden") {
		t.Fatalf("source sentence leaked into the list: %s", rec.Body.String())
	}
	var got ListPhrasesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Phrases) != 2 || got.Phrases[0].ID != "b" || got.Phrases[1].ID != "a" {
		t.Fatalf("unexpected list: %+v", got.Phrases)
	}
}

func TestListPhrasesEmptyIsAnArray(t *testing.T) {
	f := newPhraseFixture()
	rec := f.do(http.MethodGet, "/api/phrases", "")
	if strings.TrimSpace(rec.Body.String()) != `{"phrases":[]}` {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestListPhrasesError(t *testing.T) {
	f := newPhraseFixture()
	f.phrases.listErr = errors.New("boom")
	if rec := f.do(http.MethodGet, "/api/phrases", ""); rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestDeletePhrase(t *testing.T) {
	f := newPhraseFixture()

	rec := f.do(http.MethodDelete, "/api/phrases/abc123", "")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	// Deletes are scoped to the signed-in user's own phrases.
	if len(f.phrases.deleted) != 1 || f.phrases.deleted[0] != "abc123" || f.phrases.uids[0] != "u1" {
		t.Fatalf("unexpected delete: %v by %v", f.phrases.deleted, f.phrases.uids)
	}
}

func TestDeletePhraseNotFound(t *testing.T) {
	f := newPhraseFixture()
	f.phrases.deleteErr = ErrNotFound
	if rec := f.do(http.MethodDelete, "/api/phrases/missing", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestRewritePhrase(t *testing.T) {
	f := newPhraseFixture()
	f.phrases.get = &SavedPhrase{ID: "abc", Phrase: "to a minimum", Examples: []string{}, Japanese: "最小限", English: "minimize"}

	rec := f.do(http.MethodPost, "/api/phrases/abc/examples", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(f.writer.got) != 1 || f.writer.got[0].English != "minimize" {
		t.Fatalf("expected the stored sentence to be given to the AI, got %+v", f.writer.got)
	}
	if len(f.phrases.updated) != 1 || f.phrases.updated[0].Meaning != "as little as possible" {
		t.Fatalf("unexpected update: %+v", f.phrases.updated)
	}
	got := decodePhrase(t, rec)
	if got["id"] != "abc" || got["meaning"] != "as little as possible" || len(got["examples"].([]interface{})) != 3 {
		t.Fatalf("unexpected body: %v", got)
	}
}

func TestRewritePhraseAIFailureIsReported(t *testing.T) {
	f := newPhraseFixture()
	f.phrases.get = &SavedPhrase{ID: "abc", Phrase: "to a minimum"}
	f.writer.err = errors.New("gemini down")

	rec := f.do(http.MethodPost, "/api/phrases/abc/examples", "")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if len(f.phrases.updated) != 0 {
		t.Fatalf("expected nothing stored")
	}
}

func TestRewritePhraseNotFound(t *testing.T) {
	f := newPhraseFixture()
	f.phrases.getErr = ErrNotFound

	rec := f.do(http.MethodPost, "/api/phrases/missing/examples", "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if len(f.writer.got) != 0 {
		t.Fatalf("expected no AI call for a missing phrase")
	}
}

func TestPhrasesRouting(t *testing.T) {
	cases := []struct {
		method, path string
		want         int
	}{
		{http.MethodPut, "/api/phrases", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/phrases/abc", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/phrases/abc/examples", http.StatusMethodNotAllowed},
		{http.MethodDelete, "/api/phrases/abc/other", http.StatusNotFound},
		{http.MethodDelete, "/api/phrases/a/b/c", http.StatusNotFound},
		{http.MethodDelete, "/api/phrases/..", http.StatusNotFound},
	}
	for _, c := range cases {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			f := newPhraseFixture()
			if rec := f.do(c.method, c.path, ""); rec.Code != c.want {
				t.Fatalf("expected %d, got %d", c.want, rec.Code)
			}
			if len(f.phrases.deleted) != 0 {
				t.Fatalf("expected no delete")
			}
		})
	}
}

func TestCleanPhraseContent(t *testing.T) {
	got, err := cleanPhraseContent("don't have time", &PhraseContent{
		Meaning: "  not   free to do something ",
		Examples: []string{
			"Sorry, I don’t have time right now.", // curly apostrophe still matches
			"I DON'T HAVE TIME for this.",         // case is ignored
			"She doesn't have time.",              // does not use the phrase: dropped
			"  ",                                  // blank: dropped
			"We don't have time to cook tonight.", // kept
			"They don't have time on weekends.",   // over the limit: cut
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Meaning != "not free to do something" {
		t.Fatalf("unexpected meaning: %q", got.Meaning)
	}
	want := []string{"Sorry, I don’t have time right now.", "I DON'T HAVE TIME for this.", "We don't have time to cook tonight."}
	if len(got.Examples) != len(want) {
		t.Fatalf("unexpected examples: %q", got.Examples)
	}
	for i := range want {
		if got.Examples[i] != want[i] {
			t.Fatalf("unexpected examples: %q", got.Examples)
		}
	}
}

func TestCleanPhraseContentErrors(t *testing.T) {
	cases := map[string]*PhraseContent{
		"nil":           nil,
		"blank meaning": {Meaning: " ", Examples: []string{"to a minimum"}},
		"no examples":   {Meaning: "m", Examples: nil},
		"none match":    {Meaning: "m", Examples: []string{"minimal", "a minimum"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := cleanPhraseContent("to a minimum", c); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestBuildPhrasePrompt(t *testing.T) {
	p := buildPhrasePrompt(PhraseSource{Phrase: "to a minimum", Japanese: "最小限に", English: "We minimize it."})
	for _, want := range []string{"Phrase: to a minimum", "Japanese: 最小限に", "English: We minimize it.", "exactly 3", "English only", "No Japanese"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt missing %q:\n%s", want, p)
		}
	}
}
