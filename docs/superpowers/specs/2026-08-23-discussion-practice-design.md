# Discussion Practice — Design

Date: 2026-08-23
Status: Approved design, pending implementation plan

## Overview

A new learning mode for Eagle: AI-guided discussion practice. Unlike the
existing translation drill (Japanese sentence → English translation), this
mode helps learners express their own thoughts directly in English, then uses
Japanese strategically to reveal the gap between what they can think and what
they can express.

### Learning philosophy (product principle)

The app is not teaching users to produce perfect English sentences. It reduces
the gap between *what I can think* and *what I can express in English*.
Japanese is a supporting tool to reveal that gap — never the source language
for translation. When an implementation decision conflicts with this
principle, the principle wins.

Concretely:

- The AI never corrects, rewrites, or feeds ideas during the conversation.
- All teaching is deferred to a dedicated study phase after the conversation.
- Gap analysis compares ideas and intentions, not sentences.
- Taught expressions are reusable spoken chunks (2–4 per session), slightly
  above the learner's level.
- The learner immediately re-uses new expressions by retrying the original
  question.

### Session flow

```
1. answering     Present a curated discussion question; user answers in English
2. conversation  AI asks 2–5 follow-up questions in English (no corrections)
3. reflection    「日本語で答えるなら、他に言いたかったことはありますか？」— user
                 answers freely in Japanese (or skips)
4. studying      Gap analysis: expressed ideas vs missing ideas + 2–4 useful
                 expressions taught
5. retry         The original question again; learned expressions shown as
                 reference chips
6. comparison    Before/after answers side by side + AI feedback on the retry;
                 session saved to history
```

Skipping the reflection (nothing to add) jumps straight to retry with no
analyze call — no gap data means nothing to teach.

## Decisions made during brainstorming

| Decision | Choice |
| --- | --- |
| Input modality | Text only for MVP; voice later |
| Question bank | ~30 curated questions seeded from NDJSON, random pick, no topic picker UI |
| Conversation end | LLM decides within 2–5 follow-ups; server hard-caps at 5 AI turns; user can finish anytime |
| History scope | Completed sessions list + inline detail view; no expression-lifecycle tracking yet |
| Mid-session durability | Client-held state; reload loses the in-progress session; only completed sessions persist |
| Architecture | Client-driven state machine + stateless per-phase API endpoints (Approach A) |

## Data model

### `discussion_questions` (top-level collection, seeded)

| Field | Type | Notes |
| --- | --- | --- |
| question_en | string | The discussion question |
| topic | string | e.g. "environment", "work", "technology" |
| level | int | 1–5, same scale as `sentences.level` (not CEFR strings) |
| target_skills | []string | e.g. ["giving opinions", "comparing options"] |
| is_active | bool | Inactive questions are never served |
| created_at / updated_at | string | Same convention as `sentences` |

`follow_up_hint` from the original sketch is dropped (YAGNI): the follow-up
prompt already receives the full transcript plus `target_skills`.

### `users/{uid}/discussion_sessions/{sessionId}` (auto-ID, written once on completion)

| Field | Type | Notes |
| --- | --- | --- |
| question_id | string | Reference to the curated question |
| question_en, topic | string | Denormalized so history renders without joins |
| transcript | [{role, text}] | role is "user" or "ai"; the English conversation in order |
| reflection_ja | string | Japanese reflection ("" when skipped) |
| expressed_ideas | []string | Ideas the learner managed to express in English |
| missing_ideas | []string | Ideas present in Japanese but absent from the English conversation |
| expressions | [{phrase, meaning_ja, example_en}] | The 2–4 taught expressions |
| first_answer | string | Convenience copy of transcript[0] for the history list |
| retry_answer | string | The post-study answer |
| retry_feedback | string | Short AI comment on the retry |
| created_at | timestamp | |

### Seeding

- `docs/discussion_questions_seed.ndjson`: ~30 questions drafted across the
  topic list (daily life, work, technology/AI, education, environment,
  society, travel, culture, career, business — roughly 3 each, levels 2–4),
  reviewed by the owner before seeding.
- New `api/cmd/seedquestions` command modeled on `cmd/seed`: validates rows
  (level 1–5, non-empty question/topic) and writes to Firestore.

## API

All endpoints go through the existing `auth()` wrapper in `router.go`
(CORS + Firebase token verification + email allowlist).

### `GET /api/discussion/question`

Random active question: `{id, question_en, topic, level, target_skills}`.
`404` when the bank is empty (same pattern as `ErrNoCandidate`).

### `POST /api/discussion/reply`

Request: `{question_id, transcript: [{role, text}]}`. The server fetches the
question text by ID — the client never supplies it (same trust model as
`sentence_id`).

Response: `{done: bool, message: string}` — the next follow-up question, or
`done: true` with a short closing line.

Flow control is two-layered: the prompt tells Gemini to wrap up after 2–5
follow-ups and return `done`; the server hard-caps regardless — if the
transcript already contains 5 AI turns it returns `done` without calling
Gemini. The client's "Finish conversation" button skips ahead locally with no
API call.

### `POST /api/discussion/analyze`

Request: `{question_id, transcript, reflection_ja}`.

Response:
`{expressed_ideas: [...], missing_ideas: [...], expressions: [{phrase, meaning_ja, example_en}]}`
with 2–4 expressions, enforced by Gemini `responseSchema` plus server-side
validation (truncate to 4; error if fewer than 1 valid).

### `POST /api/discussion/complete`

Request (stateless API, so the client sends back everything it accumulated):
`{question_id, transcript, reflection_ja, expressed_ideas, missing_ideas, expressions, retry_answer}`.

