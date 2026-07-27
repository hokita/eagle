# Multi-Level Sentence Filter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the single-level `<select>` difficulty filter with independently toggleable levels 1-5, so a learner can practice any combination of levels at once, persisted across sessions.

**Architecture:** The Go backend's `SentenceRepository.RandomCandidate` moves from a single `level int` parameter to a `levels []int` set-membership filter; `GET /api/sentence/random?level=N` becomes `?levels=1,3,5` (comma-separated). The Next.js frontend replaces the single `<select>` with five toggle `Button`s backed by a `selectedLevels: number[]` state, persisted to `localStorage`.

**Tech Stack:** Go backend (`eagle/api`) with the existing `SentenceRepository`/Firestore seam; Next.js/React frontend (`eagle/fe`) using the existing `fetch`-based `api.ts` client and shadcn-style `Button` component; Go `testing` (+ Firestore emulator for repo-layer tests) and Vitest + React Testing Library for frontend tests; Playwright for e2e.

## Global Constraints

- `levels` query param is a comma-separated list of ints; each value must be in 1-5, else `400 Bad Request`. Duplicate values are deduped.
- An absent/empty `levels` param, or a frontend selection of zero or all five levels, means "any level" (no filter) — this must include legacy sentences with no `Level` set (`Level == 0`), exactly like today's `level=0`/absent behavior.
- A non-empty, non-all subset filters strictly to sentences whose `Level` is in that set (unleveled docs excluded), matching today's single-level behavior.
- No change to the sentence data model or `cmd/seed` — each sentence still has exactly one `Level` on import.
- Level selection is persisted client-side only, via `localStorage` key `eagle:selectedLevels` — no server-side/account persistence.
- Toggle buttons reuse the existing `Button` component (`variant="default"` selected / `"outline"` unselected) — no new UI primitive.

---

### Task 1: Repo layer — `RandomCandidate` accepts a set of levels

**Files:**
- Modify: `api/internal/app/sentence.go`
- Modify: `api/internal/app/firestore_repo.go`
- Modify: `api/internal/app/firestore_repo_test.go`
- Modify: `api/internal/app/handlers.go`
- Modify: `api/internal/app/handlers_test.go`

