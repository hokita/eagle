# Explanation Language Toggle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the user toggle the Explain button's output between English and Japanese, with the choice persisted in `localStorage` across sentences and sessions.

**Architecture:** The frontend adds an `explainLanguage` state (`'en' | 'ja'`) initialized from `localStorage`, rendered as an EN/JA toggle beside the Explain button. The chosen language is sent as a `language` field on `POST /api/answer/explain`. The Go backend validates it against an allow-list and appends a language-specific instruction line to the existing Gemini prompt — no other backend behavior changes.

**Tech Stack:** Go (backend, `net/http`, `google.golang.org/genai`), Next.js/React + TypeScript (frontend), Vitest + Testing Library (frontend tests), Go `testing` package (backend tests).

## Global Constraints

- Only two language values are ever valid: `"en"` and `"ja"` (spec: Non-goals — no other languages).
- The `localStorage` key is exactly `eagle:explainLanguage` (spec: Frontend design).
- Default language for a first-time user (no stored value) is `"en"` (spec: Architecture / data flow, step 1).
- The language preference is **not** reset by `nextSentence()` — it is a standing preference (spec: Architecture / data flow, step 6).
- No server-side persistence of the language preference (spec: Non-goals).
- Flipping the toggle while an explanation is already shown must immediately re-fetch in the new language; flipping it before any explanation exists must only update the selection, with no API call (spec: Architecture / data flow, step 3).

---

### Task 1: Backend — thread `language` through the explain endpoint

**Files:**
- Modify: `api/internal/app/explainer.go`
- Modify: `api/internal/app/gemini_explainer.go`
- Modify: `api/internal/app/sentence.go`
- Modify: `api/internal/app/handlers.go`
- Test: `api/internal/app/explainer_test.go`
- Test: `api/internal/app/gemini_explainer_test.go`
- Test: `api/internal/app/handlers_test.go`

**Interfaces:**
- Produces: `Explainer.Explain(ctx context.Context, japanese, correctAnswer, userAnswer, language string) (string, error)` — the interface every explainer implementation (including test fakes) must satisfy from this task onward.
- Produces: `buildExplainPrompt(japanese, correctAnswer, userAnswer, language string) string`.
- Produces: `validExplainLanguages map[string]bool` (keys `"en"`, `"ja"`) in `explainer.go`, reused by `handlers.go` for request validation.
- Produces: `ExplainRequest.Language string` (json tag `language`) in `sentence.go`.

