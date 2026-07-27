# Multi-Level Sentence Filter — Design Spec

## Problem

The difficulty-level filter added in `feat/sentence-difficulty-level` lets a
user pick exactly one level (1-5) or "Any level" from a single `<select>`.
In practice learners often want to practice a *range* of levels at once
(e.g. "just the easy ones, 1 and 2" or "skip level 5 for now") rather than
being locked to a single level or forced back to unfiltered "any".

## Goal

Let a user select any combination of levels 1-5 to practice, with an
explicit toggle per level, persisted across sessions.

## Non-goals

- No change to how a sentence is authored/seeded — each sentence document
  still has exactly one `Level` (1-5, or 0 for legacy unleveled docs). Only
  the *filter* becomes multi-valued.
- No UI for the "unleveled" (`Level == 0`) bucket specifically — those
  sentences continue to surface only under the "any" (no-filter) state,
  same as today.
- No server-side persistence of the user's level preference — it's a
  client-side (`localStorage`) convenience, not synced across devices.

## Architecture / data flow

1. `Translator.tsx` holds `selectedLevels: number[]`, initialized on mount
   from `localStorage` (key `eagle:selectedLevels`), defaulting to
   `[1, 2, 3, 4, 5]` (all selected) when nothing is stored yet.
2. Five toggle buttons (levels 1-5) replace the current `<select>`. Clicking
   one flips its membership in `selectedLevels`, writes the new array to
   `localStorage`, resets question state, and refetches a sentence.
3. When calling the API, the component treats "0 selected" and "5 selected"
   identically to today's "Any level" — both omit the filter entirely, so
   legacy unleveled (`Level == 0`) sentences keep showing up in the default
   state. Any other combination (1-4 levels checked) sends that exact set.
4. `api.getRandomSentence(levels?: number[])` builds
   `GET /api/sentence/random?levels=1,3` when `levels` is a non-empty array,
   otherwise omits the query param.
5. The Go handler parses `levels` as a comma-separated list of ints,
   validates each is in 1-5 (`400` otherwise), dedupes them, and passes the
   set to `RandomCandidate`. An absent/empty param means "any" (nil slice).
6. `firestoreRepo.RandomCandidate` filters candidates by set membership
   instead of equality; an empty/nil `levels` slice matches everything
   (including `Level == 0` docs), matching current "any" semantics.

## Backend design (`eagle/api`)

### Modified files

- **`sentence.go`**: update the `SentenceRepository` doc comment and
  signature —
  ```go
  // RandomCandidate returns a random non-mastered, non-reported sentence.
  // levels restricts candidates to sentences whose Level is in the set;
  // an empty levels means "any level" (no filtering), including sentences
  // with no level set.
  RandomCandidate(ctx context.Context, uid string, levels []int) (*Sentence, error)
  ```

- **`handlers.go`** (`getRandomSentence`): replace the single `level` parse
  with a `levels` parse:
  ```go
  var levels []int
  if raw := r.URL.Query().Get("levels"); raw != "" {
      seen := map[int]bool{}
      for _, part := range strings.Split(raw, ",") {
          n, err := strconv.Atoi(strings.TrimSpace(part))
          if err != nil || n < 1 || n > 5 {
              http.Error(w, "Invalid levels parameter", http.StatusBadRequest)
              return
          }
          if !seen[n] {
              seen[n] = true
              levels = append(levels, n)
          }
      }
  }
  ```

- **`firestore_repo.go`** (`RandomCandidate`): change the `level int` param
  to `levels []int`; build a `map[int]bool` (or small linear scan, given
  max 5 entries) once before the candidate loop, and replace
  `if level != 0 && sd.Level != level` with membership check against that
  set (empty set = no filter, same short-circuit as today's `level == 0`).

### Tests (TDD: red before green)

- `handlers_test.go`: extend `fakeRepo.RandomCandidate` to record
  `[]int` instead of `int`; add cases for a single level, multiple levels
  (`levels=1,3`), invalid entries (`levels=1,9` / `levels=abc` → 400),
  duplicate entries deduped, and empty/absent param → nil slice passed
  through.
- `firestore_repo_test.go`: extend existing level-filter tests to cover
  multi-level subsets — e.g. `levels=[2,3]` excludes level-1/4/5 docs *and*
  unleveled (`Level == 0`) docs; empty `levels` includes everything
  including unleveled docs (unchanged from today's `level == 0` case).

## Frontend design (`eagle/fe`)

### Modified files

- **`src/lib/api.ts`**: change
  `getRandomSentence: (level?: number) => ...` to
  ```ts
  getRandomSentence: (levels?: number[]) =>
    request<Sentence>(
      `/api/sentence/random${levels && levels.length > 0 ? `?levels=${levels.join(',')}` : ''}`
    ),
  ```

- **`src/components/Translator.tsx`**:
  - Replace `level: number` state with `selectedLevels: number[]`.
  - On mount, read `localStorage.getItem('eagle:selectedLevels')`; if
    present and valid JSON, use it, else default to `[1, 2, 3, 4, 5]`.
  - New `toggleLevel(n: number)`: flips `n`'s membership in
    `selectedLevels`, writes the result to `localStorage`, calls
    `resetQuestionState()`, and refetches via a helper that decides what to
    pass to `api.getRandomSentence` — `undefined` when the set's size is 0
    or 5, otherwise the sorted array — mirroring the current
    `handleLevelChange` pattern but for a set instead of a single value.
  - Replace the `levelSelector` `<select>` block with 5 toggle `Button`s
    (`variant={selectedLevels.includes(n) ? 'default' : 'outline'}`),
    keeping the same `aria-label="Sentence difficulty level"` region
    (individual buttons get their own accessible name, e.g.
    `aria-label={`Level ${n}`}` plus `aria-pressed={selectedLevels.includes(n)}`).

### Tests (TDD: red before green)

- `src/lib/api.test.ts`: `getRandomSentence([1, 3])` builds
  `?levels=1,3`; `getRandomSentence()` and `getRandomSentence([])` both
  omit the query param.
- `Translator.test.tsx`: all 5 toggles render active by default (or
  whatever was last persisted); clicking a toggle updates
  `localStorage` and calls `api.getRandomSentence` with the new set;
  unchecking every toggle behaves like all-checked (no filter sent);
  a fresh mount reads a previously-stored selection from `localStorage`.

### e2e

- `e2e/tests/level-filter.spec.ts`: update to toggle two checkboxes (not
  select a single dropdown value) and assert the returned sentences are
  drawn only from those levels.

## Open items for implementation

None — all decisions above were confirmed during design review.
