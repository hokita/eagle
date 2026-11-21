# Remove Narration Comments Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Delete every code comment in eagle except build-required directives, and remove every comment or lint ignore that props up code, with no behaviour change.

**Architecture:** Two throwaway tools (strip and verify) under the gitignored `.superpowers/comment-tools/` remove comments using the real language scanners (go/scanner, the TypeScript scanner), never regexes. One fix commit comes first, then one comments-only commit per area. Each removal commit is proven comments-only by comparing token streams before and after.

**Tech Stack:** Go 1.27 (api), Next.js 15 / TypeScript / Vitest (fe), Playwright (e2e), GitHub Actions YAML.

**Spec:** The user-approved brief of 2026-11-21 (strictest version), restated in Global Constraints.

## Global Constraints

- Delete every comment: warnings, GoDoc/JSDoc, commented-out code, test comments, JSX `{/* */}` comments, YAML and `.gitignore` `#` comments.
- Keep only directives a build needs. Inventory result: only `e2e/scripts/run.sh:1` and `e2e/scripts/inner.sh:1`, both `#!/usr/bin/env bash` (shebang the scripts need to run). There are no `//go:build`, `//go:embed`, `/// <reference`, license headers or generated-code markers in tracked source.
- Don't touch: `docs/**`, `README.md`, `fe/README.md`, `go.sum`, `fe/package-lock.json`, `e2e/package-lock.json`, `e2e/fixtures/*.ndjson` (data), `fe/public/**` (assets), and untracked generated files (`next-env.d.ts`, `.next/`).
- `fe/src/components/ui/*` started as shadcn copies but contain repo-written code (the focus trap in `sheet.tsx`), so they are in scope.
- Removal commits contain comment deletions only, plus the blank-line cleanup that follows (no more than one blank line in a row, no trailing whitespace). Fix commits must pass the full test suite.
- Add no code to replace a deleted warning. Real traps go in the PR body under "Traps that lost their comment".
- Never merge. Post no GitHub comments. Commit no secrets. Report port clashes and leave other processes running.

## Inventory (base 0880ab7)

| Area | Comments | Whole lines removed | Trailing (line stays) |
|---|---|---|---|
| api/cmd | 33 | 33 | 0 |
| api/internal/app (source) | 475 | 461 | 14 |
| api/internal/app (tests) | 251 | 240 | 11 |
| fe/src/lib + fe/src/test-helpers.ts | 67 | 67 | 0 |
| fe/src/components (source, includes 9 JSX comments) | 165 | 189 | 0 |
| fe/src/components (tests) | 90 | 90 | 0 |
| e2e/tests | 34 | 34 | 0 |
| config: `.github/workflows/deploy-{api,fe}.yml`, `fe/.gitignore` | 15 | 15 | 0 |
| **Total** | **1130** | **1129** | **25** |

The CSS, the other config files (`next.config.ts`, `vitest.config.ts`, `eslint.config.mjs`, `playwright.config.ts`, `firebase.json`, Dockerfile, `.env.example`, `ci.yml`, `e2e.yml`) and `api/main.go` contain no comments.

Lint and type ignores: **none in the code.** There is no `eslint-disable*`, `@ts-ignore`, `@ts-expect-error`, `biome-ignore` or `//nolint`. `fe/eslint.config.mjs` only extends `next/core-web-vitals` and `next/typescript` and turns no rule off. The api has no golangci config. One comment exists only to keep a block non-empty: `fe/src/lib/useSettings.ts:21`, the empty `catch` in `loadStoredLevels` (Task 2).

## Review Focus

- A JSX comment `{/* … */}` stripped without its braces leaves `{}`, which is a syntax error inside JSX. The strip tool must remove the whole expression container. The fe build and tsc catch this.
- A Go `/* */` comment that spans a newline acts as a newline for semicolon insertion. The token comparison catches any change.
- `//` or `#` inside strings, template literals and YAML block scalars (URLs in prompts, `run: |` scripts) must survive. The scanner-based strip and the YAML deep-equal catch this.
- A comment removed between two blank lines leaves a double blank line. Strip collapses it. gofmt and the token check stay clean.
- `fe/.gitignore` must ignore exactly the same paths after the change. Verify with a non-comment-line comparison.

---

### Task 1: Strip and verify tools (not committed)

