# Weakness Insight Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an AI-generated weakness-insight card to the top of the mistakes page that analyzes the learner's recent mistakes, surfaces recurring weaknesses (ignoring one-off typos), and is auto-generated on page load.

**Architecture:** Mirror the existing Explain feature exactly — a single-purpose `WeaknessAnalyzer` interface with a pure, unit-testable `buildWeaknessPrompt`, and a `GeminiWeaknessAnalyzer` that reuses the Explain feature's genai client wiring (SDK, model constant, timeout, and `contentGenerator` test seam). A new auth-gated `GET /api/mistakes/insight` handler loads the learner's mistakes server-side via the existing `ListMistakes`, caps to the 50 most recent, and returns `{ insight: string }`. The frontend `Mistakes` component auto-calls it after the list loads (when non-empty) and renders the result above the list.

**Tech Stack:** Go 1.25 (`google.golang.org/genai`, `net/http`, standard `testing`); Next.js / React (TypeScript, Vitest + Testing Library).

## Global Constraints

- **TDD, red before green.** Every code change starts with a failing test; no test calls the real Gemini API.
- **Model:** reuse `geminiExplainModel` (`gemini-3.1-flash-lite`) and `explainTimeout` (20s) constants — do not introduce new ones.
- **Secret:** reuse `GEMINI_API_KEY` (already required at startup) — no new env var.
- **Languages:** reuse the existing `validExplainLanguages` allow-list (`en`, `ja`) as the single source of truth. Frontend reuses `localStorage['eagle:explainLanguage']` (default `en`).
- **Scope caps:** `maxInsightMistakes = 50`. Output is a single plain-text field `{ insight: string }`; no persistence, no streaming, no structured JSON.
- **Backend commands** run from `api/`: `go test ./...`, `go vet ./...`.
- **Frontend commands** run from `fe/`: `npm test` (Vitest, non-watch).
- **Do not stage** the untracked working-tree files `phrase.txt`, `sentences_sample.csv`, `sentences_seed.ndjson` — they are unrelated seed data. Stage only the files each step names.

---

### Task 1: WeaknessAnalyzer interface, prompt builder, and response type

**Files:**
- Create: `api/internal/app/analyzer.go`
- Create: `api/internal/app/analyzer_test.go`
- Modify: `api/internal/app/sentence.go` (add response type)

**Interfaces:**
- Consumes: `MistakeSentence`, `AnswerHistory` (existing, in `sentence.go`); `validExplainLanguages` (existing, in `explainer.go`).
- Produces:
  - `type WeaknessAnalyzer interface { Analyze(ctx context.Context, mistakes []MistakeSentence, language string) (string, error) }`
  - `func buildWeaknessPrompt(mistakes []MistakeSentence, language string) string`
  - `type MistakesInsightResponse struct { Insight string \`json:"insight"\` }`

- [ ] **Step 1: Write the failing prompt tests**

Create `api/internal/app/analyzer_test.go`:

```go
package app

import (
	"strings"
	"testing"
)

func mistakesFixture() []MistakeSentence {
	return []MistakeSentence{
		{
			SentenceID:    1,
			Japanese:      "時間がありません。",
			CorrectAnswer: "I don't have time.",
			WrongAnswers: []AnswerHistory{
				{ID: 1, IncorrectAnswer: "I have no time.", CreatedAt: "2026-01-03T00:00:00Z"},
			},
		},
	}
}

func TestBuildWeaknessPromptIncludesMistakes(t *testing.T) {
	prompt := buildWeaknessPrompt(mistakesFixture(), "en")
	for _, want := range []string{"時間がありません。", "I don't have time.", "I have no time."} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to include %q", want)
		}
	}
}

func TestBuildWeaknessPromptFocusesOnPatternsAndIgnoresTypos(t *testing.T) {
	prompt := buildWeaknessPrompt(mistakesFixture(), "en")
	if !strings.Contains(prompt, "recurring") {
		t.Fatal("expected prompt to ask for recurring weaknesses")
	}
	if !strings.Contains(strings.ToLower(prompt), "typo") {
		t.Fatal("expected prompt to instruct ignoring typos")
	}
}

func TestBuildWeaknessPromptWritesInRequestedLanguage(t *testing.T) {
	en := buildWeaknessPrompt(mistakesFixture(), "en")
	if !strings.HasSuffix(en, "in English.") {
		t.Fatalf("expected English instruction suffix, got: %q", en)
	}
	ja := buildWeaknessPrompt(mistakesFixture(), "ja")
	if !strings.HasSuffix(ja, "in Japanese.") {
		t.Fatalf("expected Japanese instruction suffix, got: %q", ja)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd api && go test ./internal/app/ -run TestBuildWeaknessPrompt -v`
