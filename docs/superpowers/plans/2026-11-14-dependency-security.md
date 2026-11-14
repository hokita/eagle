# fe Dependency and Node Runtime Security Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move every Node.js runtime eagle uses (CI, deploy build, e2e, local) from EOL Node 20 to **24.22.3** (the 2026-11-11 security release), and clear the production-scope `npm audit --omit=dev` findings in `fe/` that a patch/minor update can fix. The static export must behave the same.

**Architecture:** eagle has **no Node in production**. `api/` is Go (`api/Dockerfile`: `golang:1.25.0-alpine` → `alpine:latest`, `CMD ["./main"]`), and `fe/` is Next.js `output: "export"` served as static files from `fe/out` by Firebase Hosting. So Node only runs where the static bundle is built and tested: GitHub Actions (`ci.yml`, `e2e.yml`, `deploy-fe.yml`, all `node-version: '20'`) and the local Mac. The Node work pins those to 24.22.3. The library work bumps `next` to the latest 15.5 patch, refreshes `sharp`/`nanoid`/`source-map-js` within range, and adds one `overrides` entry for Next's hard-pinned nested `postcss`. Node goes first so the library bump is installed and verified on the target runtime.

**Tech Stack:** Node.js 24.22.3 / npm 11, Volta 2.0.2 (local), Next.js 15.5, Vitest, Playwright e2e (`e2e/`), GitHub Actions `actions/setup-node@v4`.

