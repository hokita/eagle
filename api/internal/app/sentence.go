package app

import (
	"context"
	"errors"
)

type Sentence struct {
	ID             int    `json:"id"`
	Japanese       string `json:"japanese"`
	English        string `json:"english"`
	Page           string `json:"page"`
	Level          int    `json:"level"`
	CorrectCount   int    `json:"correct_count"`
	IncorrectCount int    `json:"incorrect_count"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type AnswerHistory struct {
	ID              int64  `json:"id"`
	IncorrectAnswer string `json:"incorrect_answer"`
	CreatedAt       string `json:"created_at"`
}

type MistakeSentence struct {
	SentenceID    int             `json:"sentence_id"`
	Japanese      string          `json:"japanese"`
	CorrectAnswer string          `json:"correct_answer"`
	WrongAnswers  []AnswerHistory `json:"wrong_answers"`
}

type ListMistakesResponse struct {
	Mistakes []MistakeSentence `json:"mistakes"`
}

type MistakesInsightResponse struct {
	Insight string `json:"insight"`
}

type CheckAnswerRequest struct {
	SentenceID int    `json:"sentence_id"`
	UserAnswer string `json:"user_answer"`
}

type CheckAnswerResponse struct {
	IsCorrect     bool            `json:"is_correct"`
	CorrectAnswer string          `json:"correct_answer"`
	Histories     []AnswerHistory `json:"histories"`
}

type ReportSentenceRequest struct {
	SentenceID int `json:"sentence_id"`
}

type ExplainRequest struct {
	SentenceID int    `json:"sentence_id"`
	UserAnswer string `json:"user_answer"`
	Language   string `json:"language"`
}

type ExplainResponse struct {
	Explanation string `json:"explanation"`
}

// ErrNotFound is returned when a sentence document does not exist.
var ErrNotFound = errors.New("sentence not found")

// ErrNoCandidate is returned when no sentence passes the random filter.
var ErrNoCandidate = errors.New("no candidate sentence")

// SentenceRepository is the data-access seam behind the HTTP handlers.
type SentenceRepository interface {
	// RandomCandidate returns a random non-mastered, non-reported sentence.
	// levels restricts candidates to sentences whose Level is in the set;
	// an empty levels means "any level" (no filtering), including sentences
	// with no level set.
	RandomCandidate(ctx context.Context, uid string, levels []int) (*Sentence, error)
	CorrectAnswer(ctx context.Context, id int) (string, error)
	GetSentence(ctx context.Context, id int) (japanese, english string, err error)
	ListIncorrectHistories(ctx context.Context, uid string, id int) ([]AnswerHistory, error)
	// ListMistakes returns every sentence the user has ever answered
	// incorrectly, most recently missed sentence first.
	ListMistakes(ctx context.Context, uid string) ([]MistakeSentence, error)
	RecordAnswer(ctx context.Context, uid string, id int, correct bool, answer string) error
	Report(ctx context.Context, id int) error
}
