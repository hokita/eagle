# Discussion Practice Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an AI-guided discussion practice mode: curated question → English conversation with AI follow-ups → Japanese reflection → gap analysis + taught expressions → retry → before/after comparison → session history.

**Architecture:** Client-driven state machine in the Next.js frontend; stateless per-phase endpoints in the Go API; Firestore for the curated question bank and completed sessions; Gemini (`gemini-3.1-flash-lite`) behind a `DiscussionCoach` interface with JSON response schemas.

**Tech Stack:** Go 1.25 (`net/http`, `cloud.google.com/go/firestore`, `google.golang.org/genai` v1.64.0), Next.js static export, React, Tailwind, vitest + testing-library, Playwright e2e against Firebase emulators.

**Spec:** `docs/superpowers/specs/2026-08-23-discussion-practice-design.md`

## Global Constraints

- Work happens on the existing branch `feature/discussion-practice`.
- TDD red/green for every task: write the failing test, see it fail, implement, see it pass, commit.
- Go tests: run `go test ./...` from `api/`. Firestore repo tests need the emulator: from repo root `firebase emulators:exec --project eagle-test --only firestore "cd api && go test ./internal/app -run TestFirestore -v"` (emulators:exec sets `FIRESTORE_EMULATOR_HOST` itself; tests skip when it's unset).
- Frontend tests: run `npx vitest run` from `fe/`.
- All new API endpoints go through the existing `auth()` wrapper in `api/internal/app/router.go`.
- The client never supplies question text — handlers load it by `question_id` (same trust model as `sentence_id`).
- Server constants (exact values): `maxDiscussionRequestBytes = 32 * 1024`, `maxDiscussionTurnLength = 2000`, `maxReflectionLength = 4000`, `maxTranscriptMessages = 12`, `maxAIFollowUps = 5`, `maxDiscussionSessionList = 50`, `discussionTimeout = 30 * time.Second`.
- Learning-philosophy rules baked into prompts: no grammar correction, no model answers, no idea-feeding during conversation; gap analysis compares ideas not sentences; 2–4 reusable expressions; retry feedback never rewrites.
- Spec ambiguity resolved here: the `/api/discussion/complete` call fires when the user submits the retry answer (not from a separate "Save & finish" button), because `retry_feedback` shown in the comparison view comes from that call. The comparison phase is pure display.
- Commit messages: imperative summary + `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>` trailer.

---

### Task 1: Domain types, interfaces, and transcript validation

**Files:**
- Create: `api/internal/app/discussion.go`
- Test: `api/internal/app/discussion_test.go`

**Interfaces:**
- Consumes: `ErrNotFound`, `ErrNoCandidate` from `api/internal/app/sentence.go`.
- Produces: every type below, used by Tasks 2–9. Exact signatures matter — later tasks compile against them.

- [ ] **Step 1: Write the failing tests**

Create `api/internal/app/discussion_test.go`:

```go
package app

import (
	"strings"
	"testing"
)

func msgs(texts ...string) []DiscussionMessage {
	out := make([]DiscussionMessage, len(texts))
	for i, t := range texts {
		role := "user"
		if i%2 == 1 {
			role = "ai"
		}
		out[i] = DiscussionMessage{Role: role, Text: t}
	}
	return out
}

func TestValidateTranscriptOK(t *testing.T) {
	if err := validateTranscript(msgs("I think companies.", "Why?", "Because they pollute more.")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTranscriptEmpty(t *testing.T) {
	if err := validateTranscript(nil); err == nil {
		t.Fatal("expected error for empty transcript")
	}
}

func TestValidateTranscriptTooLong(t *testing.T) {
	texts := make([]string, maxTranscriptMessages+1)
	for i := range texts {
		texts[i] = "x"
	}
	if err := validateTranscript(msgs(texts...)); err == nil {
		t.Fatal("expected error for transcript over the cap")
	}
}

func TestValidateTranscriptRolesMustAlternateStartingWithUser(t *testing.T) {
	bad := []DiscussionMessage{{Role: "ai", Text: "Why?"}}
	if err := validateTranscript(bad); err == nil {
		t.Fatal("expected error when first message is not from the user")
	}
	bad = []DiscussionMessage{{Role: "user", Text: "a"}, {Role: "user", Text: "b"}}
	if err := validateTranscript(bad); err == nil {
		t.Fatal("expected error when roles do not alternate")
	}
}

func TestValidateTranscriptRejectsBlankAndOversizedMessages(t *testing.T) {
	if err := validateTranscript(msgs("   ")); err == nil {
		t.Fatal("expected error for whitespace-only message")
	}
	if err := validateTranscript(msgs(strings.Repeat("a", maxDiscussionTurnLength+1))); err == nil {
		t.Fatal("expected error for oversized message")
	}
}

func TestCountAITurns(t *testing.T) {
	if got := countAITurns(msgs("a", "b", "c", "d", "e")); got != 2 {
		t.Fatalf("expected 2 AI turns, got %d", got)
	}
	if got := countAITurns(msgs("a")); got != 0 {
		t.Fatalf("expected 0 AI turns, got %d", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/app -run 'TestValidateTranscript|TestCountAITurns' -v`
Expected: compile FAIL — `DiscussionMessage`, `validateTranscript`, `countAITurns`, constants undefined.

- [ ] **Step 3: Write the implementation**

Create `api/internal/app/discussion.go`:

```go
package app

import (
	"context"
	"fmt"
	"strings"
)

const (
	// maxDiscussionRequestBytes bounds transcript-bearing request bodies so an
	// authenticated caller can't exhaust memory or inflate Gemini request size.
	maxDiscussionRequestBytes = 32 * 1024
	// maxDiscussionTurnLength bounds a single transcript message.
	maxDiscussionTurnLength = 2000
	// maxReflectionLength bounds the Japanese reflection text.
	maxReflectionLength = 4000
	// maxTranscriptMessages: initial answer + 5 follow-ups + 5 replies = 11,
	// plus the AI's closing line when the conversation ends = 12.
	maxTranscriptMessages = 12
	// maxAIFollowUps is the server-side hard cap on AI turns — the reply
	// handler returns done without calling Gemini once it is reached.
	maxAIFollowUps = 5
	// maxDiscussionSessionList caps the history list response.
	maxDiscussionSessionList = 50
)

type DiscussionQuestion struct {
	ID           int      `json:"id"`
	QuestionEN   string   `json:"question_en"`
	Topic        string   `json:"topic"`
	Level        int      `json:"level"`
	TargetSkills []string `json:"target_skills"`
}

type DiscussionMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type Expression struct {
	Phrase    string `json:"phrase"`
	MeaningJA string `json:"meaning_ja"`
	ExampleEN string `json:"example_en"`
}

type GapAnalysis struct {
	ExpressedIdeas []string     `json:"expressed_ideas"`
	MissingIdeas   []string     `json:"missing_ideas"`
	Expressions    []Expression `json:"expressions"`
}

type CoachReply struct {
	Done    bool   `json:"done"`
	Message string `json:"message"`
}

type DiscussionSession struct {
	ID             string              `json:"id"`
	QuestionID     int                 `json:"question_id"`
	QuestionEN     string              `json:"question_en"`
	Topic          string              `json:"topic"`
	Transcript     []DiscussionMessage `json:"transcript"`
	ReflectionJA   string              `json:"reflection_ja"`
	ExpressedIdeas []string            `json:"expressed_ideas"`
	MissingIdeas   []string            `json:"missing_ideas"`
	Expressions    []Expression        `json:"expressions"`
	FirstAnswer    string              `json:"first_answer"`
	RetryAnswer    string              `json:"retry_answer"`
	RetryFeedback  string              `json:"retry_feedback"`
	CreatedAt      string              `json:"created_at"`
}

type DiscussionSessionSummary struct {
	ID         string `json:"id"`
	QuestionEN string `json:"question_en"`
	Topic      string `json:"topic"`
	CreatedAt  string `json:"created_at"`
}

// DiscussionRepository is the data-access seam for discussion practice.
type DiscussionRepository interface {
	// RandomQuestion returns a random active question, ErrNoCandidate when
	// the bank is empty.
	RandomQuestion(ctx context.Context) (*DiscussionQuestion, error)
	// GetQuestion returns ErrNotFound for a missing id.
	GetQuestion(ctx context.Context, id int) (*DiscussionQuestion, error)
	// SaveSession writes a completed session and returns its new id.
	SaveSession(ctx context.Context, uid string, s *DiscussionSession) (string, error)
	ListSessions(ctx context.Context, uid string, limit int) ([]DiscussionSessionSummary, error)
	// GetSession returns ErrNotFound for a missing id.
	GetSession(ctx context.Context, uid, id string) (*DiscussionSession, error)
}

// DiscussionCoach is the LLM seam for the three AI steps of a session.
type DiscussionCoach interface {
	Reply(ctx context.Context, q *DiscussionQuestion, transcript []DiscussionMessage) (*CoachReply, error)
	AnalyzeGap(ctx context.Context, q *DiscussionQuestion, transcript []DiscussionMessage, reflectionJA string) (*GapAnalysis, error)
	ReviewRetry(ctx context.Context, q *DiscussionQuestion, firstAnswer, retryAnswer string, expressions []Expression) (string, error)
}

// validateTranscript enforces the transcript shape shared by every
// discussion endpoint: non-empty, capped length, roles alternating starting
// with "user", every message non-blank and within the per-turn cap.
func validateTranscript(transcript []DiscussionMessage) error {
	if len(transcript) == 0 {
		return fmt.Errorf("transcript is empty")
	}
	if len(transcript) > maxTranscriptMessages {
		return fmt.Errorf("transcript exceeds %d messages", maxTranscriptMessages)
	}
	for i, m := range transcript {
		want := "user"
		if i%2 == 1 {
			want = "ai"
		}
		if m.Role != want {
			return fmt.Errorf("message %d: expected role %q, got %q", i, want, m.Role)
		}
		if strings.TrimSpace(m.Text) == "" {
			return fmt.Errorf("message %d: text is blank", i)
		}
		if len(m.Text) > maxDiscussionTurnLength {
			return fmt.Errorf("message %d: text exceeds %d characters", i, maxDiscussionTurnLength)
		}
	}
	return nil
}

func countAITurns(transcript []DiscussionMessage) int {
	n := 0
	for _, m := range transcript {
		if m.Role == "ai" {
			n++
		}
	}
	return n
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./internal/app -run 'TestValidateTranscript|TestCountAITurns' -v`
Expected: PASS. Then `go test ./...` to confirm nothing else broke.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/discussion.go api/internal/app/discussion_test.go
git commit -m "Add discussion domain types, seams, and transcript validation

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: Prompt builders

**Files:**
- Create: `api/internal/app/discussion_prompts.go`
- Test: `api/internal/app/discussion_prompts_test.go`

**Interfaces:**
- Consumes: `DiscussionQuestion`, `DiscussionMessage`, `Expression` (Task 1).
- Produces: `buildDiscussionReplyPrompt(q *DiscussionQuestion, transcript []DiscussionMessage) string`, `buildGapAnalysisPrompt(q *DiscussionQuestion, transcript []DiscussionMessage, reflectionJA string) string`, `buildRetryReviewPrompt(q *DiscussionQuestion, firstAnswer, retryAnswer string, expressions []Expression) string`, `renderTranscript(transcript []DiscussionMessage) string`. Used by Task 3.

- [ ] **Step 1: Write the failing tests**

Create `api/internal/app/discussion_prompts_test.go`:

```go
package app

import (
	"strings"
	"testing"
)

var promptQuestion = &DiscussionQuestion{
	ID:           1,
	QuestionEN:   "Who should take more responsibility for environmental problems?",
	Topic:        "environment",
	Level:        3,
	TargetSkills: []string{"giving opinions", "giving reasons"},
}

func TestBuildDiscussionReplyPromptIncludesQuestionAndTranscript(t *testing.T) {
	got := buildDiscussionReplyPrompt(promptQuestion, msgs("I think companies.", "Why do you think so?", "Because they pollute more."))
	for _, want := range []string{
		promptQuestion.QuestionEN,
		"giving opinions, giving reasons",
		"Learner: I think companies.",
		"You: Why do you think so?",
		"Learner: Because they pollute more.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

func TestBuildDiscussionReplyPromptForbidsCorrectionAndTracksFollowUps(t *testing.T) {
	got := buildDiscussionReplyPrompt(promptQuestion, msgs("I think companies.", "Why?", "Because."))
	for _, want := range []string{
		"Never correct the learner's grammar",
		"never answer the question for them",
		"You have asked 1 follow-up question(s) so far",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

func TestBuildGapAnalysisPromptIncludesReflectionAndRules(t *testing.T) {
	got := buildGapAnalysisPrompt(promptQuestion, msgs("I think companies."), "制度を変える必要がある。")
	for _, want := range []string{
		promptQuestion.QuestionEN,
		"Learner: I think companies.",
		"制度を変える必要がある。",
		"ideas and intentions, not literal wording",
		"2 to 4",
		"level 3 of 5",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

func TestBuildRetryReviewPromptIncludesAnswersAndExpressions(t *testing.T) {
	got := buildRetryReviewPrompt(promptQuestion, "I think companies.",
		"Companies should take responsibility for their impact.",
		[]Expression{{Phrase: "take responsibility for"}, {Phrase: "make systemic changes"}})
	for _, want := range []string{
		"First answer: I think companies.",
		"New answer: Companies should take responsibility for their impact.",
		"- take responsibility for",
		"- make systemic changes",
		"Do not rewrite",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/app -run 'TestBuildDiscussion|TestBuildGap|TestBuildRetry' -v`
Expected: compile FAIL — builders undefined.

- [ ] **Step 3: Write the implementation**

Create `api/internal/app/discussion_prompts.go`:

```go
package app

import (
	"fmt"
	"strings"
)

// renderTranscript renders the conversation with "Learner:"/"You:" labels —
// "You" is the coach, so the model reads its own past turns correctly.
func renderTranscript(transcript []DiscussionMessage) string {
	var b strings.Builder
	for _, m := range transcript {
		label := "Learner"
		if m.Role == "ai" {
			label = "You"
		}
		fmt.Fprintf(&b, "%s: %s\n", label, m.Text)
	}
	return b.String()
}

// buildDiscussionReplyPrompt is a pure function (unit-testable without
// network) producing the follow-up-question prompt. The JSON shape of the
// answer is enforced by the response schema in GeminiCoach; this prompt
// carries the semantics of done/message and the learning-philosophy rules.
func buildDiscussionReplyPrompt(q *DiscussionQuestion, transcript []DiscussionMessage) string {
	var b strings.Builder
	b.WriteString("You are a friendly English conversation partner helping a Japanese learner ")
	b.WriteString("practice expressing their own opinions in English.\n\n")
	fmt.Fprintf(&b, "Discussion question: %s\n", q.QuestionEN)
	fmt.Fprintf(&b, "Practice goals for this question: %s\n\n", strings.Join(q.TargetSkills, ", "))
	b.WriteString("Conversation so far:\n")
	b.WriteString(renderTranscript(transcript))
	b.WriteString("\nRules:\n")
	b.WriteString("- Ask exactly ONE short follow-up question that draws more of the learner's own ")
	b.WriteString("thinking out — why they think so, a concrete example, the opposite view, or what they would do.\n")
	b.WriteString("- Never correct the learner's grammar or vocabulary, never rewrite their sentences, ")
	b.WriteString("and never answer the question for them or suggest ideas.\n")
	b.WriteString("- Never ask the learner to use Japanese.\n")
	b.WriteString("- Keep your message to at most 2 short sentences of natural spoken English.\n")
	fmt.Fprintf(&b, "- You have asked %d follow-up question(s) so far. ", countAITurns(transcript))
	b.WriteString("Once the learner has answered at least 2 follow-up questions and the conversation feels ")
	b.WriteString("complete, set \"done\" to true and make \"message\" a one-sentence friendly closing ")
	b.WriteString("comment instead of a question.\n")
	b.WriteString("\nRespond as JSON with fields \"done\" (boolean) and \"message\" (string).\n")
	return b.String()
}

// buildGapAnalysisPrompt compares the ideas of the English conversation with
// the Japanese reflection and asks for 2-4 reusable expressions.
func buildGapAnalysisPrompt(q *DiscussionQuestion, transcript []DiscussionMessage, reflectionJA string) string {
	var b strings.Builder
	b.WriteString("You are an English tutor. A Japanese learner discussed a question in English, ")
	b.WriteString("then wrote in Japanese what else they wanted to say but could not express in English.\n\n")
	fmt.Fprintf(&b, "Discussion question: %s\n\n", q.QuestionEN)
	b.WriteString("English conversation:\n")
	b.WriteString(renderTranscript(transcript))
	fmt.Fprintf(&b, "\nWhat the learner also wanted to say (in Japanese):\n%s\n\n", reflectionJA)
	b.WriteString("Compare the ideas and intentions, not literal wording. Produce:\n")
	b.WriteString("1. expressed_ideas: the main ideas the learner successfully communicated in English ")
	b.WriteString("(short English sentences, at most 5).\n")
	b.WriteString("2. missing_ideas: ideas present in the Japanese text that never appeared in the English ")
	b.WriteString("conversation (short English sentences, at most 5).\n")
	b.WriteString("3. expressions: 2 to 4 natural spoken-English expressions that would let the learner say ")
	b.WriteString("the missing ideas. Prefer reusable chunks (\"take responsibility for\") over single words. ")
	fmt.Fprintf(&b, "Pitch them slightly above the learner's current level — this question is level %d of 5. ", q.Level)
	b.WriteString("For each: phrase (the chunk itself), meaning_ja (a short Japanese gloss), ")
	b.WriteString("example_en (one natural example sentence using it).\n")
	return b.String()
}

// buildRetryReviewPrompt produces encouraging usage feedback on the retry —
// never a rewrite or correction list.
func buildRetryReviewPrompt(q *DiscussionQuestion, firstAnswer, retryAnswer string, expressions []Expression) string {
	var b strings.Builder
	b.WriteString("You are an English tutor. A learner answered a discussion question, studied a few new ")
	b.WriteString("expressions, and then answered the same question again.\n\n")
	fmt.Fprintf(&b, "Question: %s\n", q.QuestionEN)
	fmt.Fprintf(&b, "First answer: %s\n", firstAnswer)
	b.WriteString("Expressions taught:\n")
	for _, e := range expressions {
		fmt.Fprintf(&b, "- %s\n", e.Phrase)
	}
	fmt.Fprintf(&b, "New answer: %s\n\n", retryAnswer)
	b.WriteString("Write 2-3 friendly sentences in English: say which of the taught expressions the learner ")
	b.WriteString("actually used, and what improved compared with the first answer. ")
	b.WriteString("Do not rewrite their answer, do not list grammar mistakes, do not suggest further corrections. ")
	b.WriteString("Respond with plain text only.\n")
	return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./internal/app -run 'TestBuildDiscussion|TestBuildGap|TestBuildRetry' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/discussion_prompts.go api/internal/app/discussion_prompts_test.go
git commit -m "Add discussion prompt builders

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: GeminiCoach

**Files:**
- Create: `api/internal/app/gemini_coach.go`
- Test: `api/internal/app/gemini_coach_test.go`

**Interfaces:**
- Consumes: `DiscussionCoach` types (Task 1), prompt builders (Task 2), `contentGenerator` + `fakeContentGenerator` (already exist in `gemini_explainer.go` / `gemini_explainer_test.go`), `geminiExplainModel` constant.
- Produces: `GeminiCoach` struct with `NewGeminiCoach(ctx context.Context, apiKey string) (*GeminiCoach, error)` and the three `DiscussionCoach` methods. Used by Task 8 (main.go wiring).

- [ ] **Step 1: Write the failing tests**

Create `api/internal/app/gemini_coach_test.go`. Note `fakeContentGenerator` already exists in `gemini_explainer_test.go` (fields: `resp`, `err`, `gotModel`, `gotContents`, `gotConfig`) — reuse it, do not redeclare it.

```go
package app

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/genai"
)