Expected: build failure — `undefined: buildWeaknessPrompt` (and `MistakeSentence` already exists, so the only undefined symbol is the prompt function).

- [ ] **Step 3: Create `analyzer.go` with the interface and prompt builder**

Create `api/internal/app/analyzer.go`:

```go
package app

import (
	"context"
	"fmt"
	"strings"
)

// WeaknessAnalyzer produces a natural-language summary of a learner's
// recurring weaknesses from the set of sentences they have gotten wrong.
type WeaknessAnalyzer interface {
	Analyze(ctx context.Context, mistakes []MistakeSentence, language string) (string, error)
}

// buildWeaknessPrompt is a pure function (kept separate from the Gemini
// client so it is unit-testable without network access) that renders the
// learner's mistakes into an analysis prompt. It reuses validExplainLanguages
// semantics: "ja" produces a Japanese analysis, anything else English.
func buildWeaknessPrompt(mistakes []MistakeSentence, language string) string {
	var b strings.Builder
	b.WriteString("You are an English tutor analyzing the mistakes a Japanese learner has made ")
	b.WriteString("while translating Japanese sentences into English.\n\n")
	b.WriteString("Here are the sentences the learner has gotten wrong, each with the reference ")
	b.WriteString("English translation and the learner's incorrect attempts:\n\n")
	for _, m := range mistakes {
		b.WriteString(fmt.Sprintf("Japanese: %s\n", m.Japanese))
		b.WriteString(fmt.Sprintf("Reference English: %s\n", m.CorrectAnswer))
		for _, w := range m.WrongAnswers {
			b.WriteString(fmt.Sprintf("Learner wrote: %s\n", w.IncorrectAnswer))
		}
		b.WriteString("\n")
	}
	b.WriteString("Identify the learner's main recurring weaknesses across these mistakes — patterns ")
	b.WriteString("such as verb tense, subject-verb agreement, articles, plurals, prepositions, word ")
	b.WriteString("order, vocabulary choice, or register.\n")
	b.WriteString("Ignore one-off typos, spelling slips, and isolated misunderstandings that do not ")
	b.WriteString("represent a repeated pattern.\n")
	b.WriteString("Respond with a 1-2 sentence summary, then a short bulleted list of the top weakness ")
	b.WriteString("areas, each with a brief, actionable tip.\n")
	if language == "ja" {
		b.WriteString("Write your analysis in Japanese.")
	} else {
		b.WriteString("Write your analysis in English.")
	}
	return b.String()
}
```

- [ ] **Step 4: Add the response type to `sentence.go`**

In `api/internal/app/sentence.go`, immediately after the `ListMistakesResponse` type (around line 34), add:

```go
type MistakesInsightResponse struct {
	Insight string `json:"insight"`
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd api && go test ./internal/app/ -run TestBuildWeaknessPrompt -v`
Expected: PASS (all three tests).

- [ ] **Step 6: Commit**

```bash
cd api && go vet ./... && cd ..
git add api/internal/app/analyzer.go api/internal/app/analyzer_test.go api/internal/app/sentence.go
git commit -m "feat(api): add WeaknessAnalyzer interface and weakness prompt builder"
```

---

### Task 2: GeminiWeaknessAnalyzer

**Files:**
- Create: `api/internal/app/gemini_analyzer.go`
- Create: `api/internal/app/gemini_analyzer_test.go`

**Interfaces:**
- Consumes: `WeaknessAnalyzer` (Task 1), `buildWeaknessPrompt` (Task 1), `contentGenerator` / `geminiExplainModel` / `explainTimeout` (existing, in `gemini_explainer.go`), `fakeContentGenerator` (existing, in `gemini_explainer_test.go`).
- Produces:
  - `type GeminiWeaknessAnalyzer struct { models contentGenerator; model string }`
  - `func NewGeminiWeaknessAnalyzer(ctx context.Context, apiKey string) (*GeminiWeaknessAnalyzer, error)`
  - `func (g *GeminiWeaknessAnalyzer) Analyze(ctx context.Context, mistakes []MistakeSentence, language string) (string, error)`

