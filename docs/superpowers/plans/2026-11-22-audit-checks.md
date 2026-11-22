# eagle audit checks Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** eagle can't merge a PR that ships a known high/critical vulnerability, and anything that turns vulnerable after shipping turns the nightly red within a day.

**Architecture:** New jobs in `.github/workflows/ci.yml` (PR gate), a new `.github/workflows/audit-nightly.yml` (schedule + dispatch on `main`, WIF auth like the deploys), `.github/dependabot.yml`, every `uses:` pinned to a SHA, and both Dockerfile bases pinned by digest. Today's findings get fixed first, in two fix PRs.

**Tech Stack:** GitHub Actions, npm audit, govulncheck, Trivy (Docker image), gcloud, Dependabot.

**Spec:** `~/ghq/github.com/hokita/orch/docs/superpowers/specs/2026-11-22-audit-checks-design.md` (eagle rows of §5, §6, §7, §8, §9)

## Global Constraints

- High+ means severity HIGH or CRITICAL. No moderate/low gating.
- No `|| true`, `continue-on-error`, `--ignore-unfixed`, audit-level tricks, or dropping a dir from a scan.
- No code comments (YAML included), no lint/type ignores. SHA pins have no trailing `# vX`.
- Trivy runs from `aquasec/trivy:0.76.2@sha256:8f6bb947dbc528902e11f2f5be52f7b519f6a39d5d148f41166f2040499768fb` (published 2026-11-05, after GHSA-69fq-xp46-6x23, which hit only 0.69.4). Never `trivy-action`/`setup-trivy`.
- govulncheck: `go install golang.org/x/vuln/cmd/govulncheck@v1.9.0` on the runner only (nothing installed on the Mac).
- Nightly: `permissions: contents: read, id-token: write`, the same `WIF_PROVIDER`/`WIF_SERVICE_ACCOUNT` secrets, no new secrets or identifiers (service `eagle-api`, region `asia-northeast1`, project are already in `deploy-api.yml`).
- Exceptions (`.github/audit-exceptions.yml`) are built only if the user approves an entry. Never add one yourself.
- Merging to `main` deploys (`api/**` → Cloud Run, `fe/**` → Hosting). Workers never merge or comment; the orchestrator merges and watches the deploy.

## Facts found while planning (2026-11-22, origin/main `da34eff`)

- `fe` prod: 4 high, all the `@grpc/grpc-js@1.9.16` chain (GHSA-m9gg-hp2v-232j, GHSA-f596-whhp-79r4). `firebase@13.4.0` still pins `~1.9.0`.
- `fe` dev: 1 critical (`tar` 7.4.3), 7 high (undici 7.28.0, vite 8.1.4, flatted, js-yaml, minimatch, picomatch 2.3.1 via `eslint-config-next`, brace-expansion). All are transitive with non-major fixes.
- Trial on a lockfile copy: override `@grpc/grpc-js` 1.14.7 + `npm audit fix` + `npm update picomatch` → **0 high/critical** (prod and dev), 6 moderate left (typescript-eslint/minimatch chain, out of scope). `npm ls @grpc/grpc-js` shows only `1.14.7 overridden`.
- `e2e`: 0 findings.
- Go (OSV, no reachability): grpc 1.82.0, go-jose 4.1.4 high; x/net 0.56.0, x/crypto 0.53.0, x/text 0.38.0 unrated; otel 1.44.0 moderate. Latest: grpc 1.85.0, go-jose 4.1.9, x/net 0.63.0, x/crypto 0.59.0, x/text 0.44.0, otel 1.47.0. These raise `go` in `api/go.mod` from 1.25.0 to 1.26.0. The toolchain stays at 1.27.4 everywhere.
- Images: `golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280`, `alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf`.
- GitHub: Dependabot alerts off (404), security updates off, secret scanning + push protection on. Ruleset "Protect main" id `19733348` has no required checks. Existing PR check names: `API`, `Frontend`, `E2E`.

### Draft PR #46 findings (rollout step 1)

