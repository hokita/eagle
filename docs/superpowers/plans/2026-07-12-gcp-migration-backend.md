# GCP Migration — Backend & Seeder Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite the Go API's data layer from MySQL to Firestore behind a `SentenceRepository` interface, add Firebase ID-token auth middleware, and add a seeder that imports the master sentence data into Firestore.

**Architecture:** HTTP handlers become methods on a `Server` that depends only on a `SentenceRepository` interface, so they unit-test against an in-memory fake. A `firestoreRepo` implements that interface and is integration-tested against the Firestore emulator. A standalone `requireAuth` middleware verifies Firebase ID tokens through a `TokenVerifier` interface (real impl uses the Firebase Admin SDK; tests use a fake). A separate `cmd/seed` program upserts sentences from a newline-delimited JSON export.

**Tech Stack:** Go 1.23, `cloud.google.com/go/firestore`, `firebase.google.com/go/v4`, `google.golang.org/api/iterator`, the Firestore emulator, Go's `net/http` + `net/http/httptest`.

## Global Constraints

- Module path is `github.com/hokita/eagle`, rooted at `api/` (go.mod lives in `api/`). All package paths below are relative to `api/`.
- The public JSON response bodies MUST stay byte-for-byte compatible with the current frontend: `Sentence` fields `id, japanese, english, page, correct_count, incorrect_count, created_at, updated_at`; `CheckAnswerResponse` fields `is_correct, correct_answer, histories`; each history `id, incorrect_answer, created_at`. `page` is a JSON **string**; `histories` is never `null` (empty array instead).
- History `id` = the attempt's `created_at` in **unix microseconds** (`time.Time.UnixMicro()`) — this stays within JavaScript's safe-integer range and is only used as a React key.
- Firestore layout: shared `sentences/{id}` (doc ID = integer id as string); per-user `users/{uid}/sentence_stats/{sentenceId}` counters; per-user `users/{uid}/sentence_stats/{sentenceId}/histories/{autoId}` attempts.
- Answer correctness = `strings.EqualFold(strings.TrimSpace(userAnswer), strings.TrimSpace(english))` (case-insensitive, trimmed) — identical to today.
- The random filter keeps sentences where `correct_count - incorrect_count < 2` and `is_reported == false`.
- TDD: write the failing test first, watch it fail, implement minimally, watch it pass, commit. No auth on `/api/liveness`.
- **Single-user email allowlist**, matching the `corgi` project's convention (`backend/src/middleware/auth.ts`): one exact `ALLOWED_EMAIL` (singular, not a list), checked after token verification. A missing/invalid token OR a verified-but-disallowed email both return **401** (not 403) — mirrors corgi's `authMiddleware`, which intentionally doesn't reveal *why* auth failed.
- Frontend Firebase Auth changes and all GCP infra / CI-CD are **out of scope for this plan** — they are covered by separate plans (`...-frontend-auth.md`, `...-infra-deploy.md`).

## File structure (after this plan)

```
api/
  main.go              wiring only: build clients, register routes, serve
  sentence.go          domain types + SentenceRepository interface + sentinel errors
  handlers.go          Server struct + HTTP handler methods + writeJSON + liveness
  handlers_test.go     unit tests using an in-memory fakeRepo
  auth.go              TokenVerifier interface + context helpers + requireAuth middleware
  auth_test.go         middleware unit tests using a fakeVerifier
  auth_firebase.go     firebaseVerifier (Firebase Admin SDK) implementing TokenVerifier
  firestore_repo.go    firestoreRepo implementing SentenceRepository
  firestore_repo_test.go   emulator integration tests (skipped when emulator absent)
  cmd/seed/main.go     NDJSON -> Firestore sentence upserter
  .env.example         template listing GOOGLE_CLOUD_PROJECT, ALLOWED_EMAIL, PORT
```

Deleted: `api/main_test.go` (old server/MySQL integration tests), `api/.env` (stale untracked MySQL config — never committed to git, excluded by `.gitignore`).

---

### Task 1: Auth middleware + TokenVerifier interface — DONE (amended in review)

> Implemented with an email allowlist added after code review (crit comment: "is it only allow test@example.com?"). Investigated `corgi`'s `backend/src/middleware/auth.ts` and matched its convention: a single `ALLOWED_EMAIL` (not a list) checked after token verification, with disallowed email rejected as **401** (not 403) so the response doesn't reveal *why* auth failed. `TokenVerifier.Verify` now returns `(uid, email, error)` instead of `(uid, error)` — this changes the signature Task 4's `firebaseVerifier` must implement below.

**Files:**
- Create: `api/auth.go`
- Test: `api/auth_test.go`

**Interfaces:**
- Produces: `TokenVerifier` interface (`Verify(ctx context.Context, idToken string) (uid string, email string, err error)`); `requireAuth(v TokenVerifier, allowedEmail string, next http.HandlerFunc) http.HandlerFunc`; `withUID(ctx, uid) context.Context`; `uidFromContext(ctx) (string, bool)`.
- Consumes: nothing (isolated).

- [x] **Step 1: Write the failing tests**

`api/auth_test.go`:

