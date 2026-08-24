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

type expressionDoc struct {
	Phrase    string `firestore:"phrase"`
	MeaningJA string `firestore:"meaning_ja"`
	ExampleEN string `firestore:"example_en"`
}

type discussionSessionDoc struct {
	QuestionID     int                    `firestore:"question_id"`
	QuestionEN     string                 `firestore:"question_en"`
	Topic          string                 `firestore:"topic"`
	Transcript     []discussionMessageDoc `firestore:"transcript"`
	ReflectionJA   string                 `firestore:"reflection_ja"`
	ExpressedIdeas []string               `firestore:"expressed_ideas"`
	MissingIdeas   []string               `firestore:"missing_ideas"`
	Expressions    []expressionDoc        `firestore:"expressions"`
	FirstAnswer    string                 `firestore:"first_answer"`
	RetryAnswer    string                 `firestore:"retry_answer"`
	RetryFeedback  string                 `firestore:"retry_feedback"`
	CreatedAt      time.Time              `firestore:"created_at"`
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
	expressions := make([]expressionDoc, len(s.Expressions))
	for i, e := range s.Expressions {
		expressions[i] = expressionDoc{Phrase: e.Phrase, MeaningJA: e.MeaningJA, ExampleEN: e.ExampleEN}
	}
	return &discussionSessionDoc{
		QuestionID: s.QuestionID, QuestionEN: s.QuestionEN, Topic: s.Topic,
		Transcript: transcript, ReflectionJA: s.ReflectionJA,
		ExpressedIdeas: append([]string{}, s.ExpressedIdeas...),
		MissingIdeas:   append([]string{}, s.MissingIdeas...),
		Expressions:    expressions,
		FirstAnswer:    s.FirstAnswer, RetryAnswer: s.RetryAnswer,
		RetryFeedback: s.RetryFeedback, CreatedAt: createdAt,
	}
}

func sessionFromDoc(id string, sd *discussionSessionDoc) *DiscussionSession {
	transcript := make([]DiscussionMessage, len(sd.Transcript))
	for i, m := range sd.Transcript {
		transcript[i] = DiscussionMessage{Role: m.Role, Text: m.Text}
	}
	expressions := make([]Expression, len(sd.Expressions))
	for i, e := range sd.Expressions {
		expressions[i] = Expression{Phrase: e.Phrase, MeaningJA: e.MeaningJA, ExampleEN: e.ExampleEN}
	}
	expressed := sd.ExpressedIdeas
	if expressed == nil {
		expressed = []string{}
	}
	missing := sd.MissingIdeas
	if missing == nil {
		missing = []string{}
	}
	return &DiscussionSession{
		ID: id, QuestionID: sd.QuestionID, QuestionEN: sd.QuestionEN, Topic: sd.Topic,
		Transcript: transcript, ReflectionJA: sd.ReflectionJA,
		ExpressedIdeas: expressed, MissingIdeas: missing, Expressions: expressions,
		FirstAnswer: sd.FirstAnswer, RetryAnswer: sd.RetryAnswer,
		RetryFeedback: sd.RetryFeedback,
		CreatedAt:     sd.CreatedAt.UTC().Format(time.RFC3339),
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
