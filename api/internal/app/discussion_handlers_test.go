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

	gotQuestionIDs []int
	gotListUID     string
	gotListLimit   int
}

func (f *fakeDiscussionRepo) RandomQuestion(_ context.Context) (*DiscussionQuestion, error) {
	return f.question, f.questionErr
}
func (f *fakeDiscussionRepo) GetQuestion(_ context.Context, id int) (*DiscussionQuestion, error) {
	f.gotQuestionIDs = append(f.gotQuestionIDs, id)
	return f.question, f.questionErr
}
func (f *fakeDiscussionRepo) SaveSession(_ context.Context, uid string, s *DiscussionSession) (string, error) {
	f.saved = append(f.saved, savedSessionCall{uid, s})
	return f.savedID, f.saveErr
}
func (f *fakeDiscussionRepo) ListSessions(_ context.Context, uid string, limit int) ([]DiscussionSessionSummary, error) {
	f.gotListUID = uid
	f.gotListLimit = limit
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

	gotReplyTranscript []DiscussionMessage

	gotAnalyzeQuestion   *DiscussionQuestion
	gotAnalyzeTranscript []DiscussionMessage
	gotAnalyzeReflection string

	gotReviewQuestion    *DiscussionQuestion
	gotReviewFirstAnswer string
	gotReviewRetryAnswer string
	gotReviewExpressions []Expression
}

