# Multi-Email Allowlist Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the single-email `ALLOWED_EMAIL` exact-match check with a comma-separated `ALLOWED_EMAILS` list, so more than one address can be authorized.

**Architecture:** Add a small pure parsing function that turns the `ALLOWED_EMAILS` env var into a `[]string`; change `requireAuth`/`NewMux` to take that slice and check membership with `slices.Contains` instead of `==`; wire the parsed slice through `main.go`; update the `.env.example` placeholder and the Cloud Run deploy workflow's secret mapping.

**Tech Stack:** Go 1.25, stdlib `slices` and `strings`, existing `net/http/httptest`-based unit tests.

## Global Constraints

- Env var name is `ALLOWED_EMAILS` (plural), comma-separated (spec:
  `docs/superpowers/specs/2026-07-26-multi-email-allowlist-design.md`).
- The underlying GCP Secret Manager secret resource name stays
  `ALLOWED_EMAIL` — only the Cloud Run env var name changes. No new
  secret, no IAM changes.
- Startup must still fail fast (`log.Fatal`) if the parsed allowlist ends
  up empty, matching current behavior for the empty-string case.
- 401 (not 403) on any auth failure — unchanged, do not touch this
  behavior.
- No custom set type — use `slices.Contains` over a `[]string`.

---

### Task 1: Allowed-emails parsing helper

**Files:**
- Create: `api/internal/app/allowlist.go`
- Test: `api/internal/app/allowlist_test.go`

**Interfaces:**
- Produces: `func ParseAllowedEmails(raw string) []string` in package
  `app` — splits `raw` on `,`, trims whitespace from each entry, and
  omits empty entries. `ParseAllowedEmails("")` returns `nil` (an empty
  slice with length 0).

- [ ] **Step 1: Write the failing tests**

Create `api/internal/app/allowlist_test.go`:

```go
package app

import "testing"

func TestParseAllowedEmailsSplitsOnComma(t *testing.T) {
	got := ParseAllowedEmails("a@example.com,b@example.com")
	want := []string{"a@example.com", "b@example.com"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestParseAllowedEmailsTrimsWhitespace(t *testing.T) {
	got := ParseAllowedEmails(" a@example.com , b@example.com ")
	want := []string{"a@example.com", "b@example.com"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestParseAllowedEmailsDropsEmptyEntries(t *testing.T) {
	got := ParseAllowedEmails("a@example.com,,b@example.com,")
	want := []string{"a@example.com", "b@example.com"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestParseAllowedEmailsEmptyStringReturnsEmpty(t *testing.T) {
	got := ParseAllowedEmails("")
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %v", got)
	}
}

func TestParseAllowedEmailsSingleEmail(t *testing.T) {
	got := ParseAllowedEmails("solo@example.com")
	if len(got) != 1 || got[0] != "solo@example.com" {
		t.Fatalf("expected [solo@example.com], got %v", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/app/... -run TestParseAllowedEmails -v`
Expected: FAIL — `ParseAllowedEmails` is undefined (compile error).

- [ ] **Step 3: Write minimal implementation**

Create `api/internal/app/allowlist.go`:

```go
package app

import "strings"

// ParseAllowedEmails splits a comma-separated list of emails (e.g. the
// ALLOWED_EMAILS env var), trimming whitespace and dropping empty entries.
func ParseAllowedEmails(raw string) []string {
	var emails []string
	for _, e := range strings.Split(raw, ",") {
		e = strings.TrimSpace(e)
		if e != "" {
			emails = append(emails, e)
		}
	}
	return emails
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./internal/app/... -run TestParseAllowedEmails -v`
Expected: PASS (all 5 tests)

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/allowlist.go api/internal/app/allowlist_test.go
git commit -m "feat(api): add ParseAllowedEmails helper for multi-email allowlist"
```

---

### Task 2: Multi-email auth check

**Files:**
- Modify: `api/internal/app/auth.go`
- Modify: `api/internal/app/router.go`
- Modify: `api/internal/app/auth_test.go`

**Interfaces:**
- Consumes: nothing from Task 1 (this task changes the type the caller
  passes in, independent of how that slice was produced).
- Produces: `func requireAuth(v TokenVerifier, allowedEmails []string, next http.HandlerFunc) http.HandlerFunc`
  and `func NewMux(srv *Server, verifier TokenVerifier, allowedEmails []string, frontendURL string) *http.ServeMux`
  — both now take `[]string` instead of `string`. Task 3 wires
  `ParseAllowedEmails`'s output into these.

- [ ] **Step 1: Write the failing test**

Modify `api/internal/app/auth_test.go` — replace the whole file with:

```go
package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeVerifier struct {
	uid   string
	email string
	err   error
}

func (f fakeVerifier) Verify(_ context.Context, _ string) (string, string, error) {
	return f.uid, f.email, f.err
}

const testAllowedEmail = "test@example.com"

