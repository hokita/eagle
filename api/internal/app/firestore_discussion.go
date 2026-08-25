package app

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

type discussionQuestionDoc struct {
	QuestionEN   string   `firestore:"question_en"`
	Topic        string   `firestore:"topic"`
	Level        int      `firestore:"level"`
	TargetSkills []string `firestore:"target_skills"`
	IsActive     bool     `firestore:"is_active"`
	CreatedAt    string   `firestore:"created_at"`
	UpdatedAt    string   `firestore:"updated_at"`
}

type discussionMessageDoc struct {
	Role string `firestore:"role"`
	Text string `firestore:"text"`
}

type phraseDoc struct {
	Phrase    string `firestore:"phrase"`
	MeaningEN string `firestore:"meaning_en"`
	ExampleEN string `firestore:"example_en"`
}

type discussionSessionDoc struct {
	QuestionID       int                    `firestore:"question_id"`
	QuestionEN       string                 `firestore:"question_en"`
	Topic            string                 `firestore:"topic"`
	Transcript       []discussionMessageDoc `firestore:"transcript"`
	ReflectionJA     string                 `firestore:"reflection_ja"`
	NaturalEnglish   string                 `firestore:"natural_english"`
	NaturalnessWhyEN string                 `firestore:"naturalness_why_en"`
	NaturalnessFixEN string                 `firestore:"naturalness_fix_en"`
	Phrases          []phraseDoc            `firestore:"phrases"`
	CreatedAt        time.Time              `firestore:"created_at"`
}

func (r *firestoreRepo) userDiscussionSessions(uid string) *firestore.CollectionRef {
	return r.client.Collection("users").Doc(uid).Collection("discussion_sessions")
}

func questionFromDoc(id int, qd *discussionQuestionDoc) *DiscussionQuestion {
	skills := qd.TargetSkills
	if skills == nil {
		skills = []string{}
	}
	return &DiscussionQuestion{
		ID: id, QuestionEN: qd.QuestionEN, Topic: qd.Topic,
		Level: qd.Level, TargetSkills: skills,
	}
}

func (r *firestoreRepo) RandomQuestion(ctx context.Context) (*DiscussionQuestion, error) {
	docs, err := r.client.Collection("discussion_questions").
		Where("is_active", "==", true).Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}
	var candidates []*DiscussionQuestion
	for _, ds := range docs {
		id, convErr := strconv.Atoi(ds.Ref.ID)
		if convErr != nil {
			continue
		}
		var qd discussionQuestionDoc
		if err := ds.DataTo(&qd); err != nil {
			return nil, err
		}
		candidates = append(candidates, questionFromDoc(id, &qd))
	}
	if len(candidates) == 0 {
		return nil, ErrNoCandidate
	}
	return candidates[rand.Intn(len(candidates))], nil
}

func (r *firestoreRepo) GetQuestion(ctx context.Context, id int) (*DiscussionQuestion, error) {
	ds, err := r.client.Collection("discussion_questions").Doc(strconv.Itoa(id)).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var qd discussionQuestionDoc
	if err := ds.DataTo(&qd); err != nil {
		return nil, err
	}
	return questionFromDoc(id, &qd), nil
}

func sessionToDoc(s *DiscussionSession, createdAt time.Time) *discussionSessionDoc {
	transcript := make([]discussionMessageDoc, len(s.Transcript))
	for i, m := range s.Transcript {
		transcript[i] = discussionMessageDoc{Role: m.Role, Text: m.Text}
	}
	phrases := make([]phraseDoc, len(s.Phrases))
	for i, p := range s.Phrases {
		phrases[i] = phraseDoc{Phrase: p.Phrase, MeaningEN: p.MeaningEN, ExampleEN: p.ExampleEN}
	}
	return &discussionSessionDoc{
		QuestionID: s.QuestionID, QuestionEN: s.QuestionEN, Topic: s.Topic,
		Transcript: transcript, ReflectionJA: s.ReflectionJA,
		NaturalEnglish:   s.NaturalEnglish,
		NaturalnessWhyEN: s.NaturalnessWhyEN,
		NaturalnessFixEN: s.NaturalnessFixEN,
		Phrases:          phrases,
		CreatedAt:        createdAt,
	}
}

func sessionFromDoc(id string, sd *discussionSessionDoc) *DiscussionSession {
	transcript := make([]DiscussionMessage, len(sd.Transcript))
	for i, m := range sd.Transcript {
		transcript[i] = DiscussionMessage{Role: m.Role, Text: m.Text}
	}
	// Sessions written before the summary replaced the study/retry flow have
	// no phrases field at all; an empty list keeps the detail view rendering
	// rather than serializing null.
	phrases := make([]Phrase, len(sd.Phrases))
	for i, p := range sd.Phrases {
		phrases[i] = Phrase{Phrase: p.Phrase, MeaningEN: p.MeaningEN, ExampleEN: p.ExampleEN}
	}
	return &DiscussionSession{
		ID: id, QuestionID: sd.QuestionID, QuestionEN: sd.QuestionEN, Topic: sd.Topic,
		Transcript: transcript, ReflectionJA: sd.ReflectionJA,
		NaturalEnglish:   sd.NaturalEnglish,
		NaturalnessWhyEN: sd.NaturalnessWhyEN,
		NaturalnessFixEN: sd.NaturalnessFixEN,
		Phrases:          phrases,
		CreatedAt:        sd.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func (r *firestoreRepo) SaveSession(ctx context.Context, uid string, s *DiscussionSession) (string, error) {
	ref := r.userDiscussionSessions(uid).NewDoc()
	if _, err := ref.Set(ctx, sessionToDoc(s, r.now().UTC())); err != nil {
		return "", err
	}
	return ref.ID, nil
}

func (r *firestoreRepo) ListSessions(ctx context.Context, uid string, limit int) ([]DiscussionSessionSummary, error) {
	it := r.userDiscussionSessions(uid).
		OrderBy("created_at", firestore.Desc).Limit(limit).Documents(ctx)
	summaries := make([]DiscussionSessionSummary, 0)
	for {
		ds, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		var sd discussionSessionDoc
		if err := ds.DataTo(&sd); err != nil {
			return nil, err
		}
		summaries = append(summaries, DiscussionSessionSummary{
			ID: ds.Ref.ID, QuestionEN: sd.QuestionEN, Topic: sd.Topic,
			CreatedAt: sd.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return summaries, nil
}

func (r *firestoreRepo) GetSession(ctx context.Context, uid, id string) (*DiscussionSession, error) {
	ds, err := r.userDiscussionSessions(uid).Doc(id).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var sd discussionSessionDoc
	if err := ds.DataTo(&sd); err != nil {
		return nil, err
	}
	return sessionFromDoc(ds.Ref.ID, &sd), nil
}
