# Mistakes Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a read-only "Mistakes" page listing every sentence the user has
ever answered incorrectly — Japanese sentence, correct English answer, and
all past wrong attempts — most recently missed sentence first, reachable
from a button on the main practice card.

**Architecture:** A new `GET /api/mistakes` endpoint reuses the existing
per-user Firestore layout (`users/{uid}/sentence_stats/{sentenceId}/histories`)
via a new `ListMistakes` repository method that mirrors `RandomCandidate`'s
existing fan-out-read pattern (load all of the user's stats docs, filter in
Go) rather than adding any new Firestore schema or index. The frontend adds
a new Next.js route `/mistakes` (same `AuthGate` pattern as the main page)
that fetches the list on mount and renders it as reflowing "stacked row"
cards (not a literal `<table>`, so it stays readable on an iPhone without
horizontal scrolling).

**Tech Stack:** Go (`net/http`, `cloud.google.com/go/firestore`), Next.js
15 (App Router, static export) + React 19 + TypeScript, Tailwind, Vitest +
Testing Library, Playwright (e2e), Firestore emulator (Go integration
tests).

## Global Constraints

- Every task follows red/green TDD: write the failing test, watch it fail,
  write minimal code to pass, keep tests green.
- JSON fields are `snake_case`, matching every existing endpoint
  (`sentence_id`, `correct_answer`, `incorrect_answer`, etc.).
- No new Firestore collections, fields, or indexes — reuse the existing
  `sentence_stats` / `histories` subcollection layout exactly as-is.
- No pagination and no per-row actions on the mistakes page — v1 is a
  simple read-only list (confirmed in
  `docs/superpowers/specs/2026-07-30-mistakes-page-design.md`).
- Frontend is a Next.js static export (`output: "export"` in
  `fe/next.config.ts`) — the new route must work as a plain static page,
  no server-only APIs.

---

## Task 1: Backend — `ListMistakes` repository method

**Files:**
- Modify: `api/internal/app/sentence.go` (add types + interface method)
- Modify: `api/internal/app/firestore_repo.go` (implementation + DRY refactor of the shared history query)
- Modify: `api/internal/app/handlers_test.go` (add a trivial `fakeRepo.ListMistakes` stub so the package keeps compiling — `NewServer` requires a full `SentenceRepository`)
- Test: `api/internal/app/firestore_repo_test.go`

**Interfaces:**
- Produces: `MistakeSentence{ SentenceID int; Japanese string; CorrectAnswer string; WrongAnswers []AnswerHistory }`, `ListMistakesResponse{ Mistakes []MistakeSentence }`, and `SentenceRepository.ListMistakes(ctx context.Context, uid string) ([]MistakeSentence, error)`. Task 2's handler calls this exact method.

- [ ] **Step 1: Add the new types and interface method to `sentence.go`**

Add these two types near `AnswerHistory` (after the existing `AnswerHistory` struct, sentence.go:20-24):

```go
type MistakeSentence struct {
	SentenceID    int             `json:"sentence_id"`
	Japanese      string          `json:"japanese"`
	CorrectAnswer string          `json:"correct_answer"`
	WrongAnswers  []AnswerHistory `json:"wrong_answers"`
}

type ListMistakesResponse struct {
	Mistakes []MistakeSentence `json:"mistakes"`
}
```

Add this method to the `SentenceRepository` interface (sentence.go:58-69), right after `ListIncorrectHistories`:

```go
	// ListMistakes returns every sentence the user has ever answered
	// incorrectly, most recently missed sentence first.
	ListMistakes(ctx context.Context, uid string) ([]MistakeSentence, error)
```

- [ ] **Step 2: Add a trivial `fakeRepo.ListMistakes` stub so the package compiles**

`api/internal/app/handlers_test.go` defines `fakeRepo`, which must satisfy
`SentenceRepository` for the whole `app` package (including
`firestore_repo_test.go`, in the same package) to compile. Add this stub
right after the existing `ListIncorrectHistories` method on `fakeRepo`
(handlers_test.go:45-50):

