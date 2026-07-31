# Mistakes Page — Design Spec

## Problem

A learner's wrong answers are currently only visible transiently, right after
checking a specific sentence (`histories` on `POST /api/answer/check`). There
is no way to look back over everything you've gotten wrong across a session
or across time — you'd have to remember which sentences tripped you up.

## Goal

A new page that lists every sentence the user has ever answered incorrectly,
showing the Japanese sentence, the correct English answer, and the user's
past wrong attempts for it — grouped one entry per sentence, most recently
missed first. Read-only for v1; reachable from a button on the main practice
card.

## Non-goals

- No per-row actions (e.g. "practice this sentence now") — v1 is view-only.
  Revisit if it turns out to be a common ask.
- No pagination/infinite scroll — this is a personal study tool at a scale
  where loading the full list in one request is fine. Add pagination later
  if the list grows large enough to matter.
- No server-side Firestore query optimization (e.g. a precomputed mistakes
  index or a `Where("incorrect_count", ">", 0)` filter) — the endpoint
  reuses the existing fan-out read pattern `RandomCandidate` already uses
  (load all of the user's `sentence_stats` docs, filter in Go). Acceptable
  at current scale; can be revisited if read volume becomes a problem.

## Architecture / data flow

1. New endpoint `GET /api/mistakes` (auth-gated, like all other routes)
   returns every sentence with at least one recorded wrong answer for the
   authenticated user, most recently missed sentence first.
2. `firestoreRepo.ListMistakes(ctx, uid)` iterates
   `users/{uid}/sentence_stats`, keeps docs where `IncorrectCount > 0`, and
   for each: loads the sentence's `japanese`/`english` from the top-level
   `sentences` collection, and queries that stat doc's `histories`
   subcollection for `is_correct == false` entries (the same query
   `ListIncorrectHistories` already runs for a single sentence, generalized
   here to run once per mistaken sentence). If the referenced sentence doc
   is missing, that entry is skipped rather than failing the whole request.
3. Results are sorted in Go by each sentence's newest wrong-answer
   timestamp, descending (Firestore doc iteration order is not otherwise
   meaningful here).
4. The frontend adds a new route `/mistakes` (`AuthGate`-wrapped, same as
   the main page) that fetches the list on mount and renders it as a
   "stacked rows" list: one card per sentence with the Japanese sentence,
   the correct answer, and the wrong answers as tag chips — chosen over a
   literal `<table>` because a real learner mostly uses this on an iPhone,
   where a table needs horizontal scrolling to read; stacked rows keep the
   same information density but reflow to any width.
5. A "Mistakes" button on the main practice card's header (next to the
   existing level-filter button) links to `/mistakes`; the mistakes page
   has a "← Back" link to `/`.

## Backend design (`eagle/api`)

### Modified files

- **`sentence.go`**: add types and extend the repository interface —
  ```go
  type MistakeSentence struct {
      SentenceID    int             `json:"sentence_id"`
      Japanese      string          `json:"japanese"`
      CorrectAnswer string          `json:"correct_answer"`
      WrongAnswers  []AnswerHistory `json:"wrong_answers"`
  }

  type ListMistakesResponse struct {
      Mistakes []MistakeSentence `json:"mistakes"`
  }
  ```
  ```go
  // ListMistakes returns every sentence the user has ever answered
  // incorrectly, most recently missed first.
  ListMistakes(ctx context.Context, uid string) ([]MistakeSentence, error)
  ```

- **`handlers.go`**: new `getMistakes` handler, `GET` only, reads `uid` from
  context (same as `getRandomSentence`), calls `s.repo.ListMistakes`, writes
  `ListMistakesResponse`. No request body or query params.

- **`router.go`**: register `mux.HandleFunc("/api/mistakes", auth(srv.getMistakes))`.