- Trivy on the built image: alpine 3.24.4 OS packages 0 high+; Go binary stdlib 0; 9 HIGH in Go modules, all with fixes: go-jose 4.1.6, x/crypto 0.55.0, x/net 0.62.0, x/text 0.41.0, grpc 1.83.2.
- govulncheck v1.9.0 (once it was built with the module's toolchain via `GOTOOLCHAIN="$(go env GOVERSION)"`; built with go1.26 it can't load the 1.27.4 stdlib): 9 called vulnerabilities, all with fixes. x/net GO-2026-6617/6612/6611/6610/6603 (0.60.0), otel/sdk GO-2026-6505 (1.45.0), grpc GO-2026-6348 (1.83.1) and GO-2026-6061 (1.82.1), x/text GO-2026-5970 (0.39.0). Plus 9 imported or required but not called.
- No finding without a fixed version. Task 1's target versions cover all of them.

## Review Focus

1. A Dependabot docker PR bumping `alpine`/`golang` must keep the digest pin. Expect it to update both the tag and the digest, never fall back to an unpinned tag.
2. A fork PR or Dependabot PR has no WIF secrets. PR-gate jobs must not need GCP auth (only the nightly does).
3. Nightly against a deploy that has a new revision: the deployed digest is read at run time, never `:latest`.
4. A workflow with `uses: ./local-action` or `docker://image@sha256:…` must pass the pin check. `@v4`, `@main` or a short SHA must fail.
5. govulncheck exit 3 (a reachable vulnerability) must fail the job. A network failure fetching the vuln DB fails it too, and that's fine.

---

## PR 0: draft gates PR (rollout step 1, find unknowns)

### Task 0: open the gates branch as a draft PR with the PR-gate jobs only

**Files:** Modify `.github/workflows/ci.yml` (add the jobs from Task 3, steps 1–4 only; no pins yet).