- [ ] **Step 1: Write the failing analyzer tests**

Create `api/internal/app/gemini_analyzer_test.go`. This reuses `fakeContentGenerator`, already defined in `gemini_explainer_test.go` (same package) — do not redefine it.

```go
package app

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/genai"
)

func TestGeminiWeaknessAnalyzerReturnsText(t *testing.T) {
	fake := &fakeContentGenerator{
		resp: &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{Content: &genai.Content{Parts: []*genai.Part{{Text: "insight text"}}}},
			},
		},
	}
	g := &GeminiWeaknessAnalyzer{models: fake, model: "gemini-test"}

	got, err := g.Analyze(context.Background(),
		[]MistakeSentence{{Japanese: "x", CorrectAnswer: "y", WrongAnswers: []AnswerHistory{{IncorrectAnswer: "z"}}}},
		"en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "insight text" {
		t.Fatalf("unexpected insight: %q", got)
	}
	if fake.gotModel != "gemini-test" {
		t.Fatalf("unexpected model: %q", fake.gotModel)
	}
	if len(fake.gotContents) != 1 || len(fake.gotContents[0].Parts) != 1 || fake.gotContents[0].Parts[0].Text == "" {
		t.Fatalf("expected prompt text to be sent, got: %+v", fake.gotContents)
	}
}

func TestGeminiWeaknessAnalyzerPropagatesError(t *testing.T) {
	fake := &fakeContentGenerator{err: errors.New("network error")}
	g := &GeminiWeaknessAnalyzer{models: fake, model: "gemini-test"}

	_, err := g.Analyze(context.Background(), []MistakeSentence{{Japanese: "x"}}, "en")
	if err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd api && go test ./internal/app/ -run TestGeminiWeaknessAnalyzer -v`
Expected: build failure — `undefined: GeminiWeaknessAnalyzer`.

- [ ] **Step 3: Create `gemini_analyzer.go`**

Create `api/internal/app/gemini_analyzer.go`. It reuses `contentGenerator`, `geminiExplainModel`, and `explainTimeout` from `gemini_explainer.go` (same package):

```go
package app

import (
	"context"
	"fmt"

	"google.golang.org/genai"
)

// GeminiWeaknessAnalyzer implements WeaknessAnalyzer using the Gemini API,
// reusing the same client configuration, model, and timeout as
// GeminiExplainer.
type GeminiWeaknessAnalyzer struct {
	models contentGenerator
	model  string
}

func NewGeminiWeaknessAnalyzer(ctx context.Context, apiKey string) (*GeminiWeaknessAnalyzer, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("create genai client: %w", err)
	}
	return &GeminiWeaknessAnalyzer{models: client.Models, model: geminiExplainModel}, nil
}

func (g *GeminiWeaknessAnalyzer) Analyze(ctx context.Context, mistakes []MistakeSentence, language string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, explainTimeout)
	defer cancel()

	prompt := buildWeaknessPrompt(mistakes, language)
	contents := []*genai.Content{{Parts: []*genai.Part{{Text: prompt}}}}

	resp, err := g.models.GenerateContent(ctx, g.model, contents, nil)
	if err != nil {
		return "", fmt.Errorf("gemini generate content: %w", err)
	}
	return resp.Text(), nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd api && go test ./internal/app/ -run TestGeminiWeaknessAnalyzer -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
cd api && go vet ./... && cd ..
git add api/internal/app/gemini_analyzer.go api/internal/app/gemini_analyzer_test.go
git commit -m "feat(api): add GeminiWeaknessAnalyzer reusing the Explain client wiring"
```

---

### Task 3: Thread WeaknessAnalyzer through the Server (refactor, stays green)

This task changes `NewServer`'s signature to take the analyzer as a third dependency, symmetric with the explainer, and updates every caller. No new behavior — the deliverable is that the whole build compiles and every existing test still passes with the analyzer injectable.

**Files:**
- Modify: `api/internal/app/handlers.go` (Server struct + NewServer)
- Modify: `api/internal/app/handlers_test.go` (add `fakeAnalyzer`, update all `NewServer(...)` call sites)
- Modify: `api/main.go` (construct + pass analyzer)
- Modify: `api/cmd/e2eserver/main.go` (add `stubAnalyzer`, pass it)

