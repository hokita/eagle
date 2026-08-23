package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type savedSessionCall struct {
	uid     string
	session *DiscussionSession
}

type fakeDiscussionRepo struct {
	question    *DiscussionQuestion
	questionErr error
	savedID     string
	saveErr     error
	saved       []savedSessionCall
	summaries   []DiscussionSessionSummary
	listErr     error
	session     *DiscussionSession
	sessionErr  error
}

func (f *fakeDiscussionRepo) RandomQuestion(_ context.Context) (*DiscussionQuestion, error) {
	return f.question, f.questionErr
}
func (f *fakeDiscussionRepo) GetQuestion(_ context.Context, _ int) (*DiscussionQuestion, error) {
	return f.question, f.questionErr
}
func (f *fakeDiscussionRepo) SaveSession(_ context.Context, uid string, s *DiscussionSession) (string, error) {
	f.saved = append(f.saved, savedSessionCall{uid, s})
	return f.savedID, f.saveErr
}
func (f *fakeDiscussionRepo) ListSessions(_ context.Context, _ string, _ int) ([]DiscussionSessionSummary, error) {
	if f.summaries == nil {
		return []DiscussionSessionSummary{}, f.listErr
	}
	return f.summaries, f.listErr
}
func (f *fakeDiscussionRepo) GetSession(_ context.Context, _, _ string) (*DiscussionSession, error) {
	return f.session, f.sessionErr
}

type fakeCoach struct {
	reply       *CoachReply
	replyErr    error
	replyCalls  int
	analysis    *GapAnalysis
	analyzeErr  error
	feedback    string
	reviewErr   error
	reviewCalls int
}

func (f *fakeCoach) Reply(_ context.Context, _ *DiscussionQuestion, _ []DiscussionMessage) (*CoachReply, error) {
	f.replyCalls++
	return f.reply, f.replyErr
}
func (f *fakeCoach) AnalyzeGap(_ context.Context, _ *DiscussionQuestion, _ []DiscussionMessage, _ string) (*GapAnalysis, error) {
	return f.analysis, f.analyzeErr
}
func (f *fakeCoach) ReviewRetry(_ context.Context, _ *DiscussionQuestion, _, _ string, _ []Expression) (string, error) {
	f.reviewCalls++
	return f.feedback, f.reviewErr
}

func discussionServer(dRepo *fakeDiscussionRepo, coach *fakeCoach) *Server {
	return NewServer(&fakeRepo{}, &fakeExplainer{}, &fakeAnalyzer{}).WithDiscussion(dRepo, coach)
}

func postJSON(t *testing.T, path string, body interface{}) *http.Request {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return authed(httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b)), "u1")
}

var testQuestion = &DiscussionQuestion{
	ID: 1, QuestionEN: "Who is responsible?", Topic: "environment",
	Level: 3, TargetSkills: []string{"giving opinions"},
}