```go
package main

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
	h := requireAuth(fakeVerifier{uid: "u1", email: testAllowedEmail}, testAllowedEmail, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAuthRejectsInvalidToken(t *testing.T) {
	h := requireAuth(fakeVerifier{err: errors.New("bad token")}, testAllowedEmail, func(w http.ResponseWriter, r *http.Request) {
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
	h := requireAuth(fakeVerifier{uid: "user-123", email: testAllowedEmail}, testAllowedEmail, func(w http.ResponseWriter, r *http.Request) {
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
	h := requireAuth(fakeVerifier{uid: "u1", email: "someone-else@gmail.com"}, testAllowedEmail, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil)
	req.Header.Set("Authorization", "Bearer validtoken")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 (matches corgi's authMiddleware), got %d", rec.Code)
	}
}

func TestRequireAuthRejectsEmptyEmail(t *testing.T) {
	h := requireAuth(fakeVerifier{uid: "u1", email: ""}, testAllowedEmail, func(w http.ResponseWriter, r *http.Request) {
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
```

- [x] **Step 2: Run tests to verify they fail**

Ran: `cd api && go test -run TestRequireAuth ./...` → FAIL (compile error — signature mismatch after adding the email parameter).

- [x] **Step 3: Write the implementation**

`api/auth.go`:

```go
package main

import (
	"context"
	"net/http"
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
// header whose verified email matches allowedEmail. On success it injects the
// verified uid into the request context.
//
// This is a single-user allowlist (one exact email, not a list) matching the
// ALLOWED_EMAIL convention used by the corgi project. A disallowed or missing
// email is rejected with 401 (not 403), matching corgi's authMiddleware, so
// the response doesn't reveal whether the token was invalid or just the
// wrong account.
func requireAuth(v TokenVerifier, allowedEmail string, next http.HandlerFunc) http.HandlerFunc {
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
		if email == "" || email != allowedEmail {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r.WithContext(withUID(r.Context(), uid)))
	}
}
```

- [x] **Step 4: Run tests to verify they pass**

Ran: `cd api && go test -run TestRequireAuth ./...` → PASS (5 tests).

- [x] **Step 5: Commit**

```bash
git add api/auth.go api/auth_test.go
git commit -m "feat(api): add Firebase-token auth middleware with verifier interface"
git commit -m "feat(api): restrict auth middleware to a single ALLOWED_EMAIL (matches corgi)"
```

---

### Task 2: Domain types, repository interface, handlers, and unit tests

This task cuts the old MySQL handlers/types out of `main.go` and replaces them with interface-driven handlers. After it, `main()` is a temporary stub (real wiring lands in Task 5); handlers are fully unit-tested via `httptest`.

**Files:**
- Create: `api/sentence.go`, `api/handlers.go`, `api/handlers_test.go`
- Modify: `api/main.go` (strip to a stub)
- Delete: `api/main_test.go`

**Interfaces:**
- Consumes: `withUID`, `uidFromContext` (Task 1).
- Produces: types `Sentence`, `AnswerHistory`, `CheckAnswerRequest`, `CheckAnswerResponse`, `ReportSentenceRequest`; sentinel errors `ErrNotFound`, `ErrNoCandidate`; interface `SentenceRepository` with methods:
  - `RandomCandidate(ctx, uid string) (*Sentence, error)`
  - `CorrectAnswer(ctx, id int) (string, error)`
  - `ListIncorrectHistories(ctx, uid string, id int) ([]AnswerHistory, error)`
  - `RecordAnswer(ctx, uid string, id int, correct bool, answer string) error`
  - `Report(ctx, id int) error`
  - `Server` struct with `NewServer(repo SentenceRepository) *Server` and handler methods `getRandomSentence`, `checkAnswer`, `reportSentence`; package funcs `livenessHandler`, `writeJSON`.

- [ ] **Step 1: Delete the obsolete integration test**

```bash
git rm api/main_test.go
```

- [ ] **Step 2: Create the domain types and interface**

Create `api/sentence.go`:

```go
package main

import (
	"context"
	"errors"
)

type Sentence struct {
	ID             int    `json:"id"`
	Japanese       string `json:"japanese"`
	English        string `json:"english"`
	Page           string `json:"page"`
	CorrectCount   int    `json:"correct_count"`
	IncorrectCount int    `json:"incorrect_count"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type AnswerHistory struct {
	ID              int64  `json:"id"`
	IncorrectAnswer string `json:"incorrect_answer"`
	CreatedAt       string `json:"created_at"`
}

type CheckAnswerRequest struct {
	SentenceID int    `json:"sentence_id"`
	UserAnswer string `json:"user_answer"`
}

type CheckAnswerResponse struct {
	IsCorrect     bool            `json:"is_correct"`
	CorrectAnswer string          `json:"correct_answer"`
	Histories     []AnswerHistory `json:"histories"`
}

type ReportSentenceRequest struct {
	SentenceID int `json:"sentence_id"`
}

// ErrNotFound is returned when a sentence document does not exist.
var ErrNotFound = errors.New("sentence not found")

// ErrNoCandidate is returned when no sentence passes the random filter.
var ErrNoCandidate = errors.New("no candidate sentence")

