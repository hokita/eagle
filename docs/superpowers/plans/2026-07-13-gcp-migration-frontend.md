# GCP Migration — Frontend (Firebase Auth + Static Export) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Gate the Eagle frontend behind Google sign-in (Firebase Auth), attach the ID token to every API call, and switch the Next.js app to a static export so it can deploy to Firebase Hosting alongside the already-migrated Go/Firestore backend.

**Architecture:** A new `src/lib/firebase.ts` initializes the Firebase web SDK. A new `src/lib/api.ts` centralizes all backend calls behind a `request()` helper that attaches `Authorization: Bearer <ID token>`, replacing the three raw `fetch` calls currently inline in `page.tsx`. A new `AuthGate` component renders a `LoginScreen` when signed out and the existing translator UI (relocated to `Translator.tsx`) when signed in, with a `UserMenu` for sign-out. `next.config.ts` switches to `output: "export"`. This mirrors the `corgi` reference project's frontend almost file-for-file (`firebase.ts`, `api.ts`, `UserMenu.tsx`, its `App.tsx`/`LoginPage.tsx` pattern), including its Vitest + React Testing Library test setup, which Eagle's frontend does not have yet.

**Tech Stack:** Next.js 15 (App Router, client-only), React 19, `firebase` (web SDK), Vitest, `@testing-library/react`, `@testing-library/jest-dom`, `@vitejs/plugin-react`, TypeScript.

## Global Constraints

