# Mistakes Insight Language Toggle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a user pick English or Japanese for the Mistakes page's "Weakness Insight" summary directly from that page, instead of it being silently controlled by a preference only settable on the Translator page.

**Architecture:** Frontend-only change to `fe/src/components/Mistakes.tsx`. The backend (`GET /api/mistakes/insight`) already accepts and validates a `language` query param and needs no changes. Add a `role="group"` EN/JA button pair to the Weakness Insight card header, mirroring the existing toggle already shipped on `Translator.tsx`. The toggle reads from and writes to the same `eagle:explainLanguage` localStorage key the Translator page already owns, so the preference is shared app-wide — no new storage key, no cache-key change (the insight's sessionStorage cache is already keyed by this same value via `insightCacheKey`).

**Tech Stack:** Next.js (App Router), React, TypeScript, Vitest + Testing Library, Tailwind (shadcn/ui `Button`/`Card` primitives).

## Global Constraints

- Reuse the existing `eagle:explainLanguage` localStorage key (already read inside `loadInsight` today) — do not introduce a second key. `Translator.tsx:24` defines it as `EXPLAIN_LANGUAGE_STORAGE_KEY`; mirror that constant in `Mistakes.tsx`.
- No backend changes — `api/internal/app/handlers.go:142-178` (`getMistakesInsight`) already validates `language` and passes it to the analyzer.
- Follow red/green TDD: write the failing test, confirm it fails, write minimal code to pass, confirm it passes, then commit.
- Match the existing Translator toggle's accessibility pattern exactly: `role="group"` wrapper with an `aria-label`, `aria-pressed` on each button reflecting selection, `variant="default"` when active / `"outline"` when inactive, `size="sm"`, `type="button"`.

---

### Task 1: Add the insight-language toggle (state, handler, UI) with a disabled-while-loading guard

**Files:**
- Modify: `fe/src/components/Mistakes.tsx`
- Test: `fe/src/components/Mistakes.test.tsx`

**Interfaces:**
- Consumes: `api.getMistakesInsight(language: 'en' | 'ja')` (existing, `fe/src/lib/api.ts:79-80`); `Mistake` type (existing).
- Produces: `EXPLAIN_LANGUAGE_STORAGE_KEY` constant, `insightLanguage` state (`'en' | 'ja'`), `selectInsightLanguage(language: 'en' | 'ja'): void` handler — later tasks (and later steps in this task) build on these exact names.

- [ ] **Step 1: Write the failing tests**

Add `localStorage.clear()` to the existing `beforeEach` in `fe/src/components/Mistakes.test.tsx` (it currently only clears `sessionStorage`):

```tsx
beforeEach(() => {
  vi.clearAllMocks()
  sessionStorage.clear()
  localStorage.clear()
  mockApi.getMistakesInsight.mockResolvedValue({ insight: '' })
})
```

Then add these two tests inside the existing `describe('Mistakes', ...)` block:

```tsx
  it('shows an EN/JA insight-language toggle that persists the choice and re-fetches in the new language', async () => {
    mockApi.listMistakes.mockResolvedValue({
      mistakes: [
        {
          sentence_id: 1,
          japanese: '時間がありません。',
          correct_answer: "I don't have time.",
          wrong_answers: [{ id: 1, incorrect_answer: 'I have no time.', created_at: '2026-01-03T00:00:00Z' }],
        },
      ],
    })
    mockApi.getMistakesInsight.mockResolvedValueOnce({ insight: 'English insight.' })
    render(<Mistakes />)
    await screen.findByText('English insight.')
    expect(screen.getByRole('button', { name: 'EN' })).toHaveAttribute('aria-pressed', 'true')
    expect(mockApi.getMistakesInsight).toHaveBeenLastCalledWith('en')

    mockApi.getMistakesInsight.mockResolvedValueOnce({ insight: '日本語のインサイト。' })
    fireEvent.click(screen.getByRole('button', { name: 'JA' }))
    await screen.findByText('日本語のインサイト。')

    expect(mockApi.getMistakesInsight).toHaveBeenLastCalledWith('ja')
    expect(screen.getByRole('button', { name: 'JA' })).toHaveAttribute('aria-pressed', 'true')
    expect(localStorage.getItem('eagle:explainLanguage')).toBe('ja')
  })

  it('disables the insight-language toggle while the insight is loading', async () => {
    mockApi.listMistakes.mockResolvedValue({
      mistakes: [
        {
          sentence_id: 1,
          japanese: '時間がありません。',
          correct_answer: "I don't have time.",
          wrong_answers: [{ id: 1, incorrect_answer: 'I have no time.', created_at: '2026-01-03T00:00:00Z' }],
        },
      ],
    })
    mockApi.getMistakesInsight.mockReturnValue(new Promise(() => {}))
    render(<Mistakes />)
    await screen.findByText(/analyzing your mistakes/i)
    expect(screen.getByRole('button', { name: 'EN' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'JA' })).toBeDisabled()
  })
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd fe && npx vitest run src/components/Mistakes.test.tsx`
Expected: FAIL — both new tests fail with "Unable to find an accessible element with the role 'button' and name 'EN'" (the toggle doesn't exist yet).

- [ ] **Step 3: Implement the toggle**

In `fe/src/components/Mistakes.tsx`, add the storage key constant after the imports (mirroring `Translator.tsx:24`):

```tsx
const EXPLAIN_LANGUAGE_STORAGE_KEY = 'eagle:explainLanguage'
```

Add `insightLanguage` state alongside the other `useState` declarations inside `Mistakes()` (default `'en'` for now — restoring from storage is Task 2):

```tsx
  const [insightLanguage, setInsightLanguage] = useState<'en' | 'ja'>('en')
```

Change `loadInsight` to take language as a parameter instead of reading `localStorage` itself:

```tsx
  const loadInsight = async (currentMistakes: Mistake[], language: 'en' | 'ja') => {
    const cacheKey = insightCacheKey(language, currentMistakes)

    if (cacheKey) {
      const cached = sessionStorage.getItem(cacheKey)
      if (cached !== null) {
        setInsight(cached)
        return
      }
    }

    setInsightLoading(true)
    setInsightError(null)
    try {
      const result = await api.getMistakesInsight(language)
      setInsight(result.insight)
      if (cacheKey) {
        sessionStorage.setItem(cacheKey, result.insight)
      }
    } catch (err) {
      setInsightError(err instanceof Error ? err.message : 'Failed to load insight')
    } finally {
      setInsightLoading(false)
    }
  }

  const selectInsightLanguage = (language: 'en' | 'ja') => {
    setInsightLanguage(language)
    if (typeof window !== 'undefined') {
      localStorage.setItem(EXPLAIN_LANGUAGE_STORAGE_KEY, language)
    }
    if (mistakes && mistakes.length > 0) {
      loadInsight(mistakes, language)
    }
  }
```

Update the two existing call sites of `loadInsight` to pass `insightLanguage`:

```tsx
      const result = await api.listMistakes()
      setMistakes(result.mistakes)
      if (result.mistakes.length > 0) {
        loadInsight(result.mistakes, insightLanguage)
      }
```

```tsx
                      <Button variant="outline" size="sm" onClick={() => loadInsight(mistakes ?? [], insightLanguage)}>
                        Try Again
                      </Button>
```

Replace the Weakness Insight card's header to add the toggle next to the title:

```tsx
              <Card className="border-indigo-300">
                <CardHeader className="pb-2 flex-row items-center justify-between space-y-0">
                  <CardTitle className="text-base text-indigo-900">Weakness Insight</CardTitle>
                  <div className="flex gap-1" role="group" aria-label="Insight language">
                    <Button
                      type="button"
                      variant={insightLanguage === 'en' ? 'default' : 'outline'}
                      size="sm"
                      aria-pressed={insightLanguage === 'en'}
                      onClick={() => selectInsightLanguage('en')}
                      disabled={insightLoading}
                    >
                      EN
                    </Button>
                    <Button
                      type="button"
                      variant={insightLanguage === 'ja' ? 'default' : 'outline'}
                      size="sm"
                      aria-pressed={insightLanguage === 'ja'}
                      onClick={() => selectInsightLanguage('ja')}
                      disabled={insightLoading}
                    >
                      JA
                    </Button>
                  </div>
                </CardHeader>
```

(Everything below `</CardHeader>` inside that `Card` — the `CardContent` with the loading/error/insight rendering — is unchanged.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd fe && npx vitest run src/components/Mistakes.test.tsx`
Expected: PASS — all tests in the file pass, including the two new ones.

- [ ] **Step 5: Typecheck and lint**

Run: `cd fe && npx tsc --noEmit && npx eslint src/components/Mistakes.tsx src/components/Mistakes.test.tsx`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add fe/src/components/Mistakes.tsx fe/src/components/Mistakes.test.tsx
git commit -m "feat(fe): add EN/JA toggle for the mistakes weakness-insight language"
```

---

### Task 2: Restore the stored language preference on mount

**Files:**
- Modify: `fe/src/components/Mistakes.tsx`
- Test: `fe/src/components/Mistakes.test.tsx`

**Interfaces:**
- Consumes: `EXPLAIN_LANGUAGE_STORAGE_KEY`, `insightLanguage` state (from Task 1).
- Produces: no new names — changes only the `insightLanguage` initializer.

- [ ] **Step 1: Write the failing test**

Add to `fe/src/components/Mistakes.test.tsx`:

```tsx
  it('restores a stored ja preference on mount and fetches the insight in Japanese', async () => {
    localStorage.setItem('eagle:explainLanguage', 'ja')
    mockApi.listMistakes.mockResolvedValue({
      mistakes: [
        {
          sentence_id: 1,
          japanese: '時間がありません。',
          correct_answer: "I don't have time.",
          wrong_answers: [{ id: 1, incorrect_answer: 'I have no time.', created_at: '2026-01-03T00:00:00Z' }],
        },
      ],
    })
    mockApi.getMistakesInsight.mockResolvedValue({ insight: '日本語のインサイト。' })
    render(<Mistakes />)
    await screen.findByText('日本語のインサイト。')
    expect(mockApi.getMistakesInsight).toHaveBeenCalledWith('ja')
    expect(screen.getByRole('button', { name: 'JA' })).toHaveAttribute('aria-pressed', 'true')
  })
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd fe && npx vitest run src/components/Mistakes.test.tsx -t "restores a stored ja preference"`
Expected: FAIL — `getMistakesInsight` is called with `'en'` (the current hardcoded default) instead of `'ja'`, so `screen.findByText('日本語のインサイト。')` times out.

- [ ] **Step 3: Implement the lazy initializer**

In `fe/src/components/Mistakes.tsx`, change the `insightLanguage` state declaration added in Task 1 to restore from storage, matching `Translator.tsx:85-88`'s pattern (guarded for SSR the same way `loadInsight` already guards its `localStorage` read):

```tsx
  const [insightLanguage, setInsightLanguage] = useState<'en' | 'ja'>(() => {
    const stored = typeof window !== 'undefined' ? localStorage.getItem(EXPLAIN_LANGUAGE_STORAGE_KEY) : null
    return stored === 'ja' ? 'ja' : 'en'
  })
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd fe && npx vitest run src/components/Mistakes.test.tsx`
Expected: PASS — the whole file, including all Task 1 and Task 2 tests.

- [ ] **Step 5: Commit**

```bash
git add fe/src/components/Mistakes.tsx fe/src/components/Mistakes.test.tsx
git commit -m "feat(fe): restore the mistakes insight-language preference on mount"
```

---

### Task 3: Verify per-session cache reuse when toggling back to an already-fetched language

**Files:**
- Test: `fe/src/components/Mistakes.test.tsx`

This task adds coverage for behavior that should already exist as a consequence of Tasks 1–2 composing with the pre-existing `insightCacheKey`/`sessionStorage` caching in `loadInsight` (unchanged since before this plan). Unlike Tasks 1–2, this test is not expected to fail first — it's verifying an integration between existing caching and the new toggle, not driving new code. Write it, run it once, and confirm it passes; if it unexpectedly fails, that reveals a real gap in Task 1/2's implementation to fix (see Step 3).

**Interfaces:**
- Consumes: the toggle buttons and `selectInsightLanguage` behavior from Task 1, the `insightLanguage` restore from Task 2. No new interfaces produced.

- [ ] **Step 1: Write the test**

Add to `fe/src/components/Mistakes.test.tsx`:

```tsx
  it('reuses the cached insight when toggling back to a language already fetched this session', async () => {
    mockApi.listMistakes.mockResolvedValue({
      mistakes: [
        {
          sentence_id: 1,
          japanese: '時間がありません。',
          correct_answer: "I don't have time.",
          wrong_answers: [{ id: 1, incorrect_answer: 'I have no time.', created_at: '2026-01-03T00:00:00Z' }],
        },
      ],
    })
    mockApi.getMistakesInsight
      .mockResolvedValueOnce({ insight: 'English insight.' })
      .mockResolvedValueOnce({ insight: '日本語のインサイト。' })
    render(<Mistakes />)
    await screen.findByText('English insight.')

    fireEvent.click(screen.getByRole('button', { name: 'JA' }))
    await screen.findByText('日本語のインサイト。')
    expect(mockApi.getMistakesInsight).toHaveBeenCalledTimes(2)

    fireEvent.click(screen.getByRole('button', { name: 'EN' }))
    await screen.findByText('English insight.')
    expect(mockApi.getMistakesInsight).toHaveBeenCalledTimes(2)
  })
```

- [ ] **Step 2: Run the test**

Run: `cd fe && npx vitest run src/components/Mistakes.test.tsx -t "reuses the cached insight"`
Expected: PASS on the first run (no code change needed — `loadInsight`'s existing cache check, now driven by the toggle's `selectInsightLanguage` calls, already covers this).

- [ ] **Step 3: If it fails**

If `getMistakesInsight` is called a 3rd time, the cache check in `loadInsight` (the `if (cacheKey) { const cached = sessionStorage.getItem(cacheKey); ... }` block) isn't being reached before the toggle's re-fetch — re-read Task 1 Step 3's `loadInsight` implementation and confirm the cache check wasn't accidentally dropped when the function was changed to take `language` as a parameter. Fix and re-run until it passes.

- [ ] **Step 4: Run the full test file**

Run: `cd fe && npx vitest run src/components/Mistakes.test.tsx`
Expected: PASS — every test in the file, old and new.

- [ ] **Step 5: Commit**

```bash
git add fe/src/components/Mistakes.test.tsx
git commit -m "test(fe): cover session-cache reuse across the mistakes insight-language toggle"
```

---

## Manual Verification

After all three tasks are complete, manually confirm in a browser (per this repo's convention of testing UI changes live before calling them done):

1. Start the local dev stack (emulators + e2eserver + fe — see the `project_local_dev_stack` note in memory if unsure how).
2. Get at least one mistake recorded, then visit `/mistakes`.
3. Confirm the Weakness Insight card shows an EN/JA toggle, EN active by default (or JA if you'd previously toggled the Translator's Explain-language to Japanese).
4. Click JA — confirm the insight text re-fetches and renders in Japanese, and the JA button shows as active.
5. Reload the page — confirm JA is still selected and its insight loads without a network call (served from the session cache).
6. Visit `/` (Translator), answer a question incorrectly, click Explain — confirm its language toggle also shows JA (proving the shared preference), and switching it back to EN there is reflected back on `/mistakes` after a reload.
