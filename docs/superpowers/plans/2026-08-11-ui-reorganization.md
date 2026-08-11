# UI Reorganization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the Eagle practice screen into an answering phase and a review phase that swap in place, give both user settings a single sheet shared with the mistakes page, and reserve color for the answer verdict alone.

**Architecture:** `Translator.tsx` (582 lines) becomes a thin container owning fetch state, delegating to `AppHeader`, `QuestionCard`, `ReviewPanel`, and `SettingsSheet`. Two new `ui/` primitives (`Sheet`, `Segmented`) and two new `lib/` modules (`useSettings`, `speech`) carry the shared behavior. Phase is derived from the existing `showAnswer` boolean — no new phase state. Tasks are ordered bottom-up so each one's tests pass standalone.

**Tech Stack:** Next.js 15 (static export, App Router), React 19, TypeScript, Tailwind CSS v4 (CSS-variable theme, no config file), shadcn/ui-style local primitives on Radix, lucide-react icons, Vitest + Testing Library, Playwright for e2e.

## Global Constraints

- **Spec:** `docs/superpowers/specs/2026-08-11-ui-reorganization-design.md`. Read it before Task 1.
- **Frozen copy — do not reword.** `Check Translation`, `Next Sentence`, `Report`, `Reported`, `Correct! Well done!`, `Not quite right. Try again!`, `Correct: N`, `Incorrect: N`, `Mistakes`, `Eagle`.
- **Frozen accessible names.** Textarea keeps `aria-label="Your English translation"`. Level controls stay `<input type="checkbox">` with `aria-label="Level N"`. Mistakes nav stays a link named `Mistakes`. The practice page keeps an `<h1>Eagle</h1>`.
- **No backend changes.** No files under `api/` are touched by any task.
- **No new dependencies.** `lucide-react`, `react-markdown`, Radix and `class-variance-authority` are already installed. Adding a package is a plan violation.
- **Color budget.** Only the verdict chip may carry hue. Every other surface uses `bg-muted` / `border-border` / `bg-card`. No `bg-blue-*`, `bg-yellow-*`, `bg-purple-*` classes may survive in `Translator.tsx` or `Mistakes.tsx`.
- **Markdown hardening is preserved.** Every `ReactMarkdown` usage keeps `disallowedElements={['a', 'img']}`.
- **TDD.** Every task writes the failing test first, watches it fail, then implements. Commit at the end of each task.
- **Working directory** is `fe/` for all frontend commands, `e2e/` for Playwright.
- **Branch:** `refactor/ui-reorganization` (already created).

---

### Task 1: Add the missing design tokens

`globals.css` defines 13 tokens but `button.tsx` references `bg-accent`, `text-accent-foreground`, `bg-destructive`, `text-destructive-foreground` and `ring-offset-background` — none of which exist. In Tailwind v4 an undefined theme key generates **no utility at all**, so `variant="outline"` and `variant="ghost"` currently have no hover state whatsoever. This plan leans on both variants heavily, and the verdict chip needs semantic success/danger colors, so the token set has to be completed first.

This is the one task without a unit test — it is a stylesheet change, verified by a production build and a visual check.

**Files:**
- Modify: `fe/src/app/globals.css:3-35`

- [ ] **Step 1: Add the raw tokens to `:root`**

Insert after the `--ring: #171717;` line (`globals.css:16`):

```css
  --accent: #f5f5f5;
  --accent-foreground: #171717;
  --destructive: #b91c1c;
  --destructive-foreground: #ffffff;
  --destructive-subtle: #fef2f2;
  --destructive-subtle-foreground: #b91c1c;
  --destructive-subtle-border: #fecaca;
  --success-subtle: #f0fdf4;
  --success-subtle-foreground: #15803d;
  --success-subtle-border: #bbf7d0;
```

- [ ] **Step 2: Map them in `@theme inline`**

Insert after the `--color-ring: var(--ring);` line (`globals.css:32`):

```css
  --color-accent: var(--accent);
  --color-accent-foreground: var(--accent-foreground);
  --color-destructive: var(--destructive);
  --color-destructive-foreground: var(--destructive-foreground);
  --color-destructive-subtle: var(--destructive-subtle);
  --color-destructive-subtle-foreground: var(--destructive-subtle-foreground);
  --color-destructive-subtle-border: var(--destructive-subtle-border);
  --color-success-subtle: var(--success-subtle);
  --color-success-subtle-foreground: var(--success-subtle-foreground);
  --color-success-subtle-border: var(--success-subtle-border);
  --color-ring-offset-background: var(--background);
```

- [ ] **Step 3: Verify the build succeeds and the utilities now generate**

Run: `cd fe && npm run build`
Expected: build completes with no errors.

Then confirm the classes exist in the emitted CSS:

Run: `cd fe && grep -o "bg-accent\|bg-success-subtle\|bg-destructive-subtle" out/_next/static/css/*.css | sort -u`
Expected: at minimum `bg-accent` appears (it is referenced by `button.tsx`). `bg-success-subtle` and `bg-destructive-subtle` appear only once Task 9 uses them — their absence here is expected.

- [ ] **Step 4: Commit**

```bash
git add fe/src/app/globals.css
git commit -m "style(fe): complete the theme token set

button.tsx referenced accent, destructive and ring-offset utilities that
globals.css never defined, so outline and ghost buttons generated no hover
styles at all. Adds those plus subtle success/danger pairs for the verdict
chip."
```

---

### Task 2: Extract `speakJapanese` into `lib/speech.ts`

**Files:**
- Create: `fe/src/lib/speech.ts`
- Test: `fe/src/lib/speech.test.ts`
- Reference (do not modify yet): `fe/src/components/Translator.tsx:90-140`

**Interfaces:**
- Consumes: nothing.
- Produces: `speakJapanese(text: string, onSpeakingChange: (speaking: boolean) => void): void`

- [ ] **Step 1: Write the failing test**

Create `fe/src/lib/speech.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { speakJapanese } from './speech'

class FakeUtterance {
  lang = ''
  rate = 0
  pitch = 0
  volume = 0
  voice: unknown = null
  onstart: (() => void) | null = null
  onend: (() => void) | null = null
  onerror: (() => void) | null = null
  constructor(public text: string) {}
}

let spoken: FakeUtterance[]
let synth: {
  speak: ReturnType<typeof vi.fn>
  cancel: ReturnType<typeof vi.fn>
  getVoices: ReturnType<typeof vi.fn>
  speaking: boolean
  pending: boolean
}

beforeEach(() => {
  vi.useFakeTimers()
  spoken = []
  synth = {
    speak: vi.fn((u: FakeUtterance) => spoken.push(u)),
    cancel: vi.fn(),
    getVoices: vi.fn(() => [{ lang: 'en-US' }, { lang: 'ja-JP' }]),
    speaking: false,
    pending: false,
  }
  vi.stubGlobal('speechSynthesis', synth)
  vi.stubGlobal('SpeechSynthesisUtterance', FakeUtterance)
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('speakJapanese', () => {
  it('cancels any in-flight speech and reports speaking immediately', () => {
    const onSpeakingChange = vi.fn()
    speakJapanese('時間がありません。', onSpeakingChange)

    expect(synth.cancel).toHaveBeenCalled()
    expect(onSpeakingChange).toHaveBeenCalledWith(true)
  })

  it('speaks the text with a Japanese voice and ja-JP locale', () => {
    speakJapanese('時間がありません。', vi.fn())
    vi.advanceTimersByTime(100)

    expect(spoken).toHaveLength(1)
    expect(spoken[0].text).toBe('時間がありません。')
    expect(spoken[0].lang).toBe('ja-JP')
    expect(spoken[0].voice).toEqual({ lang: 'ja-JP' })
  })

  it('reports not-speaking when the utterance ends', () => {
    const onSpeakingChange = vi.fn()
    speakJapanese('テスト', onSpeakingChange)
    vi.advanceTimersByTime(100)

    spoken[0].onend?.()

    expect(onSpeakingChange).toHaveBeenLastCalledWith(false)
  })

  it('reports not-speaking when the utterance errors', () => {
    const onSpeakingChange = vi.fn()
    speakJapanese('テスト', onSpeakingChange)
    vi.advanceTimersByTime(100)

    spoken[0].onerror?.()

    expect(onSpeakingChange).toHaveBeenLastCalledWith(false)
  })

  it('force-resets after 500ms when Safari silently drops the utterance', () => {
    const onSpeakingChange = vi.fn()
    speakJapanese('テスト', onSpeakingChange)
    vi.advanceTimersByTime(100)

    synth.speaking = false
    synth.pending = false
    vi.advanceTimersByTime(500)

    expect(onSpeakingChange).toHaveBeenLastCalledWith(false)
  })

  it('does not force-reset while speech is genuinely in flight', () => {
    const onSpeakingChange = vi.fn()
    speakJapanese('テスト', onSpeakingChange)
    vi.advanceTimersByTime(100)
    onSpeakingChange.mockClear()

    synth.speaking = true
    vi.advanceTimersByTime(500)

    expect(onSpeakingChange).not.toHaveBeenCalledWith(false)
  })

  it('does nothing when the browser has no speech synthesis', () => {
    vi.unstubAllGlobals()
    const onSpeakingChange = vi.fn()

    expect(() => speakJapanese('テスト', onSpeakingChange)).not.toThrow()
    expect(onSpeakingChange).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd fe && npm test -- src/lib/speech.test.ts`
Expected: FAIL — `Failed to resolve import "./speech"`.

- [ ] **Step 3: Write the implementation**

Create `fe/src/lib/speech.ts`. This is a faithful move of `Translator.tsx:90-140` — the two `setTimeout` calls are Safari workarounds and must be preserved exactly:

```ts
// speakJapanese reads text aloud with a Japanese voice.
//
// The two timeouts are Safari workarounds carried over from the original
// inline implementation: voices load asynchronously (so we wait 100ms before
// building the utterance), and Safari sometimes accepts speak() without ever
// firing onstart/onend (so we re-check the queue after 500ms and clear the
// speaking flag ourselves rather than leaving the button stuck disabled).
export function speakJapanese(
  text: string,
  onSpeakingChange: (speaking: boolean) => void
): void {
  if (!('speechSynthesis' in window)) return

  speechSynthesis.cancel()
  onSpeakingChange(true)

  setTimeout(() => {
    const utterance = new SpeechSynthesisUtterance(text)

    const japaneseVoice = speechSynthesis
      .getVoices()
      .find(voice => voice.lang.startsWith('ja') || voice.lang.includes('JP'))
    if (japaneseVoice) {
      utterance.voice = japaneseVoice
    }

    utterance.lang = 'ja-JP'
    utterance.rate = 0.8
    utterance.pitch = 1
    utterance.volume = 1

    utterance.onstart = () => onSpeakingChange(true)
    utterance.onend = () => onSpeakingChange(false)
    utterance.onerror = () => onSpeakingChange(false)

    speechSynthesis.speak(utterance)

    setTimeout(() => {
      if (!speechSynthesis.speaking && !speechSynthesis.pending) {
        onSpeakingChange(false)
      }
    }, 500)
  }, 100)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd fe && npm test -- src/lib/speech.test.ts`
