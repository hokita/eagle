package app

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type savedPhraseDoc struct {
	Phrase     string    `firestore:"phrase"`
	Meaning    string    `firestore:"meaning"`
	Examples   []string  `firestore:"examples"`
	SentenceID int       `firestore:"sentence_id"`
	Japanese   string    `firestore:"japanese"`
	English    string    `firestore:"english"`
	CreatedAt  time.Time `firestore:"created_at"`
}

// savedPhrases lives under the user's own document, so every read and write
// below is confined to that user's phrases by its path.
func (r *firestoreRepo) savedPhrases(uid string) *firestore.CollectionRef {
	return r.client.Collection("users").Doc(uid).Collection("saved_phrases")
}

func (d savedPhraseDoc) toPhrase(id string) SavedPhrase {
	examples := d.Examples
	if examples == nil {
		examples = []string{}
	}
	return SavedPhrase{
		ID:         id,
		Phrase:     d.Phrase,
		Meaning:    d.Meaning,
		Examples:   examples,
		CreatedAt:  d.CreatedAt.UTC().Format(time.RFC3339Nano),
		SentenceID: d.SentenceID,
		Japanese:   d.Japanese,
		English:    d.English,
	}
}

func (r *firestoreRepo) SavePhrase(ctx context.Context, uid string, p SavedPhrase) (*SavedPhrase, error) {
	now := r.now().UTC()
	ref := r.savedPhrases(uid).NewDoc()
	doc := savedPhraseDoc{
		Phrase:     p.Phrase,
		Meaning:    p.Meaning,
		Examples:   p.Examples,
		SentenceID: p.SentenceID,
		Japanese:   p.Japanese,
		English:    p.English,
		CreatedAt:  now,
	}
	if _, err := ref.Set(ctx, doc); err != nil {
		return nil, err
	}
	saved := doc.toPhrase(ref.ID)
	return &saved, nil
}

func (r *firestoreRepo) ListPhrases(ctx context.Context, uid string, limit int) ([]SavedPhrase, error) {
	docs, err := r.savedPhrases(uid).OrderBy("created_at", firestore.Desc).Limit(limit).Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}
	phrases := make([]SavedPhrase, 0, len(docs))
	for _, ds := range docs {
		var d savedPhraseDoc
		if err := ds.DataTo(&d); err != nil {
			return nil, err
		}
		phrases = append(phrases, d.toPhrase(ds.Ref.ID))
	}
	return phrases, nil
}

func (r *firestoreRepo) GetPhrase(ctx context.Context, uid, id string) (*SavedPhrase, error) {
	ds, err := r.savedPhrases(uid).Doc(id).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var d savedPhraseDoc
	if err := ds.DataTo(&d); err != nil {
		return nil, err
	}
	p := d.toPhrase(ds.Ref.ID)
	return &p, nil
}

func (r *firestoreRepo) UpdatePhraseContent(ctx context.Context, uid, id string, content PhraseContent) error {
	_, err := r.savedPhrases(uid).Doc(id).Update(ctx, []firestore.Update{
		{Path: "meaning", Value: content.Meaning},
		{Path: "examples", Value: content.Examples},
	})
	if status.Code(err) == codes.NotFound {
		return ErrNotFound
	}
	return err
}

func (r *firestoreRepo) DeletePhrase(ctx context.Context, uid, id string) error {
	_, err := r.savedPhrases(uid).Doc(id).Delete(ctx, firestore.Exists)
	if status.Code(err) == codes.NotFound {
		return ErrNotFound
	}
	return err
}