func (f *fakeCoach) Reply(_ context.Context, _ *DiscussionQuestion, transcript []DiscussionMessage) (*CoachReply, error) {
	f.replyCalls++
	f.gotReplyTranscript = transcript
	return f.reply, f.replyErr
}
func (f *fakeCoach) AnalyzeGap(_ context.Context, q *DiscussionQuestion, transcript []DiscussionMessage, reflectionJA string) (*GapAnalysis, error) {
	f.gotAnalyzeQuestion = q
	f.gotAnalyzeTranscript = transcript
	f.gotAnalyzeReflection = reflectionJA
	return f.analysis, f.analyzeErr
}
func (f *fakeCoach) ReviewRetry(_ context.Context, q *DiscussionQuestion, firstAnswer, retryAnswer string, expressions []Expression) (string, error) {
	f.reviewCalls++
	f.gotReviewQuestion = q
	f.gotReviewFirstAnswer = firstAnswer
	f.gotReviewRetryAnswer = retryAnswer
	f.gotReviewExpressions = expressions
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
	coach := &fakeCoach{reply: &CoachReply{Message: "Why do you think so?"}}
	dRepo := &fakeDiscussionRepo{question: testQuestion}
	srv := discussionServer(dRepo, coach)
	rec := httptest.NewRecorder()
	srv.discussionReply(rec, postJSON(t, "/api/discussion/reply", DiscussionReplyRequest{
		QuestionID: 1, Transcript: msgs("I think companies."),
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got DiscussionReplyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Done || got.Message != "Why do you think so?" {
		t.Fatalf("unexpected reply: %+v", got)
	}
	if coach.replyCalls != 1 {
		t.Fatalf("expected 1 coach call, got %d", coach.replyCalls)
	}
	if len(dRepo.gotQuestionIDs) != 1 || dRepo.gotQuestionIDs[0] != 1 {
		t.Fatalf("expected GetQuestion called with id 1, got %v", dRepo.gotQuestionIDs)
	}
	if len(coach.gotReplyTranscript) != 1 || coach.gotReplyTranscript[0].Text != "I think companies." {
		t.Fatalf("expected coach to receive the 1-message transcript, got %+v", coach.gotReplyTranscript)
	}
}

// Every session is exactly discussionFollowUps follow-ups long, and that is
// the server's arithmetic — not something the model can shorten or stretch.
func TestDiscussionReplyEndsAfterTheFixedFollowUpsWithoutCallingCoach(t *testing.T) {
	coach := &fakeCoach{reply: &CoachReply{Message: "should not be used"}}
	srv := discussionServer(&fakeDiscussionRepo{question: testQuestion}, coach)
	// 5 messages = 3 user + 2 ai turns, ending with the user: both
	// follow-ups have been asked and answered.
	rec := httptest.NewRecorder()
	srv.discussionReply(rec, postJSON(t, "/api/discussion/reply", DiscussionReplyRequest{
		QuestionID: 1, Transcript: msgs("a", "b", "c", "d", "e"),
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got DiscussionReplyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.Done {
		t.Fatal("expected done=true once both follow-ups are answered")
	}
	if coach.replyCalls != 0 {
		t.Fatalf("expected no coach calls at the cap, got %d", coach.replyCalls)
	}
}

func TestDiscussionReplyAsksTheSecondFollowUp(t *testing.T) {
	coach := &fakeCoach{reply: &CoachReply{Message: "Can you give an example?"}}
	srv := discussionServer(&fakeDiscussionRepo{question: testQuestion}, coach)
	rec := httptest.NewRecorder()
	srv.discussionReply(rec, postJSON(t, "/api/discussion/reply", DiscussionReplyRequest{
		QuestionID: 1, Transcript: msgs("a", "b", "c"),
	}))
	var got DiscussionReplyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Done || got.Message != "Can you give an example?" {
		t.Fatalf("expected the second follow-up question, got %+v", got)
	}
	if coach.replyCalls != 1 {
		t.Fatalf("expected 1 coach call, got %d", coach.replyCalls)
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
	coach := &fakeCoach{analysis: analysis}
	srv := discussionServer(&fakeDiscussionRepo{question: testQuestion}, coach)
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
	if coach.gotAnalyzeQuestion != testQuestion {
		t.Fatalf("expected coach to receive the loaded question, got %+v", coach.gotAnalyzeQuestion)
	}
	if coach.gotAnalyzeReflection != "制度を変えるべき。" {
		t.Fatalf("expected reflection to be passed through, got %q", coach.gotAnalyzeReflection)
	}
	if len(coach.gotAnalyzeTranscript) != 1 || coach.gotAnalyzeTranscript[0].Text != "I think companies." {
		t.Fatalf("expected coach to receive the transcript, got %+v", coach.gotAnalyzeTranscript)
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
	if coach.gotReviewQuestion != testQuestion {
		t.Fatalf("expected coach to receive the loaded question, got %+v", coach.gotReviewQuestion)
	}
	if coach.gotReviewFirstAnswer != "I think companies." {
		t.Fatalf("expected ReviewRetry to get firstAnswer %q, got %q", "I think companies.", coach.gotReviewFirstAnswer)
	}
	if coach.gotReviewRetryAnswer != "Companies should take responsibility for their impact." {
		t.Fatalf("expected ReviewRetry to get the submitted retry answer, got %q", coach.gotReviewRetryAnswer)
	}
	if len(coach.gotReviewExpressions) != 1 || coach.gotReviewExpressions[0].Phrase != "take responsibility for" {
		t.Fatalf("expected ReviewRetry to get the request's expressions, got %+v", coach.gotReviewExpressions)
	}
}

func TestDiscussionCompleteSavesCorrections(t *testing.T) {
	dRepo := &fakeDiscussionRepo{question: testQuestion, savedID: "sess-c"}
	srv := discussionServer(dRepo, &fakeCoach{feedback: "Nice retry!"})
	rec := httptest.NewRecorder()
	srv.discussionComplete(rec, postJSON(t, "/api/discussion/complete", DiscussionCompleteRequest{
		QuestionID:  1,
		Transcript:  msgs("I am agree with you."),
		RetryAnswer: "I agree with you because they pollute more.",
		Corrections: []Correction{
			{Original: "I am agree with you.", Better: "I agree with you.", NoteJA: "agree は動詞です。"},
		},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	s := dRepo.saved[0].session
	if len(s.Corrections) != 1 || s.Corrections[0].Better != "I agree with you." {
		t.Fatalf("expected the correction to be saved, got %+v", s.Corrections)
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
		{QuestionID: 1, Transcript: msgs("a"), RetryAnswer: "ok", Corrections: []Correction{{Original: "a"}, {Original: "b"}, {Original: "c"}, {Original: "d"}}},
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

// TestDiscussionCompleteAcceptsMaximalValidSession pins the completion body
// cap above the sum of every field's individually accepted maximum — a
// session whose parts all passed earlier validation must never 400 at the
// final save step.
func TestDiscussionCompleteAcceptsMaximalValidSession(t *testing.T) {
	dRepo := &fakeDiscussionRepo{question: testQuestion, savedID: "sess-max"}
	srv := discussionServer(dRepo, &fakeCoach{feedback: "Great!"})
	// Multibyte throughout: every field at its rune limit, so the JSON body
	// is far larger in bytes than in characters — exactly the payload the
	// old byte-sized caps rejected.
	long := strings.Repeat("あ", maxDiscussionTurnLength)
	texts := make([]string, maxTranscriptMessages)
	for i := range texts {
		texts[i] = long
	}
	ideas := make([]string, maxSessionIdeas)
	for i := range ideas {
		ideas[i] = strings.Repeat("b", 200)
	}
	req := DiscussionCompleteRequest{
		QuestionID:     1,
		Transcript:     msgs(texts...),
		ReflectionJA:   strings.Repeat("あ", maxReflectionLength),
		ExpressedIdeas: ideas,
		MissingIdeas:   ideas,
		Expressions:    []Expression{{Phrase: "p", MeaningJA: "m", ExampleEN: "e"}},
		Corrections:    []Correction{{Original: "o", Better: "b", NoteJA: "n"}},
		RetryAnswer:    long,
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if len(body) <= 64*1024 {
		t.Fatalf("test payload must exceed the previous 64KiB cap to prove the sizing, got %d bytes", len(body))
	}
	if len(body) > maxDiscussionRequestBytes {
		t.Fatalf("maximal valid session (%d bytes) no longer fits the %d-byte cap — resize it",
			len(body), maxDiscussionRequestBytes)
	}
	rec := httptest.NewRecorder()
	srv.discussionComplete(rec, postJSON(t, "/api/discussion/complete", req))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for a maximal valid session, got %d: %s", rec.Code, rec.Body.String())
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
	if dRepo.gotListUID != "u1" {
		t.Fatalf("expected ListSessions called with uid u1, got %q", dRepo.gotListUID)
	}
	if dRepo.gotListLimit != maxDiscussionSessionList {
		t.Fatalf("expected ListSessions called with limit %d, got %d", maxDiscussionSessionList, dRepo.gotListLimit)
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

func TestDiscussionSessionsRejectsNestedPath(t *testing.T) {
	srv := discussionServer(&fakeDiscussionRepo{session: &DiscussionSession{ID: "s1"}}, &fakeCoach{})
	rec := httptest.NewRecorder()
	srv.discussionSessions(rec, authed(httptest.NewRequest(http.MethodGet, "/api/discussion/sessions/s1/extra", nil), "u1"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
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