func TestRequireAuthRejectsMissingHeader(t *testing.T) {
	h := requireAuth(fakeVerifier{uid: "u1", email: testAllowedEmail}, []string{testAllowedEmail}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAuthRejectsInvalidToken(t *testing.T) {
	h := requireAuth(fakeVerifier{err: errors.New("bad token")}, []string{testAllowedEmail}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil)
	req.Header.Set("Authorization", "Bearer sometoken")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAuthPassesUID(t *testing.T) {
	var gotUID string
	h := requireAuth(fakeVerifier{uid: "user-123", email: testAllowedEmail}, []string{testAllowedEmail}, func(w http.ResponseWriter, r *http.Request) {
		gotUID, _ = uidFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil)
	req.Header.Set("Authorization", "Bearer validtoken")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotUID != "user-123" {
		t.Fatalf("expected uid user-123, got %q", gotUID)
	}
}

func TestRequireAuthRejectsDisallowedEmail(t *testing.T) {
	h := requireAuth(fakeVerifier{uid: "u1", email: "someone-else@gmail.com"}, []string{testAllowedEmail}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil)
	req.Header.Set("Authorization", "Bearer validtoken")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 (matches corgi's authMiddleware, avoids leaking why auth failed), got %d", rec.Code)
	}
}

func TestRequireAuthRejectsEmptyEmail(t *testing.T) {
	h := requireAuth(fakeVerifier{uid: "u1", email: ""}, []string{testAllowedEmail}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil)
	req.Header.Set("Authorization", "Bearer validtoken")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAuthAllowsSecondEmailInList(t *testing.T) {
	h := requireAuth(fakeVerifier{uid: "u2", email: "second@example.com"}, []string{testAllowedEmail, "second@example.com"}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil)
	req.Header.Set("Authorization", "Bearer validtoken")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/app/... -run TestRequireAuth -v`
Expected: FAIL — compile error, `requireAuth` still takes `allowedEmail string`, not `[]string`.

- [ ] **Step 3: Write minimal implementation**

Modify `api/internal/app/auth.go` — replace the whole file with:

```go
package app

import (
	"context"
	"net/http"
	"slices"
	"strings"
)

// TokenVerifier verifies a bearer credential and returns the caller's uid and
// verified email address.
type TokenVerifier interface {
	Verify(ctx context.Context, idToken string) (uid string, email string, err error)
}

type ctxKey string

const uidCtxKey ctxKey = "uid"

func withUID(ctx context.Context, uid string) context.Context {
	return context.WithValue(ctx, uidCtxKey, uid)
}

func uidFromContext(ctx context.Context) (string, bool) {
	uid, ok := ctx.Value(uidCtxKey).(string)
	return uid, ok
}

// requireAuth wraps a handler, requiring a valid "Authorization: Bearer <token>"
// header whose verified email is present in allowedEmails. On success it
// injects the verified uid into the request context.
//
// This is a multi-email allowlist (exact match against a fixed,
// operator-configured set of addresses) matching the ALLOWED_EMAILS
// convention. A disallowed or missing email is rejected with 401 (not
// 403), matching corgi's authMiddleware, so the response doesn't reveal
// whether the token was invalid or just the wrong account.
func requireAuth(v TokenVerifier, allowedEmails []string, next http.HandlerFunc) http.HandlerFunc {
	const prefix = "Bearer "
	return func(w http.ResponseWriter, r *http.Request) {
		authz := r.Header.Get("Authorization")
		if !strings.HasPrefix(authz, prefix) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		idToken := strings.TrimSpace(strings.TrimPrefix(authz, prefix))
		if idToken == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		uid, email, err := v.Verify(r.Context(), idToken)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if email == "" || !slices.Contains(allowedEmails, email) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r.WithContext(withUID(r.Context(), uid)))
	}
}
```

Modify `api/internal/app/router.go`:

```go
package app

import "net/http"

// NewMux wires all HTTP routes with CORS and auth, matching the API's
// public surface exactly.
func NewMux(srv *Server, verifier TokenVerifier, allowedEmails []string, frontendURL string) *http.ServeMux {
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return withCORS(frontendURL, requireAuth(verifier, allowedEmails, h))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/sentence/random", auth(srv.getRandomSentence))
	mux.HandleFunc("/api/answer/check", auth(srv.checkAnswer))
	mux.HandleFunc("/api/answer/explain", auth(srv.explainAnswer))
	mux.HandleFunc("/api/sentence/report", auth(srv.reportSentence))
	mux.HandleFunc("/api/liveness", livenessHandler)
	return mux
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./internal/app/... -v`
Expected: PASS (all tests in the package, including Task 1's)

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/auth.go api/internal/app/router.go api/internal/app/auth_test.go
git commit -m "feat(api): support multiple allowed emails in requireAuth"
```

---

### Task 3: Wire ALLOWED_EMAILS through main.go, config, and deploy

**Files:**
- Modify: `api/main.go`
- Modify: `api/cmd/e2eserver/main.go` — a second binary (used by the
  Playwright e2e suite) that independently reads `ALLOWED_EMAIL` and
  calls `app.NewMux` with the same signature as `api/main.go`; it must
  be updated in lockstep or the module fails to build.
- Modify: `api/.env.example`
- Modify: `.github/workflows/deploy-api.yml`
- Modify: `e2e/playwright.config.ts` — sets `ALLOWED_EMAIL` in the
  `cmd/e2eserver` process env; must be renamed to `ALLOWED_EMAILS` or
  the e2e server fails its own startup check.

**Interfaces:**
- Consumes: `app.ParseAllowedEmails(raw string) []string` (Task 1),
  `app.NewMux(srv *Server, verifier TokenVerifier, allowedEmails []string, frontendURL string) *http.ServeMux` (Task 2).
- Produces: nothing consumed by later tasks — this is the final
  integration point.

- [ ] **Step 1: Update main.go**

In `api/main.go`, replace:

```go
	allowedEmail := os.Getenv("ALLOWED_EMAIL")
	if allowedEmail == "" {
		log.Fatal("ALLOWED_EMAIL is required")
	}
```

with:

```go
	allowedEmails := app.ParseAllowedEmails(os.Getenv("ALLOWED_EMAILS"))
	if len(allowedEmails) == 0 {
		log.Fatal("ALLOWED_EMAILS is required")
	}
```

And replace:

```go
	mux := app.NewMux(srv, verifier, allowedEmail, frontendURL)
```

with:

```go
	mux := app.NewMux(srv, verifier, allowedEmails, frontendURL)
```

- [ ] **Step 2: Update api/cmd/e2eserver/main.go**

This is a second binary (used by the Playwright e2e suite) that
independently reads `ALLOWED_EMAIL` and calls `app.NewMux` with the same
shape as `api/main.go`. It must change in lockstep or the module fails
to build. In `api/cmd/e2eserver/main.go`, replace:

```go
	allowedEmail := os.Getenv("ALLOWED_EMAIL")
	if allowedEmail == "" {
		log.Fatal("ALLOWED_EMAIL is required")
	}
```

with:

```go
	allowedEmails := app.ParseAllowedEmails(os.Getenv("ALLOWED_EMAILS"))
	if len(allowedEmails) == 0 {
		log.Fatal("ALLOWED_EMAILS is required")
	}
```

And replace:

```go
	mux := app.NewMux(srv, verifier, allowedEmail, frontendURL)
```

with:

```go
	mux := app.NewMux(srv, verifier, allowedEmails, frontendURL)
```

- [ ] **Step 3: Update api/.env.example**

Replace:

```
ALLOWED_EMAIL=you@gmail.com
```

with:

```
ALLOWED_EMAILS=you@gmail.com,someone-else@gmail.com
```

- [ ] **Step 4: Update the Cloud Run deploy workflow**

In `.github/workflows/deploy-api.yml`, replace:

```yaml
            --set-secrets="ALLOWED_EMAIL=ALLOWED_EMAIL:latest,GEMINI_API_KEY=GEMINI_API_KEY:latest"
```

with:

```yaml
            --set-secrets="ALLOWED_EMAILS=ALLOWED_EMAIL:latest,GEMINI_API_KEY=GEMINI_API_KEY:latest"
```

Note: the GCP secret resource itself keeps its existing name
(`ALLOWED_EMAIL`) — only the injected env var name (left-hand side)
changes. Before this deploys, the secret's stored value must be updated
to the comma-separated list, e.g.:

```bash
printf 'you@gmail.com,someone-else@gmail.com' | gcloud secrets versions add ALLOWED_EMAIL --data-file=-
```

This is a manual production step outside the repo — not part of this
task's commit.

- [ ] **Step 5: Update e2e/playwright.config.ts**

The Playwright e2e suite starts `cmd/e2eserver` with `ALLOWED_EMAIL` set
in its process env (`e2e/playwright.config.ts` around line 24). Replace:

```ts
        ALLOWED_EMAIL: 'e2e-test@example.com',
```

with:

```ts
        ALLOWED_EMAILS: 'e2e-test@example.com',
```

(Single value is fine — `ParseAllowedEmails` handles a one-entry list
the same as any other. No comma needed since there's only one e2e test
account, matching `e2e/tests/helpers.ts`'s `TEST_EMAIL` constant, which
is unchanged.)

- [ ] **Step 6: Verify the whole module builds and passes**

Run: `cd api && go vet ./... && go test ./...`
Expected: PASS, no vet warnings, no compile errors anywhere in the module
(confirms both `main.go` and `cmd/e2eserver/main.go` compile against the
new `app.ParseAllowedEmails` and `app.NewMux` signatures).

- [ ] **Step 7: Commit**

```bash
git add api/main.go api/cmd/e2eserver/main.go api/.env.example .github/workflows/deploy-api.yml e2e/playwright.config.ts
git commit -m "feat(api): wire ALLOWED_EMAILS through main, e2e server, and deploy config"
```

---

## After all tasks

- [ ] Update the secret's real value in GCP Secret Manager (manual,
  outside git) before or alongside merging, per Task 3 Step 3.
- [ ] Push the branch and open a PR (per repo convention — see recent
  merged PRs #2, #3, #5).