Expected: PASS, 7 tests.

- [ ] **Step 5: Commit**

```bash
git add fe/src/lib/speech.ts fe/src/lib/speech.test.ts
git commit -m "refactor(fe): extract speakJapanese into lib/speech

Moves the speech-synthesis helper out of Translator unchanged, including
both Safari workarounds, and covers it with tests it never had."
```

---

### Task 3: Create the `useSettings` hook

The single source of truth for both preferences. `Translator.tsx:24,46-65,85-88,173-179` and `Mistakes.tsx:11,50-53,81-89` each hand-roll their own copy of this; both will be rewritten to consume it.

**Files:**
- Create: `fe/src/lib/useSettings.ts`
- Test: `fe/src/lib/useSettings.test.ts`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `LEVELS: number[]` — `[1, 2, 3, 4, 5]`
  - `type ExplainLanguage = 'en' | 'ja'`
  - `SELECTED_LEVELS_STORAGE_KEY = 'eagle:selectedLevels'`
  - `EXPLAIN_LANGUAGE_STORAGE_KEY = 'eagle:explainLanguage'`
  - `loadStoredLevels(): number[]`
  - `loadStoredLanguage(): ExplainLanguage`
  - `levelsForRequest(levels: number[]): number[] | undefined`
  - `toggleLevel(levels: number[], n: number): number[]`
  - `levelSummary(levels: number[]): string`
  - `useSettings(): { levels, language, setLevels, setLanguage }` where `setLevels: (next: number[]) => void` and `setLanguage: (next: ExplainLanguage) => void`, both persisting as a side effect.

- [ ] **Step 1: Write the failing test**

Create `fe/src/lib/useSettings.test.ts`:

```ts
import { renderHook, act } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'
import {
  LEVELS,
  SELECTED_LEVELS_STORAGE_KEY,
  EXPLAIN_LANGUAGE_STORAGE_KEY,
  loadStoredLevels,
  loadStoredLanguage,
  levelsForRequest,
  toggleLevel,
  levelSummary,
  useSettings,
} from './useSettings'

beforeEach(() => {
  localStorage.clear()
})

describe('loadStoredLevels', () => {
  it('defaults to every level when nothing is stored', () => {
    expect(loadStoredLevels()).toEqual(LEVELS)
  })

  it('restores a stored selection', () => {
    localStorage.setItem(SELECTED_LEVELS_STORAGE_KEY, JSON.stringify([2, 3]))
    expect(loadStoredLevels()).toEqual([2, 3])
  })

  it('falls back to every level when the stored value is malformed JSON', () => {
    localStorage.setItem(SELECTED_LEVELS_STORAGE_KEY, 'not json')
    expect(loadStoredLevels()).toEqual(LEVELS)
  })

  it('falls back to every level when the stored value is not an array of numbers', () => {
    localStorage.setItem(SELECTED_LEVELS_STORAGE_KEY, JSON.stringify(['1', '2']))
    expect(loadStoredLevels()).toEqual(LEVELS)
  })
})

describe('loadStoredLanguage', () => {
  it('defaults to English', () => {
    expect(loadStoredLanguage()).toBe('en')
  })

  it('restores a stored Japanese preference', () => {
    localStorage.setItem(EXPLAIN_LANGUAGE_STORAGE_KEY, 'ja')
    expect(loadStoredLanguage()).toBe('ja')
  })

  it('treats any unrecognised stored value as English', () => {
    localStorage.setItem(EXPLAIN_LANGUAGE_STORAGE_KEY, 'fr')
    expect(loadStoredLanguage()).toBe('en')
  })
})

describe('levelsForRequest', () => {
  it('sends no filter when every level is selected', () => {
    expect(levelsForRequest(LEVELS)).toBeUndefined()
  })

  it('sends no filter when no level is selected', () => {
    expect(levelsForRequest([])).toBeUndefined()
  })

  it('sends the selection when it is a strict subset', () => {
    expect(levelsForRequest([2, 3])).toEqual([2, 3])
  })
})

describe('toggleLevel', () => {
  it('removes a selected level', () => {
    expect(toggleLevel([1, 2, 3], 2)).toEqual([1, 3])
  })

  it('adds an unselected level in ascending order', () => {
    expect(toggleLevel([1, 4], 2)).toEqual([1, 2, 4])
  })

  it('does not mutate the input', () => {
    const levels = [1, 2, 3]
    toggleLevel(levels, 2)
    expect(levels).toEqual([1, 2, 3])
  })
})

describe('levelSummary', () => {
  it('reads All when every level is selected', () => {
    expect(levelSummary(LEVELS)).toBe('All')
  })

  it('reads All when no level is selected, because that also means any level', () => {
    expect(levelSummary([])).toBe('All')
  })

  it('lists a subset in ascending order', () => {
    expect(levelSummary([3, 1])).toBe('1, 3')
  })
})

describe('useSettings', () => {
  it('hydrates both preferences from storage', () => {
    localStorage.setItem(SELECTED_LEVELS_STORAGE_KEY, JSON.stringify([2]))
    localStorage.setItem(EXPLAIN_LANGUAGE_STORAGE_KEY, 'ja')

    const { result } = renderHook(() => useSettings())

    expect(result.current.levels).toEqual([2])
    expect(result.current.language).toBe('ja')
  })

  it('persists levels when they change', () => {
    const { result } = renderHook(() => useSettings())

    act(() => result.current.setLevels([1, 2]))

    expect(result.current.levels).toEqual([1, 2])
    expect(localStorage.getItem(SELECTED_LEVELS_STORAGE_KEY)).toBe('[1,2]')
  })

  it('persists the language when it changes', () => {
    const { result } = renderHook(() => useSettings())

    act(() => result.current.setLanguage('ja'))

    expect(result.current.language).toBe('ja')
    expect(localStorage.getItem(EXPLAIN_LANGUAGE_STORAGE_KEY)).toBe('ja')
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd fe && npm test -- src/lib/useSettings.test.ts`
Expected: FAIL — `Failed to resolve import "./useSettings"`.

- [ ] **Step 3: Write the implementation**

Create `fe/src/lib/useSettings.ts`. Note the `typeof window` guards: this app is statically exported, so module-level code runs during prerender where `localStorage` does not exist. `Mistakes.tsx:51` already uses this guard; `Translator.tsx:86` does not, and standardizing on the guarded form removes that hazard.

```ts
'use client'

import { useEffect, useState } from 'react'

export const LEVELS = [1, 2, 3, 4, 5]

export type ExplainLanguage = 'en' | 'ja'

export const SELECTED_LEVELS_STORAGE_KEY = 'eagle:selectedLevels'
export const EXPLAIN_LANGUAGE_STORAGE_KEY = 'eagle:explainLanguage'

export function loadStoredLevels(): number[] {
  if (typeof window === 'undefined') return LEVELS
  try {
    const raw = window.localStorage.getItem(SELECTED_LEVELS_STORAGE_KEY)
    if (!raw) return LEVELS
    const parsed = JSON.parse(raw)
    if (Array.isArray(parsed) && parsed.every((n): n is number => typeof n === 'number')) {
      return parsed
    }
  } catch {
    // ignore malformed storage, fall back to default
  }
  return LEVELS
}

export function loadStoredLanguage(): ExplainLanguage {
  if (typeof window === 'undefined') return 'en'
  return window.localStorage.getItem(EXPLAIN_LANGUAGE_STORAGE_KEY) === 'ja' ? 'ja' : 'en'
}

// levelsForRequest returns undefined when the selection carries no
// information — every level and no level both mean "any level" to the API.
export function levelsForRequest(levels: number[]): number[] | undefined {
  return levels.length === 0 || levels.length === LEVELS.length ? undefined : levels
}

export function toggleLevel(levels: number[], n: number): number[] {
  return levels.includes(n)
    ? levels.filter(l => l !== n)
    : [...levels, n].sort((a, b) => a - b)
}

export function levelSummary(levels: number[]): string {
  return levels.length === 0 || levels.length === LEVELS.length
    ? 'All'
    : [...levels].sort((a, b) => a - b).join(', ')
}

export function useSettings() {
  const [levels, setLevelsState] = useState<number[]>(LEVELS)
  const [language, setLanguageState] = useState<ExplainLanguage>('en')

  useEffect(() => {
    setLevelsState(loadStoredLevels())
    setLanguageState(loadStoredLanguage())
  }, [])

  const setLevels = (next: number[]) => {
    setLevelsState(next)
    window.localStorage.setItem(SELECTED_LEVELS_STORAGE_KEY, JSON.stringify(next))
  }

  const setLanguage = (next: ExplainLanguage) => {
    setLanguageState(next)
    window.localStorage.setItem(EXPLAIN_LANGUAGE_STORAGE_KEY, next)
  }

  return { levels, language, setLevels, setLanguage }
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd fe && npm test -- src/lib/useSettings.test.ts`
Expected: PASS, 17 tests.

- [ ] **Step 5: Commit**

```bash
git add fe/src/lib/useSettings.ts fe/src/lib/useSettings.test.ts
git commit -m "feat(fe): add useSettings as the single owner of user preferences

Translator and Mistakes each declared their own copy of the language
storage key and their own read/parse/persist logic. This hook consolidates
both preferences so neither page touches localStorage directly."
```

---

### Task 4: Build the `Sheet` primitive

**Files:**
- Create: `fe/src/components/ui/sheet.tsx`
- Test: `fe/src/components/ui/sheet.test.tsx`

**Interfaces:**
- Consumes: `cn` from `@/lib/utils`.
- Produces: `Sheet({ open, onClose, title, children }: { open: boolean; onClose: () => void; title: string; children: React.ReactNode })`

- [ ] **Step 1: Write the failing test**

Create `fe/src/components/ui/sheet.test.tsx`:

```tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { Sheet } from './sheet'

describe('Sheet', () => {
  it('renders nothing when closed', () => {
    render(
      <Sheet open={false} onClose={vi.fn()} title="Settings">
        <p>body</p>
      </Sheet>
    )
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('renders a labelled modal dialog with its children when open', () => {
    render(
      <Sheet open onClose={vi.fn()} title="Settings">
        <p>body</p>
      </Sheet>
    )
    const dialog = screen.getByRole('dialog', { name: 'Settings' })
    expect(dialog).toHaveAttribute('aria-modal', 'true')
    expect(screen.getByText('body')).toBeInTheDocument()
  })

  it('closes when the backdrop is clicked', () => {
    const onClose = vi.fn()
    render(
      <Sheet open onClose={onClose} title="Settings">
        <p>body</p>
      </Sheet>
    )
    fireEvent.click(screen.getByTestId('sheet-backdrop'))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('does not close when the panel itself is clicked', () => {
    const onClose = vi.fn()
    render(
      <Sheet open onClose={onClose} title="Settings">
        <p>body</p>
      </Sheet>
    )
    fireEvent.click(screen.getByText('body'))
    expect(onClose).not.toHaveBeenCalled()
  })

  it('closes when Escape is pressed', () => {
    const onClose = vi.fn()
    render(
      <Sheet open onClose={onClose} title="Settings">
        <p>body</p>
      </Sheet>
    )
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('ignores Escape while closed', () => {
    const onClose = vi.fn()
    render(
      <Sheet open={false} onClose={onClose} title="Settings">
        <p>body</p>
      </Sheet>
    )
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).not.toHaveBeenCalled()
  })

  it('offers an explicit close button', () => {
    const onClose = vi.fn()
    render(
      <Sheet open onClose={onClose} title="Settings">
        <p>body</p>
      </Sheet>
    )
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd fe && npm test -- src/components/ui/sheet.test.tsx`
Expected: FAIL — `Failed to resolve import "./sheet"`.