func textResp(text string) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{Content: &genai.Content{Parts: []*genai.Part{{Text: text}}}},
		},
	}
}

func TestGeminiCoachReplyParsesJSON(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{"done":false,"message":"Why do you think so?"}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}

	got, err := g.Reply(context.Background(), promptQuestion, msgs("I think companies."))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Done || got.Message != "Why do you think so?" {
		t.Fatalf("unexpected reply: %+v", got)
	}
	if fake.gotConfig == nil || fake.gotConfig.ResponseMIMEType != "application/json" {
		t.Fatalf("expected JSON response config, got %+v", fake.gotConfig)
	}
	if fake.gotConfig.ResponseSchema == nil {
		t.Fatal("expected a response schema")
	}
	if fake.gotConfig.MaxOutputTokens != maxCoachReplyOutputTokens {
		t.Fatalf("expected MaxOutputTokens=%d, got %d", maxCoachReplyOutputTokens, fake.gotConfig.MaxOutputTokens)
	}
}

func TestGeminiCoachReplyRejectsMalformedJSON(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp("not json")}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	if _, err := g.Reply(context.Background(), promptQuestion, msgs("a")); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestGeminiCoachReplyRejectsBlankMessage(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{"done":false,"message":"  "}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	if _, err := g.Reply(context.Background(), promptQuestion, msgs("a")); err == nil {
		t.Fatal("expected error for blank follow-up message")
	}
}

func TestGeminiCoachAnalyzeGapParsesAndValidates(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{
		"expressed_ideas":["Companies are responsible."],
		"missing_ideas":["Systemic change is needed."],
		"expressions":[
			{"phrase":"take responsibility for","meaning_ja":"〜に責任を持つ","example_en":"Companies should take responsibility for pollution."},
			{"phrase":"make systemic changes","meaning_ja":"制度的な変更を行う","example_en":"Governments can make systemic changes."}
		]}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}

	got, err := g.AnalyzeGap(context.Background(), promptQuestion, msgs("I think companies."), "制度を変えるべき。")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Expressions) != 2 || got.Expressions[0].Phrase != "take responsibility for" {
		t.Fatalf("unexpected analysis: %+v", got)
	}
}

func TestGeminiCoachAnalyzeGapTruncatesToFourExpressions(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{
		"expressed_ideas":[],"missing_ideas":[],
		"expressions":[
			{"phrase":"a","meaning_ja":"あ","example_en":"A."},
			{"phrase":"b","meaning_ja":"い","example_en":"B."},
			{"phrase":"c","meaning_ja":"う","example_en":"C."},
			{"phrase":"d","meaning_ja":"え","example_en":"D."},
			{"phrase":"e","meaning_ja":"お","example_en":"E."}
		]}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	got, err := g.AnalyzeGap(context.Background(), promptQuestion, msgs("a"), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Expressions) != 4 {
		t.Fatalf("expected 4 expressions after truncation, got %d", len(got.Expressions))
	}
}

func TestGeminiCoachAnalyzeGapRejectsNoValidExpressions(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{"expressed_ideas":[],"missing_ideas":[],"expressions":[{"phrase":"  ","meaning_ja":"","example_en":""}]}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	if _, err := g.AnalyzeGap(context.Background(), promptQuestion, msgs("a"), "x"); err == nil {
		t.Fatal("expected error when no valid expression remains")
	}
}

func TestGeminiCoachReviewRetryReturnsText(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp("Nice use of the new expressions!")}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	got, err := g.ReviewRetry(context.Background(), promptQuestion, "first", "second", []Expression{{Phrase: "p"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Nice use of the new expressions!" {
		t.Fatalf("unexpected feedback: %q", got)
	}
}

func TestGeminiCoachReviewRetryRejectsEmpty(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp("   ")}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	if _, err := g.ReviewRetry(context.Background(), promptQuestion, "a", "b", nil); err == nil {
		t.Fatal("expected error for empty feedback")
	}
}

func TestGeminiCoachPropagatesError(t *testing.T) {
	fake := &fakeContentGenerator{err: errors.New("network error")}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	if _, err := g.Reply(context.Background(), promptQuestion, msgs("a")); err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/app -run TestGeminiCoach -v`
Expected: compile FAIL — `GeminiCoach`, `maxCoachReplyOutputTokens` undefined.

- [ ] **Step 3: Write the implementation**

Create `api/internal/app/gemini_coach.go`:

```go
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"
)

const (
	discussionTimeout = 30 * time.Second

	// Output bounds per call — the input side is bounded by transcript
	// validation; these keep the response side predictable too.
	maxCoachReplyOutputTokens   = 256
	maxCoachAnalyzeOutputTokens = 1024
	maxCoachReviewOutputTokens  = 512
)

var coachReplySchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"done":    {Type: genai.TypeBoolean},
		"message": {Type: genai.TypeString},
	},
	Required: []string{"done", "message"},
}

var gapAnalysisSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"expressed_ideas": {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
		"missing_ideas":   {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
		"expressions": {Type: genai.TypeArray, Items: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"phrase":     {Type: genai.TypeString},
				"meaning_ja": {Type: genai.TypeString},
				"example_en": {Type: genai.TypeString},
			},
			Required: []string{"phrase", "meaning_ja", "example_en"},
		}},
	},
	Required: []string{"expressed_ideas", "missing_ideas", "expressions"},
}

// GeminiCoach implements DiscussionCoach using the Gemini API, reusing the
// same client configuration and model as GeminiExplainer.
type GeminiCoach struct {
	models contentGenerator
	model  string
}

func NewGeminiCoach(ctx context.Context, apiKey string) (*GeminiCoach, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("create genai client: %w", err)
	}
	return &GeminiCoach{models: client.Models, model: geminiExplainModel}, nil
}

func (g *GeminiCoach) generate(ctx context.Context, prompt string, config *genai.GenerateContentConfig) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, discussionTimeout)
	defer cancel()
	contents := []*genai.Content{{Parts: []*genai.Part{{Text: prompt}}}}
	resp, err := g.models.GenerateContent(ctx, g.model, contents, config)
	if err != nil {
		return "", fmt.Errorf("gemini generate content: %w", err)
	}
	return resp.Text(), nil
}

func (g *GeminiCoach) Reply(ctx context.Context, q *DiscussionQuestion, transcript []DiscussionMessage) (*CoachReply, error) {
	text, err := g.generate(ctx, buildDiscussionReplyPrompt(q, transcript), &genai.GenerateContentConfig{
		MaxOutputTokens:  maxCoachReplyOutputTokens,
		ResponseMIMEType: "application/json",
		ResponseSchema:   coachReplySchema,
	})
	if err != nil {
		return nil, err
	}
	var reply CoachReply
	if err := json.Unmarshal([]byte(text), &reply); err != nil {
		return nil, fmt.Errorf("parse coach reply: %w", err)
	}
	if strings.TrimSpace(reply.Message) == "" {
		return nil, fmt.Errorf("coach reply has an empty message")
	}
	return &reply, nil
}

func (g *GeminiCoach) AnalyzeGap(ctx context.Context, q *DiscussionQuestion, transcript []DiscussionMessage, reflectionJA string) (*GapAnalysis, error) {
	text, err := g.generate(ctx, buildGapAnalysisPrompt(q, transcript, reflectionJA), &genai.GenerateContentConfig{
		MaxOutputTokens:  maxCoachAnalyzeOutputTokens,
		ResponseMIMEType: "application/json",
		ResponseSchema:   gapAnalysisSchema,
	})
	if err != nil {
		return nil, err
	}
	var analysis GapAnalysis
	if err := json.Unmarshal([]byte(text), &analysis); err != nil {
		return nil, fmt.Errorf("parse gap analysis: %w", err)
	}
	// Keep only well-formed expressions, then enforce the 2-4 range as
	// best we can: truncate extras, error when nothing usable remains.
	valid := analysis.Expressions[:0]
	for _, e := range analysis.Expressions {
		if strings.TrimSpace(e.Phrase) == "" {
			continue
		}
		valid = append(valid, e)
	}
	if len(valid) == 0 {
		return nil, fmt.Errorf("gap analysis produced no usable expressions")
	}
	if len(valid) > 4 {
		valid = valid[:4]
	}
	analysis.Expressions = valid
	if analysis.ExpressedIdeas == nil {
		analysis.ExpressedIdeas = []string{}
	}
	if analysis.MissingIdeas == nil {
		analysis.MissingIdeas = []string{}
	}
	return &analysis, nil
}

