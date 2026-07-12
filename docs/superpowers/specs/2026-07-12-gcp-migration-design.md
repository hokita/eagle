# Eagle → GCP Migration Design

**Date:** 2026-07-12
**Status:** Approved (design)
**Reference architecture:** [github.com/hokita/corgi](https://github.com/hokita/corgi/) (Firebase Hosting + Cloud Run + Firestore)

## Goal

Move Eagle off the current self-hosted setup (home Kubernetes + MySQL, images
in GHCR) onto Google Cloud, mirroring the corgi project's architecture:

- **Frontend** (Next.js) → Firebase Hosting (static export)
- **Backend** (Go API) → Cloud Run
- **Database** (MySQL) → Firestore (Native mode)
- **CI/CD** → GitHub Actions deploying to GCP via Workload Identity Federation

The public JSON API contract stays **byte-for-byte identical** so the frontend
needs no logic changes.

## Decisions locked in

| Area | Decision | Rationale |
| --- | --- | --- |
| Database | Firestore | Matches corgi; serverless, near-$0 at low traffic |
| Frontend host | Firebase Hosting (static Next export) | App is a pure client SPA; matches corgi |
| Backend host | Cloud Run (containerized Go) | Existing Dockerfile fits directly |
| Data migration | Seed sentences from CSV, reset history | Simplest; history only drives resurfacing |
| CI/CD | Automated via GitHub Actions (WIF) | Matches corgi; keyless |
| Auth | None (single-user) | YAGNI; unchanged from today |

## Current state (baseline)

- **api/**: Go `net/http` + MySQL (`go-sql-driver`). Answer check is a plain
  trim/lowercase string comparison (`main.go:246`) — **not** LLM-based. Health
  endpoints (`/api/readiness`, `/api/liveness`) exist for Kubernetes.
- **fe/**: Next.js 15 in `output: "standalone"`. Single client page
  (`page.tsx`, `"use client"`), no route handlers, no server components. Uses
  `next/image` (`page.tsx:241`) and browser TTS.
- **DB**: MySQL, relational. The random endpoint uses `JOIN` + `GROUP BY` /
  `HAVING correct_count - incorrect_count < 2`.
- **Deploy**: GitHub Actions builds multi-arch images → GHCR → home k8s
  (`fe/.env.production` → `http://192.168.1.101:30005`, a NodePort).

## Target architecture

```
Browser
  │
  ▼
Firebase Hosting  (static Next.js export, global CDN)
  │   rewrite  /api/**  →  Cloud Run
  ▼
Cloud Run  (Go API, runs as a dedicated service account)
  │   Firestore Go SDK  (Application Default Credentials)
  ▼
Firestore (Native mode)
  ├─ sentences/{id}                          { japanese, english, page, is_reported,
  │                                            correct_count, incorrect_count,
  │                                            created_at, updated_at }
  └─ sentences/{id}/answer_histories/{auto}   { is_correct, incorrect_answer, created_at }
```

Hosting rewrites proxy `/api/**` to Cloud Run, so the app is **same-origin** and
the hand-rolled CORS in `enableCORS` is removed.

## Firestore data model

### `sentences/{id}`

- **Document ID** = the original integer id rendered as a string (`"1"`, `"2"`,
  …). This lets the API keep emitting an integer `id` and lets `check`/`report`
  look up a doc by `strconv.Itoa(sentence_id)`.
- Fields: `japanese`, `english`, `page`, `is_reported` (bool),
  `correct_count` (int), `incorrect_count` (int), `created_at`, `updated_at`.
- The counters are **denormalized onto the doc** so the random query needs no
  join or server-side aggregation.

### `sentences/{id}/answer_histories/{autoId}` (subcollection)

- Fields: `is_correct` (bool), `incorrect_answer` (string), `created_at`.
- A subcollection (not a top-level collection) keeps the per-sentence history
  query trivial and scoped.

## Endpoint translations (contract unchanged)

### `GET /api/sentence/random`

1. Query `sentences where is_reported == false`.
2. In Go, keep docs where `correct_count - incorrect_count < 2`.
3. Pick a uniformly random survivor; return the existing JSON shape (integer
   `id`, counts read straight from the doc).
4. `404` when no candidate remains (same as today).

> **Scale note:** this reads all non-reported sentences per call. Fine for a
> personal dataset (~hundreds of rows). If it ever grows large, switch to the
> Firestore "random field" pattern (store a `random` float, query
> `where ... order by random limit 1`). Out of scope for now.

### `POST /api/answer/check`

1. Get `sentences/{id}`; `404` if it doesn't exist.
2. Query subcollection `answer_histories where is_correct == false order by
   created_at desc` → build the `histories` array.
3. Compare `trim(lower(user_answer)) == trim(lower(english))` (identical logic).
4. **Batch write**: add one `answer_histories` doc, `increment` the matching
   counter (`correct_count` or `incorrect_count`) on the sentence, bump
   `updated_at`.
5. Return the existing JSON shape. The history `id` field is derived from the
   doc's `created_at` in unix nanoseconds to preserve the integer contract — it
   is only used as a React key in the frontend (`page.tsx:367`).

### `POST /api/sentence/report`

- Update `sentences/{id}` → `is_reported = true`. `204` on success.

### Health endpoints

- `/api/liveness` stays a static `200`.
- `/api/readiness` swaps the MySQL ping for a lightweight Firestore check (or is
  dropped, since Cloud Run has its own health model). Decide during
  implementation; leaning toward a trivial Firestore round-trip.

## Backend refactor (targeted, test-driven)

Introduce a **`SentenceRepository` interface** so HTTP handlers depend on an
abstraction, not a global Firestore client:

```go
type SentenceRepository interface {
    RandomCandidate(ctx context.Context) (*Sentence, error)          // random.go
    GetByID(ctx context.Context, id int) (*Sentence, error)
    ListIncorrectHistories(ctx context.Context, id int) ([]AnswerHistory, error)
    RecordAnswer(ctx context.Context, id int, correct bool, answer string) error
    Report(ctx context.Context, id int) error
}
```

- A `firestoreRepo` implements it; handlers take the interface.
- **Why:** isolates Firestore from HTTP handling → handlers unit-test against a
  fake repo, and the Firestore impl integration-tests against the **Firestore
  emulator**. `main_test.go` is rewritten accordingly, following red/green TDD.
- Config: remove `DB_USER/DB_NAME/DB_PASSWORD/DB_ENDPOINT/CORS_ALLOW_ORIGINS`;
  add `GOOGLE_CLOUD_PROJECT` and optional `FIRESTORE_EMULATOR_HOST`. Auth via
  Application Default Credentials — the Cloud Run service account holds
  `roles/datastore.user`; local dev/tests use the emulator.
- Dependencies: drop `go-sql-driver/mysql`; add `cloud.google.com/go/firestore`.

## Frontend changes (minimal)

- `next.config.ts`: `output: "export"` **and** `images: { unoptimized: true }`
  (required because `page.tsx:241` uses `next/image`, which otherwise needs the
  Next optimization server that static export doesn't run).
- Production env: `NEXT_PUBLIC_API_URL=""` so `fetch` calls hit relative
  `/api/**` (proxied by Hosting to Cloud Run). Local dev keeps
  `http://localhost:8080`.
- No component or logic changes.

## Seeder

- A small Go program (`cmd/seed`) reads `docs/sentences.csv` and upserts
  `sentences/{id}` docs. Idempotent; `correct_count`/`incorrect_count` init to
  `0`; `is_reported` init to `false`. Runs against the emulator or prod via ADC.

## GCP infrastructure

- Enable APIs: Cloud Run, Firestore, Artifact Registry, Firebase Hosting.
- **Firestore** Native mode in one region (align with corgi's region).
- **Artifact Registry** Docker repo for the API image (replaces GHCR).
- **Cloud Run** service for the Go API + a dedicated runtime service account
  granted `roles/datastore.user`.
- **Firebase Hosting** site. `firebase.json` rewrites `/api/**` → the Cloud Run
  service. `firestore.indexes.json` declares the composite index for the
  history query: `answer_histories (is_correct ASC, created_at DESC)`.
- **Workload Identity Federation**: a GCP service account + provider that lets
  the GitHub Actions repo deploy without long-lived JSON keys.

## CI/CD (replaces the two GHCR workflows)

- `deploy-api.yml` — trigger on push to `main` touching `api/**`: build the Go
  image → push to Artifact Registry → `gcloud run deploy`.
- `deploy-fe.yml` — trigger on push to `main` touching `fe/**`: `npm ci` →
  `next build` (static export) → `firebase deploy --only hosting`.
- Both authenticate via WIF. The existing `build_api_docker_image.yaml` and
  `build_fe_docker_image.yaml` are removed.

## Testing strategy

- **Unit**: handlers tested against an in-memory fake `SentenceRepository`
  covering random/check/report happy paths and error paths (404, no candidates).
- **Integration**: `firestoreRepo` tested against the Firestore emulator —
  seeding docs, verifying counter increments, history ordering, and the
  `correct - incorrect < 2` filter.
- **Manual verification** before cutover: seed a project, run the API on Cloud
  Run, load the Hosting URL, complete a full solve/report/next loop.
- All new work follows red/green TDD (write the failing test first).

## Out of scope (YAGNI)

- Authentication / multi-user (staying single-user, no login).
- Migrating existing `answer_histories` (counts reset to zero).
- Server-side rendering (the app is client-only).
- The random-field query optimization (only if the dataset grows large).

## Rollout order (informs the implementation plan)

1. Backend repository refactor + Firestore impl + tests (emulator).
2. Seeder + local end-to-end against the emulator.
3. Frontend static-export config change.
4. GCP project bootstrap (APIs, Firestore, Artifact Registry, service accounts,
   WIF) + `firebase.json` / `firestore.indexes.json`.
5. GitHub Actions deploy workflows; retire GHCR workflows.
6. Seed prod Firestore, deploy, manual verification, cutover.
