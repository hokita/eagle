# Weakness Insight — Design Spec

## Problem

The mistakes page (`/mistakes`, `GET /api/mistakes`) lists every sentence a
learner has answered incorrectly — the Japanese, the reference English
answer, and their past wrong attempts. That's useful raw material, but the
learner still has to interpret it themselves: are these mistakes random, or
is there a pattern underneath them? Nothing on the page tells them *what*
they keep getting wrong.

## Goal

Add an AI-generated **weakness insight** to the top of the mistakes page. On
page load it analyzes the learner's recent mistakes and reports their main
recurring weaknesses — grammar structures, vocabulary gaps, register — in a
short summary plus a few actionable areas. It deliberately looks past one-off
slips: typos, spelling mistakes, and isolated misreadings that don't indicate
a real pattern. It uses the same Gemini model and single-shot-call pattern as
the existing Explain button.

## Non-goals

- **No structured JSON output.** The insight is a single plain-text field
  (`{ insight: string }`), rendered `whitespace-pre-wrap`, matching the
  Explain button's simple contract. No typed `weaknesses[]` array driving
  per-weakness UI — revisit only if distinct per-weakness treatment is
  wanted later.
- **No persistence / caching.** The insight is not stored in Firestore and
  is regenerated on each page visit, consistent with the Explain button's
  no-persistence decision. Caching is a clean later addition if regeneration
  cost or latency becomes a concern.
- **No streaming.** Like Explain, this is a single-shot request/response,
  not a chat.
- **No new language toggle.** The insight reuses the learner's existing
  explanation-language preference (`localStorage['eagle:explainLanguage']`,
  default `en`) rather than adding a separate control.
- **No server-side aggregation index.** The endpoint reuses the existing
  `ListMistakes` fan-out read; no precomputed mistakes index. Acceptable at
  current scale, matching the mistakes-page spec's stance.

## Architecture / data flow

1. The mistakes page loads its list as today (`GET /api/mistakes`) and
   renders it immediately — unchanged.
2. If the list is non-empty, the frontend auto-calls a new endpoint
   `GET /api/mistakes/insight?language=<en|ja>` (auth-gated, like every
   route). The raw list is never blocked on this call; the insight card
   shows its own loading state.
3. The Go handler loads the learner's mistakes **server-side** via
   `s.repo.ListMistakes(uid)` — never trusting mistake text from the client,
   the same security principle the Explain handler follows — and caps the
   set to the 50 most recent (the list is already sorted newest-missed
   first).
4. If the learner has no mistakes, the handler returns `{ insight: "" }`
   **without** calling Gemini.
5. Otherwise it builds a prompt from the capped mistakes via a pure
   `buildWeaknessPrompt` function and makes a single non-streaming Gemini
   call, following the same pattern as `GeminiExplainer`.
6. The backend returns `{ insight: string }`. The frontend renders it in a
   card above the list. On failure it shows an error with a retry button
   that stays clickable (matching Explain).

## Backend design (`eagle/api`, Go)

### New files

**`analyzer.go`**

```go
type WeaknessAnalyzer interface {
    Analyze(ctx context.Context, mistakes []MistakeSentence, language string) (string, error)
}

func buildWeaknessPrompt(mistakes []MistakeSentence, language string) string
```

`buildWeaknessPrompt` is a pure function, kept separate from the Gemini
client so it is unit-testable without network access — mirroring
`buildExplainPrompt`. It reuses the existing `validExplainLanguages`
allow-list (`en`, `ja`) as the single source of truth for supported
languages. The prompt instructs the model to:

