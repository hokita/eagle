package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
