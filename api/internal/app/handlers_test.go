package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	randomUIDs       []string
	correct          string
	correctErr       error
	sentenceJapanese string
	sentenceEnglish  string
	sentenceErr      error
	histories        []AnswerHistory
	recorded         []recordedAnswer
	reported         []int
	reportErr        error
	mistakes         []MistakeSentence
	mistakesErr      error

	listMistakesCalls           int
	listMistakesForInsightCalls int
}

func (f *fakeRepo) RandomCandidate(_ context.Context, uid string, levels []int) (*Sentence, error) {
	f.randomUIDs = append(f.randomUIDs, uid)
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
func (f *fakeRepo) ListMistakes(_ context.Context, _ string) ([]MistakeSentence, error) {
	f.listMistakesCalls++
	if f.mistakesErr != nil {
		return nil, f.mistakesErr
	}
	if f.mistakes == nil {
		return []MistakeSentence{}, nil
	}
	return f.mistakes, nil
}

func (f *fakeRepo) ListMistakesForInsight(_ context.Context, _ string) ([]MistakeSentence, error) {
	f.listMistakesForInsightCalls++
	if f.mistakesErr != nil {
		return nil, f.mistakesErr
	}
	if f.mistakes == nil {
		return []MistakeSentence{}, nil
	}
	return f.mistakes, nil
}
func (f *fakeRepo) RecordAnswer(_ context.Context, uid string, id int, correct bool, answer string) error {
	f.recorded = append(f.recorded, recordedAnswer{uid, id, correct, answer})
	return nil
}
func (f *fakeRepo) Report(_ context.Context, id int) error {
	if f.reportErr != nil {
		return f.reportErr
	}
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

	followUpAnswer string
	followUpErr    error
	followUpCalls  []FollowUpInput
}

func (f *fakeExplainer) Explain(_ context.Context, japanese, correctAnswer, userAnswer, language string) (string, error) {
	f.calledWith = append(f.calledWith, explainCall{japanese, correctAnswer, userAnswer, language})
	return f.explanation, f.err
}

func (f *fakeExplainer) AnswerFollowUp(_ context.Context, in FollowUpInput) (string, error) {
	f.followUpCalls = append(f.followUpCalls, in)
	return f.followUpAnswer, f.followUpErr
}

type analyzeCall struct {
	mistakes []MistakeSentence
	language string
}

type fakeAnalyzer struct {
	insight    string
	err        error
	calledWith []analyzeCall
}

func (f *fakeAnalyzer) Analyze(_ context.Context, mistakes []MistakeSentence, language string) (string, error) {
	f.calledWith = append(f.calledWith, analyzeCall{mistakes, language})
	return f.insight, f.err
}

func authed(req *http.Request, uid string) *http.Request {
	return req.WithContext(withUID(req.Context(), uid))
}

func TestGetRandomSentenceOK(t *testing.T) {
	repo := &fakeRepo{random: &Sentence{ID: 7, Japanese: "犬", English: "dog", Page: "3"}}
	srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})
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
	srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})
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
	srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})
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
	srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})
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
	srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})
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
			srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})
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
	srv := NewServer(&fakeRepo{randomErr: ErrNoCandidate}, &fakeExplainer{}, &fakeAnalyzer{})
	rec := httptest.NewRecorder()
	srv.getRandomSentence(rec, authed(httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil), "u1"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestCheckAnswerCorrect(t *testing.T) {
	repo := &fakeRepo{correct: "I don't have time."}
	srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})
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

