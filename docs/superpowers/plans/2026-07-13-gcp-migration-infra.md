# GCP Migration — Infra & CI/CD Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provision a dedicated GCP project for Eagle, wire up Workload Identity Federation for keyless GitHub Actions deploys, replace the two GHCR image-build workflows with `ci.yml` (PR checks) + `deploy-api.yml` + `deploy-fe.yml`, seed production Firestore with the real master sentence data, and do the first full end-to-end deploy + manual verification (the piece the backend and frontend plans both explicitly deferred).

**Architecture:** Cross-origin + CORS, matching `corgi`'s actual production setup — confirmed by reading its real `firebase.json` (no `/api/**` rewrite) and `frontend.yml` (bakes in the real Cloud Run URL as `VITE_API_URL` at build time). No backend or frontend code changes are needed for this; only the infra/build-time values differ from the (now-corrected) original same-origin design. `ALLOWED_EMAIL` is stored in Secret Manager and mounted into Cloud Run, matching corgi's `backend.yml` pattern, rather than a plain env var.

**Tech Stack:** `gcloud` CLI (already authenticated as `hideee.0202@gmail.com`), `firebase` CLI (v15.22.0, already installed), GitHub Actions, Workload Identity Federation (`google-github-actions/auth`), Firestore Native mode, Cloud Run, Artifact Registry, Secret Manager.

## Global Constraints

