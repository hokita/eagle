package app

import (
	"context"
	"errors"
	"math/rand"
	"sort"
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
	Level      int    `firestore:"level"`
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

func (r *firestoreRepo) RandomCandidate(ctx context.Context, uid string, levels []int) (*Sentence, error) {
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

	wantLevels := map[int]bool{}
	for _, lv := range levels {
		wantLevels[lv] = true
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
		if len(wantLevels) > 0 && !wantLevels[sd.Level] {
			continue
		}
		candidates = append(candidates, &Sentence{
			ID:             id,
			Japanese:       sd.Japanese,
			English:        sd.English,
			Page:           sd.Page,
			Level:          sd.Level,
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

func (r *firestoreRepo) GetSentence(ctx context.Context, id int) (string, string, error) {
	ds, err := r.client.Collection("sentences").Doc(strconv.Itoa(id)).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	var sd sentenceDoc
	if err := ds.DataTo(&sd); err != nil {
		return "", "", err
	}
	return sd.Japanese, sd.English, nil
}

// incorrectHistories reads a sentence's wrong-answer history, newest first.
// limit > 0 applies Limit(limit) at the query level, bounding the read
// itself rather than just the slice returned to the caller; limit <= 0
// leaves the read unbounded. Only the insight-specific path passes a limit
// — ListIncorrectHistories and ListMistakes stay unbounded, since they back
// UI that has always shown a learner's complete history.
func (r *firestoreRepo) incorrectHistories(ctx context.Context, statsRef *firestore.DocumentRef, limit int) ([]AnswerHistory, error) {
	q := statsRef.Collection("histories").
		Where("is_correct", "==", false).
		OrderBy("created_at", firestore.Desc)
	if limit > 0 {
		q = q.Limit(limit)
	}
	it := q.Documents(ctx)
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
			CreatedAt:       hd.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return histories, nil
}

func (r *firestoreRepo) ListIncorrectHistories(ctx context.Context, uid string, id int) ([]AnswerHistory, error) {
	return r.incorrectHistories(ctx, r.userStats(uid).Doc(strconv.Itoa(id)), 0)
}

// maxInsightStatsScan bounds, at the Firestore query level, how many of a
// user's most-recently-touched sentence_stats docs the insight path scans.
// Without it, a learner who has ever missed many distinct sentences forces a
// stats-doc + sentence-doc read per mistake that scales with their entire
// lifetime activity on every insight request, regardless of maxInsightMistakes
// (which only truncates the slice in Go after all of those reads already
// happened). "Most-recently-touched" (ordered by updated_at, which every
// RecordAnswer call bumps whether the attempt was correct or not) is an
// approximation of "most recently mistaken" — good enough for a weakness
// summary, and avoids requiring a composite index the way filtering on
// incorrect_count while ordering by updated_at would.
const maxInsightStatsScan = 100

// listMistakes is the shared implementation behind ListMistakes and
// ListMistakesForInsight; historyLimit is threaded straight into
// incorrectHistories (see its doc comment for the limit<=0 convention).
// scanLimit bounds the outer sentence_stats scan itself at the query level
// (see maxInsightStatsScan); scanLimit<=0 leaves it unbounded.
func (r *firestoreRepo) listMistakes(ctx context.Context, uid string, historyLimit, scanLimit int) ([]MistakeSentence, error) {
	mistakes := make([]MistakeSentence, 0)
	statsQuery := r.userStats(uid).Query
	if scanLimit > 0 {
		statsQuery = statsQuery.OrderBy("updated_at", firestore.Desc).Limit(scanLimit)
	}
	it := statsQuery.Documents(ctx)
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

		wrongAnswers, err := r.incorrectHistories(ctx, ds.Ref, historyLimit)
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

// ListMistakes returns every sentence the user has ever answered
// incorrectly, with its complete wrong-answer history, most recently missed
// sentence first. Backs the raw /api/mistakes list.
func (r *firestoreRepo) ListMistakes(ctx context.Context, uid string) ([]MistakeSentence, error) {
	return r.listMistakes(ctx, uid, 0, 0)
}

// ListMistakesForInsight is the same as ListMistakes but caps each
// sentence's wrong-answer history to maxWrongAnswersPerSentence and the
// outer sentence scan to maxInsightStatsScan, both at the query level.
// Backs GET /api/mistakes/insight only.
func (r *firestoreRepo) ListMistakesForInsight(ctx context.Context, uid string) ([]MistakeSentence, error) {
	return r.listMistakes(ctx, uid, maxWrongAnswersPerSentence, maxInsightStatsScan)
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
