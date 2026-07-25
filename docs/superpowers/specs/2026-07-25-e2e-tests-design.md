# E2E Tests for Basic Features — Design

## Goal

Add a Playwright-based end-to-end test suite covering eagle's core user flows — sign-in/out, translation checking, the Explain button, and report/next-sentence — running against the real Go API, real Next.js frontend, and real HTTP/auth wiring, but with Firebase emulators and a stubbed LLM so the suite is deterministic, free, and fast enough to run on every PR.

## Non-Goals

- Real Gemini API calls in e2e tests (stays covered by existing Go unit tests with `fakeContentGenerator`, plus the manual local-env smoke test already done for the Explain feature).
- Real Firestore/production data (Firestore emulator with seeded fixtures instead).
- Cross-browser coverage (Chromium only for now).
- Visual regression / accessibility testing (out of scope for this suite).

## Architecture

A new top-level `e2e/` directory (sibling to `api/` and `fe/`), since these tests orchestrate both processes plus emulators — not a frontend-only concern.

Moving pieces during a test run:

1. **Firebase emulators** (Auth + Firestore), started via the Firebase CLI (`firebase emulators:exec`/`emulators:start`), configured via a new `emulators` block in the existing root `firebase.json`, with fixed ports.
2. **Firestore seed data**: the existing `api/cmd/seed` binary (already reads an NDJSON file and writes to the `sentences` collection via `firestore.NewClient(ctx, projectID)`, which auto-detects `FIRESTORE_EMULATOR_HOST`) run against a new small fixture file, `e2e/fixtures/sentences.ndjson` (2-3 known sentences with fixed Japanese/English pairs).
3. **Go API server**, started via a new `api/cmd/e2eserver` entrypoint with `FIRESTORE_EMULATOR_HOST` / `FIREBASE_AUTH_EMULATOR_HOST` set (both auto-detected by the existing Firestore client and `firebaseVerifier` — no code change needed there) and wired with a canned-response stub `Explainer` instead of `GeminiExplainer`.
4. **Next.js frontend**, started with `NEXT_PUBLIC_FIREBASE_AUTH_EMULATOR_HOST` set, which makes `fe/src/lib/firebase.ts` conditionally call `connectAuthEmulator`.
5. **Playwright**, driving real Chromium: clicks "Sign in with Google" → the Auth emulator's own local fake consent screen (no real Google OAuth, no bot-detection risk) → creates/picks a fixed test account, `e2e-test@example.com` → lands on the Translator screen backed by real seeded data through the real HTTP/auth stack.

`requireAuth` checks the verified email against a single `ALLOWED_EMAIL`. The e2e server (below) sets `ALLOWED_EMAIL=e2e-test@example.com` to match the fixed emulator account Playwright signs in with — same allowlist mechanism as production, just pointed at the test identity instead of the real user's email.

This reuses the existing `Explainer` / `SentenceRepository` / `TokenVerifier` interfaces exactly as they were designed to be substituted, and keeps production `api/main.go`'s logic untouched aside from a pure extraction (see below) — no test-only conditionals in the production entrypoint.

## Backend Changes

1. **`api/handlers_router.go`** (new): extract the `mux.HandleFunc(...)` route-wiring block out of `main.go` into `newMux(srv *Server, verifier TokenVerifier, frontendURL string) *http.ServeMux`. `main.go` calls this unchanged — pure refactor, no behavior change, existing tests must still pass unmodified.
2. **`api/cmd/e2eserver/main.go`** (new): mirrors `main.go`'s wiring (real `NewFirestoreRepo`, real `NewFirebaseVerifier`, and the same `ALLOWED_EMAIL` env var read as `main.go` — set to `e2e-test@example.com` for this suite) but constructs a canned-response stub `Explainer` (same shape as the existing `fakeExplainer` test helper, returning a fixed string such as `"This is a stub explanation for e2e tests."`) instead of `NewGeminiExplainer`, and does not require `GEMINI_API_KEY`. Calls the shared `newMux`.
3. **`e2e/fixtures/sentences.ndjson`** (new): 2-3 fixed test sentences (id, japanese, english, page, is_reported), seeded via the existing `cmd/seed -file` flag.

## Frontend Changes

1. **`fe/src/lib/firebase.ts`**: after `getAuth(app)`, conditionally call `connectAuthEmulator(auth, ...)` when `NEXT_PUBLIC_FIREBASE_AUTH_EMULATOR_HOST` is set (Firebase's own documented emulator-testing pattern), guarded by the same `typeof window !== 'undefined'` check already in place. Unset in production builds, so this is a no-op there — zero prod behavior change.
2. No Firestore wiring needed on the frontend — only the Go backend talks to Firestore directly.
3. **`e2e/.env.e2e`** (new, gitignored): `NEXT_PUBLIC_FIREBASE_AUTH_EMULATOR_HOST=http://localhost:9099`, dummy Firebase config values, `NEXT_PUBLIC_API_URL=http://localhost:8080`.

## Config Changes

**`firebase.json`**: add an `emulators` block (fixed ports for `auth` and `firestore`). Additive only — no effect on the existing `hosting`/`firestore.indexes` config or the real deploy flow.

## Test Scope

Seeded Firestore data: 2-3 fixed sentences with known `japanese`/`english` pairs, so tests assert exact expected values instead of handling random real data.

Test cases, across 4 independent spec files (fresh sign-in per test, no shared state, safe to run in parallel):

**`auth.spec.ts`**
- Loads the login screen when signed out
- Sign in via the Auth emulator → lands on the Translator screen with a sentence loaded
- Sign out via `UserMenu` → returns to the login screen

**`correct-answer.spec.ts`**
- Submit the seeded sentence's exact correct answer → sees correct feedback, Correct counter increments

**`incorrect-explain.spec.ts`**
- Submit a wrong answer → sees incorrect feedback + reference answer, Incorrect counter increments, Explain button appears
- Click Explain → stubbed explanation text renders

**`report-next.spec.ts`**
- Report the current sentence → no error, UI acknowledges
- Click Next Sentence → a new sentence loads and per-sentence UI state resets (explanation cleared, textarea cleared, feedback cleared)

## CI Integration

New `.github/workflows/e2e.yml`, triggered on `pull_request`, alongside the existing Go/Vitest workflows:

1. Checkout; set up Node, Go, and Java (the Firestore/Auth emulators run on the JVM); install the Firebase CLI and Playwright (Chromium only, to keep CI time/cost down).
2. `firebase emulators:exec --only auth,firestore "<orchestration script>"`, where the wrapped script:
   - Seeds sentences via `go run ./cmd/seed -file ../e2e/fixtures/sentences.ndjson` (with `FIRESTORE_EMULATOR_HOST` set)
   - Starts `go run ./cmd/e2eserver` in the background
   - Starts the Next.js server (`next build && next start`, pointed at the emulator + local API) in the background
   - Runs `npx playwright test`
3. Uploads the Playwright HTML report as a build artifact on failure, for debugging.

## Local Dev Workflow

A single `e2e/package.json` script, `npm run test:e2e`, performs the same orchestration as CI (via Playwright's `globalSetup`/`globalTeardown` hooks, or a small shell script) so a developer can run the whole suite with one command without hand-starting emulators or servers.

## Testing (of this suite itself)

- Run the full suite locally against the emulators before merging, confirming all 8 cases pass deterministically across a few repeated runs (checking for flakiness in the sign-in-via-emulator step in particular, since it's the newest/least-proven part of the flow).
- Confirm the existing Go (`go test ./...`) and Vitest suites still pass unmodified after the `newMux` extraction.
- Confirm the CI workflow passes on an actual PR before considering this done.