- All paths below are relative to `fe/`. Path alias `@/*` → `./src/*` (existing `tsconfig.json`) must also be configured in `vitest.config.ts`.
- **No dev proxy.** Next.js `rewrites()`/`redirects()`/`headers()` error out in `next dev` too (not just production builds) once `output: 'export'` is set — confirmed against Next.js's static-export docs. Local dev talks directly to the Go API cross-origin, same as before migration: `NEXT_PUBLIC_API_URL=http://localhost:8080` in `.env.local`, unchanged. CORS on the Go API (`withCORS`, already implemented in the backend plan's Task 7) is what makes this work, not a proxy. Production uses `NEXT_PUBLIC_API_URL=""` (relative `/api/**`, same-origin via Firebase Hosting rewrites).
- `output: 'export'` requires `images: { unoptimized: true }` — both `page.tsx`/`Translator.tsx` and the new `LoginScreen.tsx` use `next/image`, and static export has no image-optimization server.
- Firebase web config is exposed via `NEXT_PUBLIC_FIREBASE_API_KEY`, `NEXT_PUBLIC_FIREBASE_AUTH_DOMAIN`, `NEXT_PUBLIC_FIREBASE_PROJECT_ID` — the same 3-field minimal set corgi's `frontend/src/firebase.ts` uses. This is not secret (Firebase's security model doesn't depend on hiding it), but real values don't exist until the infra plan provisions a Firebase project — this plan uses empty placeholders in `.env.local`/`.env.production` (both already gitignored via `fe/.gitignore`'s `.env*` pattern).
- Test stack mirrors corgi's frontend exactly: Vitest + `jsdom` + `@testing-library/react` + `@testing-library/jest-dom`. Every test file explicitly imports from `'vitest'` (no reliance on the `globals: true` runtime convenience) so ESLint sees no undefined globals.
- TDD applies to every new file with real logic: `api.ts`, `UserMenu.tsx`, `LoginScreen.tsx`, `AuthGate.tsx`. `firebase.ts`, `next.config.ts`, and the `Translator.tsx`/`page.tsx` refactor are thin wiring or relocated *existing, already-working* logic — not newly unit-tested, matching corgi's own selective coverage (it doesn't test `App.tsx`, `LoginPage.tsx`, or `firebase.ts` either). These are verified via `npm run build` and a manual dev-server check instead, per this project's convention of testing UI changes in a browser.
- The public JSON contract with the backend is unchanged (see the backend plan's Global Constraints) — `api.ts`'s types must match it exactly.
- Out of scope: GCP infra (Firebase project creation, real Firebase config values, Hosting deploy workflow, retiring `.github/workflows/build_fe_docker_image.yaml`) — covered by the infra/deploy plan. Full interactive sign-in verification also waits on that plan (no real Firebase project exists yet).

## File structure (after this plan)

```
fe/
  next.config.ts              MODIFY: output: "export", images.unoptimized
  package.json                 MODIFY: +firebase dep, +vitest/RTL devDeps, +test scripts
  vitest.config.ts              CREATE: jsdom env, @ alias, react plugin
  .env.local                     MODIFY: +NEXT_PUBLIC_FIREBASE_* (empty placeholders)
  .env.production                  MODIFY: NEXT_PUBLIC_API_URL="", +NEXT_PUBLIC_FIREBASE_* (empty)
  src/
    test-setup.ts                  CREATE: jest-dom matcher import
    lib/
      firebase.ts                   CREATE: initializeApp + getAuth
      api.ts                         CREATE: authenticated request wrapper + 3 endpoint fns + types
      api.test.ts                    CREATE
    components/
      UserMenu.tsx                    CREATE: avatar + dropdown + sign out
      UserMenu.test.tsx                CREATE
      LoginScreen.tsx                   CREATE: sign-in button + branding
      LoginScreen.test.tsx               CREATE
      AuthGate.tsx                        CREATE: onAuthStateChanged gate
      AuthGate.test.tsx                    CREATE
      Translator.tsx                        CREATE: relocated JapaneseTranslator body,
                                             now uses api.ts + accepts a user prop +
                                             renders UserMenu
    app/
      page.tsx                              MODIFY: thin AuthGate + Translator wrapper
```

Deleted: `fe/Dockerfile` (its multi-stage build copies `.next/standalone`, which
`output: 'export'` never produces — it becomes non-functional the moment
`next.config.ts` changes, so it's removed as a direct consequence of that
change, not unrelated cleanup).

---

### Task 1: Vitest + React Testing Library tooling

**Files:**
- Create: `vitest.config.ts`, `src/test-setup.ts`
- Modify: `package.json`

**Interfaces:**
- Produces: `npm test` (runs Vitest once), `npm run test:watch`.
- Consumes: nothing.

- [ ] **Step 1: Install test dependencies**

Run:
```bash
cd fe
npm install --save-dev vitest @vitejs/plugin-react jsdom @testing-library/react @testing-library/jest-dom
```
Expected: `package.json` gains these 5 packages under `devDependencies`.

- [ ] **Step 2: Add test scripts to `package.json`**

In `fe/package.json`, add to `"scripts"` (alongside the existing `dev`/`build`/`start`/`lint`):

```json
    "test": "vitest run",
    "test:watch": "vitest",
```

- [ ] **Step 3: Create the Vitest config**

Create `fe/vitest.config.ts`:

```ts
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: './src/test-setup.ts',
  },
})
```

- [ ] **Step 4: Create the test setup file**

Create `fe/src/test-setup.ts`:

```ts
import '@testing-library/jest-dom'
```

- [ ] **Step 5: Verify the runner loads cleanly**

Run: `cd fe && npm test`
Expected: Vitest starts and reports no config or module-resolution errors
(no test files exist yet, so it may report a "no test files found" failure —
that specific outcome is fine; a config/import/resolution error is not).
This confirms the runner and `@` alias are wired correctly before Task 4
writes the first real test.

- [ ] **Step 6: Commit**

```bash
git add fe/package.json fe/package-lock.json fe/vitest.config.ts fe/src/test-setup.ts
git commit -m "test(fe): add Vitest + React Testing Library (matches corgi's frontend stack)"
```

---

### Task 2: Static export config

**Files:**
- Modify: `next.config.ts`
- Delete: `Dockerfile`

**Interfaces:**
- Consumes: nothing.
- Produces: `next build` emits a static `out/` directory instead of a standalone server.

- [ ] **Step 1: Switch to static export**

Replace the entire contents of `fe/next.config.ts` with:

```ts
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "export",
  images: { unoptimized: true },
};

export default nextConfig;
```

- [ ] **Step 2: Remove the now-broken Dockerfile**

```bash
cd fe
git rm Dockerfile
```

`fe/Dockerfile`'s final stage does `COPY --from=builder /app/.next/standalone
./`, which only exists when `output: "standalone"`. With `output: "export"`
there is no standalone server to copy, so the image would fail to build.
Deployment now goes through Firebase Hosting (a later plan), not a container.

- [ ] **Step 3: Verify the build still succeeds**

Run: `cd fe && npm run build`
Expected: build succeeds and creates `fe/out/` with static HTML/CSS/JS
(the existing `fe/.gitignore` already ignores `/out/`). Since `page.tsx` still
points its fetches at `NEXT_PUBLIC_API_URL` from `.env.production`
(`http://192.168.1.101:30005` at this point in the plan — Task 8 corrects
this), the build itself does not fail; it only affects runtime fetch URLs.

- [ ] **Step 4: Commit**

```bash
git add fe/next.config.ts
git commit -m "build(fe): switch to static export (output: 'export') for Firebase Hosting"
```

---

### Task 3: Firebase web SDK initialization

**Files:**
- Create: `src/lib/firebase.ts`
- Modify: `package.json`

**Interfaces:**
- Produces: `export const auth` (a `firebase/auth` `Auth` instance) from `@/lib/firebase`, consumed by Tasks 4–7.
- Consumes: `NEXT_PUBLIC_FIREBASE_API_KEY`, `NEXT_PUBLIC_FIREBASE_AUTH_DOMAIN`, `NEXT_PUBLIC_FIREBASE_PROJECT_ID`.

- [ ] **Step 1: Install the Firebase SDK**

Run: `cd fe && npm install firebase`
Expected: `package.json` gains `firebase` under `dependencies`.

- [ ] **Step 2: Create `firebase.ts`**

Create `fe/src/lib/firebase.ts`:

```ts
import { initializeApp } from 'firebase/app'
import { getAuth } from 'firebase/auth'

const firebaseConfig = {
  apiKey: process.env.NEXT_PUBLIC_FIREBASE_API_KEY,
  authDomain: process.env.NEXT_PUBLIC_FIREBASE_AUTH_DOMAIN,
  projectId: process.env.NEXT_PUBLIC_FIREBASE_PROJECT_ID,
}

export const app = initializeApp(firebaseConfig)
export const auth = getAuth(app)
```

