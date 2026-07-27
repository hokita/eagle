# Multi-Email Allowlist — Design Spec

## Problem

Access to the API is gated by a single hardcoded allowed email
(`ALLOWED_EMAIL`), checked with an exact string match in `requireAuth`
(api/internal/app/auth.go:37-58). The doc comment on that function even
states it's "a single-user allowlist (one exact email, not a list)." There
is no way to grant a second person (or a second account of the same
person) access without replacing the one allowed address.

## Goal

Allow more than one email address to be authorized, while keeping the
allowlist model (exact match against a fixed, operator-configured set —
not a database of users, not self-service signup).

## Non-goals

- No user/account management system, database-backed user table, or
  signup flow — the allowlist stays a static, env-configured list, just
  with more than one entry.
- No per-user roles or permissions — every allowed email gets the same
  access, as today.
- No change to the underlying GCP secret resource — see Deploy section.

## Config & parsing

- Env var renamed from `ALLOWED_EMAIL` to `ALLOWED_EMAILS`, holding a
  comma-separated list, e.g.:
  `ALLOWED_EMAILS=you@gmail.com,someone-else@gmail.com`
- `api/main.go` reads `ALLOWED_EMAILS`, splits on `,`, trims surrounding
  whitespace from each entry, and drops any empty entries. If the
  resulting list is empty, `log.Fatal` — same fail-fast behavior as the
  current empty-string check.
- `.env.example` updates its placeholder to
  `ALLOWED_EMAILS=you@gmail.com,someone-else@gmail.com`.

## Auth check

- `allowedEmail string` becomes `allowedEmails []string` through the
  call chain: `main.go` → `app.NewMux` (api/internal/app/router.go:7) →
  `requireAuth` (api/internal/app/auth.go:37).
- The equality check `email != allowedEmail`
  (api/internal/app/auth.go:55) becomes a membership check using stdlib
  `slices.Contains(allowedEmails, email)`. No custom set type — the list
  will only ever hold a handful of entries, so a linear scan is simplest
  and plenty fast.
- Update the doc comment on `requireAuth` (auth.go:28-36) to drop the
  "single-user allowlist... one exact email, not a list" language and
  describe it as a multi-email allowlist instead.
- Error handling is unchanged: invalid token, missing header, or an
  email not in the list all still return 401 (not 403), so the response
  doesn't reveal *why* auth failed.

## Deploy

- `.github/workflows/deploy-api.yml:60`: change
  `--set-secrets="ALLOWED_EMAIL=ALLOWED_EMAIL:latest,GEMINI_API_KEY=GEMINI_API_KEY:latest"`
  to
  `--set-secrets="ALLOWED_EMAILS=ALLOWED_EMAIL:latest,GEMINI_API_KEY=GEMINI_API_KEY:latest"`.
- The GCP Secret Manager secret resource keeps its existing name
  (`ALLOWED_EMAIL`) — only the left-hand side (the injected env var name)
  changes. This avoids creating a new secret or updating IAM grants.
- The secret's *stored value* must be updated to the comma-separated
  list (e.g. via `gcloud secrets versions add ALLOWED_EMAIL --data-file=-`)
  before/alongside this deploy. This is a manual production step, not
  part of the code change.

## Tests (TDD: red before green)

- `api/internal/app/auth_test.go`: existing tests switch their
  `testAllowedEmail` argument from a bare string to
  `[]string{testAllowedEmail}`.
- Add a new failing test first — `TestRequireAuthAllowsSecondEmailInList`
  — asserting that a second address in a multi-entry allowlist
  (e.g. `[]string{testAllowedEmail, "second@example.com"}`, request
  authenticated as `second@example.com`) is authorized (200, correct uid
  in context). Then implement `slices.Contains` to make it pass.
- Existing `TestRequireAuthRejectsDisallowedEmail` and
  `TestRequireAuthRejectsEmptyEmail` continue to assert 401 unchanged.

## Open items for implementation

None — all decisions above were confirmed during design review.
