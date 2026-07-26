# Explanation Language Toggle — Design Spec

## Problem

The "Explain" button (see `2026-07-19-explain-button-design.md`) always
returns its explanation in English, hardcoded in the Gemini prompt. Eagle's
users are Japanese speakers learning English — some would understand an
explanation of their mistake more easily if it were written in Japanese
instead. There is currently no way to choose.

## Goal

Let the user toggle the explanation's language between English and
Japanese, next to the Explain button. The choice persists as a standing
preference across sentences and sessions.

## Non-goals

- No general app-wide UI localization (menus, labels, buttons elsewhere in
  the app stay English-only). This only affects the language the AI writes
  the explanation in.
- No support for languages other than English and Japanese.
- No server-side persistence of the language preference — it lives in the
  browser's `localStorage` only, same tier as other client-only UI state in
  this app today.

## Architecture / data flow

1. `Translator.tsx` gains new state: `explainLanguage: 'en' | 'ja'`,
   lazily initialized from `localStorage` (key `eagle:explainLanguage`),
   defaulting to `'en'` if unset or not one of the two valid values.
2. When `showAnswer && feedback === 'incorrect'`, a small EN/JA toggle
   renders next to the existing Explain button — visible even before the
   first explanation is requested, so the user can pick beforehand.
3. Toggling calls a handler that writes the new value to `localStorage`,
   updates state, and — if an `explanation` is already displayed —
   immediately re-triggers the explain call with the new language,
   replacing the shown text (going through the same `explaining` /
   `explainError` loading states as a normal click). If no explanation is
   shown yet, toggling only updates the selection.
4. `explainAnswer` takes the language as an explicit argument (not read
   from closure) so both the Explain-button click and the toggle-flip both
   pass the current language directly, avoiding any stale-closure/timing
   issue with React state batching.
5. The frontend calls `POST /api/answer/explain` with `language` added to
   the existing body. The Go backend validates it against an allow-list,
   appends a language-specific instruction line to the existing prompt, and
   makes the same single non-streaming Gemini call as today.
6. `nextSentence()` continues to reset `explanation` / `explaining` /
   `explainError` as it does today, but does **not** reset
   `explainLanguage` — it is a standing preference, not per-sentence state.

## Backend design (`eagle/api`, Go)

### Modified files

- **`sentence.go`**: extend `ExplainRequest`:
  ```go
  type ExplainRequest struct {
      SentenceID int    `json:"sentence_id"`
      UserAnswer string `json:"user_answer"`
      Language   string `json:"language"`
  }
  ```
  `ExplainResponse` is unchanged.

- **`explainer.go`**:
  - Add an allow-list used both for request validation and prompt building:
    ```go
    var validExplainLanguages = map[string]bool{"en": true, "ja": true}
    ```
  - `Explainer.Explain` gains a `language` parameter:
    ```go
    type Explainer interface {
        Explain(ctx context.Context, japanese, correctAnswer, userAnswer, language string) (string, error)
    }
    ```
  - `buildExplainPrompt(japanese, correctAnswer, userAnswer, language string) string`
    replaces the current hardcoded final line
    (`"Keep the explanation concise (2-4 sentences) and write it in English."`)
    with a language-driven line: `"...and write it in English."` for `"en"`,
    `"...and write it in Japanese."` for `"ja"`.

- **`gemini_explainer.go`**: `GeminiExplainer.Explain` signature updated to
  accept and forward `language` to `buildExplainPrompt`. No other change —
  language is pure prompt text, not a Gemini API/config concern.

- **`handlers.go`**: in `explainAnswer`, after decoding the request and
  validating `user_answer` as today, validate `req.Language` against
  `validExplainLanguages` — empty or unrecognized value returns 400
  `"Invalid language"`, mirroring the existing `user_answer` validation
  style. The validated language is passed through to
  `s.explainer.Explain(...)`.

### Tests (TDD: red before green)

- `explainer_test.go`: extend `buildExplainPrompt` tests to assert the
  prompt ends with the English instruction line when `language == "en"`
  and the Japanese instruction line when `language == "ja"`.
- `handlers_test.go`: extend the fake `Explainer` to capture the
  `language` argument it received.
  - Request with `language: "ja"` is accepted and forwarded to the fake
    explainer unchanged.
  - Request with missing or invalid `language` (e.g. `""`, `"fr"`) returns
    400 `"Invalid language"` and does not call the explainer.
- `gemini_explainer_test.go`: update existing test(s) for the new
  `Explain` signature (compile-level change; pure pass-through, no new
  behavior to assert beyond the arg being forwarded).
- No test calls the real Gemini API.

## Frontend design (`eagle/fe`)

### Modified files

- **`src/lib/api.ts`**:
  ```ts
  explainAnswer: (sentenceId: number, userAnswer: string, language: 'en' | 'ja') =>
    request<ExplainResponse>('/api/answer/explain', {
      method: 'POST',
      body: JSON.stringify({ sentence_id: sentenceId, user_answer: userAnswer, language }),
    }),
  ```
  `ExplainResponse` is unchanged.

- **`src/components/Translator.tsx`**:
  - New state: `explainLanguage: 'en' | 'ja'`, lazily initialized from
    `localStorage.getItem('eagle:explainLanguage')`, falling back to
    `'en'` when missing or invalid.
  - `explainAnswer` takes an explicit `language: 'en' | 'ja'` argument
    (rather than reading `explainLanguage` from closure) — sets
    `explaining = true`, clears `explainError`, calls
    `api.explainAnswer(currentSentence.id, userTranslation, language)`,
    stores the result or error, then clears `explaining`.
  - New `setLanguage(lang: 'en' | 'ja')` handler: persists `lang` to
    `localStorage`, updates `explainLanguage` state, and if
    `explanation !== null`, calls `explainAnswer(lang)` to refresh the
    displayed text in the new language.
  - New EN/JA toggle (two small `Button`s, active one highlighted via the
    existing `variant` prop convention) rendered next to the Explain
    button, only when `showAnswer && feedback === 'incorrect'`.
  - The Explain button's `onClick` calls `explainAnswer(explainLanguage)`.
  - `nextSentence()` is unchanged with respect to `explainLanguage` — it
    resets `explanation`, `explaining`, and `explainError` as today, but
    leaves the language preference untouched.

### Tests (TDD: red before green)

Extend `src/components/Translator.test.tsx`:
- EN/JA toggle renders next to the Explain button once
  `showAnswer && feedback === 'incorrect'`.
- Default language is `'en'` when `localStorage` is empty; clicking
  Explain calls `api.explainAnswer` with `'en'`.
- Clicking JA before any explanation exists updates the toggle's visual
  state and does not call `api.explainAnswer`.
- Clicking JA after an explanation is already shown immediately re-calls
  `api.explainAnswer` with `'ja'` and replaces the shown explanation once
  it resolves.
- The chosen language is persisted to `localStorage`, and a fresh mount
  with a pre-set `localStorage` value initializes `explainLanguage` (and
  the first Explain call) from it.

No test hits the real Gemini API or relies on anything beyond jsdom's
built-in `localStorage`.

## Open items for implementation

None — all decisions above were confirmed during design review.