func (g *GeminiCoach) ReviewRetry(ctx context.Context, q *DiscussionQuestion, firstAnswer, retryAnswer string, expressions []Expression) (string, error) {
	text, err := g.generate(ctx, buildRetryReviewPrompt(q, firstAnswer, retryAnswer, expressions), &genai.GenerateContentConfig{
		MaxOutputTokens: maxCoachReviewOutputTokens,
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("retry review produced empty feedback")
	}
	return text, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./internal/app -run TestGeminiCoach -v`
Expected: PASS. Then `go test ./...`.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/gemini_coach.go api/internal/app/gemini_coach_test.go
git commit -m "Add GeminiCoach with JSON response schemas

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: Firestore discussion repository

**Files:**
- Create: `api/internal/app/firestore_discussion.go`
- Test: `api/internal/app/firestore_discussion_test.go`

**Interfaces:**
- Consumes: `firestoreRepo` (existing, `firestore_repo.go` — has `client` and `now` fields), `DiscussionRepository` types (Task 1), emulator helpers `newEmulatorClient`/`clearFirestoreEmulator` (existing, `firestore_repo_test.go`).
- Produces: `firestoreRepo` methods `RandomQuestion`, `GetQuestion`, `SaveSession`, `ListSessions`, `GetSession` (satisfying `DiscussionRepository`). Firestore layout: top-level `discussion_questions/{id}` (numeric string ids, like `sentences`), per-user `users/{uid}/discussion_sessions/{autoId}`.

- [ ] **Step 1: Write the failing tests**

Create `api/internal/app/firestore_discussion_test.go`:

```go
package app

import (
	"context"
	"strconv"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

func seedDiscussionQuestion(t *testing.T, client *firestore.Client, id int, question, topic string, level int, active bool) {
	t.Helper()
	_, err := client.Collection("discussion_questions").Doc(strconv.Itoa(id)).Set(context.Background(), map[string]interface{}{
		"question_en": question, "topic": topic, "level": level,
		"target_skills": []string{"giving opinions"}, "is_active": active,
		"created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("seed discussion question %d: %v", id, err)
	}
}

func sampleSession() *DiscussionSession {
	return &DiscussionSession{
		QuestionID: 1,
		QuestionEN: "Who should take more responsibility for environmental problems?",
		Topic:      "environment",
		Transcript: []DiscussionMessage{
			{Role: "user", Text: "I think companies."},
			{Role: "ai", Text: "Why do you think so?"},
			{Role: "user", Text: "Because they pollute more."},
		},
		ReflectionJA:   "制度を変える必要がある。",
		ExpressedIdeas: []string{"Companies are responsible."},
		MissingIdeas:   []string{"Systemic change is needed."},
		Expressions: []Expression{
			{Phrase: "take responsibility for", MeaningJA: "〜に責任を持つ", ExampleEN: "Companies should take responsibility for pollution."},
		},
		FirstAnswer:   "I think companies.",
		RetryAnswer:   "Companies should take responsibility for their impact.",
		RetryFeedback: "Great improvement!",
	}
}

func TestFirestoreRandomQuestionFiltersInactive(t *testing.T) {
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	seedDiscussionQuestion(t, client, 1, "Active question?", "work", 2, true)
	seedDiscussionQuestion(t, client, 2, "Inactive question?", "work", 2, false)

	for i := 0; i < 5; i++ {
		q, err := repo.RandomQuestion(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if q.ID != 1 || q.QuestionEN != "Active question?" {
			t.Fatalf("expected only the active question, got %+v", q)
		}
	}
}

func TestFirestoreRandomQuestionEmpty(t *testing.T) {
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	if _, err := repo.RandomQuestion(context.Background()); err != ErrNoCandidate {
		t.Fatalf("expected ErrNoCandidate, got %v", err)
	}
}

func TestFirestoreGetQuestion(t *testing.T) {
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	seedDiscussionQuestion(t, client, 7, "A question?", "travel", 3, true)

	q, err := repo.GetQuestion(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.ID != 7 || q.Topic != "travel" || q.Level != 3 || len(q.TargetSkills) != 1 {
		t.Fatalf("unexpected question: %+v", q)
	}
	if _, err := repo.GetQuestion(context.Background(), 424242); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFirestoreSaveAndGetSession(t *testing.T) {
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	repo.now = func() time.Time { return time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC) }

	id, err := repo.SaveSession(context.Background(), "u1", sampleSession())
	if err != nil {
		t.Fatalf("save session: %v", err)
	}
	if id == "" {
		t.Fatal("expected a non-empty session id")
	}

	got, err := repo.GetSession(context.Background(), "u1", id)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.ID != id || got.QuestionID != 1 || len(got.Transcript) != 3 ||
		got.Expressions[0].Phrase != "take responsibility for" ||
		got.RetryFeedback != "Great improvement!" ||
		got.CreatedAt != "2026-08-23T10:00:00Z" {
		t.Fatalf("unexpected session: %+v", got)
	}
}

func TestFirestoreGetSessionIsolatedPerUser(t *testing.T) {
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	id, err := repo.SaveSession(context.Background(), "u1", sampleSession())
	if err != nil {
		t.Fatalf("save session: %v", err)
	}
	if _, err := repo.GetSession(context.Background(), "u2", id); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for another user's session, got %v", err)
	}
}

func TestFirestoreListSessionsNewestFirstWithLimit(t *testing.T) {
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	base := time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		i := i
		repo.now = func() time.Time { return base.Add(time.Duration(i) * time.Minute) }
		s := sampleSession()
		s.QuestionEN = "Q" + strconv.Itoa(i)
		if _, err := repo.SaveSession(context.Background(), "u1", s); err != nil {
			t.Fatalf("save session %d: %v", i, err)
		}
	}

	got, err := repo.ListSessions(context.Background(), "u1", 2)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(got) != 2 || got[0].QuestionEN != "Q2" || got[1].QuestionEN != "Q1" {
		t.Fatalf("expected newest-first capped list, got %+v", got)
	}
	if got[0].Topic != "environment" || got[0].CreatedAt == "" || got[0].ID == "" {
		t.Fatalf("summary fields missing: %+v", got[0])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `firebase emulators:exec --project eagle-test --only firestore "cd api && go test ./internal/app -run 'TestFirestoreRandomQuestion|TestFirestoreGetQuestion|TestFirestoreSaveAndGet|TestFirestoreGetSessionIsolated|TestFirestoreListSessions' -v"` (from repo root)
Expected: compile FAIL — repo methods undefined.

- [ ] **Step 3: Write the implementation**

Create `api/internal/app/firestore_discussion.go`:

```go
package app

import (
	"context"
	"errors"
	"math/rand"
	"strconv"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type discussionQuestionDoc struct {
	QuestionEN   string   `firestore:"question_en"`
	Topic        string   `firestore:"topic"`
	Level        int      `firestore:"level"`
	TargetSkills []string `firestore:"target_skills"`
	IsActive     bool     `firestore:"is_active"`
	CreatedAt    string   `firestore:"created_at"`
	UpdatedAt    string   `firestore:"updated_at"`
}

type discussionMessageDoc struct {
	Role string `firestore:"role"`
	Text string `firestore:"text"`
}

type expressionDoc struct {
	Phrase    string `firestore:"phrase"`
	MeaningJA string `firestore:"meaning_ja"`
	ExampleEN string `firestore:"example_en"`
}

type discussionSessionDoc struct {
	QuestionID     int                    `firestore:"question_id"`
	QuestionEN     string                 `firestore:"question_en"`
	Topic          string                 `firestore:"topic"`
	Transcript     []discussionMessageDoc `firestore:"transcript"`
	ReflectionJA   string                 `firestore:"reflection_ja"`
	ExpressedIdeas []string               `firestore:"expressed_ideas"`
	MissingIdeas   []string               `firestore:"missing_ideas"`
	Expressions    []expressionDoc        `firestore:"expressions"`
	FirstAnswer    string                 `firestore:"first_answer"`
	RetryAnswer    string                 `firestore:"retry_answer"`
	RetryFeedback  string                 `firestore:"retry_feedback"`
	CreatedAt      time.Time              `firestore:"created_at"`
}

func (r *firestoreRepo) userDiscussionSessions(uid string) *firestore.CollectionRef {
	return r.client.Collection("users").Doc(uid).Collection("discussion_sessions")
}

func questionFromDoc(id int, qd *discussionQuestionDoc) *DiscussionQuestion {
	skills := qd.TargetSkills
	if skills == nil {
		skills = []string{}
	}
	return &DiscussionQuestion{
		ID: id, QuestionEN: qd.QuestionEN, Topic: qd.Topic,
		Level: qd.Level, TargetSkills: skills,
	}
}

func (r *firestoreRepo) RandomQuestion(ctx context.Context) (*DiscussionQuestion, error) {
	docs, err := r.client.Collection("discussion_questions").
		Where("is_active", "==", true).Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}
	var candidates []*DiscussionQuestion
	for _, ds := range docs {
		id, convErr := strconv.Atoi(ds.Ref.ID)
		if convErr != nil {
			continue
		}
		var qd discussionQuestionDoc
		if err := ds.DataTo(&qd); err != nil {
			return nil, err
		}
		candidates = append(candidates, questionFromDoc(id, &qd))
	}
	if len(candidates) == 0 {
		return nil, ErrNoCandidate
	}
	return candidates[rand.Intn(len(candidates))], nil
}

func (r *firestoreRepo) GetQuestion(ctx context.Context, id int) (*DiscussionQuestion, error) {
	ds, err := r.client.Collection("discussion_questions").Doc(strconv.Itoa(id)).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var qd discussionQuestionDoc
	if err := ds.DataTo(&qd); err != nil {
		return nil, err
	}
	return questionFromDoc(id, &qd), nil
}

func sessionToDoc(s *DiscussionSession, createdAt time.Time) *discussionSessionDoc {
	transcript := make([]discussionMessageDoc, len(s.Transcript))
	for i, m := range s.Transcript {
		transcript[i] = discussionMessageDoc{Role: m.Role, Text: m.Text}
	}
	expressions := make([]expressionDoc, len(s.Expressions))
	for i, e := range s.Expressions {
		expressions[i] = expressionDoc{Phrase: e.Phrase, MeaningJA: e.MeaningJA, ExampleEN: e.ExampleEN}
	}
	return &discussionSessionDoc{
		QuestionID: s.QuestionID, QuestionEN: s.QuestionEN, Topic: s.Topic,
		Transcript: transcript, ReflectionJA: s.ReflectionJA,
		ExpressedIdeas: append([]string{}, s.ExpressedIdeas...),
		MissingIdeas:   append([]string{}, s.MissingIdeas...),
		Expressions:    expressions,
		FirstAnswer:    s.FirstAnswer, RetryAnswer: s.RetryAnswer,
		RetryFeedback: s.RetryFeedback, CreatedAt: createdAt,
	}
}

func sessionFromDoc(id string, sd *discussionSessionDoc) *DiscussionSession {
	transcript := make([]DiscussionMessage, len(sd.Transcript))
	for i, m := range sd.Transcript {
		transcript[i] = DiscussionMessage{Role: m.Role, Text: m.Text}
	}
	expressions := make([]Expression, len(sd.Expressions))
	for i, e := range sd.Expressions {
		expressions[i] = Expression{Phrase: e.Phrase, MeaningJA: e.MeaningJA, ExampleEN: e.ExampleEN}
	}
	expressed := sd.ExpressedIdeas
	if expressed == nil {
		expressed = []string{}
	}
	missing := sd.MissingIdeas
	if missing == nil {
		missing = []string{}
	}
	return &DiscussionSession{
		ID: id, QuestionID: sd.QuestionID, QuestionEN: sd.QuestionEN, Topic: sd.Topic,
		Transcript: transcript, ReflectionJA: sd.ReflectionJA,
		ExpressedIdeas: expressed, MissingIdeas: missing, Expressions: expressions,
		FirstAnswer: sd.FirstAnswer, RetryAnswer: sd.RetryAnswer,
		RetryFeedback: sd.RetryFeedback,
		CreatedAt:     sd.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func (r *firestoreRepo) SaveSession(ctx context.Context, uid string, s *DiscussionSession) (string, error) {
	ref := r.userDiscussionSessions(uid).NewDoc()
	if _, err := ref.Set(ctx, sessionToDoc(s, r.now().UTC())); err != nil {
		return "", err
	}
	return ref.ID, nil
}

func (r *firestoreRepo) ListSessions(ctx context.Context, uid string, limit int) ([]DiscussionSessionSummary, error) {
	it := r.userDiscussionSessions(uid).
		OrderBy("created_at", firestore.Desc).Limit(limit).Documents(ctx)
	summaries := make([]DiscussionSessionSummary, 0)
	for {
		ds, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		var sd discussionSessionDoc
		if err := ds.DataTo(&sd); err != nil {
			return nil, err
		}
		summaries = append(summaries, DiscussionSessionSummary{
			ID: ds.Ref.ID, QuestionEN: sd.QuestionEN, Topic: sd.Topic,
			CreatedAt: sd.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return summaries, nil
}

func (r *firestoreRepo) GetSession(ctx context.Context, uid, id string) (*DiscussionSession, error) {
	ds, err := r.userDiscussionSessions(uid).Doc(id).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var sd discussionSessionDoc
	if err := ds.DataTo(&sd); err != nil {
		return nil, err
	}
	return sessionFromDoc(ds.Ref.ID, &sd), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run the same `firebase emulators:exec ...` command from Step 2.
Expected: PASS. Also `cd api && go test ./...` (emulator tests skip, everything else passes).

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/firestore_discussion.go api/internal/app/firestore_discussion_test.go
git commit -m "Add Firestore discussion question and session repository

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: Server wiring + question & reply handlers

**Files:**
- Create: `api/internal/app/discussion_handlers.go`
- Modify: `api/internal/app/handlers.go` (Server struct only), `api/internal/app/router.go`
- Test: `api/internal/app/discussion_handlers_test.go`

**Interfaces:**
- Consumes: Tasks 1–2 types, `writeJSON`, `uidFromContext`, `authed` test helper, `fakeRepo`/`fakeExplainer`/`fakeAnalyzer` (existing).
- Produces: `(*Server).WithDiscussion(repo DiscussionRepository, coach DiscussionCoach) *Server`; handlers `getDiscussionQuestion`, `discussionReply`; request type `DiscussionReplyRequest{QuestionID int `+"`json:\"question_id\"`"+`; Transcript []DiscussionMessage `+"`json:\"transcript\"`"+`}`; test fakes `fakeDiscussionRepo`, `fakeCoach` reused by Tasks 6–7. Routes `/api/discussion/question`, `/api/discussion/reply`.

- [ ] **Step 1: Write the failing tests**

Create `api/internal/app/discussion_handlers_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/app -run 'TestGetDiscussionQuestion|TestDiscussionReply' -v`
Expected: compile FAIL — `WithDiscussion`, handlers, `DiscussionReplyRequest` undefined.

- [ ] **Step 3: Write the implementation**

In `api/internal/app/handlers.go`, extend the Server struct (leave `NewServer` unchanged):

```go
type Server struct {
	repo      SentenceRepository
	explainer Explainer
	analyzer  WeaknessAnalyzer

	discussions DiscussionRepository
	coach       DiscussionCoach
}
```

Create `api/internal/app/discussion_handlers.go`:

```go
package app

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
)

// WithDiscussion attaches the discussion-practice dependencies. A chained
// setter rather than new NewServer parameters so the existing constructor's
// many call sites stay unchanged.
func (s *Server) WithDiscussion(repo DiscussionRepository, coach DiscussionCoach) *Server {
	s.discussions = repo
	s.coach = coach
	return s
}

type DiscussionReplyRequest struct {
	QuestionID int                 `json:"question_id"`
	Transcript []DiscussionMessage `json:"transcript"`
}

func (s *Server) getDiscussionQuestion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q, err := s.discussions.RandomQuestion(r.Context())
	if errors.Is(err, ErrNoCandidate) {
		http.Error(w, "No questions found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("random discussion question error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, q)
}

// decodeDiscussionBody bounds and strictly decodes a discussion request
// body. Returns false after writing the 400 response itself.
func decodeDiscussionBody(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxDiscussionRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return false
	}
	return true
}

// loadDiscussionQuestion fetches the question by id, writing the error
// response itself when it fails (nil result means "already handled").
func (s *Server) loadDiscussionQuestion(w http.ResponseWriter, r *http.Request, id int) *DiscussionQuestion {
	q, err := s.discussions.GetQuestion(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "Question not found", http.StatusNotFound)
		return nil
	}
	if err != nil {
		log.Printf("get discussion question error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return nil
	}
	return q
}

func (s *Server) discussionReply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req DiscussionReplyRequest
	if !decodeDiscussionBody(w, r, &req) {
		return
	}
	if err := validateTranscript(req.Transcript); err != nil {
		http.Error(w, "Invalid transcript", http.StatusBadRequest)
		return
	}
	if req.Transcript[len(req.Transcript)-1].Role != "user" {
		http.Error(w, "Invalid transcript", http.StatusBadRequest)
		return
	}
	q := s.loadDiscussionQuestion(w, r, req.QuestionID)
	if q == nil {
		return
	}
	// Server-side hard cap: past maxAIFollowUps the conversation is over no
	// matter what the model would say — and Gemini is never even called.
	if countAITurns(req.Transcript) >= maxAIFollowUps {
		writeJSON(w, CoachReply{Done: true, Message: ""})
		return
	}
	reply, err := s.coach.Reply(r.Context(), q, req.Transcript)
	if err != nil {
		log.Printf("discussion reply error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, reply)
}

// discussionTrimmed reports whether text is non-blank after trimming and
// within limit.
func discussionTrimmed(text string, limit int) bool {
	t := strings.TrimSpace(text)
	return t != "" && len(text) <= limit
}
```

In `api/internal/app/router.go`, add inside `NewMux` after the existing routes:

```go
	mux.HandleFunc("/api/discussion/question", auth(srv.getDiscussionQuestion))
	mux.HandleFunc("/api/discussion/reply", auth(srv.discussionReply))
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./internal/app -run 'TestGetDiscussionQuestion|TestDiscussionReply' -v` then `go test ./...`
Expected: PASS. (`discussionTrimmed` is unused until Task 6 — Go allows unused functions, only unused imports/variables fail; if `strings` ends up unused the build breaks, but `discussionTrimmed` uses it.)

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/discussion_handlers.go api/internal/app/discussion_handlers_test.go api/internal/app/handlers.go api/internal/app/router.go
git commit -m "Add discussion question and reply endpoints

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: Analyze & complete handlers

**Files:**
- Modify: `api/internal/app/discussion_handlers.go`, `api/internal/app/router.go`
- Test: `api/internal/app/discussion_handlers_test.go` (append)

**Interfaces:**
- Consumes: fakes and helpers from Task 5, `uidFromContext` (existing).
- Produces: handlers `discussionAnalyze`, `discussionComplete`; types `DiscussionAnalyzeRequest`, `DiscussionCompleteRequest`, `DiscussionCompleteResponse{SessionID string `+"`json:\"session_id\"`"+`; RetryFeedback string `+"`json:\"retry_feedback\"`"+`}`. Routes `/api/discussion/analyze`, `/api/discussion/complete`.

- [ ] **Step 1: Write the failing tests**

Append to `api/internal/app/discussion_handlers_test.go`:

```go
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
```

Also add `"strings"` to the test file's imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/app -run 'TestDiscussionAnalyze|TestDiscussionComplete' -v`
Expected: compile FAIL — request types and handlers undefined.

- [ ] **Step 3: Write the implementation**

Append to `api/internal/app/discussion_handlers.go`:

```go
type DiscussionAnalyzeRequest struct {
	QuestionID   int                 `json:"question_id"`
	Transcript   []DiscussionMessage `json:"transcript"`
	ReflectionJA string              `json:"reflection_ja"`
}

type DiscussionCompleteRequest struct {
	QuestionID     int                 `json:"question_id"`
	Transcript     []DiscussionMessage `json:"transcript"`
	ReflectionJA   string              `json:"reflection_ja"`
	ExpressedIdeas []string            `json:"expressed_ideas"`
	MissingIdeas   []string            `json:"missing_ideas"`
	Expressions    []Expression        `json:"expressions"`
	RetryAnswer    string              `json:"retry_answer"`
}

type DiscussionCompleteResponse struct {
	SessionID     string `json:"session_id"`
	RetryFeedback string `json:"retry_feedback"`
}

func (s *Server) discussionAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req DiscussionAnalyzeRequest
	if !decodeDiscussionBody(w, r, &req) {
		return
	}
	if err := validateTranscript(req.Transcript); err != nil {
		http.Error(w, "Invalid transcript", http.StatusBadRequest)
		return
	}
	if !discussionTrimmed(req.ReflectionJA, maxReflectionLength) {
		http.Error(w, "Invalid reflection_ja", http.StatusBadRequest)
		return
	}
	q := s.loadDiscussionQuestion(w, r, req.QuestionID)
	if q == nil {
		return
	}
	analysis, err := s.coach.AnalyzeGap(r.Context(), q, req.Transcript, req.ReflectionJA)
	if err != nil {
		log.Printf("discussion analyze error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, analysis)
}

func (s *Server) discussionComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid, _ := uidFromContext(r.Context())
	var req DiscussionCompleteRequest
	if !decodeDiscussionBody(w, r, &req) {
		return
	}
	if err := validateTranscript(req.Transcript); err != nil {
		http.Error(w, "Invalid transcript", http.StatusBadRequest)
		return
	}
	if !discussionTrimmed(req.RetryAnswer, maxDiscussionTurnLength) {
		http.Error(w, "Invalid retry_answer", http.StatusBadRequest)
		return
	}
	// ReflectionJA is "" when the reflection was skipped; when present it
	// obeys the same bound as the analyze endpoint.
	if len(req.ReflectionJA) > maxReflectionLength {
		http.Error(w, "Invalid reflection_ja", http.StatusBadRequest)
		return
	}
	if len(req.Expressions) > 4 || len(req.ExpressedIdeas) > 20 || len(req.MissingIdeas) > 20 {
		http.Error(w, "Invalid analysis payload", http.StatusBadRequest)
		return
	}
	q := s.loadDiscussionQuestion(w, r, req.QuestionID)
	if q == nil {
		return
	}
	firstAnswer := req.Transcript[0].Text
	feedback, err := s.coach.ReviewRetry(r.Context(), q, firstAnswer, req.RetryAnswer, req.Expressions)
	if err != nil {
		log.Printf("discussion retry review error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	session := &DiscussionSession{
		QuestionID:     q.ID,
		QuestionEN:     q.QuestionEN,
		Topic:          q.Topic,
		Transcript:     req.Transcript,
		ReflectionJA:   req.ReflectionJA,
		ExpressedIdeas: req.ExpressedIdeas,
		MissingIdeas:   req.MissingIdeas,
		Expressions:    req.Expressions,
		FirstAnswer:    firstAnswer,
		RetryAnswer:    req.RetryAnswer,
		RetryFeedback:  feedback,
	}
	sessionID, err := s.discussions.SaveSession(r.Context(), uid, session)
	if err != nil {
		log.Printf("save discussion session error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, DiscussionCompleteResponse{SessionID: sessionID, RetryFeedback: feedback})
}
```

In `api/internal/app/router.go`, add:

```go
	mux.HandleFunc("/api/discussion/analyze", auth(srv.discussionAnalyze))
	mux.HandleFunc("/api/discussion/complete", auth(srv.discussionComplete))
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./internal/app -run 'TestDiscussionAnalyze|TestDiscussionComplete' -v` then `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/discussion_handlers.go api/internal/app/discussion_handlers_test.go api/internal/app/router.go
git commit -m "Add discussion analyze and complete endpoints

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: Session history endpoints

**Files:**
- Modify: `api/internal/app/discussion_handlers.go`, `api/internal/app/router.go`
- Test: `api/internal/app/discussion_handlers_test.go` (append)

**Interfaces:**
- Consumes: fakes from Task 5.
- Produces: handler `discussionSessions` serving both `GET /api/discussion/sessions` (list) and `GET /api/discussion/sessions/{id}` (detail) by parsing the path suffix; response type `DiscussionSessionsResponse{Sessions []DiscussionSessionSummary `+"`json:\"sessions\"`"+`}`.

- [ ] **Step 1: Write the failing tests**

Append to `api/internal/app/discussion_handlers_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/app -run 'TestListDiscussionSessions|TestGetDiscussionSession|TestDiscussionSessionsRejects' -v`
Expected: compile FAIL.

- [ ] **Step 3: Write the implementation**

Append to `api/internal/app/discussion_handlers.go`:

```go
type DiscussionSessionsResponse struct {
	Sessions []DiscussionSessionSummary `json:"sessions"`
}

// discussionSessions serves both the list (no path suffix) and the detail
// (suffix = session id). One handler because ServeMux's trailing-slash
// pattern would otherwise split them across two registrations anyway.
func (s *Server) discussionSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid, _ := uidFromContext(r.Context())
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/discussion/sessions"), "/")
	if id == "" {
		sessions, err := s.discussions.ListSessions(r.Context(), uid, maxDiscussionSessionList)
		if err != nil {
			log.Printf("list discussion sessions error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, DiscussionSessionsResponse{Sessions: sessions})
		return
	}
	session, err := s.discussions.GetSession(r.Context(), uid, id)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("get discussion session error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, session)
}
```

In `api/internal/app/router.go`, add:

```go
	mux.HandleFunc("/api/discussion/sessions", auth(srv.discussionSessions))
	mux.HandleFunc("/api/discussion/sessions/", auth(srv.discussionSessions))
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/discussion_handlers.go api/internal/app/discussion_handlers_test.go api/internal/app/router.go
git commit -m "Add discussion session history endpoints

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 8: Wire production server and e2e stub coach

**Files:**
- Modify: `api/main.go`, `api/cmd/e2eserver/main.go`

No new unit tests: this task is pure wiring of already-tested pieces; the compiler plus `go vet ./...` is the check, and the e2e suite (Task 16) exercises the stub end to end.

- [ ] **Step 1: Wire main.go**

In `api/main.go`, after the `analyzer` block, create the coach and attach discussion deps (the `firestoreRepo` already satisfies both repository interfaces):

```go
	coach, err := app.NewGeminiCoach(ctx, geminiAPIKey)
	if err != nil {
		log.Fatalf("failed to create Gemini coach: %v", err)
	}

	repo := app.NewFirestoreRepo(client)
	srv := app.NewServer(repo, explainer, analyzer).WithDiscussion(repo, coach)
```

(Replace the existing `srv := app.NewServer(app.NewFirestoreRepo(client), explainer, analyzer)` line.)

- [ ] **Step 2: Add stubCoach to e2eserver**

In `api/cmd/e2eserver/main.go`, add alongside the existing stubs:

```go
// stubCoach avoids real Gemini calls in e2e tests. Deterministic: two
// follow-ups, then done; fixed analysis; fixed retry feedback. The literals
// are asserted verbatim by e2e/tests/discussion.spec.ts.
type stubCoach struct{}

func (stubCoach) Reply(_ context.Context, _ *app.DiscussionQuestion, transcript []app.DiscussionMessage) (*app.CoachReply, error) {
	aiTurns := 0
	for _, m := range transcript {
		if m.Role == "ai" {
			aiTurns++
		}
	}
	if aiTurns >= 2 {
		return &app.CoachReply{Done: true, Message: "Great, thanks for sharing your thoughts!"}, nil
	}
	return &app.CoachReply{Done: false, Message: fmt.Sprintf("Stub follow-up %d: can you tell me more?", aiTurns+1)}, nil
}

func (stubCoach) AnalyzeGap(_ context.Context, _ *app.DiscussionQuestion, _ []app.DiscussionMessage, _ string) (*app.GapAnalysis, error) {
	return &app.GapAnalysis{
		ExpressedIdeas: []string{"You said companies are responsible."},
		MissingIdeas:   []string{"Systemic change is more effective than individual action."},
		Expressions: []app.Expression{
			{Phrase: "take responsibility for", MeaningJA: "〜に責任を持つ", ExampleEN: "Companies should take responsibility for their impact."},
			{Phrase: "make systemic changes", MeaningJA: "制度的な変更を行う", ExampleEN: "Governments can make systemic changes."},
		},
	}, nil
}

func (stubCoach) ReviewRetry(_ context.Context, _ *app.DiscussionQuestion, _, _ string, _ []app.Expression) (string, error) {
	return "This is a stub retry feedback for e2e tests.", nil
}
```

Add `"fmt"` to the imports, and change the server construction line to:

```go
	repo := app.NewFirestoreRepo(client)
	srv := app.NewServer(repo, stubExplainer{}, stubAnalyzer{}).WithDiscussion(repo, stubCoach{})
```

(Check the current construction line first — mirror however it names things, only adding `repo` reuse and `.WithDiscussion(repo, stubCoach{})`.)

- [ ] **Step 3: Verify it builds**

Run: `cd api && go build ./... && go vet ./... && go test ./...`
Expected: clean build, vet, and tests.

- [ ] **Step 4: Commit**

```bash
git add api/main.go api/cmd/e2eserver/main.go
git commit -m "Wire discussion coach into production and e2e servers

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 9: Seed command and curated question bank

**Files:**
- Create: `api/cmd/seedquestions/main.go`, `docs/discussion_questions_seed.ndjson`
- Test: `api/cmd/seedquestions/main_test.go`

**Interfaces:**
- Consumes: nothing from other tasks (standalone command mirroring `api/cmd/seed`).
- Produces: `go run ./cmd/seedquestions -file <ndjson>` seeding the `discussion_questions` collection. NDJSON row shape: `{"id": int, "topic": str, "level": int, "question_en": str, "target_skills": [str], "is_active": 0|1}`. Used by Task 16 (e2e fixture seeding) and by the owner to seed production.

- [ ] **Step 1: Write the failing test**

Create `api/cmd/seedquestions/main_test.go`:

```go
package main

import "testing"

func validRow() row {
	return row{
		ID: 1, Topic: "environment", Level: 3,
		QuestionEN:   "Who should take more responsibility for environmental problems?",
		TargetSkills: []string{"giving opinions"},
		IsActive:     1,
	}
}

func TestToFirestoreFieldsOK(t *testing.T) {
	fields, err := toFirestoreFields(validRow(), "2026-08-23T00:00:00Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fields["question_en"] != validRow().QuestionEN || fields["is_active"] != true ||
		fields["level"] != 3 || fields["topic"] != "environment" ||
		fields["created_at"] != "2026-08-23T00:00:00Z" {
		t.Fatalf("unexpected fields: %+v", fields)
	}
	skills, ok := fields["target_skills"].([]string)
	if !ok || len(skills) != 1 {
		t.Fatalf("unexpected target_skills: %+v", fields["target_skills"])
	}
}

func TestToFirestoreFieldsRejectsBadRows(t *testing.T) {
	bad := validRow()
	bad.Level = 6
	if _, err := toFirestoreFields(bad, "now"); err == nil {
		t.Fatal("expected error for level out of range")
	}
	bad = validRow()
	bad.QuestionEN = "  "
	if _, err := toFirestoreFields(bad, "now"); err == nil {
		t.Fatal("expected error for blank question")
	}
	bad = validRow()
	bad.Topic = ""
	if _, err := toFirestoreFields(bad, "now"); err == nil {
		t.Fatal("expected error for blank topic")
	}
	bad = validRow()
	bad.TargetSkills = nil
	if _, err := toFirestoreFields(bad, "now"); err == nil {
		t.Fatal("expected error for missing target_skills")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./cmd/seedquestions -v`
Expected: compile FAIL — package doesn't exist yet.

- [ ] **Step 3: Write the command**

Create `api/cmd/seedquestions/main.go` (mirrors `cmd/seed/main.go`):

```go
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
)

type row struct {
	ID           int      `json:"id"`
	Topic        string   `json:"topic"`
	Level        int      `json:"level"`
	QuestionEN   string   `json:"question_en"`
	TargetSkills []string `json:"target_skills"`
	IsActive     int      `json:"is_active"`
}

// toFirestoreFields builds the Firestore write payload for one NDJSON row,
// validating the fields the app depends on.
func toFirestoreFields(rw row, now string) (map[string]interface{}, error) {
	if rw.Level < 1 || rw.Level > 5 {
		return nil, fmt.Errorf("question %d: level must be 1-5, got %d", rw.ID, rw.Level)
	}
	if strings.TrimSpace(rw.QuestionEN) == "" {
		return nil, fmt.Errorf("question %d: question_en is blank", rw.ID)
	}
	if strings.TrimSpace(rw.Topic) == "" {
		return nil, fmt.Errorf("question %d: topic is blank", rw.ID)
	}
	if len(rw.TargetSkills) == 0 {
		return nil, fmt.Errorf("question %d: target_skills is empty", rw.ID)
	}
	return map[string]interface{}{
		"question_en":   rw.QuestionEN,
		"topic":         rw.Topic,
		"level":         rw.Level,
		"target_skills": rw.TargetSkills,
		"is_active":     rw.IsActive != 0,
		"created_at":    now,
		"updated_at":    now,
	}, nil
}

func main() {
	path := flag.String("file", "docs/discussion_questions_seed.ndjson", "path to NDJSON file")
	flag.Parse()

	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		log.Fatal("GOOGLE_CLOUD_PROJECT is required")
	}

	ctx := context.Background()
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("firestore client: %v", err)
	}
	defer client.Close()

	f, err := os.Open(*path)
	if err != nil {
		log.Fatalf("open %s: %v", *path, err)
	}
	defer f.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	count := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rw row
		if err := json.Unmarshal(line, &rw); err != nil {
			log.Fatalf("parse line: %v", err)
		}
		fields, err := toFirestoreFields(rw, now)
		if err != nil {
			log.Fatalf("invalid row: %v", err)
		}
		if _, err := client.Collection("discussion_questions").Doc(strconv.Itoa(rw.ID)).Set(ctx, fields); err != nil {
			log.Fatalf("write question %d: %v", rw.ID, err)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("scan: %v", err)
	}
	fmt.Printf("seeded %d discussion questions\n", count)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go test ./cmd/seedquestions -v`
Expected: PASS.

- [ ] **Step 5: Create the curated question bank**

Create `docs/discussion_questions_seed.ndjson` with exactly these 30 rows (the owner reviews this file before seeding production):

```
{"id": 1, "topic": "daily life", "level": 2, "question_en": "Do you prefer eating at home or eating out? Why?", "target_skills": ["giving opinions", "giving reasons"], "is_active": 1}
{"id": 2, "topic": "daily life", "level": 3, "question_en": "How has your daily routine changed in the last few years, and is it better now?", "target_skills": ["describing changes", "comparing past and present"], "is_active": 1}
{"id": 3, "topic": "daily life", "level": 3, "question_en": "Is it better to live in a big city or in the countryside?", "target_skills": ["comparing options", "discussing advantages and disadvantages"], "is_active": 1}
{"id": 4, "topic": "work", "level": 2, "question_en": "What is more important in a job: money or free time? Why?", "target_skills": ["giving opinions", "comparing options"], "is_active": 1}
{"id": 5, "topic": "work", "level": 3, "question_en": "Should people be allowed to work from home whenever they want?", "target_skills": ["giving opinions", "discussing advantages and disadvantages"], "is_active": 1}
{"id": 6, "topic": "work", "level": 4, "question_en": "Is it a company's job to make employees happy, or is that each person's own responsibility?", "target_skills": ["giving opinions", "giving reasons", "comparing options"], "is_active": 1}
{"id": 7, "topic": "technology", "level": 2, "question_en": "Could you live without your smartphone for one week? Why or why not?", "target_skills": ["giving opinions", "giving reasons"], "is_active": 1}
{"id": 8, "topic": "technology", "level": 3, "question_en": "Do social media bring people closer together or push them further apart?", "target_skills": ["giving opinions", "discussing advantages and disadvantages"], "is_active": 1}
{"id": 9, "topic": "technology", "level": 4, "question_en": "Should children's screen time be limited by parents, by schools, or by nobody?", "target_skills": ["comparing options", "giving reasons"], "is_active": 1}
{"id": 10, "topic": "AI", "level": 3, "question_en": "Would you trust an AI to make an important decision for you, like choosing a job?", "target_skills": ["giving opinions", "giving reasons"], "is_active": 1}
{"id": 11, "topic": "AI", "level": 3, "question_en": "Which jobs do you think AI should never replace?", "target_skills": ["giving opinions", "giving examples"], "is_active": 1}
{"id": 12, "topic": "AI", "level": 4, "question_en": "Who should be responsible when an AI system makes a serious mistake: the developer, the user, or the company?", "target_skills": ["comparing options", "giving reasons"], "is_active": 1}
{"id": 13, "topic": "education", "level": 2, "question_en": "What school subject do you think is the most useful in real life?", "target_skills": ["giving opinions", "giving examples"], "is_active": 1}
{"id": 14, "topic": "education", "level": 3, "question_en": "Who should take more responsibility for children's education: parents or schools?", "target_skills": ["comparing options", "giving reasons"], "is_active": 1}
{"id": 15, "topic": "education", "level": 4, "question_en": "Should university education be free for everyone?", "target_skills": ["giving opinions", "discussing advantages and disadvantages"], "is_active": 1}
{"id": 16, "topic": "environment", "level": 3, "question_en": "Who should take more responsibility for environmental problems: individuals, companies, or governments?", "target_skills": ["giving opinions", "giving reasons", "comparing options"], "is_active": 1}
{"id": 17, "topic": "environment", "level": 2, "question_en": "What is one thing you do, or could do, to help the environment?", "target_skills": ["giving examples", "describing habits"], "is_active": 1}
{"id": 18, "topic": "environment", "level": 4, "question_en": "Would you accept a higher cost of living if it helped stop climate change?", "target_skills": ["giving opinions", "discussing trade-offs"], "is_active": 1}
{"id": 19, "topic": "society", "level": 3, "question_en": "Should people be required to retire at a certain age?", "target_skills": ["giving opinions", "giving reasons"], "is_active": 1}
{"id": 20, "topic": "society", "level": 4, "question_en": "Is it fair that some jobs pay so much more than others?", "target_skills": ["giving opinions", "giving reasons"], "is_active": 1}
{"id": 21, "topic": "society", "level": 3, "question_en": "Do you think cash will disappear in the future? Would that be good or bad?", "target_skills": ["making predictions", "discussing advantages and disadvantages"], "is_active": 1}
{"id": 22, "topic": "travel", "level": 2, "question_en": "Do you prefer traveling alone or with other people? Why?", "target_skills": ["giving opinions", "giving reasons"], "is_active": 1}
{"id": 23, "topic": "travel", "level": 3, "question_en": "Is it better to visit many places quickly or stay in one place for a long time?", "target_skills": ["comparing options", "giving reasons"], "is_active": 1}
{"id": 24, "topic": "travel", "level": 3, "question_en": "Does tourism help or hurt the places people visit?", "target_skills": ["discussing advantages and disadvantages", "giving examples"], "is_active": 1}
{"id": 25, "topic": "culture", "level": 3, "question_en": "What is one part of your culture you would like people from other countries to understand?", "target_skills": ["explaining", "giving examples"], "is_active": 1}
{"id": 26, "topic": "culture", "level": 3, "question_en": "Is it important to keep old traditions, even when they are inconvenient?", "target_skills": ["giving opinions", "giving reasons"], "is_active": 1}
{"id": 27, "topic": "culture", "level": 4, "question_en": "When companies from other countries use parts of your culture in their products, is that a problem?", "target_skills": ["giving opinions", "giving reasons"], "is_active": 1}
{"id": 28, "topic": "career", "level": 2, "question_en": "Would you rather have a boring job with high pay or an interesting job with low pay?", "target_skills": ["comparing options", "giving reasons"], "is_active": 1}
{"id": 29, "topic": "career", "level": 3, "question_en": "Is changing jobs often good or bad for a career?", "target_skills": ["giving opinions", "discussing advantages and disadvantages"], "is_active": 1}
{"id": 30, "topic": "business", "level": 4, "question_en": "Should small local shops be protected from big chain stores?", "target_skills": ["giving opinions", "giving reasons", "discussing trade-offs"], "is_active": 1}
```

- [ ] **Step 6: Commit**

```bash
git add api/cmd/seedquestions docs/discussion_questions_seed.ndjson
git commit -m "Add seedquestions command and curated question bank

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 10: Frontend API client

**Files:**
- Modify: `fe/src/lib/api.ts`
- Test: `fe/src/lib/api.test.ts` (append)

**Interfaces:**
- Consumes: existing `request<T>` helper in `api.ts`.
- Produces (used by all component tasks): TypeScript types `DiscussionQuestion`, `DiscussionMessage`, `Expression`, `GapAnalysis`, `DiscussionReplyResponse`, `DiscussionCompleteRequest`, `DiscussionCompleteResponse`, `DiscussionSessionSummary`, `DiscussionSessionDetail`; api methods `getDiscussionQuestion()`, `discussionReply(questionId, transcript)`, `discussionAnalyze(questionId, transcript, reflectionJa)`, `discussionComplete(payload)`, `listDiscussionSessions()`, `getDiscussionSession(id)`.

- [ ] **Step 1: Write the failing tests**

Append to `fe/src/lib/api.test.ts`:

```ts
describe('api.getDiscussionQuestion', () => {
  it('sends GET /api/discussion/question', async () => {
    mockResponse({
      id: 16,
      question_en: 'Who should take more responsibility for environmental problems?',
      topic: 'environment',
      level: 3,
      target_skills: ['giving opinions'],
    })
    const result = await api.getDiscussionQuestion()
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/discussion/question'),
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: 'Bearer test-token' }),
      })
    )
    expect(result.id).toBe(16)
    expect(result.topic).toBe('environment')
  })
})

describe('api.discussionReply', () => {
  it('sends POST /api/discussion/reply with question_id and transcript', async () => {
    mockResponse({ done: false, message: 'Why do you think so?' })
    const transcript = [{ role: 'user' as const, text: 'I think companies.' }]
    const result = await api.discussionReply(16, transcript)
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/discussion/reply'),
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ question_id: 16, transcript }),
      })
    )
    expect(result.done).toBe(false)
    expect(result.message).toBe('Why do you think so?')
  })
})

