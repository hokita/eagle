package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"

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
	seedSentence(t, client, "303", "1", "C", "C", false) // remains a valid candidate

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
