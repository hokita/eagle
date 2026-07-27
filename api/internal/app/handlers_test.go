package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type recordedAnswer struct {
	uid     string
	id      int
	correct bool
	answer  string
}

type fakeRepo struct {
	random           *Sentence
	randomErr        error
	randomLevelCalls [][]int
	correct          string
	correctErr       error
	sentenceJapanese string
	sentenceEnglish  string
	sentenceErr      error
	histories        []AnswerHistory
	recorded         []recordedAnswer
	reported         []int
}

func (f *fakeRepo) RandomCandidate(_ context.Context, _ string, levels []int) (*Sentence, error) {
	f.randomLevelCalls = append(f.randomLevelCalls, levels)
	return f.random, f.randomErr
}
func (f *fakeRepo) CorrectAnswer(_ context.Context, _ int) (string, error) {
	return f.correct, f.correctErr
}
func (f *fakeRepo) GetSentence(_ context.Context, _ int) (string, string, error) {
	return f.sentenceJapanese, f.sentenceEnglish, f.sentenceErr
}
func (f *fakeRepo) ListIncorrectHistories(_ context.Context, _ string, _ int) ([]AnswerHistory, error) {
	if f.histories == nil {
		return []AnswerHistory{}, nil
	}
	return f.histories, nil
}
func (f *fakeRepo) RecordAnswer(_ context.Context, uid string, id int, correct bool, answer string) error {
	f.recorded = append(f.recorded, recordedAnswer{uid, id, correct, answer})
	return nil
}
func (f *fakeRepo) Report(_ context.Context, id int) error {
	f.reported = append(f.reported, id)
	return nil
}

type explainCall struct {
	japanese      string
	correctAnswer string
	userAnswer    string
	language      string
}

type fakeExplainer struct {
	explanation string
	err         error
	calledWith  []explainCall
}

func (f *fakeExplainer) Explain(_ context.Context, japanese, correctAnswer, userAnswer, language string) (string, error) {
	f.calledWith = append(f.calledWith, explainCall{japanese, correctAnswer, userAnswer, language})
	return f.explanation, f.err
}

func authed(req *http.Request, uid string) *http.Request {
	return req.WithContext(withUID(req.Context(), uid))
}