- [ ] **Step 3: Write the implementation**

Create `fe/src/components/ui/sheet.tsx`. The Escape listener mirrors the pattern already in `Translator.tsx:277-286`, which this replaces:

```tsx
'use client'

import * as React from 'react'
import { X } from 'lucide-react'
import { cn } from '@/lib/utils'

interface SheetProps {
  open: boolean
  onClose: () => void
  title: string
  children: React.ReactNode
  className?: string
}

export function Sheet({ open, onClose, title, children, className }: SheetProps) {
  React.useEffect(() => {
    if (!open) return
    const closeOnEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', closeOnEscape)
    return () => document.removeEventListener('keydown', closeOnEscape)
  }, [open, onClose])

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center">
      <div
        data-testid="sheet-backdrop"
        className="absolute inset-0 bg-black/25"
        onClick={onClose}
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className={cn(
          'relative w-full max-w-2xl rounded-t-2xl border border-border bg-card p-5 pb-8 shadow-lg',
          className
        )}
      >
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-base font-semibold text-card-foreground">{title}</h2>
          <button
            type="button"
            aria-label="Close"
            onClick={onClose}
            className="rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        {children}
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd fe && npm test -- src/components/ui/sheet.test.tsx`
Expected: PASS, 7 tests.

- [ ] **Step 5: Commit**

```bash
git add fe/src/components/ui/sheet.tsx fe/src/components/ui/sheet.test.tsx
git commit -m "feat(fe): add a bottom Sheet primitive

Backdrop, Escape and an explicit close button, replacing the ad-hoc
outside-click handling that lived inline in Translator's level dropdown."
```

---

### Task 5: Build the `Segmented` primitive

**Files:**
- Create: `fe/src/components/ui/segmented.tsx`
- Test: `fe/src/components/ui/segmented.test.tsx`

**Interfaces:**
- Consumes: `cn` from `@/lib/utils`.
- Produces:
  - `interface SegmentedOption { value: string; label: string }`
  - `Segmented({ options, value, onChange, label }: { options: SegmentedOption[]; value: string; onChange: (value: string) => void; label: string })`

Rendered as a `tablist` of `tab` buttons. The e2e suite selects the Explain tab by `getByRole('tab', { name: 'Explain' })`, so these roles are contractual.

- [ ] **Step 1: Write the failing test**

Create `fe/src/components/ui/segmented.test.tsx`:

```tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { Segmented } from './segmented'

const options = [
  { value: 'answer', label: 'Answer' },
  { value: 'attempts', label: 'Attempts 2' },
  { value: 'explain', label: 'Explain' },
]

describe('Segmented', () => {
  it('renders a labelled tablist with one tab per option', () => {
    render(<Segmented options={options} value="answer" onChange={vi.fn()} label="Review" />)

    expect(screen.getByRole('tablist', { name: 'Review' })).toBeInTheDocument()
    expect(screen.getAllByRole('tab')).toHaveLength(3)
    expect(screen.getByRole('tab', { name: 'Explain' })).toBeInTheDocument()
  })

  it('marks only the active option as selected', () => {
    render(<Segmented options={options} value="attempts" onChange={vi.fn()} label="Review" />)

    expect(screen.getByRole('tab', { name: 'Attempts 2' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('tab', { name: 'Answer' })).toHaveAttribute('aria-selected', 'false')
  })

  it('reports the chosen value when a tab is clicked', () => {
    const onChange = vi.fn()
    render(<Segmented options={options} value="answer" onChange={onChange} label="Review" />)

    fireEvent.click(screen.getByRole('tab', { name: 'Explain' }))

    expect(onChange).toHaveBeenCalledWith('explain')
  })

  it('moves to the next tab on ArrowRight', () => {
    const onChange = vi.fn()
    render(<Segmented options={options} value="answer" onChange={onChange} label="Review" />)

    fireEvent.keyDown(screen.getByRole('tab', { name: 'Answer' }), { key: 'ArrowRight' })

    expect(onChange).toHaveBeenCalledWith('attempts')
  })

  it('wraps from the last tab to the first on ArrowRight', () => {
    const onChange = vi.fn()
    render(<Segmented options={options} value="explain" onChange={onChange} label="Review" />)

    fireEvent.keyDown(screen.getByRole('tab', { name: 'Explain' }), { key: 'ArrowRight' })

    expect(onChange).toHaveBeenCalledWith('answer')
  })

  it('wraps from the first tab to the last on ArrowLeft', () => {
    const onChange = vi.fn()
    render(<Segmented options={options} value="answer" onChange={onChange} label="Review" />)

    fireEvent.keyDown(screen.getByRole('tab', { name: 'Answer' }), { key: 'ArrowLeft' })

    expect(onChange).toHaveBeenCalledWith('explain')
  })

  it('keeps only the active tab in the tab order', () => {
    render(<Segmented options={options} value="attempts" onChange={vi.fn()} label="Review" />)

    expect(screen.getByRole('tab', { name: 'Attempts 2' })).toHaveAttribute('tabindex', '0')
    expect(screen.getByRole('tab', { name: 'Answer' })).toHaveAttribute('tabindex', '-1')
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd fe && npm test -- src/components/ui/segmented.test.tsx`
Expected: FAIL — `Failed to resolve import "./segmented"`.

- [ ] **Step 3: Write the implementation**

Create `fe/src/components/ui/segmented.tsx`:

```tsx
'use client'

import * as React from 'react'
import { cn } from '@/lib/utils'

export interface SegmentedOption {
  value: string
  label: string
}

interface SegmentedProps {
  options: SegmentedOption[]
  value: string
  onChange: (value: string) => void
  label: string
  className?: string
}

export function Segmented({ options, value, onChange, label, className }: SegmentedProps) {
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key !== 'ArrowRight' && e.key !== 'ArrowLeft') return
    e.preventDefault()
    const index = options.findIndex(o => o.value === value)
    const delta = e.key === 'ArrowRight' ? 1 : -1
    const next = (index + delta + options.length) % options.length
    onChange(options[next].value)
  }

  return (
    <div
      role="tablist"
      aria-label={label}
      onKeyDown={handleKeyDown}
      className={cn('flex gap-1 rounded-lg border border-border bg-muted p-1', className)}
    >
      {options.map(option => {
        const selected = option.value === value
        return (
          <button
            key={option.value}
            type="button"
            role="tab"
            aria-selected={selected}
            tabIndex={selected ? 0 : -1}
            onClick={() => onChange(option.value)}
            className={cn(
              'flex-1 rounded-md px-2 py-1.5 text-xs transition-colors',
              selected
                ? 'bg-card font-semibold text-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground'
            )}
          >
            {option.label}
          </button>
        )
      })}
    </div>
  )
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd fe && npm test -- src/components/ui/segmented.test.tsx`
Expected: PASS, 7 tests.

- [ ] **Step 5: Commit**

```bash
git add fe/src/components/ui/segmented.tsx fe/src/components/ui/segmented.test.tsx
git commit -m "feat(fe): add a Segmented tablist primitive"
```

---

### Task 6: Build `SettingsSheet`

**Files:**
- Create: `fe/src/components/SettingsSheet.tsx`
- Test: `fe/src/components/SettingsSheet.test.tsx`

**Interfaces:**
- Consumes: `Sheet` (Task 4), `Segmented` (Task 5), `LEVELS` / `toggleLevel` / `ExplainLanguage` (Task 3).
- Produces:
  ```ts
  interface SettingsSheetProps {
    open: boolean
    onClose: () => void
    levels: number[]
    onLevelsChange: (levels: number[]) => void
    language: ExplainLanguage
    onLanguageChange: (language: ExplainLanguage) => void
  }
  ```

The level controls stay real checkboxes with `aria-label="Level N"` — `e2e/tests/level-filter.spec.ts:13-44` selects them that way and those five lines must keep working untouched.

- [ ] **Step 1: Write the failing test**

Create `fe/src/components/SettingsSheet.test.tsx`:

```tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import SettingsSheet from './SettingsSheet'

function renderSheet(overrides = {}) {
  const props = {
    open: true,
    onClose: vi.fn(),
    levels: [1, 2, 3, 4, 5],
    onLevelsChange: vi.fn(),
    language: 'en' as const,
    onLanguageChange: vi.fn(),
    ...overrides,
  }
  render(<SettingsSheet {...props} />)
  return props
}

describe('SettingsSheet', () => {
  it('renders a checkbox per level, reflecting the current selection', () => {
    renderSheet({ levels: [1, 3] })

    expect(screen.getByRole('checkbox', { name: 'Level 1' })).toBeChecked()
    expect(screen.getByRole('checkbox', { name: 'Level 2' })).not.toBeChecked()
    expect(screen.getByRole('checkbox', { name: 'Level 3' })).toBeChecked()
  })

  it('reports the level added when an unchecked level is clicked', () => {
    const props = renderSheet({ levels: [1, 3] })

    fireEvent.click(screen.getByRole('checkbox', { name: 'Level 2' }))

    expect(props.onLevelsChange).toHaveBeenCalledWith([1, 2, 3])
  })

  it('reports the level removed when a checked level is clicked', () => {
    const props = renderSheet({ levels: [1, 2, 3] })

    fireEvent.click(screen.getByRole('checkbox', { name: 'Level 2' }))

    expect(props.onLevelsChange).toHaveBeenCalledWith([1, 3])
  })

  it('shows the active AI language as the selected tab', () => {
    renderSheet({ language: 'ja' })

    expect(screen.getByRole('tab', { name: '日本語' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('tab', { name: 'English' })).toHaveAttribute('aria-selected', 'false')
  })

  it('reports a language change', () => {
    const props = renderSheet({ language: 'en' })

    fireEvent.click(screen.getByRole('tab', { name: '日本語' }))

    expect(props.onLanguageChange).toHaveBeenCalledWith('ja')
  })

  it('explains what the language setting affects', () => {
    renderSheet()
    expect(screen.getByText('Used for explanations and weakness insight')).toBeInTheDocument()
  })

  it('renders nothing when closed', () => {
    renderSheet({ open: false })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd fe && npm test -- src/components/SettingsSheet.test.tsx`
Expected: FAIL — `Failed to resolve import "./SettingsSheet"`.

- [ ] **Step 3: Write the implementation**

Create `fe/src/components/SettingsSheet.tsx`:

