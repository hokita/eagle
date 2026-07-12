# Eagle → GCP Migration Design

**Date:** 2026-07-12
**Status:** Approved (design)
**Reference architecture:** [github.com/hokita/corgi](https://github.com/hokita/corgi/) (Firebase Hosting + Cloud Run + Firestore + Firebase Auth)

## Goal

Move Eagle off the current self-hosted setup (home Kubernetes + MySQL, images
in GHCR) onto Google Cloud, mirroring the corgi project's architecture:

- **Frontend** (Next.js) → Firebase Hosting (static export)
- **Backend** (Go API) → Cloud Run
- **Database** (MySQL) → Firestore (Native mode)
- **Auth** → Firebase Auth with Google sign-in (OAuth); practice history is per-user
- **CI/CD** → GitHub Actions deploying to GCP via Workload Identity Federation

The public JSON response bodies stay identical so the frontend's rendering logic
is untouched; the only frontend additions are a sign-in gate and an
`Authorization` header on each request.

## Decisions locked in

| Area | Decision | Rationale |
| --- | --- | --- |
| Database | Firestore | Matches corgi; serverless, near-$0 at low traffic |
| Frontend host | Firebase Hosting (static Next export) | App is a pure client SPA; matches corgi |
| Backend host | Cloud Run (containerized Go) | Existing Dockerfile fits directly |
| Data migration | Seed sentences from a live-DB dump (~790 rows), reset history | Faithful master data; history only drives resurfacing |
| CI/CD | Automated via GitHub Actions (WIF) | Matches corgi; keyless |
| Auth | Google OAuth via Firebase Auth | Matches corgi; per-user history/counts |

## Current state (baseline)

- **api/**: Go `net/http` + MySQL (`go-sql-driver`). Answer check is a plain
  trim/lowercase string comparison (`main.go:246`) — **not** LLM-based. No auth
  (CORS `*`). Health endpoints (`/api/readiness`, `/api/liveness`) for k8s.
- **fe/**: Next.js 15 in `output: "standalone"`. Single client page
  (`page.tsx`, `"use client"`), no route handlers, no server components. Uses
  `next/image` (`page.tsx:241`) and browser TTS. No login.
- **DB**: MySQL, relational. The random endpoint uses `JOIN` + `GROUP BY` /
  `HAVING correct_count - incorrect_count < 2`. Counts are global (no user).
- **Deploy**: GitHub Actions builds multi-arch images → GHCR → home k8s
  (`fe/.env.production` → `http://192.168.1.101:30005`, a NodePort).

## Target architecture

```
Browser
  │  Firebase Auth (Google sign-in) → ID token
  ▼
Firebase Hosting  (static Next.js export, global CDN)
  │   fetch /api/**  with  Authorization: Bearer <ID token>
  │   rewrite /api/** → Cloud Run
  ▼
Cloud Run  (Go API, dedicated service account)
  │   auth middleware: verify ID token → uid   (Firebase Admin SDK)
  │   Firestore Go SDK  (Application Default Credentials)
  ▼
Firestore (Native mode)
  ├─ sentences/{id}                              shared bank
  │     { japanese, english, page, is_reported, created_at, updated_at }
  └─ users/{uid}/sentence_stats/{sentenceId}     per-user counters
        { correct_count, incorrect_count, updated_at }
        └─ histories/{auto}                      per-user attempts
              { is_correct, incorrect_answer, created_at }
```

Hosting rewrites proxy `/api/**` to Cloud Run, so the app is **same-origin** and
the hand-rolled CORS is removed. The **frontend never touches Firestore
directly** — all data flows through the authenticated API — so Firestore stays
locked to client access (no security-rules surface to maintain); the Go API
writes via the Admin SDK, which bypasses rules.

## Firestore data model

### `sentences/{id}` (shared)

- **Document ID** = the original integer id rendered as a string (`"1"`, `"2"`,
  …), so the API keeps emitting an integer `id` and `check`/`report` look up a
  doc by `strconv.Itoa(sentence_id)`.
- Fields: `japanese`, `english`, `page`, `is_reported` (bool),
  `created_at`, `updated_at`. **No counts here** — counts are per-user.
- `is_reported` stays **global**: reporting flags the shared sentence content
  for everyone (fine for a personal app; noted as an intentional choice).

### `users/{uid}/sentence_stats/{sentenceId}` (per-user)

- Per-user, per-sentence counters: `correct_count` (int),
  `incorrect_count` (int), `updated_at`. Absent doc = `(0, 0)`.

### `users/{uid}/sentence_stats/{sentenceId}/histories/{autoId}` (per-user)

- Per-user attempts: `is_correct` (bool), `incorrect_answer` (string),
  `created_at`. Nesting histories under the stats doc keeps the per-sentence
  history query trivial and symmetric with the counters.

## Authentication

- **Frontend**: Firebase Auth JS SDK with `GoogleAuthProvider`
  (`signInWithPopup`). App is gated — unauthenticated users see a sign-in
  screen; authenticated users see the translator. Each API `fetch` attaches
  `Authorization: Bearer <ID token>` (token from `getIdToken()`). Firebase web
  config (public values) is injected at build time via `NEXT_PUBLIC_FIREBASE_*`.
- **Backend**: auth **middleware** wraps every `/api/**` handler (except
  `/api/liveness`). It verifies the Firebase ID token with the Firebase Admin
  SDK for Go (`firebase.google.com/go/v4`), extracts `uid`, and injects it into
  the request context. Missing/invalid token → `401`. Token verification needs
  only the project id and Google's public certs (fetched over HTTPS) — no extra
  IAM role.

## Endpoint translations (response bodies unchanged; all now require auth)

### `GET /api/sentence/random`

1. Verify token → `uid`.
2. Query shared `sentences where is_reported == false`.
3. Read the user's `sentence_stats` collection → map `sentenceId → (correct,
   incorrect)`.
4. Keep sentences where `correct - incorrect < 2` (missing stats → `0,0` →
   kept). Pick a uniformly random survivor.
5. Return the existing JSON shape — `correct_count`/`incorrect_count` come from
   the user's stats (`0` if none). `404` when no candidate remains.

> **Scale note:** reads all non-reported sentences plus the user's stats per
> call. Fine for a personal dataset (~hundreds of rows). The Firestore
> "random field" pattern is a future option if it grows large. Out of scope now.

### `POST /api/answer/check`

1. Verify token → `uid`.
2. Get shared `sentences/{id}`; `404` if missing.
3. Query `users/{uid}/sentence_stats/{id}/histories where is_correct == false
   order by created_at desc` → build the `histories` array.
4. Compare `trim(lower(user_answer)) == trim(lower(english))` (identical logic).
5. **Batch write**: add one `histories` doc, `increment` the matching counter
   on `users/{uid}/sentence_stats/{id}` (merge-create if absent), bump
   `updated_at`.
6. Return the existing JSON shape. The history `id` field is derived from the
   doc's `created_at` in unix nanoseconds to preserve the integer contract — it
   is only a React key in the frontend (`page.tsx:367`).

### `POST /api/sentence/report`

1. Verify token → `uid` (any authenticated user may report).
2. Update shared `sentences/{id}` → `is_reported = true`. `204` on success.

### Health endpoints

- `/api/liveness` stays a static `200` and is **exempt from auth** (used by
  probes).
- `/api/readiness` swaps the MySQL ping for a lightweight Firestore check (or is
  dropped, since Cloud Run has its own health model). Decide during
  implementation; leaning toward a trivial Firestore round-trip. Exempt from
  auth.

## Backend refactor (targeted, test-driven)

Introduce a **`SentenceRepository` interface** so HTTP handlers depend on an
abstraction, not a global Firestore client. Per-user methods take `uid`:

```go
type SentenceRepository interface {
    RandomCandidate(ctx context.Context, uid string) (*Sentence, error)
    GetByID(ctx context.Context, id int) (*Sentence, error)                       // shared
    ListIncorrectHistories(ctx context.Context, uid string, id int) ([]AnswerHistory, error)
    RecordAnswer(ctx context.Context, uid string, id int, correct bool, answer string) error
    Report(ctx context.Context, id int) error                                     // shared
}
```

- A `firestoreRepo` implements it; handlers take the interface.
- An **auth middleware** verifies the Firebase ID token and puts `uid` in
  context; handlers read `uid` from context and pass it to the repo.
- **Why:** isolates Firestore and auth from HTTP handling → handlers unit-test
  against a fake repo with a stub uid, and the Firestore impl integration-tests
  against the **Firestore emulator**. `main_test.go` is rewritten accordingly,
  following red/green TDD.
- Config: remove `DB_*`/`CORS_ALLOW_ORIGINS`; add `GOOGLE_CLOUD_PROJECT` and
  optional `FIRESTORE_EMULATOR_HOST`. Auth via Application Default Credentials —
  the Cloud Run service account holds `roles/datastore.user`; local dev/tests
  use the emulator.
- Dependencies: drop `go-sql-driver/mysql`; add `cloud.google.com/go/firestore`
  and `firebase.google.com/go/v4` (for ID-token verification).

## Frontend changes

- `next.config.ts`: `output: "export"` **and** `images: { unoptimized: true }`
  (required because `page.tsx:241` uses `next/image`, which otherwise needs the
  Next optimization server that static export doesn't run).
- Add `firebase` dependency + a small `lib/firebase.ts` initializing the app and
  `GoogleAuthProvider`. Config via `NEXT_PUBLIC_FIREBASE_*` build env.
- **Sign-in gate**: an auth wrapper shows a Google sign-in screen when signed
  out and the translator when signed in; add a sign-out control.
- Attach `Authorization: Bearer <ID token>` to the three `fetch` calls
  (`random`, `check`, `report`); token from the current user's `getIdToken()`.
- Production env: `NEXT_PUBLIC_API_URL=""` so `fetch` hits relative `/api/**`
  (proxied by Hosting to Cloud Run). Local dev keeps `http://localhost:8080`.
- Existing rendering/TTS logic is unchanged.

## Master data migration (sentences)

Only the **`sentences`** master data is migrated; `answer_histories` is **not**
(per-user counts start empty).

**Source of truth — use the live DB, not the CSV.** `docs/sentences.csv` holds
only **220** rows (it matches `dml_master_3.sql` alone). The full master set is
**~790** sentences, currently in the home MySQL `sentences` table (originally
loaded by `docs/dmls/dml_master_1..3.sql`, which total 230 + 340 + 220 =
790 INSERTs). Seeding from the CSV would silently drop ~570 sentences, so we
export from the live table instead.

**Step 1 — Export** the current `sentences` table to a portable
newline-delimited JSON file, e.g.:

```sh
mysql --batch --raw -N "$DB_NAME" -e \
  'SELECT JSON_OBJECT("id", id, "japanese", japanese, "english", english,
                      "page", page, "is_reported", is_reported) FROM sentences' \
  > docs/sentences_export.ndjson
```

This captures the real current content (including any post-DML edits) plus the
existing integer `id` and `is_reported` flags.

**Step 2 — Seed** with a small Go program (`cmd/seed`) that reads the NDJSON and
upserts one Firestore doc per row: `sentences/{id}` (doc ID = the integer `id`
as a string) with `japanese`, `english`, `page`, `is_reported`, and
`created_at`/`updated_at` = now. No counters (per-user, created on demand). The
seeder is **idempotent** (re-running upserts the same doc IDs) and runs against
the Firestore emulator (local dev) or prod (via ADC).

**Fallback (no live DB access):** concatenate `docs/dmls/dml_master_1..3.sql`,
parse each `INSERT INTO sentences (...) VALUES (...)` (handling `''`-escaped
quotes), assign sequential IDs `1..N`, and feed the same seeder. Since history
is discarded there are no foreign keys to preserve, so regenerated IDs are safe.

## GCP infrastructure

- Enable APIs: Cloud Run, Firestore, Artifact Registry, Firebase Hosting,
  Identity Platform (Firebase Auth).
- **Firebase Auth**: enable the Google sign-in provider; add the Hosting domain
  (and `localhost` for dev) to authorized domains; configure the OAuth consent
  screen.
- **Firestore** Native mode in one region (align with corgi's region).
- **Artifact Registry** Docker repo for the API image (replaces GHCR).
- **Cloud Run** service for the Go API + a dedicated runtime service account
  granted `roles/datastore.user`.
- **Firebase Hosting** site. `firebase.json` rewrites `/api/**` → the Cloud Run
  service. `firestore.indexes.json` declares the composite index for the history
  query: collection `histories` `(is_correct ASC, created_at DESC)`.
- **Workload Identity Federation**: a GCP service account + provider that lets
  the GitHub Actions repo deploy without long-lived JSON keys.

## CI/CD (replaces the two GHCR workflows)

- `deploy-api.yml` — trigger on push to `main` touching `api/**`: build the Go
  image → push to Artifact Registry → `gcloud run deploy`.
- `deploy-fe.yml` — trigger on push to `main` touching `fe/**`: `npm ci` →
  `next build` (static export, with `NEXT_PUBLIC_FIREBASE_*` build env) →
  `firebase deploy --only hosting`.
- Both authenticate via WIF. The existing `build_api_docker_image.yaml` and
  `build_fe_docker_image.yaml` are removed.

## Testing strategy

- **Unit**: handlers tested against an in-memory fake `SentenceRepository` with
  a stub `uid`, covering random/check/report happy paths and error paths (404,
  no candidates). Auth middleware tested with valid/missing/invalid tokens
  (Admin SDK verifier behind a small interface so it can be faked).
- **Integration**: `firestoreRepo` tested against the Firestore emulator —
  per-user counter increments, history ordering, the `correct - incorrect < 2`
  filter, and isolation between two `uid`s.
- **Manual verification** before cutover: seed a project, deploy the API to
  Cloud Run, load the Hosting URL, sign in with Google, complete a full
  solve/report/next loop, and confirm history is scoped to the account.
- All new work follows red/green TDD (write the failing test first).

## Out of scope (YAGNI)

- Migrating existing `answer_histories` (per-user counts start at zero).
- Server-side rendering (the app is client-only).
- The random-field query optimization (only if the dataset grows large).
- Restricting *which* Google accounts may sign in (any Google account works;
  add an allowlist later if needed).

## Rollout order (informs the implementation plan)

1. Backend: repository refactor + Firestore impl + auth middleware + tests
   (emulator + fake verifier).
2. Export the live `sentences` table (~790 rows); seeder + local end-to-end
   against the emulator.
3. Frontend: static-export config + Firebase Auth sign-in gate + `Authorization`
   headers.
4. GCP project bootstrap (APIs, Firestore, Artifact Registry, service accounts,
   WIF, Firebase Auth Google provider) + `firebase.json` /
   `firestore.indexes.json`.
5. GitHub Actions deploy workflows; retire GHCR workflows.
6. Seed prod Firestore, deploy, manual verification, cutover.