func TestCheckAnswerCorrectDespiteInternalWhitespaceDifference(t *testing.T) {
	repo := &fakeRepo{correct: "Do we have to read these books? Yes, you do."}
	srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})
	body := `{"sentence_id":1,"user_answer":"Do we have to read these books?\nYes, you do."}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/answer/check", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	srv.checkAnswer(rec, req)
	var resp CheckAnswerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.IsCorrect {
		t.Fatal("expected correct despite newline instead of space")
	}
	if len(repo.recorded) != 1 || repo.recorded[0].correct != true {
		t.Fatalf("expected correct answer recorded, got %+v", repo.recorded)
	}
}

func TestCheckAnswerCorrectDespitePunctuationDifference(t *testing.T) {
	for name, tc := range map[string]struct{ correct, user string }{
		"curly apostrophe for straight":  {"It's always warm in the country.", "It’s always warm in the country."},
		"straight apostrophe for curly":  {"It’s always warm in the country.", "It's always warm in the country."},
		"missing final period":           {"It's always warm in the country.", "It's always warm in the country"},
		"both, as typed on a phone":      {"It's always warm in the country.", "It’s always warm in the country"},
		"ideographic period from an IME": {"It's always warm in the country.", "It's always warm in the country。"},
		"curly quotes around a quote":    {`He said "no".`, `He said “no”.`},
		"em dash for hyphen":             {"It's a well-known book.", "It's a well—known book."},
		"space before the final period":  {"It's always warm in the country.", "It's always warm in the country ."},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepo{correct: tc.correct}
			srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})
			bodyBytes, err := json.Marshal(map[string]any{"sentence_id": 1, "user_answer": tc.user})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			req := authed(httptest.NewRequest(http.MethodPost, "/api/answer/check", bytes.NewReader(bodyBytes)), "u1")
			rec := httptest.NewRecorder()
			srv.checkAnswer(rec, req)
			var resp CheckAnswerResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !resp.IsCorrect {
				t.Fatalf("expected %q to be graded correct against %q", tc.user, tc.correct)
			}
			if len(repo.recorded) != 1 || repo.recorded[0].correct != true {
				t.Fatalf("expected correct answer recorded, got %+v", repo.recorded)
			}
		})
	}
}

func TestCheckAnswerIncorrectDespitePunctuationNormalization(t *testing.T) {
	for name, tc := range map[string]struct{ correct, user string }{
		"missing internal comma":     {"Yes, you do.", "Yes you do."},
		"wrong word":                 {"It's always warm in the country.", "It's always warm in the town."},
		"dropped word":               {"It's always warm in the country.", "It's warm in the country"},
		"answer is punctuation only": {"It's always warm in the country.", "."},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepo{correct: tc.correct}
			srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})
			bodyBytes, err := json.Marshal(map[string]any{"sentence_id": 1, "user_answer": tc.user})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			req := authed(httptest.NewRequest(http.MethodPost, "/api/answer/check", bytes.NewReader(bodyBytes)), "u1")
			rec := httptest.NewRecorder()
			srv.checkAnswer(rec, req)
			var resp CheckAnswerResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if resp.IsCorrect {
				t.Fatalf("expected %q to be graded incorrect against %q", tc.user, tc.correct)
			}
			if len(repo.recorded) != 1 || repo.recorded[0].answer != tc.user {
				t.Fatalf("expected the answer recorded verbatim, got %+v", repo.recorded)
			}
		})
	}
}

func TestCheckAnswerIncorrectRecordsAnswer(t *testing.T) {
	repo := &fakeRepo{correct: "It's hot today."}
	srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})
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

func TestCheckAnswerUserAnswerTooLong(t *testing.T) {
	repo := &fakeRepo{correct: "It's hot today."}
	srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})
	longAnswer := strings.Repeat("a", maxUserAnswerLength+1)
	bodyBytes, err := json.Marshal(map[string]any{"sentence_id": 2, "user_answer": longAnswer})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := authed(httptest.NewRequest(http.MethodPost, "/api/answer/check", bytes.NewReader(bodyBytes)), "u1")
	rec := httptest.NewRecorder()
	srv.checkAnswer(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if len(repo.recorded) != 0 {
		t.Fatalf("expected no answer recorded for an over-length user_answer, got %+v", repo.recorded)
	}
}

func TestCheckAnswerNotFound(t *testing.T) {
	srv := NewServer(&fakeRepo{correctErr: ErrNotFound}, &fakeExplainer{}, &fakeAnalyzer{})
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
	srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})
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

func TestReportSentenceNotFound(t *testing.T) {
	repo := &fakeRepo{reportErr: ErrNotFound}
	srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})
	req := authed(httptest.NewRequest(http.MethodPost, "/api/sentence/report", strings.NewReader(`{"sentence_id":999}`)), "u1")
	rec := httptest.NewRecorder()
	srv.reportSentence(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "Sentence not found" {
		t.Fatalf("unexpected body %q", got)
	}
}

func TestCheckAndRecordGradesAndRecords(t *testing.T) {
	repo := &fakeRepo{correct: "It's fine."}
	srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})

	res, err := srv.checkAndRecord(context.Background(), "u1", 3, "it’s fine")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsCorrect {
		t.Fatalf("expected correct, got %+v", res)
	}
	res, err = srv.checkAndRecord(context.Background(), "u1", 3, "wrong")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsCorrect {
		t.Fatalf("expected incorrect, got %+v", res)
	}
	want := []recordedAnswer{{"u1", 3, true, ""}, {"u1", 3, false, "wrong"}}
	if len(repo.recorded) != 2 || repo.recorded[0] != want[0] || repo.recorded[1] != want[1] {
		t.Fatalf("recorded %+v, want %+v", repo.recorded, want)
	}
}

func TestCheckAndRecordUnknownSentence(t *testing.T) {
	repo := &fakeRepo{correctErr: ErrNotFound}
	srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})
	_, err := srv.checkAndRecord(context.Background(), "u1", 999, "x")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if len(repo.recorded) != 0 {
		t.Fatalf("expected nothing recorded, got %+v", repo.recorded)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	srv := NewServer(&fakeRepo{}, &fakeExplainer{}, &fakeAnalyzer{})
	rec := httptest.NewRecorder()
	srv.getRandomSentence(rec, authed(httptest.NewRequest(http.MethodPost, "/api/sentence/random", nil), "u1"))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestExplainAnswerOK(t *testing.T) {
	explainer := &fakeExplainer{explanation: "Your answer is also natural; the reference is just more formal."}
	repo := &fakeRepo{sentenceJapanese: "時間がありません。", sentenceEnglish: "I don't have time."}
	srv := NewServer(repo, explainer, &fakeAnalyzer{})
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

func TestExplainAnswerIgnoresClientSuppliedSentenceData(t *testing.T) {
	explainer := &fakeExplainer{explanation: "explanation"}
	repo := &fakeRepo{sentenceJapanese: "本物の文", sentenceEnglish: "the real sentence"}
	srv := NewServer(repo, explainer, &fakeAnalyzer{})
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
	srv := NewServer(repo, explainer, &fakeAnalyzer{})
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
	srv := NewServer(repo, explainer, &fakeAnalyzer{})
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
	srv := NewServer(repo, explainer, &fakeAnalyzer{})
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
	srv := NewServer(repo, explainer, &fakeAnalyzer{})
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

func followUpBody(t *testing.T, overrides map[string]any) *bytes.Reader {
	t.Helper()
	body := map[string]any{
		"sentence_id": 1,
		"user_answer": "I have no time.",
		"language":    "en",
		"explanation": "The reference uses do-support.",
		"question":    "Why is \"no time\" less natural?",
	}
	for k, v := range overrides {
		body[k] = v
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return bytes.NewReader(raw)
}

func postFollowUp(srv *Server, body *bytes.Reader) *httptest.ResponseRecorder {
	req := authed(httptest.NewRequest(http.MethodPost, "/api/answer/followup", body), "u1")
	rec := httptest.NewRecorder()
	srv.followUpAnswer(rec, req)
	return rec
}

func TestFollowUpAnswerOK(t *testing.T) {
	explainer := &fakeExplainer{followUpAnswer: "Because English marks negation on the verb here."}
	repo := &fakeRepo{sentenceJapanese: "時間がありません。", sentenceEnglish: "I don't have time."}
	srv := NewServer(repo, explainer, &fakeAnalyzer{})

	rec := postFollowUp(srv, followUpBody(t, map[string]any{
		"history": []map[string]string{{"question": "Is my answer wrong?", "answer": "Not wrong, just less common."}},
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp FollowUpResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Answer != explainer.followUpAnswer {
		t.Fatalf("unexpected answer: %q", resp.Answer)
	}
	if len(explainer.followUpCalls) != 1 {
		t.Fatalf("expected AnswerFollowUp called once, got %d", len(explainer.followUpCalls))
	}
	in := explainer.followUpCalls[0]
	if in.Japanese != "時間がありません。" || in.CorrectAnswer != "I don't have time." {
		t.Fatalf("expected sentence loaded from the repository, got %+v", in)
	}
	if in.UserAnswer != "I have no time." || in.Language != "en" {
		t.Fatalf("unexpected input: %+v", in)
	}
	if in.Explanation != "The reference uses do-support." || in.Question != `Why is "no time" less natural?` {
		t.Fatalf("unexpected input: %+v", in)
	}
	if len(in.History) != 1 || in.History[0].Question != "Is my answer wrong?" {
		t.Fatalf("expected the earlier turn passed through, got %+v", in.History)
	}
}

func TestFollowUpAnswerRejectsClientSuppliedSentenceData(t *testing.T) {
	explainer := &fakeExplainer{followUpAnswer: "answer"}
	repo := &fakeRepo{sentenceJapanese: "本物の文", sentenceEnglish: "the real sentence"}
	srv := NewServer(repo, explainer, &fakeAnalyzer{})

	rec := postFollowUp(srv, followUpBody(t, map[string]any{"japanese": "injected", "correct_answer": "injected"}))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown fields, got %d", rec.Code)
	}
	if len(explainer.followUpCalls) != 0 {
		t.Fatal("explainer should not be called when the request is rejected")
	}
}

func TestFollowUpAnswerInvalidRequests(t *testing.T) {
	longQuestion := strings.Repeat("あ", maxFollowUpQuestionLength+1)
	longAnswer := strings.Repeat("a", maxFollowUpAnswerLength+1)

	tests := []struct {
		name      string
		overrides map[string]any
	}{
		{"blank question", map[string]any{"question": "   "}},
		{"question too long", map[string]any{"question": longQuestion}},
		{"blank explanation", map[string]any{"explanation": ""}},
		{"explanation too long", map[string]any{"explanation": longAnswer}},
		{"blank user answer", map[string]any{"user_answer": "  "}},
		{"user answer too long", map[string]any{"user_answer": strings.Repeat("a", maxUserAnswerLength+1)}},
		{"unknown language", map[string]any{"language": "fr"}},
		{"missing language", map[string]any{"language": ""}},
		{"blank history answer", map[string]any{"history": []map[string]string{{"question": "q", "answer": " "}}}},
		{"blank history question", map[string]any{"history": []map[string]string{{"question": "", "answer": "a"}}}},
		{"history answer too long", map[string]any{"history": []map[string]string{{"question": "q", "answer": longAnswer}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			explainer := &fakeExplainer{}
			repo := &fakeRepo{sentenceJapanese: "x", sentenceEnglish: "y"}
			srv := NewServer(repo, explainer, &fakeAnalyzer{})

			rec := postFollowUp(srv, followUpBody(t, tt.overrides))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", rec.Code)
			}
			if len(explainer.followUpCalls) != 0 {
				t.Fatal("explainer should not be called for an invalid request")
			}
		})
	}
}

func TestFollowUpAnswerKeepsOnlyTheMostRecentHistory(t *testing.T) {
	explainer := &fakeExplainer{followUpAnswer: "answer"}
	repo := &fakeRepo{sentenceJapanese: "x", sentenceEnglish: "y"}
	srv := NewServer(repo, explainer, &fakeAnalyzer{})
	history := make([]map[string]string, maxFollowUpHistory+2)
	for i := range history {
		history[i] = map[string]string{"question": fmt.Sprintf("q%d", i), "answer": fmt.Sprintf("a%d", i)}
	}

	rec := postFollowUp(srv, followUpBody(t, map[string]any{"history": history}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	got := explainer.followUpCalls[0].History
	if len(got) != maxFollowUpHistory {
		t.Fatalf("expected %d turns, got %d", maxFollowUpHistory, len(got))
	}
	if got[0].Question != "q2" || got[len(got)-1].Question != fmt.Sprintf("q%d", len(history)-1) {
		t.Fatalf("expected the most recent turns, got %+v", got)
	}
}

func TestFollowUpAnswerSentenceNotFound(t *testing.T) {
	explainer := &fakeExplainer{}
	srv := NewServer(&fakeRepo{sentenceErr: ErrNotFound}, explainer, &fakeAnalyzer{})

	rec := postFollowUp(srv, followUpBody(t, nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if len(explainer.followUpCalls) != 0 {
		t.Fatal("explainer should not be called when sentence lookup fails")
	}
}

func TestFollowUpAnswerExplainerError(t *testing.T) {
	repo := &fakeRepo{sentenceJapanese: "x", sentenceEnglish: "y"}
	srv := NewServer(repo, &fakeExplainer{followUpErr: errors.New("gemini down")}, &fakeAnalyzer{})

	rec := postFollowUp(srv, followUpBody(t, nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestFollowUpAnswerEmptyAnswer(t *testing.T) {
	repo := &fakeRepo{sentenceJapanese: "x", sentenceEnglish: "y"}
	srv := NewServer(repo, &fakeExplainer{followUpAnswer: "   "}, &fakeAnalyzer{})

	rec := postFollowUp(srv, followUpBody(t, nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestFollowUpAnswerMethodNotAllowed(t *testing.T) {
	srv := NewServer(&fakeRepo{}, &fakeExplainer{}, &fakeAnalyzer{})
	rec := httptest.NewRecorder()

	srv.followUpAnswer(rec, authed(httptest.NewRequest(http.MethodGet, "/api/answer/followup", nil), "u1"))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestFollowUpAnswerBodyTooLarge(t *testing.T) {
	explainer := &fakeExplainer{}
	repo := &fakeRepo{sentenceJapanese: "x", sentenceEnglish: "y"}
	srv := NewServer(repo, explainer, &fakeAnalyzer{})
	huge := strings.Repeat("a", maxFollowUpRequestBytes+1)

	rec := postFollowUp(srv, followUpBody(t, map[string]any{"explanation": huge}))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if len(explainer.followUpCalls) != 0 {
		t.Fatal("explainer should not be called for an oversized request body")
	}
}

func TestExplainAnswerLLMError(t *testing.T) {
	explainer := &fakeExplainer{err: errors.New("gemini unavailable")}
	repo := &fakeRepo{sentenceJapanese: "x", sentenceEnglish: "y"}
	srv := NewServer(repo, explainer, &fakeAnalyzer{})
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
	srv := NewServer(repo, explainer, &fakeAnalyzer{})
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
	srv := NewServer(repo, explainer, &fakeAnalyzer{})
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
	srv := NewServer(repo, explainer, &fakeAnalyzer{})
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
	srv := NewServer(&fakeRepo{}, &fakeExplainer{}, &fakeAnalyzer{})
	rec := httptest.NewRecorder()
	srv.explainAnswer(rec, authed(httptest.NewRequest(http.MethodGet, "/api/answer/explain", nil), "u1"))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestGetMistakesOK(t *testing.T) {
	repo := &fakeRepo{mistakes: []MistakeSentence{
		{
			SentenceID:    701,
			Japanese:      "時間がありません。",
			CorrectAnswer: "I don't have time.",
			WrongAnswers: []AnswerHistory{
				{ID: 1, IncorrectAnswer: "I have no time.", CreatedAt: "2026-01-03T00:00:00Z"},
			},
		},
	}}
	srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})
	rec := httptest.NewRecorder()
	srv.getMistakes(rec, authed(httptest.NewRequest(http.MethodGet, "/api/mistakes", nil), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp ListMistakesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Mistakes) != 1 || resp.Mistakes[0].SentenceID != 701 {
		t.Fatalf("unexpected body: %+v", resp)
	}
	if len(resp.Mistakes[0].WrongAnswers) != 1 || resp.Mistakes[0].WrongAnswers[0].IncorrectAnswer != "I have no time." {
		t.Fatalf("unexpected wrong answers: %+v", resp.Mistakes[0])
	}
	if repo.listMistakesCalls != 1 || repo.listMistakesForInsightCalls != 0 {
		t.Fatalf("expected ListMistakes called once and ListMistakesForInsight not called, got ListMistakes=%d, ForInsight=%d",
			repo.listMistakesCalls, repo.listMistakesForInsightCalls)
	}
}

func TestGetMistakesEmpty(t *testing.T) {
	srv := NewServer(&fakeRepo{}, &fakeExplainer{}, &fakeAnalyzer{})
	rec := httptest.NewRecorder()
	srv.getMistakes(rec, authed(httptest.NewRequest(http.MethodGet, "/api/mistakes", nil), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp ListMistakesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Mistakes == nil {
		t.Fatal("mistakes must not be null")
	}
	if len(resp.Mistakes) != 0 {
		t.Fatalf("expected empty list, got %+v", resp.Mistakes)
	}
}

func TestGetMistakesRepoError(t *testing.T) {
	srv := NewServer(&fakeRepo{mistakesErr: errors.New("boom")}, &fakeExplainer{}, &fakeAnalyzer{})
	rec := httptest.NewRecorder()
	srv.getMistakes(rec, authed(httptest.NewRequest(http.MethodGet, "/api/mistakes", nil), "u1"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestGetMistakesMethodNotAllowed(t *testing.T) {
	srv := NewServer(&fakeRepo{}, &fakeExplainer{}, &fakeAnalyzer{})
	rec := httptest.NewRecorder()
	srv.getMistakes(rec, authed(httptest.NewRequest(http.MethodPost, "/api/mistakes", nil), "u1"))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestGetMistakesInsightOK(t *testing.T) {
	analyzer := &fakeAnalyzer{insight: "You often drop articles like 'the'."}
	repo := &fakeRepo{mistakes: []MistakeSentence{
		{SentenceID: 1, Japanese: "犬", CorrectAnswer: "a dog", WrongAnswers: []AnswerHistory{{ID: 1, IncorrectAnswer: "dog"}}},
	}}
	srv := NewServer(repo, &fakeExplainer{}, analyzer)
	rec := httptest.NewRecorder()
	srv.getMistakesInsight(rec, authed(httptest.NewRequest(http.MethodGet, "/api/mistakes/insight?language=en", nil), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp MistakesInsightResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Insight != analyzer.insight {
		t.Fatalf("unexpected insight: %q", resp.Insight)
	}
	if len(analyzer.calledWith) != 1 || analyzer.calledWith[0].language != "en" {
		t.Fatalf("expected Analyze called once with language=en, got %+v", analyzer.calledWith)
	}
	if repo.listMistakesForInsightCalls != 1 || repo.listMistakesCalls != 0 {
		t.Fatalf("expected ListMistakesForInsight called once and ListMistakes not called, got ForInsight=%d, ListMistakes=%d",
			repo.listMistakesForInsightCalls, repo.listMistakesCalls)
	}
}

func TestGetMistakesInsightEmptySkipsAnalyzer(t *testing.T) {
	analyzer := &fakeAnalyzer{insight: "should not be used"}
	srv := NewServer(&fakeRepo{}, &fakeExplainer{}, analyzer)
	rec := httptest.NewRecorder()
	srv.getMistakesInsight(rec, authed(httptest.NewRequest(http.MethodGet, "/api/mistakes/insight?language=en", nil), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp MistakesInsightResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Insight != "" {
		t.Fatalf("expected empty insight, got %q", resp.Insight)
	}
	if len(analyzer.calledWith) != 0 {
		t.Fatal("analyzer must not be called when there are no mistakes")
	}
}

func TestGetMistakesInsightCapsToMostRecent(t *testing.T) {
	many := make([]MistakeSentence, maxInsightMistakes+10)
	for i := range many {
		many[i] = MistakeSentence{SentenceID: i, Japanese: "x", CorrectAnswer: "y", WrongAnswers: []AnswerHistory{{IncorrectAnswer: "z"}}}
	}
	analyzer := &fakeAnalyzer{insight: "ok"}
	srv := NewServer(&fakeRepo{mistakes: many}, &fakeExplainer{}, analyzer)
	rec := httptest.NewRecorder()
	srv.getMistakesInsight(rec, authed(httptest.NewRequest(http.MethodGet, "/api/mistakes/insight?language=en", nil), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(analyzer.calledWith) != 1 || len(analyzer.calledWith[0].mistakes) != maxInsightMistakes {
		t.Fatalf("expected analyzer called with %d mistakes, got %d", maxInsightMistakes, len(analyzer.calledWith[0].mistakes))
	}
}

func TestGetMistakesInsightRepoError(t *testing.T) {
	srv := NewServer(&fakeRepo{mistakesErr: errors.New("boom")}, &fakeExplainer{}, &fakeAnalyzer{})
	rec := httptest.NewRecorder()
	srv.getMistakesInsight(rec, authed(httptest.NewRequest(http.MethodGet, "/api/mistakes/insight?language=en", nil), "u1"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestGetMistakesInsightAnalyzerError(t *testing.T) {
	repo := &fakeRepo{mistakes: []MistakeSentence{{SentenceID: 1, Japanese: "x", CorrectAnswer: "y", WrongAnswers: []AnswerHistory{{IncorrectAnswer: "z"}}}}}
	srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{err: errors.New("gemini down")})
	rec := httptest.NewRecorder()
	srv.getMistakesInsight(rec, authed(httptest.NewRequest(http.MethodGet, "/api/mistakes/insight?language=en", nil), "u1"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestGetMistakesInsightEmptyAnalyzerResultIsTreatedAsError(t *testing.T) {
	repo := &fakeRepo{mistakes: []MistakeSentence{{SentenceID: 1, Japanese: "x", CorrectAnswer: "y", WrongAnswers: []AnswerHistory{{IncorrectAnswer: "z"}}}}}
	analyzer := &fakeAnalyzer{insight: ""}
	srv := NewServer(repo, &fakeExplainer{}, analyzer)
	rec := httptest.NewRecorder()
	srv.getMistakesInsight(rec, authed(httptest.NewRequest(http.MethodGet, "/api/mistakes/insight?language=en", nil), "u1"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if len(analyzer.calledWith) != 1 {
		t.Fatalf("expected analyzer to be called once, got %d", len(analyzer.calledWith))
	}
}

func TestGetMistakesInsightWhitespaceOnlyAnalyzerResultIsTreatedAsError(t *testing.T) {
	repo := &fakeRepo{mistakes: []MistakeSentence{{SentenceID: 1, Japanese: "x", CorrectAnswer: "y", WrongAnswers: []AnswerHistory{{IncorrectAnswer: "z"}}}}}
	analyzer := &fakeAnalyzer{insight: "\n\n "}
	srv := NewServer(repo, &fakeExplainer{}, analyzer)
	rec := httptest.NewRecorder()
	srv.getMistakesInsight(rec, authed(httptest.NewRequest(http.MethodGet, "/api/mistakes/insight?language=en", nil), "u1"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestGetMistakesInsightInvalidLanguage(t *testing.T) {
	analyzer := &fakeAnalyzer{}
	repo := &fakeRepo{mistakes: []MistakeSentence{{SentenceID: 1, Japanese: "x", CorrectAnswer: "y", WrongAnswers: []AnswerHistory{{IncorrectAnswer: "z"}}}}}
	srv := NewServer(repo, &fakeExplainer{}, analyzer)
	rec := httptest.NewRecorder()
	srv.getMistakesInsight(rec, authed(httptest.NewRequest(http.MethodGet, "/api/mistakes/insight?language=fr", nil), "u1"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if len(analyzer.calledWith) != 0 {
		t.Fatal("analyzer must not be called for an invalid language")
	}
}

func TestGetMistakesInsightMethodNotAllowed(t *testing.T) {
	srv := NewServer(&fakeRepo{}, &fakeExplainer{}, &fakeAnalyzer{})
	rec := httptest.NewRecorder()
	srv.getMistakesInsight(rec, authed(httptest.NewRequest(http.MethodPost, "/api/mistakes/insight", nil), "u1"))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