```tsx
'use client'

import { Sheet } from '@/components/ui/sheet'
import { Segmented } from '@/components/ui/segmented'
import { LEVELS, toggleLevel, type ExplainLanguage } from '@/lib/useSettings'

interface SettingsSheetProps {
  open: boolean
  onClose: () => void
  levels: number[]
  onLevelsChange: (levels: number[]) => void
  language: ExplainLanguage
  onLanguageChange: (language: ExplainLanguage) => void
}

const LANGUAGE_OPTIONS = [
  { value: 'en', label: 'English' },
  { value: 'ja', label: '日本語' },
]

export default function SettingsSheet({
  open,
  onClose,
  levels,
  onLevelsChange,
  language,
  onLanguageChange,
}: SettingsSheetProps) {
  return (
    <Sheet open={open} onClose={onClose} title="Settings">
      <div className="mb-6">
        <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          Sentence levels
        </div>
        <div className="flex flex-wrap gap-2">
          {LEVELS.map(n => {
            const checked = levels.includes(n)
            return (
              <label
                key={n}
                className={
                  checked
                    ? 'cursor-pointer rounded-full border border-primary bg-primary px-3 py-1 text-sm font-semibold text-primary-foreground'
                    : 'cursor-pointer rounded-full border border-border bg-card px-3 py-1 text-sm text-muted-foreground'
                }
              >
                <input
                  type="checkbox"
                  className="sr-only"
                  checked={checked}
                  aria-label={`Level ${n}`}
                  onChange={() => onLevelsChange(toggleLevel(levels, n))}
                />
                {n}
              </label>
            )
          })}
        </div>
      </div>

      <div>
        <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          AI language
        </div>
        <Segmented
          options={LANGUAGE_OPTIONS}
          value={language}
          onChange={value => onLanguageChange(value as ExplainLanguage)}
          label="AI language"
        />
        <p className="mt-2 text-xs text-muted-foreground">
          Used for explanations and weakness insight
        </p>
      </div>
    </Sheet>
  )
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd fe && npm test -- src/components/SettingsSheet.test.tsx`
Expected: PASS, 7 tests.

- [ ] **Step 5: Commit**

```bash
git add fe/src/components/SettingsSheet.tsx fe/src/components/SettingsSheet.test.tsx
git commit -m "feat(fe): add SettingsSheet holding levels and AI language

One home for both preferences, replacing the level dropdown in the card
header and the two separate EN/JA toggles."
```

---

### Task 7: Build `AppHeader`

**Files:**
- Create: `fe/src/components/AppHeader.tsx`
- Test: `fe/src/components/AppHeader.test.tsx`

**Interfaces:**
- Consumes: existing `UserMenu` (`fe/src/components/UserMenu.tsx`), unchanged.
- Produces:
  ```ts
  interface AppHeaderProps {
    user: User            // from 'firebase/auth'
    onOpenSettings: () => void
    showMistakesLink?: boolean   // default true
  }
  ```

The `<h1>Eagle</h1>` and the link named `Mistakes` are contractual — `e2e/tests/auth.spec.ts`, `e2e/tests/mistakes.spec.ts:11,18` depend on both.

- [ ] **Step 1: Write the failing test**

Create `fe/src/components/AppHeader.test.tsx`:

```tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import type { User } from 'firebase/auth'

vi.mock('@/lib/firebase', () => ({ auth: {} }))
vi.mock('firebase/auth', () => ({ signOut: vi.fn() }))

import AppHeader from './AppHeader'

const fakeUser = { uid: 'u1', displayName: 'Jane' } as User

describe('AppHeader', () => {
  it('renders Eagle as a heading linking home', () => {
    render(<AppHeader user={fakeUser} onOpenSettings={vi.fn()} />)

    expect(screen.getByRole('heading', { name: 'Eagle' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Eagle' })).toHaveAttribute('href', '/')
  })

  it('links to the mistakes page by default', () => {
    render(<AppHeader user={fakeUser} onOpenSettings={vi.fn()} />)

    expect(screen.getByRole('link', { name: 'Mistakes' })).toHaveAttribute('href', '/mistakes')
  })

  it('hides the mistakes link when asked', () => {
    render(<AppHeader user={fakeUser} onOpenSettings={vi.fn()} showMistakesLink={false} />)

    expect(screen.queryByRole('link', { name: 'Mistakes' })).not.toBeInTheDocument()
  })

  it('opens settings when the gear is clicked', () => {
    const onOpenSettings = vi.fn()
    render(<AppHeader user={fakeUser} onOpenSettings={onOpenSettings} />)

    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))

    expect(onOpenSettings).toHaveBeenCalledTimes(1)
  })

  it('renders the user menu', () => {
    render(<AppHeader user={fakeUser} onOpenSettings={vi.fn()} />)

    expect(screen.getByRole('button', { name: /jane/i })).toBeInTheDocument()
  })
})
```

If the last assertion does not match `UserMenu`'s actual trigger, open `fe/src/components/UserMenu.tsx` and `UserMenu.test.tsx` and copy the selector they already use — do not change `UserMenu`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd fe && npm test -- src/components/AppHeader.test.tsx`
Expected: FAIL — `Failed to resolve import "./AppHeader"`.

- [ ] **Step 3: Write the implementation**

Create `fe/src/components/AppHeader.tsx`:

```tsx
'use client'

import Image from 'next/image'
import Link from 'next/link'
import { Settings } from 'lucide-react'
import type { User } from 'firebase/auth'
import UserMenu from './UserMenu'

interface AppHeaderProps {
  user: User
  onOpenSettings: () => void
  showMistakesLink?: boolean
}

export default function AppHeader({
  user,
  onOpenSettings,
  showMistakesLink = true,
}: AppHeaderProps) {
  return (
    <header className="mb-6 flex items-center justify-between gap-2">
      <Link href="/" className="flex items-center gap-2">
        <Image src="/eagle-thumbnail.png" alt="" width={28} height={28} />
        <h1 className="text-xl font-bold text-foreground">Eagle</h1>
      </Link>

      <div className="flex items-center gap-2">
        {showMistakesLink && (
          <Link
            href="/mistakes"
            className="rounded-md px-2 py-1 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground"
          >
            Mistakes
          </Link>
        )}
        <button
          type="button"
          aria-label="Settings"
          onClick={onOpenSettings}
          className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
        >
          <Settings className="h-5 w-5" />
        </button>
        <UserMenu user={user} />
      </div>
    </header>
  )
}
```

The logo `alt` is deliberately empty: the adjacent `<h1>Eagle</h1>` already names the link, and a non-empty alt would make the accessible name `"Eagle logo Eagle"` and break `getByRole('link', { name: 'Eagle' })`.

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd fe && npm test -- src/components/AppHeader.test.tsx`
Expected: PASS, 5 tests.

- [ ] **Step 5: Commit**

```bash
git add fe/src/components/AppHeader.tsx fe/src/components/AppHeader.test.tsx
git commit -m "feat(fe): add AppHeader shared by the practice and mistakes pages

Pulls navigation, settings and the user menu out of the question card
header, which was carrying a title, a description, a nav link and the
level filter in one row."
```

---

### Task 8: Build `QuestionCard`

**Files:**
- Create: `fe/src/components/QuestionCard.tsx`
- Test: `fe/src/components/QuestionCard.test.tsx`

**Interfaces:**
- Consumes: `Sentence` from `@/lib/api`, `Card` / `CardContent` from `@/components/ui/card`.
- Produces:
  ```ts
  interface QuestionCardProps {
    sentence: Sentence
    correctCount: number
    incorrectCount: number
    levelSummary: string
    isSpeaking: boolean
    onSpeak: () => void
  }
  ```

`Correct: N` and `Incorrect: N` are frozen strings — `e2e/tests/correct-answer.spec.ts:7,14` and `e2e/tests/incorrect-explain.spec.ts:7,14` match them with `/^Correct: \d+$/` and `/^Incorrect: \d+$/`, so each must be the entire text content of its own element.

- [ ] **Step 1: Write the failing test**

Create `fe/src/components/QuestionCard.test.tsx`:

```tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import QuestionCard from './QuestionCard'
import type { Sentence } from '@/lib/api'