Because `Explainer` is a shared interface, `explainer.go`, `gemini_explainer.go`, and the `fakeExplainer` in `handlers_test.go` must change together for the `app` package to compile — this task updates all test files first (the package will fail to *compile*, which is this task's "red"), then all implementation files together (package compiles and tests pass, "green").

- [ ] **Step 1: Update `explainer_test.go` — add a language-instruction test, update existing calls**

Replace the file's contents with:

```go
package app

import (
	"strings"
	"testing"
)

func TestBuildExplainPromptIncludesInputs(t *testing.T) {
	prompt := buildExplainPrompt("時間がありません。", "I don't have time.", "I have no time.", "en")
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
	prompt := buildExplainPrompt("日本語", "reference", "answer", "en")
	if !strings.Contains(prompt, "only one valid way") {
		t.Fatal("expected prompt to instruct that the reference is not the only correct answer")
	}
}

func TestBuildExplainPromptWritesInRequestedLanguage(t *testing.T) {
	enPrompt := buildExplainPrompt("日本語", "reference", "answer", "en")
	if !strings.HasSuffix(enPrompt, "write it in English.") {
		t.Fatalf("expected prompt to end with the English instruction, got: %q", enPrompt)
	}

	jaPrompt := buildExplainPrompt("日本語", "reference", "answer", "ja")
	if !strings.HasSuffix(jaPrompt, "write it in Japanese.") {
		t.Fatalf("expected prompt to end with the Japanese instruction, got: %q", jaPrompt)
	}
}
```

- [ ] **Step 2: Update `gemini_explainer_test.go` — pass a language argument to `Explain`**

In `TestGeminiExplainerExplainReturnsText`, change:
```go
	got, err := g.Explain(context.Background(), "japanese", "correct", "user")
```
to:
```go
	got, err := g.Explain(context.Background(), "japanese", "correct", "user", "en")
```

In `TestGeminiExplainerExplainPropagatesError`, change:
```go
	_, err := g.Explain(context.Background(), "japanese", "correct", "user")
```
to:
```go
	_, err := g.Explain(context.Background(), "japanese", "correct", "user", "en")
```

- [ ] **Step 3: Update `handlers_test.go` — fake explainer, existing bodies, and new language tests**

Change the `explainCall` struct and `fakeExplainer.Explain` (currently lines 58–73):
```go
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
```

In `TestExplainAnswerOK`, change the body and assertion:
```go
	body := `{"sentence_id":1,"user_answer":"I have no time.","language":"en"}`
```
```go
	call := explainer.calledWith[0]
	if call.japanese != "時間がありません。" || call.correctAnswer != "I don't have time." || call.userAnswer != "I have no time." || call.language != "en" {
		t.Fatalf("unexpected call args: %+v", call)
	}
```

In `TestExplainAnswerSentenceNotFound`, change the body:
```go
	body := `{"sentence_id":999,"user_answer":"x","language":"en"}`
```

In `TestExplainAnswerLLMError`, change the body:
```go
	body := `{"sentence_id":1,"user_answer":"z","language":"en"}`
```

Leave `TestExplainAnswerIgnoresClientSuppliedSentenceData`, `TestExplainAnswerEmptyUserAnswer`, `TestExplainAnswerUserAnswerTooLong`, and `TestExplainAnswerBodyTooLarge` unchanged — each already returns 400 before language would ever be checked (unknown-field rejection, empty/too-long answer, and oversized body all short-circuit first).

Add three new tests, right before `TestExplainAnswerMethodNotAllowed`:
```go
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
```

- [ ] **Step 4: Run tests to verify the package fails to compile**

Run: `cd api && go test ./internal/app/...`
Expected: FAIL — compile error, e.g. `not enough arguments in call to buildExplainPrompt` and/or `*fakeExplainer does not implement Explainer`.

- [ ] **Step 5: Implement `explainer.go`**

Replace the file's contents with:

```go
package app

import (
	"context"
	"fmt"
	"strings"
)

// Explainer generates a natural-language explanation comparing a learner's
// English translation to a reference translation of a Japanese sentence.
type Explainer interface {
	Explain(ctx context.Context, japanese, correctAnswer, userAnswer, language string) (string, error)
}

// validExplainLanguages is the allow-list of languages an explanation can be
// written in. Shared by request validation (handlers.go) and prompt
// building (buildExplainPrompt) as a single source of truth.
var validExplainLanguages = map[string]bool{
	"en": true,
	"ja": true,
}

func buildExplainPrompt(japanese, correctAnswer, userAnswer, language string) string {
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
	if language == "ja" {
		b.WriteString("Keep the explanation concise (2-4 sentences) and write it in Japanese.")
	} else {
		b.WriteString("Keep the explanation concise (2-4 sentences) and write it in English.")
	}
	return b.String()
}
```

- [ ] **Step 6: Implement `gemini_explainer.go`**

Change the `Explain` method (currently lines 40–52):
```go
func (g *GeminiExplainer) Explain(ctx context.Context, japanese, correctAnswer, userAnswer, language string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, explainTimeout)
	defer cancel()

	prompt := buildExplainPrompt(japanese, correctAnswer, userAnswer, language)
	contents := []*genai.Content{{Parts: []*genai.Part{{Text: prompt}}}}

	resp, err := g.models.GenerateContent(ctx, g.model, contents, nil)
	if err != nil {
		return "", fmt.Errorf("gemini generate content: %w", err)
	}
	return resp.Text(), nil
}
```

- [ ] **Step 7: Implement `sentence.go`**

Change `ExplainRequest` (currently lines 40–43):
```go
type ExplainRequest struct {
	SentenceID int    `json:"sentence_id"`
	UserAnswer string `json:"user_answer"`
	Language   string `json:"language"`
}
```

- [ ] **Step 8: Implement `handlers.go`**

In `explainAnswer`, insert a language check right after the existing `userAnswer` validation (currently lines 123–127) and before the `// The Japanese sentence...` comment:
```go
	userAnswer := strings.TrimSpace(req.UserAnswer)
	if userAnswer == "" || len(userAnswer) > maxUserAnswerLength {
		http.Error(w, "Invalid user_answer", http.StatusBadRequest)
		return
	}
	if !validExplainLanguages[req.Language] {
		http.Error(w, "Invalid language", http.StatusBadRequest)
		return
	}
```

Then change the `Explain` call (currently line 142):
```go
	explanation, err := s.explainer.Explain(r.Context(), japanese, correctAnswer, userAnswer, req.Language)
```

- [ ] **Step 9: Run tests to verify everything passes**

Run: `cd api && go test ./internal/app/... -v -run TestExplain`
Expected: PASS for all `TestExplain*` tests, including the three new ones.

Run: `cd api && go test ./...`
Expected: PASS — the full backend suite, confirming nothing else broke.

- [ ] **Step 10: Commit**

```bash
git add api/internal/app/explainer.go api/internal/app/gemini_explainer.go api/internal/app/sentence.go api/internal/app/handlers.go api/internal/app/explainer_test.go api/internal/app/gemini_explainer_test.go api/internal/app/handlers_test.go
git commit -m "feat(api): support language selection on the explain endpoint"
```

---

### Task 2: Frontend — add `language` to `api.explainAnswer`

**Files:**
- Modify: `fe/src/lib/api.ts`
- Test: `fe/src/lib/api.test.ts`

**Interfaces:**
- Consumes: none new (pure client wrapper around `POST /api/answer/explain`, matching Task 1's `ExplainRequest.Language` field).
- Produces: `api.explainAnswer(sentenceId: number, userAnswer: string, language: 'en' | 'ja') => Promise<ExplainResponse>` — the signature Task 3 will call from `Translator.tsx`.

- [ ] **Step 1: Update the existing test in `api.test.ts`**

Replace the `describe('api.explainAnswer', ...)` block (currently lines 82–99) with:
```ts
describe('api.explainAnswer', () => {
  it('sends POST /api/answer/explain with sentence_id, user_answer, and language', async () => {
    mockResponse({ explanation: 'Your answer is also natural.' })
    const result = await api.explainAnswer(1, 'I have no time.', 'en')
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/answer/explain'),
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          sentence_id: 1,
          user_answer: 'I have no time.',
          language: 'en',
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
Expected: FAIL — either a TypeScript arity error or (since `api.explainAnswer` currently ignores a 3rd argument) a body mismatch: actual body has no `language` key.

- [ ] **Step 3: Implement `api.ts`**

Change the `explainAnswer` entry in the `api` object (currently lines 62–66):
```ts
  explainAnswer: (sentenceId: number, userAnswer: string, language: 'en' | 'ja') =>
    request<ExplainResponse>('/api/answer/explain', {
      method: 'POST',
      body: JSON.stringify({ sentence_id: sentenceId, user_answer: userAnswer, language }),
    }),
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd fe && npx vitest run src/lib/api.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add fe/src/lib/api.ts fe/src/lib/api.test.ts
git commit -m "feat(fe): add language parameter to api.explainAnswer"
```

---

### Task 3: Frontend — EN/JA toggle in `Translator.tsx`

**Files:**
- Modify: `fe/src/components/Translator.tsx`
- Test: `fe/src/components/Translator.test.tsx`

**Interfaces:**
- Consumes: `api.explainAnswer(sentenceId: number, userAnswer: string, language: 'en' | 'ja') => Promise<ExplainResponse>` (Task 2).
- Produces: an `eagle:explainLanguage` value in `localStorage` (`'en'` or `'ja'`), and a visible EN/JA toggle rendered whenever the Explain button is rendered.

- [ ] **Step 1: Update `Translator.test.tsx` — existing call assertion, plus new toggle tests**

Add `localStorage.clear()` to the top-level `beforeEach` (currently lines 40–43):
```ts
beforeEach(() => {
  vi.clearAllMocks()
  mockApi.getRandomSentence.mockResolvedValue(fakeSentence)
  localStorage.clear()
})
```

In the test `'calls api.explainAnswer with the sentence id and user answer'` (currently lines 88–99), change the assertion:
```ts
    expect(mockApi.explainAnswer).toHaveBeenCalledWith(fakeSentence.id, 'I have no time.', 'en')
```

Add four new tests at the end of the `describe('Explain button', ...)` block, right before its closing `})` (currently line 148):
```ts
  it('shows an EN/JA language toggle next to the Explain button', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    await answerIncorrectly()
    expect(screen.getByRole('button', { name: 'EN' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'JA' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'EN' })).toHaveAttribute('aria-pressed', 'true')
  })

  it('does not call api.explainAnswer when switching language before explaining', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    await answerIncorrectly()
    fireEvent.click(screen.getByRole('button', { name: 'JA' }))
    expect(mockApi.explainAnswer).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'JA' })).toHaveAttribute('aria-pressed', 'true')
  })

  it('re-fetches the explanation in the new language when the toggle is switched after explaining', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    mockApi.explainAnswer.mockResolvedValueOnce({ explanation: 'English explanation.' })
    await answerIncorrectly()
    fireEvent.click(screen.getByRole('button', { name: /^explain$/i }))
    await screen.findByText('English explanation.')

    mockApi.explainAnswer.mockResolvedValueOnce({ explanation: '日本語の説明。' })
    fireEvent.click(screen.getByRole('button', { name: 'JA' }))
    await screen.findByText('日本語の説明。')

    expect(mockApi.explainAnswer).toHaveBeenLastCalledWith(fakeSentence.id, 'I have no time.', 'ja')
  })

  it('persists the selected language to localStorage and restores it on remount', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    const { unmount } = render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    fireEvent.change(screen.getByLabelText(/your english translation/i), {
      target: { value: 'I have no time.' },
    })
    fireEvent.click(screen.getByRole('button', { name: /check translation/i }))
    await screen.findByText(/not quite right/i)
    fireEvent.click(screen.getByRole('button', { name: 'JA' }))
    expect(localStorage.getItem('eagle:explainLanguage')).toBe('ja')
    unmount()

    render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    fireEvent.change(screen.getByLabelText(/your english translation/i), {
      target: { value: 'I have no time.' },
    })
    fireEvent.click(screen.getByRole('button', { name: /check translation/i }))
    await screen.findByText(/not quite right/i)
    expect(screen.getByRole('button', { name: 'JA' })).toHaveAttribute('aria-pressed', 'true')
  })
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd fe && npx vitest run src/components/Translator.test.tsx`
Expected: FAIL — no element with role `button` and name `EN`/`JA` exists yet, and the updated call assertion is missing the third argument.

- [ ] **Step 3: Implement `Translator.tsx`**

Add a module-level constant, right after the imports (after line 20):
```ts
const EXPLAIN_LANGUAGE_STORAGE_KEY = 'eagle:explainLanguage'
```

Add new state, right after the existing `explainError` state (currently line 40):
```ts
  const [explainLanguage, setExplainLanguage] = useState<'en' | 'ja'>(() => {
    const stored = localStorage.getItem(EXPLAIN_LANGUAGE_STORAGE_KEY)
    return stored === 'ja' ? 'ja' : 'en'
  })