- [ ] Push branch `chore/audit-gates` from origin/main with the four new jobs. Open it as a **draft** PR (`gh pr create --draft`, no comments).
- [ ] Wait for CI (`gh run watch --exit-status` in the background). Record every finding from `Audit: Go (govulncheck)` and `Audit: API image` (OS packages, the Go binary's stdlib/modules) under "Facts" in this plan, and commit to `chore/audit-checks`.
- [ ] If any finding has no fixed version: stop and report it to the orchestrator for the user, one advisory per entry. Don't continue to PR 1 for it.

## PR 1: `fix/api-vulns` (rollout step 2, deploys the API)

### Task 1: bump the Go modules and pin the API image bases

**Files:** Modify `api/go.mod`, `api/go.sum`, `api/Dockerfile:1,11`.

No behaviour change, so no new test. The existing `go test ./...` and e2e are the proof.

- [ ] In `api/`: `go get google.golang.org/grpc@v1.85.0 github.com/go-jose/go-jose/v4@v4.1.9 golang.org/x/net@v0.63.0 golang.org/x/crypto@v0.59.0 golang.org/x/text@v0.44.0 go.opentelemetry.io/otel@v1.47.0 go.opentelemetry.io/otel/sdk@v1.47.0 go.opentelemetry.io/otel/trace@v1.47.0 && go mod tidy`. Add any module the Task 0 govulncheck run flagged. Keep `toolchain go1.27.4`.
- [ ] Dockerfile: `FROM golang:1.27.4-alpine@sha256:70bd…b280 AS builder` and `FROM alpine:3.24.4@sha256:f59a…71bf` (full digests from Facts).
- [ ] Verify: `go vet ./... && go test ./...` pass. `docker build -t eagle-api:local api` succeeds. The e2e suite (`cd e2e && npm ci && npm run test:e2e`, using the local stack from memory `project_local_dev_stack`) passes. `go list -m google.golang.org/grpc github.com/go-jose/go-jose/v4` prints the new versions.
- [ ] Commit `fix(api): bump grpc, go-jose, x/net, x/crypto, x/text, otel; pin image bases by digest`. Open the PR. After the orchestrator merges, the `Deploy API` run must be green and `gcloud run services describe eagle-api` must show a new revision.

## PR 2: `fix/fe-vulns` (rollout steps 2 + 2b, deploys Hosting)

### Task 2: grpc-js override + dev-tree fixes in `fe`

**Files:** Modify `fe/package.json` (`overrides`), `fe/package-lock.json`.

- [ ] Red: `cd fe && npm audit --omit=dev --audit-level=high` exits 1 (4 high), and `npm audit --audit-level=high` exits 1 (1 critical, 11 high).
- [ ] Add `"@grpc/grpc-js": "1.14.7"` to `overrides`, next to the existing `next.postcss`. Then `npm install`, `npm audit fix` (never `--force`), `npm update picomatch`.
- [ ] Green: both audit commands exit 0. `npm ls @grpc/grpc-js` shows only `1.14.7 overridden`.
- [ ] Verify: `npm run lint`, `npx tsc --noEmit`, `npm test`, and `npm run build` (with the `NEXT_PUBLIC_*` values from `deploy-fe.yml`) all pass. The full e2e suite passes. `cd e2e && npm audit --audit-level=high` exits 0.
- [ ] Proof note for the PR description (spec §6): whether any vitest test loads the real `@firebase/firestore` Node entry or mocks it (`grep -rn "vi.mock('firebase" fe/src`), and `grep -rl "grpc" fe/out` on the build output. State plainly if no test exercises grpc-js.
- [ ] If the override breaks a suite or the build, try `1.13.7`. If that breaks too, stop and report to the orchestrator (exception path, user approves). Don't build the exception file.
- [ ] Commit `fix(fe): override @grpc/grpc-js to 1.14.7 and update vulnerable dev deps`. Deploy-fe must be green after the merge.

## PR 3: `chore/audit-gates` (rollout step 3; rebase PR 0 onto main after PRs 1–2)

### Task 3: PR-gate jobs in `ci.yml`

**Files:** Modify `.github/workflows/ci.yml`.

Job `name`s, which become the required checks:
1. `Audit: fe prod deps`: setup-node 24.22.3, `npm audit --omit=dev --audit-level=high` in `fe` (no install needed).
2. `Audit: Go (govulncheck)`: setup-go (`go-version-file: api/go.mod`), install govulncheck v1.9.0, `govulncheck ./...` in `api`.
3. `Audit: API image`: `docker build -t eagle-api:ci api`, then `docker run --rm -v /var/run/docker.sock:/var/run/docker.sock <trivy pin> image --scanners vuln --severity HIGH,CRITICAL --exit-code 1 eagle-api:ci`.
4. `Audit: Actions pins`: `bash .github/scripts/check-action-pins.test.sh && bash .github/scripts/check-action-pins.sh .github/workflows` (Task 4).

- [ ] After the rebase, all four jobs are green on the PR.

### Task 4: action-pin checker (TDD)

**Files:** Create `.github/scripts/check-action-pins.sh`, `.github/scripts/check-action-pins.test.sh`, fixtures `.github/scripts/fixtures/pins/{good,bad}/*.yml`.

**Interface:** `check-action-pins.sh <dir>` exits 0 when every `uses:` in `<dir>/*.yml` is `owner/repo[/path]@<40 hex>`, `./…`, or `docker://…@sha256:<64 hex>`. Otherwise it exits 1 and prints `file:line: <uses value>` for each offender.

- [ ] Red: the test script asserts `good/` → exit 0. `bad/` (`@v4`, `@main`, a 7-char SHA, `docker://alpine:3`) → exit 1, with exactly 4 offender lines. It fails because the checker doesn't exist yet.
- [ ] Green: implement with `grep -nE` in plain bash. The test passes.
- [ ] Run against `.github/workflows` and expect it to fail (unpinned today). That's the input to Task 5.

### Task 5: pin every action by SHA, unify auth on v3

**Files:** Modify all 4 existing workflows and the new `audit-nightly.yml`.

- [ ] For each `uses: owner/repo@vN`: resolve the latest tag in that major with `gh api repos/<owner>/<repo>/git/ref/tags/<tag> --jq .object.sha` (dereference annotated tags via `git/tags/<sha>`) and replace it with the bare 40-char SHA. Move `google-github-actions/auth@v2` to the latest v3 SHA in `deploy-api.yml` and `deploy-fe.yml`.
- [ ] `check-action-pins.sh .github/workflows` exits 0.
- [ ] Merging this edits both deploy workflows, so their path filters trigger both deploys. The orchestrator watches them.

### Task 6: `audit-nightly.yml`

**Files:** Create `.github/workflows/audit-nightly.yml`. `on: schedule: cron '10 18 * * *'` + `workflow_dispatch`. Top-level `permissions: contents: read, id-token: write`. Separate jobs:

- `Nightly: fe prod deps`, `Nightly: fe all deps` (`npm audit --audit-level=high` in `fe`), `Nightly: e2e all deps` (same in `e2e`).
- `Nightly: Go (govulncheck)`: `govulncheck ./...`, then `govulncheck -show verbose ./...` for imported-but-not-called modules in the log. Only the first one gates.
- `Nightly: deployed API image`: auth (WIF, v3 pin) → setup-gcloud → `gcloud auth configure-docker asia-northeast1-docker.pkg.dev --quiet` → `REV=$(gcloud run services describe eagle-api --region asia-northeast1 --project <same as deploy-api.yml> --format='value(status.latestReadyRevisionName)')` → `DIGEST=$(gcloud run revisions describe "$REV" … --format='value(status.imageDigest)')` → `docker pull "$DIGEST"` → Trivy as in Task 3 on `$DIGEST`. Trivy's gobinary scan covers the deployed stdlib/modules. No Node runtime check (eagle has no Node in production).
- `Nightly: Actions pins`: same as Task 3.4.

- [ ] Verify on the branch: `gh workflow run audit-nightly.yml --ref chore/audit-gates`. The WIF provider checks only the repository, so a branch dispatch can authenticate. All jobs green, and the deployed digest shows in the log.

### Task 7: `.github/dependabot.yml`

- [ ] `version: 2`. `gomod` `directories: ["/api"]` and `npm` `directories: ["/fe", "/e2e"]` with `open-pull-requests-limit: 0`. `docker` `directories: ["/api"]` and `github-actions` `directories: ["/"]` weekly, each with one group `patterns: ["*"]`.
- [ ] Verify after the merge: `gh run list --event dynamic --limit 10` shows the Dependabot update runs for docker and github-actions as successful (an invalid config makes them fail).

### Task 8: show the gate going red once, then merge

- [ ] On a throwaway branch `demo/audit-red` from `chore/audit-gates`: add `"lodash": "4.17.20"` to `fe` dependencies (GHSA-35jh-r3h4-6jhm, high) with a lockfile update. Open it as a draft PR. `Audit: fe prod deps` must fail. Record the run URL in PR 3's description, then close the PR and delete the branch (`gh pr close --delete-branch`). Never merge it.
- [ ] PR 3 ready for review. All checks green: `API`, `Frontend`, `E2E` + the four `Audit:` jobs. After the orchestrator merges: both deploys green, and the first `gh workflow run audit-nightly.yml` on `main` green.

## GitHub settings (orchestrator, via `gh api`)

| When | Setting | Command | Read-back |
|---|---|---|---|
| Rollout step 0, before PR 0 | Dependabot alerts + dependency graph | `gh api -X PUT repos/hokita/eagle/vulnerability-alerts` | `gh api repos/hokita/eagle/vulnerability-alerts` → 204 |
| After PR 3 merges (step 4) | Dependabot security updates | `gh api -X PUT repos/hokita/eagle/automated-security-fixes` | `.enabled == true` |
| After PR 3 merges (step 4) | Required checks on ruleset `19733348` "Protect main" | `gh api -X PUT repos/hokita/eagle/rulesets/19733348` with the existing 3 rules plus `{"type":"required_status_checks","parameters":{"strict_required_status_checks_policy":false,"required_status_checks":[{"context":"API"},{"context":"Frontend"},{"context":"E2E"},{"context":"Audit: fe prod deps"},{"context":"Audit: Go (govulncheck)"},{"context":"Audit: API image"},{"context":"Audit: Actions pins"}]}}` | rules list shows them; a test PR lists them as "Required" |
| — | Secret scanning | already on with push protection; no change | — |

## Excluded (eagle)

Moderate/low (the 6 typescript-eslint/minimatch moderates, otel moderates, if anything is left after Task 1), `npx`/`npm install -g firebase-tools@15.22.0` in workflows, Playwright browser versions, Hosting/Firestore rules, CodeQL/SAST, Node 26, the original checkout at `~/ghq/github.com/hokita/eagle`.