**Spec:**
- Library audit (eagle/fe sections): `/private/tmp/claude-501/-Users-hokita-ghq-github-com-hokita-orch/f7be1767-ea7c-4ee6-b96d-13a1a4575d88/scratchpad/node-audit.md` (finding #7) and `.../node-audit-libs.md` (`## eagle/fe`).
- Node runtime: `.../node-runtime.md` (§1 release facts, §2 eagle rows, §3 eagle CI row, §4 eagle recommendation).

## Global Constraints

- Node target: **exactly `24.22.3`** everywhere it's pinned. Node 20 is EOL and will never get a fix. Verified 2026-11-14: `nodejs.org/dist/index.json` lists v24.22.3 (2026-11-10, `security: true`, the latest v24), and `actions/node-versions` `versions-manifest.json` includes `24.22.3`, so setup-node can install it.
- No production Node service exists in eagle. Don't add one, and don't touch `api/Dockerfile` (Go). `deploy-api.yml` has no Node step.
- Libraries: only production deps of `fe/`, and only patch/minor updates. Prefer refreshing the lockfile within ranges, then a parent bump, and an `overrides` entry only as a last resort. The one dev dependency that changes is `@types/node` (`^20` → `^24`), because the user asked for it to match the runtime. Other dev-only tooling stays out of scope.
- `next` stays an exact pin. The target is **15.5.29** (latest 15.5.x, npm `backport` tag, published 2026-11-03). Its `engines.node` is `^18.18.0 || ^19.8.0 || >= 20.0.0`, so Node 24 is supported.
- Don't run `npm audit fix --force`, and ignore npm's suggested `firebase@9.14.0` (a downgrade).

## Current state (re-verified 2026-11-14 after `git fetch`)

Worktree `HEAD` == `origin/main` == `74118d6`.

**Node runtime**

| Where | Current | Target | How |
|---|---|---|---|
| `.github/workflows/ci.yml:33` (Frontend job) | `node-version: '20'` | `'24.22.3'` | edit |
| `.github/workflows/e2e.yml:22` (also runs global `firebase-tools@15.22.0` and `serve`) | `'20'` | `'24.22.3'` | edit |
| `.github/workflows/deploy-fe.yml:25` (builds `fe/out` for prod) | `'20'` | `'24.22.3'` | edit |
| `fe/package.json` volta / engines | none | `"volta": {"node": "24.22.3"}`, `"engines": {"node": ">=24.22.3 <25"}` | edit |
| `e2e/package.json` volta | none | `"volta": {"node": "24.22.3"}` | edit (so local e2e runs on the same Node as CI) |
| `fe` `@types/node` (dev) | `^20` (locked 20.19.1) | `^24` (24.19.2 today) | `npm install -D @types/node@^24` |
| Local Mac (Volta default) | 24.16.0 (unfixed) | 24.22.3 | `volta install node@24.22.3`. Pinned dirs then fetch it automatically. |
| Production (eagle-api Cloud Run, Firebase Hosting) | no Node | no Node | verify only (Task 4) |

There's no `.nvmrc`, `.node-version` or `.tool-versions` in the repo.

**Production libraries** (from `fe/package-lock.json`)

| Library | Installed | Target | Via | How |
|---|---|---|---|---|
| next | 15.3.4 | **15.5.29** | direct | parent bump: `npm install --save-exact next@15.5.29` (minor) |
| @next/env, @next/swc-* | 15.3.4 | 15.5.29 | next (exact pins) | follows the next bump |
| sharp (optional) | 0.34.2 | **0.35.5** | next (`^0.34.3 \|\| ^0.35.5` in 15.5.29) | lockfile refresh: `npm update sharp` after the bump. 0.35.5 is the minimum fixed version; engines `>=20.9.0`, so Node 24 is fine. |
| postcss (nested, `node_modules/next/node_modules/postcss`) | 8.4.31 | **8.5.33** (≥8.5.23) | next (hard pin `"postcss": "8.4.31"`, still in 15.5.29) | **override** in `fe/package.json`: `"overrides": { "next": { "postcss": "^8.5.23" } }` |
| nanoid | 3.3.16 | **3.3.20** (≥3.3.18) | next > postcss | lockfile refresh (postcss 8.5.33 requires `^3.3.20`). Otherwise `npm update nanoid` |
| source-map-js | 1.2.1 | **1.2.2** | next > postcss | lockfile refresh (postcss requires `^1.2.2`). Otherwise `npm update source-map-js` |

Root `node_modules/postcss` 8.5.18 is dev-only (tailwind). It's out of scope, but it may dedupe to 8.5.33, which is acceptable.

## Review Focus

1. **The static export changes under Node 24 + Next 15.5**: same route set in `fe/out`, every page still `.html`, and the SPA fallback still works under `serve -s`. The baseline is captured on the *old* stack (Node 20 or the current 24.16 + next 15.3.4) and compared to the new one (Task 3).
2. **The CI matrix still runs Node 20 somewhere**: the post-change grep must find no `node-version` other than `24.22.3`. Check this in the actual run logs, not just the YAML (Task 4).
3. **`@types/node` 24 type changes** (e.g. `Buffer`/`fetch`/timer typings) break `tsc --noEmit`. Task 1 runs tsc on its own so the failure is attributed to the right change.
4. **`next lint` deprecation in 15.5**: it may print a warning but must exit 0. If it fails, stop and report. Don't migrate the ESLint CLI here.
5. **The postcss override breaks Tailwind v4 CSS, or sharp 0.35 fails `npm ci` on ubuntu/Node 24**: e2e plus a visual check cover the first, and green `ci.yml` covers the second.

---

### Task 1: Pin Node 24.22.3 (CI, deploy build, e2e, local) and @types/node ^24

**Files:**
- Modify: `.github/workflows/ci.yml:33`, `.github/workflows/e2e.yml:22`, `.github/workflows/deploy-fe.yml:25`
- Modify: `fe/package.json` (`volta`, `engines`, `devDependencies.@types/node`), `fe/package-lock.json`
- Modify: `e2e/package.json` (`volta`)

- [ ] **Step 1: Record the baseline (red)**

Run from the repo root: `grep -rn "node-version" .github/workflows` → three `'20'` hits. `cd fe && node -v` → v24.16.0 (Volta default, unfixed).
Capture the baseline export **before any change**, using the e2e env values from `e2e/playwright.config.ts`:
`npm ci && NEXT_PUBLIC_API_URL=http://localhost:8080 NEXT_PUBLIC_FIREBASE_API_KEY=x NEXT_PUBLIC_FIREBASE_AUTH_DOMAIN=x NEXT_PUBLIC_FIREBASE_PROJECT_ID=x npm run build && (cd out && find . -type f -not -path './_next/*' | sort) > <scratchpad>/eagle-out-before.txt`
Also run `npm audit --omit=dev > <scratchpad>/eagle-audit-before.txt`.

- [ ] **Step 2: Install Node 24.22.3 locally**

Run: `volta install node@24.22.3`
Expected: `volta list` shows the default `node@24.22.3`.

- [ ] **Step 3: Edit the three workflows**

Set `node-version: '24.22.3'` in `ci.yml`, `e2e.yml` and `deploy-fe.yml`. Use the exact version, not `'24'`, so a cached older 24.x can't slip in.
Check: `grep -rn "node-version" .github/workflows` → exactly three hits, all `'24.22.3'`.

- [ ] **Step 4: Pin Node in fe and e2e**

Add `"volta": { "node": "24.22.3" }` and `"engines": { "node": ">=24.22.3 <25" }` to `fe/package.json`, and `"volta": { "node": "24.22.3" }` to `e2e/package.json`. `engines` is advisory for npm (no `engine-strict`). It documents the floor.
Check: `cd fe && node -v` → `v24.22.3`, and `cd e2e && node -v` → `v24.22.3`.

- [ ] **Step 5: Bump @types/node**

Run in `fe/`: `npm install -D @types/node@^24`
Expected: `package.json` has `"@types/node": "^24"` (keep the repo's bare-major style). The lockfile resolves 24.19.x.

- [ ] **Step 6: Verify on Node 24.22.3 (green)**

Run in `fe/`: `npm ci && npm run lint && npx tsc --noEmit && npm test && <same build as Step 1>`
Expected: all exit 0. The lockfile diff only touches `@types/node` (and `undici-types`, which it pulls in). Check with `git diff fe/package-lock.json | grep '^[-+]  *"version"'`.

- [ ] **Step 7: Commit**

```bash
git add .github/workflows/ci.yml .github/workflows/e2e.yml .github/workflows/deploy-fe.yml fe/package.json fe/package-lock.json e2e/package.json
git commit -m "Move fe CI, e2e and deploy build from EOL Node 20 to 24.22.3"
```

### Task 2: Bump next and refresh the vulnerable prod transitives (on Node 24.22.3)

**Files:**
- Modify: `fe/package.json` (`dependencies.next`, new top-level `overrides`), `fe/package-lock.json`

**Interfaces:** Consumes Task 1's Volta pin. Confirm `node -v` = `v24.22.3` in `fe/` before you start.

- [ ] **Step 1: Bump next (parent bump)**

Run: `npm install --save-exact next@15.5.29`
Expected: `"next": "15.5.29"`. `eslint-config-next` (dev) stays at 15.3.4.

- [ ] **Step 2: Override Next's pinned postcss**

Add `"overrides": { "next": { "postcss": "^8.5.23" } }` to `fe/package.json`, then run `npm install`.
Expected: `npm ls postcss` shows `next@15.5.29 └── postcss@8.5.33 overridden`.

- [ ] **Step 3: Refresh the remaining transitives within range**

Run: `npm update sharp nanoid source-map-js`
Expected: `npm ls sharp nanoid source-map-js` shows sharp 0.35.5, nanoid 3.3.20 and source-map-js 1.2.2.

- [ ] **Step 4: Re-run the prod audit (green)**

Run: `npm audit --omit=dev`
Expected: no `next`, `sharp`, `postcss`, `nanoid` or `source-map-js` entries. Only the known out-of-scope `firebase` → `@firebase/firestore(-compat)` → `@grpc/grpc-js` 1.9.16 chain remains.

- [ ] **Step 5: Check the scope of the lockfile diff**

Run: `git diff HEAD --stat fe/ && git diff HEAD fe/package-lock.json | grep '^[-+]  *"version"' | sort | uniq -c`
Expected: only next/@next/*, sharp/@img/*, postcss, nanoid and source-map-js move. Otherwise run `git checkout fe/package-lock.json fe/package.json` and redo Steps 1–3.

- [ ] **Step 6: Run the repo checks (same as `ci.yml` / `deploy-fe.yml`)**

Run in `fe/`: `npm ci && npm run lint && npx tsc --noEmit && npm test`
Expected: all exit 0 (a `next lint` deprecation notice is OK).

- [ ] **Step 7: Commit**

```bash
git add fe/package.json fe/package-lock.json
git commit -m "Bump next to 15.5.29 and patch its prod transitives"
```

### Task 3: Prove the static export behaves the same on Node 24.22.3 + Next 15.5.29

**Files:** none (verification only).

- [ ] **Step 1: Diff the exported page set**

Run the Task 1 Step 1 build again, writing to `<scratchpad>/eagle-out-after.txt`, then `diff` the before and after files.
Expected: no diff (same HTML routes, `404.html`, public assets; hashed `_next/` chunks are excluded). Any added or removed route means stop and investigate.

- [ ] **Step 2: Run the full e2e suite on Node 24.22.3**

Run from `e2e/`: `node -v` (→ v24.22.3), then `npm ci && npm run test:e2e`. This needs firebase-tools 15.22.0, Java 21 and Go 1.23, as in `e2e.yml`. Playwright's webServer runs `npm run build && npx serve out -s` in `fe/`.
Expected: all Playwright tests pass.

- [ ] **Step 3: Spot-check in a browser**

While the stack is up, open the home page and one practice page. The Tailwind styling should match production.

### Task 4: PR, CI and deploy confirmation

- [ ] **Step 1: Open the PR (only when the user asks to push), using the summary below.** `ci.yml` (API + Frontend) and `e2e.yml` must be green.
- [ ] **Step 2: Prove CI actually ran 24.22.3.** For both PR runs: `gh run view <id> --log | grep -E "Attempting to download 24|node-version|v24\.22\.3"`. Expected: setup-node resolved **24.22.3**, and the logs never mention 20.x.
- [ ] **Step 3: After merge, watch `deploy-fe.yml`.** It triggers on `fe/**` and on `.github/workflows/deploy-fe.yml`. Run `gh run watch <id> --exit-status` in the background until it's green, then check its log the same way as Step 2. The prod bundle was built on 24.22.3.
- [ ] **Step 4: Confirm the Hosting release.** `npx firebase-tools@15.22.0 hosting:releases:list --project eagle-473ac1` (or the Hosting console) shows a release after the merge time. Run `curl -s https://eagle-473ac1.web.app/ | grep -o '/_next/static/[^"]*' | head` and check the chunk hashes differ from the pre-merge capture. Sign in and run one practice flow.
- [ ] **Step 5: Confirm there's still no Node in production** (the only Cloud Run service is eagle-api, which this PR doesn't redeploy):
  `gcloud run revisions list --service eagle-api --region asia-northeast1 --project eagle-473ac1 --limit 1`, then `gcloud run revisions describe <rev> --region asia-northeast1 --project eagle-473ac1 --format='value(spec.containers[0].image)'`. Inspect that image's config (`gcloud artifacts docker images describe <image> --show-package-vulnerability` or `crane config <image> | jq '.config.Env, .config.Cmd'`).
  Expected: `Env` has **no `NODE_VERSION`**, and `Cmd` is `["./main"]` (Go binary). There's no runtime 24.22.3 to prove because none runs. The prod-relevant Node is the deploy-fe builder, which Step 3 proves.

## PR summary (paste into the PR body)

```markdown
## Summary
Node.js runtime (2026-11-11 Node security release, fixed in 24.22.3):
- CI (`ci.yml`), e2e (`e2e.yml`) and the production frontend build (`deploy-fe.yml`): `node-version` '20' (EOL, no fix) -> '24.22.3'
- `fe/package.json`: Volta pin `24.22.3` + `engines.node >=24.22.3 <25`; `e2e/package.json`: Volta pin `24.22.3`
- `@types/node` ^20 -> ^24
- Production has no Node: api is Go on Cloud Run, fe is a static export on Firebase Hosting. The Node change affects build/test only.

Production dependencies (`npm audit --omit=dev`, hygiene; not exploitable since no Next server runs with `output: "export"`):
- next 15.3.4 -> 15.5.29 (latest 15.5 patch)
- sharp 0.34.2 -> 0.35.5 (lockfile refresh within next's range)
- postcss (next's nested copy) 8.4.31 -> 8.5.33 via `overrides` (next 15.5 still hard-pins 8.4.31)
- nanoid 3.3.16 -> 3.3.20, source-map-js 1.2.1 -> 1.2.2 (follow postcss)

Remaining, out of scope: firebase -> @grpc/grpc-js 1.9.16 (Node-only path, not in the browser bundle; no firebase release fixes it).

## Verification
- `npm audit --omit=dev` before/after
- lint, tsc, vitest, `next build` on Node 24.22.3; exported route list identical before/after
- e2e suite green on Node 24.22.3
- CI/deploy logs show setup-node resolved 24.22.3
```

---

## Later (out of scope: major upgrades or not Node/lib)

- **next 16.x**: a major upgrade (latest 16.4.6). It isn't needed because 15.5.29 is patched. Plan it separately (`next lint` removal, other breaking changes).
- **firebase → @firebase/firestore → @grpc/grpc-js 1.9.16 (high)**: every firebase release pins `~1.9.0`. The only fix is an override across minors into a Node-only path that doesn't ship. Leave it.
- **eslint-config-next 15.3.4 → 15.5.29**: dev-only. Align it during lint tooling work.
- **`api/Dockerfile` `alpine:latest` (unpinned) and `golang:1.25.0`**: not Node. Pin and patch the Go and alpine images in a separate change.
- **Local Mac cleanup**: after Task 1, optionally remove the unused unfixed Volta images (20.20.2, 22.23.0, 24.16.0, 24.17.0). Other repos still pin 24.17.0, so do it after their own Node plans land.