const sentence: Sentence = {
  id: 1,
  japanese: '時間がありません。',
  english: "I don't have time.",
  page: '12',
  level: 2,
  correct_count: 5,
  incorrect_count: 2,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

function renderCard(overrides = {}) {
  const props = {
    sentence,
    correctCount: 5,
    incorrectCount: 2,
    levelSummary: 'All',
    isSpeaking: false,
    onSpeak: vi.fn(),
    ...overrides,
  }
  render(<QuestionCard {...props} />)
  return props
}

describe('QuestionCard', () => {
  it('shows the japanese sentence', () => {
    renderCard()
    expect(screen.getByText('時間がありません。')).toBeInTheDocument()
  })

  it('shows the counters as their own exact strings', () => {
    renderCard()
    expect(screen.getByText(/^Correct: 5$/)).toBeInTheDocument()
    expect(screen.getByText(/^Incorrect: 2$/)).toBeInTheDocument()
  })

  it('shows the active level summary', () => {
    renderCard({ levelSummary: '1, 3' })
    expect(screen.getByText('Level: 1, 3')).toBeInTheDocument()
  })

  it('speaks when the listen button is clicked', () => {
    const props = renderCard()
    fireEvent.click(screen.getByRole('button', { name: 'Listen' }))
    expect(props.onSpeak).toHaveBeenCalledTimes(1)
  })

  it('disables the listen button while speaking', () => {
    renderCard({ isSpeaking: true })
    expect(screen.getByRole('button', { name: 'Listen' })).toBeDisabled()
  })

  it('never reveals the english answer', () => {
    renderCard()
    expect(screen.queryByText("I don't have time.")).not.toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd fe && npm test -- src/components/QuestionCard.test.tsx`
Expected: FAIL — `Failed to resolve import "./QuestionCard"`.

- [ ] **Step 3: Write the implementation**

Create `fe/src/components/QuestionCard.tsx`:

```tsx
'use client'

import { Volume2 } from 'lucide-react'
import { Card, CardContent } from '@/components/ui/card'
import type { Sentence } from '@/lib/api'

interface QuestionCardProps {
  sentence: Sentence
  correctCount: number
  incorrectCount: number
  levelSummary: string
  isSpeaking: boolean
  onSpeak: () => void
}

export default function QuestionCard({
  sentence,
  correctCount,
  incorrectCount,
  levelSummary,
  isSpeaking,
  onSpeak,
}: QuestionCardProps) {
  return (
    <Card>
      <CardContent className="p-5 text-center">
        <div className="mb-3 flex items-center justify-center gap-3">
          <p className="text-2xl font-bold text-foreground">{sentence.japanese}</p>
          <button
            type="button"
            aria-label="Listen"
            onClick={onSpeak}
            disabled={isSpeaking}
            className="rounded-md border border-border p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground disabled:opacity-50"
          >
            <Volume2 className="h-4 w-4" />
          </button>
        </div>
        <div className="flex justify-center gap-3 text-xs text-muted-foreground">
          <span>Correct: {correctCount}</span>
          <span aria-hidden="true">·</span>
          <span>Incorrect: {incorrectCount}</span>
          <span aria-hidden="true">·</span>
          <span>Level: {levelSummary}</span>
        </div>
      </CardContent>
    </Card>
  )
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd fe && npm test -- src/components/QuestionCard.test.tsx`
Expected: PASS, 6 tests.

- [ ] **Step 5: Commit**

```bash
git add fe/src/components/QuestionCard.tsx fe/src/components/QuestionCard.test.tsx
git commit -m "feat(fe): add QuestionCard for the sentence, listen button and counters"
```

---

### Task 9: Build `ReviewPanel`

The centerpiece. Verdict chip plus a segmented control whose tabs appear only when they apply.

**Files:**
- Create: `fe/src/components/ReviewPanel.tsx`
- Test: `fe/src/components/ReviewPanel.test.tsx`

**Interfaces:**
- Consumes: `Segmented` / `SegmentedOption` (Task 5), `AnswerHistory` from `@/lib/api`, `Card` / `CardContent` from `@/components/ui/card`.
- Produces:
  ```ts
  export type ReviewTab = 'answer' | 'attempts' | 'explain'

  interface ReviewPanelProps {
    feedback: 'correct' | 'incorrect'
    userAnswer: string
    correctAnswer: string
    histories: AnswerHistory[]
    tab: ReviewTab
    onTabChange: (tab: ReviewTab) => void
    explanation: string | null
    explaining: boolean
    explainError: string | null
    onRetryExplain: () => void
  }
  ```

Tab state is owned by the container so `resetQuestionState` can clear it. Fetching on tab-select is the container's job too — this component only reports the change.

- [ ] **Step 1: Write the failing test**

Create `fe/src/components/ReviewPanel.test.tsx`:

```tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import ReviewPanel from './ReviewPanel'

const histories = [
  { id: 1, incorrect_answer: 'There is no time.', created_at: '2026-01-01T00:00:00Z' },
  { id: 2, incorrect_answer: 'I have not time.', created_at: '2026-01-02T00:00:00Z' },
]

function renderPanel(overrides = {}) {
  const props = {
    feedback: 'incorrect' as const,
    userAnswer: 'I have no time.',
    correctAnswer: "I don't have time.",
    histories: [],
    tab: 'answer' as const,
    onTabChange: vi.fn(),
    explanation: null,
    explaining: false,
    explainError: null,
    onRetryExplain: vi.fn(),
    ...overrides,
  }
  render(<ReviewPanel {...props} />)
  return props
}

describe('verdict', () => {
  it('shows the incorrect verdict copy', () => {
    renderPanel({ feedback: 'incorrect' })
    expect(screen.getByText('Not quite right. Try again!')).toBeInTheDocument()
  })

  it('shows the correct verdict copy', () => {
    renderPanel({ feedback: 'correct' })
    expect(screen.getByText('Correct! Well done!')).toBeInTheDocument()
  })
})

describe('tabs', () => {
  it('renders no tab control at all when correct with no history', () => {
    renderPanel({ feedback: 'correct', histories: [] })

    expect(screen.queryByRole('tab')).not.toBeInTheDocument()
    expect(screen.getByText("I don't have time.")).toBeInTheDocument()
  })

  it('adds Explain when the answer was incorrect', () => {
    renderPanel({ feedback: 'incorrect', histories: [] })

    expect(screen.getByRole('tab', { name: 'Explain' })).toBeInTheDocument()
  })

  it('adds Attempts with a count when there is history', () => {
    renderPanel({ histories })

    expect(screen.getByRole('tab', { name: 'Attempts 2' })).toBeInTheDocument()
  })

  it('omits Attempts when there is no history', () => {
    renderPanel({ histories: [] })

    expect(screen.queryByRole('tab', { name: /^Attempts/ })).not.toBeInTheDocument()
  })

  it('reports the selected tab', () => {
    const props = renderPanel({ feedback: 'incorrect' })

    fireEvent.click(screen.getByRole('tab', { name: 'Explain' }))

    expect(props.onTabChange).toHaveBeenCalledWith('explain')
  })
})

describe('answer tab', () => {
  it('shows the correct answer', () => {
    renderPanel({ tab: 'answer' })
    expect(screen.getByText("I don't have time.")).toBeInTheDocument()
  })

  it('shows what the user wrote when they were wrong', () => {
    renderPanel({ tab: 'answer', feedback: 'incorrect' })
    expect(screen.getByText('I have no time.')).toBeInTheDocument()
  })

  it('does not repeat the user answer when they were right', () => {
    renderPanel({ tab: 'answer', feedback: 'correct', userAnswer: "I don't have time." })
    expect(screen.queryByText('You wrote')).not.toBeInTheDocument()
  })
})

describe('attempts tab', () => {
  it('lists every previous incorrect answer', () => {
    renderPanel({ tab: 'attempts', histories })

    expect(screen.getByText('There is no time.')).toBeInTheDocument()
    expect(screen.getByText('I have not time.')).toBeInTheDocument()
  })
})

describe('explain tab', () => {
  it('shows a loading state while fetching', () => {
    renderPanel({ tab: 'explain', explaining: true })
    expect(screen.getByText('Explaining...')).toBeInTheDocument()
  })

  it('renders markdown as real bold text', () => {
    renderPanel({ tab: 'explain', explanation: 'Prefer **do-support** here.' })

    expect(screen.getByText('do-support').tagName).toBe('STRONG')
  })

  it('shows an error with a retry that re-requests the explanation', () => {
    const props = renderPanel({ tab: 'explain', explainError: 'Failed to load explanation' })

    expect(screen.getByText('Failed to load explanation')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Try Again' }))
    expect(props.onRetryExplain).toHaveBeenCalledTimes(1)
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd fe && npm test -- src/components/ReviewPanel.test.tsx`
Expected: FAIL — `Failed to resolve import "./ReviewPanel"`.

- [ ] **Step 3: Write the implementation**

Create `fe/src/components/ReviewPanel.tsx`. This is the only file in the plan permitted to use hue, and only on the verdict chip:

```tsx
'use client'

import ReactMarkdown from 'react-markdown'
import { CheckCircle, XCircle } from 'lucide-react'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Segmented, type SegmentedOption } from '@/components/ui/segmented'
import type { AnswerHistory } from '@/lib/api'

export type ReviewTab = 'answer' | 'attempts' | 'explain'

const explanationMarkdownComponents = {
  p: (props: React.ComponentPropsWithoutRef<'p'>) => (
    <p className="mb-2 whitespace-pre-line last:mb-0" {...props} />
  ),
  ul: (props: React.ComponentPropsWithoutRef<'ul'>) => (
    <ul className="mb-2 list-disc space-y-1 pl-5 last:mb-0" {...props} />
  ),
  ol: (props: React.ComponentPropsWithoutRef<'ol'>) => (
    <ol className="mb-2 list-decimal space-y-1 pl-5 last:mb-0" {...props} />
  ),
  li: (props: React.ComponentPropsWithoutRef<'li'>) => <li {...props} />,
  strong: (props: React.ComponentPropsWithoutRef<'strong'>) => (
    <strong className="font-semibold text-foreground" {...props} />
  ),
}

interface ReviewPanelProps {
  feedback: 'correct' | 'incorrect'
  userAnswer: string
  correctAnswer: string
  histories: AnswerHistory[]
  tab: ReviewTab
  onTabChange: (tab: ReviewTab) => void
  explanation: string | null
  explaining: boolean
  explainError: string | null
  onRetryExplain: () => void
}

const LABEL = 'mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground'

export default function ReviewPanel({
  feedback,
  userAnswer,
  correctAnswer,
  histories,
  tab,
  onTabChange,
  explanation,
  explaining,
  explainError,
  onRetryExplain,
}: ReviewPanelProps) {
  const correct = feedback === 'correct'

  const options: SegmentedOption[] = [{ value: 'answer', label: 'Answer' }]
  if (histories.length > 0) {
    options.push({ value: 'attempts', label: `Attempts ${histories.length}` })
  }
  if (!correct) {
    options.push({ value: 'explain', label: 'Explain' })
  }

  return (
    <Card>
      <CardContent className="p-5">
        <div
          className={
            correct
              ? 'mb-4 inline-flex items-center gap-1.5 rounded-full border border-success-subtle-border bg-success-subtle px-3 py-1 text-sm font-semibold text-success-subtle-foreground'
              : 'mb-4 inline-flex items-center gap-1.5 rounded-full border border-destructive-subtle-border bg-destructive-subtle px-3 py-1 text-sm font-semibold text-destructive-subtle-foreground'
          }
        >
          {correct ? <CheckCircle className="h-4 w-4" /> : <XCircle className="h-4 w-4" />}
          {correct ? 'Correct! Well done!' : 'Not quite right. Try again!'}
        </div>

        {options.length > 1 && (
          <Segmented
            options={options}
            value={tab}
            onChange={value => onTabChange(value as ReviewTab)}
            label="Review"
            className="mb-3"
          />
        )}
        <div className="rounded-lg border border-border bg-muted p-3 text-sm">
          {tab === 'answer' && (
            <>
              {!correct && (
                <>
                  <div className={LABEL}>You wrote</div>
                  <p className="mb-3 text-muted-foreground line-through">{userAnswer}</p>
                </>
              )}
              <div className={LABEL}>Correct</div>
              <p className="font-semibold text-foreground">{correctAnswer}</p>
            </>
          )}

          {tab === 'attempts' && (
            <ul className="space-y-1.5">
              {histories.map(history => (
                <li key={history.id} className="text-muted-foreground line-through">
                  {history.incorrect_answer}
                </li>
              ))}
            </ul>
          )}

          {tab === 'explain' && (
            <>
              {explaining && <p className="text-muted-foreground">Explaining...</p>}
              {!explaining && explainError && (
                <div className="space-y-2">
                  <p className="text-destructive">{explainError}</p>
                  <Button variant="outline" size="sm" onClick={onRetryExplain}>
                    Try Again
                  </Button>
                </div>
              )}
              {!explaining && !explainError && explanation && (
                <div className="text-foreground">
                  <ReactMarkdown
                    components={explanationMarkdownComponents}
                    disallowedElements={['a', 'img']}
                  >
                    {explanation}
                  </ReactMarkdown>
                </div>
              )}
            </>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
```

When only the `Answer` tab applies, no tab control renders at all — a one-option segmented control is noise. The panel body still renders; `tab` is `'answer'` in that state because the container resets it on every check.

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd fe && npm test -- src/components/ReviewPanel.test.tsx`
Expected: PASS, 14 tests.

- [ ] **Step 5: Commit**

```bash
git add fe/src/components/ReviewPanel.tsx fe/src/components/ReviewPanel.test.tsx
git commit -m "feat(fe): add ReviewPanel with a verdict chip and conditional tabs

Replaces the stack of blue, yellow and purple result blocks with one
neutral surface. Attempts appears only with history, Explain only when
the answer was wrong."
```

---

### Task 10: Rewrite `Translator` — phase 1 (answering)

Rewrite the container so the answering phase works end to end against the new components. Phase 2 arrives in Task 11; until then `showAnswer` renders nothing below the input, and `Translator.test.tsx` covers only phase-1 behavior.

**Files:**
- Rewrite: `fe/src/components/Translator.tsx`
- Rewrite: `fe/src/components/Translator.test.tsx`

**Interfaces:**
- Consumes: `AppHeader` (7), `SettingsSheet` (6), `QuestionCard` (8), `useSettings` / `levelsForRequest` / `levelSummary` / `loadStoredLevels` (3), `speakJapanese` (2).
- Produces: `Translator({ user }: { user: User })` — the default export, unchanged signature.

- [ ] **Step 1: Write the failing test**

Replace `fe/src/components/Translator.test.tsx` entirely:

```tsx
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { User } from 'firebase/auth'

vi.mock('@/lib/firebase', () => ({ auth: {} }))
vi.mock('firebase/auth', () => ({ signOut: vi.fn() }))
vi.mock('@/lib/speech', () => ({ speakJapanese: vi.fn() }))

vi.mock('@/lib/api', () => ({
  api: {
    getRandomSentence: vi.fn(),
    checkAnswer: vi.fn(),
    explainAnswer: vi.fn(),
    reportSentence: vi.fn(),
  },
}))

import { api } from '@/lib/api'
import Translator from './Translator'

const mockApi = api as unknown as {
  getRandomSentence: ReturnType<typeof vi.fn>
  checkAnswer: ReturnType<typeof vi.fn>
  explainAnswer: ReturnType<typeof vi.fn>
  reportSentence: ReturnType<typeof vi.fn>
}

const fakeUser = { uid: 'u1', displayName: 'Jane' } as User

const fakeSentence = {
  id: 1,
  japanese: '時間がありません。',
  english: "I don't have time.",
  page: '12',
  level: 2,
  correct_count: 0,
  incorrect_count: 0,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  mockApi.getRandomSentence.mockResolvedValue(fakeSentence)
})

async function renderAndLoad() {
  render(<Translator user={fakeUser} />)
  await screen.findByText(fakeSentence.japanese)
}

describe('answering phase', () => {
  it('fetches a sentence with no level filter on mount', async () => {
    await renderAndLoad()
    expect(mockApi.getRandomSentence).toHaveBeenCalledWith(undefined)
  })

  it('shows the input and no review affordances', async () => {
    await renderAndLoad()

    expect(screen.getByLabelText('Your English translation')).toBeInTheDocument()
    expect(screen.queryByRole('tab')).not.toBeInTheDocument()
    expect(screen.queryByText('Not quite right. Try again!')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Next Sentence' })).not.toBeInTheDocument()
  })

  it('disables Check Translation until something is typed', async () => {
    await renderAndLoad()

    expect(screen.getByRole('button', { name: 'Check Translation' })).toBeDisabled()

    fireEvent.change(screen.getByLabelText('Your English translation'), {
      target: { value: 'I have no time.' },
    })

    expect(screen.getByRole('button', { name: 'Check Translation' })).toBeEnabled()
  })

  it('capitalizes the first letter on blur', async () => {
    await renderAndLoad()
    const input = screen.getByLabelText('Your English translation')

    fireEvent.change(input, { target: { value: 'i have no time.' } })
    fireEvent.blur(input)

    expect(input).toHaveValue('I have no time.')
  })

  it('submits on Ctrl+Enter', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    await renderAndLoad()
    const input = screen.getByLabelText('Your English translation')

    fireEvent.change(input, { target: { value: 'I have no time.' } })
    fireEvent.keyDown(input, { key: 'Enter', ctrlKey: true })

    await waitFor(() => expect(mockApi.checkAnswer).toHaveBeenCalledWith(1, 'I have no time.'))
  })

  it('shows an error with a retry when the sentence fails to load', async () => {
    mockApi.getRandomSentence.mockRejectedValue(new Error('boom'))
    render(<Translator user={fakeUser} />)

    expect(await screen.findByText('boom')).toBeInTheDocument()

    mockApi.getRandomSentence.mockResolvedValue(fakeSentence)
    fireEvent.click(screen.getByRole('button', { name: 'Try Again' }))

    expect(await screen.findByText(fakeSentence.japanese)).toBeInTheDocument()
  })

  it('ignores a stale sentence response that resolves after a newer request', async () => {
    await renderAndLoad()

    let resolveStale: (value: unknown) => void = () => {}
    mockApi.getRandomSentence.mockImplementationOnce(
      () => new Promise(resolve => { resolveStale = resolve })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Level 1' }))

    mockApi.getRandomSentence.mockResolvedValueOnce({ ...fakeSentence, id: 9, japanese: '新しい文' })
    fireEvent.click(screen.getByRole('checkbox', { name: 'Level 2' }))
    await screen.findByText('新しい文')

    resolveStale({ ...fakeSentence, id: 5, japanese: '古い文' })

    await waitFor(() => expect(screen.queryByText('古い文')).not.toBeInTheDocument())
    expect(screen.getByText('新しい文')).toBeInTheDocument()
  })
})

describe('settings', () => {
  it('opens the settings sheet from the header gear', async () => {
    await renderAndLoad()

    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))

    expect(screen.getByRole('dialog', { name: 'Settings' })).toBeInTheDocument()
  })

  it('shows every level checked and the summary as All by default', async () => {
    await renderAndLoad()
    expect(screen.getByText('Level: All')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))
    for (const n of [1, 2, 3, 4, 5]) {
      expect(screen.getByRole('checkbox', { name: `Level ${n}` })).toBeChecked()
    }
  })

  it('narrows the filter, refetches, persists and updates the summary', async () => {
    await renderAndLoad()

    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Level 5' }))

    await waitFor(() =>
      expect(mockApi.getRandomSentence).toHaveBeenLastCalledWith([1, 2, 3, 4])
    )
    expect(localStorage.getItem('eagle:selectedLevels')).toBe('[1,2,3,4]')
    expect(screen.getByText('Level: 1, 2, 3, 4')).toBeInTheDocument()
  })

  it('restores a persisted level selection on mount', async () => {
    localStorage.setItem('eagle:selectedLevels', JSON.stringify([3]))
    await renderAndLoad()

    expect(mockApi.getRandomSentence).toHaveBeenCalledWith([3])
  })

  it('persists the AI language', async () => {
    await renderAndLoad()

    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))
    fireEvent.click(screen.getByRole('tab', { name: '日本語' }))

    expect(localStorage.getItem('eagle:explainLanguage')).toBe('ja')
  })

  it('resets an in-progress answer when a level is toggled', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    await renderAndLoad()
    fireEvent.change(screen.getByLabelText('Your English translation'), {
      target: { value: 'I have no time.' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Check Translation' }))
    await screen.findByText('Not quite right. Try again!')

    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Level 5' }))

    await waitFor(() =>
      expect(screen.queryByText('Not quite right. Try again!')).not.toBeInTheDocument()
    )
    expect(screen.getByLabelText('Your English translation')).toHaveValue('')
  })
})

describe('header', () => {
  it('links to the mistakes page', async () => {
    await renderAndLoad()
    expect(screen.getByRole('link', { name: 'Mistakes' })).toHaveAttribute('href', '/mistakes')
  })
})
```

Note: the reset-on-toggle and Ctrl+Enter tests exercise the check flow, so this file already depends on Task 11's rendering of the verdict. `ReviewPanel` exists as of Task 9, so wire it in now — Task 11 covers the review-phase behavior in depth.

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd fe && npm test -- src/components/Translator.test.tsx`
Expected: FAIL — the old component still renders `Level:` trigger buttons and has no `Settings` button.

- [ ] **Step 3: Rewrite the component**

Replace `fe/src/components/Translator.tsx` entirely:

```tsx
'use client'

import { useState, useEffect, useRef } from 'react'
import type { User } from 'firebase/auth'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import AppHeader from './AppHeader'
import SettingsSheet from './SettingsSheet'
import QuestionCard from './QuestionCard'
import ReviewPanel, { type ReviewTab } from './ReviewPanel'
import { api, type Sentence, type AnswerHistory } from '@/lib/api'
import { speakJapanese } from '@/lib/speech'
import {
  useSettings,
  loadStoredLevels,
  levelsForRequest,
  levelSummary,
  type ExplainLanguage,
} from '@/lib/useSettings'

interface Props {
  user: User
}

export default function Translator({ user }: Props) {
  const { levels, language, setLevels, setLanguage } = useSettings()

  const [currentSentence, setCurrentSentence] = useState<Sentence | null>(null)
  const [userTranslation, setUserTranslation] = useState('')
  const [feedback, setFeedback] = useState<'correct' | 'incorrect' | null>(null)
  const [showAnswer, setShowAnswer] = useState(false)
  const [loading, setLoading] = useState(true)
  const [histories, setHistories] = useState<AnswerHistory[]>([])
  const [error, setError] = useState<string | null>(null)
  const [correctCount, setCorrectCount] = useState(0)
  const [incorrectCount, setIncorrectCount] = useState(0)
  const [isReported, setIsReported] = useState(false)
  const [isSpeaking, setIsSpeaking] = useState(false)
  const [explanation, setExplanation] = useState<string | null>(null)
  const [explaining, setExplaining] = useState(false)
  const [explainError, setExplainError] = useState<string | null>(null)
  const [tab, setTab] = useState<ReviewTab>('answer')
  const [settingsOpen, setSettingsOpen] = useState(false)

  const latestRequestId = useRef(0)
  const explainRequestId = useRef(0)

  const getRandomSentence = async (levelsOverride?: number[]) => {
    const requestId = ++latestRequestId.current
    try {
      setLoading(true)
      setError(null)
      const sentence = await api.getRandomSentence(
        levelsForRequest(levelsOverride ?? levels)
      )
      if (requestId !== latestRequestId.current) return
      setCurrentSentence(sentence)
      setCorrectCount(sentence.correct_count)
      setIncorrectCount(sentence.incorrect_count)
    } catch (err) {
      if (requestId !== latestRequestId.current) return
      setError(err instanceof Error ? err.message : 'Failed to load sentence')
    } finally {
      if (requestId === latestRequestId.current) setLoading(false)
    }
  }

  useEffect(() => {
    getRandomSentence(loadStoredLevels())
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const resetQuestionState = () => {
    explainRequestId.current++
    setUserTranslation('')
    setFeedback(null)
    setShowAnswer(false)
    setHistories([])
    setError(null)
    setCorrectCount(0)
    setIncorrectCount(0)
    setIsReported(false)
    setIsSpeaking(false)
    setExplanation(null)
    setExplaining(false)
    setExplainError(null)
    setTab('answer')
    if ('speechSynthesis' in window) speechSynthesis.cancel()
  }

  const handleLevelsChange = (next: number[]) => {
    setLevels(next)
    resetQuestionState()
    getRandomSentence(next)
  }

  const handleLanguageChange = (next: ExplainLanguage) => {
    setLanguage(next)
    if (explanation || explainError) explainAnswer(next)
  }

  const explainAnswer = async (lang: ExplainLanguage) => {
    if (!currentSentence) return
    const requestId = ++explainRequestId.current
    setExplaining(true)
    setExplainError(null)
    setExplanation(null)
    try {
      const result = await api.explainAnswer(currentSentence.id, userTranslation, lang)
      if (requestId !== explainRequestId.current) return
      setExplanation(result.explanation)
    } catch (err) {
      if (requestId !== explainRequestId.current) return
      setExplainError(err instanceof Error ? err.message : 'Failed to load explanation')
    } finally {
      if (requestId === explainRequestId.current) setExplaining(false)
    }
  }

  const handleTabChange = (next: ReviewTab) => {
    setTab(next)
    if (next === 'explain' && !explanation && !explaining && !explainError) {
      explainAnswer(language)
    }
  }

  const checkTranslation = async () => {
    if (!currentSentence) return
    try {
      const result = await api.checkAnswer(currentSentence.id, userTranslation.trim())
      setFeedback(result.is_correct ? 'correct' : 'incorrect')
      setHistories(result.histories)
      setShowAnswer(true)
      setTab('answer')
      if (result.is_correct) setCorrectCount(prev => prev + 1)
      else setIncorrectCount(prev => prev + 1)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to check answer')
    }
  }

  const nextSentence = () => {
    resetQuestionState()
    getRandomSentence()
  }

  const reportSentence = async (sentenceId: number) => {
    try {
      await api.reportSentence(sentenceId)
      setIsReported(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to report sentence')
    }
  }

  const capitalizeFirstLetter = (text: string) => text.charAt(0).toUpperCase() + text.slice(1)

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 p-4">
      <div className="mx-auto flex min-h-[calc(100vh-2rem)] max-w-2xl flex-col">
        <AppHeader user={user} onOpenSettings={() => setSettingsOpen(true)} />

        {loading ? (
          <div className="text-center">
            <div className="mx-auto mb-4 h-12 w-12 animate-spin rounded-full border-b-2 border-indigo-600" />
            <p className="text-muted-foreground">Loading...</p>
          </div>
        ) : error || !currentSentence ? (
          <Card>
            <CardContent className="p-5">
              <p className="mb-4 text-foreground">{error || 'Failed to load content'}</p>
              <Button onClick={() => getRandomSentence()} className="w-full">
                Try Again
              </Button>
            </CardContent>
          </Card>
        ) : (
          <>
            <div className="space-y-3">
              <QuestionCard
                sentence={currentSentence}
                correctCount={correctCount}
                incorrectCount={incorrectCount}
                levelSummary={levelSummary(levels)}
                isSpeaking={isSpeaking}
                onSpeak={() => speakJapanese(currentSentence.japanese, setIsSpeaking)}
              />

              {!showAnswer ? (
                <Card>
                  <CardContent className="p-5">
                    <form
                      onSubmit={e => {
                        e.preventDefault()
                        if (userTranslation.trim()) checkTranslation()
                      }}
                      className="space-y-2"
                    >
                      <Label htmlFor="translation">Your translation</Label>
                      <Textarea
                        id="translation"
                        value={userTranslation}
                        onChange={e => setUserTranslation(e.target.value)}
                        placeholder="Enter your translation here..."
                        onBlur={e => {
                          if (e.target.value.trim()) {
                            setUserTranslation(capitalizeFirstLetter(e.target.value.trim()))
                          }
                        }}
                        onKeyDown={e => {
                          if (e.key === 'Enter' && e.ctrlKey && userTranslation.trim()) {
                            checkTranslation()
                          }
                        }}
                        aria-label="Your English translation"
                        aria-required="true"
                      />
                    </form>
                  </CardContent>
                </Card>
              ) : (
                feedback && (
                  <ReviewPanel
                    feedback={feedback}
                    userAnswer={userTranslation}
                    correctAnswer={currentSentence.english}
                    histories={histories}
                    tab={tab}
                    onTabChange={handleTabChange}
                    explanation={explanation}
                    explaining={explaining}
                    explainError={explainError}
                    onRetryExplain={() => explainAnswer(language)}
                  />
                )
              )}
            </div>

            <div className="mt-auto pt-4">
              {!showAnswer ? (
                <Button
                  onClick={checkTranslation}
                  disabled={!userTranslation.trim()}
                  className="w-full"
                >
                  Check Translation
                </Button>
              ) : (
                <div className="flex gap-2">
                  <Button onClick={nextSentence} className="flex-1">
                    Next Sentence
                  </Button>
                  <Button
                    variant="ghost"
                    onClick={() => reportSentence(currentSentence.id)}
                    disabled={isReported}
                  >
                    {isReported ? 'Reported' : 'Report'}
                  </Button>
                </div>
              )}
            </div>
          </>
        )}

        <SettingsSheet
          open={settingsOpen}
          onClose={() => setSettingsOpen(false)}
          levels={levels}
          onLevelsChange={handleLevelsChange}
          language={language}
          onLanguageChange={handleLanguageChange}
        />
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd fe && npm test -- src/components/Translator.test.tsx`
Expected: PASS, 13 tests.

- [ ] **Step 5: Run the whole unit suite**

Run: `cd fe && npm test`
Expected: PASS. `Mistakes.test.tsx` still passes — Task 12 has not touched it yet.

- [ ] **Step 6: Commit**

```bash
git add fe/src/components/Translator.tsx fe/src/components/Translator.test.tsx
git commit -m "refactor(fe): rebuild Translator as a two-phase container

Answering shows only the sentence and the input; the primary action sits
at the bottom of the viewport in both phases. Navigation and settings move
to AppHeader and SettingsSheet, and 582 lines drop to a container that
delegates rendering."
```

---

### Task 11: Cover the review phase

The container already renders `ReviewPanel`; this task pins its behavior down with the tests the old `Explain button` describe block used to own.

**Files:**
- Modify: `fe/src/components/Translator.test.tsx` (append)
- Modify: `fe/src/components/Translator.tsx` only if a test exposes a defect

- [ ] **Step 1: Write the failing tests**

Append to `fe/src/components/Translator.test.tsx`:

```tsx
describe('review phase', () => {
  async function answerIncorrectly(histories: AnswerHistory[] = []) {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories,
    })
    await renderAndLoad()
    fireEvent.change(screen.getByLabelText('Your English translation'), {
      target: { value: 'I have no time.' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Check Translation' }))
    await screen.findByText('Not quite right. Try again!')
  }

  it('swaps the input out for the review panel', async () => {
    await answerIncorrectly()

    expect(screen.queryByLabelText('Your English translation')).not.toBeInTheDocument()
    expect(screen.getByText(fakeSentence.english)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Next Sentence' })).toBeInTheDocument()
  })

  it('increments the incorrect counter', async () => {
    await answerIncorrectly()
    expect(screen.getByText(/^Incorrect: 1$/)).toBeInTheDocument()
  })

  it('shows the correct verdict and no Explain tab when right', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: true,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    await renderAndLoad()
    fireEvent.change(screen.getByLabelText('Your English translation'), {
      target: { value: fakeSentence.english },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Check Translation' }))

    expect(await screen.findByText('Correct! Well done!')).toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: 'Explain' })).not.toBeInTheDocument()
    expect(screen.getByText(/^Correct: 1$/)).toBeInTheDocument()
  })

  it('lists previous attempts behind the Attempts tab', async () => {
    await answerIncorrectly([
      { id: 1, incorrect_answer: 'There is no time.', created_at: '2026-01-01T00:00:00Z' },
    ])

    fireEvent.click(screen.getByRole('tab', { name: 'Attempts 1' }))

    expect(screen.getByText('There is no time.')).toBeInTheDocument()
  })

  it('fetches the explanation when the Explain tab is selected', async () => {
    mockApi.explainAnswer.mockResolvedValue({ explanation: 'Prefer **do-support**.' })
    await answerIncorrectly()

    fireEvent.click(screen.getByRole('tab', { name: 'Explain' }))

    await waitFor(() =>
      expect(mockApi.explainAnswer).toHaveBeenCalledWith(1, 'I have no time.', 'en')
    )
    expect(await screen.findByText('do-support')).toBeInTheDocument()
  })

  it('fetches in the stored language', async () => {
    localStorage.setItem('eagle:explainLanguage', 'ja')
    mockApi.explainAnswer.mockResolvedValue({ explanation: '説明' })
    await answerIncorrectly()

    fireEvent.click(screen.getByRole('tab', { name: 'Explain' }))

    await waitFor(() =>
      expect(mockApi.explainAnswer).toHaveBeenCalledWith(1, 'I have no time.', 'ja')
    )
  })

  it('does not refetch when the Explain tab is revisited', async () => {
    mockApi.explainAnswer.mockResolvedValue({ explanation: 'Once.' })
    await answerIncorrectly()

    fireEvent.click(screen.getByRole('tab', { name: 'Explain' }))
    await screen.findByText('Once.')
    fireEvent.click(screen.getByRole('tab', { name: 'Answer' }))
    fireEvent.click(screen.getByRole('tab', { name: 'Explain' }))

    expect(mockApi.explainAnswer).toHaveBeenCalledTimes(1)
  })

  it('re-fetches in the new language when the setting changes after explaining', async () => {
    mockApi.explainAnswer.mockResolvedValue({ explanation: 'English text' })
    await answerIncorrectly()
    fireEvent.click(screen.getByRole('tab', { name: 'Explain' }))
    await screen.findByText('English text')

    mockApi.explainAnswer.mockResolvedValue({ explanation: '日本語のテキスト' })
    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))
    fireEvent.click(screen.getByRole('tab', { name: '日本語' }))

    expect(await screen.findByText('日本語のテキスト')).toBeInTheDocument()
  })

  it('does not call explainAnswer when the language changes before explaining', async () => {
    await answerIncorrectly()

    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))
    fireEvent.click(screen.getByRole('tab', { name: '日本語' }))

    expect(mockApi.explainAnswer).not.toHaveBeenCalled()
  })

  it('shows an explain error with a working retry', async () => {
    mockApi.explainAnswer.mockRejectedValue(new Error('Explain failed'))
    await answerIncorrectly()

    fireEvent.click(screen.getByRole('tab', { name: 'Explain' }))
    expect(await screen.findByText('Explain failed')).toBeInTheDocument()

    mockApi.explainAnswer.mockResolvedValue({ explanation: 'Recovered.' })
    fireEvent.click(screen.getByRole('button', { name: 'Try Again' }))

    expect(await screen.findByText('Recovered.')).toBeInTheDocument()
  })

  it('discards an explanation superseded by moving to the next sentence', async () => {
    let resolveStale: (value: unknown) => void = () => {}
    mockApi.explainAnswer.mockImplementationOnce(
      () => new Promise(resolve => { resolveStale = resolve })
    )
    await answerIncorrectly()
    fireEvent.click(screen.getByRole('tab', { name: 'Explain' }))

    fireEvent.click(screen.getByRole('button', { name: 'Next Sentence' }))
    await screen.findByLabelText('Your English translation')

    resolveStale({ explanation: 'Stale explanation' })

    await waitFor(() =>
      expect(screen.queryByText('Stale explanation')).not.toBeInTheDocument()
    )
  })

  it('reports the sentence and acknowledges it', async () => {
    mockApi.reportSentence.mockResolvedValue(undefined)
    await answerIncorrectly()

    fireEvent.click(screen.getByRole('button', { name: 'Report' }))

    expect(await screen.findByRole('button', { name: 'Reported' })).toBeInTheDocument()
    expect(mockApi.reportSentence).toHaveBeenCalledWith(1)
  })

  it('resets to the answering phase on Next Sentence', async () => {
    await answerIncorrectly()

    fireEvent.click(screen.getByRole('button', { name: 'Next Sentence' }))

    expect(await screen.findByLabelText('Your English translation')).toHaveValue('')
    expect(screen.queryByText('Not quite right. Try again!')).not.toBeInTheDocument()
    expect(screen.queryByRole('tab')).not.toBeInTheDocument()
  })
})
```

Add `AnswerHistory` to the api type import at the top of the file:

```tsx
import type { AnswerHistory } from '@/lib/api'
```

- [ ] **Step 2: Run the tests and record which fail**

Run: `cd fe && npm test -- src/components/Translator.test.tsx`
Expected: most pass on Task 10's implementation. Any failure is a genuine defect in the container — fix `Translator.tsx`, not the test.

- [ ] **Step 3: Fix any defects the tests expose**

Do not weaken an assertion to make it pass. If `resetQuestionState` fails to clear the tab, fix `resetQuestionState`.

- [ ] **Step 4: Run the whole unit suite**

Run: `cd fe && npm test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add fe/src/components/Translator.test.tsx fe/src/components/Translator.tsx
git commit -m "test(fe): cover the review phase end to end

Explain now fetches on tab selection rather than a dedicated button, and
the language comes from settings, so the old Explain-toggle cases are
replaced by tab-driven equivalents."
```

---

### Task 12: Rebuild the Mistakes page

**Files:**
- Modify: `fe/src/components/Mistakes.tsx`
- Modify: `fe/src/components/Mistakes.test.tsx`
- Modify: `fe/src/app/mistakes/page.tsx` (only if it must pass `user` down — check first)

**Interfaces:**
- Consumes: `AppHeader` (7), `SettingsSheet` (6), `useSettings` (3).
- Produces: no new exports.

`insightCacheKey` and its explanatory comment (`Mistakes.tsx:13-27`) are carried over **unchanged** — the uid+language+fingerprint scoping is still correct.

- [ ] **Step 1: Check how the page supplies the user**

Run: `cat fe/src/app/mistakes/page.tsx fe/src/components/AuthGate.tsx`

`AppHeader` needs a `User`. If `AuthGate` already provides one to its children, thread it through; if `Mistakes` currently takes no props, add `user: User` to its props and pass it from the page exactly the way `fe/src/app/page.tsx` does for `Translator`.

- [ ] **Step 2: Write the failing tests**

In `fe/src/components/Mistakes.test.tsx`, delete the five insight-language-toggle cases added by PR #17 (the ones asserting on EN/JA buttons inside the insight card), and add:

```tsx
describe('header and settings', () => {
  it('renders the shared header without a link back to itself', async () => {
    await renderWithMistakes()

    expect(screen.getByRole('link', { name: 'Eagle' })).toHaveAttribute('href', '/')
    expect(screen.queryByRole('link', { name: 'Mistakes' })).not.toBeInTheDocument()
  })

  it('has no inline language toggle inside the insight card', async () => {
    await renderWithMistakes()

    expect(screen.queryByRole('tab', { name: 'EN' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'EN' })).not.toBeInTheDocument()
  })

  it('reloads the insight in the new language when settings change it', async () => {
    await renderWithMistakes()
    await screen.findByText('You drop do-support.')

    mockApi.getMistakesInsight.mockResolvedValue({ insight: '弱点の説明' })
    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))
    fireEvent.click(screen.getByRole('tab', { name: '日本語' }))

    expect(await screen.findByText('弱点の説明')).toBeInTheDocument()
    expect(localStorage.getItem('eagle:explainLanguage')).toBe('ja')
  })

  it('uses the stored language for the first insight request', async () => {
    localStorage.setItem('eagle:explainLanguage', 'ja')
    await renderWithMistakes()

    await waitFor(() => expect(mockApi.getMistakesInsight).toHaveBeenCalledWith('ja'))
  })
})
```

Write a `renderWithMistakes()` helper matching the mocking already at the top of the existing file, resolving `listMistakes` with one mistake and `getMistakesInsight` with `'You drop do-support.'`. Keep every existing cache-reuse, error, empty-state and list-rendering case.

- [ ] **Step 3: Run the tests to verify they fail**

Run: `cd fe && npm test -- src/components/Mistakes.test.tsx`
Expected: FAIL — no `Settings` button, no `Eagle` link.

- [ ] **Step 4: Update the component**

In `fe/src/components/Mistakes.tsx`:

1. Delete the local `EXPLAIN_LANGUAGE_STORAGE_KEY` (line 11) and the `insightLanguage` `useState` initializer (lines 50-53). Replace with `const { levels, language, setLevels, setLanguage } = useSettings()`.
2. Delete `selectInsightLanguage` (lines 81-89) and the EN/JA button pair in the insight `CardHeader` (lines 147-168). The header collapses to just the `Weakness Insight` title.
3. Replace the `← Back` link and `<h1>Mistakes</h1>` block (lines 114-119) with `<AppHeader user={user} onOpenSettings={() => setSettingsOpen(true)} showMistakesLink={false} />` followed by `<h2 className="mb-4 text-lg font-bold text-foreground">Mistakes</h2>`.
4. Add `settingsOpen` state and render `SettingsSheet` at the end, with `onLanguageChange={next => { setLanguage(next); if (mistakes?.length) loadInsight(mistakes, next) }}` and `onLevelsChange={setLevels}`.
5. Change `loadMistakes` to call `loadInsight(result.mistakes, language)`.
6. Recolor: `border-indigo-300` on the insight card → default border. The blue correct-answer block (line 196) → `rounded-md border border-border bg-muted px-2 py-1 text-sm text-foreground`. The yellow wrong-answer chips (lines 201-205) → `rounded-md border border-border bg-muted px-2 py-1 text-xs text-muted-foreground line-through`. `text-indigo-900` in `insightMarkdownComponents` → `text-foreground`.
7. Keep `insightCacheKey`, its comment, the `sessionStorage` cache logic, `disallowedElements={['a','img']}`, and the loading/error/empty branches exactly as they are.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd fe && npm test -- src/components/Mistakes.test.tsx`
Expected: PASS.

- [ ] **Step 6: Confirm no palette classes survive**

Run: `cd fe && grep -rn "bg-blue-\|bg-yellow-\|bg-purple-\|text-indigo-\|border-indigo-" src/components/Translator.tsx src/components/Mistakes.tsx`
Expected: no output.

- [ ] **Step 7: Run the whole unit suite and the linter**

Run: `cd fe && npm test && npm run lint && npm run build`
Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add fe/src/components/Mistakes.tsx fe/src/components/Mistakes.test.tsx fe/src/app/mistakes/page.tsx
git commit -m "refactor(fe): put Mistakes on the shared header and settings

Removes the duplicate EN/JA toggle — the language now has one control in
the settings sheet — and drops the indigo, blue and yellow blocks for the
neutral surface used across the app."
```

---

### Task 13: Update the e2e suite

Three specs need one line each; three need nothing. Verify all six.

**Files:**
- Modify: `e2e/tests/incorrect-explain.spec.ts:17`
- Modify: `e2e/tests/level-filter.spec.ts:7`
- Modify: `e2e/tests/mistakes.spec.ts:17`

- [ ] **Step 1: Update `incorrect-explain.spec.ts`**

Explain is a tab now, not a button. Replace line 17:

```ts
  await page.getByRole('tab', { name: 'Explain' }).click()
```

- [ ] **Step 2: Update `level-filter.spec.ts`**

The opener is the settings gear. Replace line 7:

```ts
  await page.getByRole('button', { name: 'Settings' }).click()
```

The five `getByRole('checkbox', { name: 'Level N' })` selectors stay exactly as they are.

- [ ] **Step 3: Update `mistakes.spec.ts`**

The `← Back` link is gone; the brand links home. Replace line 17:

```ts
  await page.getByRole('link', { name: 'Eagle' }).click()
```

- [ ] **Step 4: Run the full e2e suite**

Run: `cd e2e && ./scripts/run.sh`
Expected: all six specs PASS. `auth.spec.ts`, `correct-answer.spec.ts` and `report-next.spec.ts` must pass with **no** edits — if either of the latter two fails, frozen copy was changed somewhere and the component is wrong, not the test.

- [ ] **Step 5: Commit**

```bash
git add e2e/tests/
git commit -m "test(e2e): follow the reorganized practice screen

Explain became a tab, the level filter moved behind the settings gear,
and the mistakes back link became the brand home link. The other three
specs are unchanged, which is the point."
```

---

### Task 14: Verify on a phone viewport

The whole design is justified by phone behavior; unit and e2e tests cannot confirm it.

**Files:** none.

- [ ] **Step 1: Start the local stack**

Follow the local dev stack notes (Firebase emulators + `api/cmd/e2eserver` + `cd fe && npm run dev`).

- [ ] **Step 2: Check the primary action never moves**

At a 390×844 viewport, confirm that `Check Translation` in the answering phase and `Next Sentence` in the review phase occupy the same vertical position, and that the position does not shift for:
- a correct answer (single tab, no Explain),
- a wrong answer with no history,
- a wrong answer with several previous attempts,
- a long AI explanation (the case that used to push the button off-screen).

- [ ] **Step 3: Check the settings sheet**

Open it from both `/` and `/mistakes`. Confirm level chips toggle and refetch, the language choice persists across a reload, and changing the language on `/mistakes` re-runs the insight.

- [ ] **Step 4: Check hover and focus states**

Task 1 gave `outline` and `ghost` buttons a real hover for the first time. Confirm the header gear, the Report button and the Try Again buttons all show one, and that keyboard focus rings are visible on the segmented tabs.

- [ ] **Step 5: Commit any fixes**

If a fix is needed, write the failing test first where one is possible, then fix, then commit.

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
| --- | --- |
| Shared header, captions deleted | 7, 10 |
| Phase 1 — answering | 10 |
| Phase 2 — verdict, conditional tabs, footer | 9, 11 |
| Explain on tab select, race guard preserved | 10, 11 |
| Settings sheet, levels + language | 4, 5, 6 |
| `useSettings` single source of truth | 3 |
| Mistakes page, duplicate toggle removed | 12 |
| Component split table | 2, 4-9 |
| Color budget / neutral surfaces | 1, 9, 12 |
| Frozen copy and accessible names | Global constraints; enforced in 8, 9, 13 |
| Test plan | 3-13 |
| Manual phone verification | 14 |

**Gap found and closed:** the spec's component table lists `lib/speech.ts`, which had no task. Task 2 was added.

**Type consistency:** `ExplainLanguage` (Task 3) is the parameter type in Tasks 6, 10, 12. `ReviewTab` (Task 9) is the state type in Task 10. `SegmentedOption` (Task 5) is consumed in Tasks 6 and 9. `speakJapanese(text, onSpeakingChange)` (Task 2) matches its call in Task 10. `toggleLevel(levels, n)` (Task 3) matches its call in Task 6.

**One known coupling:** Task 10 wires in `ReviewPanel` and two of its tests touch the review phase, so Tasks 10 and 11 must land in order. This is called out in Task 10, Step 1.