- **`firestore_repo.go`**: new `ListMistakes`:
  - Iterate `r.userStats(uid)` docs (same as `RandomCandidate`'s stats
    loop); parse each doc ID as the sentence ID, decode into `statsDoc`,
    skip if `IncorrectCount == 0`.
  - For each remaining stat doc: `Get` the sentence doc by ID from
    `sentences`; if `codes.NotFound`, skip this entry (continue, don't
    error the whole request — a reported/removed sentence shouldn't break
    the mistakes list).
  - Query that stat doc's `histories` subcollection with
    `Where("is_correct", "==", false).OrderBy("created_at", firestore.Desc)`
    (identical query shape to `ListIncorrectHistories`), map to
    `[]AnswerHistory`.
  - Build a `MistakeSentence` from the above.
  - After the loop, sort the slice by `WrongAnswers[0].CreatedAt`
    descending (every included entry has at least one wrong answer, so
    `WrongAnswers` is never empty).

### Tests (TDD: red before green)

- `handlers_test.go`: `getMistakes` — empty result, a repo error surfaces
  as 500, response shape matches `ListMistakesResponse`.
- `firestore_repo_test.go`: `ListMistakes` —
  - sentence with multiple wrong answers groups them under one entry,
    newest-first within the entry (reusing existing history-ordering
    assertions);
  - a sentence with `IncorrectCount == 0` (never missed) is excluded;
  - multiple mistaken sentences come back sorted by most recent mistake
    first;
  - a stats doc whose sentence was deleted is skipped without erroring.

## Frontend design (`eagle/fe`)

### New/modified files

- **`src/lib/api.ts`**: add
  ```ts
  export interface Mistake {
    sentence_id: number
    japanese: string
    correct_answer: string
    wrong_answers: AnswerHistory[]
  }
  ```
  and
  ```ts
  listMistakes: () => request<{ mistakes: Mistake[] }>('/api/mistakes'),
  ```

- **`src/app/mistakes/page.tsx`** (new): mirrors `src/app/page.tsx`'s
  `AuthGate` wrapping, but the rendered component doesn't need the `user`
  value:
  ```tsx
  'use client'
  import AuthGate from '@/components/AuthGate'
  import Mistakes from '@/components/Mistakes'
  export default function Page() {
    return <AuthGate>{() => <Mistakes />}</AuthGate>
  }
  ```

- **`src/components/Mistakes.tsx`** (new): `'use client'` component that
  calls `api.listMistakes()` on mount and renders one of: loading spinner,
  error state with a retry button, empty state ("No mistakes yet — nice
  work!"), or the populated list — reusing the same gradient-background
  page shell and `Card` primitives as `Translator`. Header row: a
  `Link href="/"` styled as an outline button ("← Back") and the page
  title "Mistakes". Each list entry is a compact card: Japanese sentence,
  correct answer (blue badge, matching the existing "Correct Answer" style
  in `Translator`), and wrong answers rendered as wrapping amber tag chips
  (matching the existing "Previous Incorrect Answers" yellow box styling) —
  no per-row buttons or actions.

- **`src/components/Translator.tsx`**: in the `CardHeader` row that
  currently holds only `levelMenu`, add a `Mistakes` link button next to
  it:
  ```tsx
  <Button asChild variant="outline" size="sm">
    <Link href="/mistakes">Mistakes</Link>
  </Button>
  ```
  wrapped together with `levelMenu` in a `flex gap-2` container.

### Tests (TDD: red before green)

- `src/lib/api.test.ts`: `listMistakes()` hits `GET /api/mistakes` and
  returns the parsed `mistakes` array.
- `src/components/Mistakes.test.tsx` (new): loading state renders first;
  populated response renders one block per mistake with its wrong answers;
  empty `mistakes: []` renders the empty-state message; a fetch rejection
  renders the error state with a working retry button.
- `Translator.test.tsx`: the header now renders a "Mistakes" link pointing
  at `/mistakes`.

## Open items for implementation

None — all decisions above were confirmed during design review (approach,
API/data shape, and frontend layout including the mobile-width check via
the visual companion).
