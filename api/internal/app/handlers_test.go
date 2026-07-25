package app

import (
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
	random     *Sentence
	randomErr  error
	correct    string
	correctErr error
	histories  []AnswerHistory
	recorded   []recordedAnswer
	reported   []int
}

func (f *fakeRepo) RandomCandidate(_ context.Context, _ string) (*Sentence, error) {
	return f.random, f.randomErr
}
func (f *fakeRepo) CorrectAnswer(_ context.Context, _ int) (string, error) {
	return f.correct, f.correctErr
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
}

type fakeExplainer struct {
	explanation string
	err         error
	calledWith  []explainCall
}

func (f *fakeExplainer) Explain(_ context.Context, japanese, correctAnswer, userAnswer string) (string, error) {
	f.calledWith = append(f.calledWith, explainCall{japanese, correctAnswer, userAnswer})
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
	srv := NewServer(&fakeRepo{}, explainer)
	body := `{"japanese":"時間がありません。","correct_answer":"I don't have time.","user_answer":"I have no time."}`
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
	if call.japanese != "時間がありません。" || call.correctAnswer != "I don't have time." || call.userAnswer != "I have no time." {
		t.Fatalf("unexpected call args: %+v", call)
	}
}

func TestExplainAnswerLLMError(t *testing.T) {
	explainer := &fakeExplainer{err: errors.New("gemini unavailable")}
	srv := NewServer(&fakeRepo{}, explainer)
	body := `{"japanese":"x","correct_answer":"y","user_answer":"z"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/answer/explain", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	srv.explainAnswer(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
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