describe('api.discussionAnalyze', () => {
  it('sends POST /api/discussion/analyze with the reflection', async () => {
    mockResponse({ expressed_ideas: [], missing_ideas: [], expressions: [] })
    const transcript = [{ role: 'user' as const, text: 'I think companies.' }]
    await api.discussionAnalyze(16, transcript, '制度を変えるべき。')
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/discussion/analyze'),
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ question_id: 16, transcript, reflection_ja: '制度を変えるべき。' }),
      })
    )
  })
})

describe('api.discussionComplete', () => {
  it('sends POST /api/discussion/complete with the whole session payload', async () => {
    mockResponse({ session_id: 's1', retry_feedback: 'Nice!' })
    const payload = {
      question_id: 16,
      transcript: [{ role: 'user' as const, text: 'I think companies.' }],
      reflection_ja: '',
      expressed_ideas: [],
      missing_ideas: [],
      expressions: [],
      retry_answer: 'Companies should take responsibility.',
    }
    const result = await api.discussionComplete(payload)
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/discussion/complete'),
      expect.objectContaining({ method: 'POST', body: JSON.stringify(payload) })
    )
    expect(result.session_id).toBe('s1')
  })
})

describe('api.listDiscussionSessions / getDiscussionSession', () => {
  it('sends GET /api/discussion/sessions', async () => {
    mockResponse({ sessions: [{ id: 's1', question_en: 'Q', topic: 'work', created_at: '2026-08-23T10:00:00Z' }] })
    const result = await api.listDiscussionSessions()
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/discussion/sessions'),
      expect.anything()
    )
    expect(result.sessions).toHaveLength(1)
  })

  it('sends GET /api/discussion/sessions/{id}', async () => {
    mockResponse({ id: 's1', question_en: 'Q', retry_answer: 'better' })
    const result = await api.getDiscussionSession('s1')
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/discussion/sessions/s1'),
      expect.anything()
    )
    expect(result.id).toBe('s1')
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd fe && npx vitest run src/lib/api.test.ts`
Expected: FAIL — methods undefined.

- [ ] **Step 3: Write the implementation**

Append the types to `fe/src/lib/api.ts` (after the existing interfaces):

```ts
export interface DiscussionQuestion {
  id: number
  question_en: string
  topic: string
  level: number
  target_skills: string[]
}

export interface DiscussionMessage {
  role: 'user' | 'ai'
  text: string
}

export interface DiscussionReplyResponse {
  done: boolean
  message: string
}

export interface Expression {
  phrase: string
  meaning_ja: string
  example_en: string
}

export interface GapAnalysis {
  expressed_ideas: string[]
  missing_ideas: string[]
  expressions: Expression[]
}

export interface DiscussionCompleteRequest {
  question_id: number
  transcript: DiscussionMessage[]
  reflection_ja: string
  expressed_ideas: string[]
  missing_ideas: string[]
  expressions: Expression[]
  retry_answer: string
}

export interface DiscussionCompleteResponse {
  session_id: string
  retry_feedback: string
}

export interface DiscussionSessionSummary {
  id: string
  question_en: string
  topic: string
  created_at: string
}

export interface DiscussionSessionDetail {
  id: string
  question_id: number
  question_en: string
  topic: string
  transcript: DiscussionMessage[]
  reflection_ja: string
  expressed_ideas: string[]
  missing_ideas: string[]
  expressions: Expression[]
  first_answer: string
  retry_answer: string
  retry_feedback: string
  created_at: string
}
```

And add the methods inside the `api` object:

```ts
  getDiscussionQuestion: () => request<DiscussionQuestion>('/api/discussion/question'),

  discussionReply: (questionId: number, transcript: DiscussionMessage[]) =>
    request<DiscussionReplyResponse>('/api/discussion/reply', {
      method: 'POST',
      body: JSON.stringify({ question_id: questionId, transcript }),
    }),

  discussionAnalyze: (questionId: number, transcript: DiscussionMessage[], reflectionJa: string) =>
    request<GapAnalysis>('/api/discussion/analyze', {
      method: 'POST',
      body: JSON.stringify({ question_id: questionId, transcript, reflection_ja: reflectionJa }),
    }),

  discussionComplete: (payload: DiscussionCompleteRequest) =>
    request<DiscussionCompleteResponse>('/api/discussion/complete', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),

  listDiscussionSessions: () =>
    request<{ sessions: DiscussionSessionSummary[] }>('/api/discussion/sessions'),

  getDiscussionSession: (id: string) =>
    request<DiscussionSessionDetail>(`/api/discussion/sessions/${id}`),
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd fe && npx vitest run src/lib/api.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add fe/src/lib/api.ts fe/src/lib/api.test.ts
git commit -m "Add discussion endpoints to the frontend API client

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 11: ChatTranscript component

**Files:**
- Create: `fe/src/components/ChatTranscript.tsx`
- Test: `fe/src/components/ChatTranscript.test.tsx`

**Interfaces:**
- Consumes: `DiscussionMessage` (Task 10), `Button`/`Card`/`Textarea` from `fe/src/components/ui`.
- Produces: `ChatTranscript` with props `{ question: string; transcript: DiscussionMessage[]; sending: boolean; canFinish: boolean; onSend: (text: string) => void; onFinish: () => void }`. Used by Task 14.

- [ ] **Step 1: Write the failing tests**

Create `fe/src/components/ChatTranscript.test.tsx`:

```tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import ChatTranscript from './ChatTranscript'
import type { DiscussionMessage } from '@/lib/api'

const transcript: DiscussionMessage[] = [
  { role: 'user', text: 'I think companies.' },
  { role: 'ai', text: 'Why do you think so?' },
]

function renderChat(overrides = {}) {
  const props = {
    question: 'Who is responsible?',
    transcript,
    sending: false,
    canFinish: false,
    onSend: vi.fn(),
    onFinish: vi.fn(),
    ...overrides,
  }
  render(<ChatTranscript {...props} />)
  return props
}

describe('ChatTranscript', () => {
  it('shows the question and all messages', () => {
    renderChat()
    expect(screen.getByText('Who is responsible?')).toBeInTheDocument()
    expect(screen.getByText('I think companies.')).toBeInTheDocument()
    expect(screen.getByText('Why do you think so?')).toBeInTheDocument()
  })

  it('sends the trimmed draft and clears the input', () => {
    const props = renderChat()
    const input = screen.getByLabelText('Your answer')
    fireEvent.change(input, { target: { value: '  Because they pollute.  ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Send' }))
    expect(props.onSend).toHaveBeenCalledWith('Because they pollute.')
    expect(input).toHaveValue('')
  })

  it('disables Send while sending or when the draft is blank', () => {
    renderChat({ sending: true })
    expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
  })

  it('does not send a blank draft', () => {
    const props = renderChat()
    fireEvent.change(screen.getByLabelText('Your answer'), { target: { value: '   ' } })
    expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
    expect(props.onSend).not.toHaveBeenCalled()
  })

  it('hides the finish button until canFinish', () => {
    renderChat()
    expect(screen.queryByRole('button', { name: 'Finish conversation' })).not.toBeInTheDocument()
  })

  it('calls onFinish when the finish button is clicked', () => {
    const props = renderChat({ canFinish: true })
    fireEvent.click(screen.getByRole('button', { name: 'Finish conversation' }))
    expect(props.onFinish).toHaveBeenCalledTimes(1)
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd fe && npx vitest run src/components/ChatTranscript.test.tsx`
Expected: FAIL — module missing.

- [ ] **Step 3: Write the component**

Create `fe/src/components/ChatTranscript.tsx`:

```tsx
'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'
import type { DiscussionMessage } from '@/lib/api'

interface Props {
  question: string
  transcript: DiscussionMessage[]
  sending: boolean
  canFinish: boolean
  onSend: (text: string) => void
  onFinish: () => void
}

export default function ChatTranscript({
  question,
  transcript,
  sending,
  canFinish,
  onSend,
  onFinish,
}: Props) {
  const [draft, setDraft] = useState('')

  const submit = () => {
    const text = draft.trim()
    if (!text || sending) return
    setDraft('')
    onSend(text)
  }

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">{question}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="space-y-2">
          {transcript.map((message, i) => (
            <div key={i} className={message.role === 'user' ? 'text-right' : 'text-left'}>
              <span
                className={
                  message.role === 'user'
                    ? 'inline-block rounded-lg bg-indigo-600 px-3 py-2 text-sm text-white'
                    : 'inline-block rounded-lg border border-border bg-muted px-3 py-2 text-sm text-foreground'
                }
              >
                {message.text}
              </span>
            </div>
          ))}
          {sending && <p className="text-sm text-muted-foreground">Thinking…</p>}
        </div>
        <Textarea
          aria-label="Your answer"
          value={draft}
          onChange={e => setDraft(e.target.value)}
          placeholder="Answer in English"
        />
        <div className="flex gap-2">
          <Button onClick={submit} disabled={sending || draft.trim() === ''} className="flex-1">
            Send
          </Button>
          {canFinish && (
            <Button variant="outline" onClick={onFinish} disabled={sending}>
              Finish conversation
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
```

Before writing, glance at `fe/src/components/ui/textarea.tsx` and `button.tsx` for their exact prop conventions and mirror them.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd fe && npx vitest run src/components/ChatTranscript.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add fe/src/components/ChatTranscript.tsx fe/src/components/ChatTranscript.test.tsx
git commit -m "Add ChatTranscript component for the discussion conversation

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 12: ReflectionPrompt + GapAndExpressions components

**Files:**
- Create: `fe/src/components/ReflectionPrompt.tsx`, `fe/src/components/GapAndExpressions.tsx`
- Test: `fe/src/components/ReflectionPrompt.test.tsx`, `fe/src/components/GapAndExpressions.test.tsx`

**Interfaces:**
- Consumes: `GapAnalysis`, `Expression` (Task 10), ui components.
- Produces: `ReflectionPrompt` props `{ loading: boolean; onSubmit: (text: string) => void; onSkip: () => void }`; `GapAndExpressions` props `{ analysis: GapAnalysis; onContinue: () => void }`. Used by Task 14. The exact reflection question string is `日本語で答えるなら、他に言いたかったことはありますか？` — the e2e test (Task 16) asserts it verbatim.

- [ ] **Step 1: Write the failing tests**

Create `fe/src/components/ReflectionPrompt.test.tsx`:

```tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import ReflectionPrompt from './ReflectionPrompt'

function renderPrompt(overrides = {}) {
  const props = { loading: false, onSubmit: vi.fn(), onSkip: vi.fn(), ...overrides }
  render(<ReflectionPrompt {...props} />)
  return props
}

describe('ReflectionPrompt', () => {
  it('shows the Japanese reflection question', () => {
    renderPrompt()
    expect(screen.getByText('日本語で答えるなら、他に言いたかったことはありますか？')).toBeInTheDocument()
  })

  it('submits the trimmed reflection', () => {
    const props = renderPrompt()
    fireEvent.change(screen.getByLabelText('Japanese reflection'), {
      target: { value: ' 制度を変えるべき。 ' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Submit' }))
    expect(props.onSubmit).toHaveBeenCalledWith('制度を変えるべき。')
  })

  it('disables Submit when blank or loading', () => {
    renderPrompt()
    expect(screen.getByRole('button', { name: 'Submit' })).toBeDisabled()
  })

  it('skips without analyzing', () => {
    const props = renderPrompt()
    fireEvent.click(screen.getByRole('button', { name: 'Nothing to add — skip' }))
    expect(props.onSkip).toHaveBeenCalledTimes(1)
    expect(props.onSubmit).not.toHaveBeenCalled()
  })
})
```

Create `fe/src/components/GapAndExpressions.test.tsx`:

```tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import GapAndExpressions from './GapAndExpressions'
import type { GapAnalysis } from '@/lib/api'

const analysis: GapAnalysis = {
  expressed_ideas: ['Companies are responsible.'],
  missing_ideas: ['Systemic change is needed.'],
  expressions: [
    {
      phrase: 'take responsibility for',
      meaning_ja: '〜に責任を持つ',
      example_en: 'Companies should take responsibility for pollution.',
    },
  ],
}

describe('GapAndExpressions', () => {
  it('shows expressed and missing ideas', () => {
    render(<GapAndExpressions analysis={analysis} onContinue={vi.fn()} />)
    expect(screen.getByText('Companies are responsible.')).toBeInTheDocument()
    expect(screen.getByText('Systemic change is needed.')).toBeInTheDocument()
  })

  it('shows each expression with meaning and example', () => {
    render(<GapAndExpressions analysis={analysis} onContinue={vi.fn()} />)
    expect(screen.getByText('take responsibility for')).toBeInTheDocument()
    expect(screen.getByText('〜に責任を持つ')).toBeInTheDocument()
    expect(screen.getByText('Companies should take responsibility for pollution.')).toBeInTheDocument()
  })

  it('continues to the retry', () => {
    const onContinue = vi.fn()
    render(<GapAndExpressions analysis={analysis} onContinue={onContinue} />)
    fireEvent.click(screen.getByRole('button', { name: 'Try the question again' }))
    expect(onContinue).toHaveBeenCalledTimes(1)
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd fe && npx vitest run src/components/ReflectionPrompt.test.tsx src/components/GapAndExpressions.test.tsx`
Expected: FAIL — modules missing.

- [ ] **Step 3: Write the components**

Create `fe/src/components/ReflectionPrompt.tsx`:

```tsx
'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'

interface Props {
  loading: boolean
  onSubmit: (text: string) => void
  onSkip: () => void
}

export default function ReflectionPrompt({ loading, onSubmit, onSkip }: Props) {
  const [draft, setDraft] = useState('')

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">
          日本語で答えるなら、他に言いたかったことはありますか？
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">
          Write freely in Japanese — this is used to find ideas you could not yet express in
          English.
        </p>
        <Textarea
          aria-label="Japanese reflection"
          value={draft}
          onChange={e => setDraft(e.target.value)}
          placeholder="日本語で自由に書いてください"
        />
        <div className="flex gap-2">
          <Button
            onClick={() => onSubmit(draft.trim())}
            disabled={loading || draft.trim() === ''}
            className="flex-1"
          >
            {loading ? 'Analyzing…' : 'Submit'}
          </Button>
          <Button variant="outline" onClick={onSkip} disabled={loading}>
            Nothing to add — skip
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
```

Create `fe/src/components/GapAndExpressions.tsx`:

```tsx
'use client'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { GapAnalysis } from '@/lib/api'

interface Props {
  analysis: GapAnalysis
  onContinue: () => void
}

export default function GapAndExpressions({ analysis, onContinue }: Props) {
  return (
    <div className="space-y-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">What you expressed in English</CardTitle>
        </CardHeader>
        <CardContent>
          <ul className="list-disc pl-5 space-y-1 text-sm text-foreground">
            {analysis.expressed_ideas.map((idea, i) => (
              <li key={i}>{idea}</li>
            ))}
          </ul>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Ideas that stayed in Japanese</CardTitle>
        </CardHeader>
        <CardContent>
          <ul className="list-disc pl-5 space-y-1 text-sm text-foreground">
            {analysis.missing_ideas.map((idea, i) => (
              <li key={i}>{idea}</li>
            ))}
          </ul>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Expressions to close the gap</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {analysis.expressions.map(expression => (
            <div key={expression.phrase} className="rounded-md border border-border p-3">
              <p className="font-semibold text-foreground">{expression.phrase}</p>
              <p className="text-sm text-muted-foreground">{expression.meaning_ja}</p>
              <p className="text-sm italic text-foreground">{expression.example_en}</p>
            </div>
          ))}
          <Button onClick={onContinue} className="w-full">
            Try the question again
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd fe && npx vitest run src/components/ReflectionPrompt.test.tsx src/components/GapAndExpressions.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add fe/src/components/ReflectionPrompt.tsx fe/src/components/ReflectionPrompt.test.tsx fe/src/components/GapAndExpressions.tsx fe/src/components/GapAndExpressions.test.tsx
git commit -m "Add reflection and gap-study components

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 13: RetryForm + ComparisonView components

**Files:**
- Create: `fe/src/components/RetryForm.tsx`, `fe/src/components/ComparisonView.tsx`
- Test: `fe/src/components/RetryForm.test.tsx`, `fe/src/components/ComparisonView.test.tsx`

**Interfaces:**
- Consumes: `Expression` (Task 10), ui components, `next/link`.
- Produces: `RetryForm` props `{ question: string; expressions: Expression[]; loading: boolean; onSubmit: (text: string) => void }`; `ComparisonView` props `{ before: string; after: string; expressions: Expression[]; feedback: string; onRestart: () => void }`. Used by Task 14.

- [ ] **Step 1: Write the failing tests**

Create `fe/src/components/RetryForm.test.tsx`:

```tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import RetryForm from './RetryForm'
import type { Expression } from '@/lib/api'

const expressions: Expression[] = [
  { phrase: 'take responsibility for', meaning_ja: '〜に責任を持つ', example_en: 'x' },
]

function renderForm(overrides = {}) {
  const props = {
    question: 'Who is responsible?',
    expressions,
    loading: false,
    onSubmit: vi.fn(),
    ...overrides,
  }
  render(<RetryForm {...props} />)
  return props
}

describe('RetryForm', () => {
  it('shows the original question again and the expression chips', () => {
    renderForm()
    expect(screen.getByText('Who is responsible?')).toBeInTheDocument()
    expect(screen.getByText('take responsibility for')).toBeInTheDocument()
  })

  it('submits the trimmed answer', () => {
    const props = renderForm()
    fireEvent.change(screen.getByLabelText('Your improved answer'), {
      target: { value: ' Companies should take responsibility. ' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Submit answer' }))
    expect(props.onSubmit).toHaveBeenCalledWith('Companies should take responsibility.')
  })

  it('disables Submit when blank or loading', () => {
    renderForm()
    expect(screen.getByRole('button', { name: 'Submit answer' })).toBeDisabled()
  })

  it('renders no chips section without expressions', () => {
    renderForm({ expressions: [] })
    expect(screen.queryByText('Try to use:')).not.toBeInTheDocument()
  })
})
```

Create `fe/src/components/ComparisonView.test.tsx`:

```tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import ComparisonView from './ComparisonView'
import type { Expression } from '@/lib/api'

const expressions: Expression[] = [
  { phrase: 'take responsibility for', meaning_ja: '〜に責任を持つ', example_en: 'x' },
]

function renderView(overrides = {}) {
  const props = {
    before: 'I think companies.',
    after: 'Companies should take responsibility for their impact.',
    expressions,
    feedback: 'You used the new expression!',
    onRestart: vi.fn(),
    ...overrides,
  }
  render(<ComparisonView {...props} />)
  return props
}

describe('ComparisonView', () => {
  it('shows before and after answers with the feedback', () => {
    renderView()
    expect(screen.getByText('I think companies.')).toBeInTheDocument()
    expect(screen.getByText('Companies should take responsibility for their impact.')).toBeInTheDocument()
    expect(screen.getByText('You used the new expression!')).toBeInTheDocument()
  })

  it('lists the learned expressions', () => {
    renderView()
    expect(screen.getByText('take responsibility for')).toBeInTheDocument()
  })

  it('links to the history page', () => {
    renderView()
    expect(screen.getByRole('link', { name: 'View history' })).toHaveAttribute(
      'href',
      '/discussion/history'
    )
  })

  it('starts a new discussion', () => {
    const props = renderView()
    fireEvent.click(screen.getByRole('button', { name: 'New discussion' }))
    expect(props.onRestart).toHaveBeenCalledTimes(1)
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd fe && npx vitest run src/components/RetryForm.test.tsx src/components/ComparisonView.test.tsx`
Expected: FAIL — modules missing.

- [ ] **Step 3: Write the components**

Create `fe/src/components/RetryForm.tsx`:

```tsx
'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'
import type { Expression } from '@/lib/api'

interface Props {
  question: string
  expressions: Expression[]
  loading: boolean
  onSubmit: (text: string) => void
}

export default function RetryForm({ question, expressions, loading, onSubmit }: Props) {
  const [draft, setDraft] = useState('')

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">Let&apos;s try the original question again</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-foreground">{question}</p>
        {expressions.length > 0 && (
          <div className="space-y-1">
            <p className="text-xs text-muted-foreground">Try to use:</p>
            <div className="flex flex-wrap gap-1.5">
              {expressions.map(expression => (
                <span
                  key={expression.phrase}
                  className="rounded-md border border-border bg-muted px-2 py-1 text-xs text-foreground"
                >
                  {expression.phrase}
                </span>
              ))}
            </div>
          </div>
        )}
        <Textarea
          aria-label="Your improved answer"
          value={draft}
          onChange={e => setDraft(e.target.value)}
          placeholder="Answer in English"
        />
        <Button
          onClick={() => onSubmit(draft.trim())}
          disabled={loading || draft.trim() === ''}
          className="w-full"
        >
          {loading ? 'Saving…' : 'Submit answer'}
        </Button>
      </CardContent>
    </Card>
  )
}
```

Create `fe/src/components/ComparisonView.tsx`:

```tsx
'use client'

import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { Expression } from '@/lib/api'

interface Props {
  before: string
  after: string
  expressions: Expression[]
  feedback: string
  onRestart: () => void
}

export default function ComparisonView({ before, after, expressions, feedback, onRestart }: Props) {
  return (
    <div className="space-y-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Before</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">{before}</p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">After</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-foreground">{after}</p>
        </CardContent>
      </Card>

      {expressions.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">Expressions learned</CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="list-disc pl-5 space-y-1 text-sm text-foreground">
              {expressions.map(expression => (
                <li key={expression.phrase}>{expression.phrase}</li>
              ))}
            </ul>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardContent className="pt-6 space-y-3">
          <p className="text-sm text-foreground">{feedback}</p>
          <div className="flex gap-2">
            <Button onClick={onRestart} className="flex-1">
              New discussion
            </Button>
            <Link
              href="/discussion/history"
              className="flex-1 rounded-md border border-border px-3 py-2 text-center text-sm text-foreground hover:bg-accent"
            >
              View history
            </Link>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd fe && npx vitest run src/components/RetryForm.test.tsx src/components/ComparisonView.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add fe/src/components/RetryForm.tsx fe/src/components/RetryForm.test.tsx fe/src/components/ComparisonView.tsx fe/src/components/ComparisonView.test.tsx
git commit -m "Add retry and before/after comparison components

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 14: DiscussionSession orchestrator, pages, and header link

**Files:**
- Create: `fe/src/components/DiscussionSession.tsx`, `fe/src/app/discussion/page.tsx`
- Modify: `fe/src/components/AppHeader.tsx`
- Test: `fe/src/components/DiscussionSession.test.tsx`, `fe/src/components/AppHeader.test.tsx` (append)

**Interfaces:**
- Consumes: all Task 10–13 components/types, `AuthGate`, `AppHeader`, `SettingsSheet`, `useSettings` (existing).
- Produces: `DiscussionSession` (props `{ user: User }`) owning the phase state machine `loading → conversation → reflection → studying → retry → comparison`; route `/discussion`; `AppHeader` prop `showDiscussionLink?: boolean` (default true) rendering a `Discussion` link to `/discussion`.

- [ ] **Step 1: Write the failing tests**

Create `fe/src/components/DiscussionSession.test.tsx`:

```tsx
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { User } from 'firebase/auth'

vi.mock('@/lib/api', () => ({
  api: {
    getDiscussionQuestion: vi.fn(),
    discussionReply: vi.fn(),
    discussionAnalyze: vi.fn(),
    discussionComplete: vi.fn(),
  },
}))
vi.mock('@/lib/firebase', () => ({ auth: { currentUser: null } }))

import { api } from '@/lib/api'
import DiscussionSession from './DiscussionSession'

const user = { displayName: 'Tester', email: 't@example.com', photoURL: null } as unknown as User

const question = {
  id: 16,
  question_en: 'Who should take more responsibility for environmental problems?',
  topic: 'environment',
  level: 3,
  target_skills: ['giving opinions'],
}

const analysis = {
  expressed_ideas: ['Companies are responsible.'],
  missing_ideas: ['Systemic change is needed.'],
  expressions: [
    { phrase: 'take responsibility for', meaning_ja: '〜に責任を持つ', example_en: 'x' },
  ],
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(api.getDiscussionQuestion).mockResolvedValue(question)
})

async function startSession() {
  render(<DiscussionSession user={user} />)
  await waitFor(() =>
    expect(screen.getByText(question.question_en)).toBeInTheDocument()
  )
}

async function answerOnce(text: string) {
  fireEvent.change(screen.getByLabelText('Your answer'), { target: { value: text } })
  fireEvent.click(screen.getByRole('button', { name: 'Send' }))
}

describe('DiscussionSession', () => {
  it('loads a question and shows the conversation phase', async () => {
    await startSession()
    expect(screen.getByLabelText('Your answer')).toBeInTheDocument()
  })

  it('appends the AI follow-up after sending an answer', async () => {
    vi.mocked(api.discussionReply).mockResolvedValue({ done: false, message: 'Why do you think so?' })
    await startSession()
    await answerOnce('I think companies.')
    await waitFor(() => expect(screen.getByText('Why do you think so?')).toBeInTheDocument())
    expect(api.discussionReply).toHaveBeenCalledWith(16, [{ role: 'user', text: 'I think companies.' }])
  })

  it('moves to reflection when the coach says done', async () => {
    vi.mocked(api.discussionReply).mockResolvedValue({ done: true, message: 'Thanks for sharing!' })
    await startSession()
    await answerOnce('I think companies.')
    await waitFor(() =>
      expect(screen.getByText('日本語で答えるなら、他に言いたかったことはありますか？')).toBeInTheDocument()
    )
    expect(screen.getByText('Thanks for sharing!')).toBeInTheDocument()
  })

  it('analyzes the reflection and shows the study phase', async () => {
    vi.mocked(api.discussionReply).mockResolvedValue({ done: true, message: 'Thanks!' })
    vi.mocked(api.discussionAnalyze).mockResolvedValue(analysis)
    await startSession()
    await answerOnce('I think companies.')
    await waitFor(() => expect(screen.getByLabelText('Japanese reflection')).toBeInTheDocument())
    fireEvent.change(screen.getByLabelText('Japanese reflection'), {
      target: { value: '制度を変えるべき。' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Submit' }))
    await waitFor(() => expect(screen.getByText('take responsibility for')).toBeInTheDocument())
  })

  it('skipping reflection goes straight to retry with no analyze call', async () => {
    vi.mocked(api.discussionReply).mockResolvedValue({ done: true, message: 'Thanks!' })
    await startSession()
    await answerOnce('I think companies.')
    await waitFor(() => expect(screen.getByLabelText('Japanese reflection')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: 'Nothing to add — skip' }))
    expect(await screen.findByLabelText('Your improved answer')).toBeInTheDocument()
    expect(api.discussionAnalyze).not.toHaveBeenCalled()
  })

  it('completes the session and shows the comparison', async () => {
    vi.mocked(api.discussionReply).mockResolvedValue({ done: true, message: 'Thanks!' })
    vi.mocked(api.discussionAnalyze).mockResolvedValue(analysis)
    vi.mocked(api.discussionComplete).mockResolvedValue({
      session_id: 's1',
      retry_feedback: 'You used the new expression!',
    })
    await startSession()
    await answerOnce('I think companies.')
    await waitFor(() => expect(screen.getByLabelText('Japanese reflection')).toBeInTheDocument())
    fireEvent.change(screen.getByLabelText('Japanese reflection'), { target: { value: 'あ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Submit' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Try the question again' })).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: 'Try the question again' }))
    fireEvent.change(screen.getByLabelText('Your improved answer'), {
      target: { value: 'Companies should take responsibility.' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Submit answer' }))
    await waitFor(() => expect(screen.getByText('You used the new expression!')).toBeInTheDocument())
    expect(api.discussionComplete).toHaveBeenCalledWith(
      expect.objectContaining({
        question_id: 16,
        retry_answer: 'Companies should take responsibility.',
        expressions: analysis.expressions,
      })
    )
    expect(screen.getByText('I think companies.')).toBeInTheDocument()
  })

  it('shows an error with retry when the reply call fails', async () => {
    vi.mocked(api.discussionReply).mockRejectedValueOnce(new Error('API error: 500'))
    await startSession()
    await answerOnce('I think companies.')
    expect(await screen.findByText('Something went wrong. Please try again.')).toBeInTheDocument()
    vi.mocked(api.discussionReply).mockResolvedValue({ done: false, message: 'Recovered follow-up?' })
    fireEvent.click(screen.getByRole('button', { name: 'Try Again' }))
    await waitFor(() => expect(screen.getByText('Recovered follow-up?')).toBeInTheDocument())
  })
})
```

Append to `fe/src/components/AppHeader.test.tsx` (inside the existing describe, following its render helper conventions — read the file first and adapt):

```tsx
  it('shows the discussion link by default', () => {
    renderHeader()
    expect(screen.getByRole('link', { name: 'Discussion' })).toHaveAttribute('href', '/discussion')
  })

  it('hides the discussion link when showDiscussionLink is false', () => {
    renderHeader({ showDiscussionLink: false })
    expect(screen.queryByRole('link', { name: 'Discussion' })).not.toBeInTheDocument()
  })
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd fe && npx vitest run src/components/DiscussionSession.test.tsx src/components/AppHeader.test.tsx`
Expected: FAIL.

- [ ] **Step 3: Write the implementation**

In `fe/src/components/AppHeader.tsx`, add the prop and link (next to the Mistakes link):

```tsx
interface AppHeaderProps {
  user: User
  onOpenSettings: () => void
  showMistakesLink?: boolean
  showDiscussionLink?: boolean
}
```

```tsx
        {showDiscussionLink && (
          <Link
            href="/discussion"
            className="rounded-md px-2 py-1 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground"
          >
            Discussion
          </Link>
        )}
```

with `showDiscussionLink = true` added to the destructured defaults.

Create `fe/src/components/DiscussionSession.tsx`:

```tsx
'use client'

import { useEffect, useState } from 'react'
import type { User } from 'firebase/auth'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import AppHeader from './AppHeader'
import SettingsSheet from './SettingsSheet'
import ChatTranscript from './ChatTranscript'
import ReflectionPrompt from './ReflectionPrompt'
import GapAndExpressions from './GapAndExpressions'
import RetryForm from './RetryForm'
import ComparisonView from './ComparisonView'
import { useSettings } from '@/lib/useSettings'
import {
  api,
  type DiscussionQuestion,
  type DiscussionMessage,
  type GapAnalysis,
  type DiscussionCompleteResponse,
} from '@/lib/api'

type Phase = 'loading' | 'conversation' | 'reflection' | 'studying' | 'retry' | 'comparison'

interface Props {
  user: User
}

export default function DiscussionSession({ user }: Props) {
  const { levels, language, setLevels, setLanguage } = useSettings()
  const [settingsOpen, setSettingsOpen] = useState(false)

  const [phase, setPhase] = useState<Phase>('loading')
  const [question, setQuestion] = useState<DiscussionQuestion | null>(null)
  const [transcript, setTranscript] = useState<DiscussionMessage[]>([])
  const [analysis, setAnalysis] = useState<GapAnalysis | null>(null)
  const [reflectionJa, setReflectionJa] = useState('')
  const [result, setResult] = useState<DiscussionCompleteResponse | null>(null)
  const [retryAnswer, setRetryAnswer] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const loadQuestion = async () => {
    setPhase('loading')
    setError(null)
    try {
      const q = await api.getDiscussionQuestion()
      setQuestion(q)
      setPhase('conversation')
    } catch {
      setError('Failed to load a question.')
    }
  }

  useEffect(() => {
    loadQuestion()
  }, [])

  const requestReply = async (current: DiscussionMessage[]) => {
    if (!question) return
    setBusy(true)
    setError(null)
    try {
      const reply = await api.discussionReply(question.id, current)
      if (reply.done) {
        if (reply.message) {
          setTranscript([...current, { role: 'ai', text: reply.message }])
        }
        setPhase('reflection')
      } else {
        setTranscript([...current, { role: 'ai', text: reply.message }])
      }
    } catch {
      setError('Something went wrong. Please try again.')
    } finally {
      setBusy(false)
    }
  }

  const sendMessage = (text: string) => {
    const next: DiscussionMessage[] = [...transcript, { role: 'user', text }]
    setTranscript(next)
    requestReply(next)
  }

  const submitReflection = async (text: string) => {
    if (!question) return
    setBusy(true)
    setError(null)
    try {
      const gap = await api.discussionAnalyze(question.id, transcript, text)
      setReflectionJa(text)
      setAnalysis(gap)
      setPhase('studying')
    } catch {
      setError('Something went wrong. Please try again.')
    } finally {
      setBusy(false)
    }
  }

  const skipReflection = () => {
    setReflectionJa('')
    setAnalysis(null)
    setPhase('retry')
  }

  const submitRetry = async (text: string) => {
    if (!question) return
    setBusy(true)
    setError(null)
    setRetryAnswer(text)
    try {
      const res = await api.discussionComplete({
        question_id: question.id,
        transcript,
        reflection_ja: reflectionJa,
        expressed_ideas: analysis?.expressed_ideas ?? [],
        missing_ideas: analysis?.missing_ideas ?? [],
        expressions: analysis?.expressions ?? [],
        retry_answer: text,
      })
      setResult(res)
      setPhase('comparison')
    } catch {
      setError('Something went wrong. Please try again.')
    } finally {
      setBusy(false)
    }
  }

  const restart = () => {
    setQuestion(null)
    setTranscript([])
    setAnalysis(null)
    setReflectionJa('')
    setResult(null)
    setRetryAnswer('')
    loadQuestion()
  }

  // The conversation error is the only one whose input (the sent message) is
  // already consumed — recover by re-requesting a reply for the transcript
  // as it stands. Reflection/retry keep their drafts, so plain resubmission
  // covers those.
  const canRetryReply =
    phase === 'conversation' &&
    transcript.length > 0 &&
    transcript[transcript.length - 1].role === 'user'

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 p-4">
      <div className="max-w-2xl mx-auto">
        <AppHeader
          user={user}
          onOpenSettings={() => setSettingsOpen(true)}
          showDiscussionLink={false}
        />
        <h2 className="mb-4 text-lg font-bold text-foreground">Discussion</h2>

        {error && (
          <Card className="mb-3">
            <CardContent className="pt-6 space-y-2">
              <p className="text-sm text-destructive">{error}</p>
              {phase === 'loading' && (
                <Button variant="outline" size="sm" onClick={loadQuestion}>
                  Try Again
                </Button>
              )}
              {canRetryReply && (
                <Button variant="outline" size="sm" onClick={() => requestReply(transcript)}>
                  Try Again
                </Button>
              )}
            </CardContent>
          </Card>
        )}

        {phase === 'loading' && !error && (
          <div className="text-center">
            <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mx-auto mb-4"></div>
            <p className="text-muted-foreground">Loading...</p>
          </div>
        )}

        {phase === 'conversation' && question && (
          <ChatTranscript
            question={question.question_en}
            transcript={transcript}
            sending={busy}
            canFinish={transcript.length >= 3}
            onSend={sendMessage}
            onFinish={() => setPhase('reflection')}
          />
        )}

        {phase === 'reflection' && (
          <div className="space-y-3">
            {transcript.length > 0 && transcript[transcript.length - 1].role === 'ai' && (
              <Card>
                <CardContent className="pt-6">
                  <p className="text-sm text-foreground">
                    {transcript[transcript.length - 1].text}
                  </p>
                </CardContent>
              </Card>
            )}
            <ReflectionPrompt loading={busy} onSubmit={submitReflection} onSkip={skipReflection} />
          </div>
        )}

        {phase === 'studying' && analysis && (
          <GapAndExpressions analysis={analysis} onContinue={() => setPhase('retry')} />
        )}

        {phase === 'retry' && question && (
          <RetryForm
            question={question.question_en}
            expressions={analysis?.expressions ?? []}
            loading={busy}
            onSubmit={submitRetry}
          />
        )}

        {phase === 'comparison' && result && (
          <ComparisonView
            before={transcript[0]?.text ?? ''}
            after={retryAnswer}
            expressions={analysis?.expressions ?? []}
            feedback={result.retry_feedback}
            onRestart={restart}
          />
        )}
      </div>

      <SettingsSheet
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        levels={levels}
        onLevelsChange={setLevels}
        language={language}
        onLanguageChange={setLanguage}
      />
    </div>
  )
}
```

Create `fe/src/app/discussion/page.tsx`:

```tsx
'use client'

import AuthGate from '@/components/AuthGate'
import DiscussionSession from '@/components/DiscussionSession'

export default function Page() {
  return <AuthGate>{user => <DiscussionSession user={user} />}</AuthGate>
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd fe && npx vitest run`
Expected: all PASS (including untouched suites). Also `npx next build` to confirm the static export accepts the new route.

- [ ] **Step 5: Commit**

```bash
git add fe/src/components/DiscussionSession.tsx fe/src/components/DiscussionSession.test.tsx fe/src/components/AppHeader.tsx fe/src/components/AppHeader.test.tsx fe/src/app/discussion/page.tsx
git commit -m "Add discussion session page with phase state machine

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 15: SessionHistory + history page

**Files:**
- Create: `fe/src/components/SessionHistory.tsx`, `fe/src/app/discussion/history/page.tsx`
- Test: `fe/src/components/SessionHistory.test.tsx`

**Interfaces:**
- Consumes: `api.listDiscussionSessions`, `api.getDiscussionSession`, `DiscussionSessionSummary`, `DiscussionSessionDetail` (Task 10), `AppHeader`, `SettingsSheet`, `useSettings`.
- Produces: `SessionHistory` (props `{ user: User }`) and route `/discussion/history`. Detail expands inline — no dynamic route (static export).

- [ ] **Step 1: Write the failing tests**

Create `fe/src/components/SessionHistory.test.tsx`:

```tsx
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { User } from 'firebase/auth'

vi.mock('@/lib/api', () => ({
  api: {
    listDiscussionSessions: vi.fn(),
    getDiscussionSession: vi.fn(),
  },
}))
vi.mock('@/lib/firebase', () => ({ auth: { currentUser: null } }))

import { api } from '@/lib/api'
import SessionHistory from './SessionHistory'

const user = { displayName: 'Tester', email: 't@example.com', photoURL: null } as unknown as User

const summary = {
  id: 's1',
  question_en: 'Who is responsible?',
  topic: 'environment',
  created_at: '2026-08-23T10:00:00Z',
}

const detail = {
  id: 's1',
  question_id: 16,
  question_en: 'Who is responsible?',
  topic: 'environment',
  transcript: [
    { role: 'user' as const, text: 'I think companies.' },
    { role: 'ai' as const, text: 'Why?' },
  ],
  reflection_ja: '制度を変えるべき。',
  expressed_ideas: ['Companies are responsible.'],
  missing_ideas: ['Systemic change is needed.'],
  expressions: [
    { phrase: 'take responsibility for', meaning_ja: '〜に責任を持つ', example_en: 'x' },
  ],
  first_answer: 'I think companies.',
  retry_answer: 'Companies should take responsibility.',
  retry_feedback: 'Nice improvement!',
  created_at: '2026-08-23T10:00:00Z',
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('SessionHistory', () => {
  it('lists completed sessions', async () => {
    vi.mocked(api.listDiscussionSessions).mockResolvedValue({ sessions: [summary] })
    render(<SessionHistory user={user} />)
    expect(await screen.findByText('Who is responsible?')).toBeInTheDocument()
    expect(screen.getByText('environment')).toBeInTheDocument()
  })

  it('shows an empty state', async () => {
    vi.mocked(api.listDiscussionSessions).mockResolvedValue({ sessions: [] })
    render(<SessionHistory user={user} />)
    expect(await screen.findByText('No sessions yet — try a discussion!')).toBeInTheDocument()
  })

  it('expands a session inline with its details', async () => {
    vi.mocked(api.listDiscussionSessions).mockResolvedValue({ sessions: [summary] })
    vi.mocked(api.getDiscussionSession).mockResolvedValue(detail)
    render(<SessionHistory user={user} />)
    fireEvent.click(await screen.findByRole('button', { name: 'Who is responsible?' }))
    await waitFor(() => expect(screen.getByText('Nice improvement!')).toBeInTheDocument())
    expect(api.getDiscussionSession).toHaveBeenCalledWith('s1')
    expect(screen.getByText('Companies should take responsibility.')).toBeInTheDocument()
    expect(screen.getByText('take responsibility for')).toBeInTheDocument()
  })

  it('shows an error with retry when loading fails', async () => {
    vi.mocked(api.listDiscussionSessions).mockRejectedValueOnce(new Error('API error: 500'))
    render(<SessionHistory user={user} />)
    expect(await screen.findByText('Failed to load sessions.')).toBeInTheDocument()
    vi.mocked(api.listDiscussionSessions).mockResolvedValue({ sessions: [summary] })
    fireEvent.click(screen.getByRole('button', { name: 'Try Again' }))
    expect(await screen.findByText('Who is responsible?')).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd fe && npx vitest run src/components/SessionHistory.test.tsx`
Expected: FAIL — module missing.

- [ ] **Step 3: Write the component and page**

Create `fe/src/components/SessionHistory.tsx`:

```tsx
'use client'

import { useEffect, useState } from 'react'
import type { User } from 'firebase/auth'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import AppHeader from './AppHeader'
import SettingsSheet from './SettingsSheet'
import { useSettings } from '@/lib/useSettings'
import { api, type DiscussionSessionSummary, type DiscussionSessionDetail } from '@/lib/api'

interface Props {
  user: User
}

export default function SessionHistory({ user }: Props) {
  const { levels, language, setLevels, setLanguage } = useSettings()
  const [settingsOpen, setSettingsOpen] = useState(false)

  const [sessions, setSessions] = useState<DiscussionSessionSummary[] | null>(null)
  const [details, setDetails] = useState<Record<string, DiscussionSessionDetail>>({})
  const [openId, setOpenId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const loadSessions = async () => {
    setError(null)
    try {
      const result = await api.listDiscussionSessions()
      setSessions(result.sessions)
    } catch {
      setError('Failed to load sessions.')
    }
  }

  useEffect(() => {
    loadSessions()
  }, [])

  const toggle = async (id: string) => {
    if (openId === id) {
      setOpenId(null)
      return
    }
    setOpenId(id)
    if (!details[id]) {
      try {
        const detail = await api.getDiscussionSession(id)
        setDetails(prev => ({ ...prev, [id]: detail }))
      } catch {
        setError('Failed to load the session.')
        setOpenId(null)
      }
    }
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 p-4">
      <div className="max-w-2xl mx-auto">
        <AppHeader user={user} onOpenSettings={() => setSettingsOpen(true)} />
        <h2 className="mb-4 text-lg font-bold text-foreground">Discussion History</h2>

        {error ? (
          <Card>
            <CardContent className="pt-6 space-y-2">
              <p className="text-foreground">{error}</p>
              <Button onClick={loadSessions} className="w-full">
                Try Again
              </Button>
            </CardContent>
          </Card>
        ) : sessions === null ? (
          <div className="text-center">
            <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mx-auto mb-4"></div>
            <p className="text-muted-foreground">Loading...</p>
          </div>
        ) : sessions.length === 0 ? (
          <Card>
            <CardContent className="pt-6 text-center text-muted-foreground">
              No sessions yet — try a discussion!
            </CardContent>
          </Card>
        ) : (
          <div className="space-y-3">
            {sessions.map(session => {
              const detail = openId === session.id ? details[session.id] : undefined
              return (
                <Card key={session.id}>
                  <CardContent className="pt-6 space-y-3">
                    <button
                      type="button"
                      onClick={() => toggle(session.id)}
                      className="w-full text-left"
                    >
                      <p className="font-semibold text-foreground">{session.question_en}</p>
                      <p className="text-xs text-muted-foreground">
                        <span>{session.topic}</span>
                        {' · '}
                        <span>{new Date(session.created_at).toLocaleDateString()}</span>
                      </p>
                    </button>
                    {detail && (
                      <div className="space-y-3 border-t border-border pt-3 text-sm">
                        <div>
                          <p className="text-xs font-semibold text-muted-foreground">Conversation</p>
                          {detail.transcript.map((message, i) => (
                            <p key={i} className="text-foreground">
                              <span className="text-muted-foreground">
                                {message.role === 'user' ? 'You: ' : 'AI: '}
                              </span>
                              {message.text}
                            </p>
                          ))}
                        </div>
                        {detail.reflection_ja && (
                          <div>
                            <p className="text-xs font-semibold text-muted-foreground">Reflection</p>
                            <p className="text-foreground">{detail.reflection_ja}</p>
                          </div>
                        )}
                        {detail.expressions.length > 0 && (
                          <div>
                            <p className="text-xs font-semibold text-muted-foreground">
                              Expressions learned
                            </p>
                            <div className="flex flex-wrap gap-1.5">
                              {detail.expressions.map(expression => (
                                <span
                                  key={expression.phrase}
                                  className="rounded-md border border-border bg-muted px-2 py-1 text-xs text-foreground"
                                >
                                  {expression.phrase}
                                </span>
                              ))}
                            </div>
                          </div>
                        )}
                        <div>
                          <p className="text-xs font-semibold text-muted-foreground">Before</p>
                          <p className="text-muted-foreground">{detail.first_answer}</p>
                          <p className="mt-1 text-xs font-semibold text-muted-foreground">After</p>
                          <p className="text-foreground">{detail.retry_answer}</p>
                        </div>
                        <div>
                          <p className="text-xs font-semibold text-muted-foreground">Feedback</p>
                          <p className="text-foreground">{detail.retry_feedback}</p>
                        </div>
                      </div>
                    )}
                  </CardContent>
                </Card>
              )
            })}
          </div>
        )}
      </div>

      <SettingsSheet
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        levels={levels}
        onLevelsChange={setLevels}
        language={language}
        onLanguageChange={setLanguage}
      />
    </div>
  )
}
```

Create `fe/src/app/discussion/history/page.tsx`:

```tsx
'use client'

import AuthGate from '@/components/AuthGate'
import SessionHistory from '@/components/SessionHistory'

export default function Page() {
  return <AuthGate>{user => <SessionHistory user={user} />}</AuthGate>
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd fe && npx vitest run` then `npx next build`
Expected: all PASS, clean build.

- [ ] **Step 5: Commit**

```bash
git add fe/src/components/SessionHistory.tsx fe/src/components/SessionHistory.test.tsx fe/src/app/discussion/history/page.tsx
git commit -m "Add discussion session history page

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 16: End-to-end tests

**Files:**
- Create: `e2e/fixtures/discussion_questions.ndjson`, `e2e/tests/discussion.spec.ts`
- Modify: `e2e/scripts/inner.sh`

**Interfaces:**
- Consumes: `signInAndGetSentence` helper (existing `e2e/tests/helpers.ts`), stubCoach literals (Task 8), seedquestions command (Task 9), UI strings/labels from Tasks 11–15.

- [ ] **Step 1: Add the fixture**

Create `e2e/fixtures/discussion_questions.ndjson`:

```
{"id": 1, "topic": "environment", "level": 3, "question_en": "Who should take more responsibility for environmental problems: individuals, companies, or governments?", "target_skills": ["giving opinions", "giving reasons"], "is_active": 1}
```

- [ ] **Step 2: Seed it in inner.sh**

In `e2e/scripts/inner.sh`, after the existing "Seeding fixture sentences..." block, add:

```bash
echo "Seeding fixture discussion questions..."
(cd "$REPO_ROOT/api" && GOOGLE_CLOUD_PROJECT=eagle-test go run ./cmd/seedquestions -file "$E2E_DIR/fixtures/discussion_questions.ndjson")
```

- [ ] **Step 3: Write the spec**

Create `e2e/tests/discussion.spec.ts`:

```ts
import { test, expect } from '@playwright/test'
import { signInAndGetSentence } from './helpers'

const QUESTION =
  'Who should take more responsibility for environmental problems: individuals, companies, or governments?'

test('completes a discussion session end to end', async ({ page }) => {
  await signInAndGetSentence(page)

  await page.getByRole('link', { name: 'Discussion' }).click()
  await expect(page.getByText(QUESTION)).toBeVisible()

  // Initial answer + two stub follow-ups, then the stub closes the conversation.
  await page.getByLabel('Your answer').fill('I think companies.')
  await page.getByRole('button', { name: 'Send' }).click()
  await expect(page.getByText('Stub follow-up 1: can you tell me more?')).toBeVisible()

  await page.getByLabel('Your answer').fill('Because companies affect environment more.')
  await page.getByRole('button', { name: 'Send' }).click()
  await expect(page.getByText('Stub follow-up 2: can you tell me more?')).toBeVisible()

  await page.getByLabel('Your answer').fill('They can change their products.')
  await page.getByRole('button', { name: 'Send' }).click()

  // Reflection phase (stub closing line is shown above the prompt).
  await expect(page.getByText('日本語で答えるなら、他に言いたかったことはありますか？')).toBeVisible()
  await expect(page.getByText('Great, thanks for sharing your thoughts!')).toBeVisible()
  await page.getByLabel('Japanese reflection').fill('制度そのものを変える必要があると思う。')
  await page.getByRole('button', { name: 'Submit' }).click()

  // Study phase shows the stub analysis.
  await expect(page.getByText('Systemic change is more effective than individual action.')).toBeVisible()
  await expect(page.getByText('take responsibility for').first()).toBeVisible()
  await page.getByRole('button', { name: 'Try the question again' }).click()

  // Retry and comparison.
  await page
    .getByLabel('Your improved answer')
    .fill('Companies should take responsibility for their impact and make systemic changes.')
  await page.getByRole('button', { name: 'Submit answer' }).click()
  await expect(page.getByText('This is a stub retry feedback for e2e tests.')).toBeVisible()
  await expect(page.getByText('I think companies.')).toBeVisible()

  // History shows the saved session.
  await page.getByRole('link', { name: 'View history' }).click()
  await expect(page.getByRole('heading', { name: 'Discussion History' })).toBeVisible()
  await expect(page.getByText(QUESTION)).toBeVisible()
})

test('reflection can be skipped', async ({ page }) => {
  await signInAndGetSentence(page)

  await page.getByRole('link', { name: 'Discussion' }).click()
  await expect(page.getByText(QUESTION)).toBeVisible()

  await page.getByLabel('Your answer').fill('I think governments.')
  await page.getByRole('button', { name: 'Send' }).click()
  await expect(page.getByText('Stub follow-up 1: can you tell me more?')).toBeVisible()

  await page.getByLabel('Your answer').fill('Because they make the rules.')
  await page.getByRole('button', { name: 'Send' }).click()
  await expect(page.getByText('Stub follow-up 2: can you tell me more?')).toBeVisible()

  // Finish early instead of answering the second follow-up.
  await page.getByRole('button', { name: 'Finish conversation' }).click()
  await expect(page.getByText('日本語で答えるなら、他に言いたかったことはありますか？')).toBeVisible()
  await page.getByRole('button', { name: 'Nothing to add — skip' }).click()

  await page.getByLabel('Your improved answer').fill('I still think governments, because they set the rules.')
  await page.getByRole('button', { name: 'Submit answer' }).click()
  await expect(page.getByText('This is a stub retry feedback for e2e tests.')).toBeVisible()
})
```

The heading assertion requires the `<h2>` in `SessionHistory` — `getByRole('heading', { name: 'Discussion History' })` matches the `h2` added in Task 15.

- [ ] **Step 4: Run the e2e suite**

Run: `bash e2e/scripts/run.sh` (from repo root)
Expected: all specs pass, including the pre-existing ones.

- [ ] **Step 5: Commit**

```bash
git add e2e/fixtures/discussion_questions.ndjson e2e/tests/discussion.spec.ts e2e/scripts/inner.sh
git commit -m "Add discussion practice e2e coverage

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Final verification (after all tasks)

- `cd api && go build ./... && go vet ./... && go test ./...`
- `firebase emulators:exec --project eagle-test --only firestore "cd api && go test ./internal/app -v"` (from repo root)
- `cd fe && npx vitest run && npx next build && npx eslint src`
- `bash e2e/scripts/run.sh`
- Manual smoke test against the local dev stack (emulators + e2eserver + `npm run dev`) if UI polish needs checking.
- Production seeding (owner action, after review of `docs/discussion_questions_seed.ndjson`): `GOOGLE_CLOUD_PROJECT=<prod-project> go run ./cmd/seedquestions -file ../docs/discussion_questions_seed.ndjson` from `api/`.