Response: `{session_id, retry_feedback}` — one Gemini call producing 2–3
sentences: which taught expressions the retry used and what improved versus
the first answer. Never a rewrite. The session document is then written to
Firestore.

### `GET /api/discussion/sessions`

Newest-first list capped at 50: `[{id, question_en, topic, created_at}]`.

### `GET /api/discussion/sessions/{id}`

Full session document. `404` for missing sessions; per-user isolation is
structural (sessions live under `users/{uid}`).

## Backend architecture

Follows the existing seam pattern (`SentenceRepository`, `Explainer`,
`WeaknessAnalyzer`):

- **`DiscussionRepository`** interface: `RandomQuestion`, `GetQuestion`,
  `SaveSession`, `ListSessions`, `GetSession` — implemented on the existing
  `firestoreRepo`.
- **`DiscussionCoach`** interface: `Reply`, `AnalyzeGap`, `ReviewRetry` —
  implemented by `GeminiCoach` using `gemini-3.1-flash-lite` with JSON
  response schemas. If gap-analysis quality disappoints, only that call is
  upgraded to a bigger model later.
- Prompts are pure `build*Prompt` functions (unit-testable without network,
  like `buildWeaknessPrompt`).

### Prompt principles

- **Reply prompt**: friendly discussion partner; ONE short follow-up per
  turn, drawn from patterns like "Why do you think so?", "Can you give me an
  example?", "What about the opposite opinion?". Forbidden: grammar
  correction, model answers, feeding ideas, asking for Japanese.
- **Analyze prompt**: compare ideas/intentions between the English transcript
  and the Japanese reflection — not literal sentence matching. Expressions
  must be reusable spoken chunks (prefer "take responsibility for" over
  "responsibility"), directly tied to something the learner wanted to say,
  slightly above the question's level. `meaning_ja` in Japanese.
- **Retry prompt**: encouragement plus usage feedback (which taught
  expressions appeared, what improved). No rewrite, no new corrections.

### Guardrails (mirroring existing handlers)

- Request bodies capped at 32KB for transcript-bearing endpoints.
- Per-message caps: user turns ≤ 2,000 chars, reflection ≤ 4,000 chars.
- Transcript validation before any Gemini call: roles alternate starting with
  "user", ≤ 11 messages, non-empty text.
- 30s timeout and `MaxOutputTokens` on every Gemini call.

## Frontend

Two new routes, both static-export compatible:

- **`/discussion`** — the session page. `DiscussionSession` owns the state
  machine in React state:
  `answering → conversation → reflection → studying → retry → comparison`.
  No persistence; reload restarts.
- **`/discussion/history`** — fetches the sessions list client-side; detail
  shows inline (accordion/panel) rather than a dynamic `[id]` route.

Phase components (small and individually testable, like existing components):

| Component | Responsibility |
| --- | --- |
| `ChatTranscript` + input | Initial answer and follow-up conversation; "Finish conversation" button appears after ≥1 answered follow-up |
| `ReflectionPrompt` | Japanese reflection question with JA textarea and a "nothing to add" skip |
| `GapAndExpressions` | Expressed/missing idea lists + 2–4 expression cards + "Try the question again" |
| `RetryForm` | Original question re-shown; learned expressions visible as reference chips while typing |
| `ComparisonView` | Before/after side by side, expressions learned, `retry_feedback`; "Save & finish" triggers the complete call |
| `SessionHistory` | List + inline detail |

Wiring: new functions in `lib/api.ts` following its existing fetch/auth
pattern; `AppHeader` gets a Discussion link alongside Mistakes.

Philosophy call made explicit: during `conversation` the user's first answer
gets **no** feedback or correction UI at all — the transcript just grows. All
teaching is deferred to `studying`.

## Error handling

- Empty question bank → `404`; frontend shows "no questions available".
- Gemini failure/timeout on any step → `500`; the client keeps its state and
  offers "try again" for just that call — a failed AI call never loses
  session progress.
- Malformed/oversized bodies, invalid transcript shape → `400` before any
  Gemini call.
- Gemini returning malformed JSON or out-of-range results → server validates
  against the schema, truncates where possible, errors otherwise.
- Missing/foreign session on detail fetch → `404`.

## Testing

Red/green TDD throughout.

- **Go unit tests**: handlers with fake `DiscussionRepository` /
  `DiscussionCoach` (table-driven, like `handlers_test.go`); prompt builders
  as pure-function tests; the 5-turn cap returns `done` without touching the
  coach; `GeminiCoach` with a fake `contentGenerator` verifying schema
  parsing and validation.
- **Firestore repo tests**: emulator-backed like `firestore_repo_test.go` —
  `is_active` filtering, session save/list/get, per-user isolation.
- **Frontend vitest**: each phase component plus `DiscussionSession` state
  transitions with mocked `lib/api`.
- **E2E**: `stubCoach` in `cmd/e2eserver` (deterministic follow-ups → done,
  fixed gaps/expressions, fixed retry feedback) + a seeded fixture question;
  one Playwright spec walking the happy path through all six phases, one for
  history.
- **Seed command test**: NDJSON row validation, like `cmd/seed/main_test.go`.

## Out of scope (MVP)

Pronunciation scoring, CEFR assessment, spaced repetition / expression
lifecycle tracking (Learned → Used → Reused), LLM-generated initial
questions, topic/difficulty picker UI, mid-session resume, social features,
gamification, advanced analytics.

## Future direction (recorded, not designed)

- Expression lifecycle tracking and natural re-surfacing of learned
  expressions in later questions (transfer across contexts).
- Voice input via browser speech-to-text.
- LLM-assisted expansion of the curated question bank.