// SentenceRepository is the data-access seam behind the HTTP handlers.
type SentenceRepository interface {
	RandomCandidate(ctx context.Context, uid string) (*Sentence, error)
	CorrectAnswer(ctx context.Context, id int) (string, error)
	ListIncorrectHistories(ctx context.Context, uid string, id int) ([]AnswerHistory, error)
	RecordAnswer(ctx context.Context, uid string, id int, correct bool, answer string) error
	Report(ctx context.Context, id int) error
}
```

- [ ] **Step 3: Write the failing handler tests**

Create `api/handlers_test.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type recordedAnswer struct {
	uid     string
	id      int
	correct bool
	answer  string
}

type fakeRepo struct {
	random     *Sentence
	randomErr  error
	correct    string
	correctErr error
	histories  []AnswerHistory
	recorded   []recordedAnswer
	reported   []int
}

func (f *fakeRepo) RandomCandidate(_ context.Context, _ string) (*Sentence, error) {
	return f.random, f.randomErr
}
func (f *fakeRepo) CorrectAnswer(_ context.Context, _ int) (string, error) {
	return f.correct, f.correctErr
}
func (f *fakeRepo) ListIncorrectHistories(_ context.Context, _ string, _ int) ([]AnswerHistory, error) {
	if f.histories == nil {
		return []AnswerHistory{}, nil
	}
	return f.histories, nil
}
func (f *fakeRepo) RecordAnswer(_ context.Context, uid string, id int, correct bool, answer string) error {
	f.recorded = append(f.recorded, recordedAnswer{uid, id, correct, answer})
	return nil
}
func (f *fakeRepo) Report(_ context.Context, id int) error {
	f.reported = append(f.reported, id)
	return nil
}

func authed(req *http.Request, uid string) *http.Request {
	return req.WithContext(withUID(req.Context(), uid))
}

