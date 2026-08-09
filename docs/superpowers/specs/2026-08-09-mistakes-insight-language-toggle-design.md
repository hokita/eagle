# Mistakes insight language toggle

## Problem

`GET /api/mistakes/insight` already accepts and validates a `language` query
param (`en`/`ja`), and `Mistakes.tsx` already sends one — but it's read
silently from `localStorage['eagle:explainLanguage']`, a preference that can
currently only be set from the Explain toggle on the Translator page. A user
who has never visited the Translator page (or wants Japanese mistakes-insight
without touching the translator) has no way to choose the insight language
from the Mistakes page itself.

## Solution

Add a visible EN/JA toggle to the Weakness Insight card on the Mistakes page,
mirroring the existing Explain-language toggle on the Translator page
(`fe/src/components/Translator.tsx`): a `role="group"` button pair, active
language shown via `variant="default"` + `aria-pressed`, both buttons
disabled while a fetch is in flight.

The toggle reads from and writes to the **same** `eagle:explainLanguage`
localStorage key the Translator page already uses. This key is already the
one `insightCacheKey` scopes its cache entries by, so no new storage key or
cache-key change is needed — this makes explicit (via a second UI) a
preference that already existed implicitly. Setting the language from either
page updates the other.

## Behavior

- On mount, `Mistakes` reads `eagle:explainLanguage` into a new
  `insightLanguage` state (`'en' | 'ja'`, default `'en'`) — same default and
  parsing `Translator.tsx` uses.
- `loadMistakes` passes `insightLanguage` into `loadInsight`, which is
  changed to take language as a parameter instead of re-reading
  `localStorage` itself.
- Clicking EN/JA calls `selectInsightLanguage(language)`, which:
  1. updates `insightLanguage` state,
  2. persists it to `localStorage['eagle:explainLanguage']`,
  3. re-invokes `loadInsight(mistakes, language)` for the current mistakes
     list.
- `loadInsight` still checks the `sessionStorage` cache first (keyed by
  uid+language+fingerprint per the existing `insightCacheKey`), so toggling
  back to a previously-fetched language in the same session reuses the
  cached insight instead of re-calling the API.
- Toggle buttons are disabled while `insightLoading` is true — the same
  concurrent-fetch guard already applied to the Translator page's toggle
  (added there after a real bug: PR #8's fix for stale-language mismatches).
- The toggle only renders when the insight card is visible (i.e. there are
  mistakes to summarize); no change to the empty/error/loading states of the
  mistakes list itself.

## Non-goals

- No backend changes — `/api/mistakes/insight` already validates and uses
  `language` correctly.
- No new localStorage key or cache-key shape change.
- No changes to the Translator page's own toggle.

## Testing

Extend `fe/src/components/Mistakes.test.tsx`:

- Toggle renders in the insight card header with EN active by default, and
  the initial `getMistakesInsight` call uses `'en'`.
- A stored `'ja'` preference is restored on mount (initial fetch uses
  `'ja'`, JA button shows as pressed).
- Clicking JA persists the preference, re-fetches the insight in Japanese,
  and updates the displayed text.
- Toggle buttons are disabled while `insightLoading` is true.
- Switching back to a language whose insight was already fetched this
  session reuses the cache instead of calling the API again.