- **This plan creates real, billable GCP resources and real GitHub repository secrets under your account.** Every step that creates or modifies a resource (not just writes a local file) is marked **[CONFIRM]** and must not run without your explicit go-ahead at execution time — this holds regardless of whether the rest of the plan is executed in batch or step-by-step.
- Project ID: `eagle-473ac1` (first choice; if already taken globally, pick another suffix and use it consistently for the rest of this plan — this is a cosmetic collision, not a design issue). Referred to as `$PROJECT_ID` below.
- Region: `asia-northeast1` (Tokyo) — matches corgi's region, per the design spec.
- GitHub repo: `hokita/eagle`. WIF trust is scoped to exactly this repo.
- Architecture is cross-origin + CORS (see Goal/Architecture above) — **no** `firebase.json` rewrite for `/api/**`, and **no** code changes to `api/` or `fe/src/` in this plan. Only new/modified files are infra config, GitHub workflows, and env values baked in at CI build time.
- `ALLOWED_EMAIL` value: `hideee.0202@gmail.com` (matches the backend's existing `ALLOWED_EMAIL` design from the backend plan).
- Enabling the Google sign-in provider in Firebase Auth has **no CLI path** — confirmed neither `gcloud` (stable or beta) nor the `firebase` CLI expose a command for it. That step is manual (Firebase Console), documented precisely in Task 3.
- Task 13 (seeding real production data) needs network access to the live home-network MySQL instance (`192.168.1.101`, per the original `fe/.env.production`). This sandboxed environment likely cannot reach a private home LAN IP — flagged explicitly in that task rather than assumed away.
- No TDD/red-green cycle applies to pure infrastructure-provisioning steps (there's no test to fail first); "verification" instead means confirming the resource exists and behaves as expected via `gcloud`/`firebase`/`curl`. Steps that produce new committed files (workflows, `firebase.json`) are still reviewed like code.

## Resource/file structure (after this plan)

```
GCP project: eagle-473ac1 (asia-northeast1)
  APIs: run, firestore, artifactregistry, firebase, identitytoolkit, secretmanager
  Firestore: Native mode database
  Artifact Registry: eagle (Docker repo)
  IAM:
    eagle-api-runtime@...      Cloud Run runtime SA (roles/datastore.user, secret accessor)
    eagle-deployer@...          GitHub Actions deploy SA (run.admin, artifactregistry.writer,
                                 iam.serviceAccountUser, firebasehosting.admin)
  Workload Identity Federation:
    pool: github-pool
    provider: github-provider (trust condition: repository == 'hokita/eagle')
  Secret Manager:
    ALLOWED_EMAIL

Firebase:
  Auth: Google sign-in provider enabled (manual), authorized domains configured
  Hosting: site serving fe/out, no /api rewrite

eagle/ (repo)
  .firebaserc                       CREATE: default project eagle-473ac1
  firebase.json                     CREATE: hosting.public = fe/out, firestore.indexes
  firestore.indexes.json             CREATE: histories (is_correct ASC, created_at DESC)
  .github/workflows/
    ci.yml                           CREATE: PR checks (api go vet/test, fe lint/tsc/test)
    deploy-api.yml                    CREATE: build+push+deploy Go API on push to main
    deploy-fe.yml                      CREATE: build+deploy static frontend on push to main
    build_api_docker_image.yaml        DELETE (superseded by deploy-api.yml)
    build_fe_docker_image.yaml          DELETE (superseded by deploy-fe.yml)
  docs/sentences_export.ndjson        CREATE (Task 13, not committed — real data export)
```

---

### Task 1: Create the GCP project, link billing, enable APIs

**Resources:** GCP project `eagle-473ac1`, billing link, enabled APIs.

**Interfaces:**
- Produces: `$PROJECT_ID=eagle-473ac1`, used by every later task.

- [ ] **Step 1: [CONFIRM] Create the project**

```bash
gcloud projects create eagle-473ac1 --name=eagle
```
Expected: `Create in progress...` then success. If the ID is taken, pick a new
suffix and substitute it for the rest of this plan.

- [ ] **Step 2: [CONFIRM] Link billing**

```bash
gcloud billing projects link eagle-473ac1 \
  --billing-account=015EF6-5B317F-011F7B
```
(Billing account confirmed available via `gcloud billing accounts list`.)
Expected: `billingEnabled: true` in the output.

- [ ] **Step 3: [CONFIRM] Enable required APIs**

```bash
gcloud services enable \
  run.googleapis.com \
  firestore.googleapis.com \
  artifactregistry.googleapis.com \
  firebase.googleapis.com \
  identitytoolkit.googleapis.com \
  secretmanager.googleapis.com \
  iamcredentials.googleapis.com \
  --project=eagle-473ac1
```
Expected: no errors; each API appears in `gcloud services list --enabled --project=eagle-473ac1`.

- [ ] **Step 4: Verify**

```bash
gcloud projects describe eagle-473ac1 --format="value(projectId,lifecycleState)"
gcloud services list --enabled --project=eagle-473ac1 --format="value(config.name)"
```
Expected: `eagle-473ac1 ACTIVE`; the 7 APIs above listed.

---

### Task 2: Add Firebase, create the Firestore database

**Resources:** Firebase-enabled project, Firestore Native-mode database.
**Files:** Create `.firebaserc`.

**Interfaces:**
- Consumes: `$PROJECT_ID` (Task 1).
- Produces: Firestore database reachable via `GOOGLE_CLOUD_PROJECT=eagle-473ac1` (already how `api/main.go` and `api/cmd/seed/main.go` connect — no code changes).

- [ ] **Step 1: [CONFIRM] Add Firebase to the project**

```bash
firebase projects:addfirebase eagle-473ac1
```
Expected: confirms the project is now a Firebase project.

- [ ] **Step 2: [CONFIRM] Create the Firestore database**

```bash
gcloud firestore databases create \
  --project=eagle-473ac1 \
  --location=asia-northeast1 \
  --type=firestore-native
```
Expected: database `(default)` created in Native mode, `asia-northeast1`.

- [ ] **Step 3: Create `.firebaserc`**

Create `.firebaserc` at the repo root:

```json
{
  "projects": {
    "default": "eagle-473ac1"
  }
}
```

- [ ] **Step 4: Verify**

```bash
gcloud firestore databases describe --database='(default)' --project=eagle-473ac1 \
  --format="value(locationId,type)"
```
Expected: `asia-northeast1 FIRESTORE_NATIVE`.

- [ ] **Step 5: Commit**

```bash
git add .firebaserc
git commit -m "chore(infra): point Firebase project config at eagle-473ac1"
```

---

### Task 3: Enable Google sign-in (manual — no CLI path)

**Resources:** Firebase Auth provider config, OAuth consent screen, authorized domains.

**Interfaces:**
- Consumes: `$PROJECT_ID` (Task 1).
- Produces: a working Google sign-in flow for `signInWithPopup` (consumed by `fe/src/components/LoginScreen.tsx`, already built).

- [ ] **Step 1: [MANUAL] Enable the Google provider**

Neither `gcloud` (checked `identity-platform` — not a valid command group, stable
or beta) nor the `firebase` CLI (only exposes `auth:import`/`auth:export` for
user data, not provider config) can do this. In the Firebase Console:

1. Go to `https://console.firebase.google.com/project/eagle-473ac1/authentication/providers`.
2. Click **Google** in the Sign-in providers list → **Enable**.
3. Set a project support email (use `hideee.0202@gmail.com`).
4. Save.

- [ ] **Step 2: [MANUAL] Add authorized domains**

In the same Authentication settings → **Settings** tab → **Authorized domains**:
add `localhost` (usually present by default) and the Hosting domain once it
exists (`eagle-473ac1.web.app`, or a custom domain — this may need to be
revisited after Task 9 creates the Hosting site, since the exact domain
isn't known until then).

- [ ] **Step 3: Verify**

Confirm both providers/domains are visible in the Console; full functional
verification happens in Task 14 (first real sign-in against a deployed app).

---

### Task 4: Artifact Registry Docker repository

**Resources:** Artifact Registry repo `eagle` in `asia-northeast1`.

**Interfaces:**
- Produces: image path `asia-northeast1-docker.pkg.dev/eagle-473ac1/eagle/api`, consumed by Task 10 (`deploy-api.yml`).

- [ ] **Step 1: [CONFIRM] Create the repo**

```bash
gcloud artifacts repositories create eagle \
  --repository-format=docker \
  --location=asia-northeast1 \
  --project=eagle-473ac1 \
  --description="Eagle container images"
```
Expected: repo created.

- [ ] **Step 2: Verify**

```bash
gcloud artifacts repositories describe eagle \
  --location=asia-northeast1 --project=eagle-473ac1 \
  --format="value(name,format)"
```
Expected: shows the repo, format `DOCKER`.

---

### Task 5: Cloud Run runtime service account

**Resources:** IAM service account `eagle-api-runtime`, granted `roles/datastore.user`.

**Interfaces:**
- Produces: `eagle-api-runtime@eagle-473ac1.iam.gserviceaccount.com`, consumed by Task 6 (secret access) and Task 10 (`gcloud run deploy --service-account`).

- [ ] **Step 1: [CONFIRM] Create the service account**

```bash
gcloud iam service-accounts create eagle-api-runtime \
  --project=eagle-473ac1 \
  --display-name="Eagle API Cloud Run runtime"
```

- [ ] **Step 2: [CONFIRM] Grant Firestore access**

```bash
gcloud projects add-iam-policy-binding eagle-473ac1 \
  --member="serviceAccount:eagle-api-runtime@eagle-473ac1.iam.gserviceaccount.com" \
  --role="roles/datastore.user"
```

- [ ] **Step 3: Verify**

```bash
gcloud iam service-accounts describe \
  eagle-api-runtime@eagle-473ac1.iam.gserviceaccount.com \
  --project=eagle-473ac1 --format="value(email)"
```
Expected: the service account email.

---

### Task 6: `ALLOWED_EMAIL` in Secret Manager

**Resources:** Secret `ALLOWED_EMAIL` with one version; IAM grant to the runtime SA.

**Interfaces:**
- Consumes: `eagle-api-runtime` SA (Task 5).
- Produces: secret `ALLOWED_EMAIL:latest`, consumed by Task 10's `gcloud run deploy --set-secrets`.

- [ ] **Step 1: [CONFIRM] Create the secret**

```bash
gcloud secrets create ALLOWED_EMAIL --project=eagle-473ac1 --replication-policy=automatic
```

- [ ] **Step 2: [CONFIRM] Add the value**

```bash
printf '%s' 'hideee.0202@gmail.com' | \
  gcloud secrets versions add ALLOWED_EMAIL --project=eagle-473ac1 --data-file=-
```
(Uses `printf '%s'`, not `echo`, to avoid a trailing newline in the secret value.)

- [ ] **Step 3: [CONFIRM] Grant the runtime SA access**

```bash
gcloud secrets add-iam-policy-binding ALLOWED_EMAIL \
  --project=eagle-473ac1 \
  --member="serviceAccount:eagle-api-runtime@eagle-473ac1.iam.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"
```

- [ ] **Step 4: Verify**

```bash
gcloud secrets versions access latest --secret=ALLOWED_EMAIL --project=eagle-473ac1
```
Expected: prints `hideee.0202@gmail.com` with no trailing newline.

---

### Task 7: Workload Identity Federation for GitHub Actions

**Resources:** WIF pool + provider (trust scoped to `hokita/eagle`), deploy-time service account `eagle-deployer` with deploy permissions, GitHub repo secrets.

**Interfaces:**
- Consumes: `eagle-api-runtime` SA (Task 5, for `iam.serviceAccountUser` so the deployer can attach it to Cloud Run).
- Produces: GitHub repo secrets `WIF_PROVIDER`, `WIF_SERVICE_ACCOUNT`, consumed by Tasks 10–11's `google-github-actions/auth` steps.

- [ ] **Step 1: [CONFIRM] Create the workload identity pool**

```bash
gcloud iam workload-identity-pools create github-pool \
  --project=eagle-473ac1 \
  --location=global \
  --display-name="GitHub Actions pool"
```

- [ ] **Step 2: [CONFIRM] Create the OIDC provider, scoped to this repo**

```bash
gcloud iam workload-identity-pools providers create-oidc github-provider \
  --project=eagle-473ac1 \
  --location=global \
  --workload-identity-pool=github-pool \
  --display-name="GitHub provider" \
  --attribute-mapping="google.subject=assertion.sub,attribute.repository=assertion.repository" \
  --attribute-condition="assertion.repository=='hokita/eagle'" \
  --issuer-uri="https://token.actions.githubusercontent.com"
```
The `--attribute-condition` is the security-critical part: it restricts which
GitHub repo can mint tokens this provider will trust, so no other repo
(including `hokita/corgi`) can impersonate Eagle's deploy identity.

- [ ] **Step 3: [CONFIRM] Create the deployer service account**

```bash
gcloud iam service-accounts create eagle-deployer \
  --project=eagle-473ac1 \
  --display-name="Eagle GitHub Actions deployer"
```

- [ ] **Step 4: [CONFIRM] Grant the deployer its permissions**

```bash
PROJECT_NUMBER=$(gcloud projects describe eagle-473ac1 --format="value(projectNumber)")

gcloud projects add-iam-policy-binding eagle-473ac1 \
  --member="serviceAccount:eagle-deployer@eagle-473ac1.iam.gserviceaccount.com" \
  --role="roles/run.admin"

gcloud projects add-iam-policy-binding eagle-473ac1 \
  --member="serviceAccount:eagle-deployer@eagle-473ac1.iam.gserviceaccount.com" \
  --role="roles/artifactregistry.writer"

gcloud iam service-accounts add-iam-policy-binding \
  eagle-api-runtime@eagle-473ac1.iam.gserviceaccount.com \
  --project=eagle-473ac1 \
  --member="serviceAccount:eagle-deployer@eagle-473ac1.iam.gserviceaccount.com" \
  --role="roles/iam.serviceAccountUser"

gcloud projects add-iam-policy-binding eagle-473ac1 \
  --member="serviceAccount:eagle-deployer@eagle-473ac1.iam.gserviceaccount.com" \
  --role="roles/firebasehosting.admin"
```

- [ ] **Step 5: [CONFIRM] Allow the WIF provider to impersonate the deployer**

```bash
gcloud iam service-accounts add-iam-policy-binding \
  eagle-deployer@eagle-473ac1.iam.gserviceaccount.com \
  --project=eagle-473ac1 \
  --role="roles/iam.workloadIdentityUser" \
  --member="principalSet://iam.googleapis.com/projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/github-pool/attribute.repository/hokita/eagle"
```

- [ ] **Step 6: [CONFIRM] Store the provider resource name and SA email as GitHub secrets**

```bash
WIF_PROVIDER="projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/github-pool/providers/github-provider"
gh secret set WIF_PROVIDER --repo hokita/eagle --body "$WIF_PROVIDER"
gh secret set WIF_SERVICE_ACCOUNT --repo hokita/eagle --body "eagle-deployer@eagle-473ac1.iam.gserviceaccount.com"
```

- [ ] **Step 7: Verify**

```bash
gh secret list --repo hokita/eagle
```
Expected: `WIF_PROVIDER` and `WIF_SERVICE_ACCOUNT` listed.

---

### Task 8: `firebase.json` and `firestore.indexes.json`

**Files:** Create `firebase.json`, `firestore.indexes.json`.

**Interfaces:**
- Consumes: nothing new (pure config).
- Produces: Hosting config consumed by Task 11's `firebase deploy --only hosting`; Firestore index consumed by the `ListIncorrectHistories` query in `api/firestore_repo.go` (already built — this task just declares the index Firestore needs to actually serve that query efficiently).

- [ ] **Step 1: Create `firebase.json`**

Create `firebase.json` at the repo root:

```json
{
  "firestore": {
    "indexes": "firestore.indexes.json"
  },
  "hosting": {
    "public": "fe/out",
    "ignore": ["firebase.json", "**/.*", "**/node_modules/**"]
  }
}
```

No `rewrites` block — matches corgi's actual `firebase.json`, and per this
plan's cross-origin architecture, no `/api/**` rewrite is needed. Eagle is a
single-route app (just `/`), so no SPA catch-all rewrite is needed either
(unlike corgi's multi-route `react-router` app).

- [ ] **Step 2: Create `firestore.indexes.json`**

Create `firestore.indexes.json` at the repo root:

```json
{
  "indexes": [
    {
      "collectionGroup": "histories",
      "queryScope": "COLLECTION",
      "fields": [
        { "fieldPath": "is_correct", "order": "ASCENDING" },
        { "fieldPath": "created_at", "order": "DESCENDING" }
      ]
    }
  ],
  "fieldOverrides": []
}
```

This matches the query in `api/firestore_repo.go`'s `ListIncorrectHistories`:
`.Where("is_correct", "==", false).OrderBy("created_at", firestore.Desc)`.

- [ ] **Step 3: [CONFIRM] Deploy the Firestore index**

```bash
firebase deploy --only firestore:indexes --project=eagle-473ac1
```
Expected: index creation started (composite indexes can take a few minutes
to build; this is a real, billed Firestore operation).

- [ ] **Step 4: Verify**

```bash
gcloud firestore indexes composite list --project=eagle-473ac1 \
  --format="table(name,state)"
```
Expected: one composite index for `histories`, eventually `state: READY`.

- [ ] **Step 5: Commit**

```bash
git add firebase.json firestore.indexes.json
git commit -m "chore(infra): add firebase.json (Hosting, no API rewrite) and Firestore composite index"
```

---

### Task 9: Firebase Hosting site

**Resources:** Firebase Hosting site (created implicitly by the first `firebase deploy --only hosting`, or explicitly here to get its URL before Task 3's authorized-domain step needs it).

**Interfaces:**
- Produces: the Hosting URL (`https://eagle-473ac1.web.app`), consumed by Task 3 (authorized domains, if not already done) and Task 10 (`FRONTEND_URL` Cloud Run env var).

- [ ] **Step 1: [CONFIRM] Create the default Hosting site**

```bash
firebase hosting:sites:create eagle-473ac1 --project=eagle-473ac1
```
(Firebase Hosting's default site ID usually matches the project ID; this
creates it explicitly rather than waiting for the first deploy to do it
implicitly, so the URL is known before Task 10.)

- [ ] **Step 2: Verify**

```bash
firebase hosting:sites:list --project=eagle-473ac1
```
Expected: `eagle-473ac1.web.app` listed. Record this URL — it's
`$FRONTEND_URL` in Task 10 and the authorized domain to double-check in Task 3.

---

### Task 10: `deploy-api.yml` and retire the old API workflow

**Files:** Create `.github/workflows/deploy-api.yml`; delete `.github/workflows/build_api_docker_image.yaml`.

**Interfaces:**
- Consumes: `WIF_PROVIDER`/`WIF_SERVICE_ACCOUNT` secrets (Task 7), Artifact Registry repo (Task 4), `eagle-api-runtime` SA (Task 5), `ALLOWED_EMAIL` secret (Task 6), Hosting URL (Task 9).

- [ ] **Step 1: Create `deploy-api.yml`**

Create `.github/workflows/deploy-api.yml`:

```yaml
name: Deploy API

on:
  push:
    branches: [main]
    paths:
      - 'api/**'
      - '.github/workflows/deploy-api.yml'
  workflow_dispatch:

jobs:
  deploy:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      id-token: write

    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'

      - name: Vet and test
        working-directory: api
        run: go vet ./... && go test ./...

      - name: Authenticate to Google Cloud
        uses: google-github-actions/auth@v2
        with:
          workload_identity_provider: ${{ secrets.WIF_PROVIDER }}
          service_account: ${{ secrets.WIF_SERVICE_ACCOUNT }}

      - uses: google-github-actions/setup-gcloud@v2

      - name: Configure Docker for Artifact Registry
        run: gcloud auth configure-docker asia-northeast1-docker.pkg.dev --quiet

      - uses: docker/setup-buildx-action@v3

      - name: Build and push Docker image
        uses: docker/build-push-action@v6
        with:
          context: api
          push: true
          tags: asia-northeast1-docker.pkg.dev/eagle-473ac1/eagle/api:latest

      - name: Deploy to Cloud Run
        run: |
          gcloud run deploy eagle-api \
            --project=eagle-473ac1 \
            --image=asia-northeast1-docker.pkg.dev/eagle-473ac1/eagle/api:latest \
            --region=asia-northeast1 \
            --service-account=eagle-api-runtime@eagle-473ac1.iam.gserviceaccount.com \
            --allow-unauthenticated \
            --min-instances=0 \
            --max-instances=2 \
            --set-env-vars="GOOGLE_CLOUD_PROJECT=eagle-473ac1,FRONTEND_URL=https://eagle-473ac1.web.app" \
            --set-secrets="ALLOWED_EMAIL=ALLOWED_EMAIL:latest"
```

`--allow-unauthenticated` is Cloud Run's IAM-level access control — this app's
real authentication (Firebase ID token + `ALLOWED_EMAIL` allowlist) happens
inside the Go API itself (`requireAuth` middleware), same as corgi's
`backend.yml`.

- [ ] **Step 2: Delete the old workflow**

```bash
git rm .github/workflows/build_api_docker_image.yaml
```

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/deploy-api.yml
git commit -m "ci: add deploy-api.yml (WIF, Artifact Registry, Cloud Run); retire GHCR workflow"
```

(This workflow only *runs* on push to `main` — committing it to this branch
does not trigger a deploy. Task 14 covers the actual first deploy.)

---

### Task 11: `deploy-fe.yml` and retire the old frontend workflow

**Files:** Create `.github/workflows/deploy-fe.yml`; delete `.github/workflows/build_fe_docker_image.yaml`.

**Interfaces:**
- Consumes: `WIF_PROVIDER`/`WIF_SERVICE_ACCOUNT` secrets (Task 7), Hosting site (Task 9). Needs the real `NEXT_PUBLIC_FIREBASE_*` values from Task 3 (Firebase Console → Project Settings → General → your app's config, or `firebase apps:sdkconfig` once a web app is registered) and the real Cloud Run URL from Task 10's first deploy.

- [ ] **Step 1: Create `deploy-fe.yml`**

Create `.github/workflows/deploy-fe.yml` (replace the placeholder
`NEXT_PUBLIC_FIREBASE_*`/`NEXT_PUBLIC_API_URL` values with the real ones once
known — see Task 14):

```yaml
name: Deploy Frontend

on:
  push:
    branches: [main]
    paths:
      - 'fe/**'
      - 'firebase.json'
      - '.firebaserc'
      - '.github/workflows/deploy-fe.yml'
  workflow_dispatch:

jobs:
  deploy:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      id-token: write

    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: npm
          cache-dependency-path: fe/package-lock.json

      - name: Install dependencies
        working-directory: fe
        run: npm ci

      - name: Lint
        working-directory: fe
        run: npm run lint

      - name: Type check
        working-directory: fe
        run: npx tsc --noEmit

      - name: Test
        working-directory: fe
        run: npm test

      - name: Build
        working-directory: fe
        env:
          # Firebase web config is public-facing by design — security is
          # enforced by Firebase Security Rules / backend token verification,
          # not by keeping these values secret. Matches corgi's frontend.yml.
          NEXT_PUBLIC_FIREBASE_API_KEY: REPLACE_WITH_REAL_VALUE
          NEXT_PUBLIC_FIREBASE_AUTH_DOMAIN: eagle-473ac1.firebaseapp.com
          NEXT_PUBLIC_FIREBASE_PROJECT_ID: eagle-473ac1
          NEXT_PUBLIC_API_URL: https://REPLACE_WITH_REAL_CLOUD_RUN_URL
        run: npm run build

      - name: Authenticate to Google Cloud
        uses: google-github-actions/auth@v2
        with:
          project_id: eagle-473ac1
          workload_identity_provider: ${{ secrets.WIF_PROVIDER }}
          service_account: ${{ secrets.WIF_SERVICE_ACCOUNT }}
          create_credentials_file: true
          export_environment_variables: true

      - name: Deploy to Firebase Hosting
        run: npx firebase-tools@15.22.0 deploy --only hosting --project eagle-473ac1 --non-interactive
```

- [ ] **Step 2: Delete the old workflow**

```bash
git rm .github/workflows/build_fe_docker_image.yaml
```

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/deploy-fe.yml
git commit -m "ci: add deploy-fe.yml (WIF, Firebase Hosting); retire GHCR workflow"
```

---

### Task 12: `ci.yml` — PR-triggered checks

**Files:** Create `.github/workflows/ci.yml`.

**Interfaces:**
- Consumes: nothing (no cloud auth needed — pure lint/vet/test).

- [ ] **Step 1: Create `ci.yml`**

Create `.github/workflows/ci.yml`:

```yaml
name: CI

on:
  pull_request:

jobs:
  api:
    name: API
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'

      - name: Vet
        working-directory: api
        run: go vet ./...

      - name: Test
        working-directory: api
        run: go test ./...

  frontend:
    name: Frontend
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: npm
          cache-dependency-path: fe/package-lock.json

      - name: Install dependencies
        working-directory: fe
        run: npm ci

      - name: Lint
        working-directory: fe
        run: npm run lint

      - name: Type check
        working-directory: fe
        run: npx tsc --noEmit

      - name: Test
        working-directory: fe
        run: npm test
```

Note: the `api` job's tests are unit tests only (no `FIRESTORE_EMULATOR_HOST`
set), matching how the backend plan's tests already skip emulator-only cases
gracefully when the env var is absent — no emulator setup needed in CI for
this to pass.

- [ ] **Step 2: Verify locally**

Run the same commands locally to confirm they'd pass in CI:
```bash
cd api && go vet ./... && go test ./...
cd ../fe && npm run lint && npx tsc --noEmit && npm test
```
Expected: all pass (already verified individually in the backend/frontend plans).

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add PR-triggered checks (go vet/test, next lint/tsc/test) — matches corgi's ci.yml"
```

---

### Task 13: Export and seed production master data

**Resources:** `docs/sentences_export.ndjson` (not committed — real data), production Firestore `sentences` collection.

**Interfaces:**
- Consumes: the live home-network MySQL `sentences` table, or the `docs/dmls/dml_master_*.sql` fallback (per the backend plan's "Master data migration" design); `api/cmd/seed` (already built).

- [ ] **Step 1: Export the live `sentences` table**

**This sandboxed environment cannot reach `192.168.1.101` (a private home LAN
IP)** — this step likely needs to run on a machine with access to your home
network (your own laptop, or a machine on that LAN), not from here. The exact
command (per the backend plan's design):

```bash
mysql --batch --raw -N -h 192.168.1.101 -u "$DB_USER" -p "$DB_NAME" -e \
  'SELECT JSON_OBJECT("id", id, "japanese", japanese, "english", english,
                      "page", page, "is_reported", is_reported) FROM sentences' \
  > docs/sentences_export.ndjson
```

If the live DB is unreachable or decommissioned, use the fallback: parse
`docs/dmls/dml_master_1.sql`, `dml_master_2.sql`, `dml_master_3.sql`
(790 total `INSERT INTO sentences` statements) and assign sequential IDs
`1..790` — a small one-off script, not part of the committed codebase.

- [ ] **Step 2: [CONFIRM] Run the seeder against production**

```bash
export GOOGLE_CLOUD_PROJECT=eagle-473ac1
cd api && go run ./cmd/seed -file ../docs/sentences_export.ndjson
```
Requires Application Default Credentials for a principal with Firestore write
access — run as `hideee.0202@gmail.com` (already the active `gcloud auth`
account) via `gcloud auth application-default login` if ADC isn't already set
up for this account.
Expected: `seeded N sentences` where N is ~790 (or the count from the actual
export).

- [ ] **Step 3: Verify**

```bash
gcloud firestore documents list projects/eagle-473ac1/databases/'(default)'/documents/sentences \
  --project=eagle-473ac1 2>&1 | grep -c "name:"
```
Expected: a count matching the seeded total (approximate check — Firestore's
list API paginates, so this may need `--limit`/repeated calls for an exact
count; a rough non-zero confirmation is enough here).

- [ ] **Step 4: Do not commit** `docs/sentences_export.ndjson` — it's a
one-off data export, not source code. Delete it locally once seeding succeeds.

---

### Task 14: First real deploy and end-to-end manual verification

This is the checkpoint both the backend plan ("Manual verification before
cutover") and frontend plan ("Full interactive sign-in cannot be verified
until the infra plan provisions a real Firebase project") deferred to here.

**Interfaces:**
- Consumes: everything above.

- [ ] **Step 1: [CONFIRM] Trigger the first API deploy**

```bash
gh workflow run deploy-api.yml --repo hokita/eagle --ref gcp-migration
```
Or push/merge to `main` if that's the intended trigger — confirm with the
user which they want for this first deploy, since `deploy-api.yml` as
written only triggers on push to `main`, and this branch (`gcp-migration`)
isn't there yet.

- [ ] **Step 2: Get the real Cloud Run URL**

```bash
gcloud run services describe eagle-api --project=eagle-473ac1 \
  --region=asia-northeast1 --format="value(status.url)"
```

- [ ] **Step 3: Update `deploy-fe.yml` with real values**

Replace the placeholders from Task 11 with:
- `NEXT_PUBLIC_API_URL`: the URL from Step 2.
- `NEXT_PUBLIC_FIREBASE_API_KEY`: from Firebase Console → Project Settings →
  General → Web apps (register a web app here if none exists yet) → SDK
  setup and configuration.

Commit:
```bash
git add .github/workflows/deploy-fe.yml
git commit -m "ci(deploy-fe): fill in real Cloud Run URL and Firebase web config"
```

- [ ] **Step 4: [CONFIRM] Trigger the first frontend deploy**

```bash
gh workflow run deploy-fe.yml --repo hokita/eagle --ref gcp-migration
```

- [ ] **Step 5: Manual end-to-end verification**

Open the Hosting URL (`https://eagle-473ac1.web.app`) in a browser:
1. Confirm the `LoginScreen` renders (not a crash/blank page).
2. Sign in with Google (`hideee.0202@gmail.com`).
3. Confirm `Translator` renders with a real sentence.
4. Submit a translation, confirm correct/incorrect feedback and history work.
5. Click "Next Sentence", confirm a new sentence loads.
6. Click "Report" on a sentence, confirm it's excluded from future draws.
7. Open `UserMenu`, click "Sign out", confirm it returns to `LoginScreen`.
8. Try signing in with a *different* Google account (if available) — confirm
   it's rejected (401), proving the `ALLOWED_EMAIL` allowlist works in
   production, not just in tests.

- [ ] **Step 6: Update the OAuth consent screen / authorized domains if needed**

If sign-in fails with an "unauthorized domain" error, return to Task 3 Step 2
and add the actual Hosting domain shown in the browser's address bar.

---

## Verification (whole plan)

- [ ] All `[CONFIRM]` steps were run only after explicit user go-ahead.
- [ ] `gcloud services list --enabled --project=eagle-473ac1` shows all 7 required APIs.
- [ ] `gh secret list --repo hokita/eagle` shows `WIF_PROVIDER` and `WIF_SERVICE_ACCOUNT`.
- [ ] `.github/workflows/` contains exactly `ci.yml`, `deploy-api.yml`, `deploy-fe.yml` — no leftover `build_*_docker_image.yaml`.
- [ ] The full manual verification checklist in Task 14 Step 5 passes.
- [ ] `git log --oneline main..gcp-migration` shows a clean, reviewable history for the whole migration across all three plans.

## Notes

- This plan intentionally does not attempt to script the two manual Firebase
  Console steps (Task 3) — they're small, one-time, and the CLI genuinely
  doesn't support them.
- After this plan lands and Task 14 passes, use the
  `superpowers:finishing-a-development-branch` skill to decide how to merge
  `gcp-migration` into `main` (the point at which `deploy-api.yml`/
  `deploy-fe.yml` start triggering automatically on every future push).
