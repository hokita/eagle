# Explain Button Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an "Explain" button that appears when a user's translation is marked incorrect; clicking it sends the Japanese sentence, the stored reference answer, and the user's answer to Gemini and displays back an explanation.

**Architecture:** A new Go backend endpoint (`POST /api/answer/explain`) wraps a single non-streaming Gemini call behind an `Explainer` interface (mirroring the existing `SentenceRepository` seam), following corgi's single-shot `GeminiTitleGenerator` pattern rather than its streaming chat provider. The Next.js frontend adds one API function and one button/state block to the existing `Translator` component — no new components, no persistence.

**Tech Stack:** Go backend (`eagle/api`) using `google.golang.org/genai` (Google's current unified Go SDK); Next.js/React frontend (`eagle/fe`) using the existing `fetch`-based `api.ts` client; Go `testing` + `httptest` and Vitest + React Testing Library for tests.

## Global Constraints

- Response delivery is single-shot request/response — no streaming/SSE.
- Explanations are not persisted anywhere (not Firestore, not re-shown on revisit).
- The Explain button only renders when `showAnswer && feedback === 'incorrect'`.
- The explanation is written in English.
- The prompt must instruct the LLM to judge the user's answer on its own merits (natural, acceptable English), not simply diff it against the stored reference — the reference is one valid translation, not the only correct one.
- Env var name: `GEMINI_API_KEY` (matches corgi's naming).
- Model: `gemini-2.5-flash`.
- No retry/backoff logic on the Gemini call; wrap each call in a ~20s context timeout.
- On a backend/LLM error, return an error to the client (HTTP 500) — no silent fallback text, since a failed explanation has no meaningful default (unlike corgi's title generator, which degrades to a truncated string).
- No test may call the real Gemini API — the SDK call is mocked via a Go interface seam; the frontend mocks `@/lib/api`.

---

### Task 1: Explainer interface and prompt construction

**Files:**
- Create: `api/explainer.go`
- Test: `api/explainer_test.go`

**Interfaces:**
- Produces: `type Explainer interface { Explain(ctx context.Context, japanese, correctAnswer, userAnswer string) (string, error) }` and `func buildExplainPrompt(japanese, correctAnswer, userAnswer string) string` — both consumed by Task 2 (`GeminiExplainer`) and Task 3 (`Server.explainer` field / `explainAnswer` handler).

- [ ] **Step 1: Write the failing test**

Create `api/explainer_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestBuildExplainPromptIncludesInputs(t *testing.T) {
	prompt := buildExplainPrompt("時間がありません。", "I don't have time.", "I have no time.")
	if !strings.Contains(prompt, "時間がありません。") {
		t.Fatal("expected prompt to include the Japanese sentence")
	}
	if !strings.Contains(prompt, "I don't have time.") {
		t.Fatal("expected prompt to include the reference answer")
	}
	if !strings.Contains(prompt, "I have no time.") {
		t.Fatal("expected prompt to include the learner's answer")
	}
}

func TestBuildExplainPromptInstructsJudgingOnMerits(t *testing.T) {
	prompt := buildExplainPrompt("日本語", "reference", "answer")
	if !strings.Contains(prompt, "only one valid way") {
		t.Fatal("expected prompt to instruct that the reference is not the only correct answer")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./... -run TestBuildExplainPrompt -v`
Expected: FAIL with `undefined: buildExplainPrompt`

- [ ] **Step 3: Write minimal implementation**

Create `api/explainer.go`:

```go
package main

import (
	"context"
	"fmt"
	"strings"
)

// Explainer generates a natural-language explanation comparing a learner's
// English translation to a reference translation of a Japanese sentence.
type Explainer interface {
	Explain(ctx context.Context, japanese, correctAnswer, userAnswer string) (string, error)
}

func buildExplainPrompt(japanese, correctAnswer, userAnswer string) string {
	var b strings.Builder
	b.WriteString("You are an English tutor helping a Japanese speaker learn English translation.\n\n")
	b.WriteString(fmt.Sprintf("Japanese sentence: %s\n", japanese))
	b.WriteString(fmt.Sprintf("Reference English translation: %s\n", correctAnswer))
	b.WriteString(fmt.Sprintf("Learner's English translation: %s\n\n", userAnswer))
	b.WriteString("The reference translation is only one valid way to translate the sentence, not the only correct answer. ")
	b.WriteString("Judge the learner's translation on its own merits: is it natural, grammatically correct English that ")
	b.WriteString("conveys the same meaning as the Japanese sentence?\n\n")
	b.WriteString("If the learner's translation is acceptable, say so clearly and explain any difference in nuance, ")
	b.WriteString("formality, or phrasing compared to the reference — do not imply it was wrong just because it differs.\n")
	b.WriteString("If the learner's translation has a real grammar, vocabulary, or meaning error, explain what is wrong ")
	b.WriteString("and why the reference translation is more correct.\n\n")
	b.WriteString("Keep the explanation concise (2-4 sentences) and write it in English.")
	return b.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go test ./... -run TestBuildExplainPrompt -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/explainer.go api/explainer_test.go
git commit -m "feat(api): add Explainer interface and explain prompt builder"
```

---

### Task 2: GeminiExplainer (Gemini SDK wrapper)

**Files:**
- Create: `api/gemini_explainer.go`
- Test: `api/gemini_explainer_test.go`
- Modify: `api/go.mod`, `api/go.sum` (via `go get`)

**Interfaces:**
- Consumes: `Explainer` interface and `buildExplainPrompt` from Task 1.
- Produces: `type GeminiExplainer struct { models contentGenerator; model string }` implementing `Explainer`, and `func NewGeminiExplainer(ctx context.Context, apiKey string) (*GeminiExplainer, error)` — consumed by Task 4 (`main.go` wiring).

- [ ] **Step 1: Add the Gemini Go SDK dependency**

Run: `cd api && go get google.golang.org/genai`
Expected: `go.mod` gains a `require google.golang.org/genai v1.64.0` line (or newer) and `go.sum` is updated.

- [ ] **Step 2: Write the failing test**

Create `api/gemini_explainer_test.go`:

```go
package main

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/genai"
)

// fakeContentGenerator substitutes for *genai.Models in tests, so no test
// makes a real network call to Gemini.
type fakeContentGenerator struct {
	resp *genai.GenerateContentResponse
	err  error

	gotModel    string
	gotContents []*genai.Content
}

func (f *fakeContentGenerator) GenerateContent(_ context.Context, model string, contents []*genai.Content, _ *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	f.gotModel = model
	f.gotContents = contents
	return f.resp, f.err
}

func TestGeminiExplainerExplainReturnsText(t *testing.T) {
	fake := &fakeContentGenerator{
		resp: &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{Content: &genai.Content{Parts: []*genai.Part{{Text: "explanation text"}}}},
			},
		},
	}
	g := &GeminiExplainer{models: fake, model: "gemini-2.5-flash"}

	got, err := g.Explain(context.Background(), "japanese", "correct", "user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "explanation text" {
		t.Fatalf("unexpected explanation: %q", got)
	}
	if fake.gotModel != "gemini-2.5-flash" {
		t.Fatalf("unexpected model: %q", fake.gotModel)
	}
	if len(fake.gotContents) != 1 || len(fake.gotContents[0].Parts) != 1 {
		t.Fatalf("unexpected contents: %+v", fake.gotContents)
	}
	if fake.gotContents[0].Parts[0].Text == "" {
		t.Fatal("expected prompt text to be sent")
	}
}

func TestGeminiExplainerExplainPropagatesError(t *testing.T) {
	fake := &fakeContentGenerator{err: errors.New("network error")}
	g := &GeminiExplainer{models: fake, model: "gemini-2.5-flash"}

	_, err := g.Explain(context.Background(), "japanese", "correct", "user")
	if err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd api && go test ./... -run TestGeminiExplainer -v`
Expected: FAIL with `undefined: GeminiExplainer`

- [ ] **Step 4: Write minimal implementation**

Create `api/gemini_explainer.go`:

```go
package main

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/genai"
)

const (
	geminiExplainModel = "gemini-2.5-flash"
	explainTimeout      = 20 * time.Second
)

// contentGenerator is the seam between GeminiExplainer and the genai SDK, so
// tests can substitute a fake instead of making real network calls.
// *genai.Models satisfies this interface structurally.
type contentGenerator interface {
	GenerateContent(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error)
}

// GeminiExplainer implements Explainer using the Gemini API.
type GeminiExplainer struct {
	models contentGenerator
	model  string
}

func NewGeminiExplainer(ctx context.Context, apiKey string) (*GeminiExplainer, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("create genai client: %w", err)
	}
	return &GeminiExplainer{models: client.Models, model: geminiExplainModel}, nil
}

func (g *GeminiExplainer) Explain(ctx context.Context, japanese, correctAnswer, userAnswer string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, explainTimeout)
	defer cancel()

	prompt := buildExplainPrompt(japanese, correctAnswer, userAnswer)
	contents := []*genai.Content{{Parts: []*genai.Part{{Text: prompt}}}}

	resp, err := g.models.GenerateContent(ctx, g.model, contents, nil)
	if err != nil {
		return "", fmt.Errorf("gemini generate content: %w", err)
	}
	return resp.Text(), nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd api && go test ./... -run TestGeminiExplainer -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add api/gemini_explainer.go api/gemini_explainer_test.go api/go.mod api/go.sum
git commit -m "feat(api): add GeminiExplainer wrapping the Gemini Go SDK"
```

---

### Task 3: `/api/answer/explain` handler and Server wiring

**Files:**
- Modify: `api/sentence.go`
- Modify: `api/handlers.go`
- Modify: `api/handlers_test.go`

**Interfaces:**
- Consumes: `Explainer` interface from Task 1.
- Produces: `ExplainRequest{Japanese, CorrectAnswer, UserAnswer string}`, `ExplainResponse{Explanation string}`, `Server.explainAnswer(w http.ResponseWriter, r *http.Request)`, and the new `NewServer(repo SentenceRepository, explainer Explainer) *Server` signature — consumed by Task 4 (`main.go` route registration and construction).

- [ ] **Step 1: Write the failing tests**

In `api/handlers_test.go`, add `"errors"` to the import block (alongside `"context"`, `"encoding/json"`, `"net/http"`, `"net/http/httptest"`, `"strings"`, `"testing"`).

Add this type after `fakeRepo` and before `authed`:

```go
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
```

Add these test functions at the end of the file:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./... -run TestExplainAnswer -v`
Expected: FAIL to compile — `undefined: ExplainResponse`, `srv.explainAnswer undefined`, and `not enough arguments in call to NewServer`

- [ ] **Step 3: Write minimal implementation**

In `api/sentence.go`, add after the `ReportSentenceRequest` struct:

```go
type ExplainRequest struct {
	Japanese      string `json:"japanese"`
	CorrectAnswer string `json:"correct_answer"`
	UserAnswer    string `json:"user_answer"`
}

type ExplainResponse struct {
	Explanation string `json:"explanation"`
}
```

In `api/handlers.go`, replace the `Server` struct and `NewServer`:

```go
type Server struct {
	repo      SentenceRepository
	explainer Explainer
}

func NewServer(repo SentenceRepository, explainer Explainer) *Server {
	return &Server{repo: repo, explainer: explainer}
}
```

Add this handler after `reportSentence` and before `livenessHandler`:

```go
func (s *Server) explainAnswer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req ExplainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	explanation, err := s.explainer.Explain(r.Context(), req.Japanese, req.CorrectAnswer, req.UserAnswer)
	if err != nil {
		log.Printf("explain answer error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, ExplainResponse{Explanation: explanation})
}
```

In `api/handlers_test.go`, update every existing `NewServer(...)` call site to pass a second `&fakeExplainer{}` argument — there are 7 call sites:

- `TestGetRandomSentenceOK`: `srv := NewServer(repo)` → `srv := NewServer(repo, &fakeExplainer{})`
- `TestGetRandomSentenceNoCandidate`: `srv := NewServer(&fakeRepo{randomErr: ErrNoCandidate})` → `srv := NewServer(&fakeRepo{randomErr: ErrNoCandidate}, &fakeExplainer{})`
- `TestCheckAnswerCorrect`: `srv := NewServer(repo)` → `srv := NewServer(repo, &fakeExplainer{})`
- `TestCheckAnswerIncorrectRecordsAnswer`: `srv := NewServer(repo)` → `srv := NewServer(repo, &fakeExplainer{})`
- `TestCheckAnswerNotFound`: `srv := NewServer(&fakeRepo{correctErr: ErrNotFound})` → `srv := NewServer(&fakeRepo{correctErr: ErrNotFound}, &fakeExplainer{})`
- `TestReportSentence`: `srv := NewServer(repo)` → `srv := NewServer(repo, &fakeExplainer{})`
- `TestMethodNotAllowed`: `srv := NewServer(&fakeRepo{})` → `srv := NewServer(&fakeRepo{}, &fakeExplainer{})`

- [ ] **Step 4: Run all backend tests to verify they pass**

Run: `cd api && go test ./... -v`
Expected: PASS (all tests, including the pre-existing ones)

- [ ] **Step 5: Commit**

```bash
git add api/sentence.go api/handlers.go api/handlers_test.go
git commit -m "feat(api): add POST /api/answer/explain handler"
```

---

### Task 4: Wire GeminiExplainer into main.go

**Files:**
- Modify: `api/main.go`
- Modify: `api/.env.example`

**Interfaces:**
- Consumes: `NewGeminiExplainer` from Task 2, `NewServer(repo, explainer)` from Task 3.

- [ ] **Step 1: Add the required env var and construct the explainer**

In `api/main.go`, after the existing `allowedEmail` required-env check and before `client, err := firestore.NewClient(...)`, add:

```go
	geminiAPIKey := os.Getenv("GEMINI_API_KEY")
	if geminiAPIKey == "" {
		log.Fatal("GEMINI_API_KEY is required")
	}
```

Replace:

```go
	srv := NewServer(NewFirestoreRepo(client))
```

with:

```go
	explainer, err := NewGeminiExplainer(ctx, geminiAPIKey)
	if err != nil {
		log.Fatalf("failed to create Gemini explainer: %v", err)
	}

	srv := NewServer(NewFirestoreRepo(client), explainer)
```

Add the new route after the `/api/answer/check` registration:

```go
	mux.HandleFunc("/api/answer/explain", auth(srv.explainAnswer))
```

- [ ] **Step 2: Add the env var to the example file**

In `api/.env.example`, add:

```
GEMINI_API_KEY=your-gemini-api-key
```

- [ ] **Step 3: Verify the backend builds and all tests still pass**

Run: `cd api && go build ./... && go test ./... -v`
Expected: build succeeds, all tests PASS

- [ ] **Step 4: Commit**

```bash
git add api/main.go api/.env.example
git commit -m "feat(api): wire GeminiExplainer and GEMINI_API_KEY into main"
```

---

### Task 5: Frontend API client for explain

**Files:**
- Modify: `fe/src/lib/api.ts`
- Modify: `fe/src/lib/api.test.ts`

**Interfaces:**
- Produces: `interface ExplainResponse { explanation: string }` and `api.explainAnswer(japanese: string, correctAnswer: string, userAnswer: string): Promise<ExplainResponse>` — consumed by Task 6 (`Translator.tsx`).

- [ ] **Step 1: Write the failing test**

In `fe/src/lib/api.test.ts`, add after the `describe('api.reportSentence', ...)` block:

```ts
describe('api.explainAnswer', () => {
  it('sends POST /api/answer/explain with japanese, correct_answer, and user_answer', async () => {
    mockResponse({ explanation: 'Your answer is also natural.' })
    const result = await api.explainAnswer('時間がありません。', "I don't have time.", 'I have no time.')
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/answer/explain'),
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          japanese: '時間がありません。',
          correct_answer: "I don't have time.",
          user_answer: 'I have no time.',
        }),
        headers: expect.objectContaining({ Authorization: 'Bearer test-token' }),
      })
    )
    expect(result.explanation).toBe('Your answer is also natural.')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd fe && npx vitest run src/lib/api.test.ts`
Expected: FAIL — `api.explainAnswer is not a function`

- [ ] **Step 3: Write minimal implementation**

In `fe/src/lib/api.ts`, add after the `CheckAnswerResponse` interface:

```ts
export interface ExplainResponse {
  explanation: string
}
```

In the `api` object, add after `checkAnswer`:

```ts
  explainAnswer: (japanese: string, correctAnswer: string, userAnswer: string) =>
    request<ExplainResponse>('/api/answer/explain', {
      method: 'POST',
      body: JSON.stringify({ japanese, correct_answer: correctAnswer, user_answer: userAnswer }),
    }),
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd fe && npx vitest run src/lib/api.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add fe/src/lib/api.ts fe/src/lib/api.test.ts
git commit -m "feat(fe): add api.explainAnswer client function"
```

---

### Task 6: Explain button in Translator

**Files:**
- Modify: `fe/src/components/Translator.tsx`
- Create: `fe/src/components/Translator.test.tsx`

**Interfaces:**
- Consumes: `api.explainAnswer` and `ExplainResponse` from Task 5.

- [ ] **Step 1: Write the failing tests**

Create `fe/src/components/Translator.test.tsx`:

```tsx
import { render, screen, fireEvent, act } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { User } from 'firebase/auth'

vi.mock('@/lib/firebase', () => ({ auth: {} }))
vi.mock('firebase/auth', () => ({ signOut: vi.fn() }))

vi.mock('@/lib/api', () => ({
  api: {
    getRandomSentence: vi.fn(),
    checkAnswer: vi.fn(),
    explainAnswer: vi.fn(),
    reportSentence: vi.fn(),
  },
}))

import { api } from '@/lib/api'
import Translator from './Translator'

const mockApi = api as unknown as {
  getRandomSentence: ReturnType<typeof vi.fn>
  checkAnswer: ReturnType<typeof vi.fn>
  explainAnswer: ReturnType<typeof vi.fn>
  reportSentence: ReturnType<typeof vi.fn>
}

const fakeUser = { uid: 'u1', displayName: 'Jane' } as User

const fakeSentence = {
  id: 1,
  japanese: '時間がありません。',
  english: "I don't have time.",
  page: '12',
  correct_count: 0,
  incorrect_count: 0,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

beforeEach(() => {
  vi.clearAllMocks()
  mockApi.getRandomSentence.mockResolvedValue(fakeSentence)
})

async function answerIncorrectly() {
  render(<Translator user={fakeUser} />)
  await screen.findByText(fakeSentence.japanese)
  fireEvent.change(screen.getByLabelText(/your english translation/i), {
    target: { value: 'I have no time.' },
  })
  fireEvent.click(screen.getByRole('button', { name: /check translation/i }))
  await screen.findByText(/not quite right/i)
}

describe('Explain button', () => {
  it('is not shown before the answer is checked', async () => {
    render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    expect(screen.queryByRole('button', { name: /^explain$/i })).not.toBeInTheDocument()
  })

  it('is not shown when the answer was correct', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: true,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    fireEvent.change(screen.getByLabelText(/your english translation/i), {
      target: { value: fakeSentence.english },
    })
    fireEvent.click(screen.getByRole('button', { name: /check translation/i }))
    await screen.findByText(/correct! well done/i)
    expect(screen.queryByRole('button', { name: /^explain$/i })).not.toBeInTheDocument()
  })

  it('is shown when the answer was incorrect', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    await answerIncorrectly()
    expect(screen.getByRole('button', { name: /^explain$/i })).toBeInTheDocument()
  })

  it('calls api.explainAnswer with the sentence, correct answer, and user answer', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    mockApi.explainAnswer.mockResolvedValue({ explanation: 'Great nuance explanation.' })
    await answerIncorrectly()
    fireEvent.click(screen.getByRole('button', { name: /^explain$/i }))
    await screen.findByText('Great nuance explanation.')
    expect(mockApi.explainAnswer).toHaveBeenCalledWith(
      fakeSentence.japanese,
      fakeSentence.english,
      'I have no time.'
    )
  })

  it('shows a loading state while waiting for the explanation', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    let resolveExplain: (value: { explanation: string }) => void = () => {}
    mockApi.explainAnswer.mockReturnValue(
      new Promise(resolve => {
        resolveExplain = resolve
      })
    )
    await answerIncorrectly()
    fireEvent.click(screen.getByRole('button', { name: /^explain$/i }))
    await screen.findByRole('button', { name: /explaining/i })
    await act(async () => resolveExplain({ explanation: 'done' }))
    await screen.findByText('done')
  })

  it('renders the explanation once it resolves', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    mockApi.explainAnswer.mockResolvedValue({
      explanation: 'Your answer is also natural; the reference is just more formal.',
    })
    await answerIncorrectly()
    fireEvent.click(screen.getByRole('button', { name: /^explain$/i }))
    expect(
      await screen.findByText('Your answer is also natural; the reference is just more formal.')
    ).toBeInTheDocument()
  })

  it('shows an error and keeps the button clickable when the call fails', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    mockApi.explainAnswer.mockRejectedValue(new Error('API error: 500'))
    await answerIncorrectly()
    fireEvent.click(screen.getByRole('button', { name: /^explain$/i }))
    await screen.findByText('API error: 500')
    expect(screen.getByRole('button', { name: /^explain$/i })).toBeEnabled()
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd fe && npx vitest run src/components/Translator.test.tsx`
Expected: FAIL — no button with accessible name matching `/^explain$/i` exists yet

- [ ] **Step 3: Write minimal implementation**

In `fe/src/components/Translator.tsx`, add new state after the existing `isSpeaking` state declaration:

```tsx
  const [explanation, setExplanation] = useState<string | null>(null)
  const [explaining, setExplaining] = useState(false)
  const [explainError, setExplainError] = useState<string | null>(null)
```

Add this function after `reportSentence`:

```tsx
  const explainAnswer = async () => {
    if (!currentSentence) return
    setExplaining(true)
    setExplainError(null)
    try {
      const result = await api.explainAnswer(
        currentSentence.japanese,
        currentSentence.english,
        userTranslation
      )
      setExplanation(result.explanation)
    } catch (err) {
      setExplainError(err instanceof Error ? err.message : 'Failed to load explanation')
    } finally {
      setExplaining(false)
    }
  }
```

In `nextSentence`, add alongside the existing resets (after `setIsSpeaking(false)`):

```tsx
    setExplanation(null)
    setExplaining(false)
    setExplainError(null)
```

In the JSX, inside the `{showAnswer && (<div className="space-y-4"> ... )}` block, add this after the `{histories.length > 0 && (...)}` block and before its closing `</div>`:

```tsx
                  {feedback === 'incorrect' && (
                    <div className="space-y-2">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={explainAnswer}
                        disabled={explaining}
                      >
                        {explaining ? 'Explaining...' : 'Explain'}
                      </Button>

                      {explainError && (
                        <Alert className="border-red-500 bg-red-50">
                          <AlertDescription className="text-red-800">
                            {explainError}
                          </AlertDescription>
                        </Alert>
                      )}

                      {explanation && (
                        <div className="p-4 bg-purple-50 rounded-lg border border-purple-200">
                          <div className="font-semibold text-purple-900 mb-1">Explanation:</div>
                          <div className="text-purple-800 whitespace-pre-wrap">{explanation}</div>
                        </div>
                      )}
                    </div>
                  )}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd fe && npx vitest run src/components/Translator.test.tsx`
Expected: PASS

- [ ] **Step 5: Run the full frontend test suite to check for regressions**

Run: `cd fe && npm test`
Expected: PASS (all existing tests plus the new ones)

- [ ] **Step 6: Commit**

```bash
git add fe/src/components/Translator.tsx fe/src/components/Translator.test.tsx
git commit -m "feat(fe): add Explain button to Translator"
```