```

Change `explainAnswer` to take the language explicitly (currently lines 103–115):
```ts
  const explainAnswer = async (language: 'en' | 'ja') => {
    if (!currentSentence) return
    setExplaining(true)
    setExplainError(null)
    try {
      const result = await api.explainAnswer(currentSentence.id, userTranslation, language)
      setExplanation(result.explanation)
    } catch (err) {
      setExplainError(err instanceof Error ? err.message : 'Failed to load explanation')
    } finally {
      setExplaining(false)
    }
  }

  const selectExplainLanguage = (language: 'en' | 'ja') => {
    setExplainLanguage(language)
    localStorage.setItem(EXPLAIN_LANGUAGE_STORAGE_KEY, language)
    if (explanation) {
      explainAnswer(language)
    }
  }
```

Update the Explain button's `onClick` and add the toggle beside it — this replaces the entire `feedback === 'incorrect' && (...)` block (currently lines 350–376):
```tsx
                  {feedback === 'incorrect' && (
                    <div className="space-y-2">
                      <div className="flex items-center gap-2">
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => explainAnswer(explainLanguage)}
                          disabled={explaining}
                        >
                          {explaining ? 'Explaining...' : 'Explain'}
                        </Button>

                        <div className="flex gap-1" role="group" aria-label="Explanation language">
                          <Button
                            type="button"
                            variant={explainLanguage === 'en' ? 'default' : 'outline'}
                            size="sm"
                            aria-pressed={explainLanguage === 'en'}
                            onClick={() => selectExplainLanguage('en')}
                          >
                            EN
                          </Button>
                          <Button
                            type="button"
                            variant={explainLanguage === 'ja' ? 'default' : 'outline'}
                            size="sm"
                            aria-pressed={explainLanguage === 'ja'}
                            onClick={() => selectExplainLanguage('ja')}
                          >
                            JA
                          </Button>
                        </div>
                      </div>

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

Leave `nextSentence()` (lines 158–173) unchanged — it must **not** reset `explainLanguage`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd fe && npx vitest run src/components/Translator.test.tsx`
Expected: PASS — all existing and new tests in this file.

Run: `cd fe && npm test`
Expected: PASS — the full frontend suite, confirming nothing else broke.

- [ ] **Step 5: Type-check the frontend**

Run: `cd fe && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add fe/src/components/Translator.tsx fe/src/components/Translator.test.tsx
git commit -m "feat(fe): add EN/JA toggle for explanation language"
```

---

## Manual verification (after all tasks)

Once both backend and frontend changes are in, verify the feature end-to-end before considering this done:

1. Start the backend (`cd api && go run .`, with `GEMINI_API_KEY` set) and the frontend (`cd fe && npm run dev`).
2. Sign in, submit an incorrect translation, click **Explain** with **EN** selected (the default) — confirm the explanation renders in English.
3. Click **JA** — confirm the explanation is immediately re-fetched and renders in Japanese, and the Explain button briefly shows its loading state during the re-fetch.
4. Reload the page, answer another sentence incorrectly — confirm **JA** is still the selected toggle (persisted).
5. Click **Explain** with **JA** already selected, before ever having shown an explanation for this sentence — confirm it fetches directly in Japanese (no English flash).