No dedicated test — this is a thin SDK-initialization wrapper with no branching
logic, matching corgi's own untested `frontend/src/firebase.ts`. It's exercised
indirectly by every test in Tasks 4–7 (all of which `vi.mock('@/lib/firebase', ...)`).

- [ ] **Step 3: Verify it compiles**

Run: `cd fe && npx tsc --noEmit`
Expected: no new type errors.

- [ ] **Step 4: Commit**

```bash
git add fe/package.json fe/package-lock.json fe/src/lib/firebase.ts
git commit -m "feat(fe): initialize Firebase web SDK (app + auth)"
```

---

### Task 4: Authenticated API client (`src/lib/api.ts`)

This replaces the three raw `fetch` calls currently inline in `page.tsx` with
a single, testable module — mirroring corgi's `frontend/src/api.ts` pattern
(a shared `request<T>` helper attaching the bearer token, called by named
endpoint functions).

**Files:**
- Create: `src/lib/api.ts`, `src/lib/api.test.ts`

**Interfaces:**
- Consumes: `auth` from `@/lib/firebase` (Task 3).
- Produces: types `Sentence`, `AnswerHistory`, `CheckAnswerResponse`; object
  `api` with methods `getRandomSentence(): Promise<Sentence>`,
  `checkAnswer(sentenceId: number, userAnswer: string): Promise<CheckAnswerResponse>`,
  `reportSentence(sentenceId: number): Promise<void>`. Consumed by `Translator.tsx` (Task 8).

- [ ] **Step 1: Write the failing tests**

Create `fe/src/lib/api.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('./firebase', () => ({
  auth: {
    currentUser: {
      getIdToken: vi.fn().mockResolvedValue('test-token'),
    },
  },
}))

import { api } from './api'

const mockFetch = vi.fn()
global.fetch = mockFetch

beforeEach(() => {
  vi.clearAllMocks()
})

function mockResponse(body: unknown, status = 200) {
  mockFetch.mockResolvedValue({
    ok: status < 400,
    status,
    json: () => Promise.resolve(body),
  })
}

describe('api.getRandomSentence', () => {
  it('sends GET /api/sentence/random with the Authorization header', async () => {
    mockResponse({
      id: 1,
      japanese: '時間がありません。',
      english: "I don't have time.",
      page: '12',
      correct_count: 0,
      incorrect_count: 0,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    })
    const result = await api.getRandomSentence()
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/sentence/random'),
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: 'Bearer test-token' }),
      })
    )
    expect(result.id).toBe(1)
    expect(result.english).toBe("I don't have time.")
  })
})

describe('api.checkAnswer', () => {
  it('sends POST /api/answer/check with sentence_id and user_answer', async () => {
    mockResponse({ is_correct: true, correct_answer: 'Hello', histories: [] })
    const result = await api.checkAnswer(1, 'Hello')
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/answer/check'),
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ sentence_id: 1, user_answer: 'Hello' }),
        headers: expect.objectContaining({ Authorization: 'Bearer test-token' }),
      })
    )
    expect(result.is_correct).toBe(true)
  })
})

describe('api.reportSentence', () => {
  it('sends POST /api/sentence/report and resolves on a 204 with no body', async () => {
    mockFetch.mockResolvedValue({ ok: true, status: 204 })
    await expect(api.reportSentence(1)).resolves.toBeUndefined()
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/sentence/report'),
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ sentence_id: 1 }),
      })
    )
  })
})

describe('api error handling', () => {
  it('throws when the response is not ok', async () => {
    mockResponse({}, 500)
    await expect(api.getRandomSentence()).rejects.toThrow('API error: 500')
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd fe && npm test -- api.test.ts`
Expected: FAIL (module `./api` does not exist).

- [ ] **Step 3: Write the implementation**

Create `fe/src/lib/api.ts`:

```ts
import { auth } from './firebase'

export interface Sentence {
  id: number
  japanese: string
  english: string
  page: string
  correct_count: number
  incorrect_count: number
  created_at: string
  updated_at: string
}

export interface AnswerHistory {
  id: number
  incorrect_answer: string
  created_at: string
}

export interface CheckAnswerResponse {
  is_correct: boolean
  correct_answer: string
  histories: AnswerHistory[]
}

const BASE_URL = process.env.NEXT_PUBLIC_API_URL ?? ''

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const token = await auth.currentUser?.getIdToken()
  const res = await fetch(`${BASE_URL}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
      ...options?.headers,
    },
  })
  if (!res.ok) throw new Error(`API error: ${res.status}`)
  if (res.status === 204) return undefined as T
  return res.json()
}