**Interfaces:**
- Consumes: `WeaknessAnalyzer` (Task 1), `NewGeminiWeaknessAnalyzer` (Task 2).
- Produces:
  - `func NewServer(repo SentenceRepository, explainer Explainer, analyzer WeaknessAnalyzer) *Server` (Server now has an `analyzer WeaknessAnalyzer` field).
  - `type fakeAnalyzer struct { insight string; err error; calledWith []analyzeCall }` with `analyzeCall{ mistakes []MistakeSentence; language string }` (test-only, used by Task 4).

- [ ] **Step 1: Add `fakeAnalyzer` to `handlers_test.go`**

In `api/internal/app/handlers_test.go`, after the `fakeExplainer` block (after line 87), add:

```go
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
```

- [ ] **Step 2: Change the Server struct and NewServer in `handlers.go`**

In `api/internal/app/handlers.go`, replace the `Server` struct and `NewServer` (lines 22-29) with:

```go
type Server struct {
	repo      SentenceRepository
	explainer Explainer
	analyzer  WeaknessAnalyzer
}

func NewServer(repo SentenceRepository, explainer Explainer, analyzer WeaknessAnalyzer) *Server {
	return &Server{repo: repo, explainer: explainer, analyzer: analyzer}
}
```

- [ ] **Step 3: Update every `NewServer(...)` call site in `handlers_test.go`**

There are two call-site shapes; append a third argument to each:
- Replace every occurrence of `, &fakeExplainer{})` with `, &fakeExplainer{}, &fakeAnalyzer{})`.
- Replace every occurrence of `NewServer(repo, explainer)` with `NewServer(repo, explainer, &fakeAnalyzer{})`.

(After this, all ~20 `NewServer(...)` calls pass three arguments.)

- [ ] **Step 4: Update `main.go`**

In `api/main.go`, after the explainer block (lines 43-46), add the analyzer construction and update the `NewServer` call (line 48):

```go
	analyzer, err := app.NewGeminiWeaknessAnalyzer(ctx, geminiAPIKey)
	if err != nil {
		log.Fatalf("failed to create Gemini weakness analyzer: %v", err)
	}

	srv := app.NewServer(app.NewFirestoreRepo(client), explainer, analyzer)
```

- [ ] **Step 5: Update `cmd/e2eserver/main.go`**

In `api/cmd/e2eserver/main.go`, after the `stubExplainer` block (after line 24), add a stub analyzer (note the `app.` qualifier on `MistakeSentence`, since this is package `main`):

```go
// stubInsight is returned by stubAnalyzer so e2e runs never call real Gemini.
const stubInsight = "This is a stub weakness insight for e2e tests."

type stubAnalyzer struct{}

func (stubAnalyzer) Analyze(_ context.Context, _ []app.MistakeSentence, _ string) (string, error) {
	return stubInsight, nil
}
```

Then update the `NewServer` call (line 50) to:

```go
	srv := app.NewServer(app.NewFirestoreRepo(client), stubExplainer{}, stubAnalyzer{})
```

- [ ] **Step 6: Build and run the full backend suite to verify everything still passes**

Run: `cd api && go build ./... && go test ./...`
Expected: PASS — no test failures, both `api` and `api/cmd/e2eserver` compile.

- [ ] **Step 7: Commit**

```bash
cd api && go vet ./... && cd ..
git add api/internal/app/handlers.go api/internal/app/handlers_test.go api/main.go api/cmd/e2eserver/main.go
git commit -m "refactor(api): thread WeaknessAnalyzer dependency through Server"
```

---

### Task 4: getMistakesInsight handler + route

**Files:**
- Modify: `api/internal/app/handlers.go` (new handler + `maxInsightMistakes` const)
- Modify: `api/internal/app/handlers_test.go` (handler tests)
- Modify: `api/internal/app/router.go` (register route)

**Interfaces:**
- Consumes: `Server.analyzer` / `fakeAnalyzer` (Task 3), `s.repo.ListMistakes` (existing), `MistakesInsightResponse` (Task 1), `validExplainLanguages` (existing), `uidFromContext` / `writeJSON` (existing).
- Produces: `func (s *Server) getMistakesInsight(w http.ResponseWriter, r *http.Request)`; route `GET /api/mistakes/insight`.

