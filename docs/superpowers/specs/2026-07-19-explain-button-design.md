# Explain Button — Design Spec

## Problem

When a user submits an incorrect English translation, Eagle shows the one
reference "correct answer" stored in Firestore. That reference is just one
valid phrasing — the user's answer may in fact be natural, acceptable
English that simply differs in tone, formality, or word choice. Users have
no way to understand *why* their answer was marked wrong, or whether it was
really wrong at all.

## Goal

Add an "Explain" button that, on click, sends the Japanese sentence, the
stored reference answer, and the user's submitted answer to an LLM, and
displays back an explanation of the differences — including telling the
user when their answer is actually fine and the reference was just one of
several valid options.

## Non-goals

- No chat/conversation UI — this is a single-shot, on-demand explanation,
  not an ongoing dialogue.
- No streaming response — corgi's chat feature streams because it's a long
  multi-turn conversation; this is a short one-off answer, so a simple
  request/response is enough.
- No persistence of explanations — they are not stored in Firestore or
  shown again if the user revisits the sentence later.
- No explanation for correct answers — the button only appears when the
  user's answer was marked incorrect, since that's the only case where an
  explanation is needed.

## Architecture / data flow

1. User submits a translation via the existing `checkAnswer` flow. If
   incorrect, `showAnswer` becomes true and `feedback === 'incorrect'` as
   today — unchanged.
2. A new **Explain** button appears next to the "Correct Answer" block,
   only in that state.
3. On click, the frontend calls `POST /api/answer/explain` with
   `{ japanese, correct_answer, user_answer }` — values already held in
   frontend state, no additional Firestore lookup required.
4. The Go backend builds a prompt from those three strings and makes a
   single non-streaming call to Gemini, following the same
   single-shot-call pattern as corgi's `GeminiTitleGenerator` (not its
   streaming chat provider, which doesn't fit a one-off explanation).
5. The backend returns `{ explanation: string }`. The frontend shows a
   loading spinner while waiting, then renders the explanation text. On
   failure, it shows an error message and leaves the button clickable so
   the user can retry manually (no automatic retry).

## Backend design (`eagle/api`, Go)

### New files

**`explainer.go`**

```go
type Explainer interface {
    Explain(ctx context.Context, japanese, correctAnswer, userAnswer string) (string, error)
}

func buildExplainPrompt(japanese, correctAnswer, userAnswer string) string
```

`buildExplainPrompt` is a pure function, kept separate from the Gemini
client so it is unit-testable without network access. The prompt instructs
the model to:
- Treat `correctAnswer` as *one* valid reference translation, not the only
  correct one.
- Judge whether `userAnswer` is also natural, acceptable English on its
  own merits.
- If the user's answer is acceptable, say so plainly and explain the
  nuance/formality/register difference from the reference rather than
  implying the user was wrong.
- If the user's answer has an actual grammar/vocabulary/meaning error,
  explain what's wrong and why the reference is more correct.
- Write the explanation in English.

**`gemini_explainer.go`**

```go
type GeminiExplainer struct {
    client *genai.Client
    model  string
}

func NewGeminiExplainer(ctx context.Context, apiKey string) (*GeminiExplainer, error)
func (g *GeminiExplainer) Explain(ctx context.Context, japanese, correctAnswer, userAnswer string) (string, error)
```

- Uses `google.golang.org/genai`, Google's current unified Go SDK (the
  older `github.com/google/generative-ai-go/genai` is deprecated). This
  differs from corgi's Node SDK (`@google/generative-ai`) because eagle's
  backend is Go, but follows the same configuration shape: an API key from
  an env var, and a model name constant.
- Env var: `GEMINI_API_KEY` (same name as corgi, for consistency).
- Model: `gemini-2.5-flash` — one step up from corgi's title-generator
  model (`gemini-2.5-flash-lite`), since this task requires actual
  reasoning about grammar/nuance rather than short summarization.
- Each call is wrapped in a ~20s context timeout. No retry/backoff logic,
  matching corgi's approach of not retrying LLM calls.
- On error, the error is returned to the caller as-is (wrapped with
  context) — no silent fallback text. Unlike corgi's title generator,
  which degrades gracefully to a truncated string because a title is
  low-stakes, a failed explanation has no meaningful fallback — the
  feature simply didn't work, and the frontend must surface that.

### Modified files

- **`sentence.go`**: add request/response types:
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
- **`handlers.go`**: add `explainAnswer` handler on `Server`, following the
  same shape as `checkAnswer` — POST only, decode JSON body, call
  `s.explainer.Explain(r.Context(), ...)`, log + 500 on error, otherwise
  `writeJSON`. `Server` gains an `explainer Explainer` field, set via
  `NewServer(repo, explainer)`.
- **`main.go`**: construct `GeminiExplainer` from `GEMINI_API_KEY` at
  startup, pass into `NewServer`, and register
  `mux.HandleFunc("/api/answer/explain", auth(srv.explainAnswer))`.
- **`.env.example`**: add `GEMINI_API_KEY=`.

### Tests (TDD: red before green)

- `handlers_test.go`: add a fake `Explainer` (implements the interface
  in-test, matching how the existing fake repository is used elsewhere in
  this file) to test `explainAnswer`'s request decoding, response
  encoding, and error-to-status-code mapping — without calling Gemini.
- `explainer_test.go` (new): unit tests for `buildExplainPrompt` asserting
  the three inputs are present in the built prompt, and that the
  instruction to judge the user's answer on its own merits (not just diff
  against the reference) is present.
- No test calls the real Gemini API.

## Frontend design (`eagle/fe`)

### Modified files

- **`src/lib/api.ts`**: add
  ```ts
  export interface ExplainResponse {
    explanation: string
  }
  ```
  and
  ```ts
  explainAnswer: (japanese: string, correctAnswer: string, userAnswer: string) =>
    request<ExplainResponse>('/api/answer/explain', {
      method: 'POST',
      body: JSON.stringify({ japanese, correct_answer: correctAnswer, user_answer: userAnswer }),
    }),
  ```

- **`src/components/Translator.tsx`**:
  - New state: `explanation: string | null`, `explaining: boolean`,
    `explainError: string | null`.
  - New `explainAnswer()` function: sets `explaining = true`, clears any
    prior `explainError`, calls
    `api.explainAnswer(currentSentence.japanese, currentSentence.english, userTranslation)`,
    stores the result in `explanation` or the error message in
    `explainError`, then clears `explaining`.
  - New "Explain" `Button`, rendered only when
    `showAnswer && feedback === 'incorrect'`, placed near the existing
    "Correct Answer" block. Disabled and shows a loading indicator while
    `explaining`.
  - When `explanation` is set, render it in its own panel/card below the
    correct-answer block.
  - When `explainError` is set, render an `Alert` with the error message;
    the Explain button remains enabled so the user can retry.
  - `nextSentence()` resets `explanation`, `explaining`, and
    `explainError` alongside its existing state resets.

### Tests (TDD: red before green)

New `src/components/Translator.test.tsx` (no test file currently exists
for this component), mocking `api.explainAnswer` the same way other
component tests in this repo mock the API module:
- Explain button is not rendered when the answer hasn't been checked yet.
- Explain button is not rendered when the answer was correct.
- Explain button is rendered when the answer was incorrect.
- Clicking the button calls `api.explainAnswer` with the Japanese
  sentence, reference answer, and the user's submitted answer.
- A loading state is shown while the call is pending.
- The explanation is rendered once the call resolves.
- An error state is rendered if the call rejects, and the button remains
  clickable to retry.

## Open items for implementation

None — all decisions above were confirmed during design review.