**Files:**
- Create: `.superpowers/comment-tools/go/` (module with `strip` and `tokens` commands)
- Create: `.superpowers/comment-tools/ts/strip.cjs`, `.superpowers/comment-tools/ts/tokens.cjs` (load `fe/node_modules/typescript`)
- Create: `.superpowers/comment-tools/verify.sh`

**Interfaces:**
- Produces: `strip <files…>` rewrites files in place. `verify.sh <base-ref> <paths…>` exits 0 only if every changed file under the paths is comments-only relative to `<base-ref>`.

- [ ] **Step 1:** Go `strip`: delete each `token.COMMENT` span found by `go/scanner` with `ScanComments`. If the line holds nothing else, delete the whole line, otherwise delete the comment and the whitespace before it. Collapse repeated blank lines, then run `gofmt -w`.
- [ ] **Step 2:** TS `strip`: collect comment ranges with `ts.getLeadingCommentRanges`/`getTrailingCommentRanges` over every node. Remove a `JsxExpression` whose expression is empty and whose body is only a comment as a whole, braces included. Apply the same line rules as Step 1.
- [ ] **Step 3:** `tokens`: print the token stream with comments excluded and positions dropped. Go: `tok` + `lit` from `go/scanner`. TS: `ts.createScanner(…, skipTrivia=true)` kind + text, re-scanning JSX text in JSX contexts. Drop empty `{` `}` JSX containers from the base side only.
- [ ] **Step 4:** `verify.sh`: for each changed `.go`/`.ts`/`.tsx`/`.mjs`, diff `tokens(git show base:file)` against `tokens(file)`. For `.yml`, deep-equal the parsed documents with `fe/node_modules/js-yaml`. For `.gitignore`, compare the non-`#` lines. Fail loudly on any difference.
- [ ] **Step 5: Self-test:** in a scratch copy, add one non-comment change (`x := 1` → `x := 2`, and a `.tsx` attribute) and confirm `verify.sh` fails. Confirm it passes on a comments-only edit, including one JSX `{/* */}`.

### Task 2: Fix the comment-propped empty catch (fix commit)

**Files:** Modify `fe/src/lib/useSettings.ts:21-23`. Test: `fe/src/lib/useSettings.test.ts`.

- [ ] **Step 1:** Run `cd fe && npx vitest run src/lib/useSettings.test.ts`. The existing `falls back to every level when the stored value is malformed JSON` test pins the behaviour. Expected: PASS (behaviour is unchanged, so this is a refactor guarded by an existing test, not a new red test).
- [ ] **Step 2:** Replace `catch { // ignore … }` with `catch { return LEVELS }`, which keeps the same result and leaves no empty block.
- [ ] **Step 3:** Run the full check (see "Per-task verification"). Expected: all green.
- [ ] **Step 4: Commit:** `refactor(fe): return the default from loadStoredLevels' catch instead of an empty block`

### Tasks 3–10: Comments-only removal, one commit per area

Run Task 1's `strip` over each area's files, then the per-task verification, then commit `chore(<area>): remove comments`.

| Task | Area (files) |
|---|---|
| 3 | `api/cmd/**/*.go` |
| 4 | `api/internal/app/*.go` without `_test.go` |
| 5 | `api/internal/app/*_test.go` |
| 6 | `fe/src/lib/*.ts`, `fe/src/test-helpers.ts` (after Task 2) |
| 7 | `fe/src/components/**/*.tsx` without `*.test.tsx` |
| 8 | `fe/src/components/**/*.test.tsx` |
| 9 | `e2e/tests/*.ts` |
| 10 | `.github/workflows/deploy-api.yml`, `.github/workflows/deploy-fe.yml`, `fe/.gitignore`, hand-edited with the `#` lines deleted |

After each task, `git grep` the task's files for `//`, `/*` and `#` (skipping matches in strings) to confirm nothing is left. While stripping, add any comment that guarded a real trap to the PR body list (candidates are below).

### Per-task verification (every task from 2 to 10)

- `.superpowers/comment-tools/verify.sh HEAD~1 <area>` exits 0 (removal tasks only).
- api: `cd api && gofmt -l . && go vet ./... && go test ./...`. Expected: no gofmt output, vet clean, tests ok.
- fe: `cd fe && npm run lint && npx tsc --noEmit && npm test && npm run build`. Expected: all pass.
- e2e: `cd e2e && npx tsc --noEmit -p .`. Expected: no errors.

### Task 11: End-to-end run, then the PR

