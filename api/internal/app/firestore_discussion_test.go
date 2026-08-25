package app

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

func seedDiscussionQuestion(t *testing.T, client *firestore.Client, id int, question, topic string, level int, active bool) {
	t.Helper()
	_, err := client.Collection("discussion_questions").Doc(strconv.Itoa(id)).Set(context.Background(), map[string]interface{}{
		"question_en": question, "topic": topic, "level": level,
		"target_skills": []string{"giving opinions"}, "is_active": active,
		"created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("seed discussion question %d: %v", id, err)
	}
}

func sampleSession() *DiscussionSession {
	return &DiscussionSession{
		QuestionID: 1,
		QuestionEN: "Who should take more responsibility for environmental problems?",
		Topic:      "environment",
		Transcript: []DiscussionMessage{
			{Role: "user", Text: "I think companies."},
			{Role: "ai", Text: "Why do you think so?"},
			{Role: "user", Text: "Because they pollute more."},
		},
		ReflectionJA:     "制度を変える必要がある。",
		NaturalEnglish:   "I think companies are responsible, because they pollute more than anyone else.",
		NaturalnessWhyEN: "You repeated \"I think\" at the start of every turn.",
		NaturalnessFixEN: "Vary your opener — try \"For me,\" or just state it.",
		Phrases: []Phrase{
			{Phrase: "take responsibility for", MeaningEN: "to accept that something is your job or your fault", ExampleEN: "Companies should take responsibility for pollution."},
		},
	}
}

func TestFirestoreRandomQuestionFiltersInactive(t *testing.T) {
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	seedDiscussionQuestion(t, client, 1, "Active question?", "work", 2, true)
	seedDiscussionQuestion(t, client, 2, "Inactive question?", "work", 2, false)

	for i := 0; i < 5; i++ {
		q, err := repo.RandomQuestion(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if q.ID != 1 || q.QuestionEN != "Active question?" {
			t.Fatalf("expected only the active question, got %+v", q)
		}
	}
}

func TestFirestoreRandomQuestionEmpty(t *testing.T) {
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	if _, err := repo.RandomQuestion(context.Background()); err != ErrNoCandidate {
		t.Fatalf("expected ErrNoCandidate, got %v", err)
	}
}

func TestFirestoreGetQuestion(t *testing.T) {
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	seedDiscussionQuestion(t, client, 7, "A question?", "travel", 3, true)

	q, err := repo.GetQuestion(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.ID != 7 || q.Topic != "travel" || q.Level != 3 || len(q.TargetSkills) != 1 {
		t.Fatalf("unexpected question: %+v", q)
	}
	if _, err := repo.GetQuestion(context.Background(), 424242); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFirestoreSaveAndGetSession(t *testing.T) {
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	repo.now = func() time.Time { return time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC) }

	id, err := repo.SaveSession(context.Background(), "u1", sampleSession())
	if err != nil {
		t.Fatalf("save session: %v", err)
	}
	if id == "" {
		t.Fatal("expected a non-empty session id")
	}

	got, err := repo.GetSession(context.Background(), "u1", id)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.ID != id || got.QuestionID != 1 || len(got.Transcript) != 3 ||
		got.ReflectionJA != "制度を変える必要がある。" ||
		!strings.Contains(got.NaturalEnglish, "pollute more") ||
		!strings.Contains(got.NaturalnessWhyEN, "every turn") ||
		!strings.Contains(got.NaturalnessFixEN, "Vary your opener") ||
		len(got.Phrases) != 1 ||
		got.Phrases[0].Phrase != "take responsibility for" ||
		got.CreatedAt != "2026-08-23T10:00:00Z" {
		t.Fatalf("unexpected session: %+v", got)
	}
}

func TestFirestoreGetSessionIsolatedPerUser(t *testing.T) {
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	id, err := repo.SaveSession(context.Background(), "u1", sampleSession())
	if err != nil {
		t.Fatalf("save session: %v", err)
	}
	if _, err := repo.GetSession(context.Background(), "u2", id); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for another user's session, got %v", err)
	}
}

func TestFirestoreListSessionsNewestFirstWithLimit(t *testing.T) {
	client := newEmulatorClient(t)
	repo := NewFirestoreRepo(client)
	base := time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		i := i
		repo.now = func() time.Time { return base.Add(time.Duration(i) * time.Minute) }
		s := sampleSession()
		s.QuestionEN = "Q" + strconv.Itoa(i)
		if _, err := repo.SaveSession(context.Background(), "u1", s); err != nil {
			t.Fatalf("save session %d: %v", i, err)
		}
	}

	got, err := repo.ListSessions(context.Background(), "u1", 2)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(got) != 2 || got[0].QuestionEN != "Q2" || got[1].QuestionEN != "Q1" {
		t.Fatalf("expected newest-first capped list, got %+v", got)
	}
	if got[0].Topic != "environment" || got[0].CreatedAt == "" || got[0].ID == "" {
		t.Fatalf("summary fields missing: %+v", got[0])
	}
}
