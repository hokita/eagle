# UI reorganization: two-phase practice screen

## Problem

Seven features have landed on the practice screen since it was built — level
filtering (#4, #9), explain (#7 era), explain-language toggle (#8), the
mistakes page (#11), weakness insight (#12), insight-language toggle (#17) —
and each one attached itself wherever there was room. The result, on a phone
(the primary device), is:

- **The card header mixes four unrelated things.** `Translator.tsx:344-355`
  puts the card title, its description, a nav link to Mistakes, and the level
  filter dropdown in a single flex row. Navigation, settings, and content all
  compete in the same 40px strip.
- **The screen grows without bound after an answer.** A wrong answer stacks a
  feedback alert, a blue correct-answer box, a yellow previous-attempts box, an
  Explain button plus an EN/JA pair, and a purple explanation box
  (`Translator.tsx:433-551`). `Next Sentence` sits below all of it, so the
  longer the AI explanation runs, the further the primary action scrolls off
  screen.
- **Five hues carry no information.** Blue, yellow, purple, green and red
  blocks are hardcoded Tailwind palette classes written inline, while the token
  set in `globals.css:4-17` (`--muted`, `--border`, `--card`, `--primary`) goes
  entirely unused by feature code. Color is doing decoration, not signalling.
- **Buttons have no consistent home.** Speak, Level, Mistakes, Check, Explain,
  EN, JA, Next, Report — nine controls across four size/variant combinations,
  each placed next to whatever it affects.
- **Settings have no home at all.** The level filter lives in the card header;
  the AI language lives inline next to the Explain button *and again* on the
  Mistakes page. `Translator.tsx:24` and `Mistakes.tsx:11` both define
  `EXPLAIN_LANGUAGE_STORAGE_KEY = 'eagle:explainLanguage'` and each renders its
  own EN/JA pair — one preference wearing two independent UIs.

## Solution

Split the practice screen into two phases that swap in place rather than one
screen that grows, and give settings a single home shared by both pages.

**Phase 1 (answering)** shows only the sentence card and the translation
input. **Phase 2 (review)** replaces the input card with a review card: a
verdict chip, a segmented control, and one neutral panel. Both phases end with
the primary action in the same position, so `Next Sentence` never moves
regardless of explanation length.

Settings move into a bottom sheet opened from a gear in a new shared header.
It holds the level chips and the AI language control — the single instance of
a control that currently exists twice.

Color is reserved for the verdict chip alone. Every other surface becomes
`bg-muted` / `border-border`, distinguished by small uppercase labels.

### Copy and accessible names are preserved wherever the control survives

A deliberate constraint, to keep the blast radius proportional to the visual
change. Unless a control is genuinely removed, its visible text and accessible
name stay byte-identical to today:

- `Check Translation`, `Next Sentence`, `Report` / `Reported` — unchanged.
- `Correct! Well done!` and `Not quite right. Try again!` — unchanged; they
  become the contents of the verdict chip instead of an `Alert`.
- `Correct: N` / `Incorrect: N` — unchanged, restyled smaller.
- The textarea keeps `aria-label="Your English translation"`.
- The level controls stay real `<input type="checkbox">` elements with
  `aria-label="Level N"`, styled as chips. Multi-select semantics are already
  correct; only their container moves.
- The Mistakes nav stays a `<link>` with accessible name `Mistakes`; the
  practice page keeps an `<h1>Eagle</h1>`.

This holds three of the six e2e specs at zero changes and the other three to a
single line each (see Testing).

## Behavior

### Shared header (`AppHeader`, new)

Rendered by both `/` and `/mistakes`. Left: the eagle image plus `Eagle` as an
`<h1>`, linking to `/` (this replaces the Mistakes page's `← Back` link).
Right, in order: a `Mistakes` link (text, hidden on `/mistakes` itself), a
settings button carrying lucide's `Settings` icon with
`aria-label="Settings"`, and the existing `UserMenu`. `lucide-react` is
already a dependency; no new icon library is added.

The card title `Translate this sentence` and description `Translate the
Japanese sentence below into English` are deleted. A large Japanese sentence
above a field labelled *Your translation* does not need a caption.

### Practice page, phase 1 — answering

Sentence card: the Japanese sentence, a speak button, and a meta row carrying
`Correct: N`, `Incorrect: N`, and the active level summary (the same `All` /
`1, 2, 3` string `levelSummary` computes today, now read-only here since the
filter lives in the sheet).

Input card: label `Your translation`, the textarea, and the existing
capitalize-on-blur and Ctrl+Enter behaviors, unchanged.

`Check Translation` renders full-width below the cards, disabled until the
textarea is non-empty — same guard as today.

### Practice page, phase 2 — review

The input card unmounts; the sentence card stays. The review card contains:

1. **Verdict chip** — green for correct, red for incorrect, carrying today's
   copy. This is the only colored element on the screen.
2. **Segmented control**, rendered only with the tabs that apply:
   - `Answer` — always present. Shows `You wrote` (muted, struck through, and
     only when the answer was wrong) above `Correct` (emphasized).
   - `Attempts N` — only when `histories.length > 0`. The previous incorrect
     answers, one per row.
   - `Explain` — only when `feedback === 'incorrect'`. Selecting the tab
     triggers the fetch that the `Explain` button triggers today; the loading,
     error-with-retry, and `ReactMarkdown` rendering (including
     `disallowedElements={['a','img']}`) carry over unchanged. The
     `explainRequestId` race guard at `Translator.tsx:151-171` carries over
     as-is.

   A correct answer therefore shows a green chip and a single `Answer` tab.
3. **Footer** — `Next Sentence` (primary, flex-1) and `Report` (ghost),
   in the same screen position as `Check Translation` occupied in phase 1.

Phase is derived from existing `showAnswer` state; no new phase variable.
`resetQuestionState` additionally resets the selected tab to `Answer`.

### Settings sheet (`SettingsSheet`, new)

Opened by the header gear, dismissed by backdrop click, Escape, or a close
button — the same outside-click/Escape pattern already written for the level
dropdown at `Translator.tsx:269-287`, which is deleted along with
`isLevelMenuOpen` and `levelMenuRef`.

Contents:

- **Sentence levels** — five checkboxes styled as chips. Toggling one persists
  to `eagle:selectedLevels` and refetches the current sentence, exactly as
  `toggleLevel` does today.
- **AI language** — an English / 日本語 pair persisting to
  `eagle:explainLanguage`, with the helper line *Used for explanations and
  weakness insight*.

### Shared settings state (`useSettings`, new hook)

`fe/src/lib/useSettings.ts` becomes the single owner of both preferences,
absorbing `loadStoredLevels`, `levelsForRequest`, the `LEVELS` constant, the
duplicated `EXPLAIN_LANGUAGE_STORAGE_KEY`, and the read/parse/persist logic
currently copy-pasted across `Translator.tsx:24-65,85-88,173-179` and
`Mistakes.tsx:11,50-53,81-89`. Both pages consume it; neither touches
`localStorage` directly.

### Mistakes page

Header replaced by `AppHeader` plus an `<h2>Mistakes</h2>`. The insight card's
inline EN/JA pair (`Mistakes.tsx:147-168`) is deleted — language now comes
from the sheet, and changing it there re-runs `loadInsight` on this page too.
`insightCacheKey` and its uid+language+fingerprint scoping are untouched; the
comment at `Mistakes.tsx:13-21` still applies verbatim.

Mistake cards drop the blue and yellow blocks for the shared neutral surface,
with wrong answers as struck-through chips.

### Component split

`Translator.tsx` (582 lines) cannot hold this structure. It becomes a
container owning fetch state, plus:

| New file | Responsibility |
| --- | --- |
| `components/AppHeader.tsx` | Brand, Mistakes link, gear, `UserMenu` |
| `components/SettingsSheet.tsx` | Levels + AI language |
| `components/QuestionCard.tsx` | Sentence, speak, meta row |
| `components/ReviewPanel.tsx` | Verdict chip, tabs, panel bodies |
| `components/ui/sheet.tsx` | Backdrop + bottom sheet, Escape/outside-click |
| `components/ui/segmented.tsx` | Roving-tabindex tab list |
| `lib/useSettings.ts` | Levels + language, single source of truth |

`speakJapanese` (`Translator.tsx:90-140`, including its Safari workarounds)
moves to `lib/speech.ts` unchanged.

## Non-goals

- **No backend changes.** No handler, route, Firestore, or Gemini work. The
  API surface is already correct for everything here.
- **No bottom tab bar.** Option A's Practice/Mistakes tabs can be layered on
  later without redoing this; not worth the vertical space until Mistakes
  proves to be high-traffic.
- **No new icon library.** `lucide-react` covers the five or six icons needed.
- **No dark mode.** The tokens in `globals.css` would support it; out of scope.
- **No typography or spacing system rework.** Existing `ui/` primitives keep
  their current sizing.
- **No change to auth, the login screen, or `UserMenu`'s internals.**

## Testing

TDD throughout: each behavior below is written failing against the new
structure before the component is built.

**`fe/src/components/Translator.test.tsx`** (currently 465 lines) — restructured
alongside the component split, with per-component tests where the split makes
them natural:

- Phase 1 renders the sentence and input, and renders no verdict, tabs, or
  Explain affordance.
- Submitting unmounts the input card and mounts the review card; the verdict
  chip carries the existing copy for both outcomes.
- `Attempts` renders only when histories exist; `Explain` only when incorrect;
  a correct answer shows `Answer` alone.
- Selecting `Explain` fetches once, renders Markdown, and surfaces
  error-with-retry.
- The `explainRequestId` guard still discards a superseded response.
- `Next Sentence` resets the selected tab to `Answer`.

**`fe/src/components/Mistakes.test.tsx`** (currently 352 lines) — the five
toggle-specific cases from the #17 spec are deleted; language now arrives from
the hook. Cache-reuse, error, empty, and list-rendering cases stay.

**New: `SettingsSheet.test.tsx`, `useSettings.test.ts`** — persistence of both
keys, malformed-storage fallback (today's silent `catch` at
`Translator.tsx:57-59`), and the `levelsForRequest` all-or-none rule.

**e2e** (`e2e/tests/`) — blast radius, given preserved copy:

| Spec | Change |
| --- | --- |
| `auth.spec.ts` | none |
| `correct-answer.spec.ts` | none |
| `report-next.spec.ts` | none |
| `incorrect-explain.spec.ts` | `Explain` becomes a tab click, not a button click |
| `level-filter.spec.ts` | opener becomes the gear; the five `Level N` checkbox selectors are unchanged |
| `mistakes.spec.ts` | `← Back` link becomes the `Eagle` home link |

**Manual verification** on a phone-width viewport, per the local dev stack
(emulators + e2eserver + fe): the primary button stays in the same position
across a short explanation, a long explanation, and a correct answer.
