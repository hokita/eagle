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
