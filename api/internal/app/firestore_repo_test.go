package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

const emulatorProjectID = "eagle-test"

// clearFirestoreEmulator wipes all documents so each test starts from an
// empty database. The emulator is a long-lived process shared across test
// runs, so without this, documents from earlier runs (e.g. counters,
// histories) leak into later assertions.
func clearFirestoreEmulator(t *testing.T, host string) {
	t.Helper()
	url := fmt.Sprintf("http://%s/emulator/v1/projects/%s/databases/(default)/documents", host, emulatorProjectID)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatalf("build clear-emulator request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("clear emulator data: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear emulator data: unexpected status %d", resp.StatusCode)
	}
}

func newEmulatorClient(t *testing.T) *firestore.Client {
	t.Helper()
	host := os.Getenv("FIRESTORE_EMULATOR_HOST")
	if host == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set; skipping emulator test")
	}
	clearFirestoreEmulator(t, host)
	client, err := firestore.NewClient(context.Background(), emulatorProjectID)
	if err != nil {
		t.Fatalf("firestore client: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func seedSentence(t *testing.T, client *firestore.Client, id, page string, jp, en string, level int, reported bool) {
	t.Helper()
	_, err := client.Collection("sentences").Doc(id).Set(context.Background(), map[string]interface{}{
		"japanese": jp, "english": en, "page": page, "level": level, "is_reported": reported,
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

// seedManyWrongAnswers records attempts incorrect attempts at the given
// sentence for uid, one minute apart starting at base, and returns the
// expected newest-first order of their IncorrectAnswer text.
func seedManyWrongAnswers(t *testing.T, ctx context.Context, repo *firestoreRepo, uid string, sentenceID int, base time.Time, attempts int) []string {
	t.Helper()
	for i := 0; i < attempts; i++ {
		repo.now = func(i int) func() time.Time {
			return func() time.Time { return base.Add(time.Duration(i) * time.Minute) }
		}(i)
		if err := repo.RecordAnswer(ctx, uid, sentenceID, false, fmt.Sprintf("wrong-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	want := make([]string, attempts)
	for i := 0; i < attempts; i++ {
		want[i] = fmt.Sprintf("wrong-%d", attempts-1-i) // newest (highest i) first
	}
	return want
}

// TestFirestoreListMistakesForInsightBoundedAtQueryLevel is a regression
// test: without a Firestore-side limit, a learner who repeatedly submits
// wrong answers to the same sentence forces an ever-larger read (and an
// ever-larger weakness-insight prompt) on every request. The cap must live
// in the query itself, not just be applied to the results in Go after
// they've already been read — but only for the insight-specific path (see
// TestFirestoreListIncorrectHistoriesRemainsUnboundedForCheckAnswer and
// TestFirestoreListMistakesRemainsUnboundedForRawList below, which guard
// against that cap leaking into the unrelated existing-history APIs).
func TestFirestoreListMistakesForInsightBoundedAtQueryLevel(t *testing.T) {
	ctx := context.Background()
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	uid := "user-many-wrong"
	seedSentence(t, client, "301", "1", "多い間違い", "Many mistakes", 1, false)

	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	const attempts = maxWrongAnswersPerSentence + 3
	want := seedManyWrongAnswers(t, ctx, repo, uid, 301, base, attempts)

	mistakes, err := repo.ListMistakesForInsight(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(mistakes) != 1 {
		t.Fatalf("expected 1 mistaken sentence, got %d: %+v", len(mistakes), mistakes)
	}
	hs := mistakes[0].WrongAnswers
	if len(hs) != maxWrongAnswersPerSentence {
		t.Fatalf("expected the query itself to cap results at %d, got %d: %+v", maxWrongAnswersPerSentence, len(hs), hs)
	}
	// Newest first: want[0] is the most recently recorded attempt.
	if hs[0].IncorrectAnswer != want[0] {
		t.Fatalf("expected the most recent attempt to be kept, got %+v", hs[0])
	}
	for _, h := range hs {
		if h.IncorrectAnswer == "wrong-0" {
			t.Fatalf("expected the oldest attempt to be dropped by the query limit, got %+v", hs)
		}
	}
}

// TestFirestoreListMistakesForInsightBoundsSentenceScanAtQueryLevel is a
// regression test: TestFirestoreListMistakesForInsightBoundedAtQueryLevel
// above only caps wrong-answer history per sentence. Without a separate
// cap on the outer sentence_stats scan, a learner who has ever missed many
// distinct sentences still forces a read (stats doc + sentence doc per
// mistake) that scales with their entire lifetime mistake count on every
// insight request. The cap must live in the query itself.
func TestFirestoreListMistakesForInsightBoundsSentenceScanAtQueryLevel(t *testing.T) {
	ctx := context.Background()
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	uid := "user-many-sentences"

	const extra = 5
	total := maxInsightStatsScan + extra
	base := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < total; i++ {
		id := 2000 + i
		seedSentence(t, client, strconv.Itoa(id), "1", fmt.Sprintf("文%d", i), fmt.Sprintf("sentence %d", i), 1, false)
		repo.now = func(i int) func() time.Time {
			return func() time.Time { return base.Add(time.Duration(i) * time.Minute) }
		}(i)
		if err := repo.RecordAnswer(ctx, uid, id, false, "wrong"); err != nil {
			t.Fatal(err)
		}
	}

	mistakes, err := repo.ListMistakesForInsight(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(mistakes) != maxInsightStatsScan {
		t.Fatalf("expected the sentence scan itself to be capped at %d, got %d", maxInsightStatsScan, len(mistakes))
	}
}

// TestFirestoreListMistakesForInsightSkipsCorrectOnlyDocsWhenScanning is a
// regression test for a Codex review finding: the scan cap applied
// Limit(scanLimit) to the raw doc scan before the IncorrectCount==0 filter,
// so enough more-recently-touched correct-only sentences could crowd every
// actual mistake out of the capped window — ListMistakesForInsight would
// return empty (suppressing the insight entirely) even though the learner
// has a real, older mistake still visible on the raw /api/mistakes list.
func TestFirestoreListMistakesForInsightSkipsCorrectOnlyDocsWhenScanning(t *testing.T) {
	ctx := context.Background()
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	uid := "user-crowded-out"

	seedSentence(t, client, "3000", "1", "古い間違い", "old mistake", 1, false)
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	repo.now = func() time.Time { return base }
	if err := repo.RecordAnswer(ctx, uid, 3000, false, "wrong"); err != nil {
		t.Fatal(err)
	}

	// More sentences than maxInsightStatsScan, all answered correctly and all
	// touched more recently than the mistake above.
	for i := 0; i < maxInsightStatsScan+10; i++ {
		id := 3001 + i
		seedSentence(t, client, strconv.Itoa(id), "1", fmt.Sprintf("正解%d", i), fmt.Sprintf("correct %d", i), 1, false)
		repo.now = func(i int) func() time.Time {
			return func() time.Time { return base.Add(time.Duration(i+1) * time.Minute) }
		}(i)
		if err := repo.RecordAnswer(ctx, uid, id, true, ""); err != nil {
			t.Fatal(err)
		}
	}

	mistakes, err := repo.ListMistakesForInsight(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(mistakes) != 1 {
		t.Fatalf("expected the older mistake to still be found despite %d more-recent correct-only touches, got %d mistakes: %+v", maxInsightStatsScan+10, len(mistakes), mistakes)
	}
	if mistakes[0].SentenceID != 3000 {
		t.Fatalf("expected sentence 3000, got %+v", mistakes[0])
	}
}

// TestFirestoreListMistakesSentenceScanRemainsUnboundedForRawList is a
// regression test for the same bug: the insight-only scan cap must not leak
// into ListMistakes, which backs the raw /api/mistakes list and has always
// shown every sentence a learner has ever missed.
func TestFirestoreListMistakesSentenceScanRemainsUnboundedForRawList(t *testing.T) {
	ctx := context.Background()
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	uid := "user-many-sentences"

	const extra = 5
	total := maxInsightStatsScan + extra
	base := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < total; i++ {
		id := 2000 + i
		seedSentence(t, client, strconv.Itoa(id), "1", fmt.Sprintf("文%d", i), fmt.Sprintf("sentence %d", i), 1, false)
		repo.now = func(i int) func() time.Time {
			return func() time.Time { return base.Add(time.Duration(i) * time.Minute) }
		}(i)
		if err := repo.RecordAnswer(ctx, uid, id, false, "wrong"); err != nil {
			t.Fatal(err)
		}
	}

	mistakes, err := repo.ListMistakes(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(mistakes) != total {
		t.Fatalf("expected all %d mistaken sentences (unbounded), got %d", total, len(mistakes))
	}
}

// TestFirestoreListIncorrectHistoriesRemainsUnboundedForCheckAnswer is a
// regression test for a bug caught in PR review: the insight path's query
// limit must not leak into ListIncorrectHistories, which backs the
// "Previous Incorrect Answers" panel shown after checking an answer. That
// panel has always shown a learner's complete history for the current
// sentence, unrelated to weakness-insight's Gemini cost bound.
func TestFirestoreListIncorrectHistoriesRemainsUnboundedForCheckAnswer(t *testing.T) {
	ctx := context.Background()
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	uid := "user-many-wrong"
	seedSentence(t, client, "301", "1", "多い間違い", "Many mistakes", 1, false)

	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	const attempts = maxWrongAnswersPerSentence + 3
	want := seedManyWrongAnswers(t, ctx, repo, uid, 301, base, attempts)

	hs, err := repo.ListIncorrectHistories(ctx, uid, 301)
	if err != nil {
		t.Fatal(err)
	}
	if len(hs) != attempts {
		t.Fatalf("expected all %d attempts (unbounded), got %d: %+v", attempts, len(hs), hs)
	}
	for i, h := range hs {
		if h.IncorrectAnswer != want[i] {
			t.Fatalf("attempt %d: expected %q, got %q", i, want[i], h.IncorrectAnswer)
		}
	}
}

// TestFirestoreListMistakesRemainsUnboundedForRawList is a regression test
// for the same bug: ListMistakes backs the plain /api/mistakes list page,
// which has always shown a learner's complete wrong-answer history per
// sentence. Only the insight-specific ListMistakesForInsight bounds it.
func TestFirestoreListMistakesRemainsUnboundedForRawList(t *testing.T) {
	ctx := context.Background()
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	uid := "user-many-wrong"
	seedSentence(t, client, "301", "1", "多い間違い", "Many mistakes", 1, false)

	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	const attempts = maxWrongAnswersPerSentence + 3
	seedManyWrongAnswers(t, ctx, repo, uid, 301, base, attempts)

	mistakes, err := repo.ListMistakes(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(mistakes) != 1 {
		t.Fatalf("expected 1 mistaken sentence, got %d: %+v", len(mistakes), mistakes)
	}
	if len(mistakes[0].WrongAnswers) != attempts {
		t.Fatalf("expected all %d attempts (unbounded), got %d: %+v", attempts, len(mistakes[0].WrongAnswers), mistakes[0].WrongAnswers)
	}
}

func TestFirestoreRecordListAndCount(t *testing.T) {
	ctx := context.Background()
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	uid := "user-record"
	seedSentence(t, client, "201", "5", "こんにちは", "Hello", 1, false)

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

	s, err := repo.RandomCandidate(ctx, uid, nil)
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
	seedSentence(t, client, "301", "1", "A", "A", 1, false) // mastered below
	seedSentence(t, client, "302", "1", "B", "B", 1, true)  // reported
	seedSentence(t, client, "303", "1", "C", "C", 1, false) // remains a valid candidate

	// Push 301 to net +2 (correct - incorrect >= 2) -> excluded.
	if err := repo.RecordAnswer(ctx, uid, 301, true, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordAnswer(ctx, uid, 301, true, ""); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 8; i++ {
		s, err := repo.RandomCandidate(ctx, uid, nil)
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

func TestFirestoreRandomCandidateFiltersByLevel(t *testing.T) {
	ctx := context.Background()
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	uid := "user-level"
	seedSentence(t, client, "401", "1", "A", "A", 1, false)
	seedSentence(t, client, "402", "1", "B", "B", 3, false)
	seedSentence(t, client, "403", "1", "C", "C", 3, false)

	for i := 0; i < 5; i++ {
		s, err := repo.RandomCandidate(ctx, uid, []int{1})
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if s.ID != 401 {
			t.Fatalf("expected only level-1 sentence 401, got %d", s.ID)
		}
	}

	for i := 0; i < 8; i++ {
		s, err := repo.RandomCandidate(ctx, uid, []int{3})
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if s.ID != 402 && s.ID != 403 {
			t.Fatalf("expected level-3 sentence 402 or 403, got %d", s.ID)
		}
	}
}

func TestFirestoreRandomCandidateLevelZeroIncludesUnleveledDocs(t *testing.T) {
	ctx := context.Background()
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	uid := "user-unleveled"
	// Simulates a pre-backfill document seeded before the level field existed:
	// no "level" key at all, not even a zero value.
	_, err := client.Collection("sentences").Doc("501").Set(ctx, map[string]interface{}{
		"japanese": "X", "english": "X", "page": "1", "is_reported": false,
		"created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("seed unleveled sentence: %v", err)
	}

	s, err := repo.RandomCandidate(ctx, uid, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != 501 {
		t.Fatalf("expected unleveled sentence 501 under level=0, got %d", s.ID)
	}

	if _, err := repo.RandomCandidate(ctx, uid, []int{2}); err != ErrNoCandidate {
		t.Fatalf("expected ErrNoCandidate excluding unleveled doc under level=2, got %v", err)
	}
}

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

func TestFirestoreListMistakesOrdersBySubSecondPrecisionWithinSameSecond(t *testing.T) {
	ctx := context.Background()
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	uid := "user-subsecond"
	// IDs chosen so ascending doc-ID order (Firestore's default iteration
	// order for an unordered collection scan) is the OPPOSITE of the correct
	// time-based order, so this test only passes if sub-second precision is
	// actually preserved through storage and the sort, not by coincidence.
	seedSentence(t, client, "1001", "1", "A", "A-en", 1, false)
	seedSentence(t, client, "1002", "1", "B", "B-en", 1, false)

	sameSecond := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	repo.now = func() time.Time { return sameSecond.Add(100 * time.Millisecond) }
	if err := repo.RecordAnswer(ctx, uid, 1001, false, "wrong A"); err != nil {
		t.Fatal(err)
	}
	repo.now = func() time.Time { return sameSecond.Add(900 * time.Millisecond) }
	if err := repo.RecordAnswer(ctx, uid, 1002, false, "wrong B"); err != nil {
		t.Fatal(err)
	}

	mistakes, err := repo.ListMistakes(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(mistakes) != 2 {
		t.Fatalf("expected 2 mistaken sentences, got %d: %+v", len(mistakes), mistakes)
	}
	// Both wrong answers land in the same wall-clock second (12:00:00), but
	// 1002's is 800ms later. Sub-second precision must survive storage and
	// the sort for 1002 to correctly rank first.
	if mistakes[0].SentenceID != 1002 {
		t.Fatalf("expected sentence 1002 first (later within the same second), got %+v", mistakes)
	}
	if mistakes[1].SentenceID != 1001 {
		t.Fatalf("expected sentence 1001 second, got %+v", mistakes)
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