export const api = {
  getRandomSentence: () => request<Sentence>('/api/sentence/random'),

  checkAnswer: (sentenceId: number, userAnswer: string) =>
    request<CheckAnswerResponse>('/api/answer/check', {
      method: 'POST',
      body: JSON.stringify({ sentence_id: sentenceId, user_answer: userAnswer }),
    }),

  reportSentence: (sentenceId: number) =>
    request<void>('/api/sentence/report', {
      method: 'POST',
      body: JSON.stringify({ sentence_id: sentenceId }),
    }),
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd fe && npm test -- api.test.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add fe/src/lib/api.ts fe/src/lib/api.test.ts
git commit -m "feat(fe): add authenticated API client (replaces inline fetch calls)"
```

---

### Task 5: `UserMenu` component

Ported from corgi's `frontend/src/components/UserMenu.tsx`, which is
already well-tested there — same behavior, adapted to Next.js's `@/` import
alias and Eagle's `firebase/auth` re-export path.

**Files:**
- Create: `src/components/UserMenu.tsx`, `src/components/UserMenu.test.tsx`

**Interfaces:**
- Consumes: `auth` from `@/lib/firebase` (Task 3).
- Produces: `UserMenu({ user: User }): JSX.Element`, consumed by `Translator.tsx` (Task 8).

- [ ] **Step 1: Write the failing tests**

Create `fe/src/components/UserMenu.test.tsx`:

```tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { User } from 'firebase/auth'

vi.mock('@/lib/firebase', () => ({ auth: {} }))
vi.mock('firebase/auth', () => ({ signOut: vi.fn() }))

import { signOut } from 'firebase/auth'
import UserMenu from './UserMenu'

const mockSignOut = signOut as ReturnType<typeof vi.fn>

const fakeUser = {
  displayName: 'Jane Doe',
  photoURL: 'https://example.com/avatar.jpg',
  email: 'jane@example.com',
} as unknown as User

beforeEach(() => {
  vi.clearAllMocks()
})

describe('UserMenu', () => {
  it('renders the user avatar', () => {
    render(<UserMenu user={fakeUser} />)
    expect(screen.getByRole('img', { name: /jane doe/i })).toBeInTheDocument()
  })

  it('does not show the dropdown menu initially', () => {
    render(<UserMenu user={fakeUser} />)
    expect(screen.queryByRole('menu')).not.toBeInTheDocument()
  })

  it('opens the dropdown menu when the avatar is clicked', () => {
    render(<UserMenu user={fakeUser} />)
    fireEvent.click(screen.getByRole('img', { name: /jane doe/i }))
    expect(screen.getByRole('menu')).toBeInTheDocument()
  })

  it('shows a Sign out button inside the dropdown', () => {
    render(<UserMenu user={fakeUser} />)
    fireEvent.click(screen.getByRole('img', { name: /jane doe/i }))
    expect(screen.getByRole('menuitem', { name: /sign out/i })).toBeInTheDocument()
  })

  it('calls signOut when Sign out is clicked', () => {
    render(<UserMenu user={fakeUser} />)
    fireEvent.click(screen.getByRole('img', { name: /jane doe/i }))
    fireEvent.click(screen.getByRole('menuitem', { name: /sign out/i }))
    expect(mockSignOut).toHaveBeenCalledTimes(1)
  })

  it('closes the dropdown when clicking outside', () => {
    render(<UserMenu user={fakeUser} />)
    fireEvent.click(screen.getByRole('img', { name: /jane doe/i }))
    expect(screen.getByRole('menu')).toBeInTheDocument()
    fireEvent.mouseDown(document.body)
    expect(screen.queryByRole('menu')).not.toBeInTheDocument()
  })

  it('closes the dropdown when the avatar is clicked again', () => {
    render(<UserMenu user={fakeUser} />)
    const avatar = screen.getByRole('img', { name: /jane doe/i })
    fireEvent.click(avatar)
    expect(screen.getByRole('menu')).toBeInTheDocument()
    fireEvent.click(avatar)
    expect(screen.queryByRole('menu')).not.toBeInTheDocument()
  })

  it('displays the user name inside the dropdown', () => {
    render(<UserMenu user={fakeUser} />)
    fireEvent.click(screen.getByRole('img', { name: /jane doe/i }))
    expect(screen.getByText('Jane Doe')).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd fe && npm test -- UserMenu.test.tsx`
Expected: FAIL (module `./UserMenu` does not exist).

- [ ] **Step 3: Write the implementation**

Create `fe/src/components/UserMenu.tsx`:

```tsx
'use client'

import { useEffect, useRef, useState } from 'react'
import { signOut } from 'firebase/auth'
import type { User } from 'firebase/auth'
import { auth } from '@/lib/firebase'

interface Props {
  user: User
}

export default function UserMenu({ user }: Props) {
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function handleMouseDown(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleMouseDown)
    return () => document.removeEventListener('mousedown', handleMouseDown)
  }, [])

  return (
    <div ref={containerRef} className="relative">
      <img
        src={user.photoURL ?? undefined}
        alt={user.displayName ?? 'user'}
        onClick={() => setOpen((prev) => !prev)}
        className="w-8 h-8 rounded-full cursor-pointer"
      />
      {open && (
        <div
          role="menu"
          className="absolute right-0 mt-2 w-48 bg-white rounded-lg shadow-lg border border-gray-200 py-1 z-50"
        >
          <div className="px-4 py-2 text-sm text-gray-700 font-medium border-b border-gray-100">
            {user.displayName ?? user.email}
          </div>
          <button
            role="menuitem"
            onClick={() => signOut(auth)}
            className="w-full text-left px-4 py-2 text-sm text-red-600 hover:bg-gray-50 cursor-pointer"
          >
            Sign out
          </button>
        </div>
      )}
    </div>
  )
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd fe && npm test -- UserMenu.test.tsx`
Expected: PASS (8 tests).

- [ ] **Step 5: Commit**

```bash
git add fe/src/components/UserMenu.tsx fe/src/components/UserMenu.test.tsx
git commit -m "feat(fe): add UserMenu (avatar, sign-out dropdown; ported from corgi)"
```

---

### Task 6: `LoginScreen` component

**Files:**
- Create: `src/components/LoginScreen.tsx`, `src/components/LoginScreen.test.tsx`

**Interfaces:**
- Consumes: `auth` from `@/lib/firebase` (Task 3), `Button` from `@/components/ui/button` (existing).
- Produces: `LoginScreen(): JSX.Element`, consumed by `AuthGate.tsx` (Task 7).

- [ ] **Step 1: Write the failing test**

Create `fe/src/components/LoginScreen.test.tsx`:

```tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'

vi.mock('@/lib/firebase', () => ({ auth: {} }))

const mockSignIn = vi.fn().mockResolvedValue(undefined)
vi.mock('firebase/auth', () => ({
  GoogleAuthProvider: vi.fn(),
  signInWithPopup: (...args: unknown[]) => mockSignIn(...args),
}))

import LoginScreen from './LoginScreen'

describe('LoginScreen', () => {
  it('calls signInWithPopup when the sign-in button is clicked', () => {
    render(<LoginScreen />)
    fireEvent.click(screen.getByRole('button', { name: /sign in with google/i }))
    expect(mockSignIn).toHaveBeenCalledTimes(1)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd fe && npm test -- LoginScreen.test.tsx`
Expected: FAIL (module `./LoginScreen` does not exist).

- [ ] **Step 3: Write the implementation**

Create `fe/src/components/LoginScreen.tsx`:

```tsx
'use client'

import { GoogleAuthProvider, signInWithPopup } from 'firebase/auth'
import { auth } from '@/lib/firebase'
import { Button } from '@/components/ui/button'
import Image from 'next/image'

export default function LoginScreen() {
  async function handleSignIn() {
    await signInWithPopup(auth, new GoogleAuthProvider())
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center p-4">
      <div className="text-center">
        <div className="flex items-center justify-center gap-2 mb-6">
          <Image src="/eagle-thumbnail.png" alt="Eagle logo" width={40} height={40} />
          <h1 className="text-3xl font-bold text-gray-900">Eagle</h1>
        </div>
        <Button onClick={handleSignIn}>Sign in with Google</Button>
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd fe && npm test -- LoginScreen.test.tsx`
Expected: PASS (1 test).

- [ ] **Step 5: Commit**

```bash
git add fe/src/components/LoginScreen.tsx fe/src/components/LoginScreen.test.tsx
git commit -m "feat(fe): add LoginScreen (Google sign-in button)"
```

---

### Task 7: `AuthGate` component

**Files:**
- Create: `src/components/AuthGate.tsx`, `src/components/AuthGate.test.tsx`

**Interfaces:**
- Consumes: `auth` from `@/lib/firebase` (Task 3), `LoginScreen` (Task 6).
- Produces: `AuthGate({ children: (user: User) => ReactNode }): JSX.Element`, consumed by `page.tsx` (Task 8).

- [ ] **Step 1: Write the failing tests**

Create `fe/src/components/AuthGate.test.tsx`:

```tsx
import { render, screen, act } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import type { User } from 'firebase/auth'

vi.mock('@/lib/firebase', () => ({ auth: {} }))

let authCallback: ((user: User | null) => void) | undefined
vi.mock('firebase/auth', () => ({
  onAuthStateChanged: vi.fn((_auth: unknown, cb: (user: User | null) => void) => {
    authCallback = cb
    return () => {}
  }),
}))

import AuthGate from './AuthGate'

describe('AuthGate', () => {
  it('renders nothing while the auth state is still loading', () => {
    const { container } = render(<AuthGate>{() => <div>content</div>}</AuthGate>)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders the login screen when signed out', () => {
    render(<AuthGate>{() => <div>content</div>}</AuthGate>)
    act(() => authCallback!(null))
    expect(screen.getByRole('button', { name: /sign in with google/i })).toBeInTheDocument()
  })

  it('renders children with the signed-in user', () => {
    const fakeUser = { uid: 'u1', displayName: 'Jane' } as User
    render(<AuthGate>{(user) => <div>hello {user.displayName}</div>}</AuthGate>)
    act(() => authCallback!(fakeUser))
    expect(screen.getByText('hello Jane')).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd fe && npm test -- AuthGate.test.tsx`
Expected: FAIL (module `./AuthGate` does not exist).

- [ ] **Step 3: Write the implementation**

Create `fe/src/components/AuthGate.tsx`:

```tsx
'use client'

import { useEffect, useState, type ReactNode } from 'react'
import { onAuthStateChanged, type User } from 'firebase/auth'
import { auth } from '@/lib/firebase'
import LoginScreen from './LoginScreen'

interface Props {
  children: (user: User) => ReactNode
}

export default function AuthGate({ children }: Props) {
  const [user, setUser] = useState<User | null | undefined>(undefined)

  useEffect(() => onAuthStateChanged(auth, setUser), [])

  if (user === undefined) return null
  if (user === null) return <LoginScreen />
  return <>{children(user)}</>
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd fe && npm test -- AuthGate.test.tsx`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add fe/src/components/AuthGate.tsx fe/src/components/AuthGate.test.tsx
git commit -m "feat(fe): add AuthGate (onAuthStateChanged gate, shows LoginScreen when signed out)"
```

---

### Task 8: Wire it together — extract `Translator.tsx`, gate `page.tsx`, fix env files

This is the integration task: the existing 405-line `page.tsx` body moves
into `Translator.tsx` almost verbatim, with its three raw `fetch` calls
replaced by `api.ts` calls, a `user` prop added, and a `UserMenu` added to
the header. `page.tsx` becomes a 10-line wrapper. No new tests are added
here — this relocates and lightly edits already-working, already-manually-
verified UI code; it's checked via `npm run build` and a manual dev-server
pass (see Step 5), consistent with how this project verifies UI changes.

**Files:**
- Create: `src/components/Translator.tsx`
- Modify: `src/app/page.tsx`, `.env.local`, `.env.production`

**Interfaces:**
- Consumes: `api`, `Sentence`, `AnswerHistory`, `CheckAnswerResponse` from `@/lib/api` (Task 4); `UserMenu` (Task 5); `AuthGate` (Task 7).

- [ ] **Step 1: Create `Translator.tsx`**

Create `fe/src/components/Translator.tsx`:

```tsx
'use client'

import { useState, useEffect } from 'react'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import { CheckCircle, XCircle, Volume2 } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import Image from 'next/image'
import type { User } from 'firebase/auth'
import { api, type Sentence, type AnswerHistory } from '@/lib/api'
import UserMenu from './UserMenu'

interface Props {
  user: User
}

export default function Translator({ user }: Props) {
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

  const speakJapanese = (text: string) => {
    if ('speechSynthesis' in window) {
      speechSynthesis.cancel()
      setIsSpeaking(true)

      const speak = () => {
        const utterance = new SpeechSynthesisUtterance(text)

        // Try to find a Japanese voice
        const voices = speechSynthesis.getVoices()
        const japaneseVoice = voices.find(voice =>
          voice.lang.startsWith('ja') || voice.lang.includes('JP')
        )

        if (japaneseVoice) {
          utterance.voice = japaneseVoice
        }

        utterance.lang = 'ja-JP'
        utterance.rate = 0.8
        utterance.pitch = 1
        utterance.volume = 1

        utterance.onstart = () => {
          setIsSpeaking(true)
        }

        utterance.onend = () => {
          setIsSpeaking(false)
        }

        utterance.onerror = () => {
          setIsSpeaking(false)
        }

        speechSynthesis.speak(utterance)

        // Safari workaround: Force reset if no speech after 500ms
        setTimeout(() => {
          if (!speechSynthesis.speaking && !speechSynthesis.pending) {
            setIsSpeaking(false)
          }
        }, 500)
      }

      // Wait for voices to load
      setTimeout(() => {
        speak()
      }, 100)
    }
  }

  const reportSentence = async (sentenceId: number) => {
    try {
      await api.reportSentence(sentenceId)
      setIsReported(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to report sentence')
    }
  }

  const getRandomSentence = async () => {
    try {
      setLoading(true)
      setError(null)
      const sentence = await api.getRandomSentence()
      setCurrentSentence(sentence)
      setCorrectCount(sentence.correct_count)
      setIncorrectCount(sentence.incorrect_count)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load sentence')
    } finally {
      setLoading(false)
    }
  }

  const capitalizeFirstLetter = (text: string) => {
    return text.charAt(0).toUpperCase() + text.slice(1)
  }

  const checkTranslation = async () => {
    if (!currentSentence) return

    const trimmedUserTranslation = userTranslation.trim()

    try {
      const result = await api.checkAnswer(currentSentence.id, trimmedUserTranslation)
      setFeedback(result.is_correct ? 'correct' : 'incorrect')
      setHistories(result.histories)
      setShowAnswer(true)

      // Update counters
      if (result.is_correct) {
        setCorrectCount(prev => prev + 1)
      } else {
        setIncorrectCount(prev => prev + 1)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to check answer')
    }
  }

  const nextSentence = () => {
    setUserTranslation('')
    setFeedback(null)
    setShowAnswer(false)
    setHistories([])
    setError(null)
    setCorrectCount(0)
    setIncorrectCount(0)
    setIsReported(false)
    setIsSpeaking(false)
    speechSynthesis.cancel()
    getRandomSentence()
  }

  useEffect(() => {
    getRandomSentence()
  }, [])

  if (loading) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 p-4 flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mx-auto mb-4"></div>
          <p className="text-gray-600">Loading...</p>
        </div>
      </div>
    )
  }

  if (error || !currentSentence) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 p-4 flex items-center justify-center">
        <Card className="max-w-md">
          <CardHeader>
            <CardTitle className="text-red-600">Error</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-gray-700 mb-4">{error || 'Failed to load content'}</p>
            <Button onClick={() => getRandomSentence()} className="w-full">
              Try Again
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 p-4">
      <div className="max-w-2xl mx-auto">
        <div className="mb-8">
          <div className="flex items-center justify-end mb-2">
            <UserMenu user={user} />
          </div>
          <div className="flex items-center justify-center gap-2">
            <Image src="/eagle-thumbnail.png" alt="Eagle logo" width={32} height={32} />
            <h1 className="text-3xl font-bold text-gray-900">Eagle</h1>
          </div>
        </div>

        <div className="grid gap-6 mb-6">
          <Card>
            <CardHeader>
              <CardTitle>Translate this sentence</CardTitle>
              <CardDescription>Translate the Japanese sentence below into English</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="text-center">
                <div className="flex items-center justify-center gap-3 mb-2">
                  <div className="text-3xl font-bold text-gray-900">
                    {currentSentence.japanese}
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => speakJapanese(currentSentence.japanese)}
                    disabled={isSpeaking}
                    className="flex items-center px-2 py-1"
                  >
                    <Volume2 className="h-3 w-3" />
                  </Button>
                </div>
                <div className="flex justify-center gap-4 text-sm text-gray-600 mt-2">
                  <div className="flex items-center gap-1">
                    <CheckCircle className="h-4 w-4 text-green-500" />
                    <span>Correct: {correctCount}</span>
                  </div>
                  <div className="flex items-center gap-1">
                    <XCircle className="h-4 w-4 text-red-500" />
                    <span>Incorrect: {incorrectCount}</span>
                  </div>
                </div>
              </div>

              <form
                onSubmit={e => {
                  e.preventDefault()
                  if (userTranslation.trim() && !showAnswer) {
                    checkTranslation()
                  }
                }}
                className="space-y-4"
              >
                <div className="space-y-2">
                  <Label htmlFor="translation">Your English translation:</Label>
                  <Textarea
                    id="translation"
                    value={userTranslation}
                    onChange={e => setUserTranslation(e.target.value)}
                    placeholder="Enter your translation here..."
                    disabled={showAnswer}
                    onBlur={e => {
                      if (e.target.value.trim() && !showAnswer) {
                        const capitalizedTranslation = capitalizeFirstLetter(e.target.value.trim())
                        setUserTranslation(capitalizedTranslation)
                      }
                    }}
                    onKeyDown={e => {
                      if (e.key === 'Enter' && e.ctrlKey && userTranslation.trim() && !showAnswer) {
                        checkTranslation()
                      }
                    }}
                    aria-label="Your English translation"
                    aria-required="true"
                  />
                </div>

                {feedback && (
                  <Alert
                    className={
                      feedback === 'correct'
                        ? 'border-green-500 bg-green-50'
                        : 'border-red-500 bg-red-50'
                    }
                  >
                    <div className="flex items-center gap-2">
                      {feedback === 'correct' ? (
                        <CheckCircle className="h-4 w-4 text-green-600" />
                      ) : (
                        <XCircle className="h-4 w-4 text-red-600" />
                      )}
                      <AlertDescription
                        className={
                          feedback === 'correct' ? 'text-green-800' : 'text-red-800'
                        }
                      >
                        {feedback === 'correct'
                          ? 'Correct! Well done!'
                          : 'Not quite right. Try again!'}
                      </AlertDescription>
                    </div>
                  </Alert>
                )}

                {!showAnswer && (
                  <Button
                    type="submit"
                    disabled={!userTranslation.trim()}
                    className="w-full bg-gray-500 hover:bg-black text-white"
                  >
                    Check Translation
                  </Button>
                )}
              </form>

              {showAnswer && (
                <div className="space-y-4">
                  <div className="p-4 bg-blue-50 rounded-lg border border-blue-200">
                    <div className="font-semibold text-blue-900 mb-1">
                      Correct Answer:
                    </div>
                    <div className="text-blue-800">{currentSentence.english}</div>
                  </div>

                  {histories.length > 0 && (
                    <div className="p-4 bg-yellow-50 rounded-lg border border-yellow-200">
                      <div className="font-semibold text-yellow-900 mb-2">
                        Previous Incorrect Answers:
                      </div>
                      <ul className="text-yellow-800 space-y-1">
                        {histories.map(history => (
                          <li key={history.id} className="text-sm">
                            &ldquo;{history.incorrect_answer}&rdquo;
                          </li>
                        ))}
                      </ul>
                    </div>
                  )}
                </div>
              )}
            </CardContent>
            <CardFooter className="flex gap-2">
              {showAnswer && (
                <>
                  <Button onClick={nextSentence} className="flex-1">
                    Next Sentence
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      if (currentSentence) {
                        reportSentence(currentSentence.id)
                      }
                    }}
                    disabled={isReported}
                  >
                    {isReported ? 'Reported' : 'Report'}
                  </Button>
                </>
              )}
            </CardFooter>

          </Card>
        </div>
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Replace `page.tsx` with the thin auth-gated wrapper**

Replace the entire contents of `fe/src/app/page.tsx` with:

```tsx
'use client'

import AuthGate from '@/components/AuthGate'
import Translator from '@/components/Translator'

export default function Page() {
  return <AuthGate>{(user) => <Translator user={user} />}</AuthGate>
}
```

- [ ] **Step 3: Fix the env files**

Replace `fe/.env.local`:

```
NEXT_PUBLIC_API_URL=http://localhost:8080
NEXT_PUBLIC_FIREBASE_API_KEY=
NEXT_PUBLIC_FIREBASE_AUTH_DOMAIN=
NEXT_PUBLIC_FIREBASE_PROJECT_ID=
```

Replace `fe/.env.production`:

```
NEXT_PUBLIC_API_URL=
NEXT_PUBLIC_FIREBASE_API_KEY=
NEXT_PUBLIC_FIREBASE_AUTH_DOMAIN=
NEXT_PUBLIC_FIREBASE_PROJECT_ID=
```

`NEXT_PUBLIC_API_URL=` (empty) means the app fetches relative `/api/**` paths
in production, matching the same-origin Hosting-rewrite design. The three
`NEXT_PUBLIC_FIREBASE_*` values stay empty until the infra plan provisions a
real Firebase project — both files are already gitignored (`fe/.gitignore`'s
`.env*` pattern) so this is safe to leave as local placeholders.

- [ ] **Step 4: Run the full test suite and type-check**

Run: `cd fe && npm test && npx tsc --noEmit`
Expected: all tests still pass (16 total: 4 api + 8 UserMenu + 1 LoginScreen +
3 AuthGate); no type errors.

- [ ] **Step 5: Manual verification**

Run: `cd fe && npm run dev`, open `http://localhost:3000`.

Expected with empty `NEXT_PUBLIC_FIREBASE_*` values: the page loads without a
white-screen crash and shows *some* rendered UI (either the login screen or a
Firebase initialization error surfaced in the console) rather than hanging
indefinitely — confirming `AuthGate` doesn't get stuck and the build/runtime
wiring is correct. **Full interactive sign-in cannot be verified until the
infra plan provisions a real Firebase project** — note this explicitly rather
than claiming the auth flow works end-to-end.

Also run: `cd fe && npm run build`
Expected: succeeds and produces `fe/out/` (static export).

Stop the dev server when done.

- [ ] **Step 6: Commit**

```bash
git add fe/src/components/Translator.tsx fe/src/app/page.tsx fe/.env.local fe/.env.production
git commit -m "feat(fe): gate the app behind Firebase Auth sign-in

Extracts the existing translator UI into Translator.tsx (now using the
authenticated api.ts client and rendering UserMenu), and makes page.tsx
a thin AuthGate wrapper that shows LoginScreen when signed out."
```

---

## Verification (whole plan)

- [ ] `cd fe && npm test` passes all 16 tests (4 api.ts + 8 UserMenu + 1 LoginScreen + 3 AuthGate).
- [ ] `cd fe && npx tsc --noEmit` reports no errors.
- [ ] `cd fe && npm run lint` passes (all new test files use explicit `import ... from 'vitest'`, so no undefined-global lint errors are expected).
- [ ] `cd fe && npm run build` succeeds and produces `fe/out/`.
- [ ] `git grep -n "API_BASE_URL\|fetch(" fe/src/app fe/src/components` finds no raw `fetch` calls left in UI code (all routed through `@/lib/api`).
- [ ] `test -f fe/Dockerfile` fails (file removed).

## Notes for the next plan (infra/deploy)

- Provision a Firebase project, enable the Google sign-in provider, and fill
  in real values for `NEXT_PUBLIC_FIREBASE_API_KEY`, `NEXT_PUBLIC_FIREBASE_AUTH_DOMAIN`,
  `NEXT_PUBLIC_FIREBASE_PROJECT_ID` — these need to reach the GitHub Actions
  build step (as env vars/secrets) since `.env.production` is gitignored and
  contains only placeholders.
- Retire `.github/workflows/build_fe_docker_image.yaml` (it built the now-deleted
  `fe/Dockerfile`) and replace it with a workflow that runs `next build` and
  `firebase deploy --only hosting`.
- Set the Cloud Run `FRONTEND_URL` env var to the real deployed Firebase
  Hosting URL, so the backend's CORS (`withCORS`, from the backend plan's
  Task 7) restricts to it instead of defaulting to `*`.
- `firebase.json` needs a Hosting rewrite for `/api/**` → the Cloud Run
  service, matching the "production is same-origin" design.
- Once a real Firebase project exists, do the full manual verification this
  plan's Task 8 could not complete: sign in with Google, confirm `AuthGate`
  renders `Translator`, complete a solve/report/next loop, sign out, confirm
  it returns to `LoginScreen`.