func TestGetDiscussionQuestionOK(t *testing.T) {
	srv := discussionServer(&fakeDiscussionRepo{question: testQuestion}, &fakeCoach{})
	rec := httptest.NewRecorder()
	srv.getDiscussionQuestion(rec, authed(httptest.NewRequest(http.MethodGet, "/api/discussion/question", nil), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got DiscussionQuestion
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != 1 || got.QuestionEN != "Who is responsible?" {
		t.Fatalf("unexpected body: %+v", got)
	}
}

func TestGetDiscussionQuestionEmptyBank(t *testing.T) {
	srv := discussionServer(&fakeDiscussionRepo{questionErr: ErrNoCandidate}, &fakeCoach{})
	rec := httptest.NewRecorder()
	srv.getDiscussionQuestion(rec, authed(httptest.NewRequest(http.MethodGet, "/api/discussion/question", nil), "u1"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestDiscussionReplyOK(t *testing.T) {
	coach := &fakeCoach{reply: &CoachReply{Done: false, Message: "Why do you think so?"}}
	srv := discussionServer(&fakeDiscussionRepo{question: testQuestion}, coach)
	rec := httptest.NewRecorder()
	srv.discussionReply(rec, postJSON(t, "/api/discussion/reply", DiscussionReplyRequest{
		QuestionID: 1, Transcript: msgs("I think companies."),
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got CoachReply
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Done || got.Message != "Why do you think so?" {
		t.Fatalf("unexpected reply: %+v", got)
	}
	if coach.replyCalls != 1 {
		t.Fatalf("expected 1 coach call, got %d", coach.replyCalls)
	}
}

func TestDiscussionReplyCapsAITurnsWithoutCallingCoach(t *testing.T) {
	coach := &fakeCoach{reply: &CoachReply{Message: "should not be used"}}
	srv := discussionServer(&fakeDiscussionRepo{question: testQuestion}, coach)
	// 11 messages = 6 user + 5 ai turns, ending with the user.
	transcript := msgs("a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k")
	rec := httptest.NewRecorder()
	srv.discussionReply(rec, postJSON(t, "/api/discussion/reply", DiscussionReplyRequest{
		QuestionID: 1, Transcript: transcript,
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got CoachReply
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.Done {
		t.Fatal("expected done=true at the AI-turn cap")
	}
	if coach.replyCalls != 0 {
		t.Fatalf("expected no coach calls at the cap, got %d", coach.replyCalls)
	}
}

func TestDiscussionReplyRejectsBadTranscripts(t *testing.T) {
	srv := discussionServer(&fakeDiscussionRepo{question: testQuestion}, &fakeCoach{})
	cases := []DiscussionReplyRequest{
		{QuestionID: 1, Transcript: nil},
		{QuestionID: 1, Transcript: []DiscussionMessage{{Role: "ai", Text: "hi"}}},
		{QuestionID: 1, Transcript: msgs("a", "b")}, // ends with ai, not user
	}
	for i, req := range cases {
		rec := httptest.NewRecorder()
		srv.discussionReply(rec, postJSON(t, "/api/discussion/reply", req))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("case %d: expected 400, got %d", i, rec.Code)
		}
	}
}

func TestDiscussionReplyQuestionNotFound(t *testing.T) {
	srv := discussionServer(&fakeDiscussionRepo{questionErr: ErrNotFound}, &fakeCoach{})
	rec := httptest.NewRecorder()
	srv.discussionReply(rec, postJSON(t, "/api/discussion/reply", DiscussionReplyRequest{
		QuestionID: 42, Transcript: msgs("a"),
	}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestDiscussionReplyCoachError(t *testing.T) {
	srv := discussionServer(&fakeDiscussionRepo{question: testQuestion},
		&fakeCoach{replyErr: context.DeadlineExceeded})
	rec := httptest.NewRecorder()
	srv.discussionReply(rec, postJSON(t, "/api/discussion/reply", DiscussionReplyRequest{
		QuestionID: 1, Transcript: msgs("a"),
	}))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestDiscussionAnalyzeOK(t *testing.T) {
	analysis := &GapAnalysis{
		ExpressedIdeas: []string{"Companies are responsible."},
		MissingIdeas:   []string{"Systemic change is needed."},
		Expressions:    []Expression{{Phrase: "take responsibility for", MeaningJA: "〜に責任を持つ", ExampleEN: "x"}},
	}
	srv := discussionServer(&fakeDiscussionRepo{question: testQuestion}, &fakeCoach{analysis: analysis})
	rec := httptest.NewRecorder()
	srv.discussionAnalyze(rec, postJSON(t, "/api/discussion/analyze", DiscussionAnalyzeRequest{
		QuestionID: 1, Transcript: msgs("I think companies."), ReflectionJA: "制度を変えるべき。",
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got GapAnalysis
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Expressions) != 1 || got.Expressions[0].Phrase != "take responsibility for" {
		t.Fatalf("unexpected analysis: %+v", got)
	}
}

func TestDiscussionAnalyzeRejectsBadReflection(t *testing.T) {
	srv := discussionServer(&fakeDiscussionRepo{question: testQuestion}, &fakeCoach{})
	for i, reflection := range []string{"", "   ", strings.Repeat("あ", maxReflectionLength+1)} {
		rec := httptest.NewRecorder()
		srv.discussionAnalyze(rec, postJSON(t, "/api/discussion/analyze", DiscussionAnalyzeRequest{
			QuestionID: 1, Transcript: msgs("a"), ReflectionJA: reflection,
		}))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("case %d: expected 400, got %d", i, rec.Code)
		}
	}
}

func TestDiscussionCompleteOKSavesSession(t *testing.T) {
	dRepo := &fakeDiscussionRepo{question: testQuestion, savedID: "sess-1"}
	coach := &fakeCoach{feedback: "You used both expressions!"}
	srv := discussionServer(dRepo, coach)
	rec := httptest.NewRecorder()
	srv.discussionComplete(rec, postJSON(t, "/api/discussion/complete", DiscussionCompleteRequest{
		QuestionID:     1,
		Transcript:     msgs("I think companies.", "Why?", "Because they pollute."),
		ReflectionJA:   "制度を変えるべき。",
		ExpressedIdeas: []string{"Companies are responsible."},
		MissingIdeas:   []string{"Systemic change is needed."},
		Expressions:    []Expression{{Phrase: "take responsibility for"}},
		RetryAnswer:    "Companies should take responsibility for their impact.",
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got DiscussionCompleteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SessionID != "sess-1" || got.RetryFeedback != "You used both expressions!" {
		t.Fatalf("unexpected response: %+v", got)
	}
	if len(dRepo.saved) != 1 {
		t.Fatalf("expected 1 saved session, got %d", len(dRepo.saved))
	}
	saved := dRepo.saved[0]
	if saved.uid != "u1" {
		t.Fatalf("expected uid u1, got %q", saved.uid)
	}
	s := saved.session
	if s.QuestionID != 1 || s.QuestionEN != testQuestion.QuestionEN || s.Topic != "environment" ||
		s.FirstAnswer != "I think companies." ||
		s.RetryAnswer != "Companies should take responsibility for their impact." ||
		s.RetryFeedback != "You used both expressions!" || len(s.Transcript) != 3 {
		t.Fatalf("unexpected saved session: %+v", s)
	}
}

func TestDiscussionCompleteAllowsEmptyReflectionAndAnalysis(t *testing.T) {
	dRepo := &fakeDiscussionRepo{question: testQuestion, savedID: "sess-2"}
	srv := discussionServer(dRepo, &fakeCoach{feedback: "Nice retry!"})
	rec := httptest.NewRecorder()
	srv.discussionComplete(rec, postJSON(t, "/api/discussion/complete", DiscussionCompleteRequest{
		QuestionID:  1,
		Transcript:  msgs("I think companies."),
		RetryAnswer: "I still think companies, because they pollute more.",
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	s := dRepo.saved[0].session
	if s.ReflectionJA != "" || len(s.Expressions) != 0 || len(s.ExpressedIdeas) != 0 {
		t.Fatalf("expected empty reflection/analysis, got %+v", s)
	}
}

func TestDiscussionCompleteRejectsBadInput(t *testing.T) {
	srv := discussionServer(&fakeDiscussionRepo{question: testQuestion}, &fakeCoach{feedback: "x"})
	cases := []DiscussionCompleteRequest{
		{QuestionID: 1, Transcript: msgs("a"), RetryAnswer: ""},
		{QuestionID: 1, Transcript: msgs("a"), RetryAnswer: strings.Repeat("a", maxDiscussionTurnLength+1)},
		{QuestionID: 1, Transcript: msgs("a"), RetryAnswer: "ok", ReflectionJA: strings.Repeat("あ", maxReflectionLength+1)},
		{QuestionID: 1, Transcript: msgs("a"), RetryAnswer: "ok", Expressions: []Expression{{Phrase: "a"}, {Phrase: "b"}, {Phrase: "c"}, {Phrase: "d"}, {Phrase: "e"}}},
		{QuestionID: 1, Transcript: nil, RetryAnswer: "ok"},
	}
	for i, req := range cases {
		rec := httptest.NewRecorder()
		srv.discussionComplete(rec, postJSON(t, "/api/discussion/complete", req))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("case %d: expected 400, got %d", i, rec.Code)
		}
	}
}

func TestDiscussionCompleteCoachErrorDoesNotSave(t *testing.T) {
	dRepo := &fakeDiscussionRepo{question: testQuestion}
	srv := discussionServer(dRepo, &fakeCoach{reviewErr: context.DeadlineExceeded})
	rec := httptest.NewRecorder()
	srv.discussionComplete(rec, postJSON(t, "/api/discussion/complete", DiscussionCompleteRequest{
		QuestionID: 1, Transcript: msgs("a"), RetryAnswer: "ok",
	}))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if len(dRepo.saved) != 0 {
		t.Fatal("session must not be saved when feedback generation fails")
	}
}

func TestListDiscussionSessions(t *testing.T) {
	dRepo := &fakeDiscussionRepo{summaries: []DiscussionSessionSummary{
		{ID: "s2", QuestionEN: "Q2", Topic: "work", CreatedAt: "2026-08-23T10:01:00Z"},
		{ID: "s1", QuestionEN: "Q1", Topic: "travel", CreatedAt: "2026-08-23T10:00:00Z"},
	}}
	srv := discussionServer(dRepo, &fakeCoach{})
	rec := httptest.NewRecorder()
	srv.discussionSessions(rec, authed(httptest.NewRequest(http.MethodGet, "/api/discussion/sessions", nil), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got DiscussionSessionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Sessions) != 2 || got.Sessions[0].ID != "s2" {
		t.Fatalf("unexpected sessions: %+v", got)
	}
}

func TestGetDiscussionSessionDetail(t *testing.T) {
	session := &DiscussionSession{ID: "s1", QuestionEN: "Q1", FirstAnswer: "a", RetryAnswer: "b"}
	srv := discussionServer(&fakeDiscussionRepo{session: session}, &fakeCoach{})
	rec := httptest.NewRecorder()
	srv.discussionSessions(rec, authed(httptest.NewRequest(http.MethodGet, "/api/discussion/sessions/s1", nil), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got DiscussionSession
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != "s1" || got.RetryAnswer != "b" {
		t.Fatalf("unexpected session: %+v", got)
	}
}

func TestGetDiscussionSessionNotFound(t *testing.T) {
	srv := discussionServer(&fakeDiscussionRepo{sessionErr: ErrNotFound}, &fakeCoach{})
	rec := httptest.NewRecorder()
	srv.discussionSessions(rec, authed(httptest.NewRequest(http.MethodGet, "/api/discussion/sessions/nope", nil), "u1"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestDiscussionSessionsRejectsPost(t *testing.T) {
	srv := discussionServer(&fakeDiscussionRepo{}, &fakeCoach{})
	rec := httptest.NewRecorder()
	srv.discussionSessions(rec, authed(httptest.NewRequest(http.MethodPost, "/api/discussion/sessions", nil), "u1"))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