- [ ] **Step 1:** Check that ports 9099, 8081, 8080 and 3000 are free (`lsof -iTCP -sTCP:LISTEN -P | grep -E ':(9099|8081|8080|3000)\b'`). If any is taken, report the pid and stop.
- [ ] **Step 2:** Run `bash e2e/scripts/run.sh` from the repo root. Expected: every Playwright spec passes.
- [ ] **Step 3:** `git grep` the whole tracked tree outside the exclusions. Expected: only the two shebangs remain.
- [ ] **Step 4:** Push, then `gh pr create --title 'Remove narration comments'`. The body has: counts, verification, "Lint ignores removed" (none in code; the `useSettings.ts` empty-catch fix), "Traps that lost their comment", and it ends with `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.

## Traps that will lose their comment (PR body candidates)

- `api/internal/app/handlers.go:285,308`, `phrases.go:130`: sentence and reference answer are loaded server-side by id, never from the client (otherwise the app's Gemini key processes arbitrary text).
- `api/internal/app/cors.go:39`: `Vary: Origin` is set on rejected origins too (cache poisoning of legitimate requests).
- `api/internal/app/cors.go:8`: both `{project}.web.app` and `.firebaseapp.com` origins must be listed.
- `api/internal/app/auth.go:29`: a disallowed email gets 401, not 403, so the response doesn't reveal whether the account exists.
- `api/internal/app/gemini_explainer.go:87`: thinking level is low, not minimal, because gemini-3.8-flash rejects minimal.
- `api/internal/app/gemini_explainer.go:20`: thinking tokens count against `MaxOutputTokens`, so every cap carries `thinkingAllowanceTokens`.
- `api/internal/app/gemini_coach.go:22,42,58`: the summary token budget and deadline scale with the transcript; a fixed cap truncates long valid sessions.
- `api/internal/app/gemini_coach.go:179`: refinements are zipped with learner turns and must match one-to-one.
- `api/internal/app/practice.go:23`: a failed stats write is deliberately not returned, so grading still succeeds.
- `api/internal/app/discussion.go:11,21`, `handlers.go:39,400`, `fe/src/components/ReviewPanel.tsx:43`, `SavePhraseForm.tsx:15`: limits are in runes and mirror the frontend `maxLength`.
- `api/internal/app/discussion.go:83`, `discussion_handlers.go:79,103`: the server, not the model, ends sessions and generates the stored summary.
- `api/internal/app/firestore_discussion.go:43,132`: legacy session docs must still read back, with empty lists rather than null.
- `api/internal/app/firestore_repo.go:189,198`: query-level scan ceilings bound Firestore read cost on the insight path.
- `api/cmd/seedquestions/main.go:54`: must be run from `api/` for the default bank path to resolve.
- `fe/src/lib/diffWords.ts:17`: must agree with `normalizeAnswer` in `api/internal/app/handlers.go`.
- `fe/src/lib/diffWords.ts:11`: the diff is quadratic, so highlighting is skipped for long answers.
- `fe/src/components/Translator.tsx:28`: the follow-up history count mirrors the server's `maxFollowUpHistory`.
- `fe/src/lib/firebase.ts:12`: `getAuth()` must not run during the static-export prerender.
- `fe/src/lib/speech.ts:1`: the two timeouts are Safari workarounds.
- `fe/src/lib/api.ts:92`: the field is optional because the frontend and API deploy independently.
- `fe/src/components/ReviewPanel.tsx:88`: model Markdown is rendered with links and images stripped.
- `fe/src/components/DiscussionSession.tsx:84`: a retry replaces the pending user turn (two user turns in a row are rejected).
- `fe/src/components/Mistakes.tsx:15,71`: the insight cache key is scoped by user and language; a cache hit must clear the loading flag itself.
- `fe/src/components/SessionHistory.tsx:54`, `Translator.tsx:83`: request generations drop stale in-flight responses.
- `fe/src/components/ui/sheet.tsx:24,46`: the focus trap and its focusable selector include visually hidden checkboxes.
- `e2e/tests/helpers.ts:18,24`: `signInWithRedirect` needs `waitForURL`; the emulator page must finish attaching its handlers before clicks.
- `.github/workflows/deploy-api.yml:52`: the MCP token secret is mounted only once the repo variable names an existing secret (Cloud Run fails otherwise).
- `.github/workflows/deploy-fe.yml:48`: the Firebase web config is public by design; don't move it into secrets.