- Read all the provided mistakes together (each: Japanese sentence,
  reference English answer, the learner's wrong attempts).
- Identify the learner's **recurring, systematic weaknesses** — e.g. verb
  tense/agreement, articles, plurals, prepositions, word order, vocabulary
  and register gaps.
- **Explicitly disregard** one-off typos, spelling slips, and isolated
  misreadings that don't indicate a pattern.
- Produce a short summary (1–2 sentences) followed by a few bulleted
  weakness areas, each with a brief actionable note.
- Write the analysis in the requested language (`ja` → Japanese, otherwise
  English), matching how `buildExplainPrompt` handles language.

**`gemini_analyzer.go`**

```go
type GeminiWeaknessAnalyzer struct {
    models contentGenerator
    model  string
}

func NewGeminiWeaknessAnalyzer(ctx context.Context, apiKey string) (*GeminiWeaknessAnalyzer, error)
func (g *GeminiWeaknessAnalyzer) Analyze(ctx context.Context, mistakes []MistakeSentence, language string) (string, error)
```

- Reuses the same `google.golang.org/genai` SDK, the shared
  `contentGenerator` test seam (already defined in `gemini_explainer.go`),
  the same model constant (`gemini-3.1-flash-lite`), and the same ~20s
  context timeout as `GeminiExplainer`.
- Env var: `GEMINI_API_KEY` (already required at startup for Explain — no
  new secret).
- No retry/backoff; on error the error is returned wrapped, no fallback
  text (matching `GeminiExplainer`).

### Modified files

- **`sentence.go`**: add the response type and extend the analyzer surface —
  ```go
  type MistakesInsightResponse struct {
      Insight string `json:"insight"`
  }
  ```
  (`MistakeSentence` already exists and is reused as the analyzer input; no
  new request body type — the endpoint takes only the `language` query
  param.)

- **`handlers.go`**: add a `getMistakesInsight` handler on `Server`:
  - `GET` only; read `uid` from context (same as `getMistakes`).
  - Read and validate the `language` query param against
    `validExplainLanguages`; 400 on an unsupported value.
  - Call `s.repo.ListMistakes(r.Context(), uid)`; on error log + 500.
  - If the result is empty, `writeJSON(w, MistakesInsightResponse{Insight: ""})`
    and return — no Gemini call.
  - Otherwise cap to the `maxInsightMistakes` (50) most recent, call
    `s.analyzer.Analyze(...)`, log + 500 on error, else `writeJSON`.
  - `Server` gains a `analyzer WeaknessAnalyzer` field, set via
    `NewServer(repo, explainer, analyzer)`.
  - Add `maxInsightMistakes = 50` alongside the existing size constants.

- **`router.go`**: register
  `mux.HandleFunc("/api/mistakes/insight", auth(srv.getMistakesInsight))`.

- **`main.go`**: construct `GeminiWeaknessAnalyzer` from `GEMINI_API_KEY` at
  startup (the key is already read for the explainer), pass it into
  `NewServer`.

- **`cmd/e2eserver/main.go`**: add a `stubAnalyzer` (mirroring
  `stubExplainer`) returning a fixed insight string, and pass it into
  `NewServer`, so e2e/CI never call the real Gemini API.

### Tests (TDD: red before green)

- `analyzer_test.go` (new): unit tests for `buildWeaknessPrompt` asserting
  the mistakes' Japanese/reference/wrong-answer text appears in the built
  prompt, that the "focus on recurring patterns" and "ignore typos /
  one-off slips" instructions are present, and that `language == "ja"`
  requests a Japanese-language analysis.
- `handlers_test.go`: add a fake `WeaknessAnalyzer` (implementing the
  interface in-test, matching the existing fake repository / fake explainer
  pattern) to test `getMistakesInsight`:
  - empty mistakes → `{ insight: "" }` and the analyzer is **not** called;
  - a `ListMistakes` repo error → 500;
  - an analyzer error → 500;
  - a populated result → response shape matches `MistakesInsightResponse`;
  - only the `maxInsightMistakes` most recent are passed to the analyzer
    when more exist;
  - an unsupported `language` query value → 400.
- No test calls the real Gemini API.

## Frontend design (`eagle/fe`)

### Modified files

- **`src/lib/api.ts`**: add
  ```ts
  export interface MistakesInsightResponse {
    insight: string
  }
  ```
  and
  ```ts
  getMistakesInsight: (language: 'en' | 'ja') =>
    request<MistakesInsightResponse>(`/api/mistakes/insight?language=${language}`),
  ```

- **`src/components/Mistakes.tsx`**:
  - New state: `insight: string | null`, `insightLoading: boolean`,
    `insightError: string | null`.
  - Read the language preference from
    `localStorage['eagle:explainLanguage']` (default `'en'`), the same key
    the Translator's Explain toggle persists.
  - A `loadInsight()` function: sets `insightLoading = true`, clears any
    prior `insightError`, calls `api.getMistakesInsight(language)`, stores
    the result in `insight` or the message in `insightError`, then clears
    `insightLoading`.
  - After the mistakes list successfully loads, if it is **non-empty**, call
    `loadInsight()`. Skip the call entirely when the list is empty.
  - Render an **insight card above the list**: while `insightLoading`, a
    compact loading state; on success, the insight text
    (`whitespace-pre-wrap`) in a card styled to match the page (reusing the
    existing `Card` primitives, with the same accent language as the app);
    on `insightError`, an inline error with a retry button that calls
    `loadInsight()` again. When there are no mistakes (empty state), no
    insight card is shown.

### Tests (TDD: red before green)

- `src/lib/api.test.ts`: `getMistakesInsight('en')` hits
  `GET /api/mistakes/insight?language=en` and returns the parsed insight.
- `src/components/Mistakes.test.tsx`:
  - the insight is fetched after the list loads, and its text renders in a
    card above the list;
  - a loading state is shown while the insight call is pending;
  - an error state renders with a working retry button if the insight call
    rejects (the list itself still renders);
  - no insight call is made and no insight card renders when the mistakes
    list is empty.

## Open items for implementation

None — all decisions above were confirmed during design review (auto-on-load
trigger, most-recent-50 scope, plain-text output, shared language
preference, and the prompt's focus-on-patterns / ignore-typos instruction).