- [ ] **Step 1: Write the failing handler tests**

Add to `api/internal/app/handlers_test.go` (end of file). These follow the existing `TestGetMistakes*` and `TestExplainAnswer*` patterns:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd api && go test ./internal/app/ -run TestGetMistakesInsight -v`
Expected: build failure — `srv.getMistakesInsight undefined` and `undefined: maxInsightMistakes`.

- [ ] **Step 3: Add the `maxInsightMistakes` constant**

In `api/internal/app/handlers.go`, extend the existing `const (...)` block (lines 12-20) by adding:

```go
	// maxInsightMistakes bounds how many recent mistakes are sent to the
	// weakness analyzer, keeping prompt size and Gemini cost predictable as a
	// learner's mistake history grows.
	maxInsightMistakes = 50
```

- [ ] **Step 4: Add the `getMistakesInsight` handler**

In `api/internal/app/handlers.go`, add after the existing `getMistakes` handler:

```go
func (s *Server) getMistakesInsight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	language := r.URL.Query().Get("language")
	if !validExplainLanguages[language] {
		http.Error(w, "Invalid language", http.StatusBadRequest)
		return
	}
	uid, _ := uidFromContext(r.Context())
	mistakes, err := s.repo.ListMistakes(r.Context(), uid)
	if err != nil {
		log.Printf("list mistakes error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if len(mistakes) == 0 {
		writeJSON(w, MistakesInsightResponse{Insight: ""})
		return
	}
	if len(mistakes) > maxInsightMistakes {
		mistakes = mistakes[:maxInsightMistakes]
	}
	insight, err := s.analyzer.Analyze(r.Context(), mistakes, language)
	if err != nil {
		log.Printf("analyze mistakes error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, MistakesInsightResponse{Insight: insight})
}
```

- [ ] **Step 5: Register the route**

In `api/internal/app/router.go`, add after the existing `/api/mistakes` registration:

```go
	mux.HandleFunc("/api/mistakes/insight", auth(srv.getMistakesInsight))
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd api && go test ./...`
Expected: PASS (all tests, including the new `TestGetMistakesInsight*`).

- [ ] **Step 7: Commit**

```bash
cd api && go vet ./... && cd ..
git add api/internal/app/handlers.go api/internal/app/handlers_test.go api/internal/app/router.go
git commit -m "feat(api): add GET /api/mistakes/insight endpoint"
```

---

### Task 5: Frontend API client — getMistakesInsight

**Files:**
- Modify: `fe/src/lib/api.ts`
- Modify: `fe/src/lib/api.test.ts`

**Interfaces:**
- Consumes: existing `request<T>` helper in `api.ts`.
- Produces:
  - `export interface MistakesInsightResponse { insight: string }`
  - `api.getMistakesInsight(language: 'en' | 'ja') => Promise<MistakesInsightResponse>` hitting `GET /api/mistakes/insight?language=<language>`.

- [ ] **Step 1: Write the failing api test**

Add to `fe/src/lib/api.test.ts`, after the `api.listMistakes` describe block (after line 149):

```ts
describe('api.getMistakesInsight', () => {
  it('sends GET /api/mistakes/insight with the language query param', async () => {
    mockResponse({ insight: 'You often drop articles.' })
    const result = await api.getMistakesInsight('en')
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/mistakes/insight?language=en'),
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: 'Bearer test-token' }),
      })
    )
    expect(result.insight).toBe('You often drop articles.')
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd fe && npm test -- src/lib/api.test.ts`
Expected: FAIL — `api.getMistakesInsight is not a function`.

- [ ] **Step 3: Add the interface and client method**

In `fe/src/lib/api.ts`, add the interface after `ExplainResponse` (after line 28):

```ts
export interface MistakesInsightResponse {
  insight: string
}
```

Then add the method inside the `api` object, after `listMistakes` (after line 71):

```ts
  getMistakesInsight: (language: 'en' | 'ja') =>
    request<MistakesInsightResponse>(`/api/mistakes/insight?language=${language}`),
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd fe && npm test -- src/lib/api.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add fe/src/lib/api.ts fe/src/lib/api.test.ts
git commit -m "feat(fe): add getMistakesInsight to the api client"
```

---

### Task 6: Mistakes page insight card

**Files:**
- Modify: `fe/src/components/Mistakes.tsx`
- Modify: `fe/src/components/Mistakes.test.tsx`

**Interfaces:**
- Consumes: `api.getMistakesInsight` (Task 5), existing `api.listMistakes`, existing `Card`/`CardHeader`/`CardTitle`/`CardContent`/`Button` primitives.
- Produces: an insight card rendered above the mistake list, driven by `insight` / `insightLoading` / `insightError` state; auto-fetched after a non-empty list load; retriable on error.

- [ ] **Step 1: Update the test mock and write the failing insight tests**

In `fe/src/components/Mistakes.test.tsx`:

First, extend the `vi.mock('@/lib/api', ...)` block (lines 4-8) and the `mockApi` cast (lines 13-15) to include `getMistakesInsight`:

```ts
vi.mock('@/lib/api', () => ({
  api: {
    listMistakes: vi.fn(),
    getMistakesInsight: vi.fn(),
  },
}))

import { api } from '@/lib/api'
import Mistakes from './Mistakes'

const mockApi = api as unknown as {
  listMistakes: ReturnType<typeof vi.fn>
  getMistakesInsight: ReturnType<typeof vi.fn>
}
```

Then give the existing tests a default insight resolution so a non-empty list load doesn't hit an unconfigured mock. Update `beforeEach` (lines 17-19) to:

```ts
beforeEach(() => {
  vi.clearAllMocks()
  mockApi.getMistakesInsight.mockResolvedValue({ insight: '' })
})
```

Then add these tests inside the `describe('Mistakes', ...)` block:

```ts
  it('fetches and renders the weakness insight above the list', async () => {
    mockApi.listMistakes.mockResolvedValue({
      mistakes: [
        {
          sentence_id: 1,
          japanese: '時間がありません。',
          correct_answer: "I don't have time.",
          wrong_answers: [{ id: 1, incorrect_answer: 'I have no time.', created_at: '2026-01-03T00:00:00Z' }],
        },
      ],
    })
    mockApi.getMistakesInsight.mockResolvedValue({ insight: 'You often drop articles.' })
    render(<Mistakes />)
    await screen.findByText('You often drop articles.')
    expect(screen.getByText('時間がありません。')).toBeInTheDocument()
  })

  it('shows an insight loading state while the insight is pending', async () => {
    mockApi.listMistakes.mockResolvedValue({
      mistakes: [
        {
          sentence_id: 1,
          japanese: '時間がありません。',
          correct_answer: "I don't have time.",
          wrong_answers: [{ id: 1, incorrect_answer: 'I have no time.', created_at: '2026-01-03T00:00:00Z' }],
        },
      ],
    })
    mockApi.getMistakesInsight.mockReturnValue(new Promise(() => {}))
    render(<Mistakes />)
    await screen.findByText(/analyzing your mistakes/i)
  })

  it('shows an insight error with a working retry button', async () => {
    mockApi.listMistakes.mockResolvedValue({
      mistakes: [
        {
          sentence_id: 1,
          japanese: '時間がありません。',
          correct_answer: "I don't have time.",
          wrong_answers: [{ id: 1, incorrect_answer: 'I have no time.', created_at: '2026-01-03T00:00:00Z' }],
        },
      ],
    })
    mockApi.getMistakesInsight.mockRejectedValueOnce(new Error('insight boom'))
    render(<Mistakes />)
    await screen.findByText('insight boom')
    mockApi.getMistakesInsight.mockResolvedValueOnce({ insight: 'You often drop articles.' })
    fireEvent.click(screen.getByRole('button', { name: /try again/i }))
    await screen.findByText('You often drop articles.')
  })

  it('does not fetch an insight when there are no mistakes', async () => {
    mockApi.listMistakes.mockResolvedValue({ mistakes: [] })
    render(<Mistakes />)
    await screen.findByText(/no mistakes yet/i)
    expect(mockApi.getMistakesInsight).not.toHaveBeenCalled()
  })
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd fe && npm test -- src/components/Mistakes.test.tsx`
Expected: FAIL — the new tests fail (no "Analyzing your mistakes" text, insight text never rendered, and `getMistakesInsight` is called on the empty case because the component doesn't guard yet). The four pre-existing tests still pass.

- [ ] **Step 3: Add insight state and fetching to `Mistakes.tsx`**

In `fe/src/components/Mistakes.tsx`, add the three state hooks after the existing `error` state (after line 12):

```tsx
  const [insight, setInsight] = useState<string | null>(null)
  const [insightLoading, setInsightLoading] = useState(false)
  const [insightError, setInsightError] = useState<string | null>(null)
```

Add a `loadInsight` function above `loadMistakes` (it reads the shared language preference the Explain toggle persists):

```tsx
  const loadInsight = async () => {
    setInsightLoading(true)
    setInsightError(null)
    try {
      const stored = typeof window !== 'undefined' ? localStorage.getItem('eagle:explainLanguage') : null
      const language = stored === 'ja' ? 'ja' : 'en'
      const result = await api.getMistakesInsight(language)
      setInsight(result.insight)
    } catch (err) {
      setInsightError(err instanceof Error ? err.message : 'Failed to load insight')
    } finally {
      setInsightLoading(false)
    }
  }
```

Then, inside `loadMistakes`, after `setMistakes(result.mistakes)`, trigger the insight only when there are mistakes:

```tsx
      setMistakes(result.mistakes)
      if (result.mistakes.length > 0) {
        loadInsight()
      }
```

- [ ] **Step 4: Render the insight card above the list**

In `fe/src/components/Mistakes.tsx`, in the populated-list branch, render the insight card immediately before the `{mistakes?.map(...)}` block, inside the `<div className="space-y-3">` container:

```tsx
            {(insightLoading || insight || insightError) && (
              <Card className="border-indigo-300">
                <CardHeader className="pb-2">
                  <CardTitle className="text-base text-indigo-900">Weakness Insight</CardTitle>
                </CardHeader>
                <CardContent>
                  {insightLoading ? (
                    <p className="text-sm text-gray-600">Analyzing your mistakes…</p>
                  ) : insightError ? (
                    <div className="space-y-2">
                      <p className="text-sm text-red-700">{insightError}</p>
                      <Button variant="outline" size="sm" onClick={loadInsight}>
                        Try Again
                      </Button>
                    </div>
                  ) : (
                    <div className="whitespace-pre-wrap text-sm text-gray-800">{insight}</div>
                  )}
                </CardContent>
              </Card>
            )}
```

(The `Button` import already exists in this file.)

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd fe && npm test -- src/components/Mistakes.test.tsx`
Expected: PASS (all tests — the four original plus the four new ones).

- [ ] **Step 6: Run the full frontend suite and lint**

Run: `cd fe && npm test && npm run lint`
Expected: PASS — no regressions, no lint errors.

- [ ] **Step 7: Commit**

```bash
git add fe/src/components/Mistakes.tsx fe/src/components/Mistakes.test.tsx
git commit -m "feat(fe): auto-generate a weakness insight card on the mistakes page"
```

---

## Self-Review

**Spec coverage** — every spec section maps to a task:
- Endpoint `GET /api/mistakes/insight` (server-side load, cap-50, empty→no-call, language validation) → Task 4; response type → Task 1; wiring → Task 3.
- `WeaknessAnalyzer` interface + pure `buildWeaknessPrompt` (focus-on-patterns / ignore-typos, language) → Task 1.
- `GeminiWeaknessAnalyzer` reusing client/model/timeout/test-seam → Task 2.
- `main.go` real analyzer + `e2eserver` stub → Task 3.
- Frontend `api.getMistakesInsight` → Task 5; auto-load card, shared language preference, loading/error/retry, skip-when-empty → Task 6.
- All spec test cases (backend prompt, handler empty/populated/cap/error/language, gemini analyzer, api client, component states) are covered by the tests in Tasks 1–6. No test calls real Gemini.

**Placeholder scan** — no TBD/TODO; every code step contains full code.

**Type consistency** — `Analyze(ctx, []MistakeSentence, string) (string, error)`, `MistakesInsightResponse{Insight string}`, `maxInsightMistakes`, `getMistakesInsight`, and `api.getMistakesInsight(language)` are used identically across the interface (Task 1), implementation (Task 2), Server/fake (Task 3), handler/tests (Task 4), and frontend (Tasks 5–6). `NewServer`'s three-arg signature is consistent from Task 3 onward.