func TestGetRandomSentenceOK(t *testing.T) {
	repo := &fakeRepo{random: &Sentence{ID: 7, Japanese: "犬", English: "dog", Page: "3"}}
	srv := NewServer(repo)
	rec := httptest.NewRecorder()
	srv.getRandomSentence(rec, authed(httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got Sentence
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != 7 || got.English != "dog" {
		t.Fatalf("unexpected body: %+v", got)
	}
}

func TestGetRandomSentenceNoCandidate(t *testing.T) {
	srv := NewServer(&fakeRepo{randomErr: ErrNoCandidate})
	rec := httptest.NewRecorder()
	srv.getRandomSentence(rec, authed(httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil), "u1"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestCheckAnswerCorrect(t *testing.T) {
	repo := &fakeRepo{correct: "I don't have time."}
	srv := NewServer(repo)
	body := `{"sentence_id":1,"user_answer":"  i don't have TIME. "}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/answer/check", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	srv.checkAnswer(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp CheckAnswerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.IsCorrect {
		t.Fatal("expected correct")
	}
	if resp.Histories == nil {
		t.Fatal("histories must not be null")
	}
	if len(repo.recorded) != 1 || repo.recorded[0].correct != true || repo.recorded[0].uid != "u1" {
		t.Fatalf("expected one correct recorded answer for u1, got %+v", repo.recorded)
	}
}

func TestCheckAnswerIncorrectRecordsAnswer(t *testing.T) {
	repo := &fakeRepo{correct: "It's hot today."}
	srv := NewServer(repo)
	body := `{"sentence_id":2,"user_answer":"It is hot today."}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/answer/check", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	srv.checkAnswer(rec, req)
	var resp CheckAnswerResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.IsCorrect {
		t.Fatal("expected incorrect")
	}
	if len(repo.recorded) != 1 || repo.recorded[0].answer != "It is hot today." {
		t.Fatalf("expected incorrect answer recorded, got %+v", repo.recorded)
	}
}

func TestCheckAnswerNotFound(t *testing.T) {
	srv := NewServer(&fakeRepo{correctErr: ErrNotFound})
	body := `{"sentence_id":999,"user_answer":"x"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/answer/check", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	srv.checkAnswer(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestReportSentence(t *testing.T) {
	repo := &fakeRepo{}
	srv := NewServer(repo)
	body := `{"sentence_id":5}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/sentence/report", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	srv.reportSentence(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if len(repo.reported) != 1 || repo.reported[0] != 5 {
		t.Fatalf("expected sentence 5 reported, got %+v", repo.reported)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	srv := NewServer(&fakeRepo{})
	rec := httptest.NewRecorder()
	srv.getRandomSentence(rec, authed(httptest.NewRequest(http.MethodPost, "/api/sentence/random", nil), "u1"))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `cd api && go test -run 'TestGetRandom|TestCheckAnswer|TestReport|TestMethodNotAllowed' ./...`
Expected: FAIL (compile error — `NewServer`, handlers undefined).

- [ ] **Step 5: Write the handlers**

Create `api/handlers.go`:

```go
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
)

type Server struct {
	repo SentenceRepository
}

func NewServer(repo SentenceRepository) *Server {
	return &Server{repo: repo}
}

func (s *Server) getRandomSentence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid, _ := uidFromContext(r.Context())
	sentence, err := s.repo.RandomCandidate(r.Context(), uid)
	if errors.Is(err, ErrNoCandidate) {
		http.Error(w, "No sentences found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("random candidate error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, sentence)
}

func (s *Server) checkAnswer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid, _ := uidFromContext(r.Context())
	var req CheckAnswerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	correct, err := s.repo.CorrectAnswer(r.Context(), req.SentenceID)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "Sentence not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("correct answer error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	histories, err := s.repo.ListIncorrectHistories(r.Context(), uid, req.SentenceID)
	if err != nil {
		log.Printf("list histories error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	isCorrect := strings.EqualFold(strings.TrimSpace(req.UserAnswer), strings.TrimSpace(correct))
	answer := ""
	if !isCorrect {
		answer = req.UserAnswer
	}
	if err := s.repo.RecordAnswer(r.Context(), uid, req.SentenceID, isCorrect, answer); err != nil {
		log.Printf("record answer error: %v", err)
	}
	writeJSON(w, CheckAnswerResponse{
		IsCorrect:     isCorrect,
		CorrectAnswer: correct,
		Histories:     histories,
	})
}

func (s *Server) reportSentence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req ReportSentenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if err := s.repo.Report(r.Context(), req.SentenceID); err != nil {
		log.Printf("report error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func livenessHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "OK")
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode error: %v", err)
	}
}
```

- [ ] **Step 6: Strip `main.go` to a stub**

Replace the entire contents of `api/main.go` with:

```go
package main

// Wiring is completed in a later task. Handlers and types live in
// handlers.go / sentence.go and are exercised by unit tests.
func main() {}
```

- [ ] **Step 7: Run the full package tests**

Run: `cd api && go test ./...`
Expected: PASS (Task 1 + Task 2 tests; no MySQL needed).

- [ ] **Step 8: Commit**

```bash
git add api/sentence.go api/handlers.go api/handlers_test.go api/main.go
git commit -m "refactor(api): interface-driven handlers with unit tests; drop MySQL handlers"
```

---

### Task 3: Firestore repository implementation (emulator-tested)

**Files:**
- Create: `api/firestore_repo.go`, `api/firestore_repo_test.go`
- Modify: `api/go.mod`, `api/go.sum` (add Firestore deps)

**Interfaces:**
- Consumes: `SentenceRepository`, `Sentence`, `AnswerHistory`, `ErrNotFound`, `ErrNoCandidate` (Task 2).
- Produces: `NewFirestoreRepo(client *firestore.Client) *firestoreRepo` implementing `SentenceRepository`.

- [ ] **Step 1: Add the Firestore dependency**

Run: `cd api && go get cloud.google.com/go/firestore@latest google.golang.org/api@latest`
Expected: `go.mod` gains `cloud.google.com/go/firestore` and `google.golang.org/api`.

- [ ] **Step 2: Write the failing emulator tests**

Create `api/firestore_repo_test.go`:

```go
package main

import (
	"context"
	"os"
	"testing"

	"cloud.google.com/go/firestore"
)

func newEmulatorClient(t *testing.T) *firestore.Client {
	t.Helper()
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set; skipping emulator test")
	}
	client, err := firestore.NewClient(context.Background(), "eagle-test")
	if err != nil {
		t.Fatalf("firestore client: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func seedSentence(t *testing.T, client *firestore.Client, id, page string, jp, en string, reported bool) {
	t.Helper()
	_, err := client.Collection("sentences").Doc(id).Set(context.Background(), map[string]interface{}{
		"japanese": jp, "english": en, "page": page, "is_reported": reported,
		"created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("seed sentence %s: %v", id, err)
	}
}

func TestFirestoreCorrectAnswerNotFound(t *testing.T) {
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	if _, err := repo.CorrectAnswer(context.Background(), 424242); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFirestoreRecordListAndCount(t *testing.T) {
	ctx := context.Background()
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	uid := "user-record"
	seedSentence(t, client, "201", "5", "こんにちは", "Hello", false)

	if err := repo.RecordAnswer(ctx, uid, 201, false, "Hi there"); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordAnswer(ctx, uid, 201, true, ""); err != nil {
		t.Fatal(err)
	}

	hs, err := repo.ListIncorrectHistories(ctx, uid, 201)
	if err != nil {
		t.Fatal(err)
	}
	if len(hs) != 1 || hs[0].IncorrectAnswer != "Hi there" {
		t.Fatalf("expected 1 incorrect history 'Hi there', got %+v", hs)
	}
	if hs[0].ID == 0 || hs[0].CreatedAt == "" {
		t.Fatalf("history id/created_at should be populated, got %+v", hs[0])
	}

	s, err := repo.RandomCandidate(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if s.CorrectCount != 1 || s.IncorrectCount != 1 {
		t.Fatalf("expected counts 1/1, got %d/%d", s.CorrectCount, s.IncorrectCount)
	}
}

func TestFirestoreRandomExcludesMasteredAndReported(t *testing.T) {
	ctx := context.Background()
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	uid := "user-filter"
	seedSentence(t, client, "301", "1", "A", "A", false) // mastered below
	seedSentence(t, client, "302", "1", "B", "B", true)  // reported

	// Push 301 to net +2 (correct - incorrect >= 2) -> excluded.
	if err := repo.RecordAnswer(ctx, uid, 301, true, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordAnswer(ctx, uid, 301, true, ""); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 8; i++ {
		s, err := repo.RandomCandidate(ctx, uid)
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if s.ID == 301 {
			t.Fatal("mastered sentence 301 should be excluded")
		}
		if s.ID == 302 {
			t.Fatal("reported sentence 302 should be excluded")
		}
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd api && go test -run TestFirestore ./...`
Expected: FAIL (compile error — `NewFirestoreRepo` undefined).

- [ ] **Step 4: Implement the Firestore repository**

Create `api/firestore_repo.go`:

```go
package main

import (
	"context"
	"errors"
	"math/rand"
	"strconv"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type firestoreRepo struct {
	client *firestore.Client
	now    func() time.Time
}

func NewFirestoreRepo(client *firestore.Client) *firestoreRepo {
	return &firestoreRepo{client: client, now: time.Now}
}

type sentenceDoc struct {
	Japanese   string `firestore:"japanese"`
	English    string `firestore:"english"`
	Page       string `firestore:"page"`
	IsReported bool   `firestore:"is_reported"`
	CreatedAt  string `firestore:"created_at"`
	UpdatedAt  string `firestore:"updated_at"`
}

type statsDoc struct {
	CorrectCount   int `firestore:"correct_count"`
	IncorrectCount int `firestore:"incorrect_count"`
}

type historyDoc struct {
	IsCorrect       bool      `firestore:"is_correct"`
	IncorrectAnswer string    `firestore:"incorrect_answer"`
	CreatedAt       time.Time `firestore:"created_at"`
}

func (r *firestoreRepo) userStats(uid string) *firestore.CollectionRef {
	return r.client.Collection("users").Doc(uid).Collection("sentence_stats")
}

func (r *firestoreRepo) RandomCandidate(ctx context.Context, uid string) (*Sentence, error) {
	sentenceDocs, err := r.client.Collection("sentences").
		Where("is_reported", "==", false).Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}

	stats := map[int]statsDoc{}
	it := r.userStats(uid).Documents(ctx)
	for {
		ds, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		id, convErr := strconv.Atoi(ds.Ref.ID)
		if convErr != nil {
			continue
		}
		var st statsDoc
		if err := ds.DataTo(&st); err != nil {
			return nil, err
		}
		stats[id] = st
	}

	var candidates []*Sentence
	for _, ds := range sentenceDocs {
		id, convErr := strconv.Atoi(ds.Ref.ID)
		if convErr != nil {
			continue
		}
		var sd sentenceDoc
		if err := ds.DataTo(&sd); err != nil {
			return nil, err
		}
		st := stats[id]
		if st.CorrectCount-st.IncorrectCount >= 2 {
			continue
		}
		candidates = append(candidates, &Sentence{
			ID:             id,
			Japanese:       sd.Japanese,
			English:        sd.English,
			Page:           sd.Page,
			CorrectCount:   st.CorrectCount,
			IncorrectCount: st.IncorrectCount,
			CreatedAt:      sd.CreatedAt,
			UpdatedAt:      sd.UpdatedAt,
		})
	}
	if len(candidates) == 0 {
		return nil, ErrNoCandidate
	}
	return candidates[rand.Intn(len(candidates))], nil
}

func (r *firestoreRepo) CorrectAnswer(ctx context.Context, id int) (string, error) {
	ds, err := r.client.Collection("sentences").Doc(strconv.Itoa(id)).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	var sd sentenceDoc
	if err := ds.DataTo(&sd); err != nil {
		return "", err
	}
	return sd.English, nil
}

func (r *firestoreRepo) ListIncorrectHistories(ctx context.Context, uid string, id int) ([]AnswerHistory, error) {
	it := r.userStats(uid).Doc(strconv.Itoa(id)).Collection("histories").
		Where("is_correct", "==", false).
		OrderBy("created_at", firestore.Desc).Documents(ctx)
	histories := make([]AnswerHistory, 0)
	for {
		ds, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		var hd historyDoc
		if err := ds.DataTo(&hd); err != nil {
			return nil, err
		}
		histories = append(histories, AnswerHistory{
			ID:              hd.CreatedAt.UnixMicro(),
			IncorrectAnswer: hd.IncorrectAnswer,
			CreatedAt:       hd.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return histories, nil
}

func (r *firestoreRepo) RecordAnswer(ctx context.Context, uid string, id int, correct bool, answer string) error {
	now := r.now().UTC()
	statsRef := r.userStats(uid).Doc(strconv.Itoa(id))
	histRef := statsRef.Collection("histories").NewDoc()

	field := "incorrect_count"
	if correct {
		field = "correct_count"
	}

	batch := r.client.Batch()
	batch.Set(statsRef, map[string]interface{}{
		field:        firestore.Increment(1),
		"updated_at": now.Format(time.RFC3339),
	}, firestore.MergeAll)
	batch.Set(histRef, historyDoc{
		IsCorrect:       correct,
		IncorrectAnswer: answer,
		CreatedAt:       now,
	})
	_, err := batch.Commit(ctx)
	return err
}

func (r *firestoreRepo) Report(ctx context.Context, id int) error {
	_, err := r.client.Collection("sentences").Doc(strconv.Itoa(id)).
		Update(ctx, []firestore.Update{{Path: "is_reported", Value: true}})
	return err
}
```

- [ ] **Step 5: Start the emulator and run the tests**

Run:
```bash
cd api
gcloud emulators firestore start --host-port=localhost:8090 &
export FIRESTORE_EMULATOR_HOST=localhost:8090
go test -run TestFirestore ./...
```
Expected: PASS (3 tests). Then stop the emulator (`kill %1`) or leave it for later tasks.
(If `gcloud` is unavailable, the tests `t.Skip` — but you MUST run them against the emulator at least once before marking this task done.)

- [ ] **Step 6: Tidy and commit**

```bash
cd api && go mod tidy
git add api/firestore_repo.go api/firestore_repo_test.go api/go.mod api/go.sum
git commit -m "feat(api): Firestore repository with emulator integration tests"
```

---

### Task 4: Firebase token verifier

**Files:**
- Create: `api/auth_firebase.go`
- Modify: `api/go.mod`, `api/go.sum` (add Firebase Admin SDK)

**Interfaces:**
- Consumes: `TokenVerifier` (Task 1).
- Produces: `NewFirebaseVerifier(ctx context.Context, projectID string) (*firebaseVerifier, error)` implementing `TokenVerifier`.

- [ ] **Step 1: Add the Firebase Admin SDK**

Run: `cd api && go get firebase.google.com/go/v4@latest`
Expected: `go.mod` gains `firebase.google.com/go/v4`.

- [ ] **Step 2: Implement the verifier**

Create `api/auth_firebase.go`:

```go
package main

import (
	"context"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
)

type firebaseVerifier struct {
	client *auth.Client
}

// NewFirebaseVerifier builds a TokenVerifier backed by the Firebase Admin SDK.
// Honors FIREBASE_AUTH_EMULATOR_HOST for local development.
func NewFirebaseVerifier(ctx context.Context, projectID string) (*firebaseVerifier, error) {
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	client, err := app.Auth(ctx)
	if err != nil {
		return nil, err
	}
	return &firebaseVerifier{client: client}, nil
}

func (v *firebaseVerifier) Verify(ctx context.Context, idToken string) (string, string, error) {
	token, err := v.client.VerifyIDToken(ctx, idToken)
	if err != nil {
		return "", "", err
	}
	email, _ := token.Claims["email"].(string)
	return token.UID, email, nil
}
```

> Note: `Verify` returns `(uid, email, error)` — see the Task 1 amendment above (single-email allowlist, matching `corgi`'s `ALLOWED_EMAIL` convention). `token.Claims["email"]` is how the Firebase Admin Go SDK exposes the ID token's email claim.

- [ ] **Step 3: Verify it compiles and the suite still passes**

Run: `cd api && go build ./... && go test ./...`
Expected: build OK; unit tests PASS (emulator tests skip without the env var).

- [ ] **Step 4: Tidy and commit**

```bash
cd api && go mod tidy
git add api/auth_firebase.go api/go.mod api/go.sum
git commit -m "feat(api): Firebase Admin SDK token verifier"
```

---

### Task 5: Wire everything in `main.go`

**Files:**
- Modify: `api/main.go`
- Create: `api/.env.example`
- Delete: `api/.env`

**Interfaces:**
- Consumes: `NewFirestoreRepo`, `NewServer`, `NewFirebaseVerifier`, `requireAuth`, `livenessHandler`.

- [ ] **Step 1: Replace `main.go` with real wiring**

Replace the entire contents of `api/main.go` with:

```go
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/firestore"
)

func main() {
	ctx := context.Background()

	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		log.Fatal("GOOGLE_CLOUD_PROJECT is required")
	}

	allowedEmail := os.Getenv("ALLOWED_EMAIL")
	if allowedEmail == "" {
		log.Fatal("ALLOWED_EMAIL is required")
	}

	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("failed to create Firestore client: %v", err)
	}
	defer client.Close()

	verifier, err := NewFirebaseVerifier(ctx, projectID)
	if err != nil {
		log.Fatalf("failed to create auth verifier: %v", err)
	}

	srv := NewServer(NewFirestoreRepo(client))

	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return requireAuth(verifier, allowedEmail, h)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/sentence/random", auth(srv.getRandomSentence))
	mux.HandleFunc("/api/answer/check", auth(srv.checkAnswer))
	mux.HandleFunc("/api/sentence/report", auth(srv.reportSentence))
	mux.HandleFunc("/api/liveness", livenessHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
```

- [ ] **Step 2: Remove the obsolete MySQL env file and add a template**

> Correction: an earlier version of this note claimed `api/.env` "held real
> MySQL credentials committed to the repo." That was checked and is false —
> `api/.env` was always excluded by the root `.gitignore`'s `.env` pattern and
> was never tracked by git (`git log --all -- api/.env` is empty). It only
> ever existed as an untracked local file with real credentials in it.

`api/.env` is a stale, untracked local file with real MySQL credentials —
delete it from disk (there is nothing to `git rm`) rather than leaving it
around as dead config for a database that no longer exists. Add a
`.env.example` **template** instead (no real secrets, safe to commit),
matching the pattern `corgi` uses at `backend/.env.example`:

```bash
rm api/.env
```

Create `api/.env.example`:

```
GOOGLE_CLOUD_PROJECT=your-gcp-project-id
ALLOWED_EMAIL=you@gmail.com
PORT=8080
```

- [ ] **Step 3: Build and run against emulators (manual smoke test)**

Run (in one shell):
```bash
gcloud emulators firestore start --host-port=localhost:8090 &
export FIRESTORE_EMULATOR_HOST=localhost:8090
export FIREBASE_AUTH_EMULATOR_HOST=localhost:9099
export GOOGLE_CLOUD_PROJECT=eagle-local
export ALLOWED_EMAIL=test@example.com
cd api && go run .
```
Expected: logs `Server starting on port 8080` with no fatal error.
`curl -s localhost:8080/api/liveness` → `OK`.
`curl -s localhost:8080/api/sentence/random` (no token) → `unauthorized` with HTTP 401.
Stop the server and emulator when done.

- [ ] **Step 4: Full test + vet**

Run: `cd api && go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/main.go api/.env.example
git commit -m "feat(api): wire Firestore + Firebase auth into HTTP server"
```

---

### Task 6: Seeder (`cmd/seed`) for master data

**Files:**
- Create: `api/cmd/seed/main.go`

**Interfaces:**
- Standalone `package main`; does not import the API package. Reads NDJSON with fields `id` (int), `japanese`, `english`, `page` (number), `is_reported` (0/1).

- [ ] **Step 1: Write the seeder**

Create `api/cmd/seed/main.go`:

```go
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"cloud.google.com/go/firestore"
)

type row struct {
	ID         int         `json:"id"`
	Japanese   string      `json:"japanese"`
	English    string      `json:"english"`
	Page       json.Number `json:"page"`
	IsReported int         `json:"is_reported"`
}

func main() {
	path := flag.String("file", "docs/sentences_export.ndjson", "path to NDJSON export")
	flag.Parse()

	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		log.Fatal("GOOGLE_CLOUD_PROJECT is required")
	}

	ctx := context.Background()
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("firestore client: %v", err)
	}
	defer client.Close()

	f, err := os.Open(*path)
	if err != nil {
		log.Fatalf("open %s: %v", *path, err)
	}
	defer f.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	count := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rw row
		if err := json.Unmarshal(line, &rw); err != nil {
			log.Fatalf("parse line: %v", err)
		}
		_, err := client.Collection("sentences").Doc(strconv.Itoa(rw.ID)).Set(ctx, map[string]interface{}{
			"japanese":    rw.Japanese,
			"english":     rw.English,
			"page":        rw.Page.String(),
			"is_reported": rw.IsReported != 0,
			"created_at":  now,
			"updated_at":  now,
		})
		if err != nil {
			log.Fatalf("write sentence %d: %v", rw.ID, err)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("scan: %v", err)
	}
	fmt.Printf("seeded %d sentences\n", count)
}
```

- [ ] **Step 2: Build the seeder**

Run: `cd api && go build ./cmd/seed`
Expected: builds with no error.

- [ ] **Step 3: Smoke-test against the emulator**

Run:
```bash
gcloud emulators firestore start --host-port=localhost:8090 &
export FIRESTORE_EMULATOR_HOST=localhost:8090
export GOOGLE_CLOUD_PROJECT=eagle-local
printf '%s\n%s\n' \
  '{"id":1,"japanese":"これは良い本です。","english":"This is a good book.","page":32,"is_reported":0}' \
  '{"id":2,"japanese":"今日は暑いです。","english":"It is hot today.","page":15,"is_reported":0}' \
  > /tmp/seed.ndjson
cd api && go run ./cmd/seed -file /tmp/seed.ndjson
```
Expected: prints `seeded 2 sentences`. (Optional: re-run to confirm idempotency — count stays 2 in Firestore.)

- [ ] **Step 4: Commit**

```bash
git add api/cmd/seed/main.go
git commit -m "feat(seed): NDJSON -> Firestore sentence seeder"
```

---

### Task 7: CORS middleware — added after a docs correction

> Tasks 1–6 assumed production would be same-origin (via Hosting rewrites) and
> that local dev could cross-origin-proxy through Next.js `rewrites()`. That
> assumption was wrong: Next.js's static-export docs state that `rewrites()`,
> `redirects()`, and `headers()` **error out in `next dev` too**, not just in
> the production build, whenever `output: 'export'` is set. So Task 2's
> removal of CORS left local dev (`next dev` on :3000 calling the API on
> :8080) broken. Checked corgi's actual precedent (`backend/src/app.ts`) —
> it keeps CORS via the `cors` package, driven by a `FRONTEND_URL` env var,
> and documents an explicit dev value (`FRONTEND_URL=http://localhost:5173`
> in `backend/.env.example`) rather than leaving it wildcard-open. This task
> ports that pattern to Go.

**Files:**
- Create: `api/cors.go`, `api/cors_test.go`
- Modify: `api/main.go`, `api/.env.example`

**Interfaces:**
- Consumes: nothing new (wraps any `http.HandlerFunc`).
- Produces: `withCORS(allowedOrigin string, next http.HandlerFunc) http.HandlerFunc`.

- [ ] **Step 1: Write the failing tests**

Create `api/cors_test.go`:

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithCORSSetsWildcardWhenUnset(t *testing.T) {
	h := withCORS("", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil))
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected wildcard origin, got %q", got)
	}
}

func TestWithCORSSetsConfiguredOrigin(t *testing.T) {
	h := withCORS("https://eagle.example.com", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil))
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://eagle.example.com" {
		t.Fatalf("expected configured origin, got %q", got)
	}
}

func TestWithCORSHandlesPreflightWithoutCallingNext(t *testing.T) {
	called := false
	h := withCORS("", func(w http.ResponseWriter, r *http.Request) { called = true })
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodOptions, "/api/sentence/random", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if called {
		t.Fatal("next handler should not be called for an OPTIONS preflight")
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, Authorization" {
		t.Fatalf("expected Allow-Headers to include Authorization, got %q", got)
	}
}

func TestWithCORSPassesThroughNonPreflight(t *testing.T) {
	called := false
	h := withCORS("", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil))
	if !called {
		t.Fatal("expected the wrapped handler to be called for a non-preflight request")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test -run TestWithCORS ./...`
Expected: FAIL (compile error — `withCORS` undefined).

- [ ] **Step 3: Write the implementation**

Create `api/cors.go`:

```go
package main

import "net/http"

// withCORS wraps a handler with CORS headers. If allowedOrigin is empty
// (matching corgi's dev-only behavior when FRONTEND_URL is unset), it
// allows any origin; otherwise it restricts to exactly that origin.
// OPTIONS preflight requests are answered directly without invoking next,
// since a preflight has no Authorization header and must not hit auth.
func withCORS(allowedOrigin string, next http.HandlerFunc) http.HandlerFunc {
	origin := allowedOrigin
	if origin == "" {
		origin = "*"
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test -run TestWithCORS ./...`
Expected: PASS (4 tests).

- [ ] **Step 5: Wire into `main.go` and document `FRONTEND_URL`**

In `api/main.go`, read `FRONTEND_URL` (optional — unset means wildcard, matching
corgi) and wrap CORS **outside** `requireAuth`, since a browser's preflight
`OPTIONS` request carries no `Authorization` header and must be answered
before auth runs:

```go
frontendURL := os.Getenv("FRONTEND_URL")

auth := func(h http.HandlerFunc) http.HandlerFunc {
	return withCORS(frontendURL, requireAuth(verifier, allowedEmail, h))
}
```

(This replaces the existing `auth := func(h http.HandlerFunc) http.HandlerFunc { return requireAuth(verifier, allowedEmail, h) }` from Task 5.)

Update `api/.env.example` to document it, matching corgi's practice of
setting an explicit dev value rather than leaving it wildcard by default:

```
GOOGLE_CLOUD_PROJECT=your-gcp-project-id
ALLOWED_EMAIL=you@gmail.com
FRONTEND_URL=http://localhost:3000
PORT=8080
```

- [ ] **Step 6: Full test + vet + manual smoke test**

Run: `cd api && go vet ./... && go test -count=1 ./...`
Expected: PASS.

Manual (with both emulators running, `FRONTEND_URL` unset):
`curl -s -H "Origin: http://localhost:3000" -X OPTIONS -D - localhost:8080/api/sentence/random -o /dev/null`
Expected: `204`, with `Access-Control-Allow-Origin: *` and
`Access-Control-Allow-Headers: Content-Type, Authorization` in the response headers.

- [ ] **Step 7: Commit**

```bash
git add api/cors.go api/cors_test.go api/main.go api/.env.example
git commit -m "feat(api): reinstate CORS middleware, driven by FRONTEND_URL (matches corgi)"
```

---

## Verification (whole plan)

- [ ] `cd api && go vet ./... && go test ./...` passes (unit tests always; emulator tests when `FIRESTORE_EMULATOR_HOST` is set).
- [ ] `cd api && go build . && go build ./cmd/seed` both succeed.
- [ ] Manual: with both emulators running and 2+ seeded sentences, a request with a valid emulator ID token completes random → check → report; a request with no token returns 401.
- [ ] `git grep -n "go-sql-driver\|godotenv\|DB_ENDPOINT"` inside `api/` returns nothing (MySQL fully removed).
- [ ] `git grep -n "withCORS" api/main.go` shows both endpoints wrapped outside `requireAuth`.

## Notes for the next plans

- **Frontend-auth plan** must set `NEXT_PUBLIC_API_URL=""` in production, add the Firebase web SDK sign-in gate, and attach `Authorization: Bearer <ID token>` to the three fetch calls, plus `output: "export"` + `images: { unoptimized: true }` in `next.config.ts`.
- **No dev proxy is used or needed.** Next.js `rewrites()` cannot work with `output: 'export'` (errors in `next dev` too, per Next.js's static-export docs), so local dev talks directly to the Go API cross-origin — same as the pre-migration setup (`NEXT_PUBLIC_API_URL=http://localhost:8080` in dev via `.env.local`, unchanged). CORS is handled server-side by Task 7's `withCORS` middleware. Production stays same-origin via Hosting rewrites, and the frontend-auth/infra plans should set the Cloud Run `FRONTEND_URL` env var to the deployed Hosting URL once known.
- **Infra/deploy plan** must declare the composite index for collection `histories` `(is_correct ASC, created_at DESC)` in `firestore.indexes.json`, grant the Cloud Run service account `roles/datastore.user`, enable the Firebase Auth Google provider, export the live `sentences` table to `docs/sentences_export.ndjson` before running the seeder against prod, and set the Cloud Run `FRONTEND_URL` env var to the deployed Firebase Hosting URL.
- **`ALLOWED_EMAIL` must be set as a Cloud Run env var** (value `test@example.com`) — the API now hard-fails at startup if it's unset (`main.go` Task 5, Step 1). Matches the `corgi` project's single-user allowlist convention.
```