func TestGetRandomSentenceOK(t *testing.T) {
	repo := &fakeRepo{random: &Sentence{ID: 7, Japanese: "犬", English: "dog", Page: "3"}}
	srv := NewServer(repo, &fakeExplainer{})
	rec := httptest.NewRecorder()
	srv.getRandomSentence(rec, authed(httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got Sentence
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != 7 || got.English != "dog" {
		t.Fatalf("unexpected body: %+v", got)
	}
}

func TestGetRandomSentencePassesLevelsToRepo(t *testing.T) {
	repo := &fakeRepo{random: &Sentence{ID: 7}}
	srv := NewServer(repo, &fakeExplainer{})
	rec := httptest.NewRecorder()
	srv.getRandomSentence(rec, authed(httptest.NewRequest(http.MethodGet, "/api/sentence/random?levels=3", nil), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(repo.randomLevelCalls) != 1 || len(repo.randomLevelCalls[0]) != 1 || repo.randomLevelCalls[0][0] != 3 {
		t.Fatalf("expected repo called with levels [3], got %v", repo.randomLevelCalls)
	}
}

func TestGetRandomSentencePassesMultipleLevelsToRepo(t *testing.T) {
	repo := &fakeRepo{random: &Sentence{ID: 7}}
	srv := NewServer(repo, &fakeExplainer{})
	rec := httptest.NewRecorder()
	srv.getRandomSentence(rec, authed(httptest.NewRequest(http.MethodGet, "/api/sentence/random?levels=1,3,5", nil), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(repo.randomLevelCalls) != 1 {
		t.Fatalf("expected repo called once, got %d calls", len(repo.randomLevelCalls))
	}
	if got := repo.randomLevelCalls[0]; len(got) != 3 || got[0] != 1 || got[1] != 3 || got[2] != 5 {
		t.Fatalf("expected repo called with levels [1 3 5], got %v", got)
	}
}

func TestGetRandomSentenceDedupesLevels(t *testing.T) {
	repo := &fakeRepo{random: &Sentence{ID: 7}}
	srv := NewServer(repo, &fakeExplainer{})
	rec := httptest.NewRecorder()
	srv.getRandomSentence(rec, authed(httptest.NewRequest(http.MethodGet, "/api/sentence/random?levels=2,2,3", nil), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := repo.randomLevelCalls[0]; len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("expected deduped levels [2 3], got %v", got)
	}
}

func TestGetRandomSentenceNoLevelsDefaultsToAny(t *testing.T) {
	repo := &fakeRepo{random: &Sentence{ID: 7}}
	srv := NewServer(repo, &fakeExplainer{})
	rec := httptest.NewRecorder()
	srv.getRandomSentence(rec, authed(httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(repo.randomLevelCalls) != 1 || len(repo.randomLevelCalls[0]) != 0 {
		t.Fatalf("expected repo called with no levels, got %v", repo.randomLevelCalls)
	}
}

func TestGetRandomSentenceInvalidLevels(t *testing.T) {
	for _, levels := range []string{"0", "6", "-1", "abc", "3.5", "1,6", "1,abc", "1,,3"} {
		t.Run(levels, func(t *testing.T) {
			repo := &fakeRepo{random: &Sentence{ID: 7}}
			srv := NewServer(repo, &fakeExplainer{})
			rec := httptest.NewRecorder()
			srv.getRandomSentence(rec, authed(httptest.NewRequest(http.MethodGet, "/api/sentence/random?levels="+levels, nil), "u1"))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for levels=%q, got %d", levels, rec.Code)
			}
			if len(repo.randomLevelCalls) != 0 {
				t.Fatalf("expected repo not called for invalid levels=%q, got %v", levels, repo.randomLevelCalls)
			}
		})
	}
}

func TestGetRandomSentenceNoCandidate(t *testing.T) {
	srv := NewServer(&fakeRepo{randomErr: ErrNoCandidate}, &fakeExplainer{})
	rec := httptest.NewRecorder()
	srv.getRandomSentence(rec, authed(httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil), "u1"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestCheckAnswerCorrect(t *testing.T) {
	repo := &fakeRepo{correct: "I don't have time."}
	srv := NewServer(repo, &fakeExplainer{})
	body := `{"sentence_id":1,"user_answer":"  i don't have TIME. "}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/answer/check", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	srv.checkAnswer(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp CheckAnswerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.IsCorrect {
		t.Fatal("expected correct")
	}
	if resp.Histories == nil {
		t.Fatal("histories must not be null")
	}
	if len(repo.recorded) != 1 || repo.recorded[0].correct != true || repo.recorded[0].uid != "u1" {
		t.Fatalf("expected one correct recorded answer for u1, got %+v", repo.recorded)
	}
}

func TestCheckAnswerIncorrectRecordsAnswer(t *testing.T) {
	repo := &fakeRepo{correct: "It's hot today."}
	srv := NewServer(repo, &fakeExplainer{})
	body := `{"sentence_id":2,"user_answer":"It is hot today."}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/answer/check", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	srv.checkAnswer(rec, req)
	var resp CheckAnswerResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.IsCorrect {
		t.Fatal("expected incorrect")
	}
	if len(repo.recorded) != 1 || repo.recorded[0].answer != "It is hot today." {
		t.Fatalf("expected incorrect answer recorded, got %+v", repo.recorded)
	}
}

func TestCheckAnswerNotFound(t *testing.T) {
	srv := NewServer(&fakeRepo{correctErr: ErrNotFound}, &fakeExplainer{})
	body := `{"sentence_id":999,"user_answer":"x"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/answer/check", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	srv.checkAnswer(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestReportSentence(t *testing.T) {
	repo := &fakeRepo{}
	srv := NewServer(repo, &fakeExplainer{})
	body := `{"sentence_id":5}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/sentence/report", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	srv.reportSentence(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if len(repo.reported) != 1 || repo.reported[0] != 5 {
		t.Fatalf("expected sentence 5 reported, got %+v", repo.reported)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	srv := NewServer(&fakeRepo{}, &fakeExplainer{})
	rec := httptest.NewRecorder()
	srv.getRandomSentence(rec, authed(httptest.NewRequest(http.MethodPost, "/api/sentence/random", nil), "u1"))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestExplainAnswerOK(t *testing.T) {
	explainer := &fakeExplainer{explanation: "Your answer is also natural; the reference is just more formal."}
	repo := &fakeRepo{sentenceJapanese: "時間がありません。", sentenceEnglish: "I don't have time."}
	srv := NewServer(repo, explainer)
	body := `{"sentence_id":1,"user_answer":"I have no time.","language":"en"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/answer/explain", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	srv.explainAnswer(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp ExplainResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Explanation != explainer.explanation {
		t.Fatalf("unexpected explanation: %q", resp.Explanation)
	}
	if len(explainer.calledWith) != 1 {
		t.Fatalf("expected Explain called once, got %d", len(explainer.calledWith))
	}
	call := explainer.calledWith[0]
	if call.japanese != "時間がありません。" || call.correctAnswer != "I don't have time." || call.userAnswer != "I have no time." || call.language != "en" {
		t.Fatalf("unexpected call args: %+v", call)
	}
}

// TestExplainAnswerIgnoresClientSuppliedSentenceData is a regression test for
// a security finding: the japanese/correct_answer fields used to come
// straight from the client and were sent to Gemini unverified. The server
// now looks these up from the repository by sentence_id, and any client
// still sending the old japanese/correct_answer fields is rejected outright
// (DisallowUnknownFields) rather than having them silently ignored.
func TestExplainAnswerIgnoresClientSuppliedSentenceData(t *testing.T) {
	explainer := &fakeExplainer{explanation: "explanation"}
	repo := &fakeRepo{sentenceJapanese: "本物の文", sentenceEnglish: "the real sentence"}
	srv := NewServer(repo, explainer)
	body := `{"sentence_id":1,"japanese":"injected","correct_answer":"injected","user_answer":"my answer"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/answer/explain", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	srv.explainAnswer(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown fields, got %d", rec.Code)
	}
	if len(explainer.calledWith) != 0 {
		t.Fatal("explainer should not be called when the request is rejected")
	}
}

func TestExplainAnswerSentenceNotFound(t *testing.T) {
	explainer := &fakeExplainer{}
	repo := &fakeRepo{sentenceErr: ErrNotFound}
	srv := NewServer(repo, explainer)
	body := `{"sentence_id":999,"user_answer":"x","language":"en"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/answer/explain", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	srv.explainAnswer(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if len(explainer.calledWith) != 0 {
		t.Fatal("explainer should not be called when sentence lookup fails")
	}
}

func TestExplainAnswerEmptyUserAnswer(t *testing.T) {
	explainer := &fakeExplainer{}
	repo := &fakeRepo{sentenceJapanese: "x", sentenceEnglish: "y"}
	srv := NewServer(repo, explainer)
	body := `{"sentence_id":1,"user_answer":"   "}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/answer/explain", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	srv.explainAnswer(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if len(explainer.calledWith) != 0 {
		t.Fatal("explainer should not be called for an empty user_answer")
	}
}

func TestExplainAnswerUserAnswerTooLong(t *testing.T) {
	explainer := &fakeExplainer{}
	repo := &fakeRepo{sentenceJapanese: "x", sentenceEnglish: "y"}
	srv := NewServer(repo, explainer)
	longAnswer := strings.Repeat("a", maxUserAnswerLength+1)
	bodyBytes, err := json.Marshal(map[string]any{"sentence_id": 1, "user_answer": longAnswer})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := authed(httptest.NewRequest(http.MethodPost, "/api/answer/explain", bytes.NewReader(bodyBytes)), "u1")
	rec := httptest.NewRecorder()
	srv.explainAnswer(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if len(explainer.calledWith) != 0 {
		t.Fatal("explainer should not be called for an over-length user_answer")
	}
}

func TestExplainAnswerBodyTooLarge(t *testing.T) {
	explainer := &fakeExplainer{}
	repo := &fakeRepo{sentenceJapanese: "x", sentenceEnglish: "y"}
	srv := NewServer(repo, explainer)
	huge := strings.Repeat("a", maxExplainRequestBytes+1)
	body := `{"sentence_id":1,"user_answer":"` + huge + `"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/answer/explain", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	srv.explainAnswer(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if len(explainer.calledWith) != 0 {
		t.Fatal("explainer should not be called for an oversized request body")
	}
}

func TestExplainAnswerLLMError(t *testing.T) {
	explainer := &fakeExplainer{err: errors.New("gemini unavailable")}
	repo := &fakeRepo{sentenceJapanese: "x", sentenceEnglish: "y"}
	srv := NewServer(repo, explainer)
	body := `{"sentence_id":1,"user_answer":"z","language":"en"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/answer/explain", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	srv.explainAnswer(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestExplainAnswerLanguageJapanese(t *testing.T) {
	explainer := &fakeExplainer{explanation: "日本語での説明。"}
	repo := &fakeRepo{sentenceJapanese: "時間がありません。", sentenceEnglish: "I don't have time."}
	srv := NewServer(repo, explainer)
	body := `{"sentence_id":1,"user_answer":"I have no time.","language":"ja"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/answer/explain", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	srv.explainAnswer(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(explainer.calledWith) != 1 || explainer.calledWith[0].language != "ja" {
		t.Fatalf("expected Explain called with language=ja, got %+v", explainer.calledWith)
	}
}

func TestExplainAnswerInvalidLanguage(t *testing.T) {
	explainer := &fakeExplainer{}
	repo := &fakeRepo{sentenceJapanese: "x", sentenceEnglish: "y"}
	srv := NewServer(repo, explainer)
	body := `{"sentence_id":1,"user_answer":"z","language":"fr"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/answer/explain", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	srv.explainAnswer(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if len(explainer.calledWith) != 0 {
		t.Fatal("explainer should not be called for an invalid language")
	}
}

func TestExplainAnswerMissingLanguage(t *testing.T) {
	explainer := &fakeExplainer{}
	repo := &fakeRepo{sentenceJapanese: "x", sentenceEnglish: "y"}
	srv := NewServer(repo, explainer)
	body := `{"sentence_id":1,"user_answer":"z"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/answer/explain", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	srv.explainAnswer(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if len(explainer.calledWith) != 0 {
		t.Fatal("explainer should not be called when language is missing")
	}
}

func TestExplainAnswerMethodNotAllowed(t *testing.T) {
	srv := NewServer(&fakeRepo{}, &fakeExplainer{})
	rec := httptest.NewRecorder()
	srv.explainAnswer(rec, authed(httptest.NewRequest(http.MethodGet, "/api/answer/explain", nil), "u1"))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