```go
func (f *fakeRepo) ListMistakes(_ context.Context, _ string) ([]MistakeSentence, error) {
	return []MistakeSentence{}, nil
}
```

- [ ] **Step 3: Run the full test suite to confirm it still compiles and passes**

Run: `cd api && go test ./...`
Expected: PASS (no new behavior yet — this just confirms the interface change didn't break anything)

- [ ] **Step 4: Write the failing Firestore integration tests**

Append to `api/internal/app/firestore_repo_test.go`:

```go
func TestFirestoreListMistakesGroupsWrongAnswersMostRecentSentenceFirst(t *testing.T) {
	ctx := context.Background()
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	uid := "user-mistakes"
	seedSentence(t, client, "701", "1", "時間がありません。", "I don't have time.", 1, false)
	seedSentence(t, client, "702", "1", "彼は毎朝走ります。", "He runs every morning.", 1, false)

	repo.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	if err := repo.RecordAnswer(ctx, uid, 701, false, "I have no time."); err != nil {
		t.Fatal(err)
	}
	repo.now = func() time.Time { return time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC) }
	if err := repo.RecordAnswer(ctx, uid, 702, false, "He run every morning."); err != nil {
		t.Fatal(err)
	}
	repo.now = func() time.Time { return time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC) }
	if err := repo.RecordAnswer(ctx, uid, 701, false, "There is no time."); err != nil {
		t.Fatal(err)
	}

	mistakes, err := repo.ListMistakes(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(mistakes) != 2 {
		t.Fatalf("expected 2 mistaken sentences, got %d: %+v", len(mistakes), mistakes)
	}
	// 701's most recent wrong answer (Jan 3) is newer than 702's (Jan 2), so 701 sorts first.
	if mistakes[0].SentenceID != 701 {
		t.Fatalf("expected sentence 701 first (most recent mistake), got %+v", mistakes)
	}
	if mistakes[0].Japanese != "時間がありません。" || mistakes[0].CorrectAnswer != "I don't have time." {
		t.Fatalf("unexpected sentence fields: %+v", mistakes[0])
	}
	if len(mistakes[0].WrongAnswers) != 2 {
		t.Fatalf("expected 2 wrong answers for 701, got %+v", mistakes[0].WrongAnswers)
	}
	if mistakes[0].WrongAnswers[0].IncorrectAnswer != "There is no time." {
		t.Fatalf("expected newest wrong answer first, got %+v", mistakes[0].WrongAnswers)
	}
	if mistakes[1].SentenceID != 702 {
		t.Fatalf("expected sentence 702 second, got %+v", mistakes[1])
	}
}

func TestFirestoreListMistakesExcludesSentencesNeverMissed(t *testing.T) {
	ctx := context.Background()
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	uid := "user-clean"
	seedSentence(t, client, "801", "1", "A", "A-en", 1, false)
	if err := repo.RecordAnswer(ctx, uid, 801, true, ""); err != nil {
		t.Fatal(err)
	}

	mistakes, err := repo.ListMistakes(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(mistakes) != 0 {
		t.Fatalf("expected no mistakes for an all-correct sentence, got %+v", mistakes)
	}
}

func TestFirestoreListMistakesSkipsDeletedSentence(t *testing.T) {
	ctx := context.Background()
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	uid := "user-deleted"
	seedSentence(t, client, "901", "1", "A", "A-en", 1, false)
	if err := repo.RecordAnswer(ctx, uid, 901, false, "wrong"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Collection("sentences").Doc("901").Delete(ctx); err != nil {
		t.Fatal(err)
	}

	mistakes, err := repo.ListMistakes(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(mistakes) != 0 {
		t.Fatalf("expected deleted sentence to be skipped, got %+v", mistakes)
	}
}
```

- [ ] **Step 5: Run the new tests to verify they fail**

Run: `cd api && FIRESTORE_EMULATOR_HOST=localhost:8081 go test ./internal/app/... -run TestFirestoreListMistakes -v`
(Start the Firestore emulator first if it isn't already running — check the
project's existing emulator startup command, e.g. via `firebase emulators:start`,
the same one the other `firestore_repo_test.go` tests rely on.)
Expected: FAIL with `repo.ListMistakes undefined` (method not implemented yet)

- [ ] **Step 6: Implement `ListMistakes` in `firestore_repo.go`**

First, extract the shared "incorrect histories for one stats doc" query out
of `ListIncorrectHistories` (firestore_repo.go:148-172) into a private
helper so both methods share it:

```go
func (r *firestoreRepo) incorrectHistories(ctx context.Context, statsRef *firestore.DocumentRef) ([]AnswerHistory, error) {
	it := statsRef.Collection("histories").
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

func (r *firestoreRepo) ListIncorrectHistories(ctx context.Context, uid string, id int) ([]AnswerHistory, error) {
	return r.incorrectHistories(ctx, r.userStats(uid).Doc(strconv.Itoa(id)))
}
```

This replaces the entire previous body of `ListIncorrectHistories`.

Then add `ListMistakes` (new method, place it after `ListIncorrectHistories`):

```go
func (r *firestoreRepo) ListMistakes(ctx context.Context, uid string) ([]MistakeSentence, error) {
	mistakes := make([]MistakeSentence, 0)
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
		if st.IncorrectCount == 0 {
			continue
		}

		sentDs, err := r.client.Collection("sentences").Doc(ds.Ref.ID).Get(ctx)
		if status.Code(err) == codes.NotFound {
			continue
		}
		if err != nil {
			return nil, err
		}
		var sd sentenceDoc
		if err := sentDs.DataTo(&sd); err != nil {
			return nil, err
		}

		wrongAnswers, err := r.incorrectHistories(ctx, ds.Ref)
		if err != nil {
			return nil, err
		}
		if len(wrongAnswers) == 0 {
			continue
		}

		mistakes = append(mistakes, MistakeSentence{
			SentenceID:    id,
			Japanese:      sd.Japanese,
			CorrectAnswer: sd.English,
			WrongAnswers:  wrongAnswers,
		})
	}

	sort.Slice(mistakes, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, mistakes[i].WrongAnswers[0].CreatedAt)
		tj, _ := time.Parse(time.RFC3339, mistakes[j].WrongAnswers[0].CreatedAt)
		return ti.After(tj)
	})
	return mistakes, nil
}
```

Add `"sort"` to the import block at the top of `firestore_repo.go`.

- [ ] **Step 7: Run the new tests to verify they pass**

Run: `cd api && FIRESTORE_EMULATOR_HOST=localhost:8081 go test ./internal/app/... -run TestFirestoreListMistakes -v`
Expected: PASS

- [ ] **Step 8: Run the full backend test suite (including the untouched `ListIncorrectHistories` tests) to confirm the refactor didn't break anything**

Run: `cd api && FIRESTORE_EMULATOR_HOST=localhost:8081 go test ./... -v`
Expected: PASS (in particular `TestFirestoreRecordListAndCount`, which still exercises `ListIncorrectHistories`)

- [ ] **Step 9: Commit**

```bash
git add api/internal/app/sentence.go api/internal/app/firestore_repo.go api/internal/app/handlers_test.go api/internal/app/firestore_repo_test.go
git commit -m "feat(api): add ListMistakes to the sentence repository"
```

---

## Task 2: Backend — `GET /api/mistakes` endpoint

**Files:**
- Modify: `api/internal/app/handlers.go` (add `getMistakes` handler)
- Modify: `api/internal/app/router.go` (register the route)
- Modify: `api/internal/app/handlers_test.go` (upgrade the `fakeRepo` stub from Task 1, add handler tests)

**Interfaces:**
- Consumes: `SentenceRepository.ListMistakes(ctx, uid) ([]MistakeSentence, error)` and `MistakeSentence` / `ListMistakesResponse` from Task 1.
- Produces: `GET /api/mistakes` → `200 { "mistakes": [...] }` (auth-gated, same as every other route).

- [ ] **Step 1: Upgrade the `fakeRepo` stub to support configurable results**

In `handlers_test.go`, add two fields to the `fakeRepo` struct (near the
other list-style fields, handlers_test.go:21-33):

```go
	mistakes    []MistakeSentence
	mistakesErr error
```

Replace the Task 1 stub with:

```go
func (f *fakeRepo) ListMistakes(_ context.Context, _ string) ([]MistakeSentence, error) {
	if f.mistakesErr != nil {
		return nil, f.mistakesErr
	}
	if f.mistakes == nil {
		return []MistakeSentence{}, nil
	}
	return f.mistakes, nil
}
```

- [ ] **Step 2: Write the failing handler tests**

Append to `handlers_test.go`:

```go
func TestGetMistakesOK(t *testing.T) {
	repo := &fakeRepo{mistakes: []MistakeSentence{
		{
			SentenceID:    701,
			Japanese:      "時間がありません。",
			CorrectAnswer: "I don't have time.",
			WrongAnswers: []AnswerHistory{
				{ID: 1, IncorrectAnswer: "I have no time.", CreatedAt: "2026-01-03T00:00:00Z"},
			},
		},
	}}
	srv := NewServer(repo, &fakeExplainer{})
	rec := httptest.NewRecorder()
	srv.getMistakes(rec, authed(httptest.NewRequest(http.MethodGet, "/api/mistakes", nil), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp ListMistakesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Mistakes) != 1 || resp.Mistakes[0].SentenceID != 701 {
		t.Fatalf("unexpected body: %+v", resp)
	}
	if len(resp.Mistakes[0].WrongAnswers) != 1 || resp.Mistakes[0].WrongAnswers[0].IncorrectAnswer != "I have no time." {
		t.Fatalf("unexpected wrong answers: %+v", resp.Mistakes[0])
	}
}

func TestGetMistakesEmpty(t *testing.T) {
	srv := NewServer(&fakeRepo{}, &fakeExplainer{})
	rec := httptest.NewRecorder()
	srv.getMistakes(rec, authed(httptest.NewRequest(http.MethodGet, "/api/mistakes", nil), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp ListMistakesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Mistakes == nil {
		t.Fatal("mistakes must not be null")
	}
	if len(resp.Mistakes) != 0 {
		t.Fatalf("expected empty list, got %+v", resp.Mistakes)
	}
}

func TestGetMistakesRepoError(t *testing.T) {
	srv := NewServer(&fakeRepo{mistakesErr: errors.New("boom")}, &fakeExplainer{})
	rec := httptest.NewRecorder()
	srv.getMistakes(rec, authed(httptest.NewRequest(http.MethodGet, "/api/mistakes", nil), "u1"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestGetMistakesMethodNotAllowed(t *testing.T) {
	srv := NewServer(&fakeRepo{}, &fakeExplainer{})
	rec := httptest.NewRecorder()
	srv.getMistakes(rec, authed(httptest.NewRequest(http.MethodPost, "/api/mistakes", nil), "u1"))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
```

- [ ] **Step 3: Run the new tests to verify they fail**

Run: `cd api && go test ./internal/app/... -run TestGetMistakes -v`
Expected: FAIL with `srv.getMistakes undefined`

- [ ] **Step 4: Implement the handler**

Add to `handlers.go`, after `checkAnswer` (handlers.go:73-113):

```go
func (s *Server) getMistakes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid, _ := uidFromContext(r.Context())
	mistakes, err := s.repo.ListMistakes(r.Context(), uid)
	if err != nil {
		log.Printf("list mistakes error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, ListMistakesResponse{Mistakes: mistakes})
}
```

- [ ] **Step 5: Register the route**

In `router.go`, add this line after the `/api/answer/check` registration (router.go:14):

```go
	mux.HandleFunc("/api/mistakes", auth(srv.getMistakes))
```

- [ ] **Step 6: Run the new tests to verify they pass**

Run: `cd api && go test ./internal/app/... -run TestGetMistakes -v`
Expected: PASS

- [ ] **Step 7: Run the full backend test suite**

Run: `cd api && go test ./... -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add api/internal/app/handlers.go api/internal/app/router.go api/internal/app/handlers_test.go
git commit -m "feat(api): add GET /api/mistakes endpoint"
```

---

## Task 3: Frontend — `api.listMistakes` client

**Files:**
- Modify: `fe/src/lib/api.ts`
- Test: `fe/src/lib/api.test.ts`

**Interfaces:**
- Consumes: `GET /api/mistakes` → `{ mistakes: Mistake[] }` from Task 2, where `Mistake` mirrors the backend's `MistakeSentence` JSON shape.
- Produces: `export interface Mistake { sentence_id: number; japanese: string; correct_answer: string; wrong_answers: AnswerHistory[] }` and `api.listMistakes(): Promise<{ mistakes: Mistake[] }>`. Task 4's `Mistakes` component consumes both.

- [ ] **Step 1: Write the failing test**

Append to `fe/src/lib/api.test.ts`:

```ts
describe('api.listMistakes', () => {
  it('sends GET /api/mistakes with the Authorization header', async () => {
    mockResponse({
      mistakes: [
        {
          sentence_id: 1,
          japanese: '時間がありません。',
          correct_answer: "I don't have time.",
          wrong_answers: [
            { id: 1, incorrect_answer: 'I have no time.', created_at: '2026-01-03T00:00:00Z' },
          ],
        },
      ],
    })
    const result = await api.listMistakes()
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/mistakes'),
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: 'Bearer test-token' }),
      })
    )
    expect(result.mistakes).toHaveLength(1)
    expect(result.mistakes[0].sentence_id).toBe(1)
    expect(result.mistakes[0].wrong_answers[0].incorrect_answer).toBe('I have no time.')
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd fe && npm test -- api.test.ts`
Expected: FAIL with `api.listMistakes is not a function`

- [ ] **Step 3: Implement in `api.ts`**

Add this type after the existing `AnswerHistory` interface (api.ts:15-19):

```ts
export interface Mistake {
  sentence_id: number
  japanese: string
  correct_answer: string
  wrong_answers: AnswerHistory[]
}
```

Add this method to the `api` object (after `reportSentence`, api.ts:60-64):

```ts
  listMistakes: () => request<{ mistakes: Mistake[] }>('/api/mistakes'),
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd fe && npm test -- api.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add fe/src/lib/api.ts fe/src/lib/api.test.ts
git commit -m "feat(fe): add listMistakes to the api client"
```

---

## Task 4: Frontend — Mistakes page and component

**Files:**
- Create: `fe/src/components/Mistakes.tsx`
- Create: `fe/src/app/mistakes/page.tsx`
- Test: `fe/src/components/Mistakes.test.tsx`

**Interfaces:**
- Consumes: `api.listMistakes()` and `Mistake` from Task 3; `AuthGate` from `fe/src/components/AuthGate.tsx` (existing, unchanged — `children: (user: User) => ReactNode`, called with no use of `user` here).
- Produces: default-exported `Mistakes` component (no props) and the `/mistakes` route. Task 5's link in `Translator.tsx` navigates to this route.

- [ ] **Step 1: Write the failing component tests**

Create `fe/src/components/Mistakes.test.tsx`:

```tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/lib/api', () => ({
  api: {
    listMistakes: vi.fn(),
  },
}))

import { api } from '@/lib/api'
import Mistakes from './Mistakes'

const mockApi = api as unknown as {
  listMistakes: ReturnType<typeof vi.fn>
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('Mistakes', () => {
  it('shows a loading state before the list resolves', () => {
    mockApi.listMistakes.mockReturnValue(new Promise(() => {}))
    render(<Mistakes />)
    expect(screen.getByText(/loading/i)).toBeInTheDocument()
  })

  it('renders each mistake with its correct answer and wrong answers', async () => {
    mockApi.listMistakes.mockResolvedValue({
      mistakes: [
        {
          sentence_id: 1,
          japanese: '時間がありません。',
          correct_answer: "I don't have time.",
          wrong_answers: [
            { id: 1, incorrect_answer: 'I have no time.', created_at: '2026-01-03T00:00:00Z' },
            { id: 2, incorrect_answer: 'There is no time.', created_at: '2026-01-01T00:00:00Z' },
          ],
        },
      ],
    })
    render(<Mistakes />)
    await screen.findByText('時間がありません。')
    expect(screen.getByText("I don't have time.")).toBeInTheDocument()
    expect(screen.getByText('I have no time.')).toBeInTheDocument()
    expect(screen.getByText('There is no time.')).toBeInTheDocument()
  })

  it('shows an empty state when there are no mistakes', async () => {
    mockApi.listMistakes.mockResolvedValue({ mistakes: [] })
    render(<Mistakes />)
    await screen.findByText(/no mistakes yet/i)
  })

  it('shows an error state with a working retry button', async () => {
    mockApi.listMistakes.mockRejectedValueOnce(new Error('boom'))
    render(<Mistakes />)
    await screen.findByText('boom')
    mockApi.listMistakes.mockResolvedValueOnce({ mistakes: [] })
    fireEvent.click(screen.getByRole('button', { name: /try again/i }))
    await screen.findByText(/no mistakes yet/i)
  })
})
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd fe && npm test -- Mistakes.test.tsx`
Expected: FAIL (`Failed to resolve import "./Mistakes"` — the component doesn't exist yet)

- [ ] **Step 3: Implement `Mistakes.tsx`**

Create `fe/src/components/Mistakes.tsx`:

```tsx
'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { api, type Mistake } from '@/lib/api'

export default function Mistakes() {
  const [mistakes, setMistakes] = useState<Mistake[] | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const loadMistakes = async () => {
    try {
      setLoading(true)
      setError(null)
      const result = await api.listMistakes()
      setMistakes(result.mistakes)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load mistakes')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadMistakes()
  }, [])

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 p-4">
      <div className="max-w-2xl mx-auto">
        <div className="flex items-center gap-3 mb-6">
          <Button asChild variant="outline" size="sm">
            <Link href="/">&larr; Back</Link>
          </Button>
          <h1 className="text-2xl font-bold text-gray-900">Mistakes</h1>
        </div>

        {loading ? (
          <div className="text-center">
            <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mx-auto mb-4"></div>
            <p className="text-gray-600">Loading...</p>
          </div>
        ) : error ? (
          <Card>
            <CardContent className="pt-6">
              <p className="text-gray-700 mb-4">{error}</p>
              <Button onClick={loadMistakes} className="w-full">
                Try Again
              </Button>
            </CardContent>
          </Card>
        ) : mistakes && mistakes.length === 0 ? (
          <Card>
            <CardContent className="pt-6 text-center text-gray-600">
              No mistakes yet — nice work!
            </CardContent>
          </Card>
        ) : (
          <div className="space-y-3">
            {mistakes?.map(mistake => (
              <Card key={mistake.sentence_id}>
                <CardHeader className="pb-2">
                  <CardTitle className="text-base">{mistake.japanese}</CardTitle>
                </CardHeader>
                <CardContent className="space-y-2">
                  <div className="inline-block text-sm text-blue-900 bg-blue-50 border border-blue-200 rounded-md px-2 py-1">
                    {mistake.correct_answer}
                  </div>
                  <div className="flex flex-wrap gap-1.5">
                    {mistake.wrong_answers.map(wrong => (
                      <span
                        key={wrong.id}
                        className="text-xs text-yellow-900 bg-yellow-50 border border-yellow-200 rounded-md px-2 py-1"
                      >
                        {wrong.incorrect_answer}
                      </span>
                    ))}
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
```

Create `fe/src/app/mistakes/page.tsx`:

```tsx
'use client'

import AuthGate from '@/components/AuthGate'
import Mistakes from '@/components/Mistakes'

export default function Page() {
  return <AuthGate>{() => <Mistakes />}</AuthGate>
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd fe && npm test -- Mistakes.test.tsx`
Expected: PASS

- [ ] **Step 5: Run the full frontend test suite**

Run: `cd fe && npm test`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add fe/src/components/Mistakes.tsx fe/src/components/Mistakes.test.tsx fe/src/app/mistakes/page.tsx
git commit -m "feat(fe): add mistakes page and component"
```

---

## Task 5: Frontend — link to the mistakes page from the practice card

**Files:**
- Modify: `fe/src/components/Translator.tsx`
- Modify: `fe/src/components/Translator.test.tsx`

**Interfaces:**
- Consumes: the `/mistakes` route from Task 4 (link target only, no data dependency).

- [ ] **Step 1: Write the failing test**

Add this `describe` block to `fe/src/components/Translator.test.tsx`, near
the other top-level `describe` blocks:

```tsx
describe('Mistakes link', () => {
  it('links to the mistakes page', async () => {
    render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    const link = screen.getByRole('link', { name: /mistakes/i })
    expect(link).toHaveAttribute('href', '/mistakes')
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd fe && npm test -- Translator.test.tsx -t "Mistakes link"`
Expected: FAIL (`Unable to find role="link"`)

- [ ] **Step 3: Add the link**

In `Translator.tsx`, add the import (near the other imports, Translator.tsx:1-20):

```tsx
import Link from 'next/link'
```

Replace the `CardHeader` block (Translator.tsx:318-324):

```tsx
            <CardHeader className="flex flex-row items-start justify-between space-y-0">
              <div>
                <CardTitle>Translate this sentence</CardTitle>
                <CardDescription>Translate the Japanese sentence below into English</CardDescription>
              </div>
              {levelMenu}
            </CardHeader>
```

with:

```tsx
            <CardHeader className="flex flex-row items-start justify-between space-y-0">
              <div>
                <CardTitle>Translate this sentence</CardTitle>
                <CardDescription>Translate the Japanese sentence below into English</CardDescription>
              </div>
              <div className="flex items-center gap-2">
                <Button asChild variant="outline" size="sm">
                  <Link href="/mistakes">Mistakes</Link>
                </Button>
                {levelMenu}
              </div>
            </CardHeader>
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd fe && npm test -- Translator.test.tsx -t "Mistakes link"`
Expected: PASS

- [ ] **Step 5: Run the full frontend test suite**

Run: `cd fe && npm test`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add fe/src/components/Translator.tsx fe/src/components/Translator.test.tsx
git commit -m "feat(fe): link to the mistakes page from the practice card"
```

---

## Task 6: E2E — cover the mistakes page

**Files:**
- Create: `e2e/tests/mistakes.spec.ts`

**Interfaces:**
- Consumes: `signInAndGetSentence` from `e2e/tests/helpers.ts` (existing), the "Mistakes" link from Task 5, and the `/mistakes` page from Task 4.

- [ ] **Step 1: Write the e2e test**

Create `e2e/tests/mistakes.spec.ts`:

```ts
import { test, expect } from '@playwright/test'
import { signInAndGetSentence } from './helpers'

test('an incorrect answer shows up on the mistakes page', async ({ page }) => {
  const sentence = await signInAndGetSentence(page)

  await page.getByLabel('Your English translation').fill('This is definitely the wrong answer')
  await page.getByRole('button', { name: 'Check Translation' }).click()
  await expect(page.getByText('Not quite right. Try again!')).toBeVisible()

  await page.getByRole('link', { name: 'Mistakes' }).click()
  await expect(page.getByRole('heading', { name: 'Mistakes' })).toBeVisible()
  await expect(page.getByText(sentence.japanese)).toBeVisible()
  await expect(page.getByText(sentence.english)).toBeVisible()
  await expect(page.getByText('This is definitely the wrong answer')).toBeVisible()

  await page.getByRole('link', { name: /back/i }).click()
  await expect(page.getByRole('heading', { name: 'Eagle' })).toBeVisible()
})
```

This is a new, independent test file — there's no prior failing/passing
state to check beyond running it against the finished feature.

- [ ] **Step 2: Run the e2e suite**

Run: `cd e2e && npx playwright test mistakes.spec.ts`
Expected: PASS (this exercises the real Go API against the Firestore
emulator and the built static frontend, per `e2e/playwright.config.ts`)

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/mistakes.spec.ts
git commit -m "test(e2e): cover the mistakes page"
```