**Interfaces:**
- Produces: `SentenceRepository.RandomCandidate(ctx context.Context, uid string, levels []int) (*Sentence, error)` — an empty/nil `levels` means "any level". Consumed by Task 2 (handler's new comma-separated query parsing).

- [ ] **Step 1: Write the failing test**

In `api/internal/app/firestore_repo_test.go`, add this test at the end of the file:

```go
func TestFirestoreRandomCandidateFiltersByMultipleLevels(t *testing.T) {
	ctx := context.Background()
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	uid := "user-multilevel"
	seedSentence(t, client, "601", "1", "A", "A", 1, false)
	seedSentence(t, client, "602", "1", "B", "B", 3, false)
	seedSentence(t, client, "603", "1", "C", "C", 5, false)
	// Unleveled doc (pre-backfill) must not match an explicit multi-level subset.
	_, err := client.Collection("sentences").Doc("604").Set(ctx, map[string]interface{}{
		"japanese": "D", "english": "D", "page": "1", "is_reported": false,
		"created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("seed unleveled sentence: %v", err)
	}

	for i := 0; i < 8; i++ {
		s, err := repo.RandomCandidate(ctx, uid, []int{1, 3})
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if s.ID != 601 && s.ID != 602 {
			t.Fatalf("expected level-1 or level-3 sentence, got %d", s.ID)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./... -run TestFirestoreRandomCandidateFiltersByMultipleLevels -v`
Expected: FAIL to compile — `cannot use []int{1, 3} (untyped slice literal) as int value in argument to repo.RandomCandidate`

- [ ] **Step 3: Write minimal implementation**

In `api/internal/app/sentence.go`, replace:

```go
	// RandomCandidate returns a random non-mastered, non-reported sentence.
	// level selects only sentences with that difficulty (1-5); level == 0
	// means "any level" (no filtering), including sentences with no level set.
	RandomCandidate(ctx context.Context, uid string, level int) (*Sentence, error)
```

with:

```go
	// RandomCandidate returns a random non-mastered, non-reported sentence.
	// levels restricts candidates to sentences whose Level is in the set;
	// an empty levels means "any level" (no filtering), including sentences
	// with no level set.
	RandomCandidate(ctx context.Context, uid string, levels []int) (*Sentence, error)
```

In `api/internal/app/firestore_repo.go`, replace the function signature:

```go
func (r *firestoreRepo) RandomCandidate(ctx context.Context, uid string, level int) (*Sentence, error) {
```

with:

```go
func (r *firestoreRepo) RandomCandidate(ctx context.Context, uid string, levels []int) (*Sentence, error) {
```

In the same function, replace:

```go
	var candidates []*Sentence
	for _, ds := range sentenceDocs {
```

with:

```go
	wantLevels := map[int]bool{}
	for _, lv := range levels {
		wantLevels[lv] = true
	}

	var candidates []*Sentence
	for _, ds := range sentenceDocs {
```

and replace:

```go
		if level != 0 && sd.Level != level {
			continue
		}
```

with:

```go
		if len(wantLevels) > 0 && !wantLevels[sd.Level] {
			continue
		}
```

In `api/internal/app/firestore_repo_test.go`, update the existing calls to match the new signature:

- `TestFirestoreRecordListAndCount`: `s, err := repo.RandomCandidate(ctx, uid, 0)` → `s, err := repo.RandomCandidate(ctx, uid, nil)`
- `TestFirestoreRandomExcludesMasteredAndReported`: `s, err := repo.RandomCandidate(ctx, uid, 0)` → `s, err := repo.RandomCandidate(ctx, uid, nil)`
- `TestFirestoreRandomCandidateFiltersByLevel`: `s, err := repo.RandomCandidate(ctx, uid, 1)` → `s, err := repo.RandomCandidate(ctx, uid, []int{1})`, and `s, err := repo.RandomCandidate(ctx, uid, 3)` → `s, err := repo.RandomCandidate(ctx, uid, []int{3})`
- `TestFirestoreRandomCandidateLevelZeroIncludesUnleveledDocs`: `s, err := repo.RandomCandidate(ctx, uid, 0)` → `s, err := repo.RandomCandidate(ctx, uid, nil)`, and `if _, err := repo.RandomCandidate(ctx, uid, 2); err != ErrNoCandidate` → `if _, err := repo.RandomCandidate(ctx, uid, []int{2}); err != ErrNoCandidate`

In `api/internal/app/handlers.go`, in `getRandomSentence`, replace:

```go
	sentence, err := s.repo.RandomCandidate(r.Context(), uid, level)
```

with:

```go
	var levels []int
	if level != 0 {
		levels = []int{level}
	}
	sentence, err := s.repo.RandomCandidate(r.Context(), uid, levels)
```

(This is a temporary adapter that keeps the existing single `?level=N` HTTP contract working unchanged. Task 2 replaces this whole block with real multi-value parsing.)

In `api/internal/app/handlers_test.go`, replace the `fakeRepo` struct field and method:

```go
type fakeRepo struct {
	random           *Sentence
	randomErr        error
	randomLevels     []int
	correct          string
	correctErr       error
	sentenceJapanese string
	sentenceEnglish  string
	sentenceErr      error
	histories        []AnswerHistory
	recorded         []recordedAnswer
	reported         []int
}

func (f *fakeRepo) RandomCandidate(_ context.Context, _ string, level int) (*Sentence, error) {
	f.randomLevels = append(f.randomLevels, level)
	return f.random, f.randomErr
}
```

with:

```go
type fakeRepo struct {
	random           *Sentence
	randomErr        error
	randomLevelCalls [][]int
	correct          string
	correctErr       error
	sentenceJapanese string
	sentenceEnglish  string
	sentenceErr      error
	histories        []AnswerHistory
	recorded         []recordedAnswer
	reported         []int
}

func (f *fakeRepo) RandomCandidate(_ context.Context, _ string, levels []int) (*Sentence, error) {
	f.randomLevelCalls = append(f.randomLevelCalls, levels)
	return f.random, f.randomErr
}
```

Then update the three tests that reference the old field:

`TestGetRandomSentencePassesLevelToRepo`, replace:

```go
	if len(repo.randomLevels) != 1 || repo.randomLevels[0] != 3 {
		t.Fatalf("expected repo called with level 3, got %v", repo.randomLevels)
	}
```

with:

```go
	if len(repo.randomLevelCalls) != 1 || len(repo.randomLevelCalls[0]) != 1 || repo.randomLevelCalls[0][0] != 3 {
		t.Fatalf("expected repo called with levels [3], got %v", repo.randomLevelCalls)
	}
```

`TestGetRandomSentenceNoLevelDefaultsToZero`, replace:

```go
	if len(repo.randomLevels) != 1 || repo.randomLevels[0] != 0 {
		t.Fatalf("expected repo called with level 0, got %v", repo.randomLevels)
	}
```

with:

```go
	if len(repo.randomLevelCalls) != 1 || len(repo.randomLevelCalls[0]) != 0 {
		t.Fatalf("expected repo called with no levels, got %v", repo.randomLevelCalls)
	}
```

`TestGetRandomSentenceInvalidLevel`, replace:

```go
			if len(repo.randomLevels) != 0 {
				t.Fatalf("expected repo not called for invalid level=%q, got %v", level, repo.randomLevels)
			}
```

with:

```go
			if len(repo.randomLevelCalls) != 0 {
				t.Fatalf("expected repo not called for invalid level=%q, got %v", level, repo.randomLevelCalls)
			}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `gcloud emulators firestore start --host-port=localhost:8090 &`
Run: `export FIRESTORE_EMULATOR_HOST=localhost:8090`
Run: `cd api && go test ./... -v`
Expected: PASS (all tests, including the new multi-level test and the pre-existing ones). Then stop the emulator (`kill %1`) or leave it running for Task 2.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/sentence.go api/internal/app/firestore_repo.go api/internal/app/firestore_repo_test.go api/internal/app/handlers.go api/internal/app/handlers_test.go
git commit -m "refactor(api): RandomCandidate filters by a set of levels"
```

---

### Task 2: Handler — comma-separated `levels` query param

**Files:**
- Modify: `api/internal/app/handlers.go`
- Modify: `api/internal/app/handlers_test.go`

**Interfaces:**
- Consumes: `SentenceRepository.RandomCandidate(ctx, uid, levels []int)` from Task 1.
- Produces: `GET /api/sentence/random?levels=1,3,5` HTTP contract — consumed by Task 3 (frontend `api.ts`).

- [ ] **Step 1: Write the failing tests**

In `api/internal/app/handlers_test.go`, replace `TestGetRandomSentencePassesLevelToRepo`:

```go
func TestGetRandomSentencePassesLevelToRepo(t *testing.T) {
	repo := &fakeRepo{random: &Sentence{ID: 7}}
	srv := NewServer(repo, &fakeExplainer{})
	rec := httptest.NewRecorder()
	srv.getRandomSentence(rec, authed(httptest.NewRequest(http.MethodGet, "/api/sentence/random?level=3", nil), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(repo.randomLevelCalls) != 1 || len(repo.randomLevelCalls[0]) != 1 || repo.randomLevelCalls[0][0] != 3 {
		t.Fatalf("expected repo called with levels [3], got %v", repo.randomLevelCalls)
	}
}
```

with:

```go
func TestGetRandomSentencePassesLevelsToRepo(t *testing.T) {
	repo := &fakeRepo{random: &Sentence{ID: 7}}
	srv := NewServer(repo, &fakeExplainer{})
	rec := httptest.NewRecorder()
	srv.getRandomSentence(rec, authed(httptest.NewRequest(http.MethodGet, "/api/sentence/random?levels=3", nil), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(repo.randomLevelCalls) != 1 || len(repo.randomLevelCalls[0]) != 1 || repo.randomLevelCalls[0][0] != 3 {
		t.Fatalf("expected repo called with levels [3], got %v", repo.randomLevelCalls)
	}
}

func TestGetRandomSentencePassesMultipleLevelsToRepo(t *testing.T) {
	repo := &fakeRepo{random: &Sentence{ID: 7}}
	srv := NewServer(repo, &fakeExplainer{})
	rec := httptest.NewRecorder()
	srv.getRandomSentence(rec, authed(httptest.NewRequest(http.MethodGet, "/api/sentence/random?levels=1,3,5", nil), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(repo.randomLevelCalls) != 1 {
		t.Fatalf("expected repo called once, got %d calls", len(repo.randomLevelCalls))
	}
	if got := repo.randomLevelCalls[0]; len(got) != 3 || got[0] != 1 || got[1] != 3 || got[2] != 5 {
		t.Fatalf("expected repo called with levels [1 3 5], got %v", got)
	}
}

func TestGetRandomSentenceDedupesLevels(t *testing.T) {
	repo := &fakeRepo{random: &Sentence{ID: 7}}
	srv := NewServer(repo, &fakeExplainer{})
	rec := httptest.NewRecorder()
	srv.getRandomSentence(rec, authed(httptest.NewRequest(http.MethodGet, "/api/sentence/random?levels=2,2,3", nil), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := repo.randomLevelCalls[0]; len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("expected deduped levels [2 3], got %v", got)
	}
}
```

Replace `TestGetRandomSentenceNoLevelDefaultsToZero`:

```go
func TestGetRandomSentenceNoLevelDefaultsToZero(t *testing.T) {
	repo := &fakeRepo{random: &Sentence{ID: 7}}
	srv := NewServer(repo, &fakeExplainer{})
	rec := httptest.NewRecorder()
	srv.getRandomSentence(rec, authed(httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(repo.randomLevelCalls) != 1 || len(repo.randomLevelCalls[0]) != 0 {
		t.Fatalf("expected repo called with no levels, got %v", repo.randomLevelCalls)
	}
}
```

with:

```go
func TestGetRandomSentenceNoLevelsDefaultsToAny(t *testing.T) {
	repo := &fakeRepo{random: &Sentence{ID: 7}}
	srv := NewServer(repo, &fakeExplainer{})
	rec := httptest.NewRecorder()
	srv.getRandomSentence(rec, authed(httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(repo.randomLevelCalls) != 1 || len(repo.randomLevelCalls[0]) != 0 {
		t.Fatalf("expected repo called with no levels, got %v", repo.randomLevelCalls)
	}
}
```

Replace `TestGetRandomSentenceInvalidLevel`:

```go
func TestGetRandomSentenceInvalidLevel(t *testing.T) {
	for _, level := range []string{"0", "6", "-1", "abc", "3.5"} {
		t.Run(level, func(t *testing.T) {
			repo := &fakeRepo{random: &Sentence{ID: 7}}
			srv := NewServer(repo, &fakeExplainer{})
			rec := httptest.NewRecorder()
			srv.getRandomSentence(rec, authed(httptest.NewRequest(http.MethodGet, "/api/sentence/random?level="+level, nil), "u1"))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for level=%q, got %d", level, rec.Code)
			}
			if len(repo.randomLevelCalls) != 0 {
				t.Fatalf("expected repo not called for invalid level=%q, got %v", level, repo.randomLevelCalls)
			}
		})
	}
}
```

with:

```go
func TestGetRandomSentenceInvalidLevels(t *testing.T) {
	for _, levels := range []string{"0", "6", "-1", "abc", "3.5", "1,6", "1,abc", "1,,3"} {
		t.Run(levels, func(t *testing.T) {
			repo := &fakeRepo{random: &Sentence{ID: 7}}
			srv := NewServer(repo, &fakeExplainer{})
			rec := httptest.NewRecorder()
			srv.getRandomSentence(rec, authed(httptest.NewRequest(http.MethodGet, "/api/sentence/random?levels="+levels, nil), "u1"))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for levels=%q, got %d", levels, rec.Code)
			}
			if len(repo.randomLevelCalls) != 0 {
				t.Fatalf("expected repo not called for invalid levels=%q, got %v", levels, repo.randomLevelCalls)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./... -run TestGetRandomSentence -v`
Expected: FAIL — the handler still only reads the `level` query key (singular), so `?levels=...` is never parsed at all: it falls through to "no levels" and calls the repo with `nil`. `TestGetRandomSentencePassesMultipleLevelsToRepo` and `TestGetRandomSentenceDedupesLevels` fail because the repo is called with an empty slice instead of the expected levels, and `TestGetRandomSentenceInvalidLevels`'s `"1,6"` / `"1,abc"` / `"1,,3"` cases fail because they get `200` (repo called with `nil`) instead of the expected `400`.

- [ ] **Step 3: Write minimal implementation**

In `api/internal/app/handlers.go`, replace the whole level-parsing block in `getRandomSentence`:

```go
	uid, _ := uidFromContext(r.Context())
	level := 0
	if raw := r.URL.Query().Get("level"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 5 {
			http.Error(w, "Invalid level parameter", http.StatusBadRequest)
			return
		}
		level = n
	}
	var levels []int
	if level != 0 {
		levels = []int{level}
	}
	sentence, err := s.repo.RandomCandidate(r.Context(), uid, levels)
```

with:

```go
	uid, _ := uidFromContext(r.Context())
	var levels []int
	if raw := r.URL.Query().Get("levels"); raw != "" {
		seen := map[int]bool{}
		for _, part := range strings.Split(raw, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil || n < 1 || n > 5 {
				http.Error(w, "Invalid levels parameter", http.StatusBadRequest)
				return
			}
			if !seen[n] {
				seen[n] = true
				levels = append(levels, n)
			}
		}
	}
	sentence, err := s.repo.RandomCandidate(r.Context(), uid, levels)
```

(`strconv` and `strings` are already imported in this file.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./... -v`
Expected: PASS (all tests)

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/handlers.go api/internal/app/handlers_test.go
git commit -m "feat(api): support comma-separated levels query param on /api/sentence/random"
```

---

### Task 3: Frontend API client — multi-level query param

**Files:**
- Modify: `fe/src/lib/api.ts`
- Modify: `fe/src/lib/api.test.ts`

**Interfaces:**
- Consumes: `GET /api/sentence/random?levels=1,3` contract from Task 2.
- Produces: `api.getRandomSentence(levels?: number[]): Promise<Sentence>` — consumed by Task 4 (`Translator.tsx`).

- [ ] **Step 1: Write the failing test**

In `fe/src/lib/api.test.ts`, replace the `describe('api.getRandomSentence', ...)` block:

```ts
describe('api.getRandomSentence', () => {
  it('sends GET /api/sentence/random with the Authorization header', async () => {
    mockResponse({
      id: 1,
      japanese: '時間がありません。',
      english: "I don't have time.",
      page: '12',
      level: 2,
      correct_count: 0,
      incorrect_count: 0,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    })
    const result = await api.getRandomSentence()
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/sentence/random'),
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: 'Bearer test-token' }),
      })
    )
    expect(result.id).toBe(1)
    expect(result.english).toBe("I don't have time.")
  })

  it('omits the level query param when no level is given', async () => {
    mockResponse({ id: 1 })
    await api.getRandomSentence()
    const [url] = mockFetch.mock.calls[0]
    expect(url).not.toContain('level=')
  })

  it('sends the level query param when a level is given', async () => {
    mockResponse({ id: 1 })
    await api.getRandomSentence(3)
    const [url] = mockFetch.mock.calls[0]
    expect(url).toContain('/api/sentence/random?level=3')
  })
})
```

with:

```ts
describe('api.getRandomSentence', () => {
  it('sends GET /api/sentence/random with the Authorization header', async () => {
    mockResponse({
      id: 1,
      japanese: '時間がありません。',
      english: "I don't have time.",
      page: '12',
      level: 2,
      correct_count: 0,
      incorrect_count: 0,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    })
    const result = await api.getRandomSentence()
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/sentence/random'),
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: 'Bearer test-token' }),
      })
    )
    expect(result.id).toBe(1)
    expect(result.english).toBe("I don't have time.")
  })

  it('omits the levels query param when no levels are given', async () => {
    mockResponse({ id: 1 })
    await api.getRandomSentence()
    const [url] = mockFetch.mock.calls[0]
    expect(url).not.toContain('levels=')
  })

  it('omits the levels query param when given an empty array', async () => {
    mockResponse({ id: 1 })
    await api.getRandomSentence([])
    const [url] = mockFetch.mock.calls[0]
    expect(url).not.toContain('levels=')
  })

  it('sends the levels query param when levels are given', async () => {
    mockResponse({ id: 1 })
    await api.getRandomSentence([1, 3])
    const [url] = mockFetch.mock.calls[0]
    expect(url).toContain('/api/sentence/random?levels=1,3')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd fe && npx vitest run src/lib/api.test.ts`
Expected: FAIL — `api.getRandomSentence([1, 3])` still builds a `?level=` URL (or a type error if TypeScript is checked), and the new empty-array test fails since the current implementation treats `0`/falsy checks differently than an array.

- [ ] **Step 3: Write minimal implementation**

In `fe/src/lib/api.ts`, replace:

```ts
  getRandomSentence: (level?: number) =>
    request<Sentence>(`/api/sentence/random${level ? `?level=${level}` : ''}`),
```

with:

```ts
  getRandomSentence: (levels?: number[]) =>
    request<Sentence>(
      `/api/sentence/random${levels && levels.length > 0 ? `?levels=${levels.join(',')}` : ''}`
    ),
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd fe && npx vitest run src/lib/api.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add fe/src/lib/api.ts fe/src/lib/api.test.ts
git commit -m "feat(fe): api.getRandomSentence accepts multiple levels"
```

---

### Task 4: Translator — level toggle buttons with persisted selection

**Files:**
- Modify: `fe/src/components/Translator.tsx`
- Modify: `fe/src/components/Translator.test.tsx`

**Interfaces:**
- Consumes: `api.getRandomSentence(levels?: number[])` from Task 3.

- [ ] **Step 1: Write the failing tests**

In `fe/src/components/Translator.test.tsx`, add `localStorage.clear()` to the top-level `beforeEach`:

```tsx
beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  mockApi.getRandomSentence.mockResolvedValue(fakeSentence)
})
```

Replace the entire `describe('Level selector', ...)` block:

```tsx
describe('Level selector', () => {
  it('defaults to "Any level" and fetches with no level filter on mount', async () => {
    render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    expect(screen.getByLabelText(/sentence difficulty level/i)).toHaveValue('0')
    expect(mockApi.getRandomSentence).toHaveBeenCalledWith(0)
  })

  it('offers levels 1 through 5', async () => {
    render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    const select = screen.getByLabelText(/sentence difficulty level/i)
    const values = Array.from(select.querySelectorAll('option')).map(o => (o as HTMLOptionElement).value)
    expect(values).toEqual(['0', '1', '2', '3', '4', '5'])
  })

  it('refetches with the chosen level when the user picks one', async () => {
    render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    const otherSentence = { ...fakeSentence, id: 2, japanese: '違う文です。', level: 3 }
    mockApi.getRandomSentence.mockResolvedValueOnce(otherSentence)
    fireEvent.change(screen.getByLabelText(/sentence difficulty level/i), { target: { value: '3' } })
    await screen.findByText('違う文です。')
    expect(mockApi.getRandomSentence).toHaveBeenLastCalledWith(3)
  })

  it('resets in-progress answer state when the level changes', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    await answerIncorrectly()
    mockApi.getRandomSentence.mockResolvedValueOnce({ ...fakeSentence, id: 3, level: 1 })
    fireEvent.change(screen.getByLabelText(/sentence difficulty level/i), { target: { value: '1' } })
    await screen.findByLabelText(/your english translation/i)
    expect(screen.queryByText(/not quite right/i)).not.toBeInTheDocument()
    expect(screen.getByLabelText(/your english translation/i)).toHaveValue('')
  })

  it('stays visible and interactive when the chosen level has no candidates', async () => {
    render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    mockApi.getRandomSentence.mockRejectedValueOnce(new Error('API error: 404'))
    fireEvent.change(screen.getByLabelText(/sentence difficulty level/i), { target: { value: '4' } })
    await screen.findByText(/api error: 404/i)
    expect(screen.getByLabelText(/sentence difficulty level/i)).toBeEnabled()
  })
})
```

with:

```tsx
describe('Level toggles', () => {
  it('defaults to all levels selected and fetches with no level filter on mount', async () => {
    render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    for (const n of [1, 2, 3, 4, 5]) {
      expect(screen.getByRole('button', { name: `Level ${n}` })).toHaveAttribute('aria-pressed', 'true')
    }
    expect(mockApi.getRandomSentence).toHaveBeenCalledWith(undefined)
  })

  it('narrows the filter and persists the selection when a level is toggled off', async () => {
    render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    const otherSentence = { ...fakeSentence, id: 2, japanese: '違う文です。', level: 3 }
    mockApi.getRandomSentence.mockResolvedValueOnce(otherSentence)
    fireEvent.click(screen.getByRole('button', { name: 'Level 1' }))
    await screen.findByText('違う文です。')
    expect(mockApi.getRandomSentence).toHaveBeenLastCalledWith([2, 3, 4, 5])
    expect(localStorage.getItem('eagle:selectedLevels')).toBe(JSON.stringify([2, 3, 4, 5]))
  })

  it('treats deselecting every level the same as selecting them all (any level)', async () => {
    render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    for (const n of [1, 2, 3, 4, 5]) {
      mockApi.getRandomSentence.mockResolvedValueOnce(fakeSentence)
      fireEvent.click(screen.getByRole('button', { name: `Level ${n}` }))
      await screen.findByText(fakeSentence.japanese)
    }
    expect(mockApi.getRandomSentence).toHaveBeenLastCalledWith(undefined)
  })

  it('restores a previously persisted selection on mount', async () => {
    localStorage.setItem('eagle:selectedLevels', JSON.stringify([2, 4]))
    render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    expect(screen.getByRole('button', { name: 'Level 2' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: 'Level 1' })).toHaveAttribute('aria-pressed', 'false')
    expect(mockApi.getRandomSentence).toHaveBeenCalledWith([2, 4])
  })

  it('resets in-progress answer state when a level is toggled', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    await answerIncorrectly()
    mockApi.getRandomSentence.mockResolvedValueOnce({ ...fakeSentence, id: 3, level: 1 })
    fireEvent.click(screen.getByRole('button', { name: 'Level 1' }))
    await screen.findByLabelText(/your english translation/i)
    expect(screen.queryByText(/not quite right/i)).not.toBeInTheDocument()
    expect(screen.getByLabelText(/your english translation/i)).toHaveValue('')
  })

  it('stays visible and interactive when the narrowed selection has no candidates', async () => {
    render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    mockApi.getRandomSentence.mockRejectedValueOnce(new Error('API error: 404'))
    fireEvent.click(screen.getByRole('button', { name: 'Level 1' }))
    await screen.findByText(/api error: 404/i)
    expect(screen.getByRole('button', { name: 'Level 1' })).toBeEnabled()
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd fe && npx vitest run src/components/Translator.test.tsx`
Expected: FAIL — no button with accessible name `Level 1` (etc.) exists yet; `getRandomSentence` is still called with a single number, not `undefined`/an array.

- [ ] **Step 3: Write minimal implementation**

In `fe/src/components/Translator.tsx`, after the `interface Props { user: User }` block and before `export default function Translator`, add:

```tsx
const LEVELS = [1, 2, 3, 4, 5]
const SELECTED_LEVELS_STORAGE_KEY = 'eagle:selectedLevels'

function loadStoredLevels(): number[] {
  try {
    const raw = window.localStorage.getItem(SELECTED_LEVELS_STORAGE_KEY)
    if (!raw) return LEVELS
    const parsed = JSON.parse(raw)
    if (Array.isArray(parsed) && parsed.every((n): n is number => typeof n === 'number')) {
      return parsed
    }
  } catch {
    // ignore malformed storage, fall back to default
  }
  return LEVELS
}

function levelsForRequest(levels: number[]): number[] | undefined {
  return levels.length === 0 || levels.length === LEVELS.length ? undefined : levels
}
```

Replace the state declaration:

```tsx
  const [level, setLevel] = useState(0)
```

with:

```tsx
  const [selectedLevels, setSelectedLevels] = useState<number[]>(LEVELS)
```

Replace `getRandomSentence`:

```tsx
  const getRandomSentence = async (levelOverride?: number) => {
    try {
      setLoading(true)
      setError(null)
      const sentence = await api.getRandomSentence(levelOverride ?? level)
      setCurrentSentence(sentence)
      setCorrectCount(sentence.correct_count)
      setIncorrectCount(sentence.incorrect_count)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load sentence')
    } finally {
      setLoading(false)
    }
  }
```

with:

```tsx
  const getRandomSentence = async (levelsOverride?: number[]) => {
    try {
      setLoading(true)
      setError(null)
      const sentence = await api.getRandomSentence(levelsForRequest(levelsOverride ?? selectedLevels))
      setCurrentSentence(sentence)
      setCorrectCount(sentence.correct_count)
      setIncorrectCount(sentence.incorrect_count)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load sentence')
    } finally {
      setLoading(false)
    }
  }
```

Replace `handleLevelChange`:

```tsx
  const handleLevelChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const newLevel = Number(e.target.value)
    setLevel(newLevel)
    resetQuestionState()
    getRandomSentence(newLevel)
  }
```

with:

```tsx
  const toggleLevel = (n: number) => {
    const next = selectedLevels.includes(n)
      ? selectedLevels.filter(l => l !== n)
      : [...selectedLevels, n].sort((a, b) => a - b)
    setSelectedLevels(next)
    window.localStorage.setItem(SELECTED_LEVELS_STORAGE_KEY, JSON.stringify(next))
    resetQuestionState()
    getRandomSentence(next)
  }
```

Replace the mount effect:

```tsx
  useEffect(() => {
    getRandomSentence()
  }, [])
```

with:

```tsx
  useEffect(() => {
    const stored = loadStoredLevels()
    setSelectedLevels(stored)
    getRandomSentence(stored)
  }, [])
```

Replace the `levelSelector` block:

```tsx
  const levelSelector = (
    <select
      aria-label="Sentence difficulty level"
      value={level}
      onChange={handleLevelChange}
      className="h-9 rounded-md border border-input bg-background px-2 text-sm"
    >
      <option value={0}>Any level</option>
      <option value={1}>1</option>
      <option value={2}>2</option>
      <option value={3}>3</option>
      <option value={4}>4</option>
      <option value={5}>5</option>
    </select>
  )
```

with:

```tsx
  const levelToggles = (
    <div role="group" aria-label="Sentence difficulty level" className="flex gap-1">
      {LEVELS.map(n => (
        <Button
          key={n}
          type="button"
          size="sm"
          variant={selectedLevels.includes(n) ? 'default' : 'outline'}
          aria-pressed={selectedLevels.includes(n)}
          aria-label={`Level ${n}`}
          onClick={() => toggleLevel(n)}
          className="h-9 w-9 p-0"
        >
          {n}
        </Button>
      ))}
    </div>
  )
```

Replace the one usage of `{levelSelector}` in the JSX with `{levelToggles}`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd fe && npx vitest run src/components/Translator.test.tsx`
Expected: PASS

- [ ] **Step 5: Run the full frontend test suite to check for regressions**

Run: `cd fe && npm test`
Expected: PASS (all existing tests plus the new ones)

- [ ] **Step 6: Commit**

```bash
git add fe/src/components/Translator.tsx fe/src/components/Translator.test.tsx
git commit -m "feat(fe): replace level dropdown with multi-select toggle buttons"
```

---

### Task 5: e2e — multi-level toggle filtering

**Files:**
- Modify: `e2e/tests/level-filter.spec.ts`

**Interfaces:**
- Consumes: the `aria-label="Level N"` toggle buttons from Task 4 and the `?levels=` contract from Task 2.

- [ ] **Step 1: Update the test**

Replace the entire contents of `e2e/tests/level-filter.spec.ts`:

```ts
import { test, expect } from '@playwright/test'
import { signInAndGetSentence } from './helpers'

test('narrowing the level toggles down to one level only serves sentences at that level', async ({ page }) => {
  await signInAndGetSentence(page)

  await Promise.all([
    page.waitForResponse(
      res => res.url().includes('/api/sentence/random') && res.request().method() === 'GET' && res.ok()
    ),
    page.getByRole('button', { name: 'Level 1' }).click(),
  ])
  await Promise.all([
    page.waitForResponse(
      res => res.url().includes('/api/sentence/random') && res.request().method() === 'GET' && res.ok()
    ),
    page.getByRole('button', { name: 'Level 2' }).click(),
  ])
  await Promise.all([
    page.waitForResponse(
      res => res.url().includes('/api/sentence/random') && res.request().method() === 'GET' && res.ok()
    ),
    page.getByRole('button', { name: 'Level 4' }).click(),
  ])

  const [onlyLevel3] = await Promise.all([
    page.waitForResponse(
      res => res.url().includes('/api/sentence/random?levels=3') && res.request().method() === 'GET' && res.ok()
    ),
    page.getByRole('button', { name: 'Level 5' }).click(),
  ])
  expect((await onlyLevel3.json()).id).toBe(90003)

  const [anyLevel] = await Promise.all([
    page.waitForResponse(
      res =>
        res.url().includes('/api/sentence/random') &&
        !res.url().includes('levels=') &&
        res.request().method() === 'GET' &&
        res.ok()
    ),
    page.getByRole('button', { name: 'Level 3' }).click(),
  ])
  expect(anyLevel.ok()).toBe(true)
})
```

This exercises both rules from the spec: narrowing the selection down to just level 3 (by toggling 1, 2, 4, 5 off) must serve only the level-3 fixture (`id: 90003`), and toggling the last remaining level (3) off — zero selected — must fall back to "any level" (no `levels=` param), matching the all-selected default.

- [ ] **Step 2: Run the test to verify it passes**

Run: `cd e2e && npx playwright test level-filter.spec.ts`
(Assumes the Firestore/Auth emulators and dev servers are already running per the project's e2e setup. If not, run the full suite instead, which handles setup automatically: `cd e2e && npm run test:e2e`.)
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/level-filter.spec.ts
git commit -m "test(e2e): update level-filter spec for multi-select toggles"
```
