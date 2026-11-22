package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

const emulatorProjectID = "eagle-test"

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

func TestFirestoreReportUnknownSentence(t *testing.T) {
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	if err := repo.Report(context.Background(), 999999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFirestoreCorrectAnswerNotFound(t *testing.T) {
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	if _, err := repo.CorrectAnswer(context.Background(), 424242); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

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
		want[i] = fmt.Sprintf("wrong-%d", attempts-1-i)
	}
	return want
}

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
	if hs[0].IncorrectAnswer != want[0] {
		t.Fatalf("expected the most recent attempt to be kept, got %+v", hs[0])
	}
	for _, h := range hs {
		if h.IncorrectAnswer == "wrong-0" {
			t.Fatalf("expected the oldest attempt to be dropped by the query limit, got %+v", hs)
		}
	}
}

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
	seedSentence(t, client, "301", "1", "A", "A", 1, false)
	seedSentence(t, client, "302", "1", "B", "B", 1, true)
	seedSentence(t, client, "303", "1", "C", "C", 1, false)

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

func TestFirestorePhrases(t *testing.T) {
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	ctx := context.Background()
	base := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

	repo.now = func() time.Time { return base }
	first, err := repo.SavePhrase(ctx, "u1", SavedPhrase{
		Phrase: "in the worst case", Examples: []string{}, SentenceID: 3, Japanese: "最悪の場合", English: "At worst.",
	})
	if err != nil {
		t.Fatalf("save first: %v", err)
	}
	repo.now = func() time.Time { return base.Add(time.Minute) }
	second, err := repo.SavePhrase(ctx, "u1", SavedPhrase{
		Phrase: "to a minimum", Meaning: "as little as possible", Examples: []string{"Keep it to a minimum."},
	})
	if err != nil {
		t.Fatalf("save second: %v", err)
	}
	if _, err := repo.SavePhrase(ctx, "u2", SavedPhrase{Phrase: "someone else's"}); err != nil {
		t.Fatalf("save other user: %v", err)
	}

	list, err := repo.ListPhrases(ctx, "u1", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("expected u1's two phrases newest first, got %+v", list)
	}
	if list[1].English != "At worst." || list[1].Examples == nil || list[0].CreatedAt != base.Add(time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("unexpected stored fields: %+v", list)
	}

	if err := repo.UpdatePhraseContent(ctx, "u1", first.ID, PhraseContent{Meaning: "if the worst happens", Examples: []string{"In the worst case, we walk."}}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := repo.GetPhrase(ctx, "u1", first.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Meaning != "if the worst happens" || len(got.Examples) != 1 || got.Japanese != "最悪の場合" {
		t.Fatalf("unexpected after update: %+v", got)
	}

	if _, err := repo.GetPhrase(ctx, "u2", first.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for another user's phrase, got %v", err)
	}
	if err := repo.DeletePhrase(ctx, "u2", first.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound deleting another user's phrase, got %v", err)
	}
	if err := repo.UpdatePhraseContent(ctx, "u1", "missing", PhraseContent{}); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound updating a missing phrase, got %v", err)
	}

	if err := repo.DeletePhrase(ctx, "u1", first.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := repo.DeletePhrase(ctx, "u1", first.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound deleting twice, got %v", err)
	}
	if list, _ := repo.ListPhrases(ctx, "u1", 10); len(list) != 1 {
		t.Fatalf("expected one phrase left, got %d", len(list))
	}
}
